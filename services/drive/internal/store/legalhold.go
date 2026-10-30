// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Legal hold + eDiscovery de ficheros (compliance NO destructivo). Ver mig 0012.
//
// INVARIANTE: mientras un sujeto tiene un hold ACTIVO, sus ficheros y TODAS sus
// versiones NUNCA se eliminan físicamente. El borrado del usuario (soft-delete,
// files.deleted_at) los sigue enviando a la papelera, pero:
//   - purgeFileTx / purgeFolderTx (núcleos de la purga MANUAL de papelera —Mi
//     unidad y unidades de equipo— y de la cascada de carpeta) se NIEGAN
//     (ErrLegalHold) si el fichero / algún fichero del subárbol está bajo hold.
//   - ClaimAndPurgeExpiredTrash (auto-purga cross-tenant) SALTA en su claim los
//     ficheros bajo hold y las carpetas cuyo subárbol guarda alguno (gc.go), así
//     que nunca los reclama ni los purga.
//   - Las versiones (file_versions) solo mueren por cascada FK al borrar su fila
//     de files; preservar el fichero preserva sus versiones (no hay borrado de
//     versión suelto).
//
// Preservación DINÁMICA: sin sello por-fichero; cada punto de purga evalúa
// «¿existe un hold activo que cubra a este owner_sub?» (NOT EXISTS). Liberar el
// hold restaura el ciclo normal (purgable).
//
// Aislamiento por org: RLS por app.tenant + predicado `tenant = $n` EXPLÍCITO en
// cada consulta (defensa en profundidad: pgtest corre como rol que BYPASSA RLS, así
// que el predicado es lo que impide que un admin de A vea ficheros/holds/traza de B).
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

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// ---------------------------------------------------------------------------
// Legal holds
// ---------------------------------------------------------------------------

// CreateHold crea una orden de preservación y AUDITA la creación en la MISMA
// transacción (atómico: no hay hold sin traza). Subject vacío => hold de toda la
// org. Corre bajo el tenant del contexto.
func (s *Store) CreateHold(ctx context.Context, h domain.LegalHold) (domain.LegalHold, error) {
	tenant := tenancy.MustFrom(ctx).Slug
	var out domain.LegalHold
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO legal_holds (tenant, subject, reason, case_ref, created_by)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, tenant, subject, reason, case_ref, created_by, created_at, released_at, released_by`,
			tenant, nilIfEmpty(h.Subject), h.Reason, h.Case, h.CreatedBy)
		var e error
		out, e = scanHold(row)
		if e != nil {
			return fmt.Errorf("store: crear hold: %w", e)
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
// liberado. Tras liberar, los ficheros del sujeto vuelven al ciclo normal.
func (s *Store) ReleaseHold(ctx context.Context, id uuid.UUID, actor string) error {
	tenant := tenancy.MustFrom(ctx).Slug
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
			return fmt.Errorf("store: liberar hold: %w", err)
		}
		target := ""
		if subject != nil {
			target = *subject
		}
		return writeAuditTx(ctx, tx, tenant, actor, "legalhold.release", target, map[string]any{"hold_id": id})
	})
}

// ListHolds lista los holds del tenant (más recientes primero). includeReleased=false
// devuelve solo los activos (vista por defecto).
func (s *Store) ListHolds(ctx context.Context, includeReleased bool) ([]domain.LegalHold, error) {
	tenant := tenancy.MustFrom(ctx).Slug
	q := `
		SELECT id, tenant, subject, reason, case_ref, created_by, created_at, released_at, released_by
		FROM legal_holds WHERE tenant = $1`
	if !includeReleased {
		q += ` AND released_at IS NULL`
	}
	q += ` ORDER BY created_at DESC`
	var out []domain.LegalHold
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, q, tenant)
		if err != nil {
			return fmt.Errorf("store: listar holds: %w", err)
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

// ---------------------------------------------------------------------------
// Guardas de hold que consumen los núcleos de purga (store.go / gc.go)
// ---------------------------------------------------------------------------

// SujetoRetenido indica si hay un hold ACTIVO sobre `sub` (o sobre toda la
// organización). Lo pregunta Sytue Docs antes de borrar para siempre un documento
// de esa persona, tenga o no fichero en Drive (service.DocsAccion).
func (s *Store) SujetoRetenido(ctx context.Context, sub string) (bool, error) {
	t := tenancy.MustFrom(ctx)
	var held bool
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		held, err = subjectUnderHoldTx(ctx, tx, t.Slug, sub)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("store: comprobar hold de persona: %w", err)
	}
	return held, nil
}

func subjectUnderHoldTx(ctx context.Context, tx pgx.Tx, tenant, subject string) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM legal_holds WHERE tenant=$1 AND released_at IS NULL AND (subject IS NULL OR subject=$2))`, tenant, subject).Scan(&held)
	return held, err
}

