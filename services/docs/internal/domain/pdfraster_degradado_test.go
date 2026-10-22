// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"image/png"
	"testing"
)

func pixel(t *testing.T, datos []byte, x, y int) (r, g, b uint32) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(datos))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ = img.At(x, y).RGBA()
	return r >> 8, g >> 8, b >> 8
}

const shRojoAzul = `<< /ShadingType 2 /ColorSpace /DeviceRGB /Coords [0 0 100 0] /Extend [true true] ` +
	`/Function << /FunctionType 2 /Domain [0 1] /C0 [1 0 0] /C1 [0 0 1] /N 1 >> >>`

// Un degradado axial de rojo a azul, con `sh` y como patrón de relleno: a la
// izquierda rojo, a la derecha azul y en medio mitad y mitad. Antes la página
// entera salía del color medio.
func TestElPNGPintaDegradadosAxiales(t *testing.T) {
	casos := map[string][2]string{
		"sh":     {"/Sh0 sh", `/Resources << /Shading << /Sh0 ` + shRojoAzul + ` >> >>`},
		"patrón": {"/Pattern cs /P0 scn 0 0 100 100 re f", `/Resources << /Pattern << /P0 << /PatternType 2 /Shading ` + shRojoAzul + ` >> >> >>`},
	}
	for nombre, c := range casos {
		salida := rasterizar(t, pdfConContenido(t, c[0], c[1]), OpcionesRender{})
		if r, _, b := pixel(t, salida, 3, 50); r < 230 || b > 25 {
			t.Errorf("%s: a la izquierda (%d, %d) debería ser rojo", nombre, r, b)
		}
		if r, _, b := pixel(t, salida, 96, 50); b < 230 || r > 25 {
			t.Errorf("%s: a la derecha (%d, %d) debería ser azul", nombre, r, b)
		}
		if r, _, b := pixel(t, salida, 50, 50); r < 100 || r > 155 || b < 100 || b > 155 {
			t.Errorf("%s: en medio (%d, %d) debería ser mitad y mitad", nombre, r, b)
		}
	}
}

// Radial de blanco (centro) a negro (radio 40) sin extender: el centro claro, a
// 30 de él oscuro y fuera del círculo el papel.
func TestElPNGPintaDegradadosRadiales(t *testing.T) {
	sh := `<< /ShadingType 3 /ColorSpace /DeviceGray /Coords [50 50 0 50 50 40] ` +
		`/Function << /FunctionType 2 /Domain [0 1] /C0 [1] /C1 [0] /N 1 >> >>`
	salida := rasterizar(t, pdfConContenido(t, "/Sh0 sh", `/Resources << /Shading << /Sh0 `+sh+` >> >>`), OpcionesRender{})
	if v, _, _ := pixel(t, salida, 50, 50); v < 240 {
		t.Errorf("centro = %d, quería casi blanco", v)
	}
	if v, _, _ := pixel(t, salida, 50, 80); v > 80 {
		t.Errorf("a 30 del centro = %d, quería oscuro", v)
	}
	if v, _, _ := pixel(t, salida, 2, 2); v < 250 {
		t.Errorf("fuera del círculo = %d, quería el papel", v)
	}
}

// La inversa de la matriz deshace la matriz.
func TestMatrizInversa(t *testing.T) {
	m := matriz{2, 1, -1, 3, 10, -4}
	inv, ok := m.inversa()
	if !ok {
		t.Fatal("sin inversa")
	}
	if p := m.por(inv); !casiIdentidad(p) {
		t.Fatalf("m·m⁻¹ = %v", p)
	}
}

func casiIdentidad(m matriz) bool {
	for i, v := range identidad {
		if d := m[i] - v; d > 1e-9 || d < -1e-9 {
			return false
		}
	}
	return true
}
