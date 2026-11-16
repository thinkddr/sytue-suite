// SPDX-License-Identifier: Apache-2.0
package service

import (
	"image"
	"image/color"
	"testing"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

func TestEsPNG(t *testing.T) {
	casos := []struct {
		nombre string
		tipo   string
		esPng  bool
	}{
		{"foto.jpg", "image/jpeg", false},
		{"logo.png", "image/png", true},
		{"LOGO.PNG", "application/octet-stream", true},
		{"sin_extension", "image/png", true},
		{"sin_extension", "IMAGE/PNG", true},
		{"icono.svg", "image/svg+xml", false},
		{"playa.jpeg", "image/jpeg", false},
	}

	for _, c := range casos {
		a := domain.Activo{Nombre: c.nombre, Tipo: c.tipo}
		if got := EsPNG(a); got != c.esPng {
			t.Errorf("EsPNG(%+v) = %v; se esperaba %v", a, got, c.esPng)
		}
	}
}

func TestAplanarSobreBlanco(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.Set(0, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 0})
	src.Set(1, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 255})

	dst := aplanarSobreBlanco(src)

	r0, g0, b0, a0 := dst.At(0, 0).RGBA()
	if r0 != 0xffff || g0 != 0xffff || b0 != 0xffff || a0 != 0xffff {
		t.Errorf("píxel transparente aplanado = (%d, %d, %d, %d); se esperaba blanco", r0, g0, b0, a0)
	}

	r1, g1, b1, a1 := dst.At(1, 0).RGBA()
	if r1 != 0 || g1 != 0 || b1 != 0 || a1 != 0xffff {
		t.Errorf("píxel opaco aplanado = (%d, %d, %d, %d); se esperaba negro", r1, g1, b1, a1)
	}
}
