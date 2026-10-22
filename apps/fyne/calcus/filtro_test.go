// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image/color"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	fynetema "fyne.io/fyne/v2/theme"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// hojaConFiltro: una lista con cabecera en la fila 1 y el autofiltro puesto, con
// «Norte» fuera en la columna A.
const hojaConFiltro = `{"active":0,"sheets":[{"name":"H","cells":{` +
	`"A1":"Zona","B1":"Importe","A2":"Norte","B2":"10","A3":"Sur","B3":"2","A4":"Norte","B4":"7","A5":"Este","B5":"=B2+B3"},` +
	`"autofilter":{"row":0,"c1":0,"c2":1,"excl":{"0":["Norte"],"1":[3,"x"],"7":["y"]}}}]}`

// TestElAutofiltroSeLeeYSeEscribeComoEnLaWeb: el autofiltro ya no cierra el documento;
// se sanea como `sanitizeSheet` (fuera la columna que no cubre y lo que no es texto) y
// vuelve a salir al guardar.
func TestElAutofiltroSeLeeYSeEscribeComoEnLaWeb(t *testing.T) {
	lz := lienzoDePrueba()
	if !lz.Cargar(hojaConFiltro) {
		t.Fatal("un libro con autofiltro tiene que abrirse para escribir")
	}
	af := lz.Libro.Autofiltro()
	if af == nil || af.Row != 0 || af.C1 != 0 || af.C2 != 1 || len(af.Excl) != 2 ||
		strings.Join(af.Excl["0"], ",") != "Norte" || strings.Join(af.Excl["1"], ",") != "x" {
		t.Fatalf("el autofiltro se lee como %+v", af)
	}
	guardado := lz.Serializar()
	if !strings.Contains(guardado, `"autofilter":{"row":0,"c1":0,"c2":1,"excl":{"0":["Norte"],"1":["x"]}}`) {
		t.Errorf("el autofiltro no vuelve a salir al guardar: %s", guardado)
	}
	// Y el c2 por debajo de c1 se sube a c1, como el `clamp(…, c1, COLS − 1)` de la web.
	if !lz.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{},"autofilter":{"row":"3","c1":4,"c2":1}}]}`) {
		t.Fatal("no se abre")
	}
	if af := lz.Libro.Autofiltro(); af.Row != 3 || af.C1 != 4 || af.C2 != 4 {
		t.Errorf("el rango saneado es %+v", af)
	}
}

// TestElAutofiltroEscondeFilasYLaRejillaLasSalta: las filas del cuerpo con un valor
// excluido no se ven, las de debajo suben, y la rejilla sabe qué fila hay bajo cada
// punto contando solo las que se ven.
func TestElAutofiltroEscondeFilasYLaRejillaLasSalta(t *testing.T) {
	lz := lienzoDePrueba()
	lz.Cargar(hojaConFiltro)
	ocultas := lz.Libro.FilasOcultas()
	if len(ocultas) < 6 || !ocultas[1] || ocultas[2] || !ocultas[3] || ocultas[4] || ocultas[0] {
		t.Fatalf("se esconden %v y tienen que ser las filas 2 y 4 (las de «Norte»)", ocultas[:6])
	}
	r := lz.Rejilla
	vw := r.vista()
	// Fila 1 (cabecera), luego la 3 («Sur») en el segundo puesto y la 5 («Este») en el tercero.
	if vw.y(2) != vw.oy()+vw.alto() || vw.y(4) != vw.oy()+2*vw.alto() {
		t.Errorf("la fila 3 está en %v y la 5 en %v", vw.y(2), vw.y(4))
	}
	medio := func(f int) fyne.Position { return fyne.NewPos(vw.x(0)+5, vw.y(f)+5) }
	if c, ok := r.celdaEn(medio(4)); !ok || c.F != 4 {
		t.Errorf("bajo el tercer puesto está la fila %d y tiene que ser la 5", c.F+1)
	}
	// Quitar la exclusión devuelve la geometría de siempre.
	lz.Mandar(CmdFiltrar, `{"col":0,"value":"Norte","hidden":false}`)
	if lz.Libro.FilasOcultas() != nil || r.vista().y(4) != vw.oy()+4*vw.alto() {
		t.Error("sin exclusiones no se esconde nada")
	}
	if !lz.Libro.PuedeDeshacer() {
		t.Error("filtrar se deshace, como en la web")
	}
}

// TestElDesplegableDelAutofiltroFiltra: el «▾» de la cabecera abre los valores
// distintos de la columna, en orden es-ES numérico con «(Vacías)» al final, y quitar
// una casilla esconde sus filas.
func TestElDesplegableDelAutofiltroFiltra(t *testing.T) {
	lz := lienzoDePrueba()
	lz.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{"A1":"N","A2":"10","A3":"2","A4":"Ñu","A5":"Oso","A6":"Nube"},` +
		`"autofilter":{"row":0,"c1":0,"c2":0}}]}`)
	entradas := lz.Libro.ValoresFiltro(0)
	valores := make([]string, 0, len(entradas))
	for _, e := range entradas {
		valores = append(valores, e.Valor)
	}
	if got := strings.Join(valores, "|"); got != "2|10|Nube|Ñu|Oso|" {
		t.Errorf("los valores salen %q", got)
	}

	w := test.NewTempWindow(t, lz)
	w.Resize(fyne.NewSize(560, 300))
	r := lz.Rejilla
	pos, tam, ok := r.botonFiltro(0)
	if !ok {
		t.Fatal("la columna del autofiltro no tiene «▾»")
	}
	centro := fyne.NewPos(pos.X+tam.Width/2, pos.Y+tam.Height/2)
	if zona, c := r.zonaEn(centro); zona != zonaFiltro || c.C != 0 {
		t.Fatalf("bajo el «▾» está la zona %d", zona)
	}
	r.Tapped(&fyne.PointEvent{Position: centro})
	capas := w.Canvas().Overlays().List()
	if len(capas) != 1 {
		t.Fatal("el «▾» no abre el desplegable")
	}
	guarda(t, w.Canvas().Capture()) // con `SYTUE_CAPTURA`, el desplegable abierto
	// Cuelga a `mt-0.5` del «▾», no al `mt-1` de los demás emergentes.
	if abajo := lz.anclaFiltro.Position().Y + lz.anclaFiltro.Size().Height + ui.SeparacionEmergente; abajo != AltoBarra+pos.Y+tam.Height+SeparacionValores {
		t.Errorf("el desplegable cuelga a %v y tiene que ir a %v", abajo, AltoBarra+pos.Y+tam.Height+SeparacionValores)
	}
	// Y arranca en el borde izquierdo de la columna, no pegado al «▾» por la derecha.
	if x := lz.anclaFiltro.Position().X; x != r.vista().x(0) {
		t.Errorf("el desplegable arranca en x=%v y la columna A en %v", x, r.vista().x(0))
	}
	filas := filasDe(capas[0])
	if len(filas) != 6 || !filas[0].casilla.Marcada {
		t.Fatalf("el desplegable tiene %d filas", len(filas))
	}
	filas[3].Tapped(nil) // «Ñu»
	if filas[3].casilla.Marcada || !lz.Libro.FilasOcultas()[3] {
		t.Error("quitar la casilla de «Ñu» no esconde su fila")
	}

	// Encima de los valores, ordenar por la columna; y ordenar cierra el desplegable.
	var ordenar []*ui.ElementoMenu
	for _, x := range todos(capas[0]) {
		if e, ok := x.(*ui.ElementoMenu); ok {
			ordenar = append(ordenar, e)
		}
	}
	if len(ordenar) != 2 || ordenar[1].Texto != lz.t("data.sortDesc", nil) {
		t.Fatalf("el desplegable no empieza por ordenar: %d elementos", len(ordenar))
	}
	ordenar[1].Tapped(nil)
	var col []string
	for f := 1; f <= 5; f++ {
		col = append(col, lz.Libro.textoFiltro(0, f))
	}
	// Los textos, antes que los números; y entre ellos, por unidades UTF-16 de la
	// mayúscula, como el motor: «Ñ» (U+00D1) va detrás de «O».
	if got := strings.Join(col, "|"); got != "Ñu|Oso|Nube|10|2" {
		t.Errorf("de mayor a menor queda %q", got)
	}
	if len(w.Canvas().Overlays().List()) != 0 {
		t.Error("ordenar no cierra el desplegable")
	}
	if !lz.Libro.FilasOcultas()[1] {
		t.Error("«Ñu» ya no está oculta en su fila nueva")
	}
}

