// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package analisis son las herramientas de análisis de una hoja de cálculo que no son
// fórmulas pero se calculan: el esquema y los subtotales, la tabla dinámica, el
// análisis de hipótesis y los pasos de una consulta. Cada pieza es el puerto de su
// fichero de la web (`packages/office/src/editor/sheets/*.ts`) y se comprueba contra los
// MISMOS vectores (testdata/*.json), que la web lee también: el mismo caso da el mismo
// resultado en la web y en la app nativa.
//
// Nada de aquí conoce el HTML de una celda ni el lienzo: lee la hoja por funciones que
// le pasa quien llama y devuelve qué hay que cambiar.
package analisis

import (
	"cmp"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// NivelMaximo es `MAX_OUTLINE_LEVEL`: siete niveles, como Excel.
const NivelMaximo = 7

// Grupo es un `SheetGroup`: las unidades (filas o columnas) [Start, End] del detalle,
// su nivel (1, el más externo) y si está contraído.
type Grupo struct {
	Start     int  `json:"start"`
	End       int  `json:"end"`
	Level     int  `json:"level"`
	Collapsed bool `json:"collapsed,omitempty"`
}

// numeroJS es `Number(v)` para lo que puede traer un JSON: el número, el texto con un
// número, un booleano o null. Lo demás es NaN (false).
func numeroJS(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case string:
		x = strings.TrimSpace(x)
		if x == "" {
			return 0, true
		}
		n, err := strconv.ParseFloat(x, 64)
		return n, err == nil
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case nil:
		return 0, true
	}
	return 0, false
}

// entero es `Math.trunc(Number(v))` con `Number.isInteger`: el entero, o false.
func entero(v any) (int, bool) {
	n, ok := numeroJS(v)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return int(math.Trunc(n)), true
}

// SanearGrupo es `sanitizeGroup`: el grupo leído del documento, o false si no vale
// contra `tope` unidades.
func SanearGrupo(crudo any, tope int) (Grupo, bool) {
	m, ok := crudo.(map[string]any)
	if !ok {
		return Grupo{}, false
	}
	// Una clave que falta es `undefined`, y `Number(undefined)` es NaN: no vale. Un null
	// sí vale 0, como en la web.
	campo := func(k string) (int, bool) {
		v, ok := m[k]
		if !ok {
			return 0, false
		}
		return entero(v)
	}
	s, ok1 := campo("start")
	e, ok2 := campo("end")
	l, ok3 := campo("level")
	if !ok1 || !ok2 || !ok3 || s < 0 || e >= tope || s > e || l < 1 || l > NivelMaximo {
		return Grupo{}, false
	}
	c, _ := m["collapsed"].(bool)
	return Grupo{Start: s, End: e, Level: l, Collapsed: c}, true
}

// SanearGrupos es `sanitizeGroups`: los que valen, ordenados.
func SanearGrupos(crudo any, tope int) []Grupo {
	lista, _ := crudo.([]any)
	out := []Grupo{}
	for _, g := range lista {
		if v, ok := SanearGrupo(g, tope); ok {
			out = append(out, v)
		}
	}
	ordenar(out)
	return out
}

// ordenar es `sortGroups`: por inicio, nivel y fin. Estable.
func ordenar(gs []Grupo) {
	slices.SortStableFunc(gs, func(a, b Grupo) int {
		return cmp.Or(cmp.Compare(a.Start, b.Start), cmp.Compare(a.Level, b.Level), cmp.Compare(a.End, b.End))
	})
}

// Ocultas es `hiddenUnits`: las unidades del detalle de todo grupo contraído.
func Ocultas(gs []Grupo) map[int]bool {
	out := map[int]bool{}
	for _, g := range gs {
		if g.Collapsed {
			for u := g.Start; u <= g.End; u++ {
				out[u] = true
			}
		}
	}
	return out
}

// Niveles es `maxOutlineLevel`: el nivel más profundo (0 sin grupos).
func Niveles(gs []Grupo) int {
	m := 0
	for _, g := range gs {
		m = max(m, g.Level)
	}
	return m
}

// HastaNivel es `groupsAtLevel`: contrae los de nivel ≥ n y expande el resto.
func HastaNivel(gs []Grupo, n int) []Grupo {
	out := make([]Grupo, len(gs))
	for i, g := range gs {
		out[i] = Grupo{Start: g.Start, End: g.End, Level: g.Level, Collapsed: g.Level >= n}
	}
	return out
}

// Conmutar es `toggleGroup`: contrae o expande el grupo que coincide.
func Conmutar(gs []Grupo, start, end, level int) []Grupo {
	out := slices.Clone(gs)
	for i, g := range out {
		if g.Start == start && g.End == end && g.Level == level {
			out[i].Collapsed = !g.Collapsed
		}
	}
	return out
}

