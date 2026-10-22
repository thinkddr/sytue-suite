// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// GetOrCreateBillingAccount devuelve la cuenta de cobro del tenant, creándola
// (plan 'free', sin tope) si es la primera vez. Upsert idempotente bajo RLS.
func (s *Store) GetOrCreateBillingAccount(ctx context.Context) (domain.BillingAccount, error) {
	t := tenancy.MustFrom(ctx)
	var a domain.BillingAccount
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return scanBillingAccount(tx.QueryRow(ctx, `
			INSERT INTO billing_accounts (tenant) VALUES ($1)
			ON CONFLICT (tenant) DO UPDATE SET updated_at = billing_accounts.updated_at
			RETURNING tenant, plan, spend_limit_eur, created_at, updated_at`,
			t.Slug), &a)
	})
	return a, err
}

// SetPlan cambia el plan de cobro del tenant (crea la cuenta si no existe).
func (s *Store) SetPlan(ctx context.Context, plan domain.Plan) (domain.BillingAccount, error) {
	t := tenancy.MustFrom(ctx)
	var a domain.BillingAccount
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return scanBillingAccount(tx.QueryRow(ctx, `
			INSERT INTO billing_accounts (tenant, plan) VALUES ($1, $2)
			ON CONFLICT (tenant) DO UPDATE SET plan = excluded.plan, updated_at = now()
			RETURNING tenant, plan, spend_limit_eur, created_at, updated_at`,
			t.Slug, string(plan)), &a)
	})
	return a, err
}

// SetSpendLimit fija (o quita, con nil) el tope de gasto EUR del tenant.
func (s *Store) SetSpendLimit(ctx context.Context, limitEur *float64) (domain.BillingAccount, error) {
	t := tenancy.MustFrom(ctx)
	var a domain.BillingAccount
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return scanBillingAccount(tx.QueryRow(ctx, `
			INSERT INTO billing_accounts (tenant, spend_limit_eur) VALUES ($1, $2)
			ON CONFLICT (tenant) DO UPDATE SET spend_limit_eur = excluded.spend_limit_eur, updated_at = now()
			RETURNING tenant, plan, spend_limit_eur, created_at, updated_at`,
			t.Slug, limitEur), &a)
	})
	return a, err
}

// CreateInvoice persiste una factura de forma IDEMPOTENTE sobre
// UNIQUE(tenant, period_from, period_to): si ya existe una del mismo periodo, la
// devuelve sin duplicar (ON CONFLICT DO NOTHING + SELECT existente). Así
// re-ejecutar GenerateInvoice del mismo periodo no crea una segunda factura.
//
// created=true SOLO cuando el INSERT insertó fila (RETURNING dio resultado);
// false cuando cayó al SELECT de la existente. También resuelve la carrera
// concurrente: el perdedor del INSERT obtiene created=false.
func (s *Store) CreateInvoice(ctx context.Context, in domain.Invoice) (domain.Invoice, bool, error) {
	t := tenancy.MustFrom(ctx)
	linesJSON, err := json.Marshal(in.Lines)
	if err != nil {
		return domain.Invoice{}, false, fmtErr("serializar líneas de factura", err)
	}
	var out domain.Invoice
	created := false
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Inserta; si choca con el UNIQUE, no hace nada y RETURNING no da fila.
		row := tx.QueryRow(ctx, `
			INSERT INTO invoices (tenant, period_from, period_to, status, subtotal_eur, total_eur, lines, issued_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (tenant, period_from, period_to) DO NOTHING
			RETURNING id, tenant, period_from, period_to, status, subtotal_eur, total_eur, lines, created_at, issued_at`,
			t.Slug, in.PeriodFrom, in.PeriodTo, in.Status, in.SubtotalEur, in.TotalEur,
			linesJSON, in.IssuedAt)
		if err := scanInvoice(row, &out); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Ya existía: devuelve la factura persistida (idempotencia).
				return scanInvoice(tx.QueryRow(ctx, `
					SELECT id, tenant, period_from, period_to, status, subtotal_eur, total_eur, lines, created_at, issued_at
					FROM invoices WHERE tenant=$1 AND period_from=$2 AND period_to=$3`,
					t.Slug, in.PeriodFrom, in.PeriodTo), &out)
			}
			return fmtErr("crear factura", err)
		}
		created = true
		return nil
	})
	return out, created, err
}

