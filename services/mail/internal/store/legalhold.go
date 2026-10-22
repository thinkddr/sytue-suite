// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Legal hold + eDiscovery (compliance NO destructivo). Ver migración 0009.
//
// INVARIANTE: mientras un sujeto tiene un hold ACTIVO, su correo NUNCA se
// elimina físicamente. El borrado del usuario (soft-delete, messages.deleted_at)
// lo sigue ocultando de la bandeja, pero:
//   - PurgeDeleted (purga/retención/expiración) SALTA los mensajes de sujetos
//     bajo hold (NOT EXISTS legal_holds activo).
//   - DeleteMailbox (cuyo cascade borraría físicamente los mensajes de la
//     carpeta) se niega si la cuenta está bajo hold y la carpeta aún los guarda.
//
// Al liberar el hold, el correo vuelve al ciclo normal (purgable).
//
// Aislamiento por org: todo corre bajo InTenantTx (RLS por app.tenant). Un admin
// jamás resuelve ni busca correo/holds de otra org.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// AccountByAddress resuelve una cuenta por su dirección principal, acotada al
// tenant del contexto (RLS + predicado explícito de tenant, como AccountBySubject).
// La usa eDiscovery para localizar al usuario objetivo por email dentro de la org
// del admin; una dirección de otra org devuelve ErrNotFound (aislamiento).
func (s *Store) AccountByAddress(ctx context.Context, address string) (domain.Account, error) {
	var acc domain.Account
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, tenant, subject, address, display_name, quota_bytes, used_bytes, created_at, updated_at
			FROM accounts WHERE tenant = $1 AND lower(address) = lower($2)`, tenantOf(ctx), address)
		var err error
		acc, err = scanAccount(row)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, domain.ErrNotFound
	}
	return acc, err
}

// ---------------------------------------------------------------------------
// Legal holds
// ---------------------------------------------------------------------------

// CreateHold crea una orden de preservación y AUDITA la creación en la misma
// transacción (atómico: no hay hold sin traza). Subject vacío => hold de toda la
// org. Corre bajo el tenant del contexto.
func (s *Store) CreateHold(ctx context.Context, h domain.LegalHold) (domain.LegalHold, error) {
	tenant := tenantOf(ctx)
	var out domain.LegalHold
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO legal_holds (tenant, subject, reason, case_ref, created_by)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, tenant, subject, reason, case_ref, created_by, created_at, released_at, released_by`,
			tenant, nilIfEmpty(h.Subject), h.Reason, h.Case, h.CreatedBy)
		var err error
		out, err = scanHold(row)
		if err != nil {
			return fmt.Errorf("store: create hold: %w", err)
		}
		return writeAuditTx(ctx, tx, tenant, h.CreatedBy, "legalhold.create", h.Subject, map[string]any{
			"hold_id":  out.ID,
			"case":     h.Case,
			"reason":   h.Reason,
			"org_wide": out.OrgWide(),
		})
	})
	return out, err
}

// ReleaseHold libera un hold activo (por id, acotado al tenant) y AUDITA la
// liberación en la misma transacción. ErrNotFound si no existe o ya estaba
// liberado. Tras liberar, el correo del sujeto vuelve al ciclo normal.
func (s *Store) ReleaseHold(ctx context.Context, id uuid.UUID, actor string) error {
	tenant := tenantOf(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var subject *string
		err := tx.QueryRow(ctx, `
			UPDATE legal_holds SET released_at = now(), released_by = $3
			WHERE id = $1 AND tenant = $2 AND released_at IS NULL
			RETURNING subject`, id, tenant, actor).Scan(&subject)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("store: release hold: %w", err)
		}
		target := ""
		if subject != nil {
			target = *subject
		}
		return writeAuditTx(ctx, tx, tenant, actor, "legalhold.release", target, map[string]any{
			"hold_id": id,
		})
	})
}

// ListHolds lista los holds del tenant (activos primero por created_at DESC).
// includeReleased=false devuelve solo los activos (vista por defecto).
func (s *Store) ListHolds(ctx context.Context, includeReleased bool) ([]domain.LegalHold, error) {
	q := `
		SELECT id, tenant, subject, reason, case_ref, created_by, created_at, released_at, released_by
		FROM legal_holds WHERE tenant = $1`
	if !includeReleased {
		q += ` AND released_at IS NULL`
	}
	q += ` ORDER BY created_at DESC`
	var out []domain.LegalHold
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, q, tenantOf(ctx))
		if err != nil {
			return fmt.Errorf("store: list holds: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			h, err := scanHold(rows)
			if err != nil {
				return err
			}
			out = append(out, h)
		}
		return rows.Err()
	})
	return out, err
}

