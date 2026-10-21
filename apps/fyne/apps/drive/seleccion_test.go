// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"image"
	"image/color"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestMayusMarcaUnRangoDesdeElAncla es el `toggleSelect` con Mayús: del ancla al pulsado,
// en cualquier orden, SIN desmarcar lo que ya estaba.
func TestMayusMarcaUnRangoDesdeElAncla(t *testing.T) {
	els := []Elemento{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}, {ID: "e"}}
	sel := map[string]bool{"e": true}
	MarcarRango(sel, els, 3, 1)
	var marcados []string
	for _, el := range els {
		if sel[el.ID] {
			marcados = append(marcados, el.ID)
		}
	}
	if !slices.Equal(marcados, []string{"b", "c", "d", "e"}) {
		t.Errorf("el rango marca %q", marcados)
	}
}

// TestLaTarjetaLlevaCasillaYMenuEnSusEsquinas: la casilla (`top-1.5 left-1.5`) y el «⋯»
// (`top-1.5 right-1.5`) aparecen al pasar por encima; el «⋯» abre su menú SIN abrir la
// tarjeta, y fuera de él la tarjeta se abre.
//
// VISTO EN ROJO (2026-09-23, en el frontal): quitando el `enMas` de `Tapped`, pulsar el
// «⋯» abre la tarjeta.
func TestLaTarjetaLlevaCasillaYMenuEnSusEsquinas(t *testing.T) {
	tj := NuevaTarjetaDrive("playa.jpg", "jpg", false)
	var abierta, menus, marcas int
	tj.AlAbrir = func() { abierta++ }
	tj.AlMenu = func(fyne.CanvasObject) { menus++ }
	tj.AlMarcar = func() { marcas++ }
	tj.Miniatura = image.NewRGBA(image.Rect(0, 0, 8, 6))
	w := test.NewWindow(tj)
	defer w.Close()
	tam := MedidaTarjetaDrive(200)
	w.Resize(tam)
	tj.Resize(tam)
	r := test.WidgetRenderer(tj).(*tarjetaRenderer)
	if r.casilla.Visible() || r.mas.Objeto().Visible() {
		t.Error("las esquinas se ven sin pasar por encima")
	}
	tj.MouseIn(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(100, 100)}})
	if !r.casilla.Visible() || !r.mas.Objeto().Visible() {
		t.Error("al pasar por encima no aparecen la casilla y el «⋯»")
	}
	o, m := cajaMasTarjeta(tj.Size())
	casi(t, "el «⋯» va a `right-1.5`", o.X+m.Width, 200-MargenEsquina)
	tj.Tapped(&fyne.PointEvent{Position: fyne.NewPos(o.X+m.Width/2, o.Y+m.Height/2)})
	tj.Tapped(&fyne.PointEvent{Position: fyne.NewPos(100, 100)})
	if menus != 1 || abierta != 1 {
		t.Errorf("menús %d y aperturas %d; quiero 1 y 1", menus, abierta)
	}
	if r.imagen == nil {
		t.Error("la miniatura no se pinta")
	}
	w.Canvas().Capture()
}

// TestSoloLasImagenesDelServicioLlevanMiniatura: JPEG, PNG y GIF, que son las que el
// servicio sabe reducir; un SVG o una carpeta no se piden.
func TestSoloLasImagenesDelServicioLlevanMiniatura(t *testing.T) {
	for nombre, quiero := range map[string]bool{"a.jpg": true, "b.PNG": true, "c.gif": true, "d.svg": false, "e.pdf": false} {
		if got := ConMiniatura(Elemento{ID: "x", Nombre: nombre}); got != quiero {
			t.Errorf("%s: %v", nombre, got)
		}
	}
	if ConMiniatura(Elemento{ID: "x", Nombre: "fotos.jpg", Carpeta: true}) {
		t.Error("una carpeta que se llama como una imagen pide miniatura")
	}
}

