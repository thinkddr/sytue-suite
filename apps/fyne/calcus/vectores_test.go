// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Los VECTORES COMPARTIDOS con la web: `packages/office/test/vectores/calcus.json`, que
// calculan las funciones puras de la web (`vectores/calcus.ts`) y que su propio test
// comprueba. Aquí se exige que el puerto dé EXACTAMENTE lo mismo con las mismas
// entradas: el mismo caso, el mismo resultado en los dos lados.
const rutaVectores = "../../frontend/packages/office/test/vectores/calcus.json"

type vectores struct {
	Tablas struct {
		Sanear []struct {
			Entrada json.RawMessage
			Sale    json.RawMessage
		}
		Totales []struct {
			Fn      string
			Valores []json.RawMessage
			Sale    json.RawMessage
		}
		Cabeceras []struct {
			Entrada []string
			Sale    []string
		}
		NombreLibre []struct {
			Existentes []string
			Sale       string
		} `json:"nombreLibre"` //nolint:tagliatelle // el nombre lo fija la web
		Info []struct {
			Tablas []TablaFmt
			C, F   int
			Sale   *struct {
				Tabla int
				Papel string
				Banda bool
				Col   int
			}
		}
		Crecer []struct {
			Tabla TablaFmt
			C, F  int
			Sale  *struct{ C1, R1, C2, R2 int }
		}
		Resolver struct {
			Hojas []struct {
				Name   string
				Tables []TablaFmt
			}
			Cabeceras map[string]string
			Casos     []struct {
				Q struct {
					Table   string
					Column  *string
					Area    string
					ThisRow bool `json:"thisRow"` //nolint:tagliatelle // el nombre lo fija la web
				}
				Aqui struct {
					Sheet    string
					Col, Row int
				}
				Sale json.RawMessage
			}
		}
		Corchetes []struct {
			Texto  string
			Cursor int
			Sale   *struct {
				Table, Prefix string
				Start, End    int
				ThisRow       bool `json:"thisRow"` //nolint:tagliatelle // el nombre lo fija la web
			}
		}
		Refs []struct {
			Texto string
			Sale  []struct {
				Text       string
				Start, End int
				Sheet      string
				Rect       struct{ C1, R1, C2, R2 int }
				ColorIndex int `json:"colorIndex"` //nolint:tagliatelle // el nombre lo fija la web
			}
		}
	}
	Graficos struct {
		Hoja   [][]json.RawMessage
		Vistas []struct {
			Spec json.RawMessage
			Sale json.RawMessage
		}
		Sanear []struct {
			Entrada json.RawMessage
			Sale    json.RawMessage
		}
	}
	Funciones struct {
		Datos map[string]float64
		Casos []struct {
			F    string
			Sale json.RawMessage
		}
	}
	MoverObjetos struct {
		Hoja  json.RawMessage
		Casos []struct {
			D struct {
				Eje             string
				Desde, Cantidad int
				Tope            int
			}
			Sale json.RawMessage
		}
	} `json:"moverObjetos"` //nolint:tagliatelle // el nombre lo fija la web
	Minigraficos struct {
		Geometria []struct {
			Valores []float64
			Tipo    string
			Sale    struct {
				Points [][2]float64
				Bars   []struct {
					X, Y, W, H float64
					Neg        bool
				}
			}
		}
		Sanear []struct {
			Entrada json.RawMessage
			Sale    *Minigrafico
		}
	}
}

func leerVectores(t *testing.T) vectores {
	t.Helper()
	b, err := os.ReadFile(rutaVectores)
	if err != nil {
		t.Fatalf("no se leen los vectores de la web: %v", err)
	}
	var v vectores
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("los vectores no se entienden: %v", err)
	}
	return v
}

