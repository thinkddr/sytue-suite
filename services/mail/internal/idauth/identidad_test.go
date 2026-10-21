// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package idauth

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
)

// COM-14. Un token sin dirección no acredita la que el cliente presente; uno con
// dirección acredita esa (o la de registro) y ninguna otra.
func TestElTokenSinDireccionNoAcreditaNinguna(t *testing.T) {
	casos := []struct {
		identidad string
		claims    authn.Claims
		vale      bool
	}{
		{"seguridad@sytue.eu", authn.Claims{Subject: "u"}, false},
		{"", authn.Claims{Subject: "u"}, false},
		{"ana@acme.sytue.eu", authn.Claims{Subject: "u", Mailbox: "ana@acme.sytue.eu"}, true},
		{"ANA@gmail.example", authn.Claims{Subject: "u", Mailbox: "ana@acme.sytue.eu", Email: "ana@gmail.example"}, true},
		{"jefe@acme.sytue.eu", authn.Claims{Subject: "u", Mailbox: "ana@acme.sytue.eu"}, false},
	}
	for _, c := range casos {
		if err := identidadDelToken(c.identidad, c.claims); (err == nil) != c.vale {
			t.Errorf("identidad %q con %+v: err=%v; vale=%v", c.identidad, c.claims, err, c.vale)
		}
	}
}
