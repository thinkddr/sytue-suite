// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestElCuadernoNoRepiteSuNombreBajoLaBarra: en el teléfono la barra única dice
// «Mudanza» y la franja del cuaderno lo volvía a escribir justo debajo, en grande: dos
// barras con el mismo título (auditoría estética del 2026-11-24). Por debajo de 768 la
// franja se queda con sus acciones y el nombre lo lleva solo la barra de la app; desde
// 768, donde no hay barra móvil, la franja lo conserva.
//
// VISTO EN ROJO sin `RepiteLaBarra`: a 390 se ven dos «Mudanza».
func TestElCuadernoNoRepiteSuNombreBajoLaBarra(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	p, w := montarCaptura(t, 390, 844, "columns")
	tr, err := i18n.New("es", i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	suite, parar := ui.BarraSuiteDePrueba(tr, iconos.Notes)
	t.Cleanup(parar)
	w.SetContent(suite.Montar(w.Content()))
	p.Repintar()

	if n := vistos(w, "Mudanza"); n != 1 {
		t.Errorf("a 390 se ve «Mudanza» %d veces; tocaba una, la de la barra", n)
	}
	if n := vistos(w, p.tr.T("note.new", nil)); n != 1 {
		t.Errorf("a 390 la franja pierde «Nueva nota» (%d)", n)
	}

	// La franja sola, con y sin la barra encima: el nombre y su emoji van o no van.
	b := NuevaBarra(p.tr)
	b.Cuaderno = DeCuaderno(api.Notebook{ID: "c1", Title: "Mudanza", Emoji: "🏠", Layout: "columns"})
	for _, repite := range []bool{false, true} {
		b.RepiteLaBarra = repite
		ts := textosDe(b.Objeto(tema.Claro))
		if slices.Contains(ts, "Mudanza") == repite || slices.Contains(ts, "🏠") == repite {
			t.Errorf("con RepiteLaBarra=%v la franja pinta %q", repite, ts)
		}
		if !slices.Contains(ts, p.tr.T("note.new", nil)) {
			t.Errorf("con RepiteLaBarra=%v la franja pierde «Nueva nota»: %q", repite, ts)
		}
	}

	w.Resize(fyne.NewSize(1280, 800))
	p.Repintar()
	if p.barra.RepiteLaBarra {
		t.Error("a 1280, sin barra móvil, la franja esconde el nombre del cuaderno")
	}
}

// vistos cuenta los textos `s` que se ven en la ventana: visibles con todos sus padres y
// dentro del lienzo.
func vistos(w fyne.Window, s string) int {
	n := 0
	tam := w.Canvas().Size()
	d := fyne.CurrentApp().Driver()
	var ir func(o fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		if o == nil || !o.Visible() {
			return
		}
		if x, ok := o.(*canvas.Text); ok && x.Text == s {
			if p := d.AbsolutePositionForObject(x); p.X >= 0 && p.Y >= 0 && p.X < tam.Width && p.Y < tam.Height {
				n++
			}
		}
		switch x := o.(type) {
		case *fyne.Container:
			for _, h := range x.Objects {
				ir(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				ir(h)
			}
		}
	}
	ir(w.Canvas().Content())
	return n
}
