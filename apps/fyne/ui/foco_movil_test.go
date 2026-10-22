// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// TestEnElTelefonoElTecladoSoloSaleParaEscribir: el driver móvil de Fyne abre el
// teclado con cualquier cosa enfocada. En el teléfono tocar un botón no lo enfoca (y el
// teclado no sale); un campo sí. En escritorio el botón se enfoca como siempre.
//
// VISTO EN ROJO con `enfocar` llamando a `c.Focus` sin mirar si escribe.
func TestEnElTelefonoElTecladoSoloSaleParaEscribir(t *testing.T) {
	boton := NuevoBotonIcono(iconos.Camara, "Cámara", func() {})
	campo := NuevaEntrada("")
	w := test.NewWindow(container.NewVBox(boton, campo))
	t.Cleanup(w.Close)
	c := w.Canvas()

	antes := esMovil
	t.Cleanup(func() { esMovil = antes })
	esMovil = func() bool { return true }
	test.Tap(boton)
	if c.Focused() != nil {
		t.Errorf("en el teléfono tocar un botón lo enfoca (%T) y saca el teclado", c.Focused())
	}
	enfocar(c, campo.editor)
	if c.Focused() != fyne.Focusable(campo.editor) {
		t.Errorf("en el teléfono un campo no se enfoca: %T", c.Focused())
	}

	c.Unfocus()
	esMovil = func() bool { return false }
	test.Tap(boton)
	if c.Focused() != fyne.Focusable(boton) {
		t.Errorf("en escritorio el botón pulsado no se enfoca: %T", c.Focused())
	}
}
