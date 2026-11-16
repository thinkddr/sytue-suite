// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import "testing"

// TestTextoAtajoEsElDeLaCintaWeb: `textoAtajo` de `ribbon/primitives.tsx`, con el
// catálogo en castellano fuera del Mac y los símbolos dentro.
func TestTextoAtajoEsElDeLaCintaWeb(t *testing.T) {
	es := func(k string) string {
		return map[string]string{"shortcut.ctrl": "Ctrl", "shortcut.shift": "Mayús"}[k]
	}
	for _, c := range []struct{ goos, teclas, quiero string }{
		{"linux", "Ctrl+Shift+V", "Ctrl+Mayús+V"},
		{"windows", "Shift+Espacio", "Mayús+Espacio"},
		{"linux", "Alt+Intro", "Alt+Intro"},
		{"darwin", "Ctrl+Shift+V", "⌘⇧V"},
		{"darwin", "Ctrl+Y", "⌘⇧Z"},
		{"darwin", "Alt+Intro", "⌥Intro"},
	} {
		if got := TextoAtajoPara(c.goos, c.teclas, es); got != c.quiero {
			t.Errorf("%s %q → %q, quiero %q", c.goos, c.teclas, got, c.quiero)
		}
	}
}
