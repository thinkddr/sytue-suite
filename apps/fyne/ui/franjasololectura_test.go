// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestLaFranjaSoloLecturaTieneSalida es el contrato de `ReadOnlyBar.tsx`: la franja
// lleva el botón que la desbloquea, el botón funciona, y con la petición ya enviada el
// botón deja paso a la nota y no se puede volver a pedir.
//
// Los altos son los de la hoja, no los que calcula el widget: `py-1` (3,5 + 3,5) más el
// botón `h-6` con su suelo de 24 más el `border-b` = 32; sin botón, la línea de
// `text-sm` de la hoja de la suite (15,75, `tema.TextoSM`) en vez del botón = 23,75.
func TestLaFranjaSoloLecturaTieneSalida(t *testing.T) {
	pedida := 0
	f := NuevaFranjaSoloLectura("Solo lectura").Con("Pedir acceso de edición", func() { pedida++ })
	w := test.NewTempWindow(t, f)
	w.Resize(fyne.NewSize(800, 60))
	if got := f.MinSize().Height; got != 33 {
		t.Errorf("con botón mide %v de alto y la web 32", got)
	}
	test.Tap(f.Accion)
	if pedida != 1 {
		t.Fatalf("pulsar «Pedir acceso de edición» no pidió nada (%d)", pedida)
	}
	if p := f.Accion.Position(); p.X <= 400 || !f.Accion.Visible() {
		t.Errorf("el botón no está a la derecha de la franja: %v", p)
	}

	f.Nota = "Petición enviada"
	f.Refresh()
	if len(f.botones()) != 0 {
		t.Error("con la petición enviada la franja sigue ofreciendo el botón")
	}
	if got := f.MinSize().Height; got != 27 {
		t.Errorf("sin botón mide %v de alto y la web 23,75", got)
	}
}
