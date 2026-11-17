// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"image/color"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
)

// vectoresMedidas es la sección «medidas» de `calcus.json`: lo que calculan `medidaDe`,
// `acumulado`, `indiceEn`, `leerMedida`, `conMedidas`, `anchoAjustado` y `altoAjustado` de
// `sheets/rejilla.ts`, y la geometría que pinta SheetCanvas con medidas importadas.
type vectoresMedidas struct {
	Limites struct {
		Min, AnchoMax, AltoMax, Aire float32
	}
	MedidaDe []struct {
		Medidas map[string]float64
		I       int
		Serie   float32
		Tope    *float32
		Sale    float32
	}
	Geometrias []struct {
		ColWidths    map[string]float64 `json:"colWidths"`  //nolint:tagliatelle // el nombre lo fija la web
		RowHeights   map[string]float64 `json:"rowHeights"` //nolint:tagliatelle // el nombre lo fija la web
		OcultasCols  []int
		OcultasFilas []int
		NCols, NRows int
		Xs, Ys       []float32
		ColX         []float32
		RowTop       []float32
		ColEn        []int
		FilaEn       []int
	}
	Leer []struct {
		Valor string
		Sale  json.RawMessage
	}
	ConMedidas []struct {
		Medidas map[string]float64
		Indices []int
		Px      *float64
		Serie   float32
		Tope    *float32
		Sale    map[string]float64
	}
	AnchoAjustado []struct {
		Anchos []*float32
		Sale   float32
	}
	AltoAjustado []struct {
		Tamanos []float64
		Sale    float32
	}
}

func leerVectoresMedidas(t *testing.T) vectoresMedidas {
	t.Helper()
	b, err := os.ReadFile(rutaVectores)
	if err != nil {
		t.Fatalf("sin los vectores compartidos: %v", err)
	}
	var v struct{ Medidas vectoresMedidas }
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Medidas.Geometrias) == 0 {
		t.Fatal("calcus.json no trae la sección «medidas»")
	}
	return v.Medidas
}

func topeO(t *float32) float32 {
	if t == nil {
		return AnchoMax
	}
	return *t
}

// TestLasMedidasSonLasDeLaWeb: las funciones puras de las medidas dan, con las mismas
// entradas, lo mismo que las de `sheets/rejilla.ts`.
func TestLasMedidasSonLasDeLaWeb(t *testing.T) {
	v := leerVectoresMedidas(t)
	if v.Limites.Min != MinMedida || v.Limites.AnchoMax != AnchoMax || v.Limites.AltoMax != AltoMax || v.Limites.Aire != AireAjuste {
		t.Errorf("los límites de la web son %+v", v.Limites)
	}
	for _, c := range v.MedidaDe {
		if got := medidaDe(c.Medidas, c.I, c.Serie, topeO(c.Tope)); got != c.Sale {
			t.Errorf("medidaDe(%v, %d) = %v; la web dice %v", c.Medidas, c.I, got, c.Sale)
		}
	}
	for _, c := range v.Leer {
		p, ok := leerMedida(c.Valor)
		got := "null"
		switch {
		case ok && p.auto:
			got = `"auto"`
		case ok:
			got = strconv.FormatFloat(p.px, 'f', -1, 64)
		}
		if got != string(c.Sale) {
			t.Errorf("leerMedida(%q) = %s; la web dice %s", c.Valor, got, c.Sale)
		}
	}
	for _, c := range v.ConMedidas {
		antes := maps2(c.Medidas)
		got := conMedidas(c.Medidas, c.Indices, c.Px, c.Serie, topeO(c.Tope))
		if !mismasMedidas(got, c.Sale) || (got == nil) != (c.Sale == nil) {
			t.Errorf("conMedidas(%v, %v, %v) = %v; la web dice %v", c.Medidas, c.Indices, c.Px, got, c.Sale)
		}
		if !mismasMedidas(antes, c.Medidas) {
			t.Errorf("conMedidas tocó el mapa de entrada: %v", c.Medidas)
		}
	}
	for _, c := range v.AnchoAjustado {
		anchos := make([]float32, len(c.Anchos))
		for i, a := range c.Anchos {
			anchos[i] = float32(math.NaN())
			if a != nil {
				anchos[i] = *a
			}
		}
		if got := anchoAjustado(anchos, AnchoCelda); got != c.Sale {
			t.Errorf("anchoAjustado(%v) = %v; la web dice %v", anchos, got, c.Sale)
		}
	}
	for _, c := range v.AltoAjustado {
		if got := altoAjustado(c.Tamanos, AltoCelda); got != c.Sale {
			t.Errorf("altoAjustado(%v) = %v; la web dice %v", c.Tamanos, got, c.Sale)
		}
	}
}

