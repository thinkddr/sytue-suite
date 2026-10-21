// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import "testing"

// Buscar en Rooms no mira acentos ni mayúsculas, en los dos sentidos.
func TestRoomsBuscaSinAcentos(t *testing.T) {
	gente := []Miembro{{ID: "u1", Nombre: "José Müller"}, {ID: "u2", Nombre: "Ana"}}
	for _, q := range []string{"jose", "MULLER", "Jösé"} {
		if got := FiltrarMiembros(gente, q); len(got) != 1 || got[0].ID != "u1" {
			t.Errorf("FiltrarMiembros(%q) = %v", q, got)
		}
	}
	if Normalizar("  Política ") != "politica" {
		t.Errorf("Normalizar = %q", Normalizar("  Política "))
	}
}
