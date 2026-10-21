// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package graph_test

// Tests OFFLINE de la capacidad SearchCapability del driver Microsoft Graph. Reusan
// el httptest de graph_test.go (graphServer sirve /v1.0/users desde el fixture
// users_page1.json y el token endpoint). El camino feliz relaja la barrera SSRF para
// loopback; el de host privado NO la relaja y afirma ErrBlocked.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/graph"
	sytueoauth2 "git.sytue.eu/sytue/one/services/integrations/internal/oauth2"
)

func TestSearchMintsTokenMapsUsers(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth, _ := graphServer(t)

	hits, err := graph.New(sytueoauth2.NewCache()).Search(t.Context(), instance(srv.URL, "shh"), runtime(), "ada", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// El fixture users_page1.json trae 2 usuarios (Ada, Alan).
	if len(hits) != 2 {
		t.Fatalf("hits = %d, quiero 2 (usuarios de la primera página)", len(hits))
	}
	h := hits[0]
	if h.Title != "Ada Lovelace" || h.Kind != "person" || h.Source != "Microsoft 365" {
		t.Fatalf("hit inesperado: %+v", h)
	}
	if h.Snippet != "ada@contoso.com" || h.URL != "mailto:ada@contoso.com" {
		t.Fatalf("mapeo de email inesperado: snippet=%q url=%q", h.Snippet, h.URL)
	}
	// El token minteado por el driver (client-credentials) va como Bearer.
	if *lastAuth != "Bearer graph-app-token-abc123" {
		t.Fatalf("Authorization = %q, quiero el token minteado", *lastAuth)
	}
}

// TestSearchEmptyQueryNoEgress: una q vacía no toca la red y devuelve vacío.
func TestSearchEmptyQueryNoEgress(t *testing.T) {
	hits, err := graph.New(sytueoauth2.NewCache()).Search(t.Context(), instance("https://graph.microsoft.com", "shh"), runtime(), "   ", 10)
	if err != nil || hits != nil {
		t.Fatalf("Search(q vacía) = (%v, %v), quiero (nil, nil) sin egress", hits, err)
	}
}

// TestSearchPrivateURLBlocked: SIN relajar la barrera, la base apunta a loopback ⇒ el
// egress (empezando por el token endpoint) se bloquea con ssrfhttp.ErrBlocked.
func TestSearchPrivateURLBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"x"}`))
	}))
	defer srv.Close()

	_, err := graph.New(sytueoauth2.NewCache()).Search(t.Context(), instance(srv.URL, "shh"), runtime(), "ada", 10)
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("err = %v, quiero ssrfhttp.ErrBlocked (host privado)", err)
	}
}
