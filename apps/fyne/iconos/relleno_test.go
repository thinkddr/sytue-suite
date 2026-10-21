// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

import (
	"image/color"
	"testing"
)

// TestElRellenoNoLlevaTrazo: el play de 16 px en la web ocupa de x=7 a x=19 de su
// rejilla de 24, o sea de 4,67 a 12,67 px. Con el trazo de Icon.tsx encima se sale un
// píxel por cada lado, que es justo lo que este glifo no tiene que hacer.
func TestElRellenoNoLlevaTrazo(t *testing.T) {
	negro := color.NRGBA{A: 255}
	img := rasteriza(RellenoDeRuta("M7 5v14l12-7z", 16, negro).svg(pintura{tinta: negro, lado: 16}), 16, 16)
	tinta := func(x, y int) bool { return img.NRGBAAt(x, y).A > 128 }
	if tinta(3, 8) {
		t.Error("hay tinta en x=3, a la izquierda del borde del triángulo (x=4,67): el glifo lleva trazo")
	}
	if !tinta(6, 8) {
		t.Error("no hay tinta en x=6, dentro del triángulo: el glifo no se ha rellenado")
	}
}
