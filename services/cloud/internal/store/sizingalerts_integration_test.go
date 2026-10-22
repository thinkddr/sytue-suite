// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de los AVISOS PROACTIVOS DE TAMAÑO contra un PostgreSQL
// real.
//
// Lo que se prueba aquí no es lógica de Go: son las dos garantías que solo la
// base de datos puede dar y de las que depende que la función no se convierta en
// ruido —que es la única forma de que un aviso deje de servir para nada—:
//
//  1. Con N réplicas del evaluador corriendo a la vez, avisa UNA.
//  2. Repetir la misma recomendación no vuelve a avisar, y una recomendación
//     nueva espera al enfriamiento.
package store_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

func aviso(from, to string) domain.SizingAlertPayload {
	return domain.SizingAlertPayload{
		Action: domain.SizingDown, Reason: domain.ReasonOverProvisioned,
		FromSize: from, ToSize: to, MonthlyDeltaEur: -3.5, P95MemoryPercent: 12,
	}
}

func TestSizingAlertDedupeIntegration(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()

	acme := tenantCtx("acme")
	p := seedProject(t, acme, st, "tienda")
	const sinEnfriamiento = time.Duration(0)

	t.Run("el primer aviso se emite", func(t *testing.T) {
		sent, err := st.PublishSizingAlert(ctx, "acme", p.ID, sinEnfriamiento, aviso("standard", "basic"))
		if err != nil {
			t.Fatalf("PublishSizingAlert: %v", err)
		}
		if !sent {
			t.Fatal("el primer aviso del proyecto debe emitirse")
		}
	})

	// Insistir es la forma de que el cliente silencie el canal: mientras el consejo
	// no cambie, no se vuelve a emitir nada. Quien no lo aplicó ya lo tiene delante
	// en el panel del proyecto.
	t.Run("la misma recomendación no vuelve a avisar", func(t *testing.T) {
		sent, err := st.PublishSizingAlert(ctx, "acme", p.ID, sinEnfriamiento, aviso("standard", "basic"))
		if err != nil {
			t.Fatalf("PublishSizingAlert (2.ª): %v", err)
		}
		if sent {
			t.Error("repetir el mismo consejo no puede emitir un segundo aviso")
		}
	})

	// Un consejo NUEVO sí es noticia… pero no cada hora: el enfriamiento es lo que
	// impide que una recomendación que oscila (down → up → down) avise en cada
	// ciclo del evaluador.
	t.Run("una recomendación distinta espera al enfriamiento", func(t *testing.T) {
		sent, err := st.PublishSizingAlert(ctx, "acme", p.ID, 24*time.Hour, aviso("standard", "pro"))
		if err != nil {
			t.Fatalf("PublishSizingAlert (enfriamiento): %v", err)
		}
		if sent {
			t.Error("un aviso recién emitido no puede repetirse dentro del enfriamiento")
		}
	})

	t.Run("pasado el enfriamiento, el consejo nuevo sí avisa", func(t *testing.T) {
		sent, err := st.PublishSizingAlert(ctx, "acme", p.ID, sinEnfriamiento, aviso("standard", "pro"))
		if err != nil {
			t.Fatalf("PublishSizingAlert (consejo nuevo): %v", err)
		}
		if !sent {
			t.Error("cambiar la recomendación debe volver a avisar")
		}
	})

	t.Run("cada proyecto tiene su propio aviso", func(t *testing.T) {
		otra := tenantCtx("otra")
		po := seedProject(t, otra, st, "blog")
		sent, err := st.PublishSizingAlert(ctx, "otra", po.ID, sinEnfriamiento, aviso("standard", "basic"))
		if err != nil {
			t.Fatalf("PublishSizingAlert(otro proyecto): %v", err)
		}
		if !sent {
			t.Error("el aviso de un proyecto no puede consumir el de otro")
		}
	})

	// EL CASO QUE IMPORTA: N réplicas del evaluador a la vez. Sin la atomicidad del
	// INSERT ... ON CONFLICT, varias verían «aún no avisado» y el cliente recibiría
	// un webhook por réplica.
	t.Run("con 8 réplicas simultáneas solo una emite", func(t *testing.T) {
		pc := seedProject(t, acme, st, "carrera")
		const replicas = 8
		var wg sync.WaitGroup
		results := make([]bool, replicas)
		errs := make([]error, replicas)
		start := make(chan struct{})
		for i := range replicas {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start // soltar todas a la vez, para que compitan de verdad
				results[i], errs[i] = st.PublishSizingAlert(
					ctx, "acme", pc.ID, sinEnfriamiento, aviso("standard", "basic"),
				)
			}(i)
		}
		close(start)
		wg.Wait()

		sent := 0
		for i := range replicas {
			if errs[i] != nil {
				t.Fatalf("réplica %d: %v", i, errs[i])
			}
			if results[i] {
				sent++
			}
		}
		if sent != 1 {
			t.Errorf("emitieron %d réplicas de %d; debe emitir exactamente UNA", sent, replicas)
		}
	})

	// Reserva y publicación comparten transacción: si no fuera así quedaría un
	// aviso «reservado y nunca enviado», que se come el aviso entero sin que nada
	// falle.
	t.Run("cada aviso emitido deja su evento en el outbox", func(t *testing.T) {
		eventos := maintCount(t, h.Pool, `SELECT count(*) FROM events_outbox WHERE type=$1`, store.EventSizingAdvice)
		// 4 emisiones: tienda (×2, la segunda tras cambiar el consejo), blog y carrera.
		if eventos != 4 {
			t.Errorf("eventos en el outbox = %d; se esperaban 4 (uno por aviso emitido)", eventos)
		}
	})
}