// comoJSON normaliza para comparar: lo que sale en Go, pasado por JSON y leído a `any`.
func comoJSON(t *testing.T, x any) any {
	t.Helper()
	b, err := json.Marshal(x)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

func crudoJSON(raw json.RawMessage) any {
	var out any
	_ = json.Unmarshal(raw, &out)
	return out
}

// valorDe lee un valor de celda de los vectores: número, texto, booleano o {error}.
func valorDe(raw json.RawMessage) formula.Value {
	var x any
	_ = json.Unmarshal(raw, &x)
	if m, ok := x.(map[string]any); ok {
		code, _ := m["error"].(string)
		return formula.Error(code)
	}
	return x
}

func TestVectoresDeTablas(t *testing.T) {
	v := leerVectores(t)
	for _, c := range v.Tablas.Sanear {
		var crudo tablaCruda
		var got *TablaFmt
		if json.Unmarshal(c.Entrada, &crudo) == nil {
			got = crudo.sanear()
		}
		if !reflect.DeepEqual(comoJSON(t, got), crudoJSON(c.Sale)) {
			t.Errorf("sanitizeTable(%s) = %v, la web dice %s", c.Entrada, comoJSON(t, got), c.Sale)
		}
	}
	for _, c := range v.Tablas.Totales {
		vals := make([]formula.Value, len(c.Valores))
		for i, raw := range c.Valores {
			vals[i] = valorDe(raw)
		}
		if got, want := valorTotal(c.Fn, vals), valorDe(c.Sale); !reflect.DeepEqual(got, want) {
			t.Errorf("tableTotalValue(%s, %v) = %#v, la web dice %#v", c.Fn, vals, got, want)
		}
	}
	for _, c := range v.Tablas.Cabeceras {
		if got := nombresCabecera(c.Entrada); !reflect.DeepEqual(got, c.Sale) {
			t.Errorf("uniqueHeaderNames(%q) = %q, la web dice %q", c.Entrada, got, c.Sale)
		}
	}
	for _, c := range v.Tablas.NombreLibre {
		if got := nombreTablaLibre(c.Existentes); got != c.Sale {
			t.Errorf("uniqueTableName(%q) = %q, la web dice %q", c.Existentes, got, c.Sale)
		}
	}
	for _, c := range v.Tablas.Info {
		compiladas := compilarTablas(c.Tablas)
		info, ok := infoTablaEn(compiladas, c.C, c.F)
		if ok != (c.Sale != nil) {
			t.Errorf("tableCellInfo(%v, %d, %d): ¿en tabla? %v, la web dice %v", c.Tablas, c.C, c.F, ok, c.Sale != nil)
			continue
		}
		if !ok {
			continue
		}
		i := -1
		for k, ct := range compiladas {
			if ct.t.Name == info.ct.t.Name && ct.t.Range == info.ct.t.Range {
				i = k
			}
		}
		if i != c.Sale.Tabla || info.papel != c.Sale.Papel || info.banda != c.Sale.Banda || info.col != c.Sale.Col {
			t.Errorf("tableCellInfo(%v, %d, %d) = tabla %d %s banda %v col %d, la web dice %+v",
				c.Tablas, c.C, c.F, i, info.papel, info.banda, info.col, *c.Sale)
		}
	}
	for _, c := range v.Tablas.Crecer {
		ct, _ := compilarTabla(c.Tabla)
		r, ok := crecerPorEdicion(ct, c.C, c.F, Columnas-1, Filas-1)
		if ok != (c.Sale != nil) || (ok && (r.C1 != c.Sale.C1 || r.R1 != c.Sale.R1 || r.C2 != c.Sale.C2 || r.R2 != c.Sale.R2)) {
			t.Errorf("expandRangeForEdit(%s, %d, %d) = %v %v, la web dice %v", c.Tabla.Range, c.C, c.F, r, ok, c.Sale)
		}
	}
}

// libroDeVectores monta las hojas del libro de las estructuradas.
func libroDeVectores(v vectores) []tablaResoluble {
	hojas := make([]hojaLibro, len(v.Tablas.Resolver.Hojas))
	for i, h := range v.Tablas.Resolver.Hojas {
		hojas[i] = hojaLibro{nombre: h.Name, tablas: h.Tables}
	}
	return tablasResolubles(hojas, func(hoja, c, f int) string {
		return v.Tablas.Resolver.Cabeceras[hojas[hoja].nombre+"!"+strconv.Itoa(c)+","+strconv.Itoa(f)]
	})
}

func TestVectoresDeEstructuradas(t *testing.T) {
	v := leerVectores(t)
	libro := libroDeVectores(v)
	for _, c := range v.Tablas.Resolver.Casos {
		q := formula.TableQuery{Table: c.Q.Table, Area: formula.TableArea(c.Q.Area), ThisRow: c.Q.ThisRow}
		if c.Q.Column != nil {
			q.Column, q.HasColumn = *c.Q.Column, true
		}
		rr, err := resolverEstructurada(libro, q, formula.Here{Sheet: c.Aqui.Sheet, Col: c.Aqui.Col, Row: c.Aqui.Row})
		var got any
		switch {
		case err != "":
			got = map[string]any{"error": string(err)}
		case rr != nil:
			got = map[string]any{"sheet": rr.Sheet, "c1": rr.C1, "r1": rr.R1, "c2": rr.C2, "r2": rr.R2}
		}
		if !reflect.DeepEqual(comoJSON(t, got), crudoJSON(c.Sale)) {
			t.Errorf("resolveStructuredRef(%+v, %+v) = %v, la web dice %s", c.Q, c.Aqui, comoJSON(t, got), c.Sale)
		}
	}
	for _, c := range v.Tablas.Corchetes {
		b, ok := formula.TableBracketAt(c.Texto, c.Cursor)
		if ok != (c.Sale != nil) || (ok && (b.Tabla != c.Sale.Table || b.Prefijo != c.Sale.Prefix ||
			b.Inicio != c.Sale.Start || b.Fin != c.Sale.End || b.EstaFila != c.Sale.ThisRow)) {
			t.Errorf("tableBracketAt(%q, %d) = %+v %v, la web dice %+v", c.Texto, c.Cursor, b, ok, c.Sale)
		}
	}
	resolver := func(q formula.TableQuery) (string, formula.Rect, bool) {
		rr, err := resolverEstructurada(libro, q, formula.Here{Sheet: "Hoja1", Col: 1, Row: 2})
		if err != "" || rr == nil {
			return "", formula.Rect{}, false
		}
		return rr.Sheet, formula.Rect{C1: rr.C1, R1: rr.R1, C2: rr.C2, R2: rr.R2}, true
	}
	for _, c := range v.Tablas.Refs {
		got := formula.ExtractRefsCon(c.Texto, resolver)
		if len(got) != len(c.Sale) {
			t.Errorf("extractRefs(%q) da %d referencias, la web %d: %+v", c.Texto, len(got), len(c.Sale), got)
			continue
		}
		for i, r := range got {
			w := c.Sale[i]
			if r.Texto != w.Text || r.Inicio != w.Start || r.Fin != w.End || r.Hoja != w.Sheet || r.Color != w.ColorIndex ||
				r.Rect != (formula.Rect{C1: w.Rect.C1, R1: w.Rect.R1, C2: w.Rect.C2, R2: w.Rect.R2}) {
				t.Errorf("extractRefs(%q)[%d] = %+v, la web dice %+v", c.Texto, i, r, w)
			}
		}
	}
}

func TestVectoresDeMinigraficos(t *testing.T) {
	v := leerVectores(t)
	for _, c := range v.Minigraficos.Geometria {
		g := geometriaMinigrafico(c.Valores, c.Tipo)
		if len(g.Puntos) != len(c.Sale.Points) || len(g.Barras) != len(c.Sale.Bars) {
			t.Errorf("sparklineGeom(%v, %s) = %+v, la web dice %+v", c.Valores, c.Tipo, g, c.Sale)
			continue
		}
		for i, p := range g.Puntos {
			if p != c.Sale.Points[i] {
				t.Errorf("sparklineGeom(%v, %s) punto %d = %v, la web dice %v", c.Valores, c.Tipo, i, p, c.Sale.Points[i])
			}
		}
		for i, b := range g.Barras {
			w := c.Sale.Bars[i]
			if b != (barraMini{X: w.X, Y: w.Y, W: w.W, H: w.H, Neg: w.Neg}) {
				t.Errorf("sparklineGeom(%v, %s) barra %d = %+v, la web dice %+v", c.Valores, c.Tipo, i, b, w)
			}
		}
	}
	for _, c := range v.Minigraficos.Sanear {
		var crudo minigraficoCrudo
		m, ok := Minigrafico{}, false
		if json.Unmarshal(c.Entrada, &crudo) == nil {
			m, ok = crudo.sanear()
		}
		if ok != (c.Sale != nil) || (ok && m != *c.Sale) {
			t.Errorf("sanitizeSparkline(%s) = %+v %v, la web dice %+v", c.Entrada, m, ok, c.Sale)
		}
	}
}

// aJSON es la marca con las claves de `ChartMark`: solo las que tiene su clase.
func (m marca) aJSON() map[string]any {
	o := map[string]any{"k": m.K}
	opc := func() {
		if m.Op != 0 {
			o["op"] = m.Op
		}
		if m.Stroke != "" {
			o["stroke"] = m.Stroke
		}
		if m.SW != 0 {
			o["sw"] = m.SW
		}
		if m.Title != "" {
			o["title"] = m.Title
		}
	}
	switch m.K {
	case "rect":
		o["x"], o["y"], o["w"], o["h"], o["fill"] = m.X, m.Y, m.W, m.H, m.Fill
		if m.Title != "" {
			o["title"] = m.Title
		}
	case "path":
		o["d"], o["fill"] = m.D, m.Fill
		opc()
		if m.Round {
			o["round"] = true
		}
	case "circle":
		o["cx"], o["cy"], o["r"], o["fill"] = m.CX, m.CY, m.R, m.Fill
		opc()
	case "line":
		o["x1"], o["y1"], o["x2"], o["y2"], o["stroke"] = m.X1, m.Y1, m.X2, m.Y2, m.Stroke
	default:
		o["x"], o["y"], o["text"], o["size"], o["fill"], o["anchor"] = m.X, m.Y, m.Text, m.Size, m.Fill, m.Anchor
	}
	return o
}

func TestVectoresDeGraficos(t *testing.T) {
	v := leerVectores(t)
	get := func(c, f int) formula.Value {
		if f < len(v.Graficos.Hoja) && c < len(v.Graficos.Hoja[f]) {
			return valorDe(v.Graficos.Hoja[f][c])
		}
		return ""
	}
	for _, c := range v.Graficos.Vistas {
		var crudo graficoCrudo
		if err := json.Unmarshal(c.Spec, &crudo); err != nil {
			t.Fatal(err)
		}
		g, ok := crudo.sanear()
		if !ok {
			t.Fatalf("el gráfico %s no se sanea", c.Spec)
		}
		vista := verGrafico(g, get, "Otros")
		marcas := make([]any, len(vista.Escena.Marcas))
		for i, m := range vista.Escena.Marcas {
			marcas[i] = m.aJSON()
		}
		leyenda := make([]any, len(vista.Leyenda))
		for i, it := range vista.Leyenda {
			leyenda[i] = map[string]any{"name": it.Nombre, "color": it.Color}
		}
		series := vista.Series
		if series == nil {
			series = []string{}
		}
		got := map[string]any{
			"scene":  map[string]any{"w": vista.Escena.W, "h": vista.Escena.H, "marks": marcas},
			"legend": leyenda, "seriesNames": series, "empty": vista.Vacio,
		}
		want, _ := crudoJSON(c.Sale).(map[string]any)
		gotJ, _ := comoJSON(t, got).(map[string]any)
		if reflect.DeepEqual(gotJ, want) {
			continue
		}
		// Se dice la primera diferencia, que es la que sirve.
		t.Errorf("chartView(%s) no da la escena de la web", c.Spec)
		for _, k := range []string{"legend", "seriesNames", "empty"} {
			if !reflect.DeepEqual(gotJ[k], want[k]) {
				t.Errorf("  %s: %v\n  la web: %v", k, gotJ[k], want[k])
			}
		}
		ge, _ := gotJ["scene"].(map[string]any)
		we, _ := want["scene"].(map[string]any)
		if ge["w"] != we["w"] || ge["h"] != we["h"] {
			t.Errorf("  viewBox %v×%v, la web %v×%v", ge["w"], ge["h"], we["w"], we["h"])
		}
		gm, _ := ge["marks"].([]any)
		wm, _ := we["marks"].([]any)
		for i := range max(len(gm), len(wm)) {
			var a, b any
			if i < len(gm) {
				a = gm[i]
			}
			if i < len(wm) {
				b = wm[i]
			}
			if !reflect.DeepEqual(a, b) {
				t.Errorf("  marca %d (de %d/%d):\n    aquí:   %v\n    la web: %v", i, len(gm), len(wm), a, b)
				break
			}
		}
	}
	for _, c := range v.Graficos.Sanear {
		var crudo graficoCrudo
		var got any
		if json.Unmarshal(c.Entrada, &crudo) == nil {
			if g, ok := crudo.sanear(); ok {
				got = g
			}
		}
		if !reflect.DeepEqual(comoJSON(t, got), crudoJSON(c.Sale)) {
			t.Errorf("sanitizeChartSpec(%s) = %v, la web dice %s", c.Entrada, comoJSON(t, got), c.Sale)
		}
	}
}

func TestVectoresDeMoverObjetos(t *testing.T) {
	v := leerVectores(t)
	for _, c := range v.MoverObjetos.Casos {
		var leida hojaLeida
		if err := json.Unmarshal(v.MoverObjetos.Hoja, &leida); err != nil {
			t.Fatal(err)
		}
		h := nuevaHoja(leida.Name)
		cargarObjetos(&h, leida)
		h.filtro = leida.Autofilter.sanear()
		d := formula.Desplazamiento{Filas: c.D.Eje == "fila", Desde: c.D.Desde, Cantidad: c.D.Cantidad, Tope: c.D.Tope}
		moverObjetos(&h, d)
		got := hoja{Tables: h.tablas, Sparklines: h.minis, Charts: h.graficos, Slicers: h.segs, Autofilter: h.filtro}
		m, _ := comoJSON(t, got).(map[string]any)
		delete(m, "name")
		delete(m, "cells")
		if want := crudoJSON(c.Sale); !reflect.DeepEqual(m, want) {
			t.Errorf("moverObjetos(%+v):\n  aquí:   %v\n  la web: %v", c.D, m, want)
		}
	}
}

// TestVectoresDeFunciones: las fórmulas de hoja que la web calcula (cupones irregulares,
// BESSELK, el suavizado exponencial…) dan lo mismo en el motor de Go, a 1e-9 relativo: son
// dos implementaciones del mismo algoritmo, no una misma ejecución.
func TestVectoresDeFunciones(t *testing.T) {
	v := leerVectores(t).Funciones
	if len(v.Casos) < 20 {
		t.Fatalf("solo %d vectores de funciones", len(v.Casos))
	}
	e := formula.New(formula.Options{
		Now:    func() time.Time { return time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC) },
		Random: func() float64 { return 0.5 },
	})
	for ref, n := range v.Datos {
		if err := e.SetCell("Hoja1", ref, strconv.FormatFloat(n, 'g', -1, 64)); err != nil {
			t.Fatal(err)
		}
	}
	for i, c := range v.Casos {
		if err := e.SetCell("Hoja1", "Z"+strconv.Itoa(i+1), c.F); err != nil {
			t.Fatalf("%s: %v", c.F, err)
		}
	}
	for i, c := range v.Casos {
		got, _ := e.GetValue("Hoja1", "Z"+strconv.Itoa(i+1))
		var want any
		if err := json.Unmarshal(c.Sale, &want); err != nil {
			t.Fatal(err)
		}
		switch w := want.(type) {
		case float64:
			n, ok := got.(float64)
			if !ok || math.Abs(n-w) > 1e-9*math.Max(1e-3, math.Abs(w)) {
				t.Errorf("%s = %v, y la web da %v", c.F, got, w)
			}
		case map[string]any:
			if got != formula.Error(w["error"].(string)) {
				t.Errorf("%s = %v, y la web da %v", c.F, got, w["error"])
			}
		default:
			if !reflect.DeepEqual(got, w) {
				t.Errorf("%s = %v, y la web da %v", c.F, got, w)
			}
		}
	}
}
