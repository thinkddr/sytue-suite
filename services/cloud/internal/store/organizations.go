// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

const orgCols = `id, tenant, name, plan, region, notify_webhook_url, created_at, updated_at`

func scanOrg(row pgx.Row) (domain.Organization, error) {
	var o domain.Organization
	if err := row.Scan(&o.ID, &o.Slug, &o.Name, &o.Plan, &o.Region, &o.NotifyWebhookURL,
		&o.CreatedAt, &o.UpdatedAt); err != nil {
		return domain.Organization{}, err
	}
	return o, nil
}

// EnsureOrganization crea la organización del tenant activo si no existe y la
// devuelve. El slug es el propio tenant (org/<slug>).
func (s *Store) EnsureOrganization(ctx context.Context, name string, plan domain.Plan, region string) (domain.Organization, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidPlan(plan) {
		plan = domain.PlanFree
	}
	if region == "" {
		region = "fr-par"
	}
	var o domain.Organization
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Leer ANTES de escribir. Esto sirve GET /org, el arranque de la consola, y
		// `DO UPDATE SET tenant = EXCLUDED.tenant` escribe una versión nueva de la
		// fila sin cambiar ningún valor: tupla muerta, WAL y bloqueo sobre la única
		// fila del tenant en cada carga de página.
		//
		// SIN filtrar deleted_at, a propósito: el upsert tampoco lo miraba, así que
		// una organización en borrado lógico se devolvía igual. Añadir el filtro
		// aquí la haría reaparecer recreada, que es otro cambio y no este.
		row := tx.QueryRow(ctx, `SELECT `+orgCols+` FROM organizations WHERE tenant=$1`, t.Slug)
		switch got, gerr := scanOrg(row); {
		case gerr == nil:
			o = got
			return nil
		case !errors.Is(gerr, pgx.ErrNoRows):
			return gerr
		}
		// Primer contacto del tenant. Sigue siendo DO UPDATE y no DO NOTHING: si
		// otra transacción crea la fila entremedias, DO NOTHING no devolvería
		// ninguna y el alta fallaría por ErrNoRows.
		row = tx.QueryRow(ctx, `
			INSERT INTO organizations (tenant, name, plan, region)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (tenant) DO UPDATE SET tenant = EXCLUDED.tenant
			RETURNING `+orgCols, t.Slug, name, string(plan), region)
		var e error
		o, e = scanOrg(row)
		return e
	})
	return o, err
}

// GetOrganization devuelve la organización del tenant activo.
func (s *Store) GetOrganization(ctx context.Context) (domain.Organization, error) {
	t := tenancy.MustFrom(ctx)
	var o domain.Organization
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+orgCols+` FROM organizations WHERE tenant=$1 AND deleted_at IS NULL`, t.Slug)
		var e error
		o, e = scanOrg(row)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Organization{}, domain.ErrNotFound
	}
	return o, err
}

// UpdateOrganization actualiza nombre/plan/URL de aviso de la organización del
// tenant. notifyWebhookURL nil deja el valor actual (parcial).
func (s *Store) UpdateOrganization(ctx context.Context, name *string, plan *domain.Plan, notifyWebhookURL *string) (domain.Organization, error) {
	t := tenancy.MustFrom(ctx)
	var o domain.Organization
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE organizations SET
				name = COALESCE($2, name),
				plan = COALESCE($3, plan),
				notify_webhook_url = COALESCE($4, notify_webhook_url),
				updated_at = now()
			WHERE tenant=$1 AND deleted_at IS NULL
			RETURNING `+orgCols, t.Slug, name, planPtr(plan), notifyWebhookURL)
		var e error
		o, e = scanOrg(row)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Organization{}, domain.ErrNotFound
	}
	return o, err
}

func planPtr(p *domain.Plan) *string {
	if p == nil {
		return nil
	}
	s := string(*p)
	return &s
}

// GetOrganizationPlanByTenant devuelve el plan de facturación de un tenant.
// Cross-tenant (lo invoca el reconciliador vía InMaintenanceTx).
func (s *Store) GetOrganizationPlanByTenant(ctx context.Context, tenant string) (domain.Plan, error) {
	var raw string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT plan FROM organizations WHERE tenant=$1 AND deleted_at IS NULL`, tenant).
			Scan(&raw)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PlanFree, domain.ErrNotFound
	}
	if err != nil {
		return domain.PlanFree, err
	}
	return domain.Plan(raw), nil
}