// TestLaBarraDeSeleccionLlevaMover: «Mover», «Descargar» y «Papelera», en ese orden.
func TestLaBarraDeSeleccionLlevaMover(t *testing.T) {
	tr := traductor(t)
	e := NuevoExplorador(tr)
	e.Fase, e.Ventana = Exito, 1280
	e.Elementos = []Elemento{{ID: "a", Nombre: "a.txt"}}
	e.Seleccion["a"] = true
	obj := e.Objeto(testVariante())
	var botones []string
	recorrer(obj, func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok && (b.Variante == ui.Secundario || b.Variante == ui.Peligro) {
			botones = append(botones, b.Texto)
		}
	})
	quiero := []string{tr.T("files.selection.move", nil), tr.T("files.selection.download", nil), tr.T("files.selection.trash", nil)}
	for _, q := range quiero {
		if !slices.Contains(botones, q) {
			t.Errorf("la barra no lleva %q: %q", q, botones)
		}
	}
}

func testVariante() tema.Variante { return tema.Claro }

// TestArrastrarSoloMueveLoQueCambiaDeSitio es `movibles`: lo que ya está en la carpeta
// de destino no cuenta, ni una carpeta soltada en sí misma o en una hija suya.
func TestArrastrarSoloMueveLoQueCambiaDeSitio(t *testing.T) {
	destino := Elemento{ID: "c2", Nombre: "B", Carpeta: true, Ruta: "/A/B"}
	origenes := []Elemento{
		{ID: "f1", Ruta: "/x.txt"},
		{ID: "f2", Ruta: "/A/B/y.txt"},           // ya está ahí
		{ID: "c1", Ruta: "/A", Carpeta: true},    // B es hija suya
		{ID: "c2", Ruta: "/A/B", Carpeta: true},  // ella misma
		{ID: "c3", Ruta: "/Otra", Carpeta: true}, // esta sí
	}
	movibles := Movibles(origenes, destino)
	ids := make([]string, 0, len(movibles))
	for _, el := range movibles {
		ids = append(ids, el.ID)
	}
	if !slices.Equal(ids, []string{"f1", "c3"}) {
		t.Errorf("se moverían %q", ids)
	}
	if SePuedeSoltar(origenes[1:4], destino) {
		t.Error("se acepta soltar donde nada cambiaría de sitio")
	}
}

// TestLaFilaDeDestinoSeResalta: al arrastrar sobre una carpeta que lo acepta, esa fila
// se pinta `bg-selected ring-2 ring-primary`; las demás, no.
func TestLaFilaDeDestinoSeResalta(t *testing.T) {
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana = Exito, 1280
	e.Elementos = []Elemento{{ID: "c1", Nombre: "Contratos", Carpeta: true}, {ID: "f1", Nombre: "a.txt"}}
	e.AlMoverA = func([]Elemento, Elemento) {}
	e.destino = "c1"
	obj := e.Objeto(tema.Claro)
	w := test.NewWindow(obj)
	defer w.Close()
	w.Resize(fyne.NewSize(1020, 400))
	var destinos []string
	recorrer(obj, func(o fyne.CanvasObject) {
		if a, ok := o.(*filaArrastrable); ok && a.Destino {
			destinos = append(destinos, a.Nombre)
		}
	})
	if !slices.Equal(destinos, []string{"Contratos"}) {
		t.Errorf("se resaltan %q", destinos)
	}
}

