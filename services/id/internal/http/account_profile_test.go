// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import "testing"

// TestAutocompleteFor comprueba que cada campo del perfil declara su pista de
// autorrelleno: sin ella el navegador no ofrece lo que ya sabe y el formulario
// hay que teclearlo entero.
func TestAutocompleteFor(t *testing.T) {
	for _, id := range []string{"name", "given_name", "family_name", "title", "location"} {
		if autocompleteFor(id) == "off" {
			t.Errorf("el campo %q se quedó sin pista de autorrelleno", id)
		}
	}
	// Departamento no tiene token estándar en la especificación HTML; "off" es la
	// respuesta correcta, no un olvido.
	if got := autocompleteFor("department"); got != "off" {
		t.Errorf("department = %q, quiero off", got)
	}
}
