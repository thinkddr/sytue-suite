// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package aihttp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/ai/internal/config"
	"git.sytue.eu/sytue/one/services/ai/internal/provider"
)

// TestAssist_EndToEnd ejerce la cadena COMPLETA y real: JWKS + JWT firmado ->
// verificador de authn -> RequireAuth -> rate-limit -> handler -> proveedor
// Scaleway/OpenAI-compatible (upstream simulado) -> { text }. No hay stubs de nuestro propio
// código: solo se simulan las dependencias externas (JWKS y el LLM).
func TestAssist_EndToEnd(t *testing.T) {
	const (
		issuer = "https://id.test.local"
		kid    = "test-kid"
		aud    = "sytue-docs"
	)

	// --- 1) Clave RSA + JWKS servido (como Sytue ID) -------------------------
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa: %v", err)
	}
	pubJWK, _ := jwk.FromRaw(priv.Public())
	_ = pubJWK.Set(jwk.KeyIDKey, kid)
	_ = pubJWK.Set(jwk.AlgorithmKey, jwa.RS256)
	_ = pubJWK.Set(jwk.KeyUsageKey, "sig")
	set := jwk.NewSet()
	_ = set.AddKey(pubJWK)
	jwksBytes, _ := json.Marshal(set)

	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksBytes)
	}))
	defer jwksSrv.Close()

	// --- 2) Verificador real de authn contra ese JWKS ------------------------
	verifier, err := authn.NewVerifier(context.Background(), authn.VerifierConfig{
		Issuer:    issuer,
		JWKSURL:   jwksSrv.URL,
		Audiences: []string{aud},
	})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	// --- 3) Upstream Scaleway simulado + proveedor real ----------------------
	scalewaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("ruta upstream = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"RESULTADO IA"},"finish_reason":"stop"}]}`))
	}))
	defer scalewaySrv.Close()

	prov, err := provider.New(provider.Config{
		Provider: "scaleway", APIKey: "k-secreta", Model: "mistral-small-3.2-24b-instruct-2506",
		BaseURL: scalewaySrv.URL, MaxTokens: 512, Timeout: 5 * time.Second,
		HTTPClient: scalewaySrv.Client(),
	})
	if err != nil {
		t.Fatalf("provider.New: %v", err)
	}

	// --- 4) Router REAL con rate-limit por usuario/tenant --------------------
	cfg := config.Config{
		Provider: "scaleway", Model: "mistral-small-3.2-24b-instruct-2506", Timeout: 5 * time.Second,
		MaxInstructionBytes: 8 * 1024, MaxContextBytes: 256 * 1024,
	}
	cfg.Version = "test"
	router := NewRouter(Deps{
		Cfg: cfg, Verifier: verifier, Provider: prov,
		Limiter: httpx.NewTokenBucket(10, 5),
	})

	// --- 5) JWT firmado (como el access token de la sesión que reenvía el BFF) -
	now := time.Now()
	tok, err := jwt.NewBuilder().
		Issuer(issuer).Subject("user-1").Audience([]string{aud}).
		IssuedAt(now).NotBefore(now).Expiration(now.Add(time.Hour)).
		Claim("tenant", "acme").Build()
	if err != nil {
		t.Fatalf("build token: %v", err)
	}
	hdr := jws.NewHeaders()
	_ = hdr.Set(jws.KeyIDKey, kid)
	privJWK, _ := jwk.FromRaw(priv)
	_ = privJWK.Set(jwk.KeyIDKey, kid)
	_ = privJWK.Set(jwk.AlgorithmKey, jwa.RS256)
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, privJWK, jws.WithProtectedHeaders(hdr)))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// --- 6a) Salud (sin auth) -------------------------------------------------
	{
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody))
		if rec.Code != http.StatusOK {
			t.Fatalf("/healthz status = %d", rec.Code)
		}
	}

	// --- 6b) Sin token -> 401 -------------------------------------------------
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/assist",
			strings.NewReader(`{"action":"x","instruction":"y","context":"","kind":"docx"}`))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("sin token: status = %d, want 401", rec.Code)
		}
	}

	// --- 6c) Con token válido -> 200 { text } --------------------------------
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/assist",
			strings.NewReader(`{"action":"reescribir","instruction":"hazlo formal","context":"hola mundo","kind":"docx"}`))
		req.Header.Set("Authorization", "Bearer "+string(signed))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("con token: status = %d, body=%s", rec.Code, rec.Body.String())
		}
		var out struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("json: %v; body=%s", err, rec.Body.String())
		}
		if out.Text != "RESULTADO IA" {
			t.Errorf("text = %q, want RESULTADO IA", out.Text)
		}
	}
}
