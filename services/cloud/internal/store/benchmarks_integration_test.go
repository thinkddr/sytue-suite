// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// TestBenchmarksIntegration cubre lo que un doble no puede probar: que la tabla
// de benchmarks hace cumplir el k-anonimato ELLA MISMA y que se lee sin ver una
// sola fila cruda de otro inquilino.
func TestBenchmarksIntegration(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()

	clave := domain.CohortKey{Framework: "go", Services: "postgres", Size: "standard"}

	// --- El CHECK de la tabla es la última barrera ---------------------------
	// Si un cambio en el código dejara de aplicar el umbral, esto tiene que
	// fallar. Es el único motivo por el que el 5 está escrito dos veces.
	err := st.PublishCohorts(ctx, []domain.Cohort{{
		Key: clave, P50MemoryPercent: 10, P90MemoryPercent: 20,
		Tenants: domain.MinCohortTenants - 1, Projects: 10,
	}})
	if err == nil {
		t.Fatal("FUGA: la base de datos aceptó publicar una cohorte por debajo del umbral")
	}
	if !strings.Contains(err.Error(), "sizing_benchmarks") {
		t.Errorf("el error debería señalar la restricción de la tabla: %v", err)
	}

	// --- Publicación normal --------------------------------------------------
	if err := st.PublishCohorts(ctx, []domain.Cohort{{
		Key: clave, P50MemoryPercent: 12, P90MemoryPercent: 34,
		Tenants: 8, Projects: 19,
	}}); err != nil {
		t.Fatalf("publicar: %v", err)
	}

	// Cualquier inquilino la lee: ya es anónima, no hay nada que segmentar.
	for _, tenant := range []string{"acme", "otra"} {
		got, found, err := st.GetCohort(tenantCtx(tenant), clave)
		if err != nil {
			t.Fatalf("leer como %s: %v", tenant, err)
		}
		if !found || got.P90MemoryPercent != 34 || got.Tenants != 8 {
			t.Fatalf("como %s: found=%v cohorte=%+v", tenant, found, got)
		}
	}

	// Una cohorte que no existe no es un error: es lo normal al principio.
	if _, found, err := st.GetCohort(tenantCtx("acme"), domain.CohortKey{
		Framework: "docker", Services: "", Size: "pro-xl",
	}); err != nil || found {
		t.Errorf("cohorte inexistente: found=%v err=%v", found, err)
	}

	// --- Reemplazo COMPLETO: la cohorte que encoge DESAPARECE ----------------
	// Con upsert seguiría publicada para siempre con las cifras del día que sí
	// llegó al umbral, aunque hoy la formen dos empresas.
	if err := st.PublishCohorts(ctx, nil); err != nil {
		t.Fatalf("republicar vacío: %v", err)
	}
	if _, found, err := st.GetCohort(tenantCtx("acme"), clave); err != nil || found {
		t.Errorf("la cohorte retirada sigue publicada: found=%v err=%v", found, err)
	}
}

// TestCollectCohortUsageIntegration comprueba la mitad de lectura: que agrupa por
// proyecto, exige muestras suficientes y NO deja escapar cifras absolutas.
func TestCollectCohortUsageIntegration(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()

	acme := tenantCtx("acme")
	p := seedProject(t, acme, st, "tienda")

	base := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	muestrear := func(pid uuid.UUID, tenant string, n int, mem int64) {
		t.Helper()
		in := make([]store.ResourceSampleInput, 0, n)
		for i := range n {
			in = append(in, store.ResourceSampleInput{
				Tenant: tenant, ProjectID: pid,
				At:          base.Add(time.Duration(i) * store.SampleBucket),
				CPUMilli:    50,
				MemoryBytes: mem,
			})
		}
		if err := st.RecordResourceSamples(ctx, in); err != nil {
			t.Fatalf("muestrear: %v", err)
		}
	}

	// Por debajo del mínimo el proyecto no cuenta: su p95 no significa nada.
	muestrear(p.ID, "acme", domain.MinCohortSamples-1, 100<<20)
	if got, err := st.CollectCohortUsage(ctx, base.Add(-time.Hour)); err != nil || len(got) != 0 {
		t.Fatalf("con pocas muestras: %d filas, err=%v", len(got), err)
	}

	// Con las suficientes, sí — y llega como PORCENTAJE del límite del tamaño.
	muestrear(p.ID, "acme", domain.MinCohortSamples+10, 100<<20)
	got, err := st.CollectCohortUsage(ctx, base.Add(-time.Hour))
	if err != nil {
		t.Fatalf("recopilar: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("filas = %d; se esperaba 1", len(got))
	}
	u := got[0]
	if u.Tenant != "acme" {
		t.Errorf("inquilino = %q", u.Tenant)
	}
	// El proyecto sembrado no elige tamaño: cae en el por defecto, resuelto para
	// que todos los que están en basic formen UNA cohorte y no dos.
	if u.Cohort.Size != domain.DefaultInstanceSizeName {
		t.Errorf("tamaño = %q; se esperaba el por defecto", u.Cohort.Size)
	}
	// 100 MiB sobre los 512 MiB de basic ≈ 20 %.
	if u.P95MemoryPercent < 18 || u.P95MemoryPercent > 22 {
		t.Errorf("porcentaje = %d; se esperaba ~20", u.P95MemoryPercent)
	}

	// Y la clave de servicios sale de los servicios vivos del proyecto.
	if u.Cohort.Services != "" {
		t.Errorf("sin servicios la clave debe ir vacía, llegó %q", u.Cohort.Services)
	}

	// Un segundo inquilino entra en la misma llamada: es cross-tenant a propósito
	// (va bajo la política de mantenimiento).
	otra := tenantCtx("otra")
	po := seedProject(t, otra, st, "blog")
	muestrear(po.ID, "otra", domain.MinCohortSamples+1, 300<<20)
	got, err = st.CollectCohortUsage(ctx, base.Add(-time.Hour))
	if err != nil {
		t.Fatalf("recopilar con dos inquilinos: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("filas = %d; se esperaban 2 (una por proyecto)", len(got))
	}
	vistos := map[string]bool{}
	for _, x := range got {
		vistos[x.Tenant] = true
	}
	if !vistos["acme"] || !vistos["otra"] {
		t.Errorf("faltan inquilinos: %v", vistos)
	}

	// El agregado de esos dos NO se publica: son dos empresas, no cinco.
	if c := domain.BuildCohorts(got); len(c) != 0 {
		t.Errorf("se agruparon %d cohortes con 2 empresas: %+v", len(c), c)
	}

	// Sanity: con MinCohortTenants inquilinos distintos sí saldría, y es lo que
	// justifica que el umbral no sea inalcanzable en la práctica.
	sinteticos := append([]domain.ProjectUsage(nil), got...)
	for i := range domain.MinCohortTenants {
		sinteticos = append(sinteticos, domain.ProjectUsage{
			Tenant:           fmt.Sprintf("relleno-%d", i),
			Cohort:           got[0].Cohort,
			P95MemoryPercent: 20,
		})
	}
	if c := domain.BuildCohorts(sinteticos); len(c) != 1 {
		t.Errorf("cohortes = %d; se esperaba 1", len(c))
	}
}
