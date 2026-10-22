// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"
)

func TestElAtrasRespetaElCierreDeLaFicha(t *testing.T) {
	p := pantallaDePrueba(t)
	w := test.NewWindow(container.NewStack())
	defer w.Close()
	p.Atajos(w.Canvas(), nil)
	cierres := 0
	f := &Ficha{AlCerrar: func() { cierres++ }}
	p.ficha, p.abierta = f, true
	atras := func() { w.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: mobile.KeyBack}) }
	atras()
	if p.abierta || p.ficha != f || cierres != 0 {
		t.Fatal("atrás debe cerrar primero la paleta")
	}
	atras()
	if cierres != 1 || p.ficha != f {
		t.Fatal("atrás debe pedir el cierre a la ficha, conservando lo que ella decida")
	}
	f.AlCerrar = p.cerrarFicha
	atras()
	if p.ficha != nil || p.atras() {
		t.Fatal("el cierre de la ficha debe dejar atrás para el sistema en Inicio")
	}
}
