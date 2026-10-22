// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestSoltarEnCampoEntraDondeSeSuelta: el texto soltado entra en el campo por el punto
// donde cae —al principio o al final, según el lado—, sin tocar el portapapeles del
// sistema; en un campo de una línea, los saltos se vuelven espacios; y fuera del campo o
// con el campo apagado no entra nada.
func TestSoltarEnCampoEntraDondeSeSuelta(t *testing.T) {
	uno := NuevoEditorPlano(tema.TextoBase, nil)
	varias := NuevoEditorPlanoMultilinea(tema.TextoBase, nil)
	w := test.NewWindow(container.NewVBox(uno.env, varias.env))
	defer w.Close()
	w.Resize(fyne.NewSize(400, 300))
	fyne.CurrentApp().Clipboard().SetContent("lo copiado")
	d := fyne.CurrentApp().Driver()

	uno.PonerTexto("fin")
	a := d.AbsolutePositionForObject(uno)
	if !SoltarEnCampo(uno, "uno\ndos ", a.AddXY(1, uno.Size().Height/2)) || uno.Texto() != "uno dos fin" {
		t.Errorf("soltado al principio: %q", uno.Texto())
	}
	if !SoltarEnCampo(uno, "!", a.AddXY(uno.Size().Width-1, uno.Size().Height/2)) || uno.Texto() != "uno dos fin!" {
		t.Errorf("soltado al final: %q", uno.Texto())
	}
	b := d.AbsolutePositionForObject(varias)
	if !SoltarEnCampo(varias, "hola\nmundo", b.AddXY(5, 5)) || varias.Texto() != "hola\nmundo" {
		t.Errorf("soltado en el de varias líneas: %q", varias.Texto())
	}
	if w.Canvas().Focused() != fyne.Focusable(varias) {
		t.Error("el campo donde se soltó no se queda con el foco")
	}
	if c := fyne.CurrentApp().Clipboard().Content(); c != "lo copiado" {
		t.Errorf("soltar ha cambiado el portapapeles: %q", c)
	}

	if SoltarEnCampo(varias, "x", b.SubtractXY(1, 1)) || varias.Texto() != "hola\nmundo" {
		t.Error("soltado fuera del campo, ha entrado")
	}
	varias.Disable()
	if SoltarEnCampo(varias, "x", b.AddXY(5, 5)) || varias.Texto() != "hola\nmundo" {
		t.Error("soltado en un campo apagado, ha entrado")
	}
}
