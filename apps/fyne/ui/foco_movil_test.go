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
	accion := NuevoBoton("Guardar", func() {})
	casilla := NuevaCasilla("Todo el día", nil)
	conmutador := NuevoConmutador("Recordatorio", nil)
	selector := NuevoSelector([]string{"Trabajo", "Personal"}, nil)
	campo := NuevaEntrada("")
	w := test.NewWindow(container.NewVBox(boton, accion, casilla, conmutador, selector, campo))
	t.Cleanup(w.Close)
	c := w.Canvas()

	antes := esMovil
	t.Cleanup(func() { esMovil = antes })
	esMovil = func() bool { return true }
	Enfocar(c, boton)
	test.Tap(boton)
	if c.Focused() != nil {
		t.Errorf("en el teléfono tocar un botón lo enfoca (%T) y saca el teclado", c.Focused())
	}
	Enfocar(c, campo.editor)
	if c.Focused() != fyne.Focusable(campo.editor) {
		t.Errorf("en el teléfono un campo no se enfoca: %T", c.Focused())
	}

	// Pulsar controles que no escriben cierra el teclado retirando el foco del campo.
	for _, ctrl := range []fyne.Tappable{boton, accion, casilla, conmutador} {
		Enfocar(c, campo.editor)
		ctrl.Tapped(&fyne.PointEvent{})
		if c.Focused() != nil {
			t.Errorf("pulsar %T en el teléfono dejó el foco en %T y el teclado abierto", ctrl, c.Focused())
		}
	}
	Enfocar(c, campo.editor)
	selector.Tapped(&fyne.PointEvent{})
	if c.Focused() != nil {
		t.Errorf("abrir un selector en el teléfono dejó el foco en %T y el teclado abierto", c.Focused())
	}
	if selector.abierto != nil {
		selector.Alternar()
	}

	Enfocar(c, campo.editor)
	Enfocar(c, nil)
	if c.Focused() != nil {
		t.Fatal("retirar el foco deja un campo activo en el teléfono")
	}
	esMovil = func() bool { return false }
	test.Tap(boton)
	if c.Focused() != fyne.Focusable(boton) {
		t.Errorf("en escritorio el botón pulsado no se enfoca: %T", c.Focused())
	}
}
