// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// Tipos de evento de transición de uptime (canal "cloud", outbox transaccional).
// Los consume el consumer de alertas (service, patrón DeployNotifier), que los
// mapea a los eventos cortos de alert_policies (uptime.down / uptime.up).
const (
	EventUptimeDown = "cloud.uptime.down"
	EventUptimeUp   = "cloud.uptime.up"
)

const uptimeCols = `id, tenant, project_id, name, target_url, interval_seconds, enabled,
	next_run_at, last_status, last_latency_ms, last_checked_at, created_at`

func scanUptimeCheck(row pgx.Row, t string) (domain.UptimeCheck, error) {
	var c domain.UptimeCheck
	if err := row.Scan(&c.ID, &c.Tenant, &c.ProjectID, &c.Name, &c.TargetURL,
		&c.IntervalSeconds, &c.Enabled, &c.NextRunAt, &c.LastStatus,
		&c.LastLatencyMS, &c.LastCheckedAt, &c.CreatedAt); err != nil {
		return domain.UptimeCheck{}, err
	}
	if t != "" { // t vacío: conserva el tenant de la fila (lecturas cross-tenant)
		c.Tenant = t
	}
	return c, nil
}

// NewUptimeCheckRecord son los datos de creación de un uptime check. target_url
// se valida anti-SSRF en la capa service ANTES de llegar aquí.
type NewUptimeCheckRecord struct {
	ProjectID       uuid.UUID
	Name            string
	TargetURL       string
	IntervalSeconds int
	Enabled         bool
}

// CreateUptimeCheck inserta un check tras verificar que el proyecto existe y es
// del tenant. next_run_at cae al DEFAULT now() ⇒ el prober lo toma en el próximo tick.
func (s *Store) CreateUptimeCheck(ctx context.Context, rec NewUptimeCheckRecord) (domain.UptimeCheck, error) {
	t := tenancy.MustFrom(ctx)
	var c domain.UptimeCheck
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var ok bool
		if err := tx.QueryRow(ctx,
			`SELECT true FROM projects WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, rec.ProjectID).Scan(&ok); err != nil {
			return normalizeNoRows(err, domain.ErrNotFound)
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO uptime_checks (tenant, project_id, name, target_url, interval_seconds, enabled)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING `+uptimeCols,
			t.Slug, rec.ProjectID, rec.Name, rec.TargetURL, rec.IntervalSeconds, rec.Enabled)
		var e error
		c, e = scanUptimeCheck(row, t.Slug)
		if checkViolation(e) {
			return domain.ErrInvalidInput
		}
		return e
	})
	return c, err
}

// GetUptimeCheck devuelve un check por ID.
func (s *Store) GetUptimeCheck(ctx context.Context, id uuid.UUID) (domain.UptimeCheck, error) {
	t := tenancy.MustFrom(ctx)
	var c domain.UptimeCheck
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT `+uptimeCols+` FROM uptime_checks WHERE tenant=$1 AND id=$2`, t.Slug, id)
		var e error
		c, e = scanUptimeCheck(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UptimeCheck{}, domain.ErrNotFound
	}
	return c, err
}

// ListUptimeChecks lista los checks de un proyecto (orden estable por creación).
func (s *Store) ListUptimeChecks(ctx context.Context, projectID uuid.UUID) ([]domain.UptimeCheck, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.UptimeCheck
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+uptimeCols+` FROM uptime_checks WHERE tenant=$1 AND project_id=$2 ORDER BY created_at`,
			t.Slug, projectID)
		if err != nil {
			return fmtErr("listar uptime checks", err)
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanUptimeCheck(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// UpdateUptimeCheckRecord son los campos mutables (punteros = opcional).
type UpdateUptimeCheckRecord struct {
	Name            *string
	TargetURL       *string
	IntervalSeconds *int
	Enabled         *bool
}

// UpdateUptimeCheck aplica una actualización parcial. Reactivar (enabled=true) o
// cambiar el objetivo re-programa el próximo sondeo a ya (next_run_at = now()).
func (s *Store) UpdateUptimeCheck(ctx context.Context, id uuid.UUID, upd UpdateUptimeCheckRecord) (domain.UptimeCheck, error) {
	t := tenancy.MustFrom(ctx)
	// Re-sondear pronto si cambia el objetivo o se reactiva.
	reschedule := upd.TargetURL != nil || (upd.Enabled != nil && *upd.Enabled)
	var c domain.UptimeCheck
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE uptime_checks SET
				name             = COALESCE($3, name),
				target_url       = COALESCE($4, target_url),
				interval_seconds = COALESCE($5, interval_seconds),
				enabled          = COALESCE($6, enabled),
				next_run_at      = CASE WHEN $7 THEN now() ELSE next_run_at END
			WHERE tenant=$1 AND id=$2
			RETURNING `+uptimeCols,
			t.Slug, id, upd.Name, upd.TargetURL, upd.IntervalSeconds, upd.Enabled, reschedule)
		var e error
		c, e = scanUptimeCheck(row, t.Slug)
		if checkViolation(e) {
			return domain.ErrInvalidInput
		}
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UptimeCheck{}, domain.ErrNotFound
	}
	return c, err
}