func maps2(m map[string]float64) map[string]float64 {
	out := map[string]float64{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

// tramosDe son los índices sueltos en tramos de uno, que es como los guarda la hoja.
func tramosDe(indices []int) []analisis.Tramo {
	out := make([]analisis.Tramo, 0, len(indices))
	for _, i := range indices {
		out = append(out, analisis.Tramo{Start: i, End: i})
	}
	return out
}

// TestLaGeometriaConMedidasEsLaDeLaWeb es LA prueba de la pieza: un libro con anchos y
// altos importados (y alguno roto), con filas y columnas ocultas, se pinta con las MISMAS
// posiciones que calcula SheetCanvas (`colX` y `rowTop`), y bajo cada coordenada cae la
// misma columna y la misma fila.
//
// Visto en rojo con la rejilla de antes, de medida fija: la columna B empezaba en 136 y no
// en 190.
func TestLaGeometriaConMedidasEsLaDeLaWeb(t *testing.T) {
	v := leerVectoresMedidas(t)
	for n, g := range v.Geometrias {
		hoja := map[string]any{"name": "H", "cells": map[string]string{}}
		if g.ColWidths != nil {
			hoja["colWidths"] = g.ColWidths
		}
		if g.RowHeights != nil {
			hoja["rowHeights"] = g.RowHeights
		}
		if len(g.OcultasCols) > 0 {
			hoja["hiddenCols"] = tramosDe(g.OcultasCols)
		}
		if len(g.OcultasFilas) > 0 {
			hoja["hiddenRows"] = tramosDe(g.OcultasFilas)
		}
		doc, _ := json.Marshal(map[string]any{"active": 0, "sheets": []any{hoja}})
		lz := lienzoDePrueba()
		if !lz.Cargar(string(doc)) {
			t.Fatalf("caso %d: no se abre %s", n, doc)
		}
		r := lz.Rejilla
		vw := r.vista()
		for c, x := range g.ColX {
			if got := vw.x(c); got != x {
				t.Errorf("caso %d: la columna %s empieza en %v y en la web en %v", n, Ref(c, 0), got, x)
			}
		}
		k := 0
		for f := range g.NRows + 1 {
			if slices.Contains(g.OcultasFilas, f) {
				continue
			}
			if got := vw.y(f); got != g.RowTop[k] {
				t.Errorf("caso %d: la fila %d empieza en %v y en la web en %v", n, f+1, got, g.RowTop[k])
			}
			k++
		}
		// La web acota la búsqueda a su rejilla; la nativa sigue de serie más allá. Se
		// compara lo que cae dentro.
		for i, x := range g.Xs {
			if x < g.ColX[len(g.ColX)-1] {
				if got := r.colEn(x); got != g.ColEn[i] {
					t.Errorf("caso %d: en x=%v cae la columna %d y en la web la %d", n, x, got, g.ColEn[i])
				}
			}
		}
		for i, y := range g.Ys {
			if y < g.RowTop[len(g.RowTop)-1] {
				if got := r.filaEn(y); got != g.FilaEn[i] {
					t.Errorf("caso %d: en y=%v cae la fila %d y en la web la %d", n, y, got, g.FilaEn[i])
				}
			}
		}
	}
}

// TestUnAnchoImportadoSePinta: con la columna A a 200 px, el relleno de B se pinta donde
// la web lo pinta (de 240 a 336) y la fila 1 a 60 de alto baja la 2.
//
// Visto en rojo con la rejilla de medida fija: en x=300 caía la C, sin relleno.
func TestUnAnchoImportadoSePinta(t *testing.T) {
	lz := lienzoDePrueba()
	doc := `{"active":0,"sheets":[{"name":"H","cells":{},"colWidths":{"0":200},"rowHeights":{"0":60},` +
		`"styles":{"B2":{"fill":"#FEF08A"}}}]}`
	if !lz.Cargar(doc) {
		t.Fatal("no se abre el libro")
	}
	r := lz.Rejilla
	r.Move(fyne.NewPos(0, 0)) // la rejilla sola, sin la barra de fórmulas encima
	r.Resize(fyne.NewSize(500, 200))
	r.Seleccionar(2, 3, false) // lejos de lo que se mira, y sin desplazar la vista
	img := pinta(t, 500, 200, r)
	guarda(t, img)
	amarillo := color.NRGBA{R: 0xFE, G: 0xF0, B: 0x8A, A: 0xff}
	if got := px(img, 300, AltoCelda+60+12); !cerca(got, amarillo) {
		t.Errorf("B2 debería ir de 240 a 336 y de 84 a 108; en (300, 96) hay %v", got)
	}
	if got := px(img, 220, AltoCelda+60+12); cerca(got, amarillo) {
		t.Error("en x=220 todavía es la columna A, y sale el relleno de B")
	}
	if got := px(img, 300, AltoCelda+30); cerca(got, amarillo) {
		t.Error("la fila 1 mide 60: en y=54 aún no es la fila 2")
	}
}

// lienzoEnVentana es un lienzo con una ventana de pruebas, para los gestos.
func lienzoEnVentana(t *testing.T, doc string) *Lienzo {
	t.Helper()
	lz := lienzoTraducido(t)
	if doc != "" && !lz.Cargar(doc) {
		t.Fatalf("no se abre %s", doc)
	}
	w := test.NewTempWindow(t, lz)
	w.Resize(fyne.NewSize(900, 500))
	lz.Rejilla.Refresh()
	return lz
}

// TestArrastrarElBordeCambiaElAncho: el borde de la cabecera de B pone el cursor de
// redimensionar; arrastrarlo 50 px a la derecha deja B en 146 mientras se arrastra (la
// previa) y al soltar lo guarda en `colWidths`, en un paso de deshacer.
func TestArrastrarElBordeCambiaElAncho(t *testing.T) {
	lz := lienzoEnVentana(t, "")
	r := lz.Rejilla
	borde := fyne.NewPos(r.vista().x(2), AltoCelda/2)
	r.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: borde}})
	if r.Cursor() != desktop.HResizeCursor {
		t.Fatalf("sobre el borde de B el cursor es %v", r.Cursor())
	}
	r.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: borde.AddXY(0, AltoCelda*3)}})
	if r.Cursor() != desktop.CrosshairCursor {
		t.Fatal("dentro de las celdas el cursor sigue siendo el de redimensionar")
	}

	r.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: borde}, Button: desktop.MouseButtonPrimary})
	r.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: borde.AddXY(50, 3)}, Dragged: fyne.NewDelta(50, 3)})
	if got := r.vista().ancho(1); got != 146 {
		t.Errorf("mientras se arrastra, B mide %v y no los 146 de la previa", got)
	}
	if anchos, _ := lz.Libro.Medidas(); anchos != nil {
		t.Errorf("la previa no se guarda hasta soltar: %v", anchos)
	}
	r.DragEnd()
	if anchos, _ := lz.Libro.Medidas(); anchos["1"] != 146 || len(anchos) != 1 {
		t.Fatalf("al soltar, colWidths es %v", anchos)
	}
	if !strings.Contains(lz.Serializar(), `"colWidths":{"1":146}`) {
		t.Errorf("el ancho no se guarda en el documento: %s", lz.Serializar())
	}
	if r.sel != (Celda{}) || r.ext != nil {
		t.Errorf("redimensionar no selecciona: la selección es %v %v", r.sel, r.ext)
	}
	lz.Mandar("undo", "")
	if anchos, _ := lz.Libro.Medidas(); anchos != nil {
		t.Errorf("deshacer no devuelve B a la de serie: %v", anchos)
	}

	// Una fila, por su borde de abajo: 24 + 16 = 40.
	bordeFila := fyne.NewPos(AnchoCabecera/2, r.vista().y(1))
	r.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: bordeFila}, Button: desktop.MouseButtonPrimary})
	r.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: bordeFila.AddXY(0, 16)}, Dragged: fyne.NewDelta(0, 16)})
	r.DragEnd()
	if _, altos := lz.Libro.Medidas(); altos["0"] != 40 {
		t.Errorf("arrastrar el borde de la fila 1 deja rowHeights en %v", altos)
	}
}

