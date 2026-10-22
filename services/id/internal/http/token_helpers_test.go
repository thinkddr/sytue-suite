// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTokenError(t *testing.T) {
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/oauth2/token", http.NoBody)
	tokenError(rr, r, http.StatusBadRequest, "invalid_grant", "código inválido")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
	if rr.Header().Get("Cache-Control") != "no-store" || rr.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("cabeceras de no-cache ausentes")
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	if body["error"] != "invalid_grant" || body["error_description"] != "código inválido" {
		t.Fatalf("cuerpo=%v", body)
	}
}

func TestWriteTokens(t *testing.T) {
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/oauth2/token", http.NoBody)
	writeTokens(rr, r, &tokenResponse{
		AccessToken: "at", TokenType: "Bearer", ExpiresIn: 900, Scope: "openid",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache-control=%q", rr.Header().Get("Cache-Control"))
	}
	var resp tokenResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.AccessToken != "at" || resp.TokenType != "Bearer" || resp.ExpiresIn != 900 {
		t.Fatalf("respuesta=%+v", resp)
	}
	// RefreshToken/IDToken vacíos se omiten (omitempty): no aparecen en el JSON.
	if strings.Contains(rr.Body.String(), "refresh_token") {
		t.Fatalf("refresh_token no debería serializarse: %s", rr.Body.String())
	}
}

func TestContainsScope(t *testing.T) {
	cases := []struct {
		scope string
		want  string
		ok    bool
	}{
		{"openid profile offline_access", "offline_access", true},
		{"openid", "offline_access", false},
		{"", "openid", false},
		{"a b c", "b", true},
	}
	for _, tc := range cases {
		if got := containsScope(tc.scope, tc.want); got != tc.ok {
			t.Fatalf("containsScope(%q,%q)=%v, se esperaba %v", tc.scope, tc.want, got, tc.ok)
		}
	}
}
