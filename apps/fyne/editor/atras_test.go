// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"
)

func TestAtrasDelEditorVuelveDeDocumentoYPapelera(t *testing.T) {
	p := NuevaPortada(Documento, nil, "es", nil)
	w := test.NewWindow(p.Objeto())
	defer w.Close()
	abierto, cierres := true, 0
	EngancharAtras(p.Objeto(), p, func() bool { return abierto }, func() { abierto = false; cierres++ })
	atras := func() { w.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: mobile.KeyBack}) }
	atras()
	if abierto || cierres != 1 {
		t.Fatal("atrás no usa la salida del documento")
	}
	p.AbrirPapelera()
	atras()
	if p.Papelera() != nil || cierres != 1 {
		t.Fatal("atrás no vuelve de la papelera a la portada")
	}
	atras()
	if cierres != 1 {
		t.Fatal("en la portada se ha vuelto a cerrar el documento")
	}
}
