// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las opciones que no caben se cuentan, sin invadir la columna siguiente. Al
// ampliar la rejilla vuelven a verse: el resumen no pierde los valores originales.
func TestLosChipsNoInvadenLaColumnaSiguiente(t *testing.T) {
	g := nuevaRejilla(func(clave string, _ ...i18n.Vars) string { return clave })
	g.Poner([]api.BasesField{
		{ID: "estado", Name: "Estado", Kind: api.BasesKindMultiselect, Choices: []string{"Pendiente de la aprobación definitiva del departamento", "Revisado", "Publicado"}},
		{ID: "importe", Name: "Importe", Kind: api.BasesKindNumber},
	}, []api.BasesRecord{{
		ID: "r1", Data: map[string]any{"estado": []string{"Pendiente de la aprobación definitiva del departamento", "Revisado", "Publicado"}, "importe": 125.5},
	}})
	r := test.WidgetRenderer(g).(*rejillaRenderer)
	g.Resize(g.MinSize())
	r.Layout(g.Size())
	chips := r.chips[celda{0, 0}]
	mas := r.masChips[celda{0, 0}]
	if mas == nil || !mas.Visible() || mas.Texto != "+2" {
		t.Fatal("no se cuenta lo que queda fuera de la celda")
	}
	x1, _ := r.columna(1)
	for _, ch := range append([]*ui.Insignia{mas}, chips...) {
		if ch.Visible() && ch.Position().X+ch.Size().Width > x1-margenCelda {
			t.Fatalf("%q invade la columna de importe", ch.Texto)
		}
	}
	// La celda numérica termina en el margen derecho de su columna.
	xt, ancho := r.columna(1)
	numero := r.textos[0][1]
	fin := numero.Position().X + ui.Medir(numero.Text, tema.TextoSM, ui.Normal).Width
	if fin < xt+ancho-margenCelda-1 || fin > xt+ancho-margenCelda+1 {
		t.Errorf("el importe termina en %v; debería terminar en %v", fin, xt+ancho-margenCelda)
	}
	g.Resize(fyne.NewSize(g.MinSize().Width+1200, g.MinSize().Height))
	r.Layout(g.Size())
	if mas.Visible() {
		t.Fatal("el contador sigue cuando ya caben todas las opciones")
	}
	for _, ch := range chips {
		if !ch.Visible() {
			t.Fatalf("%q no reaparece al ampliar", ch.Texto)
		}
	}
}
