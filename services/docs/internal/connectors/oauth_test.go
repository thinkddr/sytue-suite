// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// --- Saneado de config OAuth2 client_credentials ---

func TestSanitizeConfigOAuth2(t *testing.T) {
	raw := json.RawMessage(`{"base_url":"https://api.example.com/v1","auth":{"type":"oauth2_client_credentials","token_url":"https://auth.example.com/oauth/token","client_id":"cid","scope":"read:data"},"junk":"drop"}`)
	out, err := SanitizeConfig(KindREST, raw)
	if err != nil {
		t.Fatalf("SanitizeConfig OAuth2 válida: %v", err)
	}
	var c RestConfig
	if err := json.Unmarshal(out, &c); err != nil {
		t.Fatalf("unmarshal salida: %v", err)
	}
	if c.Auth.Type != RestAuthOAuth2ClientCreds || c.Auth.TokenURL != "https://auth.example.com/oauth/token" ||
		c.Auth.ClientID != "cid" || c.Auth.Scope != "read:data" {
		t.Fatalf("config OAuth2 no canónica: %+v", c.Auth)
	}
	if c.Auth.ClientAuth != "body" { // default client_secret_post
		t.Fatalf("client_auth por defecto = %q; se esperaba 'body'", c.Auth.ClientAuth)
	}

	bad := []json.RawMessage{
		json.RawMessage(`{"base_url":"https://x","auth":{"type":"oauth2_client_credentials","client_id":"c"}}`),                                               // sin token_url
		json.RawMessage(`{"base_url":"https://x","auth":{"type":"oauth2_client_credentials","token_url":"ftp://t/tok","client_id":"c"}}`),                     // token_url no http(s)
		json.RawMessage(`{"base_url":"https://x","auth":{"type":"oauth2_client_credentials","token_url":"https://t/tok"}}`),                                   // sin client_id
		json.RawMessage(`{"base_url":"https://x","auth":{"type":"oauth2_client_credentials","token_url":"https://t/tok","client_id":"c","client_auth":"x"}}`), // client_auth inválido
	}
	for _, b := range bad {
		if _, err := SanitizeConfig(KindREST, b); !errors.Is(err, ErrInvalid) {
			t.Errorf("SanitizeConfig OAuth2(%s) = %v; se esperaba ErrInvalid", b, err)
		}
	}
}

// --- Clave de caché ---

func TestOAuthCacheKey(t *testing.T) {
	base := RestAuth{Type: RestAuthOAuth2ClientCreds, TokenURL: "https://t/tok", ClientID: "cid", Scope: "read", ClientAuth: "body"}
	k := oauthCacheKey(base, "secret")

	if oauthCacheKey(base, "secret") != k {
		t.Fatal("misma config+secreto debería dar la misma clave")
	}
	if oauthCacheKey(base, "otro") == k {
		t.Error("distinto client_secret debería cambiar la clave")
	}
	other := base
	other.Scope = "write"
	if oauthCacheKey(other, "secret") == k {
		t.Error("distinto scope debería cambiar la clave")
	}
	// La clave es un hash: nunca contiene el secreto en claro.
	if len(k) != 64 { // sha256 hex
		t.Fatalf("clave no parece un hash sha256 hex: %q", k)
	}
}

// --- expires_in tolerante (número o cadena) ---

func TestParseExpiresIn(t *testing.T) {
	cases := map[string]int64{
		`3600`:   3600,
		`"3600"`: 3600, // algunos proveedores lo mandan como cadena
		`0`:      0,
		``:       0,
		`"abc"`:  0,
		`null`:   0,
	}
	for in, want := range cases {
		if got := parseExpiresIn(json.RawMessage(in)); got != want {
			t.Errorf("parseExpiresIn(%s) = %d; se esperaba %d", in, got, want)
		}
	}
}

// --- Validez del token cacheado (margen de reloj) ---

func TestCachedTokenValid(t *testing.T) {
	now := time.Now()
	if !(cachedToken{accessToken: "x", expiresAt: now.Add(time.Hour)}).valid(now) {
		t.Error("token con 1h de vida debería ser válido")
	}
	if (cachedToken{accessToken: "x", expiresAt: now.Add(-time.Minute)}).valid(now) {
		t.Error("token expirado no debería ser válido")
	}
	// Dentro del margen de reloj (oauthTokenSkew): se trata como caducado.
	if (cachedToken{accessToken: "x", expiresAt: now.Add(oauthTokenSkew / 2)}).valid(now) {
		t.Error("token a punto de expirar (dentro del skew) no debería ser válido")
	}
	if (cachedToken{accessToken: "", expiresAt: now.Add(time.Hour)}).valid(now) {
		t.Error("token vacío nunca es válido")
	}
}

// --- Caché: acierto sin red ---

func TestOAuthTokenCacheHit(t *testing.T) {
	e := NewExecutor(Limits{}, "")
	cfg := RestConfig{
		BaseURL: "https://api.example.com",
		Auth: RestAuth{
			Type: RestAuthOAuth2ClientCreds, TokenURL: "https://auth.example.com/token",
			ClientID: "cid", Scope: "read", ClientAuth: "body",
		},
	}
	creds := Credentials{Secret: "shh"}
	key := oauthCacheKey(cfg.Auth, creds.Secret)

	e.tokens.mu.Lock()
	e.tokens.tokens[key] = cachedToken{accessToken: "cached-abc", expiresAt: time.Now().Add(time.Hour)}
	e.tokens.mu.Unlock()

	// force=false con entrada válida ⇒ devuelve la cacheada SIN tocar la red.
	got, err := e.oauthToken(context.Background(), cfg, creds, false)
	if err != nil || got != "cached-abc" {
		t.Fatalf("acierto de caché: got %q err %v", got, err)
	}
}

// --- SSRF: el token_url pasa por la barrera (destino interno bloqueado) ---

func TestFetchTokenSSRFBlocked(t *testing.T) {
	e := NewExecutor(Limits{}, "")
	auth := RestAuth{
		Type: RestAuthOAuth2ClientCreds, TokenURL: "http://127.0.0.1:9/token",
		ClientID: "cid", ClientAuth: "body",
	}
	// Un token_url que resuelve a loopback debe bloquearse ANTES de conectar.
	_, _, err := fetchClientCredentialsToken(context.Background(), e.client, auth, "secret")
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("token_url a loopback: se esperaba ErrBlocked, got %v", err)
	}
}
