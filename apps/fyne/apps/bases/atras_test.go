// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"
)

func TestElAtrasDeUnaBaseUsaTodasLasBases(t *testing.T) {
	tr, err := Traductor("es")
	if err != nil {
		t.Fatal(err)
	}
	w := test.NewWindow(nil)
	defer w.Close()
	p := NuevaPantalla(tr, nil, w)
	w.SetContent(p.Objeto())
	p.Arrancar()
	p.abierta = &Base{ID: "b1"}
	w.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: mobile.KeyBack})
	if p.abierta != nil || !p.Cargando || p.atras() {
		t.Fatal("atrás no vuelve a cargar Mis bases, o sigue consumiéndolo en la lista")
	}
}
