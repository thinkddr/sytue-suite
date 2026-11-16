// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
)

// pintaBarra pinta una barra roja de 20 % × 10 % centrada en el escenario, con el giro
// que se le pida, y devuelve la caja de píxeles donde sale el rojo.
func pintaBarra(t *testing.T, rot string) (w, h, cx, cy int) {
	t.Helper()
	m, err := Leer([]byte(`{"slides":[{"id":"s","boxes":[{"id":"b","x":40,"y":45,"w":20,"h":10,` +
		`"kind":"shape","shape":"rect","fill":"#FF0000","stroke":"transparent","html":""` + rot + `}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	l := NuevoLienzo(m, 0)
	l.Resize(fyne.NewSize(400, 400))
	img := pinta(t, 400, 400, l)
	guarda(t, "diapos-barra"+map[bool]string{true: "-girada", false: ""}[rot != ""]+".png", img)
	x0, x1, y0, y1 := caja(img, color.NRGBA{R: 255, A: 255})
	if x1 < 0 {
		t.Fatalf("no se pintó la barra (rot %q)", rot)
	}
	return x1 - x0 + 1, y1 - y0 + 1, (x0 + x1) / 2, (y0 + y1) / 2
}

// Una caja con `rot` se pinta girada sobre su centro, como la `rotate` de la web: la
// barra apaisada girada 90° sale de pie, con el mismo centro y las medidas cambiadas.
func TestLaCajaGiradaSePintaGiradaSobreSuCentro(t *testing.T) {
	w0, h0, cx0, cy0 := pintaBarra(t, "")
	if w0 <= h0 {
		t.Fatalf("la barra sin girar mide %dx%d: tenía que ser apaisada", w0, h0)
	}
	w, h, cx, cy := pintaBarra(t, `,"rot":90`)
	if h <= w {
		t.Errorf("girada 90° mide %dx%d: tenía que salir de pie", w, h)
	}
	if d := w - h0; d < -3 || d > 3 {
		t.Errorf("girada, el ancho %d tenía que ser el alto de antes (%d)", w, h0)
	}
	if d := h - w0; d < -3 || d > 3 {
		t.Errorf("girada, el alto %d tenía que ser el ancho de antes (%d)", h, w0)
	}
	if dx, dy := cx-cx0, cy-cy0; dx < -2 || dx > 2 || dy < -2 || dy > 2 {
		t.Errorf("el centro se movió de (%d,%d) a (%d,%d)", cx0, cy0, cx, cy)
	}
}

// El giro se conserva al releer y volver a guardar: sin el campo, un documento con una
// caja girada en la web se abriría de solo lectura (Fiel).
func TestElGiroSeConservaAlGuardar(t *testing.T) {
	crudo := []byte(`{"slides":[{"id":"s","boxes":[{"id":"b","x":1,"y":2,"w":3,"h":4,"html":"","rot":30}]}]}`)
	m, err := Leer(crudo)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Fiel(crudo) {
		t.Errorf("no es fiel: %v", m.NoFiel(crudo))
	}
	if g := GiroDe(m.Diapos[0].Cajas[0]); g != 30 {
		t.Errorf("GiroDe = %v, quiero 30", g)
	}
	for _, c := range []struct{ in, want float64 }{{-90, 270}, {720, 0}, {359.9999, 0}} {
		if g := GiroDe(Caja{Giro: c.in}); g != c.want {
			t.Errorf("GiroDe(%v) = %v, quiero %v", c.in, g, c.want)
		}
	}
}

// Captura para mirar con los ojos: un título girado 30° y una imagen de forma girada.
func TestCapturaDeCajasGiradas(t *testing.T) {
	m, err := Leer([]byte(`{"slides":[{"id":"s","boxes":[` +
		`{"id":"t","x":10,"y":30,"w":45,"h":20,"html":"<h1>Girada 30°</h1>","rot":30},` +
		`{"id":"f","x":60,"y":30,"w":25,"h":30,"kind":"shape","shape":"ellipse","fill":"#2563EB","html":"<p>Forma</p>","rot":315}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	l := NuevoLienzo(m, 0)
	l.Resize(fyne.NewSize(600, 400))
	img := pinta(t, 600, 400, l)
	guarda(t, "diapos-cajas-giradas.png", img)
	if cuentaTinta(img, color.NRGBA{R: 255, G: 255, B: 255, A: 255}) == 0 {
		t.Fatal("no se pintó nada")
	}
}
