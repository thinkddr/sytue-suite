// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaMarcaEsLaDeChromium: la casilla marcada, pintada a 8 aumentos, tiene la marca de
// Chromium medida a esos mismos aumentos (oscuro): el tramo oscuro de la fila 40 va de
// 57 a 77 (el brazo derecho, 2,1 de grosor), la 60 junta los dos brazos (18 a 61) y la
// punta de abajo queda en la 84. Con la marca del catálogo a 11 el brazo medía la mitad.
func TestLaMarcaEsLaDeChromium(t *testing.T) {
	tema.Elegir("oscuro")
	defer tema.Elegir("")
	c := nuevaCasillaNativa("", true, nil)
	c.Resize(fyne.NewSize(ladoCasillaNativa, ladoCasillaNativa))
	lienzo := software.NewCanvas()
	lienzo.SetPadded(false)
	lienzo.SetContent(container.NewWithoutLayout(c))
	lienzo.SetScale(8)
	lienzo.Resize(fyne.NewSize(ladoCasillaNativa, ladoCasillaNativa))
	img := lienzo.Capture()
	tramo := func(y int) (int, int) {
		a, b := -1, -1
		for x := range img.Bounds().Dx() {
			r, g, bl, _ := img.At(x, y).RGBA()
			if (r+g+bl)/3>>8 < 110 {
				if a < 0 {
					a = x
				}
				b = x
			}
		}
		return a, b
	}
	for _, f := range []struct{ y, a, b int }{{40, 57, 77}, {60, 18, 61}, {76, 34, 48}} {
		a, b := tramo(f.y)
		if a < f.a-3 || a > f.a+3 || b < f.b-3 || b > f.b+3 {
			t.Errorf("fila %d: la marca va de %d a %d; en Chromium, de %d a %d", f.y, a, b, f.a, f.b)
		}
	}
}
