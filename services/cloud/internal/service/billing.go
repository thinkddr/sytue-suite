// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// BillingAccount devuelve (creándola si es la primera vez) la cuenta de cobro del
// tenant activo.
func (s *Service) BillingAccount(ctx context.Context) (domain.BillingAccount, error) {
	return s.store.GetOrCreateBillingAccount(ctx)
}

// SetBillingPlan cambia el plan de cobro del tenant.
func (s *Service) SetBillingPlan(ctx context.Context, plan domain.Plan) (domain.BillingAccount, error) {
	if !domain.ValidPlan(plan) {
		return domain.BillingAccount{}, fmt.Errorf("%w: plan inválido", domain.ErrInvalidInput)
	}
	acct, err := s.store.SetPlan(ctx, plan)
	if err != nil {
		return domain.BillingAccount{}, err
	}
	s.audit(ctx, "billing.set_plan", string(plan), nil)
	return acct, nil
}

// SetSpendLimit fija (o quita, con nil) el tope de gasto EUR month-to-date del
// tenant. Un límite negativo es inválido.
func (s *Service) SetSpendLimit(ctx context.Context, limitEur *float64) (domain.BillingAccount, error) {
	if limitEur != nil && *limitEur < 0 {
		return domain.BillingAccount{}, fmt.Errorf("%w: el límite de gasto no puede ser negativo", domain.ErrInvalidInput)
	}
	// El tope acota el gasto TOTAL del mes (base del plan + overage), que es lo que
	// compara WithinSpendLimit. Debe ser >= la base del plan; si no, sería
	// insatisfacible (el gasto nunca baja de la base ya comprometida) y bloquearía
	// TODO despliegue con cero uso. ponytail: subir a un plan con base mayor que el
	// tope vigente reintroduce el caso; no se revalida aquí (gap conocido, raro).
	if limitEur != nil {
		cur, err := s.store.GetOrCreateBillingAccount(ctx)
		if err != nil {
			return domain.BillingAccount{}, err
		}
		if base := domain.PlanSpecFor(cur.Plan).BaseEur; *limitEur < base {
			return domain.BillingAccount{}, fmt.Errorf("%w: el límite (%.2f €) no puede ser menor que la cuota base del plan %s (%.2f €)",
				domain.ErrInvalidInput, *limitEur, cur.Plan, base)
		}
	}
	acct, err := s.store.SetSpendLimit(ctx, limitEur)
	if err != nil {
		return domain.BillingAccount{}, err
	}
	s.audit(ctx, "billing.set_spend_limit", fmt.Sprint(limitEur != nil), nil)
	return acct, nil
}

// GenerateInvoice calcula y persiste la factura del tenant para el periodo
// [from, to): lee el uso agregado, aplica el plan (cuota incluida + overage al
// precio de PricingTable + base), redondea a céntimos y persiste de forma
// IDEMPOTENTE (UNIQUE por periodo → re-ejecutar no duplica, devuelve la existente).
// Lo dispara un CronJob mensual vía POST /billing/invoices:generate.
func (s *Service) GenerateInvoice(ctx context.Context, from, to time.Time) (domain.Invoice, error) {
	if from.IsZero() || to.IsZero() || !from.Before(to) {
		return domain.Invoice{}, fmt.Errorf("%w: periodo de facturación inválido", domain.ErrInvalidInput)
	}
	acct, err := s.store.GetOrCreateBillingAccount(ctx)
	if err != nil {
		return domain.Invoice{}, err
	}
	agg, err := s.store.SummarizeUsage(ctx, from, to)
	if err != nil {
		return domain.Invoice{}, err
	}
	// La suscripción de la suite ya se cobra en Sytue ID mediante Mollie. Esta
	// factura contiene exclusivamente el plan y el consumo propios de Cloud.
	lines, subtotal, total := domain.BuildInvoiceLines(acct.Plan, usageMap(agg))
	now := time.Now().UTC()
	out, created, err := s.store.CreateInvoice(ctx, domain.Invoice{
		PeriodFrom:  from,
		PeriodTo:    to,
		Status:      domain.InvoiceIssued,
		SubtotalEur: subtotal,
		TotalEur:    total,
		Lines:       lines,
		IssuedAt:    &now,
	})
	if err != nil {
		return domain.Invoice{}, err
	}
	if !created {
		// Re-disparo del mismo periodo (reintento del CronJob, doble clic): se
		// devuelve la factura local existente sin duplicarla.
		return out, nil
	}
	s.audit(ctx, "billing.generate_invoice", out.ID.String(),
		map[string]any{"total_eur": out.TotalEur, "period_from": from, "period_to": to})

	return out, nil
}