// TestElDobleClicEnElBordeAjustaLaColumna: el doble clic en el borde de A la ajusta al
// texto más ancho más el aire, y con la columna entera de B y C seleccionada, el gesto
// sobre B vale para las dos.
func TestElDobleClicEnElBordeAjustaLaColumna(t *testing.T) {
	lz := lienzoEnVentana(t, "")
	r := lz.Rejilla
	largo := "Un texto bastante más largo que la columna"
	lz.Libro.Escribir(0, 3, largo)
	lz.Libro.Escribir(0, 5, "corto")
	r.DoubleTapped(&fyne.PointEvent{Position: fyne.NewPos(r.vista().x(1)+1, AltoCelda/2)})
	cara, estilo := caraCelda("", false, false)
	mide, _ := fyne.CurrentApp().Driver().RenderedTextSize(largo, EscalonCelda.Tam, estilo, cara)
	quiero := float64(math.Ceil(float64(mide.Width + AireAjuste)))
	if anchos, _ := lz.Libro.Medidas(); anchos["0"] != quiero {
		t.Errorf("el autoajuste de A da %v y el texto más ancho pide %v", anchos, quiero)
	}
	if r.editando {
		t.Error("el doble clic en el borde abrió una celda")
	}

	// B:C enteras, y el gesto sobre B: las dos (vacías, vuelven a la de serie, que ya
	// tenían: no cambia nada y no se gasta un paso de deshacer).
	lz.Libro.Escribir(2, 0, largo)
	r.Seleccionar(1, 0, false)
	r.ext = &Celda{C: 2, F: r.Fils - 1}
	if got := r.unidadesDelGesto(true, 1); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("con B:C enteras el gesto va a %v", got)
	}
	if got := r.unidadesDelGesto(true, 4); !slices.Equal(got, []int{4}) {
		t.Errorf("fuera de la selección el gesto va a %v", got)
	}
}

