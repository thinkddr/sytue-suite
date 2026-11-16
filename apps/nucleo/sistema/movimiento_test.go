// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

import "testing"

// TestFijarMenosMovimientoManda: lo fijado es lo que responde MenosMovimiento, sin que
// la consulta al sistema lo pise, y restaurar devuelve lo de antes. Es el punto por el
// que las pruebas de Fyne dejan de depender del ajuste de la máquina.
func TestFijarMenosMovimientoManda(t *testing.T) {
	restaurar := FijarMenosMovimiento(true)
	if !MenosMovimiento() {
		t.Fatal("fijado a sí, MenosMovimiento dice que no")
	}
	volver := FijarMenosMovimiento(false)
	if MenosMovimiento() {
		t.Fatal("fijado a no, MenosMovimiento dice que sí")
	}
	volver()
	if !MenosMovimiento() {
		t.Error("restaurar no ha devuelto el «sí» de antes")
	}
	restaurar()
}
