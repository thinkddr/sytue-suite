// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// TestLaCintaSePulsaComoUnBoton: el botón y la pestaña de la cinta llegan a su acción
// con clic, Intro y Espacio; y un botón inerte ni se pulsa ni se enfoca, como un
// `disabled` de la web. La copia `activable` que había antes sí lo dejaba enfocar.
func TestLaCintaSePulsaComoUnBoton(t *testing.T) {
	n := 0
	boton := NuevoBotonCinta(iconos.EdRegla, "Regla", func() { n++ })
	pestana := NuevaPestanaCinta("Inicio", func() { n++ })
	for _, b := range []interface {
		fyne.Tappable
		fyne.Focusable
	}{boton, pestana} {
		b.Tapped(nil)
		b.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
		b.TypedRune(' ')
	}
	if n != 6 {
		t.Errorf("seis pulsaciones (clic, Intro y Espacio en cada uno) llegan %d veces", n)
	}
	if pestana.AccessibilityLabel() != "Inicio" {
		t.Errorf("la pestaña se anuncia como %q", pestana.AccessibilityLabel())
	}
	boton.Inerte = true
	boton.FocusGained()
	boton.Tapped(nil)
	if boton.Enfocado() || n != 6 {
		t.Errorf("un botón inerte se enfoca (%v) o se pulsa (%d)", boton.Enfocado(), n)
	}
}