// fileUnderHoldTx indica si el fichero (tenant, id) está cubierto por algún hold
// ACTIVO (por su owner_sub o de toda la org). Lo consulta purgeFileTx antes de un
// borrado físico. El JOIN por h.tenant = f.tenant mantiene el aislamiento: un hold
// de otra org (mismo owner_sub) NUNCA preserva un fichero de esta.
func fileUnderHoldTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM files f
			JOIN legal_holds h ON h.tenant = f.tenant
			 AND (h.subject IS NULL OR h.subject = f.owner_sub)
			WHERE f.tenant = $1 AND f.id = $2 AND h.released_at IS NULL)`, tenant, id).Scan(&held)
	if err != nil {
		return false, fmt.Errorf("store: comprobar hold de fichero: %w", err)
	}
	return held, nil
}

// subtreeUnderHoldTx indica si ALGÚN fichero del subárbol (raíz = folder id) está
// bajo un hold ACTIVO. Lo consulta purgeFolderTx antes de una cascada de borrado:
// si hay retención, se niega la purga de toda la carpeta (preserva también los
// hermanos no retenidos —no destructivo—).
func subtreeUnderHoldTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx, `
		WITH RECURSIVE sub AS (
			SELECT id FROM folders WHERE tenant = $1 AND id = $2
			UNION ALL
			SELECT c.id FROM folders c JOIN sub ON c.parent_id = sub.id WHERE c.tenant = $1
		)
		SELECT EXISTS (
			SELECT 1 FROM files f
			JOIN legal_holds h ON h.tenant = f.tenant
			 AND (h.subject IS NULL OR h.subject = f.owner_sub)
			WHERE f.tenant = $1 AND f.folder_id IN (SELECT id FROM sub) AND h.released_at IS NULL)`,
		tenant, id).Scan(&held)
	if err != nil {
		return false, fmt.Errorf("store: comprobar hold del subárbol: %w", err)
	}
	return held, nil
}

// ---------------------------------------------------------------------------
// eDiscovery: búsqueda sobre los ficheros de un sujeto INCLUYENDO los preservados
// (soft-deleted), a diferencia de las rutas normales que filtran deleted_at.
// ---------------------------------------------------------------------------

// EDiscoveryFilter son los criterios de una búsqueda de eDiscovery sobre los
// ficheros del sujeto. Campos vacíos no filtran. From/To acotan created_at.
type EDiscoveryFilter struct {
	From        time.Time // created_at >= From (inclusive); cero = sin límite inferior
	To          time.Time // created_at <  To;             cero = sin límite superior
	Name        string    // subcadena del nombre (plegar)
	ContentType string    // content_type exacto (p. ej. "application/pdf")
	// LabelID acota a los ficheros con ESA etiqueta de sensibilidad (el uuid de la
	// etiqueta en Sytue ID). Es el filtro que pide una investigación real: «enséñame
	// todo lo que estaba clasificado como Confidencial». Casa contra el SNAPSHOT del
	// fichero, así que sigue funcionando aunque la etiqueta ya no exista en la
	// taxonomía. Vacío = no filtra.
	LabelID string
	Limit   int
	Offset  int
}

// EDiscoverySearch busca los ficheros de owner_sub=subject según el filtro,
// INCLUYENDO los preservados (soft-deleted). Corre bajo InMaintenanceTx para
// abarcar TAMBIÉN los ficheros del sujeto en unidades de equipo (la RLS normal
// oculta el contenido de una unidad salvo que app.team_drive apunte a ella); el
// predicado `tenant = $1` EXPLÍCITO mantiene el aislamiento por org (un admin de A
// jamás ve ficheros de B, aun bajo la política de mantenimiento o BYPASSRLS de
// pgtest). Devuelve los metadatos, más recientes primero, con la marca Preserved.
func (s *Store) EDiscoverySearch(ctx context.Context, subject string, f EDiscoveryFilter) ([]domain.EDiscoveryHit, error) {
	tenant := tenancy.MustFrom(ctx).Slug
	limit := f.Limit
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	args := []any{tenant, subject}
	preds := []string{"owner_sub = $2"} // y `tenant = $1`, escrito en la consulta
	add := func(clause string, val any) {
		args = append(args, val)
		preds = append(preds, fmt.Sprintf(clause, len(args)))
	}
	if !f.From.IsZero() {
		add("created_at >= $%d", f.From)
	}
	if !f.To.IsZero() {
		add("created_at < $%d", f.To)
	}
	if f.Name != "" {
		add("plegar(name) LIKE '%%' || plegar($%d) || '%%'", f.Name)
	}
	if f.ContentType != "" {
		add("content_type = $%d", f.ContentType)
	}
	if f.LabelID != "" {
		add("label_id = $%d", f.LabelID)
	}
	args = append(args, limit)
	limitPos := len(args)
	args = append(args, f.Offset)
	offsetPos := len(args)

	// fileCols trae deleted_at: es lo que marca Preserved en cada hit.
	query := "SELECT " + fileCols + " FROM files WHERE tenant = $1 AND " + strings.Join(preds, " AND ") +
		fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", limitPos, offsetPos)

	var out []domain.EDiscoveryHit
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("store: ediscovery search: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			h, err := scanEDiscoveryHit(rows, tenant)
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
	tenant := tenancy.MustFrom(ctx).Slug
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return writeAuditTx(ctx, tx, tenant, actor, action, target, detail)
	})
}

// ListAudit devuelve las últimas entradas de auditoría del tenant (para el test y
// la futura Console de compliance).
func (s *Store) ListAudit(ctx context.Context, limit int) ([]domain.AuditEntry, error) {
	tenant := tenancy.MustFrom(ctx).Slug
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []domain.AuditEntry
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, occurred_at, tenant, actor, action, target, detail
			FROM audit_log WHERE tenant = $1
			ORDER BY occurred_at DESC, id DESC LIMIT $2`, tenant, limit)
		if err != nil {
			return fmt.Errorf("store: listar audit: %w", err)
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
		return fmt.Errorf("store: escribir audit: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Escaneo
// ---------------------------------------------------------------------------

func scanHold(row pgx.Row) (domain.LegalHold, error) {
	var h domain.LegalHold
	var subject *string
	err := row.Scan(&h.ID, &h.Tenant, &subject, &h.Reason, &h.Case, &h.CreatedBy, &h.CreatedAt, &h.ReleasedAt, &h.ReleasedBy)
	if subject != nil {
		h.Subject = *subject
	}
	return h, err
}

func scanEDiscoveryHit(row pgx.Row, tenant string) (domain.EDiscoveryHit, error) {
	f, err := scanFile(row, tenant)
	if err != nil {
		return domain.EDiscoveryHit{}, err
	}
	return domain.EDiscoveryHit{File: f, Preserved: f.DeletedAt != nil}, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
