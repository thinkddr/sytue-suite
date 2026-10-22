// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// # La edición de fórmulas: `sheets/formula-edit.ts`
//
// Lo que necesita quien ESCRIBE una fórmula, no quien la evalúa: qué referencias hay y
// dónde (para tintarlas en la barra y marcarlas en la rejilla), qué función se está
// tecleando (autocompletar), en qué argumento está el cursor (la pista), si el cursor
// espera una referencia (señalar con el ratón o las flechas) y el ciclo de F4.
//
// El AST del parser no lleva posiciones, y aquí son imprescindibles; por eso, como en
// la web, se replica el tokenizador con posiciones. Las reglas léxicas son las MISMAS
// del parser, y el reconocimiento de referencias es el suyo (`parseRefEnd`): nunca una
// expresión regular de referencias escrita a mano.
//
// Las posiciones son de BYTES del texto UTF-8, no de unidades UTF-16 como en la web:
// todos los tokens que importan son ASCII, así que las posiciones cuentan lo mismo
// hasta el primer carácter que no lo es, y a partir de ahí quien las usa corta el
// mismo texto de Go con ellas.

// TipoFx es la clase de un token de la fórmula en edición.
type TipoFx uint8

// Las clases de token de `tokenizeFx`.
const (
	FxNum TipoFx = iota
	FxStr
	FxQSheet
	FxWord
	FxOp
	FxLParen
	FxRParen
	FxSep
	FxColon
	FxBang
	FxPercent
)

// TokFx es un token con su posición: [Inicio, Fin) en bytes, con el «=» incluido.
type TokFx struct {
	T           TipoFx
	V           string
	Inicio, Fin int
}

// TokenizeFx es `tokenizeFx`: nunca falla. Un carácter que no sabe leer se salta y una
// cadena sin cerrar llega hasta el final, que es lo que pasa a mitad de teclear.
func TokenizeFx(src string) []TokFx {
	var toks []TokFx
	n := len(src)
	poner := func(t TipoFx, a, b int) { toks = append(toks, TokFx{T: t, V: src[a:b], Inicio: a, Fin: b}) }
	for i := 0; i < n; {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case esDigito(c) || (c == '.' && i+1 < n && esDigito(src[i+1])):
			j := finNumero(src, i)
			poner(FxNum, i, j)
			i = j
		case c == '"' || c == '\'':
			j := finComillas(src, i, c)
			if c == '"' {
				poner(FxStr, i, j)
			} else {
				poner(FxQSheet, i, j)
			}
			i = j
		default:
			i = tokenSimple(src, i, poner)
		}
	}
	return toks
}

// finNumero es dónde acaba el número que empieza en i: cifras y puntos, y un exponente.
func finNumero(src string, i int) int {
	n := len(src)
	j := i + 1
	for j < n && (esDigito(src[j]) || src[j] == '.') {
		j++
	}
	if j < n && (src[j] == 'e' || src[j] == 'E') {
		j++
		if j < n && (src[j] == '+' || src[j] == '-') {
			j++
		}
		for j < n && esDigito(src[j]) {
			j++
		}
	}
	return j
}

// finComillas es dónde acaba una cadena que abre en i con la comilla q: tras la que la
// cierra, contando la doble como una comilla escrita; o al final si no cierra.
func finComillas(src string, i int, q byte) int {
	j := i + 1
	for j < len(src) {
		if src[j] == q {
			if j+1 < len(src) && src[j+1] == q {
				j += 2
				continue
			}
			return j + 1
		}
		j++
	}
	return j
}

// signosFx son los tokens de un carácter que no son operadores.
var signosFx = map[byte]TipoFx{'(': FxLParen, ')': FxRParen, ',': FxSep, ';': FxSep, ':': FxColon, '!': FxBang, '%': FxPercent}

// tokenSimple lee un operador, un signo o una palabra en i y devuelve dónde sigue.
func tokenSimple(src string, i int, poner func(TipoFx, int, int)) int {
	c := src[i]
	if i+2 <= len(src) {
		if dos := src[i : i+2]; dos == "<=" || dos == ">=" || dos == "<>" {
			poner(FxOp, i, i+2)
			return i + 2
		}
	}
	switch {
	case strings.IndexByte("+-*/^&=<>", c) >= 0:
		poner(FxOp, i, i+1)
		return i + 1
	case esInicioPalabra(c):
		j := i + 1
		for j < len(src) && esRestoPalabra(src[j]) {
			j++
		}
		poner(FxWord, i, j)
		return j
	}
	if t, ok := signosFx[c]; ok {
		poner(t, i, i+1)
	}
	return i + 1 // un carácter desconocido se ignora
}

