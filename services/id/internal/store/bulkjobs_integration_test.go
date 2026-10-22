// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Trabajos masivos visibles y cancelables (mig 0093), contra PostgreSQL real.
package store_test

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// TestTrabajoMasivoSeVeYSePara vigila las tres cosas por las que esto existe:
// que el progreso se vea mientras corre, que pedir la cancelación PARE el bucle
// en el siguiente elemento, y que un trabajo cuya petición murió no se quede
// diciendo que sigue corriendo.
func TestTrabajoMasivoSeVeYSePara(t *testing.T) {
	st, ctx := newStore(t)
	org, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	jefa := seedUser(t, ctx, st, "jefa@acme.test")

	id, err := st.CrearTrabajo(ctx, org.ID, "suspend", jefa.ID, 5)
	if err != nil {
		t.Fatalf("CrearTrabajo: %v", err)
	}

	// Dos elementos: se ve el progreso y NADIE ha pedido parar.
	for i := 1; i <= 2; i++ {
		parar, err := st.AvanzarTrabajo(ctx, id, []string{"ok"})
		if err != nil || parar {
			t.Fatalf("AvanzarTrabajo(%d): %v parar=%v", i, err, parar)
		}
	}
	tr, err := st.Trabajo(ctx, org.ID, id)
	if err != nil || tr.Hechos != 2 || tr.Total != 5 {
		t.Fatalf("progreso a mitad: %+v %v", tr, err)
	}
	if v := tr.EstadoVisible(time.Now()); v != "ejecutando" {
		t.Fatalf("estado visible = %q, quiero ejecutando", v)
	}

	// Otra petición pide parar. El siguiente avance lo obedece.
	pedida, err := st.PedirCancelacionTrabajo(ctx, org.ID, id)
	if err != nil || !pedida {
		t.Fatalf("PedirCancelacionTrabajo: %v pedida=%v", err, pedida)
	}
	parar, err := st.AvanzarTrabajo(ctx, id, []string{"ok", "ok", "ok"})
	if err != nil || !parar {
		t.Fatalf("tras pedir la cancelación el bucle tiene que parar: %v parar=%v", err, parar)
	}
	if err := st.CerrarTrabajo(ctx, id, "cancelado", []string{"ok", "ok", "ok"}); err != nil {
		t.Fatalf("CerrarTrabajo: %v", err)
	}
	tr, err = st.Trabajo(ctx, org.ID, id)
	if err != nil || tr.Estado != "cancelado" || tr.Hechos != 3 {
		t.Fatalf("tras cancelar: %+v %v, quiero cancelado con 3 de 5", tr, err)
	}
	// Y ya no hay nada que parar: cancelar lo terminado no inventa un éxito.
	if pedida, _ := st.PedirCancelacionTrabajo(ctx, org.ID, id); pedida {
		t.Error("cancelar un trabajo ya cerrado no puede decir que sí")
	}

	// Un trabajo cuya petición murió con el pod se queda en 'ejecutando' para
	// siempre. Se REPORTA como interrumpido: decir que sigue corriendo es mentir.
	huerfano, err := st.CrearTrabajo(ctx, org.ID, "suspend", jefa.ID, 10)
	if err != nil {
		t.Fatalf("CrearTrabajo (huérfano): %v", err)
	}
	tr, err = st.Trabajo(ctx, org.ID, huerfano)
	if err != nil {
		t.Fatalf("Trabajo (huérfano): %v", err)
	}
	if v := tr.EstadoVisible(time.Now().Add(store.LatidoMaximo + time.Minute)); v != "interrumpido" {
		t.Errorf("sin latido reciente el estado visible es %q, quiero interrumpido", v)
	}

	// Aislamiento: un id de otra organización no liga.
	otra, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "globex", Name: "Globex"})
	if err != nil {
		t.Fatalf("CreateOrg (otra): %v", err)
	}
	if _, err := st.Trabajo(ctx, otra.ID, id); err == nil {
		t.Error("un trabajo de otra organización no se puede leer")
	}
	if pedida, _ := st.PedirCancelacionTrabajo(ctx, otra.ID, huerfano); pedida {
		t.Error("una organización no cancela los trabajos de otra")
	}

	// Y la lista es la de la organización, la más reciente primero.
	lista, err := st.Trabajos(ctx, org.ID)
	if err != nil || len(lista) != 2 || lista[0].ID != huerfano {
		t.Fatalf("Trabajos: %+v %v", lista, err)
	}
	if otros, _ := st.Trabajos(ctx, otra.ID); len(otros) != 0 {
		t.Errorf("globex ve %d trabajos de acme", len(otros))
	}
}