// Agrupar es `addGroup`: el nivel es uno más que el de los grupos que contienen el
// rango; no duplica ni pasa del tope.
func Agrupar(gs []Grupo, start, end, tope int) []Grupo {
	s, e := max(0, min(start, end)), min(tope-1, max(start, end))
	if s > e {
		return slices.Clone(gs)
	}
	nivel := 1
	for _, g := range gs {
		if g.Start <= s && g.End >= e {
			nivel = max(nivel, g.Level+1)
		}
	}
	if nivel > NivelMaximo || slices.ContainsFunc(gs, func(g Grupo) bool { return g.Start == s && g.End == e && g.Level == nivel }) {
		return slices.Clone(gs)
	}
	out := append(slices.Clone(gs), Grupo{Start: s, End: e, Level: nivel})
	ordenar(out)
	return out
}

// Desagrupar es `removeGroup`: quita los del nivel más profundo que solapan el rango.
func Desagrupar(gs []Grupo, start, end int) []Grupo {
	s, e := min(start, end), max(start, end)
	hondo := 0
	for _, g := range gs {
		if g.Start <= e && g.End >= s {
			hondo = max(hondo, g.Level)
		}
	}
	if hondo == 0 {
		return slices.Clone(gs)
	}
	out := []Grupo{}
	for _, g := range gs {
		if g.Level != hondo || g.Start > e || g.End < s {
			out = append(out, g)
		}
	}
	return out
}

// ---- Subtotales ----

// Agregacion es la función del asistente de Subtotales (`TableTotalFn` sin «none»).
type Agregacion string

// NumeroSubtotal es `SUBTOTAL_FN_NUM`: el primer argumento de SUBTOTAL de cada una.
var NumeroSubtotal = map[Agregacion]int{"average": 1, "count": 2, "counta": 3, "max": 4, "min": 5, "sum": 9}

// salidaSubtotal es `SubtotalOut`: una fila del cuerpo nuevo.
type salidaSubtotal struct {
	tipo     string // "data", "subtotal" o "grand"
	src      int
	clave    string
	from, to int
}

// planSubtotales es `planSubtotals`: dónde van los cortes y el esquema, en índices
// relativos al cuerpo de salida.
func planSubtotales(claves []string) ([]salidaSubtotal, []Grupo) {
	var out []salidaSubtotal
	var grupos []Grupo
	if len(claves) == 0 {
		return out, grupos
	}
	clave, inicio := claves[0], 0
	cerrar := func(k string) {
		from, to := inicio, len(out)-1
		out = append(out, salidaSubtotal{tipo: "subtotal", clave: k, from: from, to: to})
		grupos = append(grupos, Grupo{Start: from, End: to, Level: 2})
		inicio = len(out)
	}
	for i, k := range claves {
		if i > 0 && k != clave {
			cerrar(clave)
			clave = k
		}
		out = append(out, salidaSubtotal{tipo: "data", src: i})
	}
	cerrar(clave)
	out = append(out, salidaSubtotal{tipo: "grand"})
	grupos = append(grupos, Grupo{Start: 0, End: len(out) - 2, Level: 1})
	return out, grupos
}

// PeticionSubtotal es `SubtotalRequest`: coordenadas absolutas 0-based; F1 es la
// cabecera.
type PeticionSubtotal struct {
	C1, F1, C2, F2 int
	ColGrupo       int
	Fn             Agregacion
	// Cols son las columnas a las que se añade el subtotal; nil, las que tengan números.
	Cols []int
	// Quitar es «Quitar todos».
	Quitar       bool
	Filas        int
	UltimaUsada  int
	RotuloTotal  string
	RotuloGrande string
	RotuloDe     func(clave string) string
}

// Lector es `SubtotalReader`: cómo se lee la hoja.
type Lector struct {
	Crudo     func(c, f int) string
	Clave     func(c, f int) string
	EsNumero  func(c, f int) bool
	TieneAlgo func(c, f int) bool
}

// Escritura es `SubtotalWrite`: una celda que escribe el asistente, en negrita.
type Escritura struct {
	C     int    `json:"c"`
	F     int    `json:"r"`
	Texto string `json:"text"`
}

// ResultadoSubtotal es `SubtotalResult` sin error.
type ResultadoSubtotal struct {
	// Desde[f] es de qué fila viene la fila f de la hoja nueva (−1: vacía o nueva).
	Desde      []int
	Escrituras []Escritura
	Grupos     []Grupo
	UltimaFila int
	Bloques    int
}

// Los errores de AplicarSubtotales, con el nombre de la web.
const (
	SinDatos = "no-data"
	SinHueco = "no-room"
)

var reSubtotal = regexp.MustCompile(`(?i)^=.*\bSUBTOTAL\s*\(`)

