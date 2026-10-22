// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElParrafoResaltaLoBuscado: los términos de la consulta llevan su fondo detrás, sin
// acentos y en el sitio del texto; sin consulta no hay ninguno.
func TestElParrafoResaltaLoBuscado(t *testing.T) {
	fondo := color.NRGBA{R: 0xff, G: 0xf3, B: 0xa3, A: 0xff}
	p := NuevoParrafo("Acta de la reunión", tema.TextoXS, Normal, "text", tema.Claro).A(400).Resaltando("reunion acta", fondo)
	p.Resize(fyne.NewSize(400, 40))
	r := test.TempWidgetRenderer(t, p)
	var fondos []*canvas.Rectangle
	for _, o := range r.Objects() {
		if rc, ok := o.(*canvas.Rectangle); ok && rc.FillColor == fondo {
			fondos = append(fondos, rc)
		}
	}
	if len(fondos) != 2 {
		t.Fatalf("hay %d fondos, quiero 2 (acta y reunión)", len(fondos))
	}
	if fondos[0].Position().X != 0 || fondos[1].Position().X <= fondos[0].Size().Width {
		t.Errorf("los fondos no están donde su texto: %v %v", fondos[0].Position(), fondos[1].Position())
	}
	quiero := Medir("reunión", tema.TextoXS, Normal).Width
	if w := fondos[1].Size().Width; w < quiero-0.5 || w > quiero+0.5 {
		t.Errorf("el fondo de «reunión» mide %v, quiero %v", w, quiero)
	}
	sin := NuevoParrafo("Acta", tema.TextoXS, Normal, "text", tema.Claro).A(400)
	for _, o := range test.TempWidgetRenderer(t, sin).Objects() {
		if _, ok := o.(*canvas.Rectangle); ok {
			t.Fatal("sin consulta no debe haber fondos")
		}
	}
}
