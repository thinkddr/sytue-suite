// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Tokenizador y analizador de descenso recursivo de lo que va tras el «=». Es
// parser.ts línea a línea: misma precedencia ECMA-376 (la negación unaria liga más
// fuerte que «^», por eso -2^2 = 4), mismos tokens, mismos casos de error.

// CellRef es una referencia a una celda. Col y Row son 0-based; Sheet vacío = sin
// cualificar.
type CellRef struct {
	Sheet  string
	Col    int
	Row    int
	ColAbs bool
	RowAbs bool
}

type clase uint8

const (
	nNum clase = iota
	nStr
	nBool
	nRef
	nRange
	nName
	nStructured
	nUnary
	nPercent
	nBinary
	nCall
	// nVacio es un argumento OMITIDO entre separadores: IF(A1,,2), GROUPBY(A:A,B:B,SUM,,0).
	nVacio
	// nArray es una constante matricial {1,2;3,4}: filas con «;», columnas con «,», solo literales.
	nArray
)

// Eje de un rango de eje entero (A:A, 1:1).
const (
	ejeNinguno = 0
	ejeCol     = 1
	ejeFila    = 2
)

// nodo es el AST. Un solo struct con los campos de todas las clases: es lo que el
// original expresa con una unión discriminada, y evita una interfaz por clase.
type nodo struct {
	clase  clase
	num    float64
	str    string // nStr: el texto; nName y nCall: el nombre; nBinary y nUnary: el operador
	b      bool
	a, z   CellRef // nRef usa a; nRange usa a y z
	eje    int     // nRange: ejeNinguno, ejeCol o ejeFila
	hoja   string  // nName: hoja que lo cualifica
	tabla  TableQuery
	izq    *nodo // nBinary; nUnary y nPercent guardan aquí su operando
	der    *nodo
	args   []*nodo
	matriz Matrix // nArray: sus valores
}

// ErrSintaxis es el ParseError del original: la fórmula no se entiende.
var ErrSintaxis = errors.New("formula: sintaxis")

func errSintaxis(msg string) error { return &errorSintaxis{msg} }

type errorSintaxis struct{ msg string }

func (e *errorSintaxis) Error() string   { return "formula: " + e.msg }
func (e *errorSintaxis) Is(t error) bool { return t == ErrSintaxis }

/* ---------- Coordenadas A1 ---------- */

