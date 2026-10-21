// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tokens

import (
	"image/color"
	"testing"
)

// Vectores de source-over CSS: border al 12% sobre card-blue al 16% o 22%.
// Las esperas son literales, independientes del helper; un nivel admite convertir
// los alfa decimales de CSS a los bytes que almacena image/color.NRGBA.
func TestSobreConAlfaConservaLaTransparenciaDeDosCapas(t *testing.T) {
	for _, caso := range []struct {
		fg, bg, css color.NRGBA
	}{
		{color.NRGBA{R: 16, G: 20, B: 26, A: 31}, color.NRGBA{R: 36, G: 87, B: 232, A: 41}, color.NRGBA{R: 27, G: 56, B: 137, A: 67}},
		{color.NRGBA{R: 240, G: 240, B: 240, A: 31}, color.NRGBA{R: 36, G: 87, B: 232, A: 56}, color.NRGBA{R: 114, G: 146, B: 235, A: 80}},
		{color.NRGBA{R: 200, G: 10, B: 20, A: 128}, color.NRGBA{R: 100, G: 100, B: 100, A: 255}, color.NRGBA{R: 150, G: 55, B: 60, A: 255}},
		{color.NRGBA{R: 3, G: 4, B: 5}, color.NRGBA{R: 100, G: 120, B: 140, A: 41}, color.NRGBA{R: 100, G: 120, B: 140, A: 41}},
		{color.NRGBA{R: 200, G: 10, B: 20, A: 255}, color.NRGBA{R: 100, G: 120, B: 140, A: 41}, color.NRGBA{R: 200, G: 10, B: 20, A: 255}},
		{color.NRGBA{}, color.NRGBA{}, color.NRGBA{}},
	} {
		got := SobreConAlfa(caso.fg, caso.bg)
		d := func(a, b uint8) int { return max(int(a)-int(b), int(b)-int(a)) }
		if d(got.R, caso.css.R) > 1 || d(got.G, caso.css.G) > 1 || d(got.B, caso.css.B) > 1 || got.A != caso.css.A {
			t.Errorf("%v sobre %v = %v, CSS = %v", caso.fg, caso.bg, got, caso.css)
		}
		if caso.bg.A == 255 && got != SobreSRGB(caso.fg, caso.bg) {
			t.Error("el fondo opaco cambió la composición que ya usaba la suite")
		}
	}
}
