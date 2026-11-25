// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestRedactarEsElMasFlotanteEnTodosLosAnchos: «Redactar» es UN control, el «+» flotante
// de abajo a la derecha, también en escritorio (producto, 2026-11-25; el guardián web es
// `boton-flotante.test.ts`). El menú lateral ya no lo repite en su cabecera.
//
// VISTO EN ROJO con el botón de la cabecera del menú de antes y con la capa escondida
// desde 768.
func TestRedactarEsElMasFlotanteEnTodosLosAnchos(t *testing.T) {
	tr := traductor(t)
	p := NuevaPantalla(tr, "es")
	redactado := 0
	p.Menu.AlRedactar = func() { redactado++ }
	w := test.NewTempWindow(t, p.Objeto())
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))

	var fab *ui.BotonFlotante
	recorrer(w.Content(), func(o fyne.CanvasObject) {
		if f, es := o.(*ui.BotonFlotante); es {
			fab = f
		}
	})
	// En el menú: la bandeja vacía sí ofrece «Redactar» en su estado vacío, como la web.
	recorrer(p.armazon.Menu, func(o fyne.CanvasObject) {
		if b, es := o.(*ui.Boton); es && b.Texto == tr.T("nav.compose", nil) {
			t.Error("el menú lateral sigue con su botón «Redactar»: crear es el «+» flotante")
		}
	})
	if fab == nil || fab.Nombre != tr.T("nav.compose", nil) {
		t.Fatal("a 1280 no está el «+» flotante de «Redactar»")
	}
	pos := fab.Position()
	margen := ui.MargenFlotanteEn(1280)
	if pos.X+fab.Size().Width != 1280-margen || pos.Y+fab.Size().Height != 800-margen {
		t.Errorf("el «+» está en %v y va en la esquina de abajo a la derecha", pos)
	}
	test.Tap(fab)
	if redactado != 1 {
		t.Errorf("el «+» abrió el redactor %d veces", redactado)
	}
}
