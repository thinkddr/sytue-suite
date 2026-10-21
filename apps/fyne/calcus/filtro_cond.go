// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Filtros por CONDICIÓN del autofiltro (`sheets/filtro.ts` de la web): mayor/menor/entre,
// contiene/empieza/termina, vacías, N superiores/inferiores, por encima/debajo de la media
// y por color de relleno o de texto. Una condición vive en `SheetAutoFilter.cond[col]`,
// que es modelo COMPARTIDO: lo que aquí no se leyera se perdería al guardar para todos.

// CondFiltro es una `FilterCondition`. Los operandos A y B van como los tecleó la persona.
// El orden de los campos es el de la web, para que una condición que no se toca salga con
// los mismos bytes con los que entró.
type CondFiltro struct {
	Op string `json:"op"`
	A  string `json:"a,omitempty"`
	B  string `json:"b,omitempty"`
	// N es el de «N superiores/inferiores» (o su porcentaje con Percent).
	N       int  `json:"n,omitempty"`
	Percent bool `json:"percent,omitempty"`
	// Color es el #rrggbb de `fill` (relleno manual) y `fontColor`, en minúsculas.
	Color string `json:"color,omitempty"`
}

// OpsFiltro son FILTER_OPS, en el orden en que los enseña el menú.
var OpsFiltro = []string{
	"gt", "ge", "lt", "le", "eq", "ne", "between", "notBetween",
	"contains", "notContains", "begins", "ends", "blank", "notBlank",
	"top", "bottom", "aboveAvg", "belowAvg", "fill", "fontColor",
}

// OperandosFiltro es `operandCount`: cuántos operandos de texto pide cada operador.
func OperandosFiltro(op string) int {
	switch op {
	case "between", "notBetween":
		return 2
	case "blank", "notBlank", "top", "bottom", "aboveAvg", "belowAvg", "fill", "fontColor":
		return 0
	}
	return 1
}

var reHexFiltro = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// sanearCondFiltro es `sanitizeFilterCond`: la condición saneada desde datos no confiables
// (documento, comando o vista); false si no vale.
func sanearCondFiltro(crudo any) (CondFiltro, bool) {
	r, ok := crudo.(map[string]any)
	if !ok {
		return CondFiltro{}, false
	}
	op, _ := r["op"].(string)
	if !slices.Contains(OpsFiltro, op) {
		return CondFiltro{}, false
	}
	out := CondFiltro{Op: op}
	// El texto vale tal cual se tecleó, con sus espacios: 255 unidades UTF-16 como mucho
	// (el `length` de JavaScript) y algo más que blancos.
	texto := func(x any) (string, bool) {
		s, ok := x.(string)
		return s, ok && len(utf16.Encode([]rune(s))) <= 255 && strings.TrimSpace(s) != ""
	}
	k := OperandosFiltro(op)
	if k >= 1 {
		if out.A, ok = texto(r["a"]); !ok {
			return CondFiltro{}, false
		}
	}
	if k == 2 {
		if out.B, ok = texto(r["b"]); !ok {
			return CondFiltro{}, false
		}
	}
	if op == "top" || op == "bottom" {
		n := enteroJS(r["n"])
		if n < 1 || n > 100000 {
			return CondFiltro{}, false
		}
		out.N = n
		out.Percent = r["percent"] == true && n <= 100
	}
	if op == "fill" || op == "fontColor" {
		color, _ := r["color"].(string)
		if !reHexFiltro.MatchString(color) {
			return CondFiltro{}, false
		}
		out.Color = strings.ToLower(color)
	}
	return out, true
}

// sanearCondsFiltro es `sanitizeFilterConds`: el `cond` de un autofiltro, columna a columna
// dentro de [c1, c2]; nil si queda vacío.
func sanearCondsFiltro(crudo map[string]any, c1, c2 int) map[string]CondFiltro {
	var out map[string]CondFiltro
	for clave, v := range crudo {
		n, err := strconv.ParseFloat(strings.TrimSpace(clave), 64)
		if err != nil || n != math.Trunc(n) || int(n) < c1 || int(n) > c2 {
			continue
		}
		if cond, ok := sanearCondFiltro(v); ok {
			if out == nil {
				out = map[string]CondFiltro{}
			}
			out[strconv.Itoa(int(n))] = cond
		}
	}
	return out
}

