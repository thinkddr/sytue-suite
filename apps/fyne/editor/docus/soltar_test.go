// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestSoltarTextoEnLaPagina: el texto soltado sobre la página se escribe donde cae —aquí,
// en la esquina de abajo a la derecha, que es el final del documento—, como el `drop`
// del navegador en el `contenteditable`; y soltado fuera de la página no escribe nada.
func TestSoltarTextoEnLaPagina(t *testing.T) {
	f := folioCon(t, "<p>Hola</p>")
	// Soltar enfoca la página, y el foco arranca el parpadeo del cursor. Es una
	// [fyne.Animation] (ver `escribir.go`) y no un temporizador propio: el driver de
	// pruebas la tica una sola vez en el sitio, sin dejar una rutina viva detrás.
	w := test.NewWindow(f.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	d := fyne.CurrentApp().Driver()
	a, tam := d.AbsolutePositionForObject(f.pag), f.pag.Size()

	if f.Soltar("fuera", a.SubtractXY(1, 1)) || strings.Contains(f.Serializar(), "fuera") {
		t.Fatalf("soltado fuera de la página, ha entrado: %s", f.Serializar())
	}
	if !f.Soltar(" mundo", a.Add(tam).SubtractXY(1, 1)) {
		t.Fatal("soltado sobre la página, no ha entrado")
	}
	if got := f.Serializar(); !strings.Contains(got, "Hola mundo") {
		t.Errorf("el documento queda %s", got)
	}
}