// EjeEntero es la última fila o columna posible (2^20 − 1): el tope de un eje libre en
// «A:A» o «1:1». Quien pinta lo acota a la rejilla.
const EjeEntero = 1_048_575

// Rect es un rectángulo de celdas de base cero, ya ordenado.
type Rect struct{ C1, R1, C2, R2 int }

var reRango = regexp.MustCompile(`^\$?([A-Za-z]{1,3})\$?(\d{1,7})(?::\$?([A-Za-z]{1,3})\$?(\d{1,7}))?$`)

// ParseRangeRef es `parseRangeRef` de `sheets/format.ts`: «B2» o «A1:C10» (con o sin
// «$»), normalizado. false si no lo es.
func ParseRangeRef(texto string) (Rect, bool) {
	m := reRango.FindStringSubmatch(strings.TrimSpace(texto))
	if m == nil {
		return Rect{}, false
	}
	c1, r1 := ColToNum(m[1]), filaDe(m[2])
	c2, r2 := c1, r1
	if m[3] != "" {
		c2, r2 = ColToNum(m[3]), filaDe(m[4])
	}
	return Rect{C1: min(c1, c2), R1: min(r1, r2), C2: max(c1, c2), R2: max(r1, r2)}, true
}

// clase es `endpointKind`: la clase de extremo de un token de palabra o número.
func claseExtremo(t *TokFx) (int, bool) {
	if t == nil || (t.T != FxWord && t.T != FxNum) {
		return 0, false
	}
	e, ok := parseRefEnd(t.V)
	return e.clase, ok
}

// rectDe es `rangeRectOf`: el rectángulo de una referencia SIN hoja.
func rectDe(sola string) (Rect, bool) {
	a, b, ok := strings.Cut(sola, ":")
	if !ok {
		return ParseRangeRef(sola)
	}
	x, okX := parseRefEnd(a)
	y, okY := parseRefEnd(b)
	if !okX || !okY || x.clase != y.clase {
		return Rect{}, false
	}
	switch x.clase {
	case extremoCol:
		return Rect{C1: min(x.col, y.col), R1: 0, C2: max(x.col, y.col), R2: EjeEntero}, true
	case extremoFila:
		return Rect{C1: 0, R1: min(x.fila, y.fila), C2: EjeEntero, R2: max(x.fila, y.fila)}, true
	}
	return Rect{C1: min(x.col, y.col), R1: min(x.fila, y.fila), C2: max(x.col, y.col), R2: max(x.fila, y.fila)}, true
}

// nombreHoja es `sheetNameOf`: el nombre de una hoja sin sus comillas.
func nombreHoja(t TokFx) string {
	if t.T != FxQSheet {
		return t.V
	}
	s := strings.TrimSuffix(strings.TrimPrefix(t.V, "'"), "'")
	return strings.ReplaceAll(s, "''", "'")
}

// ---- lo que hay bajo el cursor ----

// FuncToken es el identificador que se está tecleando: su prefijo hasta el cursor y
// dónde empieza y acaba (para sustituirlo entero al aceptar).
type FuncToken struct {
	Prefijo     string
	Inicio, Fin int
}

// FuncTokenAt es `funcTokenAt`: la palabra bajo el cursor que se puede autocompletar
// como función. false fuera de una palabra, con el prefijo vacío o tras un «!».
func FuncTokenAt(texto string, cursor int) (FuncToken, bool) {
	if !strings.HasPrefix(texto, "=") {
		return FuncToken{}, false
	}
	toks := TokenizeFx(texto)
	for k, t := range toks {
		if t.T != FxWord || cursor < t.Inicio || cursor > t.Fin {
			continue
		}
		if k > 0 && toks[k-1].T == FxBang {
			return FuncToken{}, false // una referencia cualificada por hoja
		}
		if cursor == t.Inicio {
			return FuncToken{}, false
		}
		return FuncToken{Prefijo: texto[t.Inicio:cursor], Inicio: t.Inicio, Fin: t.Fin}, true
	}
	return FuncToken{}, false
}

// ArgHint es la función que envuelve al cursor y el argumento en el que está.
type ArgHint struct {
	Nombre   string // en MAYÚSCULAS
	Arg      int    // de base cero
	Apertura int    // dónde está su «(»
}