// TestLaTarjetaSeArrastraAUnaCarpeta: en la rejilla, arrastrar la tarjeta de un fichero
// sobre la de una carpeta la resalta (`ring-2`, que sobresale 2 del borde) sin rehacer la
// rejilla, y al soltar se mueve lo arrastrado a esa carpeta. Fuera de toda tarjeta no
// hay destino.
//
// VISTO EN ROJO (2026-09-24, en el frontal): construyendo siempre la tarjeta sin arrastre,
// dice que con ratón la tarjeta no se arrastra.
func TestLaTarjetaSeArrastraAUnaCarpeta(t *testing.T) {
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana, e.Rejilla = Exito, 1280, true
	e.Elementos = []Elemento{{ID: "c1", Nombre: "Contratos", Carpeta: true}, {ID: "f1", Nombre: "a.txt"}}
	var movidos []string
	var a Elemento
	e.AlMoverA = func(o []Elemento, d Elemento) {
		for _, x := range o {
			movidos = append(movidos, x.ID)
		}
		a = d
	}
	obj := e.Objeto(tema.Claro)
	w := test.NewWindow(obj)
	defer w.Close()
	w.Resize(fyne.NewSize(1020, 600))
	if len(e.tarjetas) != 2 {
		t.Fatalf("hay %d tarjetas", len(e.tarjetas))
	}
	fichero, ok := e.tarjetas[1].objeto.(*tarjetaArrastrable)
	if !ok {
		t.Fatal("con ratón la tarjeta no se arrastra")
	}
	d := fyne.CurrentApp().Driver()
	centro := func(o fyne.CanvasObject) fyne.Position {
		p := d.AbsolutePositionForObject(o)
		return fyne.NewPos(p.X+o.Size().Width/2, p.Y+o.Size().Height/2)
	}
	carpeta := e.tarjetas[0]
	fichero.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: centro(carpeta.objeto)}})
	if e.destino != "c1" || !carpeta.t.Destino || carpeta.t.grosorHalo() != 2 {
		t.Fatalf("sobre la carpeta el destino es %q y su resalte %v", e.destino, carpeta.t.Destino)
	}
	fichero.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: fyne.NewPos(1, 590)}})
	if e.destino != "" || carpeta.t.Destino {
		t.Error("fuera de las tarjetas sigue habiendo destino")
	}
	fichero.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: centro(carpeta.objeto)}})
	fichero.DragEnd()
	if !slices.Equal(movidos, []string{"f1"}) || a.ID != "c1" || carpeta.t.Destino {
		t.Errorf("al soltar se mueve %q a %q", movidos, a.ID)
	}
}

// TestLaCarpetaEsDeColorPlenoEnMarco: en la forma Marco una carpeta de la cuadrícula es
// una tarjeta de recurso de color pleno (`cardTone` en FileBrowser.tsx): el relleno de su
// tono, el nombre y la meta en la tinta de ese relleno, y la previsualización y el filete
// como velos de ella. En Carril, y un fichero en cualquier forma, la tarjeta blanca.
//
// VISTO EN ROJO con `tintas` sin mirar la forma: la carpeta sale de color también en
// Carril.
func TestLaCarpetaEsDeColorPlenoEnMarco(t *testing.T) {
	carpeta := NuevaTarjetaDrive("Contratos", "", true)
	carpeta.Tono = ui.TarjetaVioleta
	carpeta.Resize(MedidaTarjetaDrive(200))
	r := test.WidgetRenderer(carpeta).(*tarjetaRenderer)
	blanco := tema.Color("surface", tema.Claro)
	if c := carpeta.tintas(tema.Claro); c.pleno || c.fondo != blanco {
		t.Fatalf("en Carril la carpeta pinta %v; quiero la superficie, %v", c.fondo, blanco)
	}

	tema.ElegirForma("marco")
	t.Cleanup(tema.FormaDeLosGuardianes)
	carpeta.Refresh()
	relleno, tinta := ui.TonoSolido(ui.TarjetaVioleta, tema.Claro)
	if c := carpeta.tintas(tema.Claro); !c.pleno || c.fondo != relleno {
		t.Fatalf("en Marco la carpeta pinta %v; quiero el relleno de su tono, %v", c.fondo, relleno)
	}
	if r.nombre.Color != color.Color(tinta) || r.meta.Color != color.Color(tinta) {
		t.Errorf("el nombre va en %v y la meta en %v; quiero la tinta del relleno, %v", r.nombre.Color, r.meta.Color, tinta)
	}
	if r.previa.FillColor == color.Color(tema.Color("surface-sunken", tema.Claro)) || r.filete.FillColor == color.Color(tema.Color("border", tema.Claro)) {
		t.Error("la previsualización y el filete siguen con los colores de la superficie blanca")
	}
	fichero := NuevaTarjetaDrive("acta.pdf", "pdf", false)
	if fichero.tintas(tema.Claro).pleno {
		t.Error("un fichero no es una tarjeta de recurso: va en blanco")
	}
}

