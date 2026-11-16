// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

func TestCambiarIdentidadRetiraInclusoUnDialogoNoDescartable(t *testing.T) {
	w := test.NewWindow(container.NewWithoutLayout())
	defer w.Close()
	w.Resize(fyne.NewSize(320, 640))
	d := NuevoDialogo("Private account", NuevaEntrada("Private value"))
	d.NoDescartable = true
	d.Mostrar(w.Canvas())
	if w.Canvas().Overlays().Top() == nil {
		t.Fatal("private fixture did not show")
	}
	test.Tap(d.capa.fondo)
	if w.Canvas().Overlays().Top() == nil {
		t.Fatal("fixture was dismissible")
	}
	OlvidarSuperposiciones(w.Canvas())
	if w.Canvas().Overlays().Top() != nil || w.Canvas().Focused() != nil {
		t.Fatal("old identity retained an overlay or focus")
	}
	d.Cerrar()
}
