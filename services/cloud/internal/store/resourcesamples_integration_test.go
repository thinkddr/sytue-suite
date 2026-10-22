// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// TestMuestrasDeUsoIntegration cubre las dos propiedades de resource_samples que
// no se pueden probar con un doble: que la RLS aísla de verdad entre inquilinos y
// que la escritura conserva el PICO del intervalo.
func TestMuestrasDeUsoIntegration(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)

	acme := tenantCtx("acme")
	otra := tenantCtx("otra")
	pa := seedProject(t, acme, st, "tienda")
	po := seedProject(t, otra, st, "blog")

	base := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	escribir := func(tenant string, pid uuid.UUID, at time.Time, cpu int, mem int64) {
		t.Helper()
		err := st.RecordResourceSamples(context.Background(), []store.ResourceSampleInput{{
			Tenant: tenant, ProjectID: pid, At: at, CPUMilli: cpu, MemoryBytes: mem,
		}})
		if err != nil {
			t.Fatalf("registrar muestras (%s): %v", tenant, err)
		}
	}

	escribir("acme", pa.ID, base, 100, 200<<20)
	escribir("otra", po.ID, base, 999, 900<<20)

	// --- Aislamiento: cada inquilino ve SOLO lo suyo -------------------------
	mias, err := st.ListResourceSamples(acme, pa.ID, base.Add(-time.Hour))
	if err != nil {
		t.Fatalf("listar como acme: %v", err)
	}
	if len(mias) != 1 || mias[0].CPUMilli != 100 {
		t.Fatalf("acme ve %d muestras propias; se esperaba 1 con cpu 100", len(mias))
	}
	ajenas, err := st.ListResourceSamples(acme, po.ID, base.Add(-time.Hour))
	if err != nil {
		t.Fatalf("listar el proyecto ajeno como acme: %v", err)
	}
	if len(ajenas) != 0 {
		t.Fatalf("FUGA: acme lee %d muestras del proyecto de otra organización", len(ajenas))
	}

	// --- El pico manda: re-escribir el MISMO cubo no baja el máximo ----------
	// Es de lo que depende no proponer una bajada que acabe en OOMKill: si el
	// conflicto se resolviera con «gana el último», el pico se perdería en
	// silencio y el motor vería una app plana que en realidad tiene ráfagas.
	escribir("acme", pa.ID, base.Add(3*time.Minute), 40, 800<<20) // mismo cubo de 15 min
	escribir("acme", pa.ID, base.Add(7*time.Minute), 10, 50<<20)  // y otra vez, más baja

	mias, err = st.ListResourceSamples(acme, pa.ID, base.Add(-time.Hour))
	if err != nil {
		t.Fatalf("listar tras el pico: %v", err)
	}
	if len(mias) != 1 {
		t.Fatalf("filas = %d; las tres escrituras caen en el MISMO cubo de 15 min", len(mias))
	}
	if want := int64(800 << 20); mias[0].MemoryBytes != want {
		t.Errorf("memoria = %d; se esperaba el pico %d", mias[0].MemoryBytes, want)
	}
	if mias[0].CPUMilli != 100 {
		t.Errorf("cpu = %d; se esperaba el pico 100", mias[0].CPUMilli)
	}

	// --- Purga ---------------------------------------------------------------
	escribir("acme", pa.ID, base.Add(-48*time.Hour), 5, 5<<20)
	n, err := st.PurgeResourceSamples(context.Background(), base.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("purgar: %v", err)
	}
	if n != 1 {
		t.Errorf("purgadas %d; se esperaba 1 (solo la vieja)", n)
	}
}
