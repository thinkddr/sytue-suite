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

func TestSelectorYMenuMovilEnHojaYDesplazablesConElDedo(t *testing.T) {
	telefono(t)
	opciones := make([]string, 30)
	for i := range opciones {
		opciones[i] = "Opción móvil"
	}
	s := NuevoSelector(opciones, nil)
	s.Elegida = 2
	w := test.NewTempWindow(t, container.NewWithoutLayout(s))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 844))
	s.Move(fyne.NewPos(24, 120))
	s.Resize(MedidaSelector(CampoMD, 342))
	s.Tapped(&fyne.PointEvent{})
	t.Cleanup(func() {
		if s.abierto != nil {
			s.abierto.Hide()
		}
	})

	cp, _ := w.Canvas().Overlays().Top().(*capa)
	if cp == nil {
		t.Fatal("el selector móvil no abrió su capa")
	}
	p := cp.panel.(*panelLista)
	if !p.hoja || p.Size().Width != 390 || p.Position().Y+p.Size().Height != 844 {
		t.Fatalf("el selector móvil no se presentó como hoja inferior: hoja=%v pos=%v tam=%v", p.hoja, p.Position(), p.Size())
	}
	if fila := p.hijos[2].(*opcion); fila.Size().Height != SueloTactilBarra {
		t.Fatalf("la fila táctil del selector mide %v; quiero %v", fila.Size().Height, SueloTactilBarra)
	}
	antes := p.desplazado
	p.Dragged(&fyne.DragEvent{Dragged: fyne.Delta{DY: -120}})
	p.DragEnd()
	if p.desplazado <= antes {
		t.Fatalf("arrastrar el selector con el dedo no desplazó las opciones: antes=%v después=%v", antes, p.desplazado)
	}
	s.Alternar()

	items := make([]fyne.CanvasObject, 30)
	for i := range items {
		items[i] = NuevoElementoMenu("Acción", 0, nil)
	}
	m := NuevoMenu(items...)
	m.Mostrar(s)
	t.Cleanup(m.Cerrar)
	m.Dragged(&fyne.DragEvent{Dragged: fyne.Delta{DY: -100}})
	m.DragEnd()
	if m.desplazado <= 0 {
		t.Fatalf("arrastrar el menú con el dedo no desplazó sus elementos: desplazado=%v", m.desplazado)
	}
}

// TestElSelectorSinAnchoMideSuOpcionMasLarga: un `<select>` sin ancho mide su opción más
// larga, y no recorta «Gestión» en «G…».
//
// VISTO EN ROJO (2026-11-06): el mínimo dejaba un rem para el texto.
func TestElSelectorSinAnchoMideSuOpcionMasLarga(t *testing.T) {
	for _, talla := range []TallaCampo{CampoMD, CampoSM} {
		s := NuevoSelector([]string{"Lectura", "Gestión de permisos"}, nil)
		s.Talla = talla
		m, sel := medidasCampo[talla], medidasSelector[talla]
		quiero := Medir("Gestión de permisos", m.texto, Normal).Width + 2*tema.Borde + m.margen + sel.derecha
		if got := s.MinSize().Width; got < quiero {
			t.Errorf("talla %v: el selector mide %v y su opción más larga pide %v", talla, got, quiero)
		}
		s.Ancho = 50
		if got := s.MinSize().Width; got != 50 {
			t.Errorf("talla %v: con Ancho 50 mide %v", talla, got)
		}
	}
}
