// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"reflect"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
)

// TestUnLibroConOcultasSeEditaYSaleIgual: con `hiddenRows` y `hiddenCols` el libro no se
// abre en solo lectura, lo oculto no se ve, y al guardar sale igual, también lo que cae
// más allá de esta rejilla (la web tiene un millón de filas).
func TestUnLibroConOcultasSeEditaYSaleIgual(t *testing.T) {
	doc := `{"active":0,"sheets":[{"name":"H","cells":{"A1":"x"},` +
		`"hiddenRows":[{"start":2,"end":3},{"start":200,"end":1048575}],"hiddenCols":[{"start":1,"end":1}]}]}`
	lz := NuevoLienzo(NuevoLibro())
	if !lz.Cargar(doc) {
		t.Fatal("un libro con filas ocultas se abre en solo lectura")
	}
	var a, b any
	_ = json.Unmarshal([]byte(doc), &a)
	_ = json.Unmarshal([]byte(lz.Serializar()), &b)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("entra\n %s\ny sale\n %s", doc, lz.Serializar())
	}
	oc := lz.Libro.FilasOcultas()
	if len(oc) < 5 || oc[1] || !oc[2] || !oc[3] || oc[4] {
		t.Errorf("no se esconden justo las filas 3 y 4: %v", oc[:5])
	}
	if oc := lz.Libro.ColumnasOcultas(); len(oc) < 3 || oc[0] || !oc[1] || oc[2] {
		t.Errorf("no se esconde justo la columna B: %v", oc)
	}
}

// TestOcultarYMostrarComoEnLaWeb son las cuatro órdenes del menú sobre la selección.
func TestOcultarYMostrarComoEnLaWeb(t *testing.T) {
	lz := NuevoLienzo(NuevoLibro())
	r := lz.Rejilla
	r.sel, r.ext = Celda{C: 0, F: 2}, &Celda{C: 1, F: 4}
	lz.Mandar(CmdOcultar, `{"axis":"row","op":"hide"}`)
	if f, _ := lz.Libro.TramosOcultos(); !reflect.DeepEqual(f, []analisis.Tramo{{Start: 2, End: 4}}) {
		t.Fatalf("ocultar las filas 3 a 5 deja %v", f)
	}
	if oc := lz.Libro.FilasOcultas(); !oc[3] || oc[5] {
		t.Error("las filas ocultas siguen a la vista")
	}
	lz.Mandar(CmdOcultar, `{"axis":"col","op":"hide"}`)
	if _, c := lz.Libro.TramosOcultos(); !reflect.DeepEqual(c, []analisis.Tramo{{Start: 0, End: 1}}) {
		t.Fatalf("ocultar las columnas A y B deja %v", c)
	}
	// Con `SYTUE_CAPTURA`, la rejilla con la raya doble donde hay algo oculto.
	w := test.NewTempWindow(t, lz)
	w.Resize(fyne.NewSize(640, 320))
	guarda(t, w.Canvas().Capture())
	// La fila 1 y la columna A: se recuperan seleccionando la de al lado.
	r.sel, r.ext = Celda{C: 2, F: 5}, nil
	lz.Mandar(CmdOcultar, `{"axis":"row","op":"show"}`)
	lz.Mandar(CmdOcultar, `{"axis":"col","op":"show"}`)
	if f, c := lz.Libro.TramosOcultos(); f != nil || c != nil {
		t.Errorf("mostrar junto al bloque no lo enseña: filas %v columnas %v", f, c)
	}
	lz.Deshacer(false)
	if _, c := lz.Libro.TramosOcultos(); c == nil {
		t.Error("deshacer no vuelve a ocultar las columnas")
	}
}

// TestNoSeOcultaLaUltima: ocultarlo todo no se hace y se avisa, como en la web.
func TestNoSeOcultaLaUltima(t *testing.T) {
	lz := NuevoLienzo(NuevoLibro())
	var aviso string
	lz.AlAviso = func(_ ui.TonoAviso, titulo, _ string) { aviso = titulo }
	r := lz.Rejilla
	r.sel, r.ext = Celda{C: 0, F: 0}, &Celda{C: 0, F: MaxFilas - 1}
	lz.Mandar(CmdOcultar, `{"axis":"row","op":"hide"}`)
	if f, _ := lz.Libro.TramosOcultos(); f != nil || aviso != "sheet.msg.hideAll.rows" {
		t.Errorf("ocultar todas las filas deja %v y avisa %q", f, aviso)
	}
}

