// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestClaveDeEnlace(t *testing.T) {
	if h, err := HashClaveDeEnlace(""); h != "" || err != nil {
		t.Fatalf("vacía: %q %v", h, err)
	}
	for _, mala := range []string{"corta", strings.Repeat("x", ClaveDeEnlaceMax+1)} {
		if _, err := HashClaveDeEnlace(mala); !errors.Is(err, ErrClaveDeEnlaceInvalida) {
			t.Errorf("%d caracteres: %v", len(mala), err)
		}
	}
	h, err := HashClaveDeEnlace("contraseña buena")
	if err != nil || !strings.HasPrefix(h, "$argon2id$") || strings.Contains(h, "buena") {
		t.Fatalf("hash: %q %v", h, err)
	}
	if !VerificarClaveDeEnlace("contraseña buena", h) {
		t.Error("la contraseña correcta no verifica")
	}
	for _, mala := range []string{"", "contraseña mala", strings.Repeat("x", ClaveDeEnlaceMax+1)} {
		if VerificarClaveDeEnlace(mala, h) {
			t.Errorf("verifica %q", mala)
		}
	}
	if VerificarClaveDeEnlace("contraseña buena", "") || VerificarClaveDeEnlace("contraseña buena", "no-es-un-hash") {
		t.Error("verifica contra un hash vacío o corrupto")
	}

	p := PruebaDeClave(h)
	if !PruebaValida(p, h) {
		t.Error("la prueba del propio hash no vale")
	}
	otro, _ := HashClaveDeEnlace("contraseña buena") // misma contraseña, otra sal
	for _, c := range []struct{ prueba, hash string }{{"", h}, {p, otro}, {p + "x", h}, {h, h}, {p, ""}} {
		if PruebaValida(c.prueba, c.hash) {
			t.Errorf("vale una prueba que no es: %q", c.prueba)
		}
	}
	if strings.Contains(p, h) || p == h {
		t.Error("la prueba deja ver el hash")
	}
}
