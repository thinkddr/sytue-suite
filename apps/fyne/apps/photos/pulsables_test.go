// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// TestLaPestanaDeNavegacionSePulsaYSeAnunciaComoEnlace: llega a su acción con clic,
// Intro y Espacio, y se anuncia como el `<a>` que es en Nav.tsx, no como botón.
func TestLaPestanaDeNavegacionSePulsaYSeAnunciaComoEnlace(t *testing.T) {
	n := 0
	p := NuevaPestanaNav("Fotos", iconos.Imagen, false, func() { n++ })
	p.Tapped(nil)
	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	p.TypedRune(' ')
	if n != 3 {
		t.Errorf("tres pulsaciones llegan %d veces", n)
	}
	if p.AccessibilityRole() != fyne.AccessibleRoleLink || p.AccessibilityLabel() != "Fotos" {
		t.Errorf("se anuncia como %q «%s»", p.AccessibilityRole(), p.AccessibilityLabel())
	}
}
