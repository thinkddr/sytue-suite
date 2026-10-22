// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"net/url"
	"testing"
)

func TestDestinoDeEnlace(t *testing.T) {
	const id = "0b7e4c1a-2f3d-4a5b-8c6d-7e8f9a0b1c2d"
	for _, c := range []struct {
		enlace, base string
		ok           bool
	}{
		{"sytue-bases:/bases/base/" + id, id, true},
		{"sytue-bases:base/" + id, id, true}, // sin barra: va en Opaque
		{"https://bases.sytue.eu/bases/acme/base/" + id + "?table=x&record=y", id, true},
		{"sytue-bases:/bases/base/0B7E4C1A-2F3D-4A5B-8C6D-7E8F9A0B1C2D", id, true},
		{"sytue-bases:", "", true},
		{"sytue-bases:/bases/", "", true},
		{"https://bases.sytue.eu/bases/acme", "", true},
		{"sytue-bases:/bases/base/no-es-un-id", "", false},
		{"sytue-bases:/bases/base/" + id + "/otra", "", false},
		{"sytue-bases:/bases/base", "", true}, // una organización llamada «base»
		{"https://bases.sytue.eu/bases/base/base/" + id, id, true},
		{"https://bases.sytue.eu/bases/f/un-token-de-formulario", "", false},
		{"sytue-bases:/a/b/c/base/" + id, "", false},
	} {
		u, err := url.Parse(c.enlace)
		if err != nil {
			t.Fatal(err)
		}
		if base, ok := DestinoDeEnlace(u); base != c.base || ok != c.ok {
			t.Errorf("%s: (%q, %v); se esperaba (%q, %v)", c.enlace, base, ok, c.base, c.ok)
		}
	}
}
