// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tokens

import (
	"image/color"
	"testing"
)

// TestAplanarYSobreSRGBSonLaMismaCuenta: las dos composiciones del paquete —con el
// alfa dentro del color o aparte— tienen que dar el MISMO byte. Si una se retoca y la
// otra no, los bordes (SobreSRGB) y lo atenuado (Aplanar) dejan de casar por un nivel.
func TestAplanarYSobreSRGBSonLaMismaCuenta(t *testing.T) {
	fondos := []color.NRGBA{{255, 255, 255, 255}, {31, 41, 55, 255}, {233, 230, 221, 255}}
	for _, fondo := range fondos {
		for a := 0; a <= 255; a += 3 {
			for v := 0; v <= 255; v += 5 {
				fg := color.NRGBA{R: uint8(v), G: uint8(255 - v), B: uint8(v / 2), A: uint8(a)}
				if s, p := SobreSRGB(fg, fondo), Aplanar(fg, float64(a)/255, fondo); s != p {
					t.Fatalf("%v sobre %v: SobreSRGB da %v y Aplanar %v", fg, fondo, s, p)
				}
			}
		}
	}
}

// TestConAlfaMultiplicaElAlfaDelToken: `border-border/60` sobre un token que ya es
// translúcido deja el 60 % de SU alfa, no un 60 % a secas.
func TestConAlfaMultiplicaElAlfaDelToken(t *testing.T) {
	for _, c := range []struct{ a, quiero uint8 }{{255, 153}, {31, 19}, {0, 0}} {
		if got := ConAlfa(color.NRGBA{R: 1, G: 2, B: 3, A: c.a}, 0.6); got != (color.NRGBA{R: 1, G: 2, B: 3, A: c.quiero}) {
			t.Errorf("ConAlfa(alfa %d, 0,6) = %v, se esperaba alfa %d y el color intacto", c.a, got, c.quiero)
		}
	}
}
