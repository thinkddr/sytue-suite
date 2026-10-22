// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

// Tests OFFLINE del fan-out de la búsqueda federada del gateway. Prueban la lógica de
// fusión + degradación por-conector con capacidades mock, sin store ni PostgreSQL: el
// handler HTTP solo añade el plumbing (RLS + carga de secretos + Runtime) por encima.

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

// fakeSearch es una SearchCapability mock: devuelve hits fijos o un error.
type fakeSearch struct {
	hits []connector.ExternalHit
	err  error
}

func (f fakeSearch) Search(context.Context, connector.Instance, connector.Runtime, string, int) ([]connector.ExternalHit, error) {
	return f.hits, f.err
}

func job(id string, sc connector.SearchCapability) searchJob {
	return searchJob{cap: sc, inst: connector.Instance{ID: id, TemplateKey: "mock"}}
}

// TestFanOutSearchDegradesPerConnector: dos instancias, una OK y una que falla; la
// que falla degrada (se omite) sin romper la federación.
func TestFanOutSearchDegradesPerConnector(t *testing.T) {
	ok := job("ok", fakeSearch{hits: []connector.ExternalHit{{Title: "Ada", Source: "M365"}}})
	boom := job("boom", fakeSearch{err: errors.New("upstream 502")})

	hits := fanOutSearch(context.Background(), []searchJob{ok, boom}, "ada", 20)
	if len(hits) != 1 || hits[0].Title != "Ada" {
		t.Fatalf("hits = %+v, quiero solo el del conector OK (el que falla degrada)", hits)
	}
}

// TestFanOutSearchCapsTotal: la fusión respeta el límite total.
func TestFanOutSearchCapsTotal(t *testing.T) {
	many := make([]connector.ExternalHit, 30)
	for i := range many {
		many[i] = connector.ExternalHit{Title: "x", Source: "s"}
	}
	hits := fanOutSearch(context.Background(), []searchJob{job("a", fakeSearch{hits: many})}, "x", 10)
	if len(hits) != 10 {
		t.Fatalf("len = %d, quiero recorte al límite total 10", len(hits))
	}
}

// TestFanOutSearchEmptyJobs: sin conectores, respuesta vacía (no nil) y sin pánico.
func TestFanOutSearchEmptyJobs(t *testing.T) {
	hits := fanOutSearch(context.Background(), nil, "x", 10)
	if hits == nil || len(hits) != 0 {
		t.Fatalf("hits = %+v, quiero un slice vacío no-nil", hits)
	}
}

// TestParseSearchLimit acota el límite total a [1, searchMaxLimit] con default.
func TestParseSearchLimit(t *testing.T) {
	cases := map[string]int{"": searchDefaultLimit, "0": searchDefaultLimit, "-3": searchDefaultLimit, "5": 5, "999": searchMaxLimit}
	for in, want := range cases {
		if got := parseSearchLimit(in); got != want {
			t.Errorf("parseSearchLimit(%q) = %d, quiero %d", in, got, want)
		}
	}
}

// TestConnectionSet parsea el filtro CSV de ids de instancia.
func TestConnectionSet(t *testing.T) {
	if connectionSet("  ") != nil {
		t.Fatal("connections vacío debe ser nil (sin filtro)")
	}
	got := connectionSet("a, b ,,c")
	if len(got) != 3 || !got["a"] || !got["b"] || !got["c"] {
		t.Fatalf("connectionSet = %v, quiero {a,b,c}", got)
	}
}

// Compile-time: fakeSearch satisface el contrato de capacidad de búsqueda.
var _ connector.SearchCapability = fakeSearch{}
