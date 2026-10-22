// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/mime"
)

// Enqueue inserta una unidad de salida en la cola (sobre PostgreSQL; sin
// Redis/Valkey). Emite NOTIFY para despertar a los workers de inmediato.
func (s *Store) Enqueue(ctx context.Context, o *domain.OutboundMessage) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := enqueueTx(ctx, tx, tenantOf(ctx), o); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_notify('sytue_mail_outbound', $1)`, o.ID.String()); err != nil {
			return fmt.Errorf("store: notify outbound: %w", err)
		}
		return nil
	})
}

// enqueueTx inserta una fila de ENTREGA (hold=”, liberable ya: next_retry_at
// por defecto now()) sobre una transacción existente. Lo reutilizan Enqueue (con
// NOTIFY) y la liberación de diferidos (PromoteDueHeld, dentro de su tx de
// mantenimiento). `tenant` debe estar fijado en la sesión (app.tenant) para RLS.
func enqueueTx(ctx context.Context, tx pgx.Tx, tenant string, o *domain.OutboundMessage) error {
	row := tx.QueryRow(ctx, `
		INSERT INTO outbound (tenant, account_id, message_id, mail_from, recipients, blob_key, state)
		VALUES ($1, $2, $3, $4, $5, $6, 'queued')
		RETURNING id, created_at, updated_at`,
		tenant, o.AccountID, nilUUID(o.MessageID), o.From, o.Recipients, o.BlobKey)
	if err := row.Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return fmt.Errorf("store: enqueue: %w", err)
	}
	o.State = domain.OutQueued
	return nil
}

// EnqueueHeld encola una submission DIFERIDA (programado/deshacer): una fila
// 'queued' RETENIDA (hold != ”) cuya next_retry_at es el instante de liberación.
// NO se entrega directamente: el worker (PromoteDueHeld) la libera al vencer,
// creando la copia en Sent y las filas de entrega por dominio. message_id es NULL
// (aún no hay copia en Sent). Devuelve el id (para cancelar/editar/deshacer).
func (s *Store) EnqueueHeld(ctx context.Context, o *domain.OutboundMessage, releaseAt time.Time, hold string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO outbound (tenant, account_id, mail_from, recipients, blob_key, subject, state, hold, next_retry_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'queued', $7, $8)
			RETURNING id`,
			tenantOf(ctx), o.AccountID, o.From, o.Recipients, o.BlobKey, o.Subject, hold, releaseAt.UTC()).Scan(&id)
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("store: enqueue held: %w", err)
	}
	return id, nil
}

// ClaimDue reclama hasta `limit` mensajes drenables (queued/deferred con
// next_retry_at <= now) marcándolos 'sending' atómicamente. Usa FOR UPDATE SKIP
// LOCKED para reparto seguro entre workers concurrentes. Drena la cola de TODOS
// los tenants: corre en transacción de mantenimiento (app.maintenance='on') para
// habilitar el acceso cross-tenant vía la política PERMISSIVE (sin BYPASSRLS).
func (s *Store) ClaimDue(ctx context.Context, limit int) ([]domain.OutboundMessage, error) {
	if limit <= 0 {
		limit = 10
	}
	var out []domain.OutboundMessage
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			WITH claimed AS (
				SELECT id FROM outbound
				WHERE state IN ('queued','deferred') AND hold = '' AND next_retry_at <= now()
				ORDER BY next_retry_at
				FOR UPDATE SKIP LOCKED
				LIMIT $1
			)
			UPDATE outbound o SET state = 'sending', updated_at = now()
			FROM claimed c WHERE o.id = c.id
			RETURNING o.id, o.tenant, o.account_id, o.message_id, o.mail_from,
			          o.recipients, o.blob_key, o.state, o.attempts, o.last_error,
			          o.next_retry_at, o.created_at, o.updated_at, o.subject`, limit)
		if err != nil {
			return fmt.Errorf("store: claim due: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			o, err := scanOutbound(rows)
			if err != nil {
				return err
			}
			out = append(out, o)
		}
		return rows.Err()
	})
	return out, err
}

// MarkSent marca una unidad de salida como entregada con éxito. Corre en
// transacción de mantenimiento (el worker es cross-tenant, sin BYPASSRLS).
func (s *Store) MarkSent(ctx context.Context, id uuid.UUID) error {
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE outbound SET state = 'sent', attempts = attempts + 1, last_error = '', updated_at = now()
			WHERE id = $1`, id)
		return err
	})
}

