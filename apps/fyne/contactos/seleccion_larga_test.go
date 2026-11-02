// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"testing"

	"fyne.io/fyne/v2"
)

func TestMantenerContactoSeleccionaSinAbrirLaFicha(t *testing.T) {
	f := NuevaFilaContacto()
	abiertas, marcadas := 0, 0
	f.AlAbrir = func() { abiertas++ }
	f.AlMarcar = func() { marcadas++ }
	f.TappedSecondary(nil)
	if abiertas != 0 || marcadas != 1 {
		t.Fatalf("la pulsación larga abre %d fichas y marca %d contactos", abiertas, marcadas)
	}
	f.Tapped(&fyne.PointEvent{Position: fyne.NewPos(150, 20)})
	if abiertas != 1 || marcadas != 1 {
		t.Fatal("el toque normal ha cambiado de función")
	}
}
