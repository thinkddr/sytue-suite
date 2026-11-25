// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// TestUnMontajeRetiradoNoRecolocaElArmazon: el giro de prueba de `recorrido/montaje`
// pone el armazón bajo la barra de la suite y lo devuelve a la ventana sin ella. Al
// estrecharse después, la barra vieja recolocaba su montaje —que ya no se pinta— y le
// devolvía al armazón la medida de entonces: el ERP seguía a 1280 en una ventana de 400
// y pintaba el carril de escritorio (visto el 2026-11-24 al darle barra móvil).
func TestUnMontajeRetiradoNoRecolocaElArmazon(t *testing.T) {
	a := NuevoArmazon(contenidoDePrueba())
	a.Menu = NuevoLateral("Menú", nil)
	b := NuevaBarraSuite(traductorPrueba(t), nil, iconos.ERP)
	t.Cleanup(b.Parar)
	w := test.NewWindow(b.Montar(a))
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))
	fyne.DoAndWait(func() {})
	w.Resize(fyne.NewSize(390, 844))
	fyne.DoAndWait(func() {})
	if a.suite == nil {
		t.Fatal("el montaje no ha apuntado su barra en el armazón")
	}

	w.SetContent(a)
	w.Resize(fyne.NewSize(400, 1600))
	fyne.DoAndWait(func() {})
	fyne.DoAndWait(func() {})
	if got := a.Size(); got != fyne.NewSize(400, 1600) {
		t.Errorf("sin el montaje el armazón mide %v en una ventana de 400×1600", got)
	}
	if bm, es := a.barraMovil().(*BarraMovil); !es || bm.enSuite {
		t.Error("sin la barra de la suite en la ventana, la barra móvil no puede ir dentro de ella")
	}
}