// MarkDeferred reprograma una unidad para reintento con backoff, o la marca como
// 'bounced' si supera maxAttempts. lastErr es el motivo (4xx SMTP transitorio).
func (s *Store) MarkDeferred(ctx context.Context, id uuid.UUID, lastErr string, backoff time.Duration, maxAttempts int) (bounced bool, err error) {
	err = s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		var attempts int
		if err := tx.QueryRow(ctx, `SELECT attempts FROM outbound WHERE id = $1`, id).Scan(&attempts); err != nil {
			return fmt.Errorf("store: leer intentos: %w", err)
		}
		attempts++
		if attempts >= maxAttempts {
			bounced = true
			_, e := tx.Exec(ctx, `
				UPDATE outbound SET state = 'bounced', attempts = $2, last_error = $3, updated_at = now()
				WHERE id = $1`, id, attempts, truncErr(lastErr))
			return e
		}
		_, e := tx.Exec(ctx, `
			UPDATE outbound SET state = 'deferred', attempts = $2, last_error = $3,
			       next_retry_at = now() + $4::interval, updated_at = now()
			WHERE id = $1`, id, attempts, truncErr(lastErr), backoff.String())
		return e
	})
	return bounced, err
}

// MarkBounced marca una unidad como rebotada definitivamente (error 5xx SMTP).
// Corre en transacción de mantenimiento (worker cross-tenant, sin BYPASSRLS).
func (s *Store) MarkBounced(ctx context.Context, id uuid.UUID, lastErr string) error {
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE outbound SET state = 'bounced', attempts = attempts + 1, last_error = $2, updated_at = now()
			WHERE id = $1`, id, truncErr(lastErr))
		return err
	})
}

// ---------------------------------------------------------------------------
// Envío diferido: programados (vista/gestión) + liberación por el worker.
// Aislamiento por DUEÑO: RLS por tenant + predicado explícito account_id en todo
// listado/edición/cancelación (un usuario nunca toca los diferidos de otro).
// ---------------------------------------------------------------------------

// ListScheduled pagina por KEYSET (next_retry_at, id) los mensajes PROGRAMADOS
// pendientes de una cuenta (state='queued', hold='scheduled'), los más próximos
// primero. `before`/`beforeID` son el (next_retry_at, id) del último de la página
// previa (cero = primera página). Coste por página constante (outbound_scheduled_idx).
func (s *Store) ListScheduled(ctx context.Context, accountID uuid.UUID, limit int, before time.Time, beforeID uuid.UUID) ([]domain.OutboundMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, mail_from, recipients, subject, next_retry_at, created_at
		FROM outbound
		WHERE tenant = $1 AND account_id = $2 AND state = 'queued' AND hold = 'scheduled'`
	args := []any{tenantOf(ctx), accountID}
	if !before.IsZero() {
		// Keyset ASC: (release_at, id) estrictamente MAYORES que el último visto.
		q += fmt.Sprintf(" AND (next_retry_at, id) > ($%d, $%d)", len(args)+1, len(args)+2)
		args = append(args, before, beforeID)
	}
	q += fmt.Sprintf(" ORDER BY next_retry_at ASC, id ASC LIMIT $%d", len(args)+1)
	args = append(args, limit)
	var out []domain.OutboundMessage
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return fmt.Errorf("store: list scheduled: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var o domain.OutboundMessage
			if err := rows.Scan(&o.ID, &o.From, &o.Recipients, &o.Subject, &o.NextRetryAt, &o.CreatedAt); err != nil {
				return err
			}
			o.State, o.Hold = domain.OutQueued, domain.HoldScheduled
			out = append(out, o)
		}
		return rows.Err()
	})
	return out, err
}

