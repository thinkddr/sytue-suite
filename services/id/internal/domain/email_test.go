// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// El campo de acceso llega con el correo REPETIDO cuando un gestor de contraseñas
// rellena dos veces sin limpiar lo anterior. Ese valor no casa con ninguna cuenta,
// así que sin normalizarlo el acceso falla con «email o contraseña incorrectos»
// —con la contraseña correcta— y manda a buscar el problema donde no está.
func TestNormalizeEmail(t *testing.T) {
	casos := map[string]string{
		"admin@sytue.eu admin@sytue.eu": "admin@sytue.eu",
		"  Daniel.Delgado@Sytue.com  ":                      "admin@sytue.eu",
		"a@b.com\ta@b.com":                                  "a@b.com",
		"a@b.com\na@b.com":                                  "a@b.com",
		"a@b.com":                                           "a@b.com",
		"   ":                                               "",
		"":                                                  "",
	}
	for in, want := range casos {
		if got := NormalizeEmail(in); got != want {
			t.Errorf("NormalizeEmail(%q) = %q, quiero %q", in, got, want)
		}
	}
}
