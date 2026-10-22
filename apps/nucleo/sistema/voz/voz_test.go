// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package voz

import "testing"

// TestLaVozEsLaDelIdioma: de la lista de `say -v '?'` —nombres con espacios y paréntesis
// incluidos— sale la voz de la región pedida, o si no la primera de la lengua; y ninguna
// si no hay de esa lengua (se lee con la del sistema).
func TestLaVozEsLaDelIdioma(t *testing.T) {
	const lista = `Alice               it_IT    # Ciao! Mi chiamo Alice.
Eddy (Spanish (Mexico)) es_MX    # ¡Hola! Me llamo Eddy.
Mónica              es_ES    # ¡Hola! Me llamo Mónica.
Samantha            en_US    # Hello! My name is Samantha.
`
	for _, c := range []struct{ idioma, quiero string }{
		{"es-ES", "Mónica"},
		{"es-MX", "Eddy (Spanish (Mexico))"},
		{"es", "Eddy (Spanish (Mexico))"},
		{"es_AR", "Eddy (Spanish (Mexico))"},
		{"de-DE", ""},
		{"en", "Samantha"},
	} {
		if got := vozDe(lista, c.idioma); got != c.quiero {
			t.Errorf("vozDe(%q) = %q; quiero %q", c.idioma, got, c.quiero)
		}
	}
}

// TestSinTextoNoSeHabla: un documento sin texto no arranca nada (la app ya lo dice con
// `pdf.speak.noText`).
func TestSinTextoNoSeHabla(t *testing.T) {
	callar, err := Leer(" \n\t", "es", func() { t.Error("se ha llamado a alAcabar sin leer nada") })
	if err != nil || callar == nil {
		t.Fatalf("Leer de nada: callar nil = %v, error %v", callar == nil, err)
	}
	callar()
}
