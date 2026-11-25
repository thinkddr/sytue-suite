// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubnotify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// TestLaSenalVaALaOrganizacionDeLoEscrito (ADR 0057): quien escribe en un documento de
// otra organización (la ruta pasa a la anfitriona) recibe la señal en la suya, y el
// propietario y sus colaboradores, en la anfitriona, que es donde lo ven. Lo añadido
// con el contexto de la escritura sigue en la organización de quien escribe.
// VISTO EN ROJO mandándolo todo a la organización de quien escribe.
func TestLaSenalVaALaOrganizacionDeLoEscrito(t *testing.T) {
	c := New("http://hub.invalid", "s")
	f := conRelojFalso(c)
	h := c.TrasEscribir(nil, "docs")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		AvisarTambien(r.Context(), "compañera")
		anfitriona, err := tenancy.New("globex")
		if err != nil {
			t.Fatal(err)
		}
		AvisarTambien(tenancy.Into(r.Context(), anfitriona), "dueña", "editora")
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodPatch, "/documents/d1", http.NoBody)
	r = r.WithContext(authn.Into(r.Context(), authn.Claims{Subject: "ana", Tenant: "acme"}))
	h.ServeHTTP(httptest.NewRecorder(), r)
	quiero := []claveCambio{{"acme", "ana", "docs"}, {"acme", "compañera", "docs"}, {"globex", "dueña", "docs"}, {"globex", "editora", "docs"}}
	if got := f.vaciar(); !slices.Equal(got, quiero) {
		t.Fatalf("la señal va a %v; quiero %v", got, quiero)
	}
}

// TestQuienesVenSeConsultaUnaVezPorRecurso: la verja de escritura de un servicio puede
// pasarse varias veces en una petición (un lote de registros de una base); quienes ven el
// recurso se consultan UNA vez por recurso, y fuera de una escritura con señal, ninguna.
// Un fallo de la consulta deja la señal sin ellos, sin tumbar la escritura.
func TestQuienesVenSeConsultaUnaVezPorRecurso(t *testing.T) {
	consultas := 0
	quienes := func(subjects ...string) func() ([]string, error) {
		return func() ([]string, error) {
			consultas++
			return subjects, nil
		}
	}
	AvisarQuienesVen(context.Background(), "base-1", quienes("ana"))
	if consultas != 0 {
		t.Fatal("fuera de una escritura con señal se consulta")
	}
	ctx, avisados := EscrituraDePrueba(context.Background())
	for range 3 {
		AvisarQuienesVen(ctx, "base-1", quienes("ana", "bea"))
	}
	AvisarQuienesVen(ctx, "base-2", quienes("eva"))
	AvisarQuienesVen(ctx, "base-3", func() ([]string, error) { consultas++; return []string{"x"}, errors.New("caída") })
	if got := avisados(); consultas != 3 || !slices.Equal(got, []string{"ana", "bea", "eva"}) {
		t.Fatalf("%d consultas, avisados %v", consultas, got)
	}
}