// letras es el nombre de columna A..Z, AA….
func letras(c int) string {
	s := ""
	for n := c + 1; n > 0; n = (n - 1) / 26 {
		s = string(rune('A'+(n-1)%26)) + s
	}
	return s
}

// AplicarSubtotales es `applySubtotals`: el asistente entero sin tocar la hoja. Dice de
// dónde sale cada fila, qué celdas se escriben y cuál es el esquema; o el error.
//
//nolint:gocognit,gocyclo // es applySubtotals paso a paso, para compararlas línea a línea
func AplicarSubtotales(p PeticionSubtotal, l Lector) (ResultadoSubtotal, string) {
	f2 := min(p.F2, p.Filas-1)
	filaSubtotal := func(f int) bool {
		for c := p.C1; c <= p.C2; c++ {
			if reSubtotal.MatchString(l.Crudo(c, f)) {
				return true
			}
		}
		return false
	}
	vacia := func(f int) bool {
		for c := p.C1; c <= p.C2; c++ {
			if l.Crudo(c, f) != "" {
				return false
			}
		}
		return true
	}
	var cuerpo []int
	for f := p.F1 + 1; f <= f2; f++ {
		cuerpo = append(cuerpo, f)
	}
	for len(cuerpo) > 0 && vacia(cuerpo[len(cuerpo)-1]) {
		cuerpo = cuerpo[:len(cuerpo)-1]
	}
	var datos []int
	for _, f := range cuerpo {
		if !filaSubtotal(f) {
			datos = append(datos, f)
		}
	}

	var sale []int // filas del cuerpo nuevo: la de origen o −1
	var res ResultadoSubtotal
	if p.Quitar {
		sale = append(sale, datos...)
	} else {
		if len(datos) == 0 {
			return res, SinDatos
		}
		claves := make([]string, len(datos))
		for i, f := range datos {
			claves[i] = l.Clave(p.ColGrupo, f)
		}
		plan, grupos := planSubtotales(claves)
		var cols []int
		for c := p.C1; c <= p.C2; c++ {
			if c == p.ColGrupo {
				continue
			}
			elegida := slices.Contains(p.Cols, c)
			if p.Cols == nil {
				elegida = slices.ContainsFunc(datos, func(f int) bool {
					if p.Fn == "counta" {
						return l.TieneAlgo(c, f)
					}
					return l.EsNumero(c, f)
				})
			}
			if elegida {
				cols = append(cols, c)
			}
		}
		n := strconv.Itoa(NumeroSubtotal[p.Fn])
		rango := func(c, from, to int) string {
			return letras(c) + strconv.Itoa(p.F1+from+2) + ":" + letras(c) + strconv.Itoa(p.F1+to+2)
		}
		for i, o := range plan {
			f := p.F1 + 1 + i
			if o.tipo == "data" {
				sale = append(sale, datos[o.src])
				continue
			}
			sale = append(sale, -1)
			rotulo := p.RotuloGrande
			from, to := 0, len(plan)-2
			if o.tipo == "subtotal" {
				from, to = o.from, o.to
				rotulo = p.RotuloTotal
				if o.clave != "" {
					rotulo = p.RotuloDe(o.clave)
				}
			}
			res.Escrituras = append(res.Escrituras, Escritura{C: p.ColGrupo, F: f, Texto: rotulo})
			for _, c := range cols {
				res.Escrituras = append(res.Escrituras, Escritura{C: c, F: f, Texto: "=SUBTOTAL(" + n + ";" + rango(c, from, to) + ")"})
			}
		}
		for _, g := range grupos {
			res.Grupos = append(res.Grupos, Grupo{Start: p.F1 + 1 + g.Start, End: min(p.Filas-1, p.F1+1+g.End), Level: g.Level})
			if g.Level == 2 {
				res.Bloques++
			}
		}
	}

	ultimo := p.F1
	if len(cuerpo) > 0 {
		ultimo = cuerpo[len(cuerpo)-1]
	}
	desp := len(sale) - len(cuerpo)
	if p.F1+len(sale) > p.Filas-1 || p.UltimaUsada+desp > p.Filas-1 {
		return ResultadoSubtotal{}, SinHueco
	}
	res.Desde = make([]int, p.Filas)
	for f := range res.Desde {
		res.Desde[f] = -1
	}
	for f := 0; f <= p.F1; f++ {
		res.Desde[f] = f
	}
	for i, src := range sale {
		res.Desde[p.F1+1+i] = src
	}
	for f := ultimo + 1; f < p.Filas; f++ {
		if g := f + desp; g >= 0 && g < p.Filas {
			res.Desde[g] = f
		}
	}
	res.UltimaFila = min(p.Filas-1, p.F1+len(sale))
	return res, ""
}
