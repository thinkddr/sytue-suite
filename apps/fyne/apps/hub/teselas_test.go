// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"image"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestLaTeselaApagadaPintaLaMarcaEnGris es el `opacity-40 grayscale` de la tesela
// deshabilitada de la web: la marca sale en gris, no solo más clara. Encendida, la
// misma marca conserva su color. Sin el `Apagar` del renderer, la marca de la tesela
// apagada sale en color y el test lo dice.
func TestLaTeselaApagadaPintaLaMarcaEnGris(t *testing.T) {
	saturacion := func(apagada bool) int {
		t.Helper()
		ts := NuevaTeselaApp(Productos[1], "Correo", "Sytue Mail — Correo", nil)
		ts.Deshabilitada = apagada
		var img image.Image
		var marca image.Rectangle
		fyne.DoAndWait(func() {
			w := test.NewWindow(ts)
			t.Cleanup(w.Close)
			w.SetPadded(false)
			r := test.WidgetRenderer(ts).(*teselaRenderer)
			ts.Resize(fyne.NewSize(240, AltoTesela(1)))
			w.Resize(fyne.NewSize(240, AltoTesela(1)))
			img = w.Canvas().Capture()
			o := r.marca.Objeto()
			p, s := o.Position(), o.Size()
			marca = image.Rect(int(p.X), int(p.Y), int(p.X+s.Width), int(p.Y+s.Height))
		})
		peor := 0
		for y := marca.Min.Y; y < marca.Max.Y; y++ {
			for x := marca.Min.X; x < marca.Max.X; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				hi, lo := max(r, g, b)>>8, min(r, g, b)>>8
				peor = max(peor, int(hi-lo))
			}
		}
		return peor
	}
	if s := saturacion(false); s < 40 {
		t.Fatalf("la marca encendida de Mail no tiene color (saturación %d): el test no mide nada", s)
	}
	// Dos de margen: el redondeo de la captura y del suavizado de bordes.
	if s := saturacion(true); s > 2 {
		t.Errorf("la marca de la tesela apagada sale en color (saturación %d); va en gris, como `grayscale` en la web", s)
	}
}
