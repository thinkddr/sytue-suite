// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestElAutocompletadoTraslucidoOcultaLasCeldas(t *testing.T) {
	tema.Elegir("traslucido")
	t.Cleanup(func() { tema.Elegir("") })
	lista := nuevasSugerencias(nil)
	lista.items, lista.valores = []string{"Uno", "Dos"}, true
	lista.Resize(fyne.NewSize(320, 100))
	debajo := canvas.NewRectangle(color.NRGBA{R: 255, A: 255})
	debajo.Resize(lista.Size())
	img := pinta(t, 320, 100, debajo, lista)
	if got, want := px(img, 300, 80), tema.Color("menu-surface", tema.Translucido); !cerca(got, want) {
		t.Errorf("las celdas se mezclan con las sugerencias: %v; el papel opaco es %v", got, want)
	}
	if got, want := px(img, 300, 10), tema.Color("menu-selected", tema.Translucido); !cerca(got, want) {
		t.Errorf("la opción elegida pinta %v; el resaltado del menú es %v", got, want)
	}
}