// GetInvoice devuelve una factura del tenant por id.
func (s *Store) GetInvoice(ctx context.Context, id uuid.UUID) (domain.Invoice, error) {
	t := tenancy.MustFrom(ctx)
	var out domain.Invoice
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return scanInvoice(tx.QueryRow(ctx, `
			SELECT id, tenant, period_from, period_to, status, subtotal_eur, total_eur, lines, created_at, issued_at
			FROM invoices WHERE tenant=$1 AND id=$2`, t.Slug, id), &out)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Invoice{}, domain.ErrNotFound
	}
	return out, err
}

// ListInvoices lista las facturas del tenant por periodo DESC (keyset por
// (period_from, id) para no usar OFFSET a escala). after cero = primera página.
func (s *Store) ListInvoices(ctx context.Context, afterPeriod time.Time, afterID uuid.UUID, limit int) ([]domain.Invoice, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var out []domain.Invoice
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, period_from, period_to, status, subtotal_eur, total_eur, lines, created_at, issued_at
			FROM invoices
			WHERE tenant=$1 AND ($2 = false OR (period_from, id) < ($3, $4))
			ORDER BY period_from DESC, id DESC LIMIT $5`,
			t.Slug, !afterPeriod.IsZero(), afterPeriod, afterID, limit)
		if err != nil {
			return fmtErr("listar facturas", err)
		}
		defer rows.Close()
		for rows.Next() {
			var inv domain.Invoice
			if err := scanInvoice(rows, &inv); err != nil {
				return err
			}
			out = append(out, inv)
		}
		return rows.Err()
	})
	return out, err
}

// ListTenantsToInvoice devuelve, CROSS-TENANT, los tenants a los que les falta
// la factura del periodo [from, to) y que tienen algo que facturar. Lo usa el
// emisor mensual (service.RunInvoicer).
//
// Los dos filtros van en SQL, no en Go, y cada uno resuelve un problema distinto:
//
//   - El NOT EXISTS contra invoices hace que el barrido CONVERJA: en cuanto la
//     factura del periodo existe, el tenant deja de aparecer y no se vuelve a
//     agregar su uso. Sin él, cada ciclo recorrería a TODOS los tenants haciendo
//     una consulta de uso por cabeza para no emitir nada (CreateInvoice es
//     idempotente por el UNIQUE de periodo).
//   - El «plan de pago O uso en el periodo» evita emitir una factura de 0,00 €
//     cada mes a cada organización del plan gratuito que un día abrió la pestaña
//     de facturación (GetOrCreateBillingAccount crea la cuenta al mirarla).
func (s *Store) ListTenantsToInvoice(ctx context.Context, from, to time.Time, limit int) ([]string, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	var out []string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT b.tenant
			FROM billing_accounts b
			WHERE NOT EXISTS (
			          SELECT 1 FROM invoices i
			          WHERE i.tenant = b.tenant AND i.period_from = $1 AND i.period_to = $2)
			  AND (b.plan <> 'free'
			       OR EXISTS (
			              SELECT 1 FROM usage_records u
			              WHERE u.tenant = b.tenant AND u.period_from >= $1 AND u.period_from < $2))
			ORDER BY b.tenant
			LIMIT $3`, from, to, limit)
		if err != nil {
			return fmtErr("listar tenants a facturar", err)
		}
		defer rows.Close()
		for rows.Next() {
			var tenant string
			if err := rows.Scan(&tenant); err != nil {
				return err
			}
			out = append(out, tenant)
		}
		return rows.Err()
	})
	return out, err
}

// scanner abstrae pgx.Row y pgx.Rows para reutilizar el escaneo.
type scanner interface {
	Scan(dest ...any) error
}

func scanBillingAccount(row scanner, a *domain.BillingAccount) error {
	if err := row.Scan(&a.Tenant, &a.Plan, &a.SpendLimitEur, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return err
	}
	return nil
}

func scanInvoice(row scanner, inv *domain.Invoice) error {
	var linesJSON []byte
	if err := row.Scan(&inv.ID, &inv.Tenant, &inv.PeriodFrom, &inv.PeriodTo, &inv.Status,
		&inv.SubtotalEur, &inv.TotalEur, &linesJSON, &inv.CreatedAt, &inv.IssuedAt); err != nil {
		return err
	}
	if len(linesJSON) > 0 {
		if err := json.Unmarshal(linesJSON, &inv.Lines); err != nil {
			return err
		}
	}
	return nil
}
