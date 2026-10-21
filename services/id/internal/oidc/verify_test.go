// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

const testIssuer = "https://id.test"

// testKeys genera un par RSA y devuelve la clave privada JWK (con kid+alg) y el
// JWKS público correspondiente.
func testKeys(t *testing.T) (jwk.Key, jwk.Set) {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generar RSA: %v", err)
	}
	priv, err := jwk.FromRaw(raw)
	if err != nil {
		t.Fatalf("jwk privada: %v", err)
	}
	_ = priv.Set(jwk.KeyIDKey, "test-kid")
	_ = priv.Set(jwk.AlgorithmKey, jwa.RS256)

	pub, err := jwk.FromRaw(raw.Public())
	if err != nil {
		t.Fatalf("jwk pública: %v", err)
	}
	_ = pub.Set(jwk.KeyIDKey, "test-kid")
	_ = pub.Set(jwk.AlgorithmKey, jwa.RS256)
	_ = pub.Set(jwk.KeyUsageKey, "sig")

	set := jwk.NewSet()
	if err := set.AddKey(pub); err != nil {
		t.Fatalf("añadir clave: %v", err)
	}
	return priv, set
}

func buildToken(t *testing.T, iss string, exp time.Time) jwt.Token {
	t.Helper()
	tok, err := jwt.NewBuilder().
		Issuer(iss).
		Subject("user-1").
		IssuedAt(time.Now()).
		Expiration(exp).
		Build()
	if err != nil {
		t.Fatalf("build token: %v", err)
	}
	return tok
}

func signRS256(t *testing.T, tok jwt.Token, key jwk.Key) string {
	t.Helper()
	b, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, key))
	if err != nil {
		t.Fatalf("firmar: %v", err)
	}
	return string(b)
}

func TestVerify_ValidRS256(t *testing.T) {
	priv, set := testKeys(t)
	raw := signRS256(t, buildToken(t, testIssuer, time.Now().Add(time.Hour)), priv)

	tok, err := Verify(raw, testIssuer, "", set)
	if err != nil {
		t.Fatalf("Verify de token válido falló: %v", err)
	}
	if tok.Subject() != "user-1" {
		t.Fatalf("subject = %q, quería user-1", tok.Subject())
	}
}

// buildTokenAud construye un token con un conjunto de audiencias.
func buildTokenAud(t *testing.T, iss string, aud []string, exp time.Time) jwt.Token {
	t.Helper()
	tok, err := jwt.NewBuilder().
		Issuer(iss).
		Subject("user-1").
		Audience(aud).
		IssuedAt(time.Now()).
		Expiration(exp).
		Build()
	if err != nil {
		t.Fatalf("build token: %v", err)
	}
	return tok
}

func TestVerify_AcceptsExpectedAudience(t *testing.T) {
	priv, set := testKeys(t)
	const resource = "https://id.test"
	// aud incluye el resource server y el client (como emitiría el IdP).
	raw := signRS256(t, buildTokenAud(t, testIssuer, []string{resource, "sytue-one"}, time.Now().Add(time.Hour)), priv)
	if _, err := Verify(raw, testIssuer, resource, set); err != nil {
		t.Fatalf("Verify rechazó un token con la audiencia esperada: %v", err)
	}
}

func TestVerify_RejectsWrongAudience(t *testing.T) {
	priv, set := testKeys(t)
	// El token va dirigido a otro recurso; exigir "https://id.test" debe fallar.
	raw := signRS256(t, buildTokenAud(t, testIssuer, []string{"https://otro.recurso"}, time.Now().Add(time.Hour)), priv)
	if _, err := Verify(raw, testIssuer, "https://id.test", set); err == nil {
		t.Fatal("Verify aceptó un token con audiencia incorrecta")
	} else if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestVerify_RejectsMissingAudienceWhenRequired(t *testing.T) {
	priv, set := testKeys(t)
	// Token sin `aud`; si el resource server exige audiencia, debe rechazarse.
	raw := signRS256(t, buildToken(t, testIssuer, time.Now().Add(time.Hour)), priv)
	if _, err := Verify(raw, testIssuer, "https://id.test", set); err == nil {
		t.Fatal("Verify aceptó un token sin audiencia cuando se exige aud")
	}
}

func TestVerify_RejectsAlgNone(t *testing.T) {
	_, set := testKeys(t)
	// Token firmado con alg=none (jwa.NoSignature). Debe rechazarse por la
	// allowlist ANTES de intentar verificar la firma.
	tok := buildToken(t, testIssuer, time.Now().Add(time.Hour))
	unsigned, err := jwt.Sign(tok, jwt.WithInsecureNoSignature())
	if err != nil {
		t.Fatalf("firmar none: %v", err)
	}
	if _, err := Verify(string(unsigned), testIssuer, "", set); err == nil {
		t.Fatal("Verify aceptó un token alg=none")
	} else if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestVerify_RejectsWrongIssuer(t *testing.T) {
	priv, set := testKeys(t)
	raw := signRS256(t, buildToken(t, "https://evil.example", time.Now().Add(time.Hour)), priv)
	if _, err := Verify(raw, testIssuer, "", set); err == nil {
		t.Fatal("Verify aceptó un issuer incorrecto")
	}
}

func TestVerify_RejectsExpired(t *testing.T) {
	priv, set := testKeys(t)
	raw := signRS256(t, buildToken(t, testIssuer, time.Now().Add(-time.Hour)), priv)
	if _, err := Verify(raw, testIssuer, "", set); err == nil {
		t.Fatal("Verify aceptó un token expirado")
	}
}

func TestVerify_RejectsUnknownKey(t *testing.T) {
	priv, _ := testKeys(t)
	_, otherSet := testKeys(t) // JWKS que NO contiene la clave firmante
	raw := signRS256(t, buildToken(t, testIssuer, time.Now().Add(time.Hour)), priv)
	if _, err := Verify(raw, testIssuer, "", otherSet); err == nil {
		t.Fatal("Verify aceptó un token firmado con clave desconocida")
	}
}

func TestVerify_RejectsGarbage(t *testing.T) {
	_, set := testKeys(t)
	if _, err := Verify("no-es-un-jwt", testIssuer, "", set); err == nil {
		t.Fatal("Verify aceptó basura")
	}
}

func TestBuildDiscovery_NoPlainPKCE(t *testing.T) {
	d := BuildDiscovery("https://id.sytue.eu")
	for _, m := range d.CodeChallengeMethodsSupported {
		if strings.EqualFold(m, "plain") {
			t.Fatal("discovery no debe anunciar el método PKCE plain")
		}
	}
	// Debe firmar solo con algoritmos de la allowlist.
	for _, a := range d.IDTokenSigningAlgValuesSupported {
		if a == "none" {
			t.Fatal("discovery no debe anunciar alg=none")
		}
	}
}
