// SPDX-License-Identifier: Apache-2.0
package oidc

import (
	"testing"
	"time"
)

// TestIssueAccessToken_MFAEnabled: los servicios exigen «segundo factor si la
// cuenta lo tiene» leyendo `mfa_enabled` del Bearer (authn.IdentificacionReciente,
// «soltar la fuente» de la ADR 0086). Va con su valor, también `false`; sin dato
// no se emite, y el servicio entonces exige segundo factor.
func TestIssueAccessToken_MFAEnabled(t *testing.T) {
	priv, set := testKeys(t)
	km := &KeyManager{}
	km.keys = []keyEntry{{kid: "test-kid", private: priv}}
	km.active = &km.keys[0]
	const issuer = "https://id.test"
	iss := NewIssuer(km, issuer, issuer, time.Minute, time.Hour)
	for _, valor := range []bool{true, false} {
		raw, err := iss.IssueAccessToken(TokenClaims{Subject: "user-1", Audience: "sytue-notes", Scope: "openid", MFAEnabled: &valor}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		tok, err := Verify(raw, issuer, issuer, set)
		if err != nil {
			t.Fatal(err)
		}
		if v, ok := tok.Get("mfa_enabled"); !ok || v != valor {
			t.Fatalf("mfa_enabled = %v (%v), quiero %v", v, ok, valor)
		}
	}
	raw, err := iss.IssueAccessToken(TokenClaims{Subject: "svc", Audience: "sytue-notes", Scope: "openid"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tok, err := Verify(raw, issuer, issuer, set)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tok.Get("mfa_enabled"); ok {
		t.Fatal("sin dato no se inventa mfa_enabled")
	}
}
