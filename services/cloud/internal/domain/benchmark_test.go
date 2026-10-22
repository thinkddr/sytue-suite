// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain_test

import (
	"fmt"
	"testing"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

func uso(tenant, framework, services, size string, pct int) domain.ProjectUsage {
	return domain.ProjectUsage{
		Tenant:           tenant,
		Cohort:           domain.CohortKey{Framework: framework, Services: services, Size: size},
		P95MemoryPercent: pct,
	}
}

// TestCohorteBajoElUmbralNoSePublica es LA prueba de este fichero: agregar entre
// inquilinos es lo que la RLS existe para impedir, así que una cohorte que no
// reúne bastantes empresas distintas no puede salir por ninguna parte.
func TestCohorteBajoElUmbralNoSePublica(t *testing.T) {
	us := make([]domain.ProjectUsage, 0, domain.MinCohortTenants)
	for i := range domain.MinCohortTenants - 1 {
		us = append(us, uso(fmt.Sprintf("t%d", i), "go", "postgres", "standard", 20))
	}
	if got := domain.BuildCohorts(us); len(got) != 0 {
		t.Fatalf("con %d inquilinos se publicó una cohorte: %+v", domain.MinCohortTenants-1, got)
	}

	// Y con uno más, sí.
	us = append(us, uso("tN", "go", "postgres", "standard", 20))
	if got := domain.BuildCohorts(us); len(got) != 1 {
		t.Fatalf("con el umbral justo debería publicarse 1 cohorte, salieron %d", len(got))
	}
}

// TestElUmbralCuentaEMPRESAS_NoPROYECTOS: diez proyectos de la misma empresa
// siguen siendo una empresa. Contar proyectos convertiría el umbral en un adorno
// —a un solo cliente le bastaría con crear cinco proyectos para «anonimizarse» a
// sí mismo—.
func TestElUmbralCuentaEMPRESAS_NoPROYECTOS(t *testing.T) {
	us := make([]domain.ProjectUsage, 0, 20)
	for i := range 20 {
		us = append(us, uso("acme", "go", "postgres", "standard", 10+i))
	}
	if got := domain.BuildCohorts(us); len(got) != 0 {
		t.Fatalf("20 proyectos de UN inquilino no son una cohorte: %+v", got)
	}
}

// TestPercentilesDeLaCohorte comprueba la cifra que se publica.
func TestPercentilesDeLaCohorte(t *testing.T) {
	pcts := []int{5, 10, 12, 15, 90}
	us := make([]domain.ProjectUsage, 0, len(pcts))
	for i, p := range pcts {
		us = append(us, uso(fmt.Sprintf("t%d", i), "astro", "", "standard", p))
	}
	got := domain.BuildCohorts(us)
	if len(got) != 1 {
		t.Fatalf("cohortes = %d", len(got))
	}
	c := got[0]
	if c.Tenants != 5 || c.Projects != 5 {
		t.Errorf("auditoría mal: %d inquilinos / %d proyectos", c.Tenants, c.Projects)
	}
	// La mediana es 12; el p90 se lleva el pico de 90, que es lo que evita
	// recomendar un tamaño donde una de cada diez apps no cabría.
	if c.P50MemoryPercent != 12 {
		t.Errorf("p50 = %d; se esperaba 12", c.P50MemoryPercent)
	}
	if c.P90MemoryPercent != 90 {
		t.Errorf("p90 = %d; se esperaba 90", c.P90MemoryPercent)
	}
}

// TestLasCohortesNoSeMezclan: framework, servicios y tamaño son la clave, y cada
// combinación se cuenta por separado. Si se mezclaran, cinco inquilinos repartidos
// entre dos cohortes distintas publicarían las dos sin llegar al umbral en ninguna.
func TestLasCohortesNoSeMezclan(t *testing.T) {
	us := make([]domain.ProjectUsage, 0, 2*domain.MinCohortTenants)
	for i := range domain.MinCohortTenants {
		us = append(us,
			uso(fmt.Sprintf("t%d", i), "go", "postgres", "standard", 20),
			uso(fmt.Sprintf("t%d", i), "astro", "postgres", "standard", 20))
	}
	got := domain.BuildCohorts(us)
	if len(got) != 2 {
		t.Fatalf("cohortes = %d; se esperaban 2 (go y astro por separado)", len(got))
	}
	if got[0].Key.Framework != "astro" || got[1].Key.Framework != "go" {
		t.Errorf("el orden tiene que ser estable: %+v", got)
	}
}

// TestServicesKeyEsEstable: el orden en que el cliente creó los servicios no
// puede partir una cohorte en dos.
func TestServicesKeyEsEstable(t *testing.T) {
	a := domain.ServicesKey([]domain.ServiceKind{domain.ServiceStorage, domain.ServicePostgres})
	b := domain.ServicesKey([]domain.ServiceKind{domain.ServicePostgres, domain.ServiceStorage})
	if a != b {
		t.Errorf("mismo conjunto, claves distintas: %q vs %q", a, b)
	}
	if a != "postgres+storage" {
		t.Errorf("clave = %q", a)
	}
	// Duplicados y vacíos no cuentan.
	if got := domain.ServicesKey([]domain.ServiceKind{domain.ServicePostgres, domain.ServicePostgres}); got != "postgres" {
		t.Errorf("con duplicados: %q", got)
	}
	if got := domain.ServicesKey(nil); got != "" {
		t.Errorf("sin servicios: %q", got)
	}
}

// --- ApplyCohort: rellenar el silencio de los primeros días ----------------

func cohorte(size string, p90 int, tenants ...int) domain.Cohort {
	n := 12
	if len(tenants) > 0 {
		n = tenants[0]
	}
	return domain.Cohort{
		Key:              domain.CohortKey{Framework: "go", Size: size},
		P50MemoryPercent: p90 / 2,
		P90MemoryPercent: p90,
		Tenants:          n,
		Projects:         n * 2,
	}
}

// sinDatos es lo que devuelve el motor para un proyecto recién creado.
func sinDatos(size string) domain.SizingAdvice {
	return domain.RecommendInstanceSize(size, 1, nil)
}

func TestLaCohorteRellenaElHuecoDelProyectoNuevo(t *testing.T) {
	// `standard` es 1 GiB; el p90 de la cohorte es el 20 % → 205 MiB, que cabe de
	// sobra en `basic` (512 MiB).
	got := domain.ApplyCohort(sinDatos("standard"), 1, cohorte("standard", 20))
	if got.Action != domain.SizingDown || got.Reason != domain.ReasonCohortOverProvisioned {
		t.Fatalf("acción/razón = %s/%s", got.Action, got.Reason)
	}
	if got.To != "basic" {
		t.Errorf("destino = %q; se esperaba basic", got.To)
	}
	if got.MonthlyDeltaEur >= 0 {
		t.Errorf("bajar tiene que ahorrar; delta = %v", got.MonthlyDeltaEur)
	}
	if got.Benchmark == nil || got.Benchmark.Tenants != 12 {
		t.Errorf("falta la auditoría de la cohorte: %+v", got.Benchmark)
	}
}

// TestElHistoricoPropioGanaALaCohorte: si el motor ya decidió con datos del
// proyecto, la cohorte no puede pisarlo. Los datos de terceros son un relleno,
// nunca una corrección.
func TestElHistoricoPropioGanaALaCohorte(t *testing.T) {
	medido := domain.SizingAdvice{
		Action: domain.SizingKeep, Reason: domain.ReasonPeakBlocksDowngrade,
		From: "standard", To: "standard",
	}
	got := domain.ApplyCohort(medido, 1, cohorte("standard", 5))
	if got.Action != domain.SizingKeep || got.Reason != domain.ReasonPeakBlocksDowngrade {
		t.Fatalf("la cohorte pisó una decisión medida: %s/%s", got.Action, got.Reason)
	}
	if got.Benchmark != nil {
		t.Errorf("tampoco debe adornarla con cifras de otros: %+v", got.Benchmark)
	}
}

// TestConLaCohorteApretadaSoloSeInforma: si las apps comparables van justas, no
// se propone nada —ni bajar, que no cabría, ni subir, que sería cobrarle más a
// alguien de quien no sabemos nada—.
func TestConLaCohorteApretadaSoloSeInforma(t *testing.T) {
	got := domain.ApplyCohort(sinDatos("standard"), 1, cohorte("standard", 98))
	if got.Action != domain.SizingKeep {
		t.Fatalf("acción = %s; con la cohorte al 98 %% solo se informa", got.Action)
	}
	if got.Benchmark == nil {
		t.Error("informar sí: la cifra se enseña aunque no haya acción")
	}
}

// TestUnaCohorteBajoElUmbralNoLlegaAlConsejo es la última barrera: aunque algo
// consiguiera leer una cohorte de 2 empresas, no se muestra.
func TestUnaCohorteBajoElUmbralNoLlegaAlConsejo(t *testing.T) {
	got := domain.ApplyCohort(sinDatos("standard"), 1, cohorte("standard", 20, 2))
	if got.Benchmark != nil || got.Action != domain.SizingKeep {
		t.Fatalf("se usó una cohorte de 2 empresas: %+v", got)
	}
}

// TestBajaUnEscalonYNoLosQueHaganFalta: con cero datos propios, saltar dos
// tamaños de un clic es una apuesta.
func TestBajaUnEscalonYNoLosQueHaganFalta(t *testing.T) {
	// `pro` es 2 GiB y la cohorte usa el 5 % (102 MiB): cabría en basic, pero solo
	// se propone standard.
	got := domain.ApplyCohort(sinDatos("pro"), 1, cohorte("pro", 5))
	if got.To != "standard" {
		t.Errorf("destino = %q; se esperaba un solo escalón (standard)", got.To)
	}
}

// TestEnElMasPequenoSoloSeInforma: desde el tamaño más pequeño no hay a dónde
// bajar, pero la cifra de la cohorte se sigue enseñando.
func TestEnElMasPequenoSoloSeInforma(t *testing.T) {
	got := domain.ApplyCohort(sinDatos("basic"), 1, cohorte("basic", 10))
	if got.Action != domain.SizingKeep {
		t.Errorf("acción = %s; desde basic no hay a dónde bajar", got.Action)
	}
	if got.Benchmark == nil {
		t.Error("la cifra de la cohorte se sigue enseñando")
	}
}

// TestSinTamanoNoSeAgrupa: un proyecto sin tamaño no es comparable, y antes que
// inventarle una cohorte se ignora.
func TestSinTamanoNoSeAgrupa(t *testing.T) {
	us := make([]domain.ProjectUsage, 0, domain.MinCohortTenants)
	for i := range domain.MinCohortTenants {
		us = append(us, uso(fmt.Sprintf("t%d", i), "go", "", "", 20))
	}
	if got := domain.BuildCohorts(us); len(got) != 0 {
		t.Fatalf("se agrupó sin tamaño: %+v", got)
	}
}
