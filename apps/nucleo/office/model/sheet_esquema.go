// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"slices"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
)

// El ESQUEMA de una hoja de Calcus (`SheetData.rowGroups` y `colGroups`, `SheetGroup` de
// `types.ts`): grupos de filas o columnas, anidados por nivel (1 = el más externo), con el
// resumen justo DESPUÉS del detalle (end+1) y un «colapsado» por grupo. Excel y LibreOffice
// lo guardan de otra manera: Excel, como un `outlineLevel` por fila o columna y los ocultos
// con `hidden`; ODF, como `table:table-row-group` y `table:table-column-group` anidados con
// `table:display`. Aquí se traducen en los dos sentidos.

const (
	maxNivelEsquema  = 7
	maxGruposHoja    = 2000
	maxFilasEsquema  = 50000 // filas que se escriben solo por pertenecer a un grupo
	maxUnidadesGrupo = 1 << 20
)

// esquemaHoja es el esquema de una SheetData; va embebido en sheetData. Con él van las filas
// y columnas ocultas A MANO (`hiddenRows`, `hiddenCols`: tramos {start, end} 0-based), que no
// son esquema pero se escriben por el mismo sitio: Excel las guarda como `hidden="1"` sin
// `outlineLevel`, y ODF como `table:visibility="collapse"` fuera de un grupo.
type esquemaHoja struct {
	RowGroups  []sheetGroup     `json:"rowGroups,omitempty"`  //nolint:tagliatelle // el nombre lo fija el editor
	ColGroups  []sheetGroup     `json:"colGroups,omitempty"`  //nolint:tagliatelle // el nombre lo fija el editor
	HiddenRows []analisis.Tramo `json:"hiddenRows,omitempty"` //nolint:tagliatelle // el nombre lo fija el editor
	HiddenCols []analisis.Tramo `json:"hiddenCols,omitempty"` //nolint:tagliatelle // el nombre lo fija el editor
}

// sanearEsquema deja el esquema y las ocultas de una hoja como los dejaría el editor.
func (e *esquemaHoja) sanearEsquema() {
	e.RowGroups, e.ColGroups = sanearGrupos(e.RowGroups, maxFilasHoja), sanearGrupos(e.ColGroups, maxColumnasHoja)
	e.HiddenRows = analisis.RecortarTramos(e.HiddenRows, maxFilasHoja)
	e.HiddenCols = analisis.RecortarTramos(e.HiddenCols, maxColumnasHoja)
	if len(e.HiddenRows) == 0 {
		e.HiddenRows = nil
	}
	if len(e.HiddenCols) == 0 {
		e.HiddenCols = nil
	}
}

// ocultaUnidad dice si la unidad u no se ve por el esquema (un grupo colapsado) o porque se
// ocultó a mano.
func ocultaUnidad(gs []sheetGroup, ts []analisis.Tramo, u int) bool {
	_, oculta := nivelDe(gs, u)
	return oculta || analisis.TramoOculto(ts, u)
}

// ocultasSueltas son las unidades ocultas de un fichero importado que NO explica un grupo
// colapsado del esquema, en tramos: las que se ocultaron a mano. `ocultas` son todas las que
// el fichero da por ocultas (0-based).
func ocultasSueltas(ocultas []analisis.Tramo, gs []sheetGroup, limite int) []analisis.Tramo {
	ts := analisis.RecortarTramos(ocultas, limite)
	for _, g := range gs {
		if !g.Collapsed {
			continue
		}
		var out []analisis.Tramo
		for _, t := range ts {
			if t.End < g.Start || t.Start > g.End {
				out = append(out, t)
				continue
			}
			if t.Start < g.Start {
				out = append(out, analisis.Tramo{Start: t.Start, End: g.Start - 1})
			}
			if t.End > g.End {
				out = append(out, analisis.Tramo{Start: g.End + 1, End: t.End})
			}
		}
		ts = out
	}
	if len(ts) == 0 {
		return nil
	}
	return ts
}

// tramosDeUnidades junta unas unidades sueltas (0-based) en tramos.
func tramosDeUnidades(us map[int]bool) []analisis.Tramo {
	ts := make([]analisis.Tramo, 0, len(us))
	for u := range us {
		ts = append(ts, analisis.Tramo{Start: u, End: u})
	}
	return analisis.RecortarTramos(ts, maxFilasHoja)
}

