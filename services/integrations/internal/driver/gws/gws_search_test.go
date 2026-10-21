// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package gws_test

// Tests OFFLINE de la capacidad SearchCapability del driver Google Workspace. El
// happy path usa un mux propio para servir usuarios del Directory con name/fullName
// (y uno sin él, para el fallback a primaryEmail) + el token endpoint. El de host
// privado NO relaja la barrera SSRF y afirma ErrBlocked. Reusan serviceAccountJSON,
// instance, runtime, allowLoopback y mustFixture de gws_test.go.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/gws"
	sytueoauth2 "git.sytue.eu/sytue/one/services/integrations/internal/oauth2"
)

func TestSearchMapsDirectoryUsers(t *testing.T) {
	allowLoopback(t)
	token := mustFixture(t, "token.json")
	var lastAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(token) })
	mux.HandleFunc("/admin/directory/v1/users", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"users":[
			{"primaryEmail":"ada@acme.com","name":{"fullName":"Ada Lovelace"}},
			{"primaryEmail":"grace@acme.com"}
		]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	saJSON, _ := serviceAccountJSON(t, srv.URL+"/token")

	hits, err := gws.New(sytueoauth2.NewCache()).Search(t.Context(), instance(srv.URL, saJSON), runtime(), "a", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, quiero 2", len(hits))
	}
	if hits[0].Title != "Ada Lovelace" || hits[0].URL != "mailto:ada@acme.com" || hits[0].Source != "Google Workspace" {
		t.Fatalf("hit 0 inesperado: %+v", hits[0])
	}
	// Sin name.fullName ⇒ Title cae al primaryEmail.
	if hits[1].Title != "grace@acme.com" || hits[1].Kind != "person" {
		t.Fatalf("hit 1 (fallback a email) inesperado: %+v", hits[1])
	}
	// El token minteado por el driver (jwt-bearer) va como Bearer.
	if lastAuth != "Bearer gws-access-token-xyz" {
		t.Fatalf("Authorization = %q, quiero el token minteado", lastAuth)
	}
}

// TestSearchPrivateURLBlocked: SIN relajar la barrera, la base apunta a loopback ⇒ el
// egress (empezando por el token endpoint) se bloquea con ssrfhttp.ErrBlocked.
func TestSearchPrivateURLBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"x"}`))
	}))
	defer srv.Close()
	saJSON, _ := serviceAccountJSON(t, srv.URL+"/token")

	_, err := gws.New(sytueoauth2.NewCache()).Search(t.Context(), instance(srv.URL, saJSON), runtime(), "a", 10)
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("err = %v, quiero ssrfhttp.ErrBlocked (host privado)", err)
	}
}
