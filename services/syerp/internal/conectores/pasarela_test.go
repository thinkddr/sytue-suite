// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package conectores

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestLaInstanciaNoSaleDeSuRuta (NEG-26): una instancia que no es un id no lleva
// la petición, con el token de quien pide, a otra ruta de la pasarela.
func TestLaInstanciaNoSaleDeSuRuta(t *testing.T) {
	var rutas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rutas = append(rutas, r.URL.RequestURI())
		_, _ = w.Write([]byte(`{"columns":[],"rows":[]}`))
	}))
	defer srv.Close()
	p := NewPasarela(srv.URL)
	for _, mala := range []string{"x/../../../operator/oauth-clients", "abc?x=", "../admin"} {
		if _, err := p.Consultar(context.Background(), mala, "Bearer t", map[string]any{}); err == nil {
			t.Errorf("%q: se aceptó", mala)
		}
	}
	if len(rutas) != 0 {
		t.Fatalf("llegaron a la pasarela: %v", rutas)
	}
	if _, err := p.Consultar(context.Background(), "6f1c2a4e-1b2c-4d5e-8f90-123456789abc", "Bearer t", map[string]any{}); err != nil || len(rutas) != 1 ||
		rutas[0] != "/api/v1/instances/6f1c2a4e-1b2c-4d5e-8f90-123456789abc/data/query" {
		t.Fatalf("una instancia buena: %v %v", rutas, err)
	}
}
