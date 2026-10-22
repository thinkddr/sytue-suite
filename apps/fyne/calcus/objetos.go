// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"crypto/rand"
	"fmt"
	"math"
	"slices"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// # Los objetos de la hoja: minigráficos, gráficos y segmentaciones
//
// Son los `Sparkline`, `ChartSpec` y `Slicer` de `types.ts`, con los saneados de
// `sheets/format.ts` y `sheets/slicer.ts`: lo que la web descartaría al cargar, aquí
// también, y lo que se guarda sale con las mismas claves y sin los valores por defecto.
// Las tablas con formato, que traen más lógica, van en `tablas.go`.

// Minigrafico es `Sparkline`: un gráfico diminuto dentro de la celda anfitriona.
type Minigrafico struct {
	Cell  string `json:"cell"`
	Range string `json:"range"`
	Type  string `json:"type"` // line, column o winloss
	Color string `json:"color,omitempty"`
}

// TiposMinigrafico son `SPARK_TYPES` con la clave de su rótulo (`SPARK_TYPE_OPTIONS`).
var TiposMinigrafico = []struct{ Valor, Clave string }{
	{"line", "spark.kind.line"}, {"column", "spark.kind.column"}, {"winloss", "spark.kind.winloss"},
}

// ColoresMinigrafico son `SPARK_COLORS`: azul, verde, rojo y ámbar de SERIE.
var ColoresMinigrafico = []string{"#2563EB", "#16A34A", "#DC2626", "#D97706"}

// minigraficoCrudo es un minigráfico tal y como llega.
type minigraficoCrudo struct {
	Cell, Range, Type, Color any
}

// sanear es `sanitizeSparkline`.
func (m minigraficoCrudo) sanear() (Minigrafico, bool) {
	celda, ok1 := m.Cell.(string)
	rango, ok2 := m.Range.(string)
	tipo, ok3 := m.Type.(string)
	if !ok1 || !ok2 || !ok3 || !rangoValido(celda) || !rangoValido(rango) ||
		!slices.ContainsFunc(TiposMinigrafico, func(t struct{ Valor, Clave string }) bool { return t.Valor == tipo }) {
		return Minigrafico{}, false
	}
	out := Minigrafico{Cell: celda, Range: rango, Type: tipo}
	if c, ok := m.Color.(string); ok && reHex.MatchString(c) {
		out.Color = c
	}
	return out, true
}

func rangoValido(s string) bool {
	_, ok := formula.ParseRangeRef(s)
	return ok
}

// Grafico es `ChartSpec`: un gráfico que flota sobre la rejilla.
type Grafico struct {
	ID    string   `json:"id"`
	Type  string   `json:"type"`
	Range string   `json:"range"`
	Title string   `json:"title,omitempty"`
	X     *float64 `json:"x,omitempty"`
	Y     *float64 `json:"y,omitempty"`
	// El contrato es el SheetData de la web, que va en camelCase.
	LineSeries    []int `json:"lineSeries,omitempty"`    //nolint:tagliatelle // el nombre lo fija la web
	SecondaryAxis []int `json:"secondaryAxis,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	// El tamaño del ÁREA del gráfico en px; nil es el de por defecto (278 × 156,375).
	W *float64 `json:"w,omitempty"`
	H *float64 `json:"h,omitempty"`
	// Las series por filas, apilado, leyenda, etiquetas de datos, retícula, títulos de
	// los ejes y colores por serie. Lo ausente es lo de siempre.
	ByRow   bool     `json:"byRow,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	Stacked bool     `json:"stacked,omitempty"`
	Legend  string   `json:"legend,omitempty"`
	Labels  bool     `json:"labels,omitempty"`
	Grid    *bool    `json:"grid,omitempty"`
	XTitle  string   `json:"xTitle,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	YTitle  string   `json:"yTitle,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	Colors  []string `json:"colors,omitempty"`
}

// TiposGrafico son `CHART_TYPES`, en el orden del selector de la cinta
// (`CHART_TYPE_OPTIONS`), con su clave.
var TiposGrafico = []struct{ Valor, Clave string }{
	{"bar", "chart.kind.bar"},
	{"hbar", "chart.kind.hbar"},
	{"line", "chart.kind.line"},
	{"area", "chart.kind.area"},
	{"combo", "chart.kind.combo"},
	{"pie", "chart.kind.pie"},
	{"donut", "chart.kind.doughnut"},
	{"scatter", "chart.kind.scatter"},
	{"radar", "chart.kind.radar"},
}

func tipoGraficoValido(t string) bool {
	return slices.ContainsFunc(TiposGrafico, func(x struct{ Valor, Clave string }) bool { return x.Valor == t })
}

// TopePosicion es `CHART_POS_MAX`: el x o el y más lejano de una tarjeta.
const TopePosicion = 2200

// TamanoMinGrafico y TamanoMaxGrafico son `CHART_SIZE_MIN/MAX`: los topes del área.
const (
	TamanoMinGrafico = 120
	TamanoMaxGrafico = 1400
)

// graficoCrudo es un gráfico tal y como llega.
type graficoCrudo struct {
	ID, Type, Range, Title, X, Y, W, H any
	LineSeries                         any `json:"lineSeries"`    //nolint:tagliatelle // el nombre lo fija la web
	SecondaryAxis                      any `json:"secondaryAxis"` //nolint:tagliatelle // el nombre lo fija la web
	ByRow                              any `json:"byRow"`         //nolint:tagliatelle // el nombre lo fija la web
	Stacked, Legend, Labels, Grid      any
	XTitle                             any `json:"xTitle"` //nolint:tagliatelle // el nombre lo fija la web
	YTitle                             any `json:"yTitle"` //nolint:tagliatelle // el nombre lo fija la web
	Colors                             any
}

