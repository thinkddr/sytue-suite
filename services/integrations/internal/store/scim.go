// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// SyncChannel es el canal del bus por el que viajan los trabajos de sincronización
// SCIM saliente. El Dispatcher del binario escucha aquí; POST /sync publica.
const SyncChannel events.Channel = "integrations"

// SyncRequestedType es el tipo de evento que dispara una reconciliación.
const SyncRequestedType = "integrations.sync.requested"

// SyncJob es la carga útil del evento de sincronización (qué instancia reconciliar).
// La org viaja en Event.Tenant (para reconstruir el contexto por-tenant en el job).
type SyncJob struct {
	InstanceID string `json:"instance_id"`
}

// EnqueueSync verifica (bajo RLS) que la instancia existe en la org activa y
// publica un evento de reconciliación en el outbox, ATÓMICAMENTE en la misma
// transacción: si la instancia no existe, no se encola nada. El Dispatcher lo
// recogerá con su backoff/attempts. ErrNotFound si la instancia no está en la org.
func (s *Store) EnqueueSync(ctx context.Context, instanceID string) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	ev, err := events.NewEvent(SyncChannel, SyncRequestedType, t.Slug, SyncJob{InstanceID: instanceID})
	if err != nil {
		return fmt.Errorf("store: construir evento de sync: %w", err)
	}
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_instances WHERE org_id = current_setting('app.tenant', true) AND id = $1)`, instanceID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return domain.ErrNotFound
		}
		return events.Publish(ctx, tx, ev)
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("store: encolar sync: %w", err)
	}
	return nil
}

// LoadScimMap carga EN BLOQUE el mapeo Sytue↔remoto de una instancia (usuarios y
// grupos). El reconcile lo indexa en memoria para diffear sin N+1. RLS por org.
func (s *Store) LoadScimMap(ctx context.Context, instanceID string) ([]domain.ScimMapEntry, error) {
	out := make([]domain.ScimMapEntry, 0, 64)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT kind, sytue_id, remote_id, state_hash FROM scim_map WHERE org_id = current_setting('app.tenant', true) AND instance_id = $1`, instanceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e domain.ScimMapEntry
			var kind string
			if err := rows.Scan(&kind, &e.SytueID, &e.RemoteID, &e.Hash); err != nil {
				return err
			}
			e.Kind = domain.ScimKind(kind)
			out = append(out, e)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: cargar scim_map: %w", err)
	}
	return out, nil
}

// UpsertScimMap inserta o actualiza el mapeo de una entidad (tras crear/adoptar/
// actualizar el recurso remoto). RLS por org (org_id del contexto activo).
func (s *Store) UpsertScimMap(ctx context.Context, instanceID string, e domain.ScimMapEntry) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	return s.execAffecting(ctx, "upsert scim_map", `
		INSERT INTO scim_map (instance_id, org_id, kind, sytue_id, remote_id, state_hash)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (instance_id, kind, sytue_id)
		DO UPDATE SET remote_id = EXCLUDED.remote_id, state_hash = EXCLUDED.state_hash, updated_at = now()`,
		instanceID, t.Slug, string(e.Kind), e.SytueID, e.RemoteID, e.Hash)
}

// DeleteScimMap borra el mapeo de una entidad (tras desactivarla en el SaaS): el
// recurso remoto se desactiva/borra, pero el vínculo local desaparece para que un
// re-alta futura vuelva a adoptar por filtro. RLS por org.
func (s *Store) DeleteScimMap(ctx context.Context, instanceID string, kind domain.ScimKind, sytueID string) error {
	return s.execAffecting(ctx, "borrar scim_map",
		`DELETE FROM scim_map WHERE org_id = current_setting('app.tenant', true) AND instance_id = $1 AND kind = $2 AND sytue_id = $3`,
		instanceID, string(kind), sytueID)
}

// StartSyncRun abre una fila de sync_runs en estado 'running' y devuelve su id.
func (s *Store) StartSyncRun(ctx context.Context, instanceID string) (string, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return "", err
	}
	var id string
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO sync_runs (instance_id, org_id, status) VALUES ($1, $2, 'running') RETURNING id`,
			instanceID, t.Slug).Scan(&id)
	})
	if err != nil {
		return "", fmt.Errorf("store: abrir sync_run: %w", err)
	}
	return id, nil
}

// FinishSyncRun cierra una fila de sync_runs con los contadores y el estado final.
func (s *Store) FinishSyncRun(ctx context.Context, runID string, c domain.SyncCounts, status string) error {
	return s.execAffecting(ctx, "cerrar sync_run", `
		UPDATE sync_runs
		   SET status = $2, created_count = $3, updated_count = $4,
		       deactivated_count = $5, error_count = $6, finished_at = now()
		 WHERE org_id = current_setting('app.tenant', true) AND id = $1`,
		runID, status, c.Created, c.Updated, c.Deactivated, c.Errors)
}

// ListSyncRuns devuelve el historial de sincronizaciones de una instancia, más
// reciente primero (índice keyset). limit acota el tamaño de página.
func (s *Store) ListSyncRuns(ctx context.Context, instanceID string, limit int) ([]domain.SyncRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	out := make([]domain.SyncRun, 0, limit)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, instance_id, status, created_count, updated_count,
			       deactivated_count, error_count, started_at, finished_at
			  FROM sync_runs
			 WHERE org_id = current_setting('app.tenant', true) AND instance_id = $1
			 ORDER BY started_at DESC, id DESC
			 LIMIT $2`, instanceID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var run domain.SyncRun
			if err := rows.Scan(&run.ID, &run.InstanceID, &run.Status,
				&run.Counts.Created, &run.Counts.Updated, &run.Counts.Deactivated,
				&run.Counts.Errors, &run.StartedAt, &run.FinishedAt); err != nil {
				return err
			}
			out = append(out, run)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar sync_runs: %w", err)
	}
	return out, nil
}

// SyncCandidate es una conexión que el barrido periódico debe reconciliar.
type SyncCandidate struct {
	ID    string
	OrgID string
}

// ListSyncCandidates enumera las conexiones ACTIVAS de TODAS las organizaciones cuya
// plantilla declara aprovisionamiento SCIM.
//
// Cruza el aislamiento por tenant a propósito y por la vía legítima: una transacción de
// MANTENIMIENTO (`app.maintenance='on'`), que activa la política PERMISSIVE de la
// migración 0008. Las transacciones de negocio nunca fijan ese flag, así que el
// aislamiento por organización sigue intacto y el rol de aplicación sigue sin poder
// saltarse la RLS. No hace falta un rol con BYPASSRLS —que era lo que el TODO del código
// asumía— y por eso no se introduce.
//
// El filtro por plantilla se hace EN SQL y no en Go: encolar una reconciliación para una
// conexión que no aprovisiona sería trabajo que el outbox acabaría descartando, y con
// muchas organizaciones eso es ruido caro.
func (s *Store) ListSyncCandidates(ctx context.Context, limit int) ([]SyncCandidate, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	out := make([]SyncCandidate, 0, 64)
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT i.id, i.org_id
			FROM app_instances i
			JOIN app_templates t ON t.key = i.template_key
			WHERE i.status = 'active'
			  AND t.enabled
			  AND t.manifest->>'provisioning' = 'scim'
			ORDER BY i.updated_at
			LIMIT $1`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c SyncCandidate
			if err := rows.Scan(&c.ID, &c.OrgID); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: enumerar conexiones a reconciliar: %w", err)
	}
	return out, nil
}
