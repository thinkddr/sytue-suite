// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package authn

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lestrrat-go/jwx/v2/jwa"
)

func TestNewVerifierRejectsNoneAlg(t *testing.T) {
	_, err := NewVerifier(context.Background(), VerifierConfig{
		Issuer:      "https://id.sytue.eu",
		AllowedAlgs: []jwa.SignatureAlgorithm{jwa.NoSignature},
	})
	if err == nil {
		t.Fatal("el algoritmo none debe rechazarse en la construcción")
	}
}

func TestClaimsLogValueOmitsPII(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&buf, nil))
	claims := Claims{Subject: "u1", Tenant: "acme", Email: "ada@sytue.eu", Name: "Ada", Roles: []string{"admin"}}
	l.Info("auth", "claims", claims)

	s := buf.String()
	if bytes.Contains(buf.Bytes(), []byte("ada@sytue.eu")) || bytes.Contains(buf.Bytes(), []byte("Ada")) {
		t.Fatalf("LogValue filtró PII: %s", s)
	}
	var m map[string]any
	_ = json.Unmarshal(buf.Bytes(), &m)
	c, _ := m["claims"].(map[string]any)
	if c["sub"] != "u1" || c["tenant"] != "acme" {
		t.Fatalf("LogValue no expone identificadores no PII: %v", c)
	}
}

func TestRequireTenant(t *testing.T) {
	v := &Verifier{} // no requiere red para este middleware
	protected := v.RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Sin tenant válido -> 403.
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req = req.WithContext(Into(req.Context(), Claims{Subject: "u1"}))
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sin tenant debe ser 403, got %d", rec.Code)
	}

	// Con tenant válido -> 200.
	req = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req = req.WithContext(Into(req.Context(), Claims{Subject: "u1", Tenant: "acme"}))
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("con tenant debe pasar, got %d", rec.Code)
	}
}

func TestRequireRoleAndScope(t *testing.T) {
	v := &Verifier{}
	base := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	roleMW := v.RequireRole("admin")(base)
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req = req.WithContext(Into(req.Context(), Claims{Roles: []string{"member"}}))
	rec := httptest.NewRecorder()
	roleMW.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("rol insuficiente debe ser 403, got %d", rec.Code)
	}
}
