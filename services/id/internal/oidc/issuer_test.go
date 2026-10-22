// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

// kmWithKey construye un KeyManager en memoria con una clave activa (privada +
// pública), sin tocar la base de datos, para probar la emisión/firma/JWKS.
func kmWithKey(t *testing.T) (*KeyManager, jwk.Set) {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("RSA: %v", err)
	}
	priv, err := jwk.FromRaw(raw)
	if err != nil {
		t.Fatal(err)
	}
	_ = priv.Set(jwk.KeyIDKey, "kid-1")
	_ = priv.Set(jwk.AlgorithmKey, jwa.RS256)
	pub, err := jwk.FromRaw(raw.Public())
	if err != nil {
		t.Fatal(err)
	}
	_ = pub.Set(jwk.KeyIDKey, "kid-1")
	_ = pub.Set(jwk.AlgorithmKey, jwa.RS256)
	_ = pub.Set(jwk.KeyUsageKey, "sig")

	km := &KeyManager{keys: []keyEntry{{kid: "kid-1", alg: jwa.RS256, private: priv, public: pub, active: true}}}
	km.active = &km.keys[0]

	set := jwk.NewSet()
	_ = set.AddKey(pub)
	return km, set
}

func TestIssueIDToken(t *testing.T) {
	km, set := kmWithKey(t)
	iss := NewIssuer(km, testIssuer, "", time.Minute, time.Hour)

	raw, err := iss.IssueIDToken(TokenClaims{
		Subject:       "user-1",
		Audience:      "sytue-one",
		Nonce:         "n0nce",
		AuthTime:      time.Now(),
		AMR:           []string{"pwd", "mfa"},
		Tenant:        "acme",
		OrgColor:      "cobalto",
		OrgForma:      "carril",
		Roles:         []string{"owner"},
		Email:         "user@acme.eu",
		EmailVerified: true,
		Name:          "User One",
	}, time.Now())
	if err != nil {
		t.Fatalf("IssueIDToken: %v", err)
	}

	// El ID token va dirigido al client: se verifica exigiendo esa audiencia.
	tok, err := Verify(raw, testIssuer, "sytue-one", set)
	if err != nil {
		t.Fatalf("Verify ID token: %v", err)
	}
	if tok.Subject() != "user-1" {
		t.Errorf("subject = %q", tok.Subject())
	}
	if v, ok := tok.Get("nonce"); !ok || v != "n0nce" {
		t.Errorf("nonce ausente: %v", v)
	}
	if v, ok := tok.Get("email"); !ok || v != "user@acme.eu" {
		t.Errorf("email ausente: %v", v)
	}
	if v, ok := tok.Get("tenant"); !ok || v != "acme" {
		t.Errorf("tenant ausente: %v", v)
	}
	if v, ok := tok.Get("org_color"); !ok || v != "cobalto" {
		t.Errorf("org_color ausente: %v", v)
	}
	if v, ok := tok.Get("org_forma"); !ok || v != "carril" {
		t.Errorf("org_forma ausente: %v", v)
	}
}

func TestIssueIDTokenMinimalClaims(t *testing.T) {
	km, set := kmWithKey(t)
	iss := NewIssuer(km, testIssuer, "", time.Minute, time.Hour)
	// Solo subject+audience; las ramas opcionales se omiten.
	raw, err := iss.IssueIDToken(TokenClaims{Subject: "u", Audience: "c", AuthTime: time.Now()}, time.Now())
	if err != nil {
		t.Fatalf("IssueIDToken minimal: %v", err)
	}
	tok, err := Verify(raw, testIssuer, "c", set)
	if err != nil {
		t.Fatalf("Verify minimal: %v", err)
	}
	// El color de la casa no viaja: sin claim, la web pinta sin atributo.
	if _, ok := tok.Get("org_color"); ok {
		t.Error("org_color no debe emitirse cuando la organización no ha elegido color")
	}
	if _, ok := tok.Get("org_forma"); ok {
		t.Error("org_forma no debe emitirse cuando la organización no ha elegido forma")
	}
}

func TestAccessTokenTTLDefaults(t *testing.T) {
	km, _ := kmWithKey(t)
	// TTLs no positivos caen a los valores por defecto (15m / 1h).
	iss := NewIssuer(km, testIssuer, "", 0, 0)
	if iss.AccessTokenTTL() != 15*time.Minute {
		t.Fatalf("AccessTokenTTL por defecto = %v", iss.AccessTokenTTL())
	}
	// TTL explícito se respeta.
	iss = NewIssuer(km, testIssuer, "", 42*time.Second, time.Hour)
	if iss.AccessTokenTTL() != 42*time.Second {
		t.Fatalf("AccessTokenTTL explícito = %v", iss.AccessTokenTTL())
	}
}

func TestPublicJWKS(t *testing.T) {
	km, _ := kmWithKey(t)
	set, err := km.PublicJWKS()
	if err != nil {
		t.Fatalf("PublicJWKS: %v", err)
	}
	if set.Len() != 1 {
		t.Fatalf("JWKS debe tener 1 clave, tiene %d", set.Len())
	}
	if _, ok := set.LookupKeyID("kid-1"); !ok {
		t.Fatal("la clave kid-1 debe estar en el JWKS público")
	}
}

func TestSigningKeyNoActive(t *testing.T) {
	km := &KeyManager{}
	if _, err := km.signingKey(); err == nil {
		t.Fatal("sin clave activa signingKey debe fallar")
	}
	// Un Issuer sobre un KeyManager sin clave no puede firmar.
	iss := NewIssuer(km, testIssuer, "", time.Minute, time.Hour)
	if _, err := iss.IssueAccessToken(TokenClaims{Subject: "u", Audience: "c"}, time.Now()); err == nil {
		t.Fatal("IssueAccessToken sin clave debe fallar")
	}
}
