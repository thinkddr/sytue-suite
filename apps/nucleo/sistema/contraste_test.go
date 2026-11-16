// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

import "testing"

// TestFijarMasContrasteManda: lo fijado es lo que responde MasContraste, sin que la
// consulta al sistema lo pise, y restaurar devuelve lo de antes.
func TestFijarMasContrasteManda(t *testing.T) {
	restaurar := FijarMasContraste(true)
	if !MasContraste() {
		t.Fatal("fijado a sí, MasContraste dice que no")
	}
	volver := FijarMasContraste(false)
	if MasContraste() {
		t.Fatal("fijado a no, MasContraste dice que sí")
	}
	volver()
	if !MasContraste() {
		t.Error("restaurar no ha devuelto el «sí» de antes")
	}
	restaurar()
}
