// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"math"
	"strconv"
	"strings"
)

// Los OBJETOS de una hoja de Calcus —tablas con formato, minigráficos, gráficos y
// segmentaciones— tal y como los guarda el editor (`SheetTable`, `Sparkline`,
// `ChartSpec` y `Slicer` de `packages/office/src/editor/types.ts`). El servicio no los
// interpreta al guardar (doc:sheet es un blob), pero sí al EXPORTAR a .xlsx/.ods y al
// IMPORTAR de ellos: ahí se traducen a lo que entienden Excel y LibreOffice (tablas,
// DrawingML, sparklines de x14 y de calcext) y de vuelta, sin perder nada.

// objetosHoja son los objetos de una SheetData; van embebidos en sheetData.
type objetosHoja struct {
	Tables     []sheetTable     `json:"tables,omitempty"`
	Sparklines []sheetSparkline `json:"sparklines,omitempty"`
	Charts     []sheetChart     `json:"charts,omitempty"`
	Slicers    []sheetSlicer    `json:"slicers,omitempty"`
}

type sheetTable struct {
	Name      string            `json:"name"`
	Range     string            `json:"range"`
	Style     string            `json:"style,omitempty"`
	Headers   *bool             `json:"headers,omitempty"`
	TotalsRow bool              `json:"totalsRow,omitempty"` //nolint:tagliatelle // el nombre lo fija el editor
	Totals    map[string]string `json:"totals,omitempty"`
}

type sheetSparkline struct {
	Cell  string `json:"cell"`
	Range string `json:"range"`
	Type  string `json:"type"`
	Color string `json:"color,omitempty"`
}

type sheetChart struct {
	ID            string   `json:"id"`
	Type          string   `json:"type"`
	Range         string   `json:"range"`
	Title         string   `json:"title,omitempty"`
	X             *float64 `json:"x,omitempty"`
	Y             *float64 `json:"y,omitempty"`
	LineSeries    []int    `json:"lineSeries,omitempty"`    //nolint:tagliatelle // el nombre lo fija el editor
	SecondaryAxis []int    `json:"secondaryAxis,omitempty"` //nolint:tagliatelle // el nombre lo fija el editor
	W             *float64 `json:"w,omitempty"`
	H             *float64 `json:"h,omitempty"`
	ByRow         bool     `json:"byRow,omitempty"` //nolint:tagliatelle // el nombre lo fija el editor
	Stacked       bool     `json:"stacked,omitempty"`
	Legend        string   `json:"legend,omitempty"`
	Labels        bool     `json:"labels,omitempty"`
	Grid          *bool    `json:"grid,omitempty"`
	XTitle        string   `json:"xTitle,omitempty"` //nolint:tagliatelle // el nombre lo fija el editor
	YTitle        string   `json:"yTitle,omitempty"` //nolint:tagliatelle // el nombre lo fija el editor
	Colors        []string `json:"colors,omitempty"`
}

type sheetSlicer struct {
	ID  string `json:"id"`
	Col int    `json:"col"`
	// Pivot es la tabla dinámica que segmenta (y Col, un campo de su origen); vacío, el
	// autofiltro de la hoja.
	Pivot string   `json:"pivot,omitempty"`
	X     *float64 `json:"x,omitempty"`
	Y     *float64 `json:"y,omitempty"`
}

// Las medidas de la rejilla del editor: celdas de 96×24 px. Las posiciones de los
// gráficos van en px de la rejilla y se anclan a la celda en la que caen.
const (
	anchoCeldaPx = 96
	altoCeldaPx  = 24
	emuPorPx     = 9525
	// El área de un gráfico por defecto (`DEFAULT_PLOT_W/H` de Chart.tsx).
	anchoGraficoPx = 278.0
	altoGraficoPx  = 278.0 * 180 / 320
)

// coloresSerie es `SERIES_COLORS`: el color de cada serie si el gráfico no dice otro.
var coloresSerie = []string{"#2563EB", "#DC2626", "#16A34A", "#7C3AED", "#D97706", "#DB2777", "#0D9488"}

func (g sheetChart) color(k int) string {
	if k < len(g.Colors) && g.Colors[k] != "" {
		return g.Colors[k]
	}
	if k < len(coloresSerie) {
		return coloresSerie[k]
	}
	return "#94A3B8"
}

func (g sheetChart) area() (w, h float64) {
	w, h = anchoGraficoPx, altoGraficoPx
	if g.W != nil {
		w = *g.W
	}
	if g.H != nil {
		h = *g.H
	}
	return w, h
}

func (g sheetChart) posicion() (x, y float64) {
	x, y = 96, 48
	if g.X != nil {
		x = *g.X
	}
	if g.Y != nil {
		y = *g.Y
	}
	return x, y
}

// rango es un rectángulo 1-based e inclusivo.
type rango struct{ c1, r1, c2, r2 int }