// ArgHintAt es `argHintAt`: la función NOMBRADA más interna que rodea al cursor. Los
// paréntesis de agrupación cuentan la profundidad pero no dan pista.
func ArgHintAt(texto string, cursor int) (ArgHint, bool) {
	if !strings.HasPrefix(texto, "=") {
		return ArgHint{}, false
	}
	toks := TokenizeFx(texto)
	var pila []ArgHint
	for k, t := range toks {
		if t.Inicio >= cursor {
			break
		}
		switch t.T {
		case FxLParen:
			nombre := ""
			if k > 0 && toks[k-1].T == FxWord {
				nombre = strings.ToUpper(toks[k-1].V)
			}
			pila = append(pila, ArgHint{Nombre: nombre, Apertura: t.Inicio})
		case FxRParen:
			if len(pila) > 0 {
				pila = pila[:len(pila)-1]
			}
		case FxSep:
			if len(pila) > 0 {
				pila[len(pila)-1].Arg++
			}
		default: // el resto no abre, no cierra y no separa
		}
	}
	for i := len(pila) - 1; i >= 0; i-- {
		if pila[i].Nombre != "" {
			return pila[i], true
		}
	}
	return ArgHint{}, false
}

// ExpectsRef es `expectsRef`: el cursor está donde va un operando —tras el «=», un
// operador, un separador, un «(» o un «!»—, que es cuando pulsar una celda la escribe.
func ExpectsRef(texto string, cursor int) bool {
	if !strings.HasPrefix(texto, "=") {
		return false
	}
	var previo *TokFx
	toks := TokenizeFx(texto)
	for i := range toks {
		if toks[i].Fin > cursor {
			break
		}
		previo = &toks[i]
	}
	if previo == nil {
		return false
	}
	switch previo.T {
	case FxOp, FxSep, FxLParen, FxBang:
		return true
	default:
		return false
	}
}

// CycleRefAbs es `cycleRefAbs` (F4): A1 → $A$1 → A$1 → $A1 → A1, con el prefijo de
// hoja intacto y el MISMO estado en los dos extremos de un rango.
func CycleRefAbs(ref string) string {
	prefijo, cuerpo := "", ref
	if i := strings.LastIndexByte(ref, '!'); i >= 0 {
		prefijo, cuerpo = ref[:i+1], ref[i+1:]
	}
	partes := strings.Split(cuerpo, ":")
	primera, err := ParseCellWord(partes[0], "")
	if err != nil {
		return ref
	}
	estados := [4][2]bool{{false, false}, {true, true}, {false, true}, {true, false}}
	actual := 0
	switch {
	case primera.ColAbs && primera.RowAbs:
		actual = 1
	case primera.RowAbs:
		actual = 2
	case primera.ColAbs:
		actual = 3
	}
	sig := estados[(actual+1)%4]
	for i, p := range partes {
		r, err := ParseCellWord(p, "")
		if err != nil {
			continue
		}
		partes[i] = anclaje(sig[0]) + NumToCol(r.Col) + anclaje(sig[1]) + strconv.Itoa(r.Row+1)
	}
	return prefijo + strings.Join(partes, ":")
}

// CycleRefAt es `cycleRefAt`: F4 sobre la referencia que toca el cursor. Devuelve el
// texto nuevo y el cursor al final de la referencia (su largo cambia).
func CycleRefAt(texto string, cursor int) (string, int, bool) {
	r, ok := RefAtCaret(texto, cursor)
	if !ok {
		return texto, cursor, false
	}
	nueva := CycleRefAbs(r.Texto)
	return texto[:r.Inicio] + nueva + texto[r.Fin:], r.Inicio + len(nueva), true
}

// RefAtCaret es `refAtCaret`: la referencia cuyo tramo toca el cursor, ambos extremos
// incluidos. Es la que un clic de señalar SUSTITUYE en vez de añadir otra.
func RefAtCaret(texto string, cursor int) (Referencia, bool) {
	for _, r := range ExtractRefs(texto) {
		if cursor >= r.Inicio && cursor <= r.Fin {
			return r, true
		}
	}
	return Referencia{}, false
}

// ---- las referencias de una fórmula ----

// Referencia es una referencia hallada en una fórmula: su texto tal cual (con la hoja
// si la lleva), dónde está, su rectángulo y su índice de color, que es el mismo para la
// misma zona aunque cambie el anclaje.
type Referencia struct {
	Texto       string
	Inicio, Fin int
	Hoja        string // vacía si es local
	Rect        Rect
	Color       int
}