// TestLosComandosDeMedidaVanALaSeleccion: `rowheight` con «40» pone las filas de la
// selección a 40; «auto» las ajusta a su letra mayor; lo que no se lee avisa y no toca
// nada; y la hoja protegida sin permiso de formato lo veta.
func TestLosComandosDeMedidaVanALaSeleccion(t *testing.T) {
	lz := lienzoEnVentana(t, "")
	r := lz.Rejilla
	var aviso string
	lz.AlAviso = func(_ ui.TonoAviso, titulo, _ string) { aviso = titulo }
	r.Seleccionar(0, 1, false)
	r.ext = &Celda{C: 3, F: 2}
	lz.Mandar(CmdAltoFila, "40")
	if _, altos := lz.Libro.Medidas(); altos["1"] != 40 || altos["2"] != 40 || len(altos) != 2 {
		t.Errorf("«40» deja rowHeights en %v", altos)
	}
	lz.Mandar(CmdAnchoCol, "120,5px")
	if anchos, _ := lz.Libro.Medidas(); anchos["0"] != 121 || anchos["3"] != 121 || len(anchos) != 4 {
		t.Errorf("«120,5px» deja colWidths en %v", anchos)
	}
	lz.Mandar(CmdAnchoCol, "ancho")
	if aviso != lz.t("sheet.resize.invalid", nil) {
		t.Errorf("una medida que no se lee avisa con %q", aviso)
	}
	if anchos, _ := lz.Libro.Medidas(); anchos["0"] != 121 {
		t.Errorf("una medida que no se lee cambió colWidths: %v", anchos)
	}
	// Auto: la fila 2 con una letra de 20 pt pide ceil(20·4/3·1,3 + 6) = 41; la 3, vacía,
	// vuelve a la de serie.
	lz.Libro.Escribir(0, 1, "grande")
	lz.Mandar(CmdEstilo, `{"size":20}`)
	lz.Mandar(CmdAltoFila, "auto")
	if _, altos := lz.Libro.Medidas(); altos["1"] != 41 || len(altos) != 1 {
		t.Errorf("«auto» deja rowHeights en %v", altos)
	}
	// Una fila alta empuja a las de debajo, y la celda de la 3 empieza donde acaba la 2.
	if vw := r.vista(); vw.y(2) != vw.y(1)+41 {
		t.Errorf("la fila 3 empieza en %v y la 2 acaba en %v", vw.y(2), vw.y(1)+41)
	}

	lz.Mandar(CmdProteger, `{}`)
	lz.Mandar(CmdAltoFila, "80")
	if _, altos := lz.Libro.Medidas(); altos["1"] != 41 {
		t.Errorf("con la hoja protegida sin permiso de formato, rowHeights pasa a %v", altos)
	}
}