// isAccountHeldTx indica si la cuenta está cubierta por algún hold ACTIVO (suyo o
// de toda la org). Lo consulta DeleteMailbox antes de un borrado que cascadearía
// a los mensajes de la carpeta.
func isAccountHeldTx(ctx context.Context, tx pgx.Tx, accountID uuid.UUID) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM legal_holds h
			JOIN accounts a ON a.tenant = h.tenant AND (h.subject IS NULL OR h.subject = a.subject)
			WHERE a.id = $1 AND h.released_at IS NULL)`, accountID).Scan(&held)
	return held, err
}

// PurgeDeleted elimina FÍSICAMENTE los mensajes ya soft-deleted anteriores a
// olderThan, SALVO los de sujetos bajo hold activo (que se preservan). Es la
// pieza de retención/expiración/erasure que el legal hold veta; el borrado ya
// descontó used_bytes al hacer el soft-delete, así que aquí no se re-descuenta.
// Devuelve el nº de filas purgadas y las claves de S3 que ya no referencia
// nadie —ni otra fila de la organización ni, en los blobs compartidos, ninguna
// otra organización—, para que quien llama las borre del almacén. Corre bajo el
// tenant del contexto.
//
// ponytail: primitivo con la lógica de hold; sin scheduler propio (hoy la
// papelera no se auto-purga: añadir un job cambiaría el comportamiento actual).
// Cuando aterrice una política de retención / erasure RGPD, que llame aquí y el
// hold ya queda respetado. Para purga global, iterar tenants.
func (s *Store) PurgeDeleted(ctx context.Context, olderThan time.Time) (int64, []string, error) {
	var n int64
	var huerfanas []string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Defensa en profundidad: acotar por tenant en la consulta ADEMÁS de RLS.
		// Evita que, con un rol BYPASSRLS, la purga cruce tenants (sería catastrófico).
		rows, err := tx.Query(ctx, `
			DELETE FROM messages m
			USING accounts a
			WHERE m.account_id = a.id
			  AND m.tenant = $2 AND a.tenant = $2
			  AND m.deleted_at IS NOT NULL
			  AND m.deleted_at < $1
			  AND NOT EXISTS (
			      SELECT 1 FROM legal_holds h
			      WHERE h.tenant = a.tenant AND h.released_at IS NULL
			        AND (h.subject IS NULL OR h.subject = a.subject))
			RETURNING m.blob_key`, olderThan, tenantOf(ctx))
		if err != nil {
			return fmt.Errorf("store: purge deleted: %w", err)
		}
		var claves []string
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				rows.Close()
				return fmt.Errorf("store: purge deleted: %w", err)
			}
			claves = append(claves, k)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("store: purge deleted: %w", err)
		}
		n = int64(len(claves))
		huerfanas, err = soltarBlobs(ctx, tx, claves)
		return err
	})
	return n, huerfanas, err
}

// ---------------------------------------------------------------------------
// eDiscovery: búsqueda sobre el correo de un usuario objetivo INCLUYENDO el
// preservado por hold (a diferencia de SearchMessages, que filtra deleted_at).
// ---------------------------------------------------------------------------

// EDiscoveryFilter son los criterios de una búsqueda de eDiscovery sobre la
// cuenta objetivo. Campos vacíos no filtran. From/To acotan received_at.
type EDiscoveryFilter struct {
	From     time.Time // received_at >= From (inclusive); cero = sin límite inferior
	To       time.Time // received_at <  To;            cero = sin límite superior
	Query    string    // FTS sobre asunto+remitente+snippet
	FromAddr string    // subcadena del remitente (plegar)
	Subject  string    // subcadena del asunto (plegar)
	Limit    int
	Offset   int
}

// ediscoveryCols espeja selectMessageCols + deleted_at (para marcar Preserved).
const ediscoveryCols = `
	id, tenant, account_id, mailbox_id, thread_id, uid, rfc822_msg_id, in_reply_to,
	subject, from_addr, from_name, to_addrs, cc_addrs, bcc_addrs, reply_to,
	snippet, blob_key, size_bytes, flags, has_attach,
	spf_result, dkim_result, dmarc_result, received_at, sent_at, deleted_at`

// EDiscoverySearch busca el correo de accountID (el usuario objetivo) según el
// filtro, INCLUYENDO el preservado (soft-deleted). Owner-scoped por account_id y
// bajo RLS del tenant del admin: un accountID de otra org no casa (aislamiento).
// Devuelve los sobres, más recientes primero, con la marca Preserved.
func (s *Store) EDiscoverySearch(ctx context.Context, accountID uuid.UUID, f EDiscoveryFilter) ([]domain.EDiscoveryHit, error) {
	limit := f.Limit
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	// Defensa en profundidad: filtrar por tenant en la consulta ADEMÁS de RLS
	// (como AccountBySubject). Garantiza el aislamiento por org aunque RLS no
	// aplicara (rol BYPASSRLS): un admin JAMÁS ve correo de otra org por eDiscovery.
	args := []any{accountID, tenantOf(ctx)}
	preds := []string{"account_id = $1"} // la organización ($2) va escrita en la consulta de abajo
	add := func(clause string, val any) {
		args = append(args, val)
		preds = append(preds, fmt.Sprintf(clause, len(args)))
	}
	if !f.From.IsZero() {
		add("received_at >= $%d", f.From)
	}
	if !f.To.IsZero() {
		add("received_at < $%d", f.To)
	}
	if t := strings.TrimSpace(f.Query); t != "" {
		add(ftsMensaje+" @@ plainto_tsquery('simple', plegar($%d))", t)
	}
	if f.FromAddr != "" {
		add("plegar(from_addr) LIKE '%%' || plegar($%d) || '%%'", f.FromAddr)
	}
	if f.Subject != "" {
		add("plegar(subject) LIKE '%%' || plegar($%d) || '%%'", f.Subject)
	}
	args = append(args, limit)
	limitPos := len(args)
	args = append(args, f.Offset)
	offsetPos := len(args)

	query := "SELECT " + ediscoveryCols + " FROM messages WHERE tenant = $2 AND " + strings.Join(preds, " AND ") +
		fmt.Sprintf(" ORDER BY received_at DESC, id DESC LIMIT $%d OFFSET $%d", limitPos, offsetPos)

	var out []domain.EDiscoveryHit
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("store: ediscovery search: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			h, err := scanHit(rows)
			if err != nil {
				return err
			}
			out = append(out, h)
		}
		return rows.Err()
	})
	return out, err
}

// ---------------------------------------------------------------------------
// Auditoría
// ---------------------------------------------------------------------------

// WriteAudit registra una acción de acceso privilegiado (búsqueda/export de
// eDiscovery). La creación/liberación de hold se auditan dentro de su propia
// transacción (writeAuditTx). Corre bajo el tenant del contexto.
func (s *Store) WriteAudit(ctx context.Context, actor, action, target string, detail any) error {
	tenant := tenantOf(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return writeAuditTx(ctx, tx, tenant, actor, action, target, detail)
	})
}

// ListAudit devuelve las últimas entradas de auditoría del tenant (para el test
// y la futura Console de compliance).
func (s *Store) ListAudit(ctx context.Context, limit int) ([]domain.AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []domain.AuditEntry
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, occurred_at, tenant, actor, action, target, detail
			FROM audit_log WHERE tenant = $1
			ORDER BY occurred_at DESC, id DESC LIMIT $2`, tenantOf(ctx), limit)
		if err != nil {
			return fmt.Errorf("store: list audit: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var e domain.AuditEntry
			var detail []byte
			if err := rows.Scan(&e.ID, &e.OccurredAt, &e.Tenant, &e.Actor, &e.Action, &e.Target, &detail); err != nil {
				return err
			}
			e.Detail = detail
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}

func writeAuditTx(ctx context.Context, tx pgx.Tx, tenant, actor, action, target string, detail any) error {
	b, err := json.Marshal(detail)
	if err != nil || b == nil {
		b = []byte("{}")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (tenant, actor, action, target, detail)
		VALUES ($1, $2, $3, $4, $5)`, tenant, actor, action, target, b); err != nil {
		return fmt.Errorf("store: write audit: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Escaneo
// ---------------------------------------------------------------------------

func scanHold(s scanner) (domain.LegalHold, error) {
	var h domain.LegalHold
	var subject *string
	err := s.Scan(&h.ID, &h.Tenant, &subject, &h.Reason, &h.Case, &h.CreatedBy, &h.CreatedAt, &h.ReleasedAt, &h.ReleasedBy)
	if subject != nil {
		h.Subject = *subject
	}
	return h, err
}

func scanHit(rows pgx.Rows) (domain.EDiscoveryHit, error) {
	var m domain.Message
	var uid int64
	var spf, dkim, dmarc string
	var sentAt, deletedAt *time.Time
	err := rows.Scan(&m.ID, &m.Tenant, &m.AccountID, &m.MailboxID, &m.ThreadID, &uid,
		&m.MessageID, &m.InReplyTo, &m.Subject, &m.FromAddr, &m.FromName,
		&m.To, &m.Cc, &m.Bcc, &m.ReplyTo, &m.Snippet, &m.BlobKey, &m.SizeBytes,
		&m.Flags, &m.HasAttach, &spf, &dkim, &dmarc, &m.ReceivedAt, &sentAt, &deletedAt)
	if err != nil {
		return domain.EDiscoveryHit{}, err
	}
	m.UID = u32(uid)
	if sentAt != nil {
		m.SentAt = *sentAt
	}
	m.SPFResult = domain.AuthResult(spf)
	m.DKIMResult = domain.AuthResult(dkim)
	m.DMARCResult = domain.AuthResult(dmarc)
	return domain.EDiscoveryHit{Message: m, Preserved: deletedAt != nil}, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
