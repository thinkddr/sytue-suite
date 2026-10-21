// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package commerce

import "testing"

// Los precios y las cuotas son el contrato: si cambian sin querer, se cobra o se
// concede lo que no es. Este test los clava a los números publicados el
// 2026-09-13 (ADR 0038) para que cambiarlos sea un acto consciente.
func TestPlanes(t *testing.T) {
	casos := []struct {
		plan    Plan
		precio  int
		almacen int64
		buzon   int64
	}{
		{PlanPersonalFree, 0, 5 << 30, 1 << 30},
		{PlanPro, 500, 200 << 30, 10 << 30},
		{PlanEnterprise, 2500, 1 << 40, 25 << 30},
	}
	for _, c := range casos {
		if !ValidPlan(c.plan) {
			t.Fatalf("%s: no es válido", c.plan)
		}
		if c.plan.PriceCents() != c.precio || c.plan.StorageBytes() != c.almacen || c.plan.MailboxBytes() != c.buzon {
			t.Fatalf("%s: precio=%d almacén=%d buzón=%d", c.plan, c.plan.PriceCents(), c.plan.StorageBytes(), c.plan.MailboxBytes())
		}
	}
	if ValidPlan("suite_one") || Plan("suite_one").PriceCents() != 0 {
		t.Fatal("un plan desconocido no es válido y no cuesta nada")
	}
	if !PlanPersonalFree.Free() || PlanPro.Free() {
		t.Fatal("solo el personal gratuito es gratis")
	}
	// El descuento educativo es solo del Pro, y la pasarela lo reconoce.
	if PlanPro.UnitPriceCents(true) != 250 || PlanEnterprise.UnitPriceCents(true) != 2500 {
		t.Fatalf("descuento educativo: pro=%d enterprise=%d", PlanPro.UnitPriceCents(true), PlanEnterprise.UnitPriceCents(true))
	}
	if !ValidSeatPriceCents(250) || !ValidSeatPriceCents(500) || !ValidSeatPriceCents(2500) || ValidSeatPriceCents(600) {
		t.Fatal("los precios válidos son 250, 500 y 2500")
	}
}