// RescheduleHeld cambia el instante de liberación de un PROGRAMADO del usuario.
// El guard state='queued' AND hold='scheduled' hace la operación atómica frente
// al worker: si ya se liberó (state<>'queued') no casa ninguna fila. account_id
// asegura que solo el dueño edita. ErrScheduleGone si ya no es editable.
func (s *Store) RescheduleHeld(ctx context.Context, id, accountID uuid.UUID, releaseAt time.Time) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE outbound SET next_retry_at = $4, updated_at = now()
			WHERE tenant = $1 AND id = $2 AND account_id = $3
			  AND state = 'queued' AND hold = 'scheduled'`,
			tenantOf(ctx), id, accountID, releaseAt.UTC())
		if err != nil {
			return fmt.Errorf("store: reschedule: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrScheduleGone
		}
		return nil
	})
}

// CancelHeld cancela un diferido (programado o deshacer) del usuario y devuelve
// su blob_key + hold para que la capa superior lo reabra como borrador. Es la
// carrera clave con el worker: el UPDATE ... WHERE state='queued' toma el mismo
// bloqueo de fila que el claim de liberación (FOR UPDATE); gana quien commitea
// primero. Si el worker ya lo liberó (state<>'queued'), no casa nada y devuelve
// ErrScheduleGone (demasiado tarde) — nunca un doble efecto. account_id aísla por
// dueño. hold IN (scheduled,undo) evita cancelar una fila de entrega normal.
func (s *Store) CancelHeld(ctx context.Context, id, accountID uuid.UUID) (blobKey, hold string, err error) {
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `
			UPDATE outbound SET state = 'cancelled', updated_at = now()
			WHERE tenant = $1 AND id = $2 AND account_id = $3
			  AND state = 'queued' AND hold IN ('scheduled','undo')
			RETURNING blob_key, hold`,
			tenantOf(ctx), id, accountID).Scan(&blobKey, &hold)
		if errors.Is(e, pgx.ErrNoRows) {
			return domain.ErrScheduleGone
		}
		return e
	})
	return blobKey, hold, err
}

// PromoteDueHeld LIBERA los diferidos vencidos (hold != ”, next_retry_at<=now):
// por cada uno crea la copia en Sent y encola su entrega por dominio, y borra la
// fila retenida — TODO en UNA transacción de mantenimiento (exactly-once, como el
// ClaimDueScheduled de Chat). FOR UPDATE SKIP LOCKED reparte entre réplicas sin
// doble liberación; si el commit falla, ni se envía ni se borra (reintento en el
// próximo tick, sin pérdida). Cross-tenant vía app.maintenance; app.tenant se fija
// por fila para que los INSERT en messages/outbound pasen su RLS. Devuelve cuántas
// filas liberó. getBlob baja el .eml firmado (S3); ver la nota ponytail.
//
// ponytail: se baja el blob de S3 DENTRO de la tx que sostiene el lock de la fila
// retenida. La liberación es de baja frecuencia (programados/deshacer, no la ruta
// caliente) y el lock es exclusivo de esa fila (SKIP LOCKED), así que la I/O en tx
// es aceptable. Si la tasa de liberación creciera, desnormalizar el sobre en la
// fila (to/cc/message_id/size) y crear la copia en Sent sin tocar S3.
//
//nolint:gocognit // barrido de la cola en UNA transacción: promover, reintentar y descartar comparten cursor y contadores.
func (s *Store) PromoteDueHeld(ctx context.Context, now time.Time, limit int, getBlob func(ctx context.Context, tenant, key string) ([]byte, error)) (int, error) {
	if limit <= 0 {
		limit = 10
	}
	type heldRow struct {
		id, accountID uuid.UUID
		tenant, from  string
		blobKey       string
		recipients    []string
	}
	var released int
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, account_id, mail_from, recipients, blob_key
			FROM outbound
			WHERE state = 'queued' AND hold <> '' AND next_retry_at <= $1
			ORDER BY next_retry_at
			FOR UPDATE SKIP LOCKED
			LIMIT $2`, now.UTC(), limit)
		if err != nil {
			return fmt.Errorf("store: claim held: %w", err)
		}
		var due []heldRow
		for rows.Next() {
			var h heldRow
			if err := rows.Scan(&h.id, &h.tenant, &h.accountID, &h.from, &h.recipients, &h.blobKey); err != nil {
				rows.Close()
				return err
			}
			due = append(due, h)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		var doneIDs []uuid.UUID
		for _, h := range due {
			// app.tenant por fila: los INSERT en messages/outbound pasan su RLS por
			// tenant (esas tablas no tienen política de mantenimiento).
			if err := pg.SetTenant(ctx, tx, h.tenant); err != nil {
				return err
			}
			raw, err := getBlob(ctx, h.tenant, h.blobKey)
			if err != nil {
				// Transitorio (S3): abortar el lote; nada se commitea, reintento próximo tick.
				return fmt.Errorf("store: leer blob diferido %s: %w", h.blobKey, err)
			}
			parsed, perr := mime.Parse(bytes.NewReader(raw))
			if perr != nil {
				log.From(ctx).Warn("diferido descartado: .eml ilegible", "id", h.id, "error", perr)
				doneIDs = append(doneIDs, h.id)
				continue
			}
			var sentID uuid.UUID
			if e := tx.QueryRow(ctx, `SELECT id FROM mailboxes WHERE tenant = current_setting('app.tenant', true) AND account_id = $1 AND role = 'sent'`, h.accountID).Scan(&sentID); e != nil {
				if errors.Is(e, pgx.ErrNoRows) {
					log.From(ctx).Warn("diferido descartado: sin buzón Enviados", "id", h.id)
					doneIDs = append(doneIDs, h.id)
					continue
				}
				return e
			}
			env := parsed.Envelope
			env.AccountID = h.accountID
			env.MailboxID = sentID
			env.BlobKey = h.blobKey
			env.SizeBytes = int64(len(raw))
			env.Flags = []string{domain.FlagSeen}
			if e := insertMessageTx(ctx, tx, h.tenant, &env); e != nil {
				return e
			}
			for _, grp := range groupRecipientsByDomain(h.recipients) {
				o := &domain.OutboundMessage{AccountID: h.accountID, MessageID: env.ID, From: h.from, Recipients: grp, BlobKey: h.blobKey}
				if e := enqueueTx(ctx, tx, h.tenant, o); e != nil {
					return e
				}
			}
			doneIDs = append(doneIDs, h.id)
			released++
		}
		if len(doneIDs) > 0 {
			if _, err := tx.Exec(ctx, `DELETE FROM outbound WHERE id = ANY($1)`, doneIDs); err != nil {
				return err
			}
		}
		return nil
	})
	return released, err
}

