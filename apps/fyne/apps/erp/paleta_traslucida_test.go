// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestLaPaletaTraslucidaOcultaLaPantalla(t *testing.T) {
	tema.Elegir("traslucido")
	t.Cleanup(func() { tema.Elegir("") })
	const ancho, alto float32 = 800, 600
	p := NuevaPaleta(traductor(t), []Modelo{{Clave: "uno", Etiqueta: "Uno"}, {Clave: "dos", Etiqueta: "Dos"}})
	panel := p.Objeto(tema.Translucido, fyne.NewSize(ancho, alto))
	debajo := canvas.NewRectangle(color.NRGBA{R: 255, A: 255})
	debajo.Resize(fyne.NewSize(ancho, alto))
	panel.Resize(debajo.Size())
	w := test.NewTempWindow(t, container.NewWithoutLayout(debajo, panel))
	w.SetPadded(false)
	w.Resize(debajo.Size())
	img := w.Canvas().Capture()
	escala := float32(img.Bounds().Dx()) / ancho
	x := (ancho-AnchoPaleta)/2 + AnchoPaleta - 10
	y := alto*FraccionArribaPaleta + tema.Borde + AltoCampoPaleta + tema.Borde + ListaPaletaP + 1.5*AltoFilaPaleta()
	if got, want := pixel(img, x*escala, y*escala), tema.Color("menu-surface", tema.Translucido); !cercaDe(got, want) {
		t.Errorf("la pantalla se mezcla con los módulos: %v; el papel opaco es %v", got, want)
	}
}
