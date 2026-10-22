// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Estado de las FUSIONES de organizaciones (mig 0031). La fusión es un proceso
// largo que toca diez bases, así que su progreso se persiste por servicio: un
// fallo en el séptimo no obliga a repetir los seis primeros y, sobre todo, deja
// escrito dónde se quedó.

// OrgMerge es una fusión de organizaciones y su progreso.
type OrgMerge struct {
	ID          string            `json:"id"`
	SourceOrgID string            `json:"source_org_id"`
	TargetOrgID string            `json:"target_org_id"`
	Status      string            `json:"status"`
	RequestedBy string            `json:"requested_by"`
	Services    map[string]string `json:"services"`
	Error       string            `json:"error"`
	StartedAt   *time.Time        `json:"started_at"`
	FinishedAt  *time.Time        `json:"finished_at"`
	CreatedAt   time.Time         `json:"created_at"`
}

const mergeCols = `id, source_org_id, target_org_id, status, requested_by,
	services, error, started_at, finished_at, created_at`

func scanMerge(row pgx.Row) (OrgMerge, error) {
	var m OrgMerge
	var services []byte
	err := row.Scan(&m.ID, &m.SourceOrgID, &m.TargetOrgID, &m.Status, &m.RequestedBy,
		&services, &m.Error, &m.StartedAt, &m.FinishedAt, &m.CreatedAt)
	if err != nil {
		return OrgMerge{}, err
	}
	if len(services) > 0 {
		if err := json.Unmarshal(services, &m.Services); err != nil {
			return OrgMerge{}, err
		}
	}
	if m.Services == nil {
		m.Services = map[string]string{}
	}
	return m, nil
}

