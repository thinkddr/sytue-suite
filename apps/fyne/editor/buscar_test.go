// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import "testing"

// TestCasarEsElDeLaWeb: `matchIndices` de exec.ts — sin mayúsculas por defecto, sin
// solapes, y «palabra completa» con acentos y eñes como letras.
func TestCasarEsElDeLaWeb(t *testing.T) {
	for _, c := range []struct {
		pajar, aguja   string
		mayus, palabra bool
		quiero         int
	}{
		{"Hola hola HOLA", "hola", false, false, 3},
		{"Hola hola HOLA", "hola", true, false, 1},
		{"holas hola", "hola", false, true, 1},
		{"añoaño año", "año", false, true, 1},
		{"aaaa", "aa", false, false, 2}, // sin solapes
		{"x", "", false, false, 0},
	} {
		if got := len(Casar([]rune(c.pajar), []rune(c.aguja), c.mayus, c.palabra)); got != c.quiero {
			t.Errorf("Casar(%q, %q, mayús %v, palabra %v) = %d, quiero %d", c.pajar, c.aguja, c.mayus, c.palabra, got, c.quiero)
		}
	}
}
