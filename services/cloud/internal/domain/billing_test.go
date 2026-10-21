// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"math"
	"testing"
)

// TestPricingTableMatchesCharged fija los precios cobrados (chargedEur de la tabla
// de mercado investigada). Un cambio accidental de tarifa rompe este test.
func TestPricingTableMatchesCharged(t *testing.T) {
	want := map[string]float64{
		"build_minutes":    0.005,
		"compute_gb_hours": 0.009,
		"storage_gb":       0.02,
	}
	if len(PricingTable) != len(want) {
		t.Fatalf("PricingTable tiene %d métricas, esperadas %d", len(PricingTable), len(want))
	}
	for m, w := range want {
		if got := PricingTable[m]; got != w {
			t.Errorf("precio de %s = %v, esperado %v", m, got, w)
		}
		if _, ok := MetricUnit[m]; !ok {
			t.Errorf("falta unidad para %s", m)
		}
	}
}

func TestValidInvoiceStatus(t *testing.T) {
	for _, s := range []string{InvoiceDraft, InvoiceIssued, InvoicePaid, InvoiceVoid} {
		if !ValidInvoiceStatus(s) {
			t.Errorf("%q debería ser válido", s)
		}
	}
	if ValidInvoiceStatus("pending") {
		t.Error("pending no debería ser válido")
	}
}

func TestValidPlanBilling(t *testing.T) {
	if !ValidPlan(PlanFree) || !ValidPlan(PlanPro) || !ValidPlan(PlanTeam) {
		t.Error("free/pro/team deberían ser válidos")
	}
	if ValidPlan(Plan("enterprise")) {
		t.Error("enterprise no debería ser válido")
	}
}

func approxEq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// TestBuildInvoiceLines_QuotaOverageRounding ejercita la matemática: cuota
// incluida restada, overage al precio unitario, base del plan y redondeo a
// céntimos.
func TestBuildInvoiceLines_QuotaOverageRounding(t *testing.T) {
	// Plan pro: base 19; cuotas 1000 build_min, 1000 gb·h, 25 GB.
	usage := map[string]float64{
		"build_minutes":    1500, // 500 de overage × 0,005 = 2,50
		"compute_gb_hours": 900,  // dentro de cuota (1000) → 0
		"storage_gb":       30,   // 5 de overage × 0,02 = 0,10
	}
	lines, subtotal, total := BuildInvoiceLines(PlanPro, usage)

	if len(lines) != 3 {
		t.Fatalf("esperadas 3 líneas, got %d", len(lines))
	}
	// Orden alfabético determinista: build_minutes, compute_gb_hours, storage_gb.
	if lines[0].Metric != "build_minutes" || !approxEq(lines[0].Quantity, 500) || !approxEq(lines[0].SubtotalEur, 2.50) {
		t.Errorf("línea build_minutes incorrecta: %+v", lines[0])
	}
	if lines[1].Metric != "compute_gb_hours" || !approxEq(lines[1].Quantity, 0) || !approxEq(lines[1].SubtotalEur, 0) {
		t.Errorf("línea compute_gb_hours incorrecta: %+v", lines[1])
	}
	if lines[2].Metric != "storage_gb" || !approxEq(lines[2].Quantity, 5) || !approxEq(lines[2].SubtotalEur, 0.10) {
		t.Errorf("línea storage_gb incorrecta: %+v", lines[2])
	}
	if !approxEq(subtotal, 2.60) {
		t.Errorf("subtotal = %v, esperado 2,60", subtotal)
	}
	if !approxEq(total, 21.60) { // 2,60 + 19 base
		t.Errorf("total = %v, esperado 21,60", total)
	}
}

// TestBuildInvoiceLines_FreePlanWithinQuota: uso bajo la cuota free → 0 overage,
// 0 base.
func TestBuildInvoiceLines_FreePlanWithinQuota(t *testing.T) {
	usage := map[string]float64{"build_minutes": 50, "storage_gb": 0.5}
	lines, subtotal, total := BuildInvoiceLines(PlanFree, usage)
	if len(lines) != 2 {
		t.Fatalf("esperadas 2 líneas, got %d", len(lines))
	}
	if !approxEq(subtotal, 0) || !approxEq(total, 0) {
		t.Errorf("free bajo cuota: subtotal=%v total=%v, esperados 0", subtotal, total)
	}
}

// TestBuildInvoiceLines_UnknownMetricIgnored: una métrica sin tarifa (p. ej.
// egress diferido) no rompe la factura ni añade línea.
func TestBuildInvoiceLines_UnknownMetricIgnored(t *testing.T) {
	usage := map[string]float64{"egress_gb": 9999, "build_minutes": 200}
	lines, _, total := BuildInvoiceLines(PlanFree, usage)
	if len(lines) != 1 || lines[0].Metric != "build_minutes" {
		t.Fatalf("esperada 1 línea build_minutes, got %+v", lines)
	}
	// 200 - 100 cuota = 100 × 0,005 = 0,50, base free 0.
	if !approxEq(total, 0.50) {
		t.Errorf("total = %v, esperado 0,50", total)
	}
}

func TestRoundCents(t *testing.T) {
	if got := RoundCents(2.5049); !approxEq(got, 2.50) {
		t.Errorf("RoundCents(2.5049) = %v, esperado 2,50", got)
	}
	if got := RoundCents(2.505); !approxEq(got, 2.51) {
		t.Errorf("RoundCents(2.505) = %v, esperado 2,51", got)
	}
}

// ---------------------------------------------------------------------------
// Suscripción a la suite: requisito de entrada de Sytue Cloud
// ---------------------------------------------------------------------------

func TestMeetsSuiteRequirement(t *testing.T) {
	// Free NO es gratis a secas: exige al menos un puesto de la suite.
	if MeetsSuiteRequirement(PlanFree, 0) {
		t.Error("Free sin puestos de suite no debe cumplir el requisito")
	}
	if !MeetsSuiteRequirement(PlanFree, 1) {
		t.Error("Free con un puesto debe cumplir")
	}
	// Hoy la regla fijada solo afecta a Free; Pro/Team no lo exigen.
	for _, p := range []Plan{PlanPro, PlanTeam} {
		if !MeetsSuiteRequirement(p, 0) {
			t.Errorf("%s no exige puestos de suite hoy", p)
		}
	}
	// Un plan desconocido cae a Free (PlanSpecFor), así que hereda su requisito.
	if MeetsSuiteRequirement(Plan("inventado"), 0) {
		t.Error("un plan desconocido debe heredar el requisito de Free")
	}
}

func TestFacturaCloudNoDuplicaLosPuestosDeSuite(t *testing.T) {
	lines, subtotal, total := BuildInvoiceLines(PlanFree, map[string]float64{
		"suite_seats": 3,
	})
	if len(lines) != 0 || subtotal != 0 || total != 0 {
		t.Errorf("Cloud no debe volver a cobrar Sytue One: lines=%v subtotal=%v total=%v", lines, subtotal, total)
	}
}