// celdaFiltro es una `FilterCell`: el valor calculado de una celda del cuerpo y su estilo
// manual.
type celdaFiltro struct {
	v      formula.Value
	estilo Estilo
}

// pasaOrden aplica gt/ge/lt/le al resultado de una comparación (−1, 0 o 1).
func pasaOrden(op string, o int) bool {
	switch op {
	case "gt":
		return o > 0
	case "ge":
		return o >= 0
	case "lt":
		return o < 0
	}
	return o <= 0
}

// evaluarFiltro es `evalFilterColumn`: ¿pasa cada celda la condición? (true = se ve). Los
// N superiores y la media se calculan sobre TODA la columna del cuerpo, no sobre lo que ya
// dejan pasar otras columnas (como el formato condicional).
//
//nolint:gocognit,gocyclo // una rama por operador, como el `switch` de filtro.ts
func evaluarFiltro(cond CondFiltro, celdas []celdaFiltro) []bool {
	var nums []float64
	for _, c := range celdas {
		if n, ok := c.v.(float64); ok {
			nums = append(nums, n)
		}
	}
	aNum, hayA := numeroEstricto(cond.A)
	bNum, hayB := numeroEstricto(cond.B)
	aLow := strings.ToLower(strings.TrimSpace(cond.A))
	var umbral float64
	hayUmbral := false
	if (cond.Op == "top" || cond.Op == "bottom") && len(nums) > 0 {
		orden := slices.Clone(nums)
		slices.Sort(orden)
		if cond.Op == "top" {
			slices.Reverse(orden)
		}
		cuantos := min(max(cond.N, 1), len(orden))
		if cond.Percent {
			cuantos = max(1, int(math.Ceil(float64(max(cond.N, 1))/100*float64(len(orden)))))
		}
		umbral, hayUmbral = orden[min(cuantos, len(orden))-1], true // incluye empates (criterio Excel)
	}
	var media float64
	if len(nums) > 0 {
		var suma float64
		for _, n := range nums {
			suma += n
		}
		media = suma / float64(len(nums))
	}

	out := make([]bool, len(celdas))
	for i, c := range celdas {
		n, esNum := c.v.(float64)
		texto := formula.Display(c.v)
		low := strings.ToLower(strings.TrimSpace(texto))
		enRango := func(a, b float64) bool { return esNum && n >= min(a, b) && n <= max(a, b) }
		switch cond.Op {
		case "gt", "ge", "lt", "le":
			switch {
			case esNum && hayA:
				out[i] = pasaOrden(cond.Op, cmpFloat(n, aNum))
			case low == "" || esNum != hayA: // texto contra número: no se ordenan
			default:
				out[i] = pasaOrden(cond.Op, ordenEs.CompareString(low, aLow))
			}
		case "eq":
			out[i] = low == aLow
			if esNum && hayA {
				out[i] = n == aNum
			}
		case "ne":
			out[i] = low != aLow
			if esNum && hayA {
				out[i] = n != aNum
			}
		case "between":
			out[i] = hayA && hayB && enRango(aNum, bNum)
		case "notBetween":
			// Una celda sin número no está «fuera» de nada: se oculta, no se cuela.
			out[i] = hayA && hayB && esNum && !enRango(aNum, bNum)
		case "contains":
			out[i] = strings.Contains(low, aLow)
		case "notContains":
			out[i] = !strings.Contains(low, aLow)
		case "begins":
			out[i] = strings.HasPrefix(low, aLow)
		case "ends":
			out[i] = strings.HasSuffix(low, aLow)
		case "blank":
			out[i] = texto == ""
		case "notBlank":
			out[i] = texto != ""
		case "top":
			out[i] = esNum && hayUmbral && n >= umbral
		case "bottom":
			out[i] = esNum && hayUmbral && n <= umbral
		case "aboveAvg":
			out[i] = esNum && len(nums) > 0 && n > media
		case "belowAvg":
			out[i] = esNum && len(nums) > 0 && n < media
		case "fill":
			out[i] = cond.Color != "" && strings.EqualFold(c.estilo.Relleno, cond.Color)
		case "fontColor":
			out[i] = cond.Color != "" && strings.EqualFold(c.estilo.Tinta, cond.Color)
		}
	}
	return out
}