// sheetGroup es el `SheetGroup` del editor; start y end son 0-based.
type sheetGroup struct {
	Start     int  `json:"start"`
	End       int  `json:"end"`
	Level     int  `json:"level"`
	Collapsed bool `json:"collapsed,omitempty"`
}

// sanearGrupos deja los grupos válidos contra `limite` unidades, ordenados como `sortGroups`
// (inicio, nivel, fin) y acotados en número.
func sanearGrupos(gs []sheetGroup, limite int) []sheetGroup {
	var out []sheetGroup
	for _, g := range gs {
		if g.Start < 0 || g.End >= limite || g.Start > g.End || g.Level < 1 || g.Level > maxNivelEsquema {
			continue
		}
		if len(out) >= maxGruposHoja {
			break
		}
		out = append(out, g)
	}
	slices.SortFunc(out, func(a, b sheetGroup) int {
		if a.Start != b.Start {
			return a.Start - b.Start
		}
		if a.Level != b.Level {
			return a.Level - b.Level
		}
		return a.End - b.End
	})
	return out
}

// nivelDe es el nivel de esquema de una unidad (el del grupo más interno que la cubre) y si
// algún grupo colapsado la oculta.
func nivelDe(gs []sheetGroup, u int) (nivel int, oculta bool) {
	for _, g := range gs {
		if u >= g.Start && u <= g.End {
			nivel = max(nivel, g.Level)
			oculta = oculta || g.Collapsed
		}
	}
	return nivel, oculta
}

// nivelMaximo es el mayor nivel de los grupos.
func nivelMaximo(gs []sheetGroup) int {
	n := 0
	for _, g := range gs {
		n = max(n, g.Level)
	}
	return n
}

// unidadesDeResumen son las unidades de resumen (end+1) de los grupos colapsados: Excel las
// marca con `collapsed="1"`.
func resumenColapsado(gs []sheetGroup, u int) bool {
	for _, g := range gs {
		if g.Collapsed && g.End+1 == u {
			return true
		}
	}
	return false
}