// leerRango lee «B2» o «$A$1:C10» (el `parseRangeRef` del editor).
func leerRango(s string) (rango, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), "$", "")
	a, b, dos := strings.Cut(s, ":")
	r1, c1, ok := ParseA1(a)
	if !ok {
		return rango{}, false
	}
	r2, c2 := r1, c1
	if dos {
		if r2, c2, ok = ParseA1(b); !ok {
			return rango{}, false
		}
	}
	return rango{c1: min(c1, c2), r1: min(r1, r2), c2: max(c1, c2), r2: max(r1, r2)}, true
}

func (r rango) a1() string {
	if r.c1 == r.c2 && r.r1 == r.r2 {
		return A1Ref(r.r1, r.c1)
	}
	return A1Ref(r.r1, r.c1) + ":" + A1Ref(r.r2, r.c2)
}

// absoluto es el rango con `$` y la hoja delante, como lo escriben Excel y Calc:
// `'Hoja 1'!$A$2:$A$5`.
func (r rango) absoluto(hoja string) string {
	ref := "$" + ColumnToLetters(r.c1) + "$" + strconv.Itoa(r.r1)
	if r.c1 != r.c2 || r.r1 != r.r2 {
		ref += ":$" + ColumnToLetters(r.c2) + "$" + strconv.Itoa(r.r2)
	}
	return hojaCitada(hoja) + "!" + ref
}

// hojaCitada pone comillas simples a un nombre de hoja que las necesita.
func hojaCitada(h string) string {
	simple := h != ""
	for _, c := range h {
		letra := c == '_' || c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c > 127 && c != ' '
		if !letra {
			simple = false
		}
	}
	if simple && (h[0] < '0' || h[0] > '9') {
		return h
	}
	return "'" + strings.ReplaceAll(h, "'", "''") + "'"
}

// nombresCabecera es `uniqueHeaderNames`: un encabezado vacío pasa a «ColumnaN» y un
// repetido se sufija con 2, 3, etc.
func nombresCabecera(crudos []string) []string {
	out := make([]string, 0, len(crudos))
	usados := map[string]bool{}
	for i, r := range crudos {
		n := strings.TrimSpace(r)
		if n == "" {
			n = "Columna" + strconv.Itoa(i+1)
		}
		if usados[strings.ToLower(n)] {
			k := 2
			for usados[strings.ToLower(n+strconv.Itoa(k))] {
				k++
			}
			n += strconv.Itoa(k)
		}
		usados[strings.ToLower(n)] = true
		out = append(out, n)
	}
	return out
}

// seriesDeGrafico es lo que `extractChartData` lee de un rango, pero en REFERENCIAS: el
// rango de los rótulos de categoría, y por serie la celda de su nombre y el rango de
// sus valores. `esTexto` dice si una celda tiene texto (para decidir la cabecera).
type serieRef struct {
	nombre  *rango // nil si la serie no tiene cabecera
	valores rango
}

func seriesDeGrafico(r rango, porFilas bool, esTexto func(fila, col int) bool) (categorias *rango, series []serieRef) {
	// Se razona en el sentido de las series y se traspone al final si van por filas.
	c1, r1, c2, r2 := r.c1, r.r1, r.c2, r.r2
	texto := esTexto
	if porFilas {
		c1, r1, c2, r2 = r.r1, r.c1, r.r2, r.c2
		texto = func(f, c int) bool { return esTexto(c, f) }
	}
	orient := func(x rango) rango {
		if porFilas {
			return rango{c1: x.r1, r1: x.c1, c2: x.r2, r2: x.c2}
		}
		return x
	}
	conRotulos := c2 > c1
	d1 := c1
	if conRotulos {
		d1++
	}
	cab := r2 > r1
	for c := d1; cab && c <= c2; c++ {
		if !texto(r1, c) {
			cab = false
		}
	}
	f1 := r1
	if cab {
		f1++
	}
	if conRotulos && f1 <= r2 {
		cat := orient(rango{c1: c1, r1: f1, c2: c1, r2: r2})
		categorias = &cat
	}
	for c := d1; c <= c2; c++ {
		s := serieRef{valores: orient(rango{c1: c, r1: f1, c2: c, r2: r2})}
		if cab {
			n := orient(rango{c1: c, r1: r1, c2: c, r2: r1})
			s.nombre = &n
		}
		series = append(series, s)
	}
	return categorias, series
}

// pxAEmu y emuAPx pasan de px de la rejilla a EMU (1 px = 9525 EMU) y de vuelta; la
// vuelta se redondea a la centésima, que es lo que distingue el editor.
func pxAEmu(px float64) int64 { return int64(math.Round(px * emuPorPx)) }

func emuAPx(emu int64) float64 { return math.Round(float64(emu)/emuPorPx*100) / 100 }
