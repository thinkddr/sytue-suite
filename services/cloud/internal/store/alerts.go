// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

const alertCols = `id, tenant, project_id, event, channel_url, enabled, created_at`

func scanAlertPolicy(row pgx.Row, t string) (domain.AlertPolicy, error) {
	var p domain.AlertPolicy
	if err := row.Scan(&p.ID, &p.Tenant, &p.ProjectID, &p.Event, &p.ChannelURL,
		&p.Enabled, &p.CreatedAt); err != nil {
		return domain.AlertPolicy{}, err
	}
	if t != "" {
		p.Tenant = t
	}
	return p, nil
}

// NewAlertPolicyRecord son los datos de creación de una policy. channel_url se
// valida anti-SSRF en la capa service ANTES de llegar aquí. ProjectID nil = org.
type NewAlertPolicyRecord struct {
	ProjectID  *uuid.UUID
	Event      string
	ChannelURL string
	Enabled    bool
}

// CreateAlertPolicy inserta una policy. Si lleva project_id, verifica que el
// proyecto existe y es del tenant (evita policies colgando de proyectos ajenos).
func (s *Store) CreateAlertPolicy(ctx context.Context, rec NewAlertPolicyRecord) (domain.AlertPolicy, error) {
	t := tenancy.MustFrom(ctx)
	var p domain.AlertPolicy
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if rec.ProjectID != nil {
			var ok bool
			if err := tx.QueryRow(ctx,
				`SELECT true FROM projects WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
				t.Slug, *rec.ProjectID).Scan(&ok); err != nil {
				return normalizeNoRows(err, domain.ErrNotFound)
			}
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO alert_policies (tenant, project_id, event, channel_url, enabled)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING `+alertCols,
			t.Slug, rec.ProjectID, rec.Event, rec.ChannelURL, rec.Enabled)
		var e error
		p, e = scanAlertPolicy(row, t.Slug)
		if checkViolation(e) {
			return domain.ErrInvalidInput
		}
		return e
	})
	return p, err
}

// ListAlertPolicies lista todas las policies del tenant (recurso de organización).
func (s *Store) ListAlertPolicies(ctx context.Context) ([]domain.AlertPolicy, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.AlertPolicy
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+alertCols+` FROM alert_policies WHERE tenant=$1 ORDER BY created_at`, t.Slug)
		if err != nil {
			return fmtErr("listar alert policies", err)
		}
		defer rows.Close()
		for rows.Next() {
			p, err := scanAlertPolicy(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// UpdateAlertPolicyRecord son los campos mutables (punteros = opcional).
type UpdateAlertPolicyRecord struct {
	Event      *string
	ChannelURL *string
	Enabled    *bool
}

// UpdateAlertPolicy aplica una actualización parcial.
func (s *Store) UpdateAlertPolicy(ctx context.Context, id uuid.UUID, upd UpdateAlertPolicyRecord) (domain.AlertPolicy, error) {
	t := tenancy.MustFrom(ctx)
	var p domain.AlertPolicy
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE alert_policies SET
				event       = COALESCE($3, event),
				channel_url = COALESCE($4, channel_url),
				enabled     = COALESCE($5, enabled)
			WHERE tenant=$1 AND id=$2
			RETURNING `+alertCols,
			t.Slug, id, upd.Event, upd.ChannelURL, upd.Enabled)
		var e error
		p, e = scanAlertPolicy(row, t.Slug)
		if checkViolation(e) {
			return domain.ErrInvalidInput
		}
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AlertPolicy{}, domain.ErrNotFound
	}
	return p, err
}

// DeleteAlertPolicy elimina una policy (y en cascada sus alert_deliveries).
func (s *Store) DeleteAlertPolicy(ctx context.Context, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM alert_policies WHERE tenant=$1 AND id=$2`, t.Slug, id)
		if err != nil {
			return fmtErr("borrar alert policy", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// MatchAlertPolicies resuelve las policies habilitadas del tenant que casan un
// evento: el mismo `event` y alcance de proyecto (project_id = projectID) O de
// organización (project_id IS NULL). Cross-tenant: el consumer pasa el tenant
// del evento y se fija app.tenant bajo RLS. Sirve el índice alert_policies_route_idx.
func (s *Store) MatchAlertPolicies(ctx context.Context, tenant, event string, projectID uuid.UUID) ([]domain.AlertPolicy, error) {
	var out []domain.AlertPolicy
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+alertCols+` FROM alert_policies
			WHERE tenant=$1 AND event=$2 AND enabled AND (project_id = $3 OR project_id IS NULL)`,
			tenant, event, projectID)
		if err != nil {
			return fmtErr("resolver alert policies", err)
		}
		defer rows.Close()
		for rows.Next() {
			p, err := scanAlertPolicy(rows, tenant)
			if err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// ClaimAlertDelivery reclama de forma idempotente el envío de (policy_id,
// event_id): INSERT ON CONFLICT DO NOTHING. Devuelve claimed=true solo si esta
// llamada insertó (no se había enviado). El consumer envía solo si reclama y
// libera (Release) si el envío falla, para que un reintento del Dispatcher lo
// reenvíe. event_id = id del evento del outbox (estable entre reentregas).
func (s *Store) ClaimAlertDelivery(ctx context.Context, tenant string, policyID, eventID uuid.UUID) (bool, error) {
	claimed := false
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO alert_deliveries (policy_id, event_id, tenant)
			VALUES ($1, $2, $3)
			ON CONFLICT (policy_id, event_id) DO NOTHING`, policyID, eventID, tenant)
		if err != nil {
			return fmtErr("reclamar alert delivery", err)
		}
		claimed = tag.RowsAffected() == 1
		return nil
	})
	return claimed, err
}

// ReleaseAlertDelivery libera la reclamación cuando el envío falla (para reenvío).
func (s *Store) ReleaseAlertDelivery(ctx context.Context, tenant string, policyID, eventID uuid.UUID) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM alert_deliveries WHERE tenant=$1 AND policy_id=$2 AND event_id=$3`,
			tenant, policyID, eventID); err != nil {
			return fmtErr("liberar alert delivery", err)
		}
		return nil
	})
}
