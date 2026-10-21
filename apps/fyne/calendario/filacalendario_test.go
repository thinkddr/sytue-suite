// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestLaFilaDeCalendarioSeEnfocaConSuAro: la casilla de «Mis calendarios» se alcanza
// con el tabulador, como en la web, y al enfocarla se ve el aro BLANCO del cromo
// alrededor de la casilla —no de la fila— con la esquina de Chromium. Antes la fila no
// era enfocable y el teclado se la saltaba.
func TestLaFilaDeCalendarioSeEnfocaConSuAro(t *testing.T) {
	test.NewTempApp(t)
	f := NuevaFilaCalendario("Personal", color.NRGBA{R: 0x1a, G: 0x6f, B: 0xbb, A: 0xff}, true)
	f.RotuloOpciones = "Opciones de Personal"
	f.Resize(fyne.NewSize(231, AltoFilaCalendario()))
	var _ fyne.Focusable = f

	blanco := color.Color(tema.Color("sidebar-focus", tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
	// El aro abierto de `ui.Aro` son cuatro franjas pintadas; la de arriba, que lleva las
	// dos esquinas, es la que se mira: 14 de casilla más 2 de aro por cada lado.
	pintada := func(r *canvas.Raster) image.Image {
		return r.Generator(int(r.Size().Width), int(r.Size().Height))
	}
	aro := func() *canvas.Raster {
		for _, o := range test.WidgetRenderer(f).Objects() {
			r, es := o.(*canvas.Raster)
			if es && r.Size().Width == LadoCasillaCalendario+4 && r.Size().Height < LadoCasillaCalendario &&
				color.NRGBAModel.Convert(pintada(r).At(int(r.Size().Width)/2, 0)) == blanco {
				return r
			}
		}
		return nil
	}
	a := aro()
	if a == nil {
		t.Fatal("la fila no tiene un aro de `sidebar-focus`, el foco del carril")
	}
	if a.Visible() {
		t.Error("el aro se ve sin foco")
	}
	kebab := func() fyne.CanvasObject {
		for _, o := range test.WidgetRenderer(f).Objects() {
			if _, es := o.(*ui.AccionLateral); es {
				return o
			}
		}
		return nil
	}
	if kebab().Visible() {
		t.Error("el kebab se ve sin puntero ni foco")
	}

	f.FocusGained()
	if !a.Visible() {
		t.Fatal("con el foco no se ve el aro")
	}
	// La esquina de Chromium (2) más el aro: la franja de arriba mide los 2 del aro y
	// los 2 de la esquina, y su pico queda sin pintar.
	if _, _, _, pico := pintada(a).At(0, 0).RGBA(); a.Size().Height != 2+ui.RadioCasillaNativa || pico != 0 {
		t.Errorf("la franja de arriba del aro mide %v de alto y su pico tiene alfa %v; la esquina de la casilla pide %v y redondeada",
			a.Size().Height, pico, 2+ui.RadioCasillaNativa)
	}
	if !kebab().Visible() {
		t.Error("con el foco en la fila el kebab sigue escondido (`group-focus-within`)")
	}

	// El foco pasa al kebab: la fila lo pierde y el kebab NO se esconde.
	f.FocusLost()
	kebab().(*ui.AccionLateral).FocusGained()
	if !kebab().Visible() {
		t.Error("el kebab se esconde al recibir el foco")
	}
	if a.Visible() {
		t.Error("el aro de la casilla sigue visible con el foco en el kebab")
	}

	// Espacio conmuta, como en una casilla.
	conmutada := false
	f.AlConmutar = func() { conmutada = true }
	f.TypedRune(' ')
	if !conmutada {
		t.Error("Espacio no conmuta la casilla")
	}
}
