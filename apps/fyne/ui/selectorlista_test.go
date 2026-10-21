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

// TestLaListaDelSelectorNoSeSaleDeLaVentana: con cientos de opciones la lista se queda
// en el hueco bajo su selector —no lo tapa— y se desplaza POR DENTRO hasta el final, con
// la elegida a la vista; mientras queda algo por ver, el borde de abajo se funde y lleva
// su flecha. Y con la lista abierta las flechas siguen moviendo la elección, se haya
// abierto con el ratón o con el teclado. Con pocas opciones va tal cual.
//
// VISTO EN ROJO sin enfocar el panel al abrir: con la lista abierta el teclado no es de
// nadie y `Down` no mueve la elección.
func TestLaListaDelSelectorNoSeSaleDeLaVentana(t *testing.T) {
	opciones := make([]string, 400)
	for i := range opciones {
		opciones[i] = "Zona"
	}
	s := NuevoSelector(opciones, nil)
	s.Elegida = 200
	w := test.NewTempWindow(t, container.NewWithoutLayout(s))
	w.Resize(fyne.NewSize(400, 600))
	s.Move(fyne.NewPos(20, 40))
	s.Resize(MedidaSelector(CampoMD, 200))
	s.Tapped(&fyne.PointEvent{}) // con el ratón
	cp, _ := w.Canvas().Overlays().Top().(*capa)
	if cp == nil {
		t.Fatal("la lista no se ha abierto")
	}
	p := cp.panel.(*panelLista)
	r := test.WidgetRenderer(p).(*panelRenderer)

	// Donde está de verdad el selector: la ventana de prueba lleva su relleno.
	debajo := fyne.CurrentApp().Driver().AbsolutePositionForObject(s).Y + s.Size().Height + tema.Espacio(1)
	hueco := w.Canvas().Size().Height - debajo - MargenVentana
	if p.Position().Y != debajo || p.Size().Height != hueco {
		t.Fatalf("la lista va en y=%v con %v de alto; quiero el hueco bajo el selector: y=%v y %v", p.Position().Y, p.Size().Height, debajo, hueco)
	}
	m := tema.Borde + tema.Espacio(1)
	if y := m + 200*AltoOpcion - p.desplazado; y < 0 || y+AltoOpcion > hueco {
		t.Errorf("la elegida queda en y=%v dentro de un panel de %v: no se ve", y, hueco)
	}
	if !r.aviso.velo.Visible() {
		t.Error("quedan doscientas opciones por debajo y el panel no avisa")
	}
	// Las flechas, con la lista abierta: el foco es del panel, que se las pasa al selector.
	if w.Canvas().Focused() != fyne.Focusable(p) {
		t.Errorf("con la lista abierta el foco está en %T y no en el panel", w.Canvas().Focused())
	}
	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	if s.Elegida != 201 {
		t.Errorf("la flecha abajo deja la elección en %d; quiero 201", s.Elegida)
	}
	// Hasta el final: la última fila entera a la vista, y el aviso se va.
	p.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.Delta{DY: -1e6}})
	if fin := m + 400*AltoOpcion - p.desplazado; fin != hueco-m {
		t.Errorf("al final la última fila acaba en y=%v de un panel de %v; quiero %v", fin, hueco, hueco-m)
	}
	if r.aviso.velo.Visible() {
		t.Error("al llegar al final el aviso sigue ahí y tapa la última opción")
	}
	p.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if s.abierto != nil {
		t.Error("Escape no cierra la lista")
	}

	pocas := NuevoSelector([]string{"a", "b"}, nil)
	w.SetContent(container.NewWithoutLayout(pocas))
	pocas.Move(fyne.NewPos(20, 40))
	pocas.Resize(MedidaSelector(CampoMD, 200))
	pocas.Alternar()
	t.Cleanup(func() {
		if pocas.abierto != nil {
			pocas.abierto.Hide()
		}
	})
	cp, _ = w.Canvas().Overlays().Top().(*capa)
	if cp == nil {
		t.Fatal("la lista corta no se ha abierto")
	}
	corta := cp.panel.(*panelLista)
	if corta.Size().Height != corta.MinSize().Height || corta.desplazado != 0 || test.WidgetRenderer(corta).(*panelRenderer).aviso.velo.Visible() {
		t.Error("dos opciones no necesitan desplazamiento ni aviso")
	}
}

// TestElAltoDeLaClaseMandaSobreLaTalla: `<Select class="h-8">` baja la caja a 28 y deja
// el ancho y lo demás de la talla.
func TestElAltoDeLaClaseMandaSobreLaTalla(t *testing.T) {
	s := NuevoSelector([]string{"7 días"}, nil)
	s.Ancho, s.Alto = 90, tema.Espacio(8)
	if m := s.MinSize(); m.Height != 32 || m.Width != 90 {
		t.Errorf("con h-8 mide %v", m)
	}
	s.Alto = 0
	if m := s.MinSize(); m.Height != MedidaSelector(CampoMD, 90).Height {
		t.Errorf("sin alto mide %v y no el de la talla", m)
	}
}
