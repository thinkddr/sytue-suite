// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package oidc

import (
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwt"
)

// TestDeviceTrustEnLosDosTokens: `device_trust` va en el ACCESS token —lo aplican
// los servicios— y en el ID token, con el valor evaluado; sin dato no se emite, y
// un servicio lo lee entonces como `none`.
func TestDeviceTrustEnLosDosTokens(t *testing.T) {
	priv, set := testKeys(t)
	km := &KeyManager{}
	km.keys = []keyEntry{{kid: "test-kid", private: priv}}
	km.active = &km.keys[0]
	const issuer = "https://id.test"
	iss := NewIssuer(km, issuer, issuer, time.Minute, time.Hour)

	leer := func(raw, aud string) jwt.Token {
		t.Helper()
		tok, err := Verify(raw, issuer, aud, set)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	for _, valor := range []string{"none", "standard"} {
		c := TokenClaims{Subject: "user-1", Audience: "sytue-notes", Scope: "openid", DeviceTrust: valor, AuthTime: time.Now()}
		at, err := iss.IssueAccessToken(c, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if v, ok := leer(at, issuer).Get("device_trust"); !ok || v != valor {
			t.Errorf("access token: device_trust = %v (%v), quiero %q", v, ok, valor)
		}
		idt, err := iss.IssueIDToken(c, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if v, ok := leer(idt, "sytue-notes").Get("device_trust"); !ok || v != valor {
			t.Errorf("ID token: device_trust = %v (%v), quiero %q", v, ok, valor)
		}
	}
	at, err := iss.IssueAccessToken(TokenClaims{Subject: "svc", Audience: "sytue-notes", Scope: "openid"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := leer(at, issuer).Get("device_trust"); ok {
		t.Error("sin sesión detrás no se inventa device_trust")
	}
}
