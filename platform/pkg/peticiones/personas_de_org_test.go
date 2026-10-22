// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package peticiones

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// La puerta sin sesión de Sytue ID se pide con su secreto, para UNA organización, y
// sin secreto ni se intenta.
func TestPersonasDeOrg(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v struct {
			Org        string              `json:"org"`
			Emails     []string            `json:"emails"`
			Candidatos map[string][]string `json:"candidatos"`
		}
		_ = json.NewDecoder(r.Body).Decode(&v)
		if r.URL.Path != "/internal/directory/resolve" || r.Header.Get("Authorization") != "Bearer s" || v.Org != "acme" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		if len(v.Candidatos["bob@globex.es"]) != 1 {
			http.Error(w, "sin candidatos", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"users":{"ana@acme.es":"u-ana"},"con_permiso":{"bob@globex.es":"u-bob"}}`))
	}))
	defer srv.Close()
	got, err := PersonasDeOrg(context.Background(), srv.URL, "s", "acme", ConsultaDirectorio{
		Emails: []string{"ana@acme.es", "x@y.es"}, Candidatos: map[string][]string{"bob@globex.es": {"u-bob"}},
	})
	if err != nil || len(got.Miembros) != 1 || got.Miembros["ana@acme.es"] != "u-ana" || got.ConPermiso["bob@globex.es"] != "u-bob" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := PersonasDeOrg(context.Background(), srv.URL, "otro", "acme", ConsultaDirectorio{}); err == nil {
		t.Fatal("un 401 tiene que ser un error, no «nadie»")
	}
	if _, err := PersonasDeOrg(context.Background(), srv.URL, "", "acme", ConsultaDirectorio{}); err == nil {
		t.Fatal("sin secreto no se pregunta")
	}
}

// Las organizaciones de una persona, con el mismo secreto; sin secreto ni se pregunta.
func TestOrganizacionesDe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v struct {
			UserID string `json:"user_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&v)
		if r.URL.Path != "/internal/people/orgs" || r.Header.Get("Authorization") != "Bearer s" || v.UserID != "u-ana" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"orgs":["acme","globex"],"personal":true}`))
	}))
	defer srv.Close()
	got, personal, err := OrganizacionesDe(context.Background(), srv.URL, "s", "u-ana")
	if err != nil || len(got) != 2 || got[0] != "acme" || !personal {
		t.Fatalf("%v %v %v", got, personal, err)
	}
	if _, _, err := OrganizacionesDe(context.Background(), srv.URL, "otro", "u-ana"); err == nil {
		t.Fatal("un 401 tiene que ser un error, no «ninguna»")
	}
	if _, _, err := OrganizacionesDe(context.Background(), srv.URL, "", "u-ana"); err == nil {
		t.Fatal("sin secreto no se pregunta")
	}
}
