// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package recorrido

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestLaRotacionRestauraElFixtureAntesDelRecorrido(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)
	antes := tema.EsMarco()
	w := test.NewTempWindow(t, widget.NewButton("Acción", nil))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(900, 600))
	tactil := false
	cambiar := func(valor bool) func() {
		previo := tactil
		tactil = valor
		return func() { tactil = previo }
	}
	ComprobarRotacion(t, w, "fixture", cambiar, false)
	if tactil || tema.EsMarco() != antes || w.Canvas().Size() != fyne.NewSize(900, 600) {
		t.Fatal("la rotación cambió el modo táctil, la forma o la ventana del recorrido")
	}
}

func TestLaRotacionPreparaLosDatosTrasCambiarElTamano(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)
	b := widget.NewButton("Acción al pie", nil)
	w := test.NewTempWindow(t, container.NewWithoutLayout(b))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(900, 600))
	b.Move(fyne.NewPos(1000, 1000)) // Sin preparar, la geometría tiene que rechazarlo.
	cambiar := func(bool) func() { return func() {} }
	ComprobarRotacion(t, w, "datos", cambiar, false, func() {
		tam := w.Canvas().Size()
		b.Resize(fyne.NewSize(32, 32))
		b.Move(fyne.NewPos(tam.Width-32, tam.Height-32))
	})
}

func TestLaRotacionDetectaControlesFueraDelViewport(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)
	b := widget.NewButton("Acción", nil)
	w := test.NewTempWindow(t, container.NewWithoutLayout(b))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(100, 100))
	b.Resize(fyne.NewSize(40, 40))
	b.Move(fyne.NewPos(90, 10))
	_, fallos := geometriaRotacion(w.Canvas())
	if !strings.Contains(strings.Join(fallos, " "), "fuera del viewport") {
		t.Fatal("el guardián acepta un mando fuera del viewport")
	}
	b.Move(fyne.NewPos(10, 10))
	if _, fallos := geometriaRotacion(w.Canvas()); len(fallos) != 0 {
		t.Fatalf("rechaza un mando que cabe: %v", fallos)
	}
}

func TestLaRotacionNoConfundeContenidoDesplazableConOverflow(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)
	for _, propio := range []bool{false, true} {
		b := widget.NewButton("Acción al final", nil)
		fondo := canvas.NewRectangle(color.NRGBA{})
		fondo.SetMinSize(fyne.NewSize(80, 500))
		contenido := container.NewWithoutLayout(fondo, b)
		b.Resize(fyne.NewSize(60, 40))
		b.Move(fyne.NewPos(10, 450))
		scroll := container.NewVScroll(contenido)
		var viewport fyne.CanvasObject = scroll
		if propio {
			v := &viewportDeRotacion{contenido: contenido}
			v.ExtendBaseWidget(v)
			viewport = v
		}
		w := test.NewTempWindow(t, viewport)
		w.SetPadded(false)
		w.Resize(fyne.NewSize(100, 100))
		if !propio {
			scroll.ScrollToBottom()
		}
		if mandos, fallos := geometriaRotacion(w.Canvas()); mandos == 0 || len(fallos) != 0 {
			t.Fatalf("desplazador propio=%v: rechaza contenido recortado: mandos=%d, fallos=%v", propio, mandos, fallos)
		}
	}
}

// Como ui.DesplazaLado: Scrollable propio que el pintor recorta a su viewport.
type viewportDeRotacion struct {
	widget.BaseWidget
	contenido fyne.CanvasObject
}

func (v *viewportDeRotacion) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(v.contenido)
}

func (*viewportDeRotacion) Scrolled(*fyne.ScrollEvent) {}
