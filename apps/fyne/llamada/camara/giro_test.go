// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package camara

import "testing"

// TestElFotogramaSaleDerecho: un sensor trasero montado a 90 grados con el teléfono de
// pie se gira 90; la delantera a 270, también 270 —y con el teléfono tumbado, lo que
// dice Camera2—. Y girar mueve cada píxel adonde toca: la esquina de arriba a la
// izquierda de lo que ve el sensor acaba arriba a la derecha tras 90 grados.
//
// VISTO EN ROJO con la delantera con el signo de la trasera (el directo sale bocabajo
// con el teléfono tumbado) y con `girar` devolviendo la imagen tal cual.
func TestElFotogramaSaleDerecho(t *testing.T) {
	for _, c := range []struct {
		sensor    int
		delantera bool
		pantalla  int
		quiero    int
	}{
		{90, false, 0, 90},
		{90, false, 90, 0},
		{90, false, 270, 180},
		{270, true, 0, 270},
		{270, true, 90, 0},
		{270, true, 270, 180},
		{0, false, 0, 0},
	} {
		if got := giroPara(c.sensor, c.delantera, c.pantalla); got != c.quiero {
			t.Errorf("sensor %d delantera %v pantalla %d: gira %d y toca %d", c.sensor, c.delantera, c.pantalla, got, c.quiero)
		}
	}

	img := I420(4, 2)
	img.Y[0], img.Cb[0] = 200, 50 // arriba a la izquierda
	for _, c := range []struct {
		grados, w, h, x, y int
	}{{90, 2, 4, 1, 0}, {180, 4, 2, 3, 1}, {270, 2, 4, 0, 3}, {0, 4, 2, 0, 0}} {
		g := girar(img, c.grados)
		if g.Rect.Dx() != c.w || g.Rect.Dy() != c.h {
			t.Fatalf("a %d grados mide %v", c.grados, g.Rect)
		}
		if g.Y[c.y*g.YStride+c.x] != 200 {
			t.Errorf("a %d grados la esquina no está en (%d,%d)", c.grados, c.x, c.y)
		}
		if g.Cb[(c.y/2)*g.CStride+c.x/2] != 50 {
			t.Errorf("a %d grados el croma no acompaña a su luma", c.grados)
		}
	}
}