// GetInvoice devuelve una factura del tenant por id.
func (s *Service) GetInvoice(ctx context.Context, id uuid.UUID) (domain.Invoice, error) {
	return s.store.GetInvoice(ctx, id)
}

// ListInvoices lista las facturas del tenant (periodo DESC, keyset).
func (s *Service) ListInvoices(ctx context.Context, afterPeriod time.Time, afterID uuid.UUID, limit int) ([]domain.Invoice, error) {
	return s.store.ListInvoices(ctx, afterPeriod, afterID, limit)
}

// CurrentSpend estima el gasto month-to-date del tenant: uso del mes en curso
// aplicado al plan (overage al precio de PricingTable + base del plan).
func (s *Service) CurrentSpend(ctx context.Context) (float64, error) {
	acct, err := s.store.GetOrCreateBillingAccount(ctx)
	if err != nil {
		return 0, err
	}
	return s.spendMonthToDate(ctx, acct.Plan)
}

// WithinSpendLimit compara el gasto month-to-date con el tope del tenant. Sin
// tope (nil) siempre está dentro y NO consulta el uso (barato para el caso común).
func (s *Service) WithinSpendLimit(ctx context.Context) (bool, error) {
	acct, err := s.store.GetOrCreateBillingAccount(ctx)
	if err != nil {
		return false, err
	}
	if acct.SpendLimitEur == nil {
		return true, nil
	}
	spend, err := s.spendMonthToDate(ctx, acct.Plan)
	if err != nil {
		return false, err
	}
	return spend <= *acct.SpendLimitEur, nil
}

// spendMonthToDate calcula el gasto del mes en curso para un plan.
func (s *Service) spendMonthToDate(ctx context.Context, plan domain.Plan) (float64, error) {
	now := time.Now().UTC()
	agg, err := s.store.SummarizeUsage(ctx, monthStart(now), now)
	if err != nil {
		return 0, err
	}
	_, _, total := domain.BuildInvoiceLines(plan, usageMap(agg))
	return total, nil
}

// usageMap reduce el uso agregado a métrica → cantidad.
func usageMap(agg []store.UsageAggregate) map[string]float64 {
	m := make(map[string]float64, len(agg))
	for _, a := range agg {
		m[a.Metric] = a.Total
	}
	return m
}

// monthStart devuelve el inicio (UTC) del mes de t.
func monthStart(t time.Time) time.Time {
	y, m, _ := t.UTC().Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
}

// MeetsSuiteRequirement comprueba si la organización cumple el requisito de
// suscripción a la suite de su plan (Free exige al menos un puesto).
//
// Va aquí, junto a WithinSpendLimit, porque es el mismo tipo de control: una
// condición comercial que se evalúa antes de comprometer recursos. El dato vive
// en las memberships espejadas de la organización y el criterio, en el dominio.
func (s *Service) MeetsSuiteRequirement(ctx context.Context) (bool, error) {
	org, err := s.store.GetOrganization(ctx)
	if err != nil {
		return false, err
	}
	seats, err := s.SuiteSeats(ctx)
	if err != nil {
		return false, err
	}
	return domain.MeetsSuiteRequirement(org.Plan, seats), nil
}

// SuiteSeats son los puestos observados en la organización: se DERIVAN del número
// de miembros, no de un valor tecleado que quedaría obsoleto. La autorización de
// pago real la aplica Sytue ID antes de emitir el tenant; esta es una barrera local
// adicional para no comprometer cómputo en una organización vacía.
//
// ponytail: se lee en el instante de facturar, así que un alta a mitad de mes
// cuenta como mes completo. Prorratear exige historial de membresía; cuando haga
// falta, el sitio es este.
func (s *Service) SuiteSeats(ctx context.Context) (int, error) {
	return s.store.CountMemberships(ctx)
}
