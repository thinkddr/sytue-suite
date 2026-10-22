// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package personas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// El cliente pregunta a Sytue ID con su secreto, recuerda también lo que no tiene
// nombre y, si Sytue ID no contesta, el aviso sale sin nombre.
func TestClienteNombresOrgs(t *testing.T) {
	llamadas := 0
	id := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llamadas++
		if r.URL.Path != "/internal/orgs/names" || r.Header.Get("Authorization") != "Bearer s" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		var req struct{ Slugs []string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(map[string]any{"orgs": []map[string]string{{"slug": "otra", "name": "Acme Ingeniería S.L."}}})
	}))
	defer id.Close()

	c := NuevosNombresOrgs(id.URL+"/", "s")
	ctx := context.Background()
	for range 2 {
		if n := c.Nombres(ctx, []string{"otra", "sin-nombre"}); len(n) != 1 || n["otra"] != "Acme Ingeniería S.L." {
			t.Fatalf("nombres: %v", n)
		}
	}
	if llamadas != 1 {
		t.Fatalf("la segunda vez sale de la caché, también la que no tiene nombre: %d llamadas", llamadas)
	}
	if n := NuevosNombresOrgs(id.URL, "otro").Nombres(ctx, []string{"otra"}); len(n) != 0 {
		t.Fatalf("sin respuesta de Sytue ID no hay nombre: %v", n)
	}
}
