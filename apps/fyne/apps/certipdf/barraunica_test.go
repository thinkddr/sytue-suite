// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestElDocumentoVaEnLaBarraDeLaApp: con un PDF abierto, en escritorio la cinta dice el
// nombre del documento delante de las pestañas (no salía en ninguna parte) y en el
// teléfono la salida, el nombre y el panel van en la barra de la app, que es la de la
// suite: una barra y la fila de las pestañas, en vez de dos barras más esa fila.
func TestElDocumentoVaEnLaBarraDeLaApp(t *testing.T) {
	p, w := documentoDeRecorrido(t)
	tr, err := i18n.New("es", i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	barra, restaurar := ui.BarraSuiteDePrueba(tr, iconos.PDF)
	defer restaurar()
	w.SetContent(barra.Montar(p.Objeto()))
	donde := fyne.CurrentApp().Driver().AbsolutePositionForObject

	w.Resize(fyne.NewSize(1280, 800))
	fyne.DoAndWait(func() {})
	var titulo *ui.Rotulo
	todos(w.Content(), func(o fyne.CanvasObject) {
		if r, ok := o.(*ui.Rotulo); ok && r.Texto == "a.pdf" {
			titulo = r
		}
	})
	if titulo == nil || !titulo.Visible() || titulo.Size().Width <= 0 {
		t.Fatal("en escritorio la cinta no dice el nombre del documento")
	}
	if p.movil.Visible() {
		t.Error("en escritorio no hay barra de la app")
	}

	defer ui.TactilDePrueba(true)()
	w.Resize(fyne.NewSize(390, 844))
	fyne.DoAndWait(func() {})
	fyne.DoAndWait(func() {})
	if y, alto := donde(p.movil).Y, p.movil.Size().Height; y != 0 || alto != ui.AltoBarraSuite() {
		t.Errorf("la barra de la app está en y=%v con %v de alto; quiero la de la suite", y, alto)
	}
	if p.movil.Titulo != "a.pdf" || !p.movil.EnDetalle || len(p.movil.Acciones) != 1 {
		t.Errorf("la barra de la app dice %q (volver %v) con %d acciones", p.movil.Titulo, p.movil.EnDetalle, len(p.movil.Acciones))
	}
	if !p.barra.EnBarraApp {
		t.Fatal("a 390 la cinta no sabe que la salida va en la barra de la app")
	}
	if y := donde(p.marco).Y; y != ui.AltoBarraSuite() {
		t.Errorf("la cinta empieza en y=%v; quiero justo bajo la barra única (%v)", y, ui.AltoBarraSuite())
	}
	var salidas int
	var ver func(fyne.CanvasObject)
	ver = func(o fyne.CanvasObject) {
		if o == nil || !o.Visible() {
			return
		}
		if b, ok := o.(*ui.BotonIcono); ok && b.Rotulo == p.t("pdf.back") {
			salidas++
		}
		switch x := o.(type) {
		case *fyne.Container:
			for _, c := range x.Objects {
				ver(c)
			}
		case fyne.Widget:
			for _, c := range test.WidgetRenderer(x).Objects() {
				ver(c)
			}
		}
	}
	ver(p.marco)
	if salidas != 0 {
		t.Errorf("a 390 la cinta sigue con su propia salida (%d)", salidas)
	}
}

// TestAlAbrirLasHojasMidenSuZoom: dentro del armazón, el documento recién abierto pinta
// sus hojas a la medida del zoom al ancho. El desplazador se colocaba antes de calcular
// ese zoom y, sin otra pasada, las hojas se quedaban a la del zoom 1 (100×162 en un
// hueco de 1000), arrimadas a la derecha.
func TestAlAbrirLasHojasMidenSuZoom(t *testing.T) {
	p, _ := documentoDeRecorrido(t)
	pag := p.Contenido.Paginas[0]
	if got, quiero := p.hojas[0].Size().Width, float32(pag.Ancho*p.Zoom); p.Zoom <= 1 || got < quiero-1 || got > quiero+1 {
		t.Errorf("al %.2f la primera hoja mide %v de ancho; quiero %v", p.Zoom, got, quiero)
	}
}
