// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package directory_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/directory"
)

// mockID es un doble en memoria de los endpoints admin del directorio de Sytue ID
// que el cliente modela: /oauth2/token (client_credentials) y los listados admin de
// usuarios (paginado por cursor) y grupos (+ miembros por grupo).
type mockID struct {
	tokenCalls int
	// users se sirven de dos en dos para ejercer el seguimiento de nextCursor.
	users [][2]string // {id, status}
}

func (m *mockID) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/oauth2/token":
			m.tokenCalls++
			if got := r.FormValue("grant_type"); got != "client_credentials" {
				t.Errorf("grant_type = %q, quiero client_credentials", got)
			}
			if got := r.FormValue("client_id"); got != "sytue-gateway" {
				t.Errorf("client_id = %q, quiero sytue-gateway", got)
			}
			if got := r.FormValue("organization"); got != "acme" {
				t.Errorf("organization = %q, quiero acme", got)
			}
			writeJSON(w, map[string]any{"access_token": "tok-123", "token_type": "Bearer", "expires_in": 3600})

		case r.URL.Path == "/api/v1/admin/users":
			if got := r.Header.Get("Authorization"); got != "Bearer tok-123" {
				t.Errorf("Authorization = %q, quiero Bearer tok-123", got)
			}
			start := 0
			if c := r.URL.Query().Get("cursor"); c != "" {
				start, _ = strconv.Atoi(c)
			}
			end := start + 2
			if end > len(m.users) {
				end = len(m.users)
			}
			items := make([]map[string]any, 0, 2)
			for _, u := range m.users[start:end] {
				items = append(items, map[string]any{"id": u[0], "email": u[0] + "@acme.test", "name": "N " + u[0], "status": u[1]})
			}
			next := ""
			if end < len(m.users) {
				next = strconv.Itoa(end)
			}
			writeJSON(w, map[string]any{"items": items, "nextCursor": next})

		case r.URL.Path == "/api/v1/admin/groups":
			writeJSON(w, map[string]any{"groups": []map[string]any{
				{"id": "g1", "name": "Engineering", "description": "eng"},
			}})

		case strings.HasPrefix(r.URL.Path, "/api/v1/admin/groups/"):
			writeJSON(w, map[string]any{
				"group":   map[string]any{"id": "g1", "name": "Engineering"},
				"members": []map[string]any{{"user_id": "u1"}, {"user_id": "u2"}},
			})

		default:
			t.Errorf("ruta inesperada: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func newClient(url string) *directory.Client {
	return directory.New(directory.Config{BaseURL: url, ClientID: "sytue-gateway", ClientSecret: "s3cr3t"})
}

func TestListUsersPaginated(t *testing.T) {
	m := &mockID{users: [][2]string{{"u1", "active"}, {"u2", "active"}, {"u3", "suspended"}, {"u4", "active"}, {"u5", "active"}}}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()

	users, err := newClient(srv.URL).ListUsers(t.Context(), "acme")
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 5 {
		t.Fatalf("usuarios = %d, quiero 5 (paginación seguida): %+v", len(users), users)
	}
	// El estado se mapea a Active (solo "active" ⇒ true).
	byID := map[string]directory.User{}
	for _, u := range users {
		byID[u.ID] = u
	}
	if !byID["u1"].Active || byID["u3"].Active {
		t.Fatalf("mapeo de estado incorrecto: u1=%v u3=%v", byID["u1"].Active, byID["u3"].Active)
	}
	if byID["u1"].Email != "u1@acme.test" {
		t.Fatalf("email inesperado: %q", byID["u1"].Email)
	}
}

func TestListGroupsResolvesMembers(t *testing.T) {
	m := &mockID{}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()

	groups, err := newClient(srv.URL).ListGroups(t.Context(), "acme")
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != "g1" {
		t.Fatalf("grupos inesperados: %+v", groups)
	}
	if len(groups[0].Members) != 2 || groups[0].Members[0] != "u1" {
		t.Fatalf("miembros no resueltos: %+v", groups[0].Members)
	}
}

func TestTokenCached(t *testing.T) {
	m := &mockID{users: [][2]string{{"u1", "active"}}}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()

	c := newClient(srv.URL)
	ctx := t.Context()
	if _, err := c.ListUsers(ctx, "acme"); err != nil {
		t.Fatalf("ListUsers 1: %v", err)
	}
	if _, err := c.ListGroups(ctx, "acme"); err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	// Varias llamadas, un solo canje de token (cacheado por org).
	if m.tokenCalls != 1 {
		t.Fatalf("canjes de token = %d, quiero 1 (cacheado)", m.tokenCalls)
	}
}
