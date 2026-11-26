// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ocrwindows

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"runtime"
	"slices"
	"testing"
)

func pngDe(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// La imagen llega al motor en BGRA8 opaca, sin relleno, y con un fondo transparente que
// es papel (blanco), no negro.
func TestLaImagenVaEnBGRA8Opaca(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	img.Set(1, 0, color.NRGBA{B: 255, A: 255})
	// (2,0) y la segunda fila, transparentes.
	m, err := paraOCR(pngDe(t, img), 2600)
	if err != nil {
		t.Fatal(err)
	}
	if m.ancho != 3 || m.alto != 2 || m.escala != 1 || len(m.bgra) != 3*2*4 {
		t.Fatalf("%d×%d escala %v, %d bytes", m.ancho, m.alto, m.escala, len(m.bgra))
	}
	if got := m.bgra[0:4]; !bytes.Equal(got, []byte{0, 0, 255, 255}) {
		t.Fatalf("rojo en BGRA: %v", got)
	}
	if got := m.bgra[4:8]; !bytes.Equal(got, []byte{255, 0, 0, 255}) {
		t.Fatalf("azul en BGRA: %v", got)
	}
	if got := m.bgra[8:12]; !bytes.Equal(got, []byte{255, 255, 255, 255}) {
		t.Fatalf("lo transparente no es blanco: %v", got)
	}
	if _, err := paraOCR([]byte("no es una imagen"), 2600); err == nil {
		t.Fatal("una imagen ilegible")
	}
}

// Una página A4 a 300 ppp pasa del lado máximo del motor: se reduce con su proporción y
// las cajas vuelven a la escala de la original.
func TestUnaPaginaGrandeSeReduceYLasCajasVuelven(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 2480, 3508))
	m, err := paraOCR(pngDe(t, src), 2600)
	if err != nil {
		t.Fatal(err)
	}
	if m.alto != 2600 || m.ancho != int(math.Round(2480*2600.0/3508)) || len(m.bgra) != m.ancho*m.alto*4 {
		t.Fatalf("%d×%d (%d bytes)", m.ancho, m.alto, len(m.bgra))
	}
	p, ok := m.aOriginal(palabraCruda{texto: " Contrato ", x: 100, y: 200, w: 300, h: 40})
	f := 3508.0 / 2600
	if !ok || p.Texto != "Contrato" || p.X != int(math.Floor(100*f)) || p.Y != int(math.Floor(200*f)) ||
		p.X+p.Ancho != int(math.Ceil(400*f)) || p.Y+p.Alto != int(math.Ceil(240*f)) {
		t.Fatalf("%+v", p)
	}
	// Lo que se sale se recorta; sin texto, sin área o con NaN, no vale.
	if p, ok := m.aOriginal(palabraCruda{texto: "x", x: 1800, y: 2590, w: 100, h: 50}); !ok || p.X+p.Ancho != 2480 || p.Y+p.Alto != 3508 {
		t.Fatalf("recorte: %+v", p)
	}
	for _, w := range []palabraCruda{{texto: " ", w: 1, h: 1}, {texto: "x", w: 0, h: 1}, {texto: "x", w: float32(math.NaN()), h: 1}} {
		if _, ok := m.aOriginal(w); ok {
			t.Errorf("%+v vale", w)
		}
	}
}

func TestIdiomasDeTesseractYDeWindows(t *testing.T) {
	for etiqueta, quiero := range map[string]string{
		"es-ES": "spa", "en-US": "eng", "de-DE": "deu", "pt-BR": "por", "nb-NO": "nor",
		"zh-Hans-CN": "chi_sim", "zh-Hant-TW": "chi_tra", "zh-TW": "chi_tra", "sr-Latn-RS": "srp_latn", "sr-Cyrl-RS": "srp",
	} {
		if got, ok := codigoTesseract(etiqueta); !ok || got != quiero {
			t.Errorf("%s → %q %v, quiero %q", etiqueta, got, ok, quiero)
		}
	}
	if _, ok := codigoTesseract("tlh-Latn"); ok {
		t.Error("un idioma que no conoce")
	}
	instaladas := []string{"en-US", "es-ES"}
	for pedido, quiero := range map[string]string{"spa+eng": "es-ES", "eng": "en-US", "deu+eng": "en-US", "deu": "", "": "", "es-ES": "es-ES"} {
		if got := elegirIdioma(pedido, instaladas); got != quiero {
			t.Errorf("%q → %q, quiero %q", pedido, got, quiero)
		}
	}
}

// Idiomas y Reconocer, con un sistema de mentira: los códigos de la pantalla, el idioma
// elegido y las cajas en la imagen de entrada.
func TestIdiomasYReconocerConUnSistemaDeMentira(t *testing.T) {
	antesP, antesR := preguntar, pasarAlMotor
	t.Cleanup(func() { preguntar, pasarAlMotor = antesP, antesR })
	preguntar = func() (consulta, error) {
		return consulta{etiquetas: []string{"es-ES", "en-US", "en-GB", "tlh-Latn"}, maxLado: 100}, nil
	}
	var etiqueta string
	var ancho, alto int
	pasarAlMotor = func(_ context.Context, bgra []byte, w, h int, e string) ([]palabraCruda, error) {
		etiqueta, ancho, alto = e, w, h
		if len(bgra) != w*h*4 {
			t.Errorf("%d bytes para %d×%d", len(bgra), w, h)
		}
		return []palabraCruda{{texto: "Hola", x: 10, y: 10, w: 20, h: 5}, {texto: "", x: 1, y: 1, w: 1, h: 1}}, nil
	}
	if !Disponible() {
		t.Fatal("con idiomas, disponible")
	}
	idiomas, err := Idiomas()
	if err != nil || !slices.Equal(idiomas, []string{"spa", "eng", "tlh-Latn"}) {
		t.Fatal(idiomas, err)
	}
	ps, err := Reconocer(t.Context(), pngDe(t, image.NewGray(image.Rect(0, 0, 400, 200))), "spa+eng")
	if err != nil || etiqueta != "es-ES" || ancho != 100 || alto != 50 {
		t.Fatal(ps, err, etiqueta, ancho, alto)
	}
	if len(ps) != 1 || ps[0] != (Palabra{Texto: "Hola", X: 40, Y: 40, Ancho: 80, Alto: 20}) {
		t.Fatalf("%+v", ps)
	}

	// Sin idiomas instalados no se ofrece, y un fallo del sistema se dice tal cual.
	preguntar = func() (consulta, error) { return consulta{}, nil }
	if Disponible() {
		t.Fatal("sin idiomas, disponible")
	}
	if _, err := Idiomas(); !errors.Is(err, ErrNoDisponible) {
		t.Fatal(err)
	}
	roto := errors.New("roto")
	preguntar = func() (consulta, error) { return consulta{etiquetas: []string{"es-ES"}}, nil }
	pasarAlMotor = func(context.Context, []byte, int, int, string) ([]palabraCruda, error) { return nil, roto }
	if _, err := Reconocer(t.Context(), pngDe(t, image.NewGray(image.Rect(0, 0, 4, 4))), ""); !errors.Is(err, roto) {
		t.Fatal(err)
	}
}

// Fuera de Windows no hay nada que ofrecer.
func TestFueraDeWindowsNoEsta(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("en Windows sí está")
	}
	if Disponible() {
		t.Fatal("disponible fuera de Windows")
	}
	if _, err := Reconocer(t.Context(), nil, ""); !errors.Is(err, ErrNoDisponible) {
		t.Fatal(err)
	}
}
