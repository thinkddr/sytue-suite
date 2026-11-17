// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestLasTeselasSeRecorrenConFlechasInicioYFin es la paridad de `navegarProducto`
// (web): con el foco en la primera tesela, la flecha derecha va a la segunda, la de
// abajo a la primera de la fila siguiente —tantas teselas más allá como columnas— y
// Fin a la última. Sin el `TypedKey` de la tesela, las flechas suben a la ventana y el
// foco no se mueve.
func TestLasTeselasSeRecorrenConFlechasInicioYFin(t *testing.T) {
	for _, caso := range []struct {
		ancho float32
		cols  int
	}{{500, 2}, {800, 3}, {1280, 5}} {
		t.Run(fmt.Sprint(caso.cols, " columnas"), func(t *testing.T) {
			l := lanzadorDePrueba(t)
			var r *lanzadorRenderer
			var w fyne.Window
			fyne.DoAndWait(func() {
				w = test.NewWindow(l)
				w.SetPadded(false)
				r = test.WidgetRenderer(l).(*lanzadorRenderer)
				l.Resize(fyne.NewSize(caso.ancho, r.alto(caso.ancho)))
				w.Resize(fyne.NewSize(caso.ancho, r.alto(caso.ancho)))
			})
			t.Cleanup(w.Close)
			ts := r.teselas
			c := w.Canvas()
			pulsa := func(k fyne.KeyName) {
				fyne.DoAndWait(func() { c.Focused().TypedKey(&fyne.KeyEvent{Name: k}) })
			}
			espera := func(tecla string, want *TeselaApp) {
				t.Helper()
				if got := c.Focused(); got != want {
					t.Errorf("tras %s el foco está en %v y debía ir a %s", tecla, got, want.Producto.Nombre)
				}
			}
			fyne.DoAndWait(func() { c.Focus(ts[0]) })
			pulsa(fyne.KeyRight)
			espera("→", ts[1])
			pulsa(fyne.KeyLeft)
			espera("←", ts[0])
			pulsa(fyne.KeyLeft) // borde: no se mueve
			espera("← en el borde", ts[0])
			pulsa(fyne.KeyDown)
			espera("↓", ts[caso.cols])
			pulsa(fyne.KeyUp)
			espera("↑", ts[0])
			pulsa(fyne.KeyEnd)
			espera("Fin", ts[len(ts)-1])
			pulsa(fyne.KeyHome)
			espera("Inicio", ts[0])
		})
	}
}