// filasDeEsquema son las filas (1-based) que hay que escribir aunque estén vacías: las de
// los grupos (hasta un tope) y los resúmenes de los colapsados.
func filasDeEsquema(sd sheetData) []int {
	set := map[int]bool{}
	for _, g := range sanearGrupos(sd.RowGroups, maxFilasHoja) {
		for u := g.Start; u <= g.End && len(set) < maxFilasEsquema; u++ {
			set[u+1] = true
		}
		if g.Collapsed && g.End+2 <= maxFilasHoja {
			set[g.End+2] = true
		}
	}
	// Las ocultas a mano también: una fila vacía que no se escribe no puede ir oculta.
	// ponytail: con el tope, un tramo hasta el final de la hoja sale oculto solo en sus
	// primeras filas; las de después están vacías y no se echan de menos.
	for _, t := range sd.HiddenRows {
		for u := t.Start; u <= t.End && len(set) < maxFilasEsquema; u++ {
			set[u+1] = true
		}
	}
	out := make([]int, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	slices.Sort(out)
	return out
}

// atributosEsquemaFila son los atributos de `<row>` del esquema: nivel, oculta y colapsada.
func atributosEsquemaFila(sd sheetData, fila int) (nivel int, oculta, colapsada bool) {
	nivel, _ = nivelDe(sd.RowGroups, fila-1)
	return nivel, ocultaUnidad(sd.RowGroups, sd.HiddenRows, fila-1), resumenColapsado(sd.RowGroups, fila-1)
}

// colsXlsx es el `<cols>` de la hoja: una entrada por tramo de columnas contiguas con el
// mismo ancho y el mismo esquema. "" si no hay ninguna.
func colsXlsx(sd sheetData) string {
	type col struct {
		ancho             float64 // 0 = de serie
		nivel             int
		oculta, colapsada bool
	}
	cols := map[int]col{}
	for _, i := range medidasOrdenadas(sd.ColWidths, maxColumnasHoja) {
		w, _ := medidaValida(sd.ColWidths[strconv.Itoa(i)], maxAnchoPx)
		cols[i] = col{ancho: w}
	}
	for _, g := range sanearGrupos(sd.ColGroups, maxColumnasHoja) {
		for u := g.Start; u <= g.End+1 && u < maxColumnasHoja; u++ {
			c := cols[u]
			c.nivel, c.oculta = nivelDe(sd.ColGroups, u)
			c.colapsada = resumenColapsado(sd.ColGroups, u)
			cols[u] = c
		}
	}
	for _, t := range analisis.RecortarTramos(sd.HiddenCols, maxColumnasHoja) {
		for u := t.Start; u <= t.End; u++ {
			c := cols[u]
			c.oculta = true
			cols[u] = c
		}
	}
	idx := make([]int, 0, len(cols))
	for i := range cols {
		idx = append(idx, i)
	}
	slices.Sort(idx)
	var b strings.Builder
	for i := 0; i < len(idx); {
		c := cols[idx[i]]
		j := i
		for j+1 < len(idx) && idx[j+1] == idx[j]+1 && cols[idx[j+1]] == c {
			j++
		}
		w := c.ancho
		attrs := ""
		if w == 0 {
			w = anchoSeriePx
		} else {
			attrs += ` customWidth="1"`
		}
		if c.nivel > 0 {
			attrs += ` outlineLevel="` + strconv.Itoa(c.nivel) + `"`
		}
		if c.oculta {
			attrs += ` hidden="1"`
		}
		if c.colapsada {
			attrs += ` collapsed="1"`
		}
		b.WriteString(`<col min="` + strconv.Itoa(idx[i]+1) + `" max="` + strconv.Itoa(idx[j]+1) + `" width="` +
			strconv.FormatFloat(anchoExcel(w), 'f', -1, 64) + `"` + attrs + `/>`)
		i = j + 1
	}
	if b.Len() == 0 {
		return ""
	}
	return `<cols>` + b.String() + `</cols>`
}

// gruposDeNiveles reconstruye los grupos de un eje a partir del nivel de esquema de cada
// unidad (0-based): para cada nivel L, cada tramo contiguo de unidades con nivel >= L es un
// grupo de nivel L, colapsado si todas sus unidades están ocultas.
func gruposDeNiveles(nivel map[int]int, ocultas map[int]bool, limite int) []sheetGroup {
	var unidades []int
	topNivel := 0
	for u, n := range nivel {
		if n > 0 && u >= 0 && u < limite {
			unidades = append(unidades, u)
			topNivel = max(topNivel, min(n, maxNivelEsquema))
		}
	}
	slices.Sort(unidades)
	var out []sheetGroup
	for l := 1; l <= topNivel; l++ {
		for i := 0; i < len(unidades); {
			if min(nivel[unidades[i]], maxNivelEsquema) < l {
				i++
				continue
			}
			j := i
			todasOcultas := ocultas[unidades[i]]
			for j+1 < len(unidades) && unidades[j+1] == unidades[j]+1 && min(nivel[unidades[j+1]], maxNivelEsquema) >= l {
				j++
				todasOcultas = todasOcultas && ocultas[unidades[j]]
			}
			if len(out) < maxGruposHoja {
				out = append(out, sheetGroup{Start: unidades[i], End: unidades[j], Level: l, Collapsed: todasOcultas})
			}
			i = j + 1
		}
	}
	return sanearGrupos(out, limite)
}

// filasExtra son las filas (1-based, ordenadas) que se escriben aunque no tengan celdas: las
// de alto propio y las del esquema.
func filasExtra(sd sheetData) []int {
	out := append(filasConAlto(sd), filasDeEsquema(sd)...)
	slices.Sort(out)
	return slices.Compact(out)
}

// atributosFilaXlsx son los atributos de `<row>` que no son el alto: el nivel de esquema, si
// la oculta un grupo colapsado o el autofiltro, y si es el resumen de un grupo colapsado.
func atributosFilaXlsx(sd sheetData, fila int) string {
	nivel, oculta, colapsada := atributosEsquemaFila(sd, fila)
	attrs := ""
	if nivel > 0 {
		attrs += ` outlineLevel="` + strconv.Itoa(nivel) + `"`
	}
	if oculta {
		attrs += ` hidden="1"`
	} else {
		attrs += atributoOculta(sd, fila)
	}
	if colapsada {
		attrs += ` collapsed="1"`
	}
	return attrs
}

// niveles son los `outlineLevelRow` y `outlineLevelCol` de `sheetFormatPr`.
func niveles(sd sheetData) string {
	s := ""
	if n := nivelMaximo(sd.RowGroups); n > 0 {
		s += ` outlineLevelRow="` + strconv.Itoa(n) + `"`
	}
	if n := nivelMaximo(sd.ColGroups); n > 0 {
		s += ` outlineLevelCol="` + strconv.Itoa(n) + `"`
	}
	return s
}