// TestElCuerpoDelFiltroEsElDeLaWeb es el `autofilterBody` de sheets-tables.test.ts.
func TestElCuerpoDelFiltroEsElDeLaWeb(t *testing.T) {
	tablas := compilarTablas([]TablaFmt{{Name: "T", Range: "A1:B5", TotalsRow: true}})
	celdas := slices.Values([]Celda{{C: 2, F: 2}, {C: 3, F: 6}, {C: 4, F: 19}, {C: 0, F: 29}})
	for _, c := range []struct {
		af     Autofiltro
		f1, f2 int
	}{
		{Autofiltro{Row: 0, C1: 0, C2: 1}, 1, 3},
		{Autofiltro{Row: 2, C1: 2, C2: 3}, 3, 6},
		{Autofiltro{Row: 9, C1: 5, C2: 5}, 10, 9},
	} {
		if f1, f2 := cuerpoDelFiltro(tablas, &c.af, celdas); f1 != c.f1 || f2 != c.f2 {
			t.Errorf("%+v: %d-%d, y en la web %d-%d", c.af, f1, f2, c.f1, c.f2)
		}
	}
}

// TestElBotonDeAutofiltroLoPoneYLoQuita: con una celda, sobre el rectángulo usado; otra
// vez, lo quita. Una hoja vacía no se filtra.
func TestElBotonDeAutofiltroLoPoneYLoQuita(t *testing.T) {
	lz := lienzoDePrueba()
	lz.Mandar(CmdAutofiltro, "")
	if lz.Libro.Autofiltro() != nil {
		t.Error("una hoja vacía no se filtra")
	}
	lz.Libro.Escribir(1, 2, "a")
	lz.Libro.Escribir(3, 5, "b")
	lz.Mandar(CmdAutofiltro, "")
	if af := lz.Libro.Autofiltro(); af == nil || af.Row != 2 || af.C1 != 1 || af.C2 != 3 || !lz.Estado().Autofiltro {
		t.Errorf("con una celda el autofiltro es %+v, y tiene que ir de B3 a D", af)
	}
	lz.Mandar(CmdAutofiltro, "")
	if lz.Libro.Autofiltro() != nil {
		t.Error("el botón no quita el autofiltro")
	}
}

