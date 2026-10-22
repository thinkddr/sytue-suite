// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

func floatPtr(f float64) *float64 { return &f }

// TestWithinSpendLimit cubre los tres casos: sin tope (nil), bajo tope y sobre él.
func TestWithinSpendLimit(t *testing.T) {
	ctx := tenantCtx(t)

	// nil: siempre dentro, sin consultar el uso.
	fs := &fakeStore{billingAcct: domain.BillingAccount{Plan: domain.PlanFree}}
	s := newTestService(fs, nil)
	if ok, err := s.WithinSpendLimit(ctx); err != nil || !ok {
		t.Fatalf("tope nil: ok=%v err=%v, esperado ok", ok, err)
	}

	// Bajo tope: gasto free = base 0 + overage. build 200 → 100 overage × 0,005 = 0,50.
	fs = &fakeStore{
		billingAcct: domain.BillingAccount{Plan: domain.PlanFree, SpendLimitEur: floatPtr(1.00)},
		usageAgg:    []store.UsageAggregate{{Metric: "build_minutes", Total: 200}},
	}
	s = newTestService(fs, nil)
	if ok, err := s.WithinSpendLimit(ctx); err != nil || !ok {
		t.Fatalf("bajo tope: ok=%v err=%v, esperado dentro", ok, err)
	}

	// Sobre tope: mismo gasto 0,50 pero tope 0,25 → fuera.
	fs = &fakeStore{
		billingAcct: domain.BillingAccount{Plan: domain.PlanFree, SpendLimitEur: floatPtr(0.25)},
		usageAgg:    []store.UsageAggregate{{Metric: "build_minutes", Total: 200}},
	}
	s = newTestService(fs, nil)
	if ok, err := s.WithinSpendLimit(ctx); err != nil || ok {
		t.Fatalf("sobre tope: ok=%v err=%v, esperado fuera", ok, err)
	}
}

// TestCurrentSpendIncludesBase: el gasto month-to-date incluye la base del plan.
func TestCurrentSpendIncludesBase(t *testing.T) {
	fs := &fakeStore{
		billingAcct: domain.BillingAccount{Plan: domain.PlanPro}, // base 19
		usageAgg:    []store.UsageAggregate{{Metric: "build_minutes", Total: 500}},
	}
	s := newTestService(fs, nil)
	spend, err := s.CurrentSpend(tenantCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	// 500 < cuota pro 1000 → 0 overage; solo base 19.
	if spend != 19 {
		t.Errorf("gasto = %v, esperado 19 (solo base)", spend)
	}
}

// TestGenerateInvoiceMath: la factura persiste el cálculo correcto (cuota+overage+base).
func TestGenerateInvoiceMath(t *testing.T) {
	fs := &fakeStore{
		billingAcct: domain.BillingAccount{Plan: domain.PlanPro},
		usageAgg:    []store.UsageAggregate{{Metric: "build_minutes", Total: 1500}}, // 500 over × 0,005 = 2,50
	}
	s := newTestService(fs, nil)
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	inv, err := s.GenerateInvoice(tenantCtx(t), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if inv.SubtotalEur != 2.50 || inv.TotalEur != 21.50 {
		t.Errorf("subtotal=%v total=%v, esperado 2,50 / 21,50", inv.SubtotalEur, inv.TotalEur)
	}
	if inv.Status != domain.InvoiceIssued {
		t.Errorf("status=%q, esperado issued", inv.Status)
	}
	if fs.newInvoice == nil || fs.newInvoice.Status != domain.InvoiceIssued {
		t.Error("no se persistió la factura")
	}
}

// TestGenerateInvoiceIdempotent: si el store ya tiene una factura del periodo
// (simulando ON CONFLICT DO NOTHING + SELECT), se devuelve esa sin duplicar.
func TestGenerateInvoiceIdempotent(t *testing.T) {
	existing := domain.Invoice{ID: uuid.New(), Status: domain.InvoiceIssued, TotalEur: 99}
	fs := &fakeStore{
		billingAcct: domain.BillingAccount{Plan: domain.PlanFree},
		usageAgg:    []store.UsageAggregate{{Metric: "build_minutes", Total: 1500}},
		invoice:     existing, // el store devuelve la existente (created=false)
	}
	s := newTestService(fs, nil)
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	inv, err := s.GenerateInvoice(tenantCtx(t), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if inv.ID != existing.ID || inv.TotalEur != 99 {
		t.Errorf("no idempotente: got %+v, esperada la existente %v", inv, existing.ID)
	}
}

// TestSetSpendLimitRejectsBelowPlanBase: un tope por debajo de la base del plan
// sería insatisfacible (bloquearía todo despliegue con cero uso) → ErrInvalidInput.
func TestSetSpendLimitRejectsBelowPlanBase(t *testing.T) {
	ctx := tenantCtx(t)
	fs := &fakeStore{billingAcct: domain.BillingAccount{Plan: domain.PlanPro}} // base 19
	s := newTestService(fs, nil)

	if _, err := s.SetSpendLimit(ctx, floatPtr(10)); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("tope 10 < base 19 debería ser ErrInvalidInput, got %v", err)
	}
	if _, err := s.SetSpendLimit(ctx, floatPtr(25)); err != nil {
		t.Errorf("tope 25 >= base 19 debería aceptarse, got %v", err)
	}
	// Sin tope (nil) siempre válido, sin importar el plan.
	if _, err := s.SetSpendLimit(ctx, nil); err != nil {
		t.Errorf("quitar el tope (nil) debería aceptarse, got %v", err)
	}
}

func TestGenerateInvoiceRejectsBadPeriod(t *testing.T) {
	s := newTestService(&fakeStore{}, nil)
	now := time.Now().UTC()
	if _, err := s.GenerateInvoice(tenantCtx(t), now, now); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("from==to debería ser inválido, got %v", err)
	}
	if _, err := s.GenerateInvoice(tenantCtx(t), now, now.Add(-time.Hour)); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("to<from debería ser inválido, got %v", err)
	}
}

// TestDeployRejectedOverSpendLimit: el choke point rechaza el build si el tenant
// superó su tope de gasto.
func TestDeployRejectedOverSpendLimit(t *testing.T) {
	fs := &fakeStore{
		project:     domain.Project{ID: uuid.New(), Slug: "app", ProductionRef: "main"},
		billingAcct: domain.BillingAccount{Plan: domain.PlanFree, SpendLimitEur: floatPtr(0.10)},
		usageAgg:    []store.UsageAggregate{{Metric: "build_minutes", Total: 500}}, // 400 over × 0,005 = 2,00 > 0,10
	}
	s := newTestService(fs, nil)
	_, _, err := s.Deploy(tenantCtx(t), DeployInput{
		ProjectID: fs.project.ID,
		Target:    domain.TargetProduction,
	})
	if !errors.Is(err, domain.ErrSpendLimitExceeded) {
		t.Fatalf("esperado ErrSpendLimitExceeded, got %v", err)
	}
	if fs.newBuild != nil {
		t.Error("no debía encolarse un build al superar el tope")
	}
}