// CreateOrgMerge abre una fusión. El índice parcial de la migración impide que
// una organización esté en dos fusiones abiertas: el conflicto se traduce a
// ErrConflict y la API responde 409 en vez de arrancar dos procesos que se
// pisarían las filas.
func (s *Store) CreateOrgMerge(ctx context.Context, q pg.Querier, sourceID, targetID, requestedBy string) (OrgMerge, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO org_merges (source_org_id, target_org_id, requested_by)
	             VALUES ($1, $2, $3) RETURNING ` + mergeCols
	m, err := scanMerge(q.QueryRow(ctx, sql, sourceID, targetID, requestedBy))
	return m, errf("CreateOrgMerge", err)
}

// GetOrgMerge devuelve una fusión por id.
func (s *Store) GetOrgMerge(ctx context.Context, id string) (OrgMerge, error) {
	const sql = `SELECT ` + mergeCols + ` FROM org_merges WHERE id = $1`
	m, err := scanMerge(s.DB().QueryRow(ctx, sql, id))
	return m, errf("GetOrgMerge", err)
}

// ListOrgMerges devuelve las fusiones más recientes primero.
func (s *Store) ListOrgMerges(ctx context.Context, limit int) ([]OrgMerge, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const sql = `SELECT ` + mergeCols + ` FROM org_merges ORDER BY created_at DESC LIMIT $1`
	rows, err := s.DB().Query(ctx, sql, limit)
	if err != nil {
		return nil, errf("ListOrgMerges", err)
	}
	defer rows.Close()
	var out []OrgMerge
	for rows.Next() {
		m, err := scanMerge(rows)
		if err != nil {
			return nil, errf("ListOrgMerges", err)
		}
		out = append(out, m)
	}
	return out, errf("ListOrgMerges", rows.Err())
}

// StartOrgMerge marca la fusión como en curso. Solo desde 'pending' o 'failed':
// así dos peticiones simultáneas de reanudación no lanzan dos procesos (la
// segunda no encuentra fila que actualizar y recibe ErrNotFound).
func (s *Store) StartOrgMerge(ctx context.Context, id string) error {
	const sql = `UPDATE org_merges
	             SET status = 'running', started_at = coalesce(started_at, now()),
	                 error = '', updated_at = now()
	             WHERE id = $1 AND status IN ('pending', 'failed')`
	tag, err := s.DB().Exec(ctx, sql, id)
	if err != nil {
		return errf("StartOrgMerge", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// MarkMergeService anota el estado de UN servicio dentro de la fusión. El JSONB
// se actualiza con jsonb_set para no perder el progreso de los demás si dos
// escrituras se solapan.
func (s *Store) MarkMergeService(ctx context.Context, id, service, status string) error {
	const sql = `UPDATE org_merges
	             SET services = jsonb_set(services, ARRAY[$2], to_jsonb($3::text), true),
	                 updated_at = now()
	             WHERE id = $1`
	_, err := s.DB().Exec(ctx, sql, id, service, status)
	return errf("MarkMergeService", err)
}

// FinishOrgMerge cierra la fusión. Un error no vacío la deja en 'failed' para
// que se pueda reanudar; sin error queda 'completed'.
func (s *Store) FinishOrgMerge(ctx context.Context, id, failure string) error {
	const sql = `UPDATE org_merges
	             SET status = CASE WHEN $2 = '' THEN 'completed' ELSE 'failed' END,
	                 error = $2,
	                 finished_at = CASE WHEN $2 = '' THEN now() ELSE finished_at END,
	                 updated_at = now()
	             WHERE id = $1`
	_, err := s.DB().Exec(ctx, sql, id, failure)
	return errf("FinishOrgMerge", err)
}

// MergeIdentity mueve la IDENTIDAD del origen al destino en una transacción:
// pertenencias y estado terminal de la organización absorbida.
//
// Un usuario que ya pertenece al destino conserva SU rol allí: la fusión no
// puede ascender a nadie por venir de una organización donde era owner, ni
// degradar a quien ya era admin del destino. El resto entra con el rol que
// tenía, salvo `owner`, que pasa a `admin`: el destino ya tiene sus propietarios
// y absorber una organización no debe multiplicarlos.
func (s *Store) MergeIdentity(ctx context.Context, sourceID, targetID string) (moved int, err error) {
	err = s.InTx(ctx, func(q pg.Querier) error {
		if err := s.suspendExternalOrg(ctx, q, sourceID, domain.StatusSuspended); err != nil {
			return err
		}
		const sqlMembers = `
			INSERT INTO memberships (org_id, user_id, role)
			SELECT $2, ms.user_id,
			       CASE WHEN ms.role = 'owner' THEN 'admin' ELSE ms.role END
			FROM memberships ms
			WHERE ms.org_id = $1
			ON CONFLICT (org_id, user_id) DO NOTHING`
		tag, err := q.Exec(ctx, sqlMembers, sourceID, targetID)
		if err != nil {
			return errf("MergeIdentity miembros", err)
		}
		moved = int(tag.RowsAffected())

		if _, err := q.Exec(ctx, `DELETE FROM memberships WHERE org_id = $1`, sourceID); err != nil {
			return errf("MergeIdentity limpiar origen", err)
		}
		const sqlOrg = `
			UPDATE organizations
			SET status = 'merged', merged_into_org_id = $2, merged_at = now(), updated_at = now()
			WHERE id = $1`
		if _, err := q.Exec(ctx, sqlOrg, sourceID, targetID); err != nil {
			return errf("MergeIdentity marcar origen", err)
		}
		if err := s.queueExternalOrgActivation(ctx, q, targetID); err != nil {
			return err
		}
		// Las plazas regaladas al origen siguen al destino: se concedieron a un
		// cliente, y ese cliente ahora vive allí.
		if _, err := q.Exec(ctx,
			`UPDATE seat_grants SET org_id = $2, updated_at = now()
			 WHERE org_id = $1 AND revoked_at IS NULL`, sourceID, targetID); err != nil {
			return errf("MergeIdentity plazas", err)
		}
		// El alta guiada de quien venía del origen SIGUE AL DESTINO Y QUEDA CERRADA.
		//
		// Sin esto, `onboarding_progress.org_id` se queda apuntando a la
		// organización absorbida mientras el `DELETE FROM memberships` de arriba
		// acaba de quitarle al usuario su pertenencia allí. Todo el asistente de
		// alta re-verifica ser OWNER de la org de su propia fila contra la BD, así
		// que a partir de la fusión respondía 403 «No eres propietario de esta
		// organización» en cada paso. Y como el portal manda a /bienvenida a quien
		// no tenga el alta completada, el usuario aterrizaba justo en la pantalla
		// que ya no podía cargar. Pasó en producción con corporate.
		//
		// Cerrarla, no solo reapuntarla: el alta terminó de hecho —la persona ya
		// está dentro de una organización operativa— y dejarla abierta contra el
		// destino tampoco funcionaría, porque a un `owner` del origen esta misma
		// función lo entra al destino como `admin`, y el asistente exige OWNER.
		//
		// COALESCE conserva la fecha original de quien ya la tuviera: la fusión no
		// reescribe cuándo se dio de alta nadie.
		if _, err := q.Exec(ctx,
			`UPDATE onboarding_progress
			 SET org_id = $2, completed_at = COALESCE(completed_at, now()), updated_at = now()
			 WHERE org_id = $1`, sourceID, targetID); err != nil {
			return errf("MergeIdentity alta guiada", err)
		}
		return nil
	})
	return moved, err
}