// DeleteUptimeCheck elimina un check.
func (s *Store) DeleteUptimeCheck(ctx context.Context, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM uptime_checks WHERE tenant=$1 AND id=$2`, t.Slug, id)
		if err != nil {
			return fmtErr("borrar uptime check", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// ClaimedCheck son los datos que el prober necesita para sondear un check y
// detectar la transición (LastStatus es el estado ANTES de este sondeo).
type ClaimedCheck struct {
	ID              uuid.UUID
	Tenant          string
	ProjectID       uuid.UUID
	Name            string
	TargetURL       string
	IntervalSeconds int
	LastStatus      string
}

// ClaimDueChecks reclama atómicamente hasta `limit` checks vencidos y habilitados
// (cualquier tenant) y los "arrienda" fijando next_run_at = now()+interval, de
// modo que otra réplica no los re-tome en el mismo ciclo. FOR UPDATE SKIP LOCKED
// reparte el trabajo entre réplicas sin colisión. Cross-tenant vía InMaintenanceTx
// (política uptime_checks_maintenance, 0010): con FORCE RLS y rol sin BYPASSRLS
// sería deny-by-default. El sondeo HTTP se hace FUERA de esta tx (no se retienen
// locks durante ~10s); el resultado se registra luego con RecordCheckResult.
func (s *Store) ClaimDueChecks(ctx context.Context, now time.Time, limit int) ([]ClaimedCheck, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []ClaimedCheck
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			UPDATE uptime_checks SET next_run_at = $1 + make_interval(secs => interval_seconds)
			WHERE id IN (
				SELECT id FROM uptime_checks
				WHERE enabled AND next_run_at <= $1
				ORDER BY next_run_at
				FOR UPDATE SKIP LOCKED
				LIMIT $2
			)
			RETURNING id, tenant, project_id, name, target_url, interval_seconds, last_status`,
			now, limit)
		if err != nil {
			return fmtErr("reclamar uptime checks", err)
		}
		defer rows.Close()
		for rows.Next() {
			var c ClaimedCheck
			if err := rows.Scan(&c.ID, &c.Tenant, &c.ProjectID, &c.Name, &c.TargetURL,
				&c.IntervalSeconds, &c.LastStatus); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// uptimeTransitionEvent devuelve el tipo de evento del bus a emitir cuando el
// estado de un check pasa de old a new, y ok=false si no hay transición que
// notificar. El baseline (old = "" desconocido, primer sondeo) NO emite: una
// transición up↔down requiere un estado previo conocido.
func uptimeTransitionEvent(old, next string) (string, bool) {
	if old == next || old == domain.UptimeStatusUnknown {
		return "", false
	}
	switch next {
	case domain.UptimeStatusDown:
		return EventUptimeDown, true
	case domain.UptimeStatusUp:
		return EventUptimeUp, true
	default:
		return "", false
	}
}

// UptimeEventPayload es el cuerpo del evento de transición (lo lee el consumer de
// alertas para enrutar por proyecto y componer el webhook).
type UptimeEventPayload struct {
	CheckID   uuid.UUID `json:"check_id"`
	ProjectID uuid.UUID `json:"project_id"`
	Name      string    `json:"name"`
	TargetURL string    `json:"target_url"`
	Status    string    `json:"status"`
	LatencyMS int       `json:"latency_ms"`
}

// RecordCheckResult guarda el último estado del sondeo y, si hay transición
// up↔down (comparando con el estado previo del claim), publica el evento en el
// outbox DENTRO de la misma tx (atómico). Cross-tenant: fija app.tenant al tenant
// del check reclamado. now es el instante del sondeo (last_checked_at).
func (s *Store) RecordCheckResult(ctx context.Context, c ClaimedCheck, status string, latencyMS int, now time.Time) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, c.Tenant); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE uptime_checks SET last_status=$3, last_latency_ms=$4, last_checked_at=$5
			WHERE tenant=$1 AND id=$2`, c.Tenant, c.ID, status, latencyMS, now); err != nil {
			return fmtErr("registrar resultado de uptime check", err)
		}
		evType, ok := uptimeTransitionEvent(c.LastStatus, status)
		if !ok {
			return nil
		}
		ev, err := events.NewEvent(EventChannel, evType, c.Tenant, UptimeEventPayload{
			CheckID: c.ID, ProjectID: c.ProjectID, Name: c.Name,
			TargetURL: c.TargetURL, Status: status, LatencyMS: latencyMS,
		})
		if err != nil {
			return err
		}
		return events.Publish(ctx, tx, ev)
	})
}
