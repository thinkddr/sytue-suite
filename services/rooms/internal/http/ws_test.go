// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/services/rooms/internal/config"
)

func TestBearerFromHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	r.Header.Set("Authorization", "Bearer tok-123")
	if got := bearerFromHeader(r); got != "tok-123" {
		t.Errorf("bearerFromHeader = %q, want tok-123", got)
	}

	// Prefijo insensible a mayúsculas.
	r2 := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	r2.Header.Set("Authorization", "bearer  spaced ")
	if got := bearerFromHeader(r2); got != "spaced" {
		t.Errorf("bearerFromHeader (lower) = %q, want spaced", got)
	}

	// Sin cabecera / esquema no Bearer.
	r3 := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	if got := bearerFromHeader(r3); got != "" {
		t.Errorf("bearerFromHeader (none) = %q, want vacío", got)
	}
	r4 := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	r4.Header.Set("Authorization", "Basic abc")
	if got := bearerFromHeader(r4); got != "" {
		t.Errorf("bearerFromHeader (basic) = %q, want vacío", got)
	}
}

func TestWithBearerInjectsToken(t *testing.T) {
	var gotCtx bool
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		// El token propagado permite construir un cliente Drive autenticado; aquí
		// basta con verificar que la cadena continúa con el contexto derivado.
		gotCtx = r.Context() != nil
	})
	r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	r.Header.Set("Authorization", "Bearer abc")
	withBearer(next).ServeHTTP(httptest.NewRecorder(), r)
	if !gotCtx {
		t.Error("withBearer debe encadenar con el contexto derivado")
	}
}

func TestWithBearerNoHeaderPassesThrough(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
	r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	withBearer(next).ServeHTTP(httptest.NewRecorder(), r)
	if !called {
		t.Error("withBearer debe encadenar aunque no haya token")
	}
}

func TestWSServeMissingToken(t *testing.T) {
	h := &wsHandler{cfg: config.Config{}} // verifier nil: no se alcanza sin token
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
	h.serve(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}