// TestElBordeJuntoALasFijasEsElDeLaUltimaFija: con A:B inmovilizadas y la hoja desplazada,
// la primera columna que se ve tras las fijas no es la del puesto anterior (tapada), así
// que el asa a la izquierda de su borde es la de B, la que se ve ahí.
func TestElBordeJuntoALasFijasEsElDeLaUltimaFija(t *testing.T) {
	lz := lienzoEnVentana(t, "")
	r := lz.Rejilla
	lz.Libro.Fijar(2, 0)
	r.desX = 3 * AnchoCelda // C, D y E quedan bajo las fijas: la primera que se ve es F
	r.Refresh()
	vw := r.vista()
	if got := r.colEn(vw.x(5) + 1); got != 5 {
		t.Fatalf("tras las fijas se ve %d y no F", got)
	}
	col, i, ok := r.asaMedidaEn(fyne.NewPos(vw.x(5)+1, AltoCelda/2))
	if !ok || !col || i != 1 {
		t.Errorf("el asa junto a las fijas es %v %d %v y no la de B", col, i, ok)
	}
}

// TestLaPreviaEsDeTodasLasDelGesto: con B:C enteras seleccionadas, arrastrar el borde de
// B pinta ya las dos con la medida nueva, que es lo que guarda al soltar.
func TestLaPreviaEsDeTodasLasDelGesto(t *testing.T) {
	lz := lienzoEnVentana(t, "")
	r := lz.Rejilla
	r.Seleccionar(1, 0, false)
	r.ext = &Celda{C: 2, F: r.Fils - 1}
	borde := fyne.NewPos(r.vista().x(2), AltoCelda/2)
	r.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: borde}, Button: desktop.MouseButtonPrimary})
	r.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: borde.AddXY(20, 0)}, Dragged: fyne.NewDelta(20, 0)})
	if vw := r.vista(); vw.ancho(1) != 116 || vw.ancho(2) != 116 {
		t.Errorf("la previa deja B en %v y C en %v, no las dos en 116", vw.ancho(1), vw.ancho(2))
	}
	r.DragEnd()
	if anchos, _ := lz.Libro.Medidas(); anchos["1"] != 116 || anchos["2"] != 116 {
		t.Errorf("al soltar, colWidths es %v", anchos)
	}
}
