// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func TestElAtrasRecogeHerramientaYPanelAntesDeVolver(t *testing.T) {
	p := NuevaPantalla(func(c string, _ i18n.Vars) string { return c })
	w := test.NewWindow(container.NewStack())
	defer w.Close()
	p.Atajos(w.Canvas())
	if p.atras() {
		t.Fatal("la portada no debe invocar la salida de un documento")
	}
	p.Contenido = &Contenido{Total: 1}
	p.barra.Herramienta, p.PanelAbierto = Resaltar, true
	salidas := 0
	p.AlVolver = func() { salidas++ }
	atras := func() { w.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: mobile.KeyBack}) }
	atras()
	if p.barra.Herramienta != Mano || !p.PanelAbierto || salidas != 0 {
		t.Fatal("atrás debe soltar primero la herramienta")
	}
	atras()
	if p.PanelAbierto || salidas != 0 {
		t.Fatal("atrás debe recoger el panel antes de salir")
	}
	atras()
	if salidas != 1 {
		t.Fatal("atrás no usa la salida de la cinta")
	}
	p.Contenido, p.doc = nil, &Documento{}
	atras()
	if salidas != 2 {
		t.Fatal("un PDF sin contenido disponible también necesita salida")
	}
}
