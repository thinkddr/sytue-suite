// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"sync"
	"testing"
)

func estadoCon(t *testing.T, k Kind, contenido string) *State {
	t.Helper()
	op, err := ContentOp(k, contenido)
	if err != nil {
		t.Fatal(err)
	}
	st := NewState()
	if _, err := st.Apply(op); err != nil {
		t.Fatal(err)
	}
	return st
}

func miniaturaDe(t *testing.T, k Kind, contenido string) image.Image {
	t.Helper()
	raw, err := Miniatura(k, estadoCon(t, k, contenido))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("la miniatura no es un PNG: %v", err)
	}
	return img
}

func jsonMini(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// rgb lee un píxel como RRGGBB sin alfa.
func rgb(img image.Image, x, y int) [3]uint8 {
	c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	return [3]uint8{c.R, c.G, c.B}
}

// tintaEn cuenta los píxeles de la zona que NO son del color de fondo dado: es como se
// ve que hay texto escrito sin depender de la forma exacta de cada glifo.
func tintaEn(img image.Image, r image.Rectangle, fondo [3]uint8) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if rgb(img, x, y) != fondo {
				n++
			}
		}
	}
	return n
}

func pngRojo(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.NRGBA{R: 220, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

var blanco = [3]uint8{255, 255, 255}

// La miniatura de un documento es la franja superior de su primera página, con su texto:
// el encabezado se ve arriba y lo de la página tres no pinta nada.
func TestMiniaturaDeUnDocumentoEnseñaSuPrimeraPagina(t *testing.T) {
	img := miniaturaDe(t, KindDocument, `<h1>Informe trimestral</h1><p>Ventas y gastos del trimestre.</p>`)
	if b := img.Bounds(); b.Dx() != MiniaturaAncho || b.Dy() != miniaturaAltoDocumento {
		t.Fatalf("tamaño %v, se esperaba %d×%d", b.Size(), MiniaturaAncho, miniaturaAltoDocumento)
	}
	if rgb(img, 2, 2) != blanco {
		t.Errorf("el folio debe ser blanco, es %v", rgb(img, 2, 2))
	}
	if n := tintaEn(img, image.Rect(0, 0, MiniaturaAncho, 120), blanco); n < 200 {
		t.Errorf("arriba de la página no hay texto (%d píxeles con tinta)", n)
	}
	vacio := miniaturaDe(t, KindDocument, ``)
	if n := tintaEn(vacio, vacio.Bounds(), blanco); n != 0 {
		t.Errorf("un documento vacío es un folio en blanco, y tiene %d píxeles con tinta", n)
	}
}

// La de una hoja es su rejilla: cabeceras de columna y fila, el relleno de una celda y
// su texto en su sitio, empezando en A1 aunque el contenido empiece más abajo.
func TestMiniaturaDeUnaHojaEsSuRejilla(t *testing.T) {
	modelo := map[string]any{"active": 0, "sheets": []map[string]any{{
		"name":   "Hoja1",
		"cells":  map[string]string{"B2": "<b>Total</b>", "C3": "=2+3"},
		"styles": map[string]any{"B2": map[string]any{"fill": "#00AA00"}},
	}}}
	img := miniaturaDe(t, KindSpreadsheet, jsonMini(t, modelo))
	k := hojaEscala
	cab := [3]uint8{hojaCabecera.R, hojaCabecera.G, hojaCabecera.B}
	if got := rgb(img, 200, 3); got != cab {
		t.Errorf("la fila de cabeceras debe ser %v, es %v", cab, got)
	}
	// B2: de x = 40+96 a 40+192 (px del editor) y de y = 24+24 a 24+48, a escala.
	b2 := image.Rect(int((40+96)*k)+2, int(48*k)+2, int((40+192)*k)-2, int(72*k)-2)
	verde := 0
	for y := b2.Min.Y; y < b2.Max.Y; y++ {
		for x := b2.Min.X; x < b2.Max.X; x++ {
			if rgb(img, x, y) == [3]uint8{0, 0xAA, 0} {
				verde++
			}
		}
	}
	if verde < b2.Dx()*b2.Dy()/2 {
		t.Errorf("B2 debe salir con su relleno verde (%d de %d píxeles)", verde, b2.Dx()*b2.Dy())
	}
	if n := tintaEn(img, b2, [3]uint8{0, 0xAA, 0}); n < 20 {
		t.Errorf("B2 no tiene su texto (%d píxeles)", n)
	}
	// C3 enseña el VALOR de la fórmula, alineado a la derecha como un número.
	c3 := image.Rect(int((40+192)*k)+2, int(72*k)+2, int((40+288)*k)-2, int(96*k)-2)
	mitad := c3.Min.X + c3.Dx()/2
	izq := tintaEn(img, image.Rect(c3.Min.X, c3.Min.Y, mitad, c3.Max.Y), blanco)
	der := tintaEn(img, image.Rect(mitad, c3.Min.Y, c3.Max.X, c3.Max.Y), blanco)
	if der < 10 || izq > der/4 {
		t.Errorf("el 5 de =2+3 debe ir a la derecha de C3 (izquierda %d, derecha %d)", izq, der)
	}
}

// La de una presentación es su primera diapositiva: el fondo del tema, las formas con su
// color, las imágenes embebidas y el texto (también el griego, que la letra de la marca
// no trae y pone la de reserva).
func TestMiniaturaDeUnaPresentacionEsSuPrimeraDiapositiva(t *testing.T) {
	modelo := map[string]any{"theme": "carbon", "slides": []map[string]any{
		{"id": "a", "boxes": []map[string]any{
			{"id": "t", "x": 5, "y": 5, "w": 90, "h": 30, "kind": "text", "html": "<h1>Καλημέρα κόσμε</h1>"},
			{"id": "f", "x": 0, "y": 60, "w": 20, "h": 30, "kind": "shape", "shape": "rect", "fill": "#2563EB", "html": ""},
			{"id": "i", "x": 50, "y": 60, "w": 20, "h": 30, "kind": "image", "html": "", "src": pngRojo(t, 4, 4)},
		}},
		{"id": "b", "bg": "#FFFFFF", "boxes": []map[string]any{}},
	}}
	img := miniaturaDe(t, KindPresentation, jsonMini(t, modelo))
	b := img.Bounds()
	if b.Dx() != MiniaturaAncho || b.Dy() != 270 {
		t.Fatalf("una diapositiva 16:9 mide %d×270, y esta %v", MiniaturaAncho, b.Size())
	}
	carbon := [3]uint8{0x0A, 0x0A, 0x0A}
	if got := rgb(img, b.Dx()-3, b.Dy()-3); got != carbon {
		t.Errorf("el fondo debe ser el del tema Carbón, es %v", got)
	}
	if got := rgb(img, 40, 200); got != [3]uint8{0x25, 0x63, 0xEB} {
		t.Errorf("la forma debe salir azul, es %v", got)
	}
	if got := rgb(img, 280, 200); got != [3]uint8{220, 0, 0} {
		t.Errorf("la imagen debe salir roja, es %v", got)
	}
	if n := tintaEn(img, image.Rect(24, 13, 456, 94), carbon); n < 300 {
		t.Errorf("el título en griego no se ha escrito (%d píxeles)", n)
	}
}

// Una imagen que declara cincuenta mil por cincuenta mil píxeles no se decodifica: la
// caja sale como marcador y el servidor no reserva 10 GB.
func TestMiniaturaNoDecodificaUnaImagenDescomunal(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	raw := b.Bytes()
	// IHDR: ancho y alto en los bytes 16-23. Se cambian sin recalcular el CRC, que
	// DecodeConfig no comprueba: la cabecera miente y la defensa tiene que estar antes.
	copy(raw[16:24], []byte{0, 0, 0xC3, 0x50, 0, 0, 0xC3, 0x50})
	src := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	img := lienzoMiniatura(100, 100, hojaFondo)
	if dibujarImagen(img, src, 0, 0, 100, 100) {
		t.Fatal("se aceptó una imagen de 50 000 × 50 000 píxeles")
	}
}

// El servicio dibuja dos miniaturas a la vez (http/miniaturas.go) y las caras de letra
// se comparten entre llamadas; una cara de opentype no admite uso concurrente, así que
// dos hojas o diapositivas a la vez no pueden pisarse el búfer del glifo (con -race).
func TestMiniaturasALaVezNoSePisanLaLetra(t *testing.T) {
	hoja := estadoCon(t, KindSpreadsheet, jsonMini(t, map[string]any{"active": 0, "sheets": []map[string]any{{
		"name": "Hoja1", "cells": map[string]string{"A1": "Presupuesto mensual", "B2": "1234,5"},
	}}}))
	diapo := estadoCon(t, KindPresentation, jsonMini(t, map[string]any{"slides": []map[string]any{
		{"id": "a", "boxes": []map[string]any{
			{"id": "t", "x": 5, "y": 5, "w": 90, "h": 30, "kind": "text", "html": "<h1>Título y más texto</h1>"},
		}},
	}}))
	quieta, err := Miniatura(KindSpreadsheet, hoja)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			k, st := KindSpreadsheet, hoja
			if i%2 == 1 {
				k, st = KindPresentation, diapo
			}
			for range 3 {
				raw, err := Miniatura(k, st)
				if err == nil && k == KindSpreadsheet && !bytes.Equal(raw, quieta) {
					err = errors.New("una hoja dibujada a la vez que otras sale distinta")
				}
				if err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestMiniaturaEsDeterministaYRechazaOtrosTipos(t *testing.T) {
	st := estadoCon(t, KindDocument, `<p>Hola</p>`)
	a, err := Miniatura(KindDocument, st)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Miniatura(KindDocument, st)
	if !bytes.Equal(a, b) {
		t.Error("el mismo estado debe dar los mismos bytes")
	}
	if _, err := Miniatura(Kind("pdf"), st); !errors.Is(err, ErrMiniaturaTipo) {
		t.Errorf("un tipo sin miniatura debe dar ErrMiniaturaTipo, da %v", err)
	}
}

// Crear con contenido (una plantilla, una importación) programa la miniatura; crear un
// documento vacío, no: no hay nada que dibujar y la portada pinta el glifo.
func TestCrearConContenidoProgramaLaMiniatura(t *testing.T) {
	svc := NewService(&fakeStore{})
	var docs []string
	svc.AlGuardar = func(_ context.Context, id string) { docs = append(docs, id) }
	subj := Subject{Sub: "ada", Tenant: "acme"}
	if _, _, err := svc.Create(context.Background(), subj, KindDocument, "Vacío", ""); err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Fatalf("un documento vacío no tiene miniatura que dibujar: %v", docs)
	}
	op, _ := ContentOp(KindDocument, "<p>Hola</p>")
	if _, _, err := svc.CreateSeeded(context.Background(), subj, KindDocument, "Con texto", "", []Op{op}); err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0] != "new" {
		t.Fatalf("crear con contenido debe programar su miniatura: %v", docs)
	}
}
