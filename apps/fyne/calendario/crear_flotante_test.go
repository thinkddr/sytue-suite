// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestCrearEventoEsElMasFlotanteEnEscritorio: «Crear evento» es UN control, el «+»
// flotante de abajo a la derecha, también en escritorio (producto, 2026-11-25; el
// guardián web es `boton-flotante.test.ts`). El menú lateral ya no lo lleva arriba, y
// pulsarlo abre el formulario.
//
// VISTO EN ROJO con el botón de la cabecera del menú de antes y con la capa escondida
// desde 768.
func TestCrearEventoEsElMasFlotanteEnEscritorio(t *testing.T) {
	a, w := appConVentana(t, &escrituras{})
	rotulo := a.P.t("nav.createEvent", nil)
	var fab *ui.BotonFlotante
	recorrer(w.Content(), func(o fyne.CanvasObject) {
		if f, es := o.(*ui.BotonFlotante); es {
			fab = f
		}
	})
	if fab == nil || !fab.Visible() || fab.Nombre != rotulo {
		t.Fatalf("a %v no está el «+» flotante de «Crear evento»", w.Canvas().Size())
	}
	tam := w.Canvas().Size()
	margen := ui.MargenFlotanteEn(tam.Width)
	if pos := fab.Position(); pos.X+fab.Size().Width != tam.Width-margen ||
		pos.Y+fab.Size().Height != tam.Height-margen {
		t.Errorf("el «+» está en %v y va en la esquina de abajo a la derecha de %v", pos, tam)
	}
	recorrer(a.P.menu(), func(o fyne.CanvasObject) {
		if b, es := o.(*ui.Boton); es && b.Texto == rotulo {
			t.Error("el menú lateral sigue con su botón «Crear evento»: crear es el «+» flotante")
		}
	})
	antes := len(w.Canvas().Overlays().List())
	test.Tap(fab)
	if len(w.Canvas().Overlays().List()) <= antes {
		t.Error("el «+» flotante no abre el formulario de crear")
	}
}