// TestLasOcultasSiguenALaEstructura: insertar delante las baja; borrarlas las quita.
func TestLasOcultasSiguenALaEstructura(t *testing.T) {
	lz := NuevoLienzo(NuevoLibro())
	r := lz.Rejilla
	r.sel, r.ext = Celda{C: 0, F: 3}, nil
	lz.Mandar(CmdOcultar, `{"axis":"row","op":"hide"}`)
	r.sel = Celda{C: 0, F: 0}
	lz.Mandar(CmdInsFila, "above")
	if f, _ := lz.Libro.TramosOcultos(); !reflect.DeepEqual(f, []analisis.Tramo{{Start: 4, End: 4}}) {
		t.Fatalf("insertar encima deja las ocultas en %v", f)
	}
	r.sel, r.ext = Celda{C: 0, F: 3}, &Celda{C: 0, F: 5}
	lz.Mandar(CmdBorrarFilas, "")
	if f, _ := lz.Libro.TramosOcultos(); f != nil {
		t.Errorf("borrar la fila oculta la deja en %v", f)
	}
}

// TestSubtotalSeSaltaLasOcultasAMano: SUBTOTAL(109) no cuenta lo oculto a mano.
func TestSubtotalSeSaltaLasOcultasAMano(t *testing.T) {
	lz := NuevoLienzo(NuevoLibro())
	for f, v := range []string{"1", "10", "100"} {
		lz.Libro.CargarEn(0, 0, f, v)
	}
	lz.Libro.CargarEn(0, 2, 0, "=SUBTOTAL(109,A1:A3)")
	if got := lz.Libro.Valor(2, 0); got != 111.0 {
		t.Fatalf("sin ocultas, %v", got)
	}
	lz.Rejilla.sel, lz.Rejilla.ext = Celda{C: 0, F: 1}, nil
	lz.Mandar(CmdOcultar, `{"axis":"row","op":"hide"}`)
	if got := lz.Libro.Valor(2, 0); got != 101.0 {
		t.Errorf("con la fila 2 oculta, %v", got)
	}
}

// TestOcultarConLaHojaProtegida va con el permiso de formato, como en la web.
func TestOcultarConLaHojaProtegida(t *testing.T) {
	lz := NuevoLienzo(NuevoLibro())
	lz.Libro.Proteger(&Proteccion{}, "")
	lz.Rejilla.sel = Celda{C: 0, F: 3}
	lz.Mandar(CmdOcultar, `{"axis":"row","op":"hide"}`)
	if f, _ := lz.Libro.TramosOcultos(); f != nil {
		t.Errorf("con la hoja protegida sin permiso de formato se oculta: %v", f)
	}
	if !lz.Libro.vetado(CmdOcultar) {
		t.Error("ocultar no está vetado con la hoja protegida")
	}
}

// TestElMenuOcultarMandaSusOrdenes: el ojo del grupo Celdas de Datos abre las cuatro
// órdenes, y la de «Ocultar filas» oculta las de la selección y cierra el menú.
func TestElMenuOcultarMandaSusOrdenes(t *testing.T) {
	lz := lienzoDePrueba()
	lz.Rejilla.sel = Celda{C: 0, F: 4}
	ctx := &editor.Contexto{Tipo: editor.Hoja, Lienzo: lz}
	cuerpo := lz.cuerpoDatos(ctx)
	w := test.NewTempWindow(t, container.NewBorder(cuerpo, nil, nil, nil, lz))
	w.Resize(fyne.NewSize(1280, 800))
	b := botonDeCinta(cuerpo, "data.hideShow")
	if b == nil {
		t.Fatal("Datos no tiene «Ocultar y mostrar»")
	}
	b.Tapped(nil)
	var items []*ui.ElementoMenu
	for _, o := range todos(w.Canvas().Overlays().Top()) {
		if it, ok := o.(*ui.ElementoMenu); ok {
			items = append(items, it)
		}
	}
	rotulos := make([]string, 0, len(items))
	for _, it := range items {
		rotulos = append(rotulos, it.Texto)
	}
	if quiero := []string{"data.hideRows", "data.showRows", "data.hideCols", "data.showCols"}; !reflect.DeepEqual(rotulos, quiero) {
		t.Fatalf("el menú tiene %v", rotulos)
	}
	guarda(t, w.Canvas().Capture())
	items[0].Tapped(nil)
	if f, _ := lz.Libro.TramosOcultos(); !reflect.DeepEqual(f, []analisis.Tramo{{Start: 4, End: 4}}) {
		t.Errorf("«Ocultar filas» deja %v", f)
	}
	if w.Canvas().Overlays().Top() != nil {
		t.Error("el menú sigue abierto")
	}
}