// groupRecipientsByDomain agrupa destinatarios por dominio (un lote de entrega por
// dominio, como el groupByDomain de delivery: el sender resuelve el MX de uno).
func groupRecipientsByDomain(rcpts []string) map[string][]string {
	out := make(map[string][]string)
	for _, r := range rcpts {
		d := ""
		if at := strings.LastIndexByte(r, '@'); at >= 0 {
			d = strings.ToLower(r[at+1:])
		}
		out[d] = append(out[d], r)
	}
	return out
}

// ---------------------------------------------------------------------------
// Rate-limit de recepción por IP (ventana minuto a minuto sobre Postgres).
// ---------------------------------------------------------------------------

// HitInboundRate incrementa el contador de la IP en la ventana del minuto actual
// y devuelve el recuento resultante. El llamador compara contra el límite.
func (s *Store) HitInboundRate(ctx context.Context, ip string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		INSERT INTO inbound_rate (ip, window_min, count)
		VALUES ($1, date_trunc('minute', now()), 1)
		ON CONFLICT (ip, window_min) DO UPDATE SET count = inbound_rate.count + 1
		RETURNING count`, ip).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: hit inbound rate: %w", err)
	}
	return count, nil
}

// HitAuthFailRate incrementa el contador de FALLOS DE AUTENTICACIÓN de la IP en
// la ventana del minuto actual y devuelve el recuento resultante. El llamador
// compara contra el límite.
//
// Aparte de HitInboundRate a propósito: aquello cuenta mensajes de otros MTA
// (donde 60/min es tráfico normal), esto cuenta contraseñas falladas por un
// cliente (donde 10/min ya es un ataque). Compartir contador haría que un pico
// de correo entrante cerrara el acceso a los buzones desde esa IP.
func (s *Store) HitAuthFailRate(ctx context.Context, ip string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		INSERT INTO auth_fail_rate (ip, window_min, count)
		VALUES ($1, date_trunc('minute', now()), 1)
		ON CONFLICT (ip, window_min) DO UPDATE SET count = auth_fail_rate.count + 1
		RETURNING count`, ip).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: hit auth fail rate: %w", err)
	}
	return count, nil
}

// AuthFailRate devuelve el recuento de fallos de la IP en el minuto actual SIN
// incrementarlo. Se consulta antes de validar la contraseña: si ya se pasó del
// límite, ni se molesta al IdP.
func (s *Store) AuthFailRate(ctx context.Context, ip string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT coalesce(max(count), 0) FROM auth_fail_rate
		WHERE ip = $1 AND window_min = date_trunc('minute', now())`, ip).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: auth fail rate: %w", err)
	}
	return count, nil
}

// GCAuthFailRate purga las ventanas de fallos de autenticación anteriores a olderThan.
func (s *Store) GCAuthFailRate(ctx context.Context, olderThan time.Duration) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM auth_fail_rate WHERE window_min < now() - $1::interval`, olderThan.String())
	return err
}

// GCInboundRate purga las ventanas de rate-limit anteriores a olderThan.
func (s *Store) GCInboundRate(ctx context.Context, olderThan time.Duration) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM inbound_rate WHERE window_min < now() - $1::interval`, olderThan.String())
	return err
}

func scanOutbound(s scanner) (domain.OutboundMessage, error) {
	var o domain.OutboundMessage
	var msgID *uuid.UUID
	var state string
	// `subject` viaja desnormalizado en la tabla (lo puso el panel de programados) y
	// aquí lo necesita el aviso de NO ENTREGA: decir «no salió tu mensaje» sin decir
	// cuál obliga a adivinar.
	err := s.Scan(&o.ID, &o.Tenant, &o.AccountID, &msgID, &o.From, &o.Recipients,
		&o.BlobKey, &state, &o.Attempts, &o.LastError, &o.NextRetryAt, &o.CreatedAt,
		&o.UpdatedAt, &o.Subject)
	if err != nil {
		return o, err
	}
	if msgID != nil {
		o.MessageID = *msgID
	}
	o.State = domain.OutboundState(state)
	return o, nil
}

func nilUUID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}

func truncErr(s string) string {
	const maxLen = 1000
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