// filasDe son las filas del desplegable que cuelgan de una capa.
func filasDe(o fyne.CanvasObject) []*casillaXS {
	var out []*casillaXS
	for _, x := range todos(o) {
		if f, ok := x.(*casillaXS); ok {
			out = append(out, f)
		}
	}
	return out
}

// TestLasListasDeLosPanelesNoLlevanSombra: un `overflow-auto` de la web no pinta sombra
// en el borde; el desplazamiento de Fyne sí, salvo con el tema sin sombras.
func TestLasListasDeLosPanelesNoLlevanSombra(t *testing.T) {
	filas := make([]fyne.CanvasObject, 20)
	for i := range filas {
		filas[i] = ui.NuevoRotulo("x", tema.TextoXS, ui.Normal, "text")
	}
	tope := tema.Espacio(32) // `max-h-32`
	o := topeAlto(tope, container.New(ui.Columna{}, filas...))
	th, ok := o.(*container.ThemeOverride)
	if !ok {
		t.Fatalf("la lista con tope es un %T", o)
	}
	if c := th.Theme.Color(fynetema.ColorNameShadow, fynetema.VariantLight); c != color.Transparent {
		t.Errorf("la sombra del borde es %v", c)
	}
	if o.MinSize().Height != tope {
		t.Errorf("el tope de alto no se respeta: %v", o.MinSize())
	}
}