// ColToNum: «A»→0, «Z»→25, «AA»→26….
func ColToNum(letras string) int {
	n := 0
	for i := 0; i < len(letras); i++ {
		c := letras[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		n = n*26 + int(c-64)
	}
	return n - 1
}

// NumToCol: 0→«A», 25→«Z», 26→«AA»….
func NumToCol(col int) string {
	n := col + 1
	out := ""
	for n > 0 {
		r := (n - 1) % 26
		out = string(rune('A'+r)) + out
		n = (n - 1) / 26
	}
	return out
}

var (
	reCelda = regexp.MustCompile(`^(\$?)([A-Za-z]{1,3})(\$?)(\d+)$`)
	reCol   = regexp.MustCompile(`^(\$?)([A-Za-z]{1,3})$`)
	reFila  = regexp.MustCompile(`^(\$?)(\d{1,7})$`)
)

// filaDe es `Number(txt) - 1` para una fila. ponytail: una fila con más cifras de las
// que caben en un int se queda en el tope; ninguna hoja llega ahí.
func filaDe(txt string) int {
	n, err := strconv.Atoi(txt)
	if err != nil {
		return 1<<31 - 2
	}
	return n - 1
}

// ParseCellWord parsea «A1», «$A$1», «B$3»… Falla si no encaja.
func ParseCellWord(w, sheet string) (CellRef, error) {
	m := reCelda.FindStringSubmatch(w)
	if m == nil {
		return CellRef{}, errSintaxis("referencia inválida: " + w)
	}
	return CellRef{Sheet: sheet, Col: ColToNum(m[2]), Row: filaDe(m[4]), ColAbs: m[1] == "$", RowAbs: m[3] == "$"}, nil
}

// Clase de extremo de una referencia.
const (
	extremoCelda = iota
	extremoCol
	extremoFila
)

type extremo struct {
	clase  int
	col    int
	fila   int
	colAbs bool
	rowAbs bool
}

// parseRefEnd clasifica una palabra como celda (A1), columna entera (A) o fila entera
// (1). Es la ÚNICA fuente de verdad del reconocimiento de referencias.
func parseRefEnd(w string) (extremo, bool) {
	if m := reCelda.FindStringSubmatch(w); m != nil {
		return extremo{extremoCelda, ColToNum(m[2]), filaDe(m[4]), m[1] == "$", m[3] == "$"}, true
	}
	if m := reCol.FindStringSubmatch(w); m != nil {
		return extremo{extremoCol, ColToNum(m[2]), 0, m[1] == "$", false}, true
	}
	if m := reFila.FindStringSubmatch(w); m != nil {
		return extremo{extremoFila, 0, filaDe(m[2]), false, m[1] == "$"}, true
	}
	return extremo{}, false
}

func (e extremo) ref(sheet string) CellRef {
	return CellRef{Sheet: sheet, Col: e.col, Row: e.fila, ColAbs: e.colAbs, RowAbs: e.rowAbs}
}

/* ---------- Tokenizador ---------- */

type tipoTok uint8

const (
	tNum tipoTok = iota
	tStr
	tWord
	tQSheet
	tOp
	tLParen
	tRParen
	tSep
	tColon
	tBang
	tPercent
	tBracket
	tArray // el interior crudo de una constante matricial, sin las llaves
)

type tok struct {
	t tipoTok
	v string // en tBracket: el texto interior crudo, sin los corchetes exteriores
}

func esDigito(c byte) bool { return c >= '0' && c <= '9' }

func esInicioPalabra(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_' || c == '$'
}

func esRestoPalabra(c byte) bool { return esInicioPalabra(c) || esDigito(c) || c == '.' }

// entrecomillado lee desde s[i] (la comilla de apertura) hasta la de cierre; la
// comilla doblada es una comilla literal.
func entrecomillado(s string, i int, comilla byte) (string, int) {
	var out strings.Builder
	j := i + 1
	for j < len(s) {
		if s[j] == comilla {
			if j+1 < len(s) && s[j+1] == comilla {
				out.WriteByte(comilla)
				j += 2
				continue
			}
			j++
			break
		}
		out.WriteByte(s[j])
		j++
	}
	return out.String(), j
}

// corchete captura la zona balanceada de `Tabla[…]` como un solo token. El apóstrofo
// escapa el siguiente carácter.
func corchete(s string, i int) (string, int, error) {
	var out strings.Builder
	depth, j := 1, i+1
	for j < len(s) && depth > 0 {
		ch := s[j]
		if ch == '\'' {
			out.WriteByte(ch)
			if j+1 < len(s) {
				out.WriteByte(s[j+1])
			}
			j += 2
			continue
		}
		if ch == '[' {
			depth++
		} else if ch == ']' {
			depth--
			if depth == 0 {
				j++
				break
			}
		}
		out.WriteByte(ch)
		j++
	}
	if depth != 0 {
		return "", 0, errSintaxis("falta «]» en la referencia de tabla")
	}
	return out.String(), j, nil
}

func numero(s string, i int) int {
	n, j := len(s), i+1
	for j < n && (esDigito(s[j]) || s[j] == '.') {
		j++
	}
	if j < n && (s[j] == 'e' || s[j] == 'E') {
		j++
		if j < n && (s[j] == '+' || s[j] == '-') {
			j++
		}
		for j < n && esDigito(s[j]) {
			j++
		}
	}
	return j
}

var simples = map[byte]tipoTok{'(': tLParen, ')': tRParen, ',': tSep, ';': tSep, ':': tColon, '!': tBang, '%': tPercent}

// lexLiteral lee un número, un texto entre comillas o una hoja entre apóstrofos.
func lexLiteral(s string, i, n int) (tok, int, bool) {
	switch c := s[i]; {
	case esDigito(c) || (c == '.' && i+1 < n && esDigito(s[i+1])):
		j := numero(s, i)
		return tok{tNum, s[i:j]}, j, true
	case c == '"':
		v, j := entrecomillado(s, i, '"')
		return tok{tStr, v}, j, true
	case c == '\'':
		v, j := entrecomillado(s, i, '\'')
		return tok{tQSheet, v}, j, true
	}
	return tok{}, i, false
}

// lexOperador lee «<=», «>=», «<>» y los operadores de un solo carácter.
func lexOperador(s string, i, n int) (tok, int, bool) {
	if i+1 < n && (s[i:i+2] == "<=" || s[i:i+2] == ">=" || s[i:i+2] == "<>") {
		return tok{tOp, s[i : i+2]}, i + 2, true
	}
	if strings.IndexByte("+-*/^&=<>", s[i]) >= 0 {
		return tok{tOp, s[i : i+1]}, i + 1, true
	}
	return tok{}, i, false
}

// lexMatriz lee una constante matricial desde s[i] (la llave de apertura) hasta su cierre,
// respetando los textos entre comillas: devuelve su interior crudo y dónde sigue.
func lexMatriz(s string, i int) (string, int, error) {
	j, enTexto := i+1, false
	for j < len(s) && (enTexto || s[j] != '}') {
		if s[j] == '"' {
			enTexto = !enTexto
		}
		j++
	}
	if j >= len(s) {
		return "", 0, errSintaxis("falta «}» en la constante matricial")
	}
	return s[i+1 : j], j + 1, nil
}

func lex(s string) ([]tok, error) {
	var toks []tok
	n := len(s)
	for i := 0; i < n; {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		if t, j, ok := lexLiteral(s, i, n); ok {
			toks, i = append(toks, t), j
			continue
		}
		if t, j, ok := lexOperador(s, i, n); ok {
			toks, i = append(toks, t), j
			continue
		}
		switch {
		case c == '{':
			v, j, err := lexMatriz(s, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, tok{tArray, v})
			i = j
		case c == '[':
			v, j, err := corchete(s, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, tok{tBracket, v})
			i = j
		case esInicioPalabra(c):
			j := i + 1
			for j < n && esRestoPalabra(s[j]) {
				j++
			}
			toks = append(toks, tok{tWord, s[i:j]})
			i = j
		default:
			t, ok := simples[c]
			if !ok {
				return nil, errSintaxis("carácter inesperado")
			}
			toks = append(toks, tok{t, s[i : i+1]})
			i++
		}
	}
	return toks, nil
}

/* ---------- Analizador ---------- */

type analizador struct {
	toks []tok
	p    int
	// prof es cuánto se ha anidado: paréntesis, «^» y signos pasan todos por unary.
	prof int
}

// maxAnidamiento acota la recursión del analizador (FYE-02). Sin él, una celda con
// cientos de miles de «(» desbordaba la pila al CARGAR la hoja —un error fatal de
// Go, sin recover posible— y una cadena de «-» se evaluaba en tiempo cuadrático.
// Excel admite 64 niveles de funciones anidadas; 256 de lo que sea sobra.
const maxAnidamiento = 256

var finTok = tok{t: 255}

func (a *analizador) peek() tok {
	if a.p < len(a.toks) {
		return a.toks[a.p]
	}
	return finTok
}

func (a *analizador) next() (tok, error) {
	if a.p >= len(a.toks) {
		return tok{}, errSintaxis("fin de expresión inesperado")
	}
	a.p++
	return a.toks[a.p-1], nil
}

func (a *analizador) esOp(ops ...string) bool {
	t := a.peek()
	if t.t != tOp {
		return false
	}
	for _, o := range ops {
		if t.v == o {
			return true
		}
	}
	return false
}

// nivel es un escalón de precedencia de operadores binarios asociativos por la
// izquierda: comparación, «&», suma y producto comparten este cuerpo.
func (a *analizador) nivel(ops []string, siguiente func() (*nodo, error)) (*nodo, error) {
	izq, err := siguiente()
	if err != nil {
		return nil, err
	}
	for a.esOp(ops...) {
		o, _ := a.next()
		der, err := siguiente()
		if err != nil {
			return nil, err
		}
		izq = &nodo{clase: nBinary, str: o.v, izq: izq, der: der}
	}
	return izq, nil
}

var (
	opsComparar = []string{"=", "<>", "<", ">", "<=", ">="}
	opsConcat   = []string{"&"}
	opsSuma     = []string{"+", "-"}
	opsProducto = []string{"*", "/"}
)

func (a *analizador) compare() (*nodo, error) { return a.nivel(opsComparar, a.concat) }
func (a *analizador) concat() (*nodo, error)  { return a.nivel(opsConcat, a.addSub) }
func (a *analizador) addSub() (*nodo, error)  { return a.nivel(opsSuma, a.mulDiv) }
func (a *analizador) mulDiv() (*nodo, error)  { return a.nivel(opsProducto, a.power) }

func (a *analizador) power() (*nodo, error) {
	base, err := a.unary()
	if err != nil {
		return nil, err
	}
	if a.esOp("^") {
		a.p++
		exp, err := a.power() // asociativo por la derecha
		if err != nil {
			return nil, err
		}
		return &nodo{clase: nBinary, str: "^", izq: base, der: exp}, nil
	}
	return base, nil
}

func (a *analizador) unary() (*nodo, error) {
	a.prof++
	defer func() { a.prof-- }()
	if a.prof > maxAnidamiento {
		return nil, errSintaxis("la fórmula anida demasiado")
	}
	if a.esOp("-", "+") {
		o, _ := a.next()
		operando, err := a.unary()
		if err != nil {
			return nil, err
		}
		return &nodo{clase: nUnary, str: o.v, izq: operando}, nil
	}
	return a.postfix()
}

func (a *analizador) postfix() (*nodo, error) {
	n, err := a.primary()
	if err != nil {
		return nil, err
	}
	for a.peek().t == tPercent {
		a.p++
		n = &nodo{clase: nPercent, izq: n}
	}
	return n, nil
}

func (a *analizador) primary() (*nodo, error) {
	t, err := a.next()
	if err != nil {
		return nil, err
	}
	switch t.t {
	case tNum:
		// Fila desnuda como extremo izquierdo de un rango de filas enteras: «3:5».
		if a.peek().t == tColon {
			if e, ok := parseRefEnd(t.v); ok && e.clase == extremoFila {
				a.p++
				return a.finishRange(e, "")
			}
		}
		return &nodo{clase: nNum, num: jsNumber(t.v)}, nil
	case tStr:
		return &nodo{clase: nStr, str: t.v}, nil
	case tArray:
		m, err := parseArrayLiteral(t.v)
		if err != nil {
			return nil, err
		}
		return &nodo{clase: nArray, matriz: m}, nil
	case tLParen:
		e, err := a.compare()
		if err != nil {
			return nil, err
		}
		if c, err := a.next(); err != nil || c.t != tRParen {
			return nil, errSintaxis("falta «)»")
		}
		return e, nil
	case tWord, tQSheet:
		return a.reference(t)
	case tBracket:
		// Corchete suelto `[@Columna]`: referencia estructurada de la tabla implícita.
		return structuredNode("", t.v), nil
	default: // un operador o un cierre no abren una expresión
	}
	return nil, errSintaxis("token inesperado: «" + t.v + "»")
}

func (a *analizador) llamada(nombre string) (*nodo, error) {
	a.p++ // «(»
	n := &nodo{clase: nCall, str: nombre}
	// Un argumento puede faltar entre dos separadores o antes del «)» tras un «,»; «F()»
	// sin nada sigue siendo cero argumentos.
	argumento := func() (*nodo, error) {
		if t := a.peek().t; t == tSep || t == tRParen {
			return &nodo{clase: nVacio}, nil
		}
		return a.compare()
	}
	if a.peek().t != tRParen {
		for {
			arg, err := argumento()
			if err != nil {
				return nil, err
			}
			n.args = append(n.args, arg)
			if a.peek().t != tSep {
				break
			}
			a.p++
		}
	}
	if c, err := a.next(); err != nil || c.t != tRParen {
		return nil, errSintaxis("falta «)» en la llamada")
	}
	return n, nil
}

// reference resuelve una palabra (o hoja) en ref, rango, llamada, booleano o nombre.
func (a *analizador) reference(head tok) (*nodo, error) {
	sheet, conHoja, word := "", false, head
	if a.peek().t == tBang {
		a.p++
		sheet, conHoja = head.v, true
		ref, err := a.next()
		// Tras «!» va una referencia: palabra (A1, A, $1…) o número (fila desnuda).
		if err != nil || (ref.t != tWord && ref.t != tNum) {
			return nil, errSintaxis("se esperaba una referencia tras «!»")
		}
		word = ref
	}
	// El original pregunta `!sheet`: una hoja de nombre vacío (''!A1) cuenta como sin
	// cualificar. ponytail: aquí además resuelve a la hoja en curso y no a una hoja «»,
	// porque Sheet vacío ES «sin cualificar»; nadie llama «» a una hoja.
	sinHoja := !conHoja || sheet == ""
	if sinHoja && a.peek().t == tBracket {
		b, _ := a.next()
		return structuredNode(word.v, b.v), nil
	}
	if sinHoja && a.peek().t == tLParen {
		return a.llamada(word.v)
	}
	if sinHoja {
		switch strings.ToUpper(word.v) {
		case "TRUE":
			return &nodo{clase: nBool, b: true}, nil
		case "FALSE":
			return &nodo{clase: nBool, b: false}, nil
		}
	}
	if e, ok := parseRefEnd(word.v); ok {
		if a.peek().t == tColon {
			a.p++
			return a.finishRange(e, sheet)
		}
		// Solo una CELDA suelta es referencia; una columna o fila sola cae a nombre.
		if e.clase == extremoCelda {
			return &nodo{clase: nRef, a: e.ref(sheet)}, nil
		}
	}
	return &nodo{clase: nName, str: word.v, hoja: sheet}, nil
}

// finishRange cierra un rango tras el «:»: exige extremos de la MISMA clase y
// normaliza los de eje entero invertidos (C:A → A:C).
func (a *analizador) finishRange(ini extremo, sheet string) (*nodo, error) {
	bt, err := a.next()
	if err != nil {
		return nil, err
	}
	fin, ok := extremo{}, false
	if bt.t == tWord || bt.t == tNum {
		fin, ok = parseRefEnd(bt.v)
	}
	if !ok {
		return nil, errSintaxis("se esperaba una referencia tras «:»")
	}
	if ini.clase != fin.clase {
		return nil, errSintaxis("extremos de rango incompatibles")
	}
	if ini.clase == extremoCelda {
		return &nodo{clase: nRange, a: ini.ref(sheet), z: fin.ref(sheet)}, nil
	}
	eje, invertir := ejeCol, ini.col > fin.col
	if ini.clase == extremoFila {
		eje, invertir = ejeFila, ini.fila > fin.fila
	}
	if invertir {
		ini, fin = fin, ini
	}
	return &nodo{clase: nRange, a: ini.ref(sheet), z: fin.ref(sheet), eje: eje}, nil
}

/* ---------- Referencias estructuradas de tabla ---------- */

// TableArea es el área de una referencia estructurada.
type TableArea string

// Las áreas de una tabla.
const (
	AreaData    TableArea = "data"
	AreaAll     TableArea = "all"
	AreaHeaders TableArea = "headers"
	AreaTotals  TableArea = "totals"
)

// TableQuery es la petición de resolución de `Tabla[Columna]`. Table vacío = tabla
// implícita (la que contiene la celda); HasColumn falso = solo se pide un área.
type TableQuery struct {
	Table     string
	Column    string
	HasColumn bool
	Area      TableArea
	ThisRow   bool
}

func structuredNode(tabla, interior string) *nodo {
	q := ParseBracketSpec(interior)
	q.Table = tabla
	return &nodo{clase: nStructured, tabla: q}
}

var reEscape = regexp.MustCompile(`'([\[\]#'@])`)

func unescapeSpec(s string) string { return reEscape.ReplaceAllString(s, "$1") }

// sinEspacios quita lo que `\s` de JavaScript considera espacio (no solo el ASCII).
func sinEspacios(r rune) rune {
	if jsEspacio(r) {
		return -1
	}
	return r
}

var areas = map[string]TableArea{
	"todo": AreaAll, "all": AreaAll,
	"datos": AreaData, "data": AreaData,
	"cabeceras": AreaHeaders, "headers": AreaHeaders, "header": AreaHeaders,
	"totales": AreaTotals, "totals": AreaTotals, "total": AreaTotals,
}

// clasificar aplica un especificador simple (`#Área`, `@Columna`, `@` o un nombre de
// columna) sobre q. El prefijo se mira SIN desescapar: `['@Total]` es una columna.
func clasificar(q *TableQuery, crudo string) {
	t := jsTrim(crudo)
	switch {
	case strings.HasPrefix(t, "#"):
		clave := jsLower(strings.Map(sinEspacios, t[1:]))
		if clave == "estafila" || clave == "thisrow" {
			q.ThisRow = true
		} else if area, ok := areas[clave]; ok {
			q.Area = area
		}
	case strings.HasPrefix(t, "@"):
		q.ThisRow = true
		if resto := jsTrim(t[1:]); resto != "" {
			q.Column, q.HasColumn = unescapeSpec(resto), true
		}
	default:
		q.Column, q.HasColumn = unescapeSpec(t), true
	}
}

// bracketGroups extrae los grupos `[…]` de `[#Datos],[Columna]`.
func bracketGroups(inner string) []string {
	var partes []string
	for i := 0; i < len(inner); {
		if inner[i] != '[' {
			i++
			continue
		}
		var out strings.Builder
		depth, j := 1, i+1
		for j < len(inner) && depth > 0 {
			c := inner[j]
			if c == '\'' {
				if j+1 < len(inner) {
					out.WriteByte(inner[j+1])
				}
				j += 2
				continue
			}
			if c == '[' {
				depth++
			} else if c == ']' {
				depth--
				if depth == 0 {
					break
				}
			}
			out.WriteByte(c)
			j++
		}
		partes = append(partes, out.String())
		i = j + 1
	}
	return partes
}

// ParseBracketSpec interpreta el interior de un corchete estructurado. Función pura.
func ParseBracketSpec(inner string) TableQuery {
	q := TableQuery{Area: AreaData}
	t := jsTrim(inner)
	partes := []string{t}
	if strings.HasPrefix(t, "[") {
		partes = bracketGroups(t)
	}
	for _, p := range partes {
		clasificar(&q, p)
	}
	return q
}

// parseFormula parsea una expresión; acepta un «=» inicial opcional.
func parseFormula(input string) (*nodo, error) {
	toks, err := lex(strings.TrimPrefix(input, "="))
	if err != nil {
		return nil, err
	}
	a := &analizador{toks: toks}
	n, err := a.compare()
	if err != nil {
		return nil, err
	}
	if a.p < len(a.toks) {
		return nil, errSintaxis("sobra «" + a.peek().v + "» tras la expresión")
	}
	return n, nil
}

// erroresLiterales son los errores que se pueden escribir como elemento de una constante matricial.
var erroresLiterales = map[string]Error{
	"#DIV/0!": ErrDiv0, "#VALUE!": ErrValue, "#NAME?": ErrName, "#REF!": ErrRef,
	"#N/A": ErrNA, "#NUM!": ErrNum, "#NULL!": ErrNull,
}

var reNumeroLiteral = regexp.MustCompile(`^[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?$`)

// parseArrayLiteral es el interior de una constante matricial («1,2;3,4») como filas de
// valores: números con signo, textos entre comillas, VERDADERO/FALSO, errores y huecos. Todas
// las filas con las mismas columnas, como en Excel. Es `parseArrayLiteral` de parser.ts.
func parseArrayLiteral(inner string) (Matrix, error) {
	rows := Matrix{{}}
	var cur strings.Builder
	enTexto, eraTexto := false, false
	cierra := func() error {
		v, err := elementoLiteral(strings.TrimSpace(cur.String()), eraTexto)
		if err != nil {
			return err
		}
		rows[len(rows)-1] = append(rows[len(rows)-1], v)
		cur.Reset()
		eraTexto = false
		return nil
	}
	for _, c := range inner {
		switch {
		case c == '"':
			enTexto, eraTexto = !enTexto, true
			cur.WriteRune(c)
		case !enTexto && c == ',':
			if err := cierra(); err != nil {
				return nil, err
			}
		case !enTexto && c == ';':
			if err := cierra(); err != nil {
				return nil, err
			}
			rows = append(rows, []Value{})
		default:
			cur.WriteRune(c)
		}
	}
	if err := cierra(); err != nil {
		return nil, err
	}
	for _, r := range rows {
		if len(r) != len(rows[0]) {
			return nil, errSintaxis("las filas de una constante matricial deben tener las mismas columnas")
		}
	}
	return rows, nil
}

// elementoLiteral lee un elemento de una constante matricial; `entrecomillado` dice que llevaba
// comillas («""» es un texto vacío y no un hueco).
func elementoLiteral(t string, entrecomillado bool) (Value, error) {
	switch {
	case entrecomillado:
		if len(t) < 2 || t[0] != '"' || t[len(t)-1] != '"' || strings.Contains(strings.ReplaceAll(t[1:len(t)-1], `""`, ""), `"`) {
			return nil, errSintaxis("elemento de constante matricial no válido")
		}
		return strings.ReplaceAll(t[1:len(t)-1], `""`, `"`), nil
	case t == "":
		return "", nil
	case strings.EqualFold(t, "TRUE") || strings.EqualFold(t, "VERDADERO"):
		return true, nil
	case strings.EqualFold(t, "FALSE") || strings.EqualFold(t, "FALSO"):
		return false, nil
	case erroresLiterales[strings.ToUpper(t)] != "":
		return erroresLiterales[strings.ToUpper(t)], nil
	case reNumeroLiteral.MatchString(t):
		return jsNumber(t), nil
	}
	return nil, errSintaxis("elemento de constante matricial no válido: «" + t + "»")
}