// ExtractRefs es `extractRefs` sin resolutor: las referencias y los rangos de una
// fórmula, en orden de aparición. Lo de dentro de una cadena, los nombres de función
// (una palabra seguida de «(») y los nombres de columna de `Tabla[…]` no son
// referencias.
func ExtractRefs(texto string) []Referencia { return ExtractRefsCon(texto, nil) }

// ResolutorEstructurada es `StructuredResolver`: la hoja y el rectángulo de una
// referencia `Tabla[Columna]`, o false si no se resuelve (tabla o columna desconocidas).
// Lo pone quien sabe qué tablas hay: el lienzo.
type ResolutorEstructurada func(q TableQuery) (hoja string, r Rect, ok bool)

// ExtractRefsCon es `extractRefs(text, resolveStructured)`: con resolutor, además de
// las A1 reconoce las estructuradas `Tabla[Columna]` y las resalta como el rango al que
// apuntan. El color va por zona DISTINTA y en orden de aparición, así que una
// estructurada y la A1 que cubre lo mismo comparten color, como en la web.
func ExtractRefsCon(texto string, resolver ResolutorEstructurada) []Referencia {
	if !strings.HasPrefix(texto, "=") {
		return nil
	}
	toks := TokenizeFx(texto)
	corchetes := tramosCorchete(texto, toks)
	var out []Referencia
	for k := 0; k < len(toks); k++ {
		r, salto, ok := refEn(texto, toks, k, corchetes)
		k += salto
		if ok {
			out = append(out, r)
		}
	}
	if resolver != nil {
		out = append(out, estructuradas(texto, corchetes, resolver)...)
		slices.SortStableFunc(out, func(a, b Referencia) int { return a.Inicio - b.Inicio })
	}
	colores := map[string]int{}
	for i := range out {
		r := &out[i]
		clave := r.Hoja + "!" + strconv.Itoa(r.Rect.C1) + "," + strconv.Itoa(r.Rect.R1) + "," +
			strconv.Itoa(r.Rect.C2) + "," + strconv.Itoa(r.Rect.R2)
		color, visto := colores[clave]
		if !visto {
			color = len(colores)
			colores[clave] = color
		}
		r.Color = color
	}
	return out
}

// letraNombreTabla es `TABLE_NAME_CH`: lo que cabe en un nombre de tabla.
func letraNombreTabla(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_' || b == '$' || b == '.'
}

// inicioNombreTabla es dónde empieza el nombre de tabla que acaba justo antes de `fin`.
func inicioNombreTabla(texto string, fin int) int {
	for fin > 0 && letraNombreTabla(texto[fin-1]) {
		fin--
	}
	return fin
}

// estructuradas es `collectStructuredRefs`: las `Tabla[…]` de los corchetes cerrados,
// resueltas. Las de un corchete sin cerrar o que no se resuelven no se resaltan.
func estructuradas(texto string, corchetes [][2]int, resolver ResolutorEstructurada) []Referencia {
	var out []Referencia
	for _, c := range corchetes {
		abre, cierra := c[0], c[1]
		if cierra >= len(texto) || texto[cierra] != ']' {
			continue
		}
		ini := inicioNombreTabla(texto, abre)
		q := ParseBracketSpec(texto[abre+1 : cierra])
		q.Table = texto[ini:abre]
		hoja, r, ok := resolver(q)
		if !ok {
			continue
		}
		out = append(out, Referencia{Texto: texto[ini : cierra+1], Inicio: ini, Fin: cierra + 1, Hoja: hoja, Rect: r})
	}
	return out
}

// CorcheteTabla es `TableBracket`: el nombre de columna que se teclea dentro de
// `Tabla[…` bajo el cursor.
type CorcheteTabla struct {
	// Tabla es el nombre antes del «[»; vacío si es la implícita («[@Col]»).
	Tabla string
	// Prefijo es lo tecleado tras el «[» (y el «@»), y [Inicio, Fin) su tramo.
	Prefijo     string
	Inicio, Fin int
	// EstaFila dice que había un «@».
	EstaFila bool
}

