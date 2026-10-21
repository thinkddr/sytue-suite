// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Integración del store de facturación (0015) contra un PostgreSQL real: la
// idempotencia de CreateInvoice sobre UNIQUE(tenant, period_from, period_to) y el
// upsert de billing_accounts. Detrás del build-tag `integration` (sin Docker no
// corre en CI por defecto).
package store_test

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

func TestBillingAccountUpsertAndInvoiceIdempotent(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	// GetOrCreate es idempotente: dos llamadas devuelven la misma cuenta 'free'.
	a1, err := st.GetOrCreateBillingAccount(ctx)
	if err != nil {
		t.Fatalf("GetOrCreateBillingAccount: %v", err)
	}
	if a1.Plan != domain.PlanFree || a1.SpendLimitEur != nil {
		t.Fatalf("cuenta inicial inesperada: %+v", a1)
	}
	limit := 42.0
	if _, err := st.SetSpendLimit(ctx, &limit); err != nil {
		t.Fatalf("SetSpendLimit: %v", err)
	}
	a2, _ := st.GetOrCreateBillingAccount(ctx)
	if a2.SpendLimitEur == nil || *a2.SpendLimitEur != 42 {
		t.Fatalf("tope no persistido: %+v", a2)
	}

	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	now := time.Now().UTC()
	inv := domain.Invoice{
		PeriodFrom: from, PeriodTo: to, Status: domain.InvoiceIssued,
		SubtotalEur: 2.50, TotalEur: 2.50, IssuedAt: &now,
		Lines: []domain.InvoiceLine{{Metric: "build_minutes", Quantity: 500, UnitPriceEur: 0.005, SubtotalEur: 2.50}},
	}
	got1, created1, err := st.CreateInvoice(ctx, inv)
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if !created1 {
		t.Fatalf("la 1ª generación debe reportar created=true")
	}
	// Re-ejecutar el MISMO periodo devuelve la MISMA factura (idempotencia), aunque
	// los importes de entrada cambien: no se duplica ni se sobreescribe. created=false
	// → el llamador no genera una segunda factura ni repite efectos.
	inv.TotalEur = 999
	got2, created2, err := st.CreateInvoice(ctx, inv)
	if err != nil {
		t.Fatalf("CreateInvoice (2ª): %v", err)
	}
	if created2 {
		t.Fatalf("el re-disparo del mismo periodo debe reportar created=false")
	}
	if got1.ID != got2.ID {
		t.Fatalf("idempotencia rota: %v != %v", got1.ID, got2.ID)
	}
	if got2.TotalEur != 2.50 {
		t.Errorf("la 2ª generación no debe sobrescribir el total: %v", got2.TotalEur)
	}
	if len(got1.Lines) != 1 || got1.Lines[0].Metric != "build_minutes" {
		t.Errorf("líneas jsonb mal persistidas: %+v", got1.Lines)
	}

	invs, err := st.ListInvoices(ctx, time.Time{}, got1.ID, 10)
	if err != nil {
		t.Fatalf("ListInvoices: %v", err)
	}
	if len(invs) != 1 {
		t.Fatalf("esperada 1 factura, got %d", len(invs))
	}
}
