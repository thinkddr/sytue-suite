// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

// TestElTecladoNoTapaElCampo: con el campo enfocado al fondo de una página que se
// desplaza, cuando la ventana encoge (sale el teclado del teléfono) MostrarFoco lo sube
// hasta que se ve, aunque la página quepa entera y no se pudiera desplazar (el hueco del
// teclado). Si ya se ve, no mueve nada. VISTO EN ROJO sin el caso del campo por debajo y
// sin el hueco (la página entera cabía y el desplazamiento se quedaba en 0).
func TestElTecladoNoTapaElCampo(t *testing.T) {
	relleno := canvas.NewRectangle(nil)
	relleno.SetMinSize(fyne.NewSize(300, 900))
	campo := NuevaEntrada("")
	hueco := &HuecoTeclado{}
	s := container.NewVScroll(container.New(hueco, container.NewVBox(relleno, campo)))
	w := test.NewWindow(s)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(360, 1000))
	w.Canvas().Focus(campo.editor)
	if MostrarFoco(s, w.Canvas(), 0) {
		t.Fatal("con el campo a la vista no hay nada que mover")
	}
	// El teclado tapa la mitad de abajo; el lienzo NO encoge, como en el móvil.
	if !MostrarFoco(s, w.Canvas(), 500) || s.Offset.Y <= 0 {
		d := fyne.CurrentApp().Driver()
		t.Fatalf("con el teclado fuera el desplazamiento se queda en %v (scroll %v %v, campo %v %v, contenido %v, zona %v)",
			s.Offset.Y, d.AbsolutePositionForObject(s), s.Size(), d.AbsolutePositionForObject(campo.editor), campo.editor.Size(),
			s.Content.Size(), fmt.Sprint(w.Canvas().InteractiveArea()))
	}
	fin := fyne.CurrentApp().Driver().AbsolutePositionForObject(campo).Y + campo.Size().Height
	if fin > 500 {
		t.Errorf("el campo acaba en y=%v, debajo del teclado (500)", fin)
	}
	// Al irse el teclado, el hueco desaparece.
	MostrarFoco(s, w.Canvas(), 0)
	if hueco.Alto != 0 {
		t.Errorf("sin teclado queda un hueco de %v", hueco.Alto)
	}
}

// TestUnCampoAvisaAlEnfocarseEnElTelefono: en el teléfono, un campo que recibe el foco
// llama a AlEscribir con su lienzo; en escritorio no. VISTO EN ROJO sin avisarEscritura.
func TestUnCampoAvisaAlEnfocarseEnElTelefono(t *testing.T) {
	campo := NuevaEntrada("")
	w := test.NewWindow(campo)
	t.Cleanup(w.Close)
	var avisos []fyne.Canvas
	AlEscribir = func(c fyne.Canvas) { avisos = append(avisos, c) }
	antes := esMovil
	t.Cleanup(func() { AlEscribir, esMovil = nil, antes })
	esMovil = func() bool { return false }
	w.Canvas().Focus(campo.editor)
	if len(avisos) != 0 {
		t.Error("en escritorio no hay teclado que esquivar")
	}
	w.Canvas().Unfocus()
	esMovil = func() bool { return true }
	w.Canvas().Focus(campo.editor)
	if len(avisos) != 1 || avisos[0] != w.Canvas() {
		t.Errorf("en el teléfono el campo avisó %d veces", len(avisos))
	}
}
