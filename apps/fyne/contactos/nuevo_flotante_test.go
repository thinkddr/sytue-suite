// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestNuevoContactoEsElMasFlotanteEnTodosLosAnchos: «Nuevo contacto» es UN control, el «+»
// flotante de abajo a la derecha, también en escritorio (producto, 2026-11-25; el
// guardián web es `boton-flotante.test.ts`). El menú lateral ya no lo lleva arriba. Sin
// libreta va apagado, y sin red no está, como en la web.
//
// VISTO EN ROJO con el botón de la cabecera del menú de antes y con la capa escondida
// desde 768.
func TestNuevoContactoEsElMasFlotanteEnTodosLosAnchos(t *testing.T) {
	p := pantallaDeMuestra(t)
	creados := 0
	p.AlCrearContacto = func() { creados++ }
	rotulo := p.t("nav.newContact", nil)
	flotante := func() *ui.BotonFlotante {
		var fab *ui.BotonFlotante
		recorrer(p.capaFlotante(), func(o fyne.CanvasObject) {
			if f, es := o.(*ui.BotonFlotante); es {
				fab = f
			}
		})
		return fab
	}

	w := test.NewTempWindow(t, p.Construir())
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))
	var fab *ui.BotonFlotante
	recorrer(w.Content(), func(o fyne.CanvasObject) {
		if f, es := o.(*ui.BotonFlotante); es {
			fab = f
		}
	})
	if fab == nil || !fab.Visible() || fab.Nombre != rotulo {
		t.Fatal("a 1280 no está el «+» flotante de «Nuevo contacto»")
	}
	if pos := fab.Position(); pos.X+fab.Size().Width != 1280-ui.MargenFlotanteEn(1280) {
		t.Errorf("el «+» está en %v y va en la esquina de abajo a la derecha", pos)
	}
	recorrer(p.menu(), func(o fyne.CanvasObject) {
		if b, es := o.(*ui.Boton); es && b.Texto == rotulo {
			t.Error("el menú lateral sigue con su botón «Nuevo contacto»: crear es el «+» flotante")
		}
	})
	test.Tap(fab)
	if creados != 1 {
		t.Errorf("el «+» abrió el alta %d veces", creados)
	}

	p.Activa = nil
	if f := flotante(); f == nil || !f.Inerte {
		t.Error("sin libreta el «+» sigue vivo")
	}
	p.SinConexion = true
	if f := flotante(); f != nil {
		t.Error("sin red el «+» sigue a la vista")
	}
}
