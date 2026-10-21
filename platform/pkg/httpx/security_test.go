// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeadersDefaults(t *testing.T) {
	h := SecurityHeaders(DefaultSecurityHeaders())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "https://x/", http.NoBody)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	got := rec.Header()
	if got.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("falta nosniff")
	}
	if got.Get("X-Frame-Options") != "DENY" {
		t.Error("falta X-Frame-Options DENY")
	}
	if got.Get("Referrer-Policy") != "no-referrer" {
		t.Error("falta Referrer-Policy")
	}
	if got.Get("Cross-Origin-Opener-Policy") != "same-origin" {
		t.Error("falta COOP")
	}
	if !strings.Contains(got.Get("Strict-Transport-Security"), "includeSubDomains") {
		t.Errorf("HSTS incompleto: %q", got.Get("Strict-Transport-Security"))
	}
	if got.Get("Permissions-Policy") == "" {
		t.Error("falta Permissions-Policy")
	}
}

func TestHSTSOnlyOverTLS(t *testing.T) {
	h := SecurityHeaders(DefaultSecurityHeaders())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	// Sin TLS ni X-Forwarded-Proto=https: no HSTS.
	req := httptest.NewRequest(http.MethodGet, "http://x/", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS no debe emitirse sobre HTTP plano")
	}

	// Con X-Forwarded-Proto=https: sí HSTS.
	req = httptest.NewRequest(http.MethodGet, "http://x/", http.NoBody)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("HSTS debe emitirse tras proxy TLS")
	}
}

func TestCSPConfigurable(t *testing.T) {
	cfg := DefaultSecurityHeaders()
	cfg.ContentSecurityPolicy = DefaultCSP
	rec := httptest.NewRecorder()
	SecurityHeaders(cfg)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "https://x/", http.NoBody))
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors 'none'") || !strings.Contains(csp, "default-src 'self'") {
		t.Fatalf("CSP inesperada: %q", csp)
	}
}