// TableBracketAt es `tableBracketAt`: el corchete de tabla abierto que envuelve al
// cursor, solo en la forma SIMPLE `Tabla[col` o la implícita `[@col`. Las anidadas
// (`[[#Datos],[col]]`) y las de área (`[#…]`) no se autocompletan.
func TableBracketAt(texto string, cursor int) (CorcheteTabla, bool) {
	if !strings.HasPrefix(texto, "=") || cursor > len(texto) {
		return CorcheteTabla{}, false
	}
	abre := -1
	for i := cursor - 1; i >= 0; i-- {
		if texto[i] == ']' {
			return CorcheteTabla{}, false // el corchete de antes ya se cerró
		}
		if texto[i] == '[' {
			abre = i
			break
		}
	}
	if abre < 0 {
		return CorcheteTabla{}, false
	}
	ini := inicioNombreTabla(texto, abre)
	if ini > 0 && (texto[ini-1] == '[' || texto[ini-1] == ',') {
		return CorcheteTabla{}, false // forma anidada [[#…],[…]]
	}
	j, fila := abre+1, false
	if j < len(texto) && texto[j] == '@' {
		fila = true
		j++
	}
	// `text.slice(j, caret)` con j > caret (el «@» justo tras el cursor) es "", no un fallo.
	prefijo := ""
	if j <= cursor {
		prefijo = texto[j:cursor]
	}
	if strings.ContainsAny(prefijo, "[],#") {
		return CorcheteTabla{}, false
	}
	return CorcheteTabla{Tabla: texto[ini:abre], Prefijo: prefijo, Inicio: j, Fin: cursor, EstaFila: fila}, true
}

// refEn mira si en el token k empieza una referencia (con su hoja delante y su rango
// detrás). `salto` son los tokens de más que se ha comido (los del rango).
func refEn(texto string, toks []TokFx, k int, corchetes [][2]int) (r Referencia, salto int, ok bool) {
	tok := func(i int) *TokFx {
		if i < 0 || i >= len(toks) {
			return nil
		}
		return &toks[i]
	}
	t := toks[k]
	clase, ok := claseExtremo(&t)
	if !ok || dentroDeCorchetes(t.Inicio, corchetes) {
		return Referencia{}, 0, false
	}
	if s := tok(k + 1); s != nil && (s.T == FxLParen || s.T == FxBang) {
		return Referencia{}, 0, false // un nombre de función o de hoja
	}
	inicio, hoja := t.Inicio, ""
	if p, p2 := tok(k-1), tok(k-2); p != nil && p.T == FxBang && p2 != nil && (p2.T == FxWord || p2.T == FxQSheet) {
		inicio, hoja = p2.Inicio, nombreHoja(*p2)
	}
	fin := t.Fin
	if s, s2 := tok(k+1), tok(k+2); s != nil && s.T == FxColon && mismaClase(s2, clase) && !esLParen(tok(k+3)) {
		fin, salto = s2.Fin, 2
	} else if clase != extremoCelda {
		return Referencia{}, 0, false // una columna o una fila SUELTA no es una referencia
	}
	rect, ok := rectDe(texto[t.Inicio:fin])
	if !ok {
		return Referencia{}, salto, false
	}
	return Referencia{Texto: texto[inicio:fin], Inicio: inicio, Fin: fin, Hoja: hoja, Rect: rect}, salto, true
}

func dentroDeCorchetes(p int, corchetes [][2]int) bool {
	for _, s := range corchetes {
		if p > s[0] && p < s[1] {
			return true
		}
	}
	return false
}

func mismaClase(t *TokFx, clase int) bool {
	c, ok := claseExtremo(t)
	return ok && c == clase
}

func esLParen(t *TokFx) bool { return t != nil && t.T == FxLParen }

// tramosCorchete es `tableBracketSpans`: los corchetes de tabla de nivel superior
// ([abre, cierra]), sin mirar dentro de las cadenas. Uno sin cerrar llega al final.
func tramosCorchete(texto string, toks []TokFx) [][2]int {
	var cadenas []TokFx
	for _, t := range toks {
		if t.T == FxStr {
			cadenas = append(cadenas, t)
		}
	}
	var out [][2]int
	prof, abre, si := 0, -1, 0
	for i := 0; i < len(texto); i++ {
		for si < len(cadenas) && cadenas[si].Fin <= i {
			si++
		}
		if si < len(cadenas) && i >= cadenas[si].Inicio {
			continue
		}
		switch texto[i] {
		case '[':
			if prof == 0 {
				abre = i
			}
			prof++
		case ']':
			if prof > 0 {
				prof--
				if prof == 0 {
					out = append(out, [2]int{abre, i})
				}
			}
		}
	}
	if prof > 0 {
		out = append(out, [2]int{abre, len(texto)})
	}
	return out
}