// sanear es `sanitizeChartSpec`: un id que falta se inventa, como hace la web con los
// documentos editados a mano.
func (g graficoCrudo) sanear() (Grafico, bool) {
	tipo, ok1 := g.Type.(string)
	rango, ok2 := g.Range.(string)
	if !ok1 || !ok2 || !tipoGraficoValido(tipo) || !rangoValido(rango) {
		return Grafico{}, false
	}
	out := Grafico{Type: tipo, Range: rango}
	if id, ok := g.ID.(string); ok && id != "" {
		out.ID = id
	} else {
		out.ID = nuevoID()
	}
	if t, ok := g.Title.(string); ok && t != "" {
		out.Title = t
	}
	out.X, out.Y = posicion(g.X), posicion(g.Y)
	out.LineSeries, out.SecondaryAxis = indicesSerie(g.LineSeries), indicesSerie(g.SecondaryAxis)
	out.W, out.H = tamanoGrafico(g.W), tamanoGrafico(g.H)
	out.ByRow, out.Stacked, out.Labels = g.ByRow == true, g.Stacked == true, g.Labels == true
	if l, ok := g.Legend.(string); ok && (l == "top" || l == "right" || l == "none") {
		out.Legend = l
	}
	if b, ok := g.Grid.(bool); ok && !b {
		out.Grid = &b
	}
	out.XTitle, out.YTitle = tituloDeEje(g.XTitle), tituloDeEje(g.YTitle)
	out.Colors = coloresDeSerie(g.Colors)
	return out, true
}

// tamanoGrafico es `clampChartSize` de un número finito; nil si no lo es.
func tamanoGrafico(v any) *float64 {
	x, ok := v.(float64)
	if !ok || math.IsNaN(x) || math.IsInf(x, 0) {
		return nil
	}
	t := min(TamanoMaxGrafico, max(TamanoMinGrafico, redondearJS(x)))
	return &t
}

// tituloDeEje es `cleanAxisTitle`: recortado a 80 caracteres; "" si no queda nada.
func tituloDeEje(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	r := []rune(jsTrimCalc(s))
	return string(r[:min(len(r), 80)])
}

// coloresDeSerie es `cleanSeriesColors`: #RRGGBB o "" por serie, 26 como mucho, sin
// los vacíos del final.
func coloresDeSerie(v any) []string {
	lista, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, min(len(lista), Columnas))
	for _, x := range lista[:min(len(lista), Columnas)] {
		c, _ := x.(string)
		if !reHex.MatchString(c) {
			c = ""
		}
		out = append(out, c)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// posicion es `clampChartPos` de un número finito; nil si no lo es.
func posicion(v any) *float64 {
	x, ok := v.(float64)
	if !ok || math.IsNaN(x) || math.IsInf(x, 0) {
		return nil
	}
	p := min(TopePosicion, max(0, redondearJS(x)))
	return &p
}

// indicesSerie es `cleanSeriesIdx`: enteros de 0 a MaxColumnas-1, sin repetir y en
// orden; nil si no queda ninguno.
func indicesSerie(v any) []int {
	lista, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []int
	for _, x := range lista {
		n, ok := x.(float64)
		if ok && n == math.Trunc(n) && n >= 0 && n < MaxColumnas && !slices.Contains(out, int(n)) {
			out = append(out, int(n))
		}
	}
	slices.Sort(out)
	return out
}

// Segmentacion es `Slicer`: la tarjeta que filtra una columna del autofiltro o, con
// Pivot, un campo (Col, del origen) de esa tabla dinámica de la hoja.
type Segmentacion struct {
	ID    string   `json:"id"`
	Col   int      `json:"col"`
	Pivot string   `json:"pivot,omitempty"`
	X     *float64 `json:"x,omitempty"`
	Y     *float64 `json:"y,omitempty"`
}

// segmentacionCruda es una segmentación tal y como llega.
type segmentacionCruda struct {
	ID, Col, Pivot, X, Y any
}

// sanear es `sanitizeSlicer`.
func (s segmentacionCruda) sanear() (Segmentacion, bool) {
	col, ok := numeroEntero(s.Col)
	if !ok || col < 0 || col >= MaxColumnas {
		return Segmentacion{}, false
	}
	out := Segmentacion{Col: col, X: posicion(s.X), Y: posicion(s.Y)}
	if p, ok := s.Pivot.(string); ok {
		out.Pivot = p
	}
	if id, ok := s.ID.(string); ok && id != "" {
		out.ID = id
	} else {
		out.ID = nuevoID()
	}
	return out, true
}

// numeroEntero es `Math.trunc(Number(v))` cuando da un entero finito.
func numeroEntero(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, false
		}
		return int(math.Trunc(x)), true
	case string:
		t := jsTrimCalc(x)
		if t == "" {
			return 0, true // `Number('')` es 0
		}
		n, err := strconv.ParseFloat(t, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		return int(math.Trunc(n)), true
	case bool:
		return enteroJS(x), true
	}
	// ponytail: `null` y una clave que falta llegan igual (nil); la web da 0 al primero y
	// NaN al segundo. Se toma la ausencia, que es lo que escribe cualquiera a mano.
	return 0, false
}

// nuevoID es `crypto.randomUUID()`: un UUID de versión 4.
func nuevoID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
