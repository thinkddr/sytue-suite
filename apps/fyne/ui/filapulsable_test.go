// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Con SinRelleno el contenido llega al borde y va ARRIBA: es la tarjeta entera de una
// galería, cuya portada no debe bajar cuando la fila la estira la tarjeta más alta. Sin
// él sigue siendo la fila de siempre, con su `px-2 py-1` y centrada.
func TestLaFilaPulsableSinRellenoLlegaAlBorde(t *testing.T) {
	contenido := canvas.NewRectangle(tema.Color("text", tema.Claro))
	contenido.SetMinSize(fyne.NewSize(100, 40))
	f := NuevaFilaPulsable("Abrir", contenido, nil)
	f.SinRelleno, f.Radio = true, tema.RadioLG
	test.WidgetRenderer(f).Layout(fyne.NewSize(200, 120))
	if got := MedidaFilaPulsable(f); got.Width != 100 {
		t.Errorf("sin relleno, el ancho es el del contenido: %v", got)
	}
	if p, s := contenido.Position(), contenido.Size(); p != (fyne.Position{}) || s.Width != 200 {
		t.Errorf("sin relleno el contenido va arriba y a todo el ancho: %v %v", p, s)
	}

	otro := canvas.NewRectangle(tema.Color("text", tema.Claro))
	otro.SetMinSize(fyne.NewSize(100, 40))
	g := NuevaFilaPulsable("Abrir", otro, nil)
	test.WidgetRenderer(g).Layout(fyne.NewSize(200, 120))
	if p := otro.Position(); p.X != rellenoFilaPulsableX || p.Y != 40 {
		t.Errorf("con relleno, `px-2` y centrada: %v", p)
	}
}