// TestLaFranjaDeCarpetasEnMarco: en la forma Marco, en la vista de lista y sin búsqueda,
// las carpetas salen ARRIBA como una franja de teselas de color pleno y la lista se queda
// con lo demás, cada parte en su orden (`partirEnTeselas` de teselas.ts). Son dos piezas
// y UNA serie de posiciones: el rango de Mayús va de una tesela a una fila sin saltarse
// nada. En Carril, en la cuadrícula, en una búsqueda o sin carpetas no se parte nada.
//
// VISTO EN ROJO con `marcar` contando sobre `Elementos`: con una carpeta DESPUÉS de un
// fichero en lo que manda el servicio, el rango marca lo que no se ve entre los dos.
func TestLaFranjaDeCarpetasEnMarco(t *testing.T) {
	ids := func(els []Elemento) []string {
		out := make([]string, len(els))
		for i, el := range els {
			out[i] = el.ID
		}
		return out
	}
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana = Exito, 1280
	// A propósito con una carpeta detrás de un fichero: la franja no reordena, parte.
	e.Elementos = []Elemento{
		{ID: "c1", Nombre: "Contratos", Carpeta: true},
		{ID: "f1", Nombre: "a.txt"},
		{ID: "c2", Nombre: "Diseño", Carpeta: true},
		{ID: "f2", Nombre: "b.txt"},
		{ID: "f3", Nombre: "c.txt"},
	}
	if c, r := e.Partes(); c != nil || len(r) != 5 {
		t.Fatalf("en Carril la franja parte %v y %v; no tiene que partir nada", ids(c), ids(r))
	}

	tema.ElegirForma("marco")
	t.Cleanup(tema.FormaDeLosGuardianes)
	c, r := e.Partes()
	if !slices.Equal(ids(c), []string{"c1", "c2"}) || !slices.Equal(ids(r), []string{"f1", "f2", "f3"}) {
		t.Fatalf("la franja lleva %v y la lista %v", ids(c), ids(r))
	}
	if !slices.Equal(ids(e.EnOrden()), []string{"c1", "c2", "f1", "f2", "f3"}) {
		t.Errorf("lo que se ve va en el orden %v", ids(e.EnOrden()))
	}
	// El rango, sobre la serie que se ve: de la segunda tesela a la segunda fila.
	sel := map[string]bool{}
	MarcarRango(sel, e.EnOrden(), 1, 3)
	if !sel["c2"] || !sel["f1"] || !sel["f2"] || sel["c1"] || sel["f3"] {
		t.Errorf("el rango de la tesela a la fila marca %v", sel)
	}

	// Pintada: dos teselas de color pleno arriba y tres filas en la lista.
	obj := e.Objeto(tema.Claro)
	w := test.NewWindow(obj)
	defer w.Close()
	w.Resize(fyne.NewSize(1020, 600))
	var teselas []*TarjetaDrive
	recorrer(obj, func(o fyne.CanvasObject) {
		if x, ok := o.(*TarjetaDrive); ok && x.Franja {
			teselas = append(teselas, x)
		}
	})
	if len(teselas) != 2 || len(e.filasDeLista()) != 3 {
		t.Fatalf("hay %d teselas en la franja y %d filas en la lista; quiero 2 y 3", len(teselas), len(e.filasDeLista()))
	}
	if got := teselas[0].Size().Height; got != AltoTeselaFranja {
		t.Errorf("la tesela mide %v de alto y la de la web, %v", got, AltoTeselaFranja)
	}
	tonos := ui.TonosTarjeta()
	if teselas[0].Tono != tonos[0] || teselas[1].Tono != tonos[1] || !teselas[0].tintas(tema.Claro).pleno {
		t.Errorf("las teselas llevan los tonos %q y %q; quiero los dos primeros, de color pleno", teselas[0].Tono, teselas[1].Tono)
	}

	// Donde no hay franja.
	carpetas := e.Elementos
	for _, c := range []struct {
		que               string
		rejilla, busqueda bool
		elementos         []Elemento
	}{
		{"en la cuadrícula", true, false, carpetas},
		{"en una búsqueda", false, true, carpetas},
		{"sin carpetas", false, false, []Elemento{{ID: "f1"}, {ID: "f2"}}},
	} {
		e.Rejilla, e.EnBusqueda, e.Elementos = c.rejilla, c.busqueda, c.elementos
		if franja, _ := e.Partes(); franja != nil {
			t.Errorf("%s hay franja: %v", c.que, ids(franja))
		}
	}
}
