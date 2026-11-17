// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
)

// TestElFolioCuentaElDocumentoYLaSeleccion: el recuento del documento lleva las palabras
// de la barra y los párrafos de verdad; el de la selección, solo lo seleccionado aunque
// cruce dos párrafos.
func TestElFolioCuentaElDocumentoYLaSeleccion(t *testing.T) {
	f := folioCon(t, "<h1>Título</h1><p>Uno <strong>dos</strong>, tres</p><p>cuatro cinco</p>")
	doc, sel := f.Recuento()
	if sel != nil {
		t.Fatalf("sin selección el recuento trae una: %+v", sel)
	}
	// «dos</strong>,» deja la coma suelta: 7 palabras en la barra, como en la web.
	quiero := editor.Recuento{Palabras: 7, Caracteres: 6 + 13 + 12, CaracteresSinEspacios: 6 + 11 + 11, Parrafos: 3}
	if doc != quiero {
		t.Fatalf("documento %+v, quiero %+v", doc, quiero)
	}
	if _, _, palabras := f.Estadisticas(); palabras != doc.Palabras {
		t.Fatalf("la barra dice %d palabras y el recuento %d", palabras, doc.Palabras)
	}
	if _, hay := f.PalabrasSeleccionadas(); hay {
		t.Fatal("sin selección no hay palabras seleccionadas")
	}

	// De «tres» (párrafo 1, desde la runa 9) a «cuatro» (párrafo 2, hasta la 6).
	f.anc, f.cur = punto{1, 9}, punto{2, 6}
	_, sel = f.Recuento()
	if sel == nil || *sel != (editor.Recuento{Palabras: 2, Caracteres: 10, CaracteresSinEspacios: 10, Parrafos: 2}) {
		t.Fatalf("selección %+v", sel)
	}
	if n, hay := f.PalabrasSeleccionadas(); !hay || n != 2 {
		t.Fatalf("palabras seleccionadas %d (%v), quiero 2", n, hay)
	}
}

// TestLasPalabrasDelDocumentoLeenLasEntidades: el navegador cuenta el texto con las
// entidades resueltas, y la barra nativa tiene que decir lo mismo. VISTO EN ROJO
// (2026-11-13): «d&nbsp;e» era UNA palabra en Fyne y dos en la web, y al seleccionarlo
// todo la barra decía «Palabras: 2 de 1».
func TestLasPalabrasDelDocumentoLeenLasEntidades(t *testing.T) {
	for html, quiero := range map[string]int{
		"<p>d&nbsp;e</p>":          2,
		"<p>a &amp; b</p>":         3,
		"<p>uno&#160;dos tres</p>": 3,
		"<p>x&#x3000;y</p>":        2,
	} {
		if got := Palabras(html); got != quiero {
			t.Errorf("Palabras(%q) = %d, quiero %d", html, got, quiero)
		}
	}
	f := folioCon(t, "<p>d&nbsp;e</p>")
	f.anc, f.cur = punto{0, 0}, punto{0, 3}
	doc, sel := f.Recuento()
	if sel == nil || sel.Palabras > doc.Palabras {
		t.Fatalf("toda la selección %+v dice más palabras que el documento %+v", sel, doc)
	}
}
