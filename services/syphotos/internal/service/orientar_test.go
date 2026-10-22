// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"image"
	"image/color"
	"testing"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// Una imagen 2×1 con una marca en la esquina superior izquierda es suficiente
// para distinguir las ocho orientaciones: lo que las diferencia es DÓNDE acaba
// esa esquina, y con más píxeles el test deja de leerse.
//
//	guardado:  A B
//	           C D
func patron() image.Image {
	m := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	m.Set(0, 0, color.NRGBA{R: 255, A: 255})         // A rojo
	m.Set(1, 0, color.NRGBA{G: 255, A: 255})         // B verde
	m.Set(0, 1, color.NRGBA{B: 255, A: 255})         // C azul
	m.Set(1, 1, color.NRGBA{R: 255, G: 255, A: 255}) // D amarillo
	return m
}

func nombreDe(c color.Color) string {
	r, g, b, _ := c.RGBA()
	switch {
	case r > 0x8000 && g > 0x8000:
		return "D"
	case r > 0x8000:
		return "A"
	case g > 0x8000:
		return "B"
	case b > 0x8000:
		return "C"
	}
	return "?"
}

// rejilla devuelve el contenido como texto, para poder afirmar sobre él.
func rejilla(img image.Image) string {
	b := img.Bounds()
	out := ""
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out += nombreDe(img.At(x, y))
		}
		if y < b.Max.Y-1 {
			out += "/"
		}
	}
	return out
}

// LAS OCHO, contra la tabla del estándar. Son ocho casos fijos desde 1992 y de
// memoria se confunden justo los cuatro con espejo — que es la mitad que nadie
// prueba porque casi ninguna cámara los produce.
func TestOrientar_lasOchoDelEstandar(t *testing.T) {
	casos := map[domain.Orientacion]string{
		1: "AB/CD", // como está
		2: "BA/DC", // espejo horizontal
		3: "DC/BA", // media vuelta
		4: "CD/AB", // espejo vertical
		5: "AC/BD", // espejo + un cuarto (transpuesta)
		6: "CA/DB", // un cuarto horario
		7: "DB/CA", // espejo + tres cuartos
		8: "BD/AC", // tres cuartos horarios
	}
	for o, esperado := range casos {
		got := rejilla(orientar(patron(), o))
		if got != esperado {
			t.Errorf("orientación %d: salió %q, se esperaba %q", o, got, esperado)
		}
	}
}

// EL ESPEJO VA ANTES DEL GIRO. Invertir el orden da el resultado correcto para
// 1, 3, 6 y 8 y equivocado para 2, 4, 5 y 7: por eso el test de arriba lleva
// las ocho y no una muestra.
func TestOrientar_elEspejoVaAntesDelGiro(t *testing.T) {
	// La 5 es transponer. Si el espejo se aplicara DESPUÉS del giro, saldría
	// "DB/CA" —que es la 7— en vez de "AC/BD".
	if got := rejilla(orientar(patron(), 5)); got != "AC/BD" {
		t.Errorf("la orientación 5 salió %q; con el orden invertido saldría «DB/CA»", got)
	}
}

// Las orientaciones con cuarto de vuelta INTERCAMBIAN los lados. Reservar el
// lienzo con el ancho equivocado deja la cuadrícula llena de huecos que saltan.
func TestOrientar_intercambiaLosLados(t *testing.T) {
	ancho := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for _, o := range []domain.Orientacion{5, 6, 7, 8} {
		b := orientar(ancho, o).Bounds()
		if b.Dx() != 2 || b.Dy() != 4 {
			t.Errorf("orientación %d: 4×2 quedó %dx%d, se esperaba 2×4", o, b.Dx(), b.Dy())
		}
	}
	for _, o := range []domain.Orientacion{1, 2, 3, 4} {
		b := orientar(ancho, o).Bounds()
		if b.Dx() != 4 || b.Dy() != 2 {
			t.Errorf("orientación %d: 4×2 quedó %dx%d", o, b.Dx(), b.Dy())
		}
	}
}

// Sin rotación no se copia: es el caso más frecuente con diferencia y copiar
// una imagen de 24 megapíxeles para no cambiarla es puro desperdicio.
func TestOrientar_sinRotacionDevuelveElMismo(t *testing.T) {
	src := patron()
	for _, o := range []domain.Orientacion{0, 1, 9, 200} {
		if got := orientar(src, o); got != src {
			t.Errorf("orientación %d copió la imagen sin necesidad", o)
		}
	}
}
