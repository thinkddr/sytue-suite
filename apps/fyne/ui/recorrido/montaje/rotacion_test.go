// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package montaje

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestElMontajeDeRotacionDevuelveLaAppAlRecorrido(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)
	previo := tema.AppElegida()
	t.Cleanup(func() { tema.ElegirApp(previo) })
	tema.ElegirApp("drive")
	b := widget.NewButton("Acción", nil)
	contenido := container.NewWithoutLayout(b)
	w := test.NewTempWindow(t, contenido)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(900, 600))
	b.Move(fyne.NewPos(1000, 1000))
	ComprobarRotacion(t, w, "fixture", iconos.Docs, false, func() {
		tam := contenido.Size()
		b.Resize(fyne.NewSize(32, 32))
		b.Move(fyne.NewPos(tam.Width-32, tam.Height-32))
	})
	if w.Canvas().Content() != contenido || w.Canvas().Size() != fyne.NewSize(900, 600) || tema.AppElegida() != "drive" {
		t.Fatal("el montaje no ha devuelto el contenido, tamaño y producto originales")
	}
}