func cmpFloat(a, b float64) int {
	switch {
	case a > b:
		return 1
	case a < b:
		return -1
	}
	return 0
}

// ---- edición de criterios (`setExcluded`, `setCond`) ----

// conExcluidos es `setExcluded`: el mismo autofiltro con los valores EXCLUIDOS de `col`
// sustituidos. Excluir algo quita la condición de esa columna: los valores y la condición
// son dos maneras de filtrar la misma columna. El autofiltro es inmutable: devuelve otro.
func (af *Autofiltro) conExcluidos(col int, excluidos []string) *Autofiltro {
	clave := strconv.Itoa(col)
	nuevo := &Autofiltro{Row: af.Row, C1: af.C1, C2: af.C2, Excl: maps.Clone(af.Excl), Cond: maps.Clone(af.Cond)}
	delete(nuevo.Excl, clave)
	if len(excluidos) > 0 {
		if nuevo.Excl == nil {
			nuevo.Excl = map[string][]string{}
		}
		nuevo.Excl[clave] = excluidos
		delete(nuevo.Cond, clave)
	}
	return nuevo.limpio()
}

// conCondicion es `setCond`: pone (o con nil quita) la condición de `col`; ponerla quita
// los valores excluidos de esa columna.
func (af *Autofiltro) conCondicion(col int, cond *CondFiltro) *Autofiltro {
	clave := strconv.Itoa(col)
	nuevo := &Autofiltro{Row: af.Row, C1: af.C1, C2: af.C2, Excl: maps.Clone(af.Excl), Cond: maps.Clone(af.Cond)}
	delete(nuevo.Cond, clave)
	if cond != nil {
		if nuevo.Cond == nil {
			nuevo.Cond = map[string]CondFiltro{}
		}
		nuevo.Cond[clave] = *cond
		delete(nuevo.Excl, clave)
	}
	return nuevo.limpio()
}

// limpio deja a nil los mapas vacíos: el modelo guarda JSON mínimo (`crit`).
func (af *Autofiltro) limpio() *Autofiltro {
	if len(af.Excl) == 0 {
		af.Excl = nil
	}
	if len(af.Cond) == 0 {
		af.Cond = nil
	}
	return af
}

// filtrada dice si la columna tiene algún criterio: lo que enciende su desplegable.
func (af *Autofiltro) filtrada(col int) bool {
	clave := strconv.Itoa(col)
	_, hayCond := af.Cond[clave]
	return len(af.Excl[clave]) > 0 || hayCond
}

// PonerCondicionFiltro es `filtercond`: la condición de la columna (nil la quita). false si
// no hay autofiltro o la columna no es suya.
func (l *Libro) PonerCondicionFiltro(col int, cond *CondFiltro) bool {
	af := l.h().filtro
	if af == nil || col < af.C1 || col > af.C2 {
		return false
	}
	l.anotar()
	l.h().filtro = af.conCondicion(col, cond)
	l.version++
	return true
}

// CondicionFiltro es la condición de esa columna del autofiltro de la hoja activa, o nil.
func (l *Libro) CondicionFiltro(col int) *CondFiltro {
	af := l.h().filtro
	if af == nil {
		return nil
	}
	if c, ok := af.Cond[strconv.Itoa(col)]; ok {
		return &c
	}
	return nil
}
