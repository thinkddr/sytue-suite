// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package integrationshttp

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// NEG-06. La lista de quién consulta la gestionan quien creó la conexión y los
// administradores; abre la conexión a una persona o a un grupo, y quitarla la cierra.
func TestLaListaAbreLaConexionAQuienNombra(t *testing.T) {
	a := montarAlmacen(t, func(h *Handlers, r chi.Router) {
		r.Get("/instances/{id}/readers", h.GetReaders)
		r.Put("/instances/{id}/readers", h.PutReaders)
	})
	lista := func(q quien, id string, l ...domain.Lector) int {
		t.Helper()
		estado, _ := a.pedir(t, q, http.MethodPut, "/instances/"+id+"/readers", map[string]any{"readers": l})
		return estado
	}
	bea1 := domain.Lector{Kind: domain.LectorPersona, ID: "bea", Name: "Bea"}
	finanzas := domain.Lector{Kind: domain.LectorGrupo, ID: "finanzas", Name: "finanzas"}

	// Bea no la gestiona: ni la ve ni se añade.
	if estado, _ := a.pedir(t, bea, http.MethodGet, "/instances/"+a.deAna+"/readers", nil); estado != http.StatusForbidden {
		t.Errorf("Bea lee la lista de Ana: %d, quiero 403", estado)
	}
	if estado := lista(bea, a.deAna, bea1); estado != http.StatusForbidden {
		t.Errorf("Bea se añade a la lista de Ana: %d, quiero 403", estado)
	}
	// Ana la creó: la gestiona aunque ya no administre, y añade a Bea.
	if estado := lista(ana, a.deAna, bea1); estado != http.StatusOK {
		t.Fatalf("Ana añade a Bea: %d", estado)
	}
	if got := a.consulta(t, bea, a.deAna); got != "ok" {
		t.Errorf("Bea, ya en la lista: %q", got)
	}
	estado, cuerpo := a.pedir(t, ana, http.MethodGet, "/instances/"+a.deAna+"/readers", nil)
	var leida lectoresWire
	if err := json.Unmarshal(cuerpo, &leida); err != nil || estado != http.StatusOK {
		t.Fatalf("Ana lee su lista: %d %s", estado, cuerpo)
	}
	if leida.CreatedBy != "ana" || len(leida.Readers) != 1 || leida.Readers[0] != bea1 {
		t.Errorf("lista de Ana: %+v", leida)
	}
	// Ana no crea la de antes: no la gestiona. Carla administra: sí.
	if estado := lista(ana, a.deAntes, bea1); estado != http.StatusForbidden {
		t.Errorf("Ana cambia la lista de una conexión ajena: %d, quiero 403", estado)
	}

	// Un grupo en lugar de la persona: entra quien está en el grupo, y Bea, que no está,
	// sale. Así se ve también que la lista se SUSTITUYE entera.
	if estado := lista(carla, a.deAna, finanzas); estado != http.StatusOK {
		t.Fatalf("Carla pone el grupo: %d", estado)
	}
	if got := a.consulta(t, dani, a.deAna); got != "ok" {
		t.Errorf("Dani, de finanzas: %q", got)
	}
	if got := a.consulta(t, bea, a.deAna); got != "connection_not_allowed" {
		t.Errorf("Bea, ya fuera de la lista: %q", got)
	}
	if got := a.busca(t, dani); len(got) != 1 || got[0] != a.deAna {
		t.Errorf("la búsqueda de Dani consulta %v; solo la que abre su grupo", got)
	}

	// Lo que no es una persona ni un grupo no entra.
	if estado := lista(carla, a.deAna, domain.Lector{Kind: "tenant", ID: "acme"}); estado != http.StatusBadRequest {
		t.Errorf("toda la organización por la puerta de atrás: %d, quiero 400", estado)
	}
}