// TestListSizingCandidatesIntegration comprueba el prefiltro: descarta en SQL los
// proyectos sobre los que el motor no diría nada, que es lo que hace que el
// barrido sea barato con decenas de miles de organizaciones.
func TestListSizingCandidatesIntegration(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()

	acme := tenantCtx("acme")
	p := seedProject(t, acme, st, "tienda")
	base := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	desde := base.Add(-time.Hour)

	muestrear := func(pid uuid.UUID, tenant string, n int) {
		t.Helper()
		in := make([]store.ResourceSampleInput, 0, n)
		for i := range n {
			in = append(in, store.ResourceSampleInput{
				Tenant: tenant, ProjectID: pid,
				At:          base.Add(time.Duration(i) * store.SampleBucket),
				CPUMilli:    50,
				MemoryBytes: 100 << 20,
			})
		}
		if err := st.RecordResourceSamples(ctx, in); err != nil {
			t.Fatalf("muestrear: %v", err)
		}
	}

	// Con menos muestras de las que el motor exige no hay consejo posible: cargar
	// su histórico para acabar callando es exactamente el trabajo que este filtro
	// evita.
	muestrear(p.ID, "acme", domain.MinSamplesForAdvice-1)
	got, err := st.ListSizingCandidates(ctx, desde, 100)
	if err != nil {
		t.Fatalf("ListSizingCandidates: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("con pocas muestras: %d candidatos, se esperaban 0", len(got))
	}

	muestrear(p.ID, "acme", domain.MinSamplesForAdvice+5)
	got, err = st.ListSizingCandidates(ctx, desde, 100)
	if err != nil {
		t.Fatalf("ListSizingCandidates (suficientes): %v", err)
	}
	if len(got) != 1 || got[0].ProjectID != p.ID || got[0].Tenant != "acme" {
		t.Fatalf("candidatos = %+v; se esperaba solo tienda/acme", got)
	}

	// El barrido es cross-tenant a propósito (política de mantenimiento): un
	// evaluador por inquilino no escalaría.
	otra := tenantCtx("otra")
	po := seedProject(t, otra, st, "blog")
	muestrear(po.ID, "otra", domain.MinSamplesForAdvice+1)
	got, err = st.ListSizingCandidates(ctx, desde, 100)
	if err != nil {
		t.Fatalf("ListSizingCandidates (dos inquilinos): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("candidatos = %d; se esperaban 2 (uno por proyecto)", len(got))
	}

	// Las muestras viejas no cuentan: la ventana la fija el llamante.
	if got, err := st.ListSizingCandidates(ctx, base.Add(30*24*time.Hour), 100); err != nil || len(got) != 0 {
		t.Errorf("con la ventana en el futuro: %d candidatos, err=%v", len(got), err)
	}
}
