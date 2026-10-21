// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaBarraDeProgresoPintaLoHecho: `h-1.5` (5,25 px) declarado; a 200 de ancho y al
// 25 %, el relleno `primary` llega a x = 50 y el resto es el carril `border`; con
// Peligro, el relleno es `danger`. Comprobado en píxeles a ×2.
//
// Visto en ROJO pintando el relleno a todo el ancho: el píxel de x = 150 sale en
// `primary`.
func TestLaBarraDeProgresoPintaLoHecho(t *testing.T) {
	if got := MedidaBarraProgreso().Height; got != 6 {
		t.Fatalf("la barra declara %v de alto y `h-1.5` son 5,25", got)
	}
	b := NuevaBarraProgreso("Subiendo prueba.mp4", 0.25)
	w := test.NewWindow(b)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	if c, ok := w.Canvas().(interface{ SetScale(float32) }); ok {
		c.SetScale(2)
	}
	w.Resize(fyne.NewSize(200, 5.25))
	img := w.Canvas().Capture()
	px := func(x int) color.NRGBA {
		r, g, bl, a := img.At(x*2, 5).RGBA()
		return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: uint8(a >> 8)}
	}
	cerca := func(a, b color.NRGBA) bool {
		d := func(x, y uint8) bool { return int(x)-int(y) > 3 || int(y)-int(x) > 3 }
		return !d(a.R, b.R) && !d(a.G, b.G) && !d(a.B, b.B)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if got, quiero := px(25), tema.Color("primary", v); !cerca(got, quiero) {
		t.Errorf("lo hecho pinta %v y `primary` es %v", got, quiero)
	}
	if got, quiero := px(150), tema.Color("border", v); !cerca(got, quiero) {
		t.Errorf("lo que falta pinta %v y el carril `border` es %v", got, quiero)
	}
	if got := b.AccessibilityLabel(); got != "Subiendo prueba.mp4 25 %" {
		t.Errorf("el nombre accesible es %q", got)
	}
	b.Peligro = true
	b.Refresh()
	img = w.Canvas().Capture()
	if got, quiero := px(25), tema.Color("danger", v); !cerca(got, quiero) {
		t.Errorf("con Peligro lo hecho pinta %v y `danger` es %v", got, quiero)
	}
}
