// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image/color"
	"math"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// RibbonGroup real, Chromium remoto 2026-10-04: raíz16, Figtree cargada,
// grupo160, fila148, px-1.5=6; Herramientas mide66.25 y comienza en46.875.
// Si el rótulo manda sobre la fila, la fila se centra bajo él.
func TestElGrupoCentraSuFilaYSuRotulo(t *testing.T) {
	boton := func() fyne.CanvasObject {
		r := canvas.NewRectangle(color.Transparent)
		r.SetMinSize(fyne.NewSize(28, 28))
		return r
	}
	for _, c := range []struct {
		n                  int
		rotulo             string
		xFila, xRotulo     float32
		comoLoDiceLaMedida string
	}{
		// Cinco botones: fila148 + px-1.5 a ambos lados =160.
		{5, "Herramientas", MargenGrupo, 46.875, "medido en la web"},
		// Un botón bajo un rótulo más ancho: la fila se centra bajo el rótulo.
		{1, "Herramientas", 0, MargenGrupo, "el rótulo manda"},
	} {
		ctl := make([]fyne.CanvasObject, c.n)
		for i := range ctl {
			ctl[i] = boton()
		}
		g := NuevoGrupo(c.rotulo, ctl...)
		w := test.NewWindow(g)
		w.SetPadded(false)
		w.Resize(MedidaGrupo(g))
		ancho := MedidaGrupo(g).Width
		fila := float32(c.n)*28 + float32(c.n-1)*HuecoControl
		xFila := c.xFila
		if c.n == 1 {
			xFila = (ancho - fila) / 2
		}
		if got := ctl[0].Position().X; math.Abs(float64(got-xFila)) > 0.02 {
			t.Errorf("%s: el primer control está en x=%g y no en %g", c.comoLoDiceLaMedida, got, xFila)
		}
		r := test.WidgetRenderer(g).(*grupoRenderer)
		rot := ui.Medir(c.rotulo, tema.Texto2XS, ui.Normal).Width
		quiero := (ancho - rot) / 2
		if c.n == 5 {
			quiero = c.xRotulo
		}
		if got := r.rotulo.Position().X; math.Abs(float64(got-quiero)) > 0.2 {
			t.Errorf("%s: el rótulo está en x=%g y no en %g", c.comoLoDiceLaMedida, got, quiero)
		}
		w.Close()
	}
}
