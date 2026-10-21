// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package formula es @sytue/formula en Go: el motor de hoja de cálculo de Sytue Calcus
// y del ERP. Mismo parser, mismo grafo de dependencias, mismos derrames de arrays y las
// mismas funciones, con semántica OpenFormula / ECMA-376.
//
// «Lo mismo» no es una intención: testdata/trazas.json es lo que el motor TypeScript
// CONTESTÓ mientras corrían sus propios tests (scripts/grabar-formulas.ts), y el test de
// este paquete exige la misma respuesta a cada operación. Una hoja de cálculo que
// diverge en el decimoquinto decimal no avisa.
//
// Donde el original se apoya en JavaScript —cómo se escribe un número, cómo se ordenan
// dos textos, qué es un espacio— aquí se reproduce JavaScript, no lo que Go haría.
package formula

import (
	"errors"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Value es el valor calculado de una celda o subexpresión: float64, string, bool o
// Error. Una celda vacía vale "".
type Value = any

// Matrix es un rango calculado, filas × columnas. Es un tipo propio y no un alias para
// que un `switch` distinga sin ambigüedad el resultado escalar del matricial.
type Matrix [][]Value

// Error es un error de fórmula. Es un VALOR de celda: se propaga como tal.
type Error string

// Los códigos canónicos de una hoja de cálculo.
const (
	ErrDiv0  Error = "#DIV/0!"
	ErrValue Error = "#VALUE!"
	ErrName  Error = "#NAME?"
	ErrRef   Error = "#REF!"
	ErrNA    Error = "#N/A"
	ErrNum   Error = "#NUM!"
	ErrNull  Error = "#NULL!"
	ErrSpill Error = "#SPILL!" // un array dinámico no cabe: choca con una celda ocupada
	ErrCalc  Error = "#CALC!"  // un array vacío (FILTER/UNIQUE sin resultados)
	ErrCirc  Error = "#CIRC!"  // referencia circular
)

func (e Error) String() string { return string(e) }

// IsError dice si v es un error de fórmula.
func IsError(v Value) bool {
	_, ok := v.(Error)
	return ok
}

// FormatNumber formatea al «General» aproximado: enteros tal cual y decimales a 15
// cifras significativas, que es lo que colapsa el ruido binario (0.1+0.2 → «0.3»).
func FormatNumber(n float64) string {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return "#NUM!"
	}
	if n == math.Trunc(n) {
		return jsNumero(n)
	}
	redondeado, err := strconv.ParseFloat(aPrecision15(n), 64)
	if err != nil {
		return jsNumero(n)
	}
	return jsNumero(redondeado)
}

// aPrecision15 es `n.toPrecision(15)`, en notación científica. JavaScript redondea la
// mitad HACIA ARRIBA sobre la expansión decimal exacta; strconv redondea al par. Solo
// difieren en un empate exacto —2^-22 es uno—, y solo entonces se paga la exacta.
func aPrecision15(n float64) string {
	const cifras = 15
	abs := math.Abs(n)
	d17 := strconv.FormatFloat(abs, 'e', 16, 64) // d.dddddddddddddddd e±XX
	if d17[cifras+1] != '5' || d17[cifras+2] != '0' {
		return strconv.FormatFloat(n, 'e', cifras-1, 64)
	}
	// Posible empate. Un float64 tiene como mucho 767 cifras decimales significativas.
	exacto := new(big.Float).SetPrec(2200).SetFloat64(abs).Text('e', 800)
	mantisa, exp, _ := strings.Cut(exacto, "e")
	digitos := []byte(strings.Replace(mantisa, ".", "", 1))
	if digitos[cifras] >= '5' {
		i := cifras - 1
		for ; i >= 0 && digitos[i] == '9'; i-- {
			digitos[i] = '0'
		}
		if i < 0 { // 9,99…95 → 10,0…0
			e, _ := strconv.Atoi(exp)
			digitos, exp = append([]byte{'1'}, digitos...), strconv.Itoa(e+1)
		} else {
			digitos[i]++
		}
	}
	signo := ""
	if n < 0 {
		signo = "-"
	}
	return signo + string(digitos[0]) + "." + string(digitos[1:cifras]) + "e" + exp
}

// jsNumero es `String(n)`: el algoritmo Number::toString de ECMAScript. Decimal entre
// 1e-6 y 1e21, y notación exponencial fuera («1e+21», «1.5e-7»).
// NumeroJS es `String(n)` de JavaScript: el número más corto que vuelve a dar n, con
// el mismo paso a notación exponencial. Es como la web escribe un número en una celda.
func NumeroJS(n float64) string { return jsNumero(n) }

func jsNumero(n float64) string {
	switch {
	case math.IsNaN(n):
		return "NaN"
	case math.IsInf(n, 1):
		return "Infinity"
	case math.IsInf(n, -1):
		return "-Infinity"
	case n == 0:
		return "0"
	}
	signo := ""
	if n < 0 {
		signo, n = "-", -n
	}
	mantisa, expTxt, _ := strings.Cut(strconv.FormatFloat(n, 'e', -1, 64), "e")
	digitos := strings.Replace(mantisa, ".", "", 1)
	exp, _ := strconv.Atoi(expTxt)
	k, punto := len(digitos), exp+1 // punto: cuántas cifras van antes de la coma
	switch {
	case k <= punto && punto <= 21:
		return signo + digitos + strings.Repeat("0", punto-k)
	case 0 < punto && punto <= 21:
		return signo + digitos[:punto] + "." + digitos[punto:]
	case -6 < punto && punto <= 0:
		return signo + "0." + strings.Repeat("0", -punto) + digitos
	}
	e := strconv.Itoa(exp)
	if exp >= 0 {
		e = "+" + e
	}
	if k == 1 {
		return signo + digitos + "e" + e
	}
	return signo + digitos[:1] + "." + digitos[1:] + "e" + e
}

// jsNumber es `Number(s)` para las formas que produce el tokenizador y aceptan las
// regex de literal: lo que no parsea es NaN, y lo que desborda es ±Infinity.
func jsNumber(s string) float64 {
	n, err := strconv.ParseFloat(s, 64)
	if err != nil && !errorsIsRange(err) {
		return math.NaN()
	}
	return n
}

func errorsIsRange(err error) bool {
	var ne *strconv.NumError
	return errors.As(err, &ne) && errors.Is(ne.Err, strconv.ErrRange)
}

var reNumero = regexp.MustCompile(`^[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?$`)

// numeroDeTexto es el tramo común de coerceNumber y parseLiteral: un número limpio, con
// «%» final opcional.
func numeroDeTexto(t string) (float64, bool) {
	s, pct := t, false
	if strings.HasSuffix(s, "%") {
		pct, s = true, jsTrim(s[:len(s)-1])
	}
	if !reNumero.MatchString(s) {
		return 0, false
	}
	n := jsNumber(s)
	if pct {
		n /= 100
	}
	return n, true
}

// CoerceNumber es la coacción a número de hoja: booleano→1/0, texto→parseado (vacío es
// 0 y lo no numérico #VALUE!), error→se propaga.
func CoerceNumber(v Value) (float64, Error) {
	switch x := v.(type) {
	case float64:
		return x, ""
	case bool:
		if x {
			return 1, ""
		}
		return 0, ""
	case Error:
		return 0, x
	case string:
		t := jsTrim(x)
		if t == "" {
			return 0, "" // celda en blanco ≈ "" en contexto numérico
		}
		if n, ok := numeroDeTexto(t); ok {
			return n, ""
		}
	}
	return 0, ErrValue
}

// CoerceText es la coacción a texto: número→formateado, booleano→«TRUE»/«FALSE».
func CoerceText(v Value) (string, Error) {
	switch x := v.(type) {
	case string:
		return x, ""
	case float64:
		return FormatNumber(x), ""
	case bool:
		if x {
			return "TRUE", ""
		}
		return "FALSE", ""
	case Error:
		return "", x
	}
	return "", ErrValue
}

// CoerceBoolean es la coacción a booleano: número→(≠0), texto→TRUE/FALSE sin distinguir
// mayúsculas (vacío es FALSE; otro, #VALUE!).
func CoerceBoolean(v Value) (bool, Error) {
	switch x := v.(type) {
	case bool:
		return x, ""
	case float64:
		return x != 0, "" // NaN !== 0 es true en JavaScript, y aquí también
	case Error:
		return false, x
	case string:
		switch jsUpper(jsTrim(x)) {
		case "", "FALSE":
			return false, ""
		case "TRUE":
			return true, ""
		}
	}
	return false, ErrValue
}

// rango ordena los tipos mixtos: número < texto < booleano.
func rango(v Value) int {
	switch v.(type) {
	case float64:
		return 0
	case string:
		return 1
	}
	return 2
}

// CompareValues compara con la ordenación de hoja de cálculo: -1, 0 o 1, o el error si
// alguno lo es. El texto no distingue mayúsculas.
func CompareValues(a, b Value) (int, Error) {
	if e, ok := a.(Error); ok {
		return 0, e
	}
	if e, ok := b.(Error); ok {
		return 0, e
	}
	if ra, rb := rango(a), rango(b); ra != rb {
		if ra < rb {
			return -1, ""
		}
		return 1, ""
	}
	switch x := a.(type) {
	case float64:
		y := b.(float64) //nolint:errcheck,forcetypeassert // mismo rango: mismo tipo
		switch {
		case x < y:
			return -1, ""
		case x > y:
			return 1, ""
		}
		return 0, "" // también con NaN, como `a < b ? -1 : a > b ? 1 : 0`
	case bool:
		y := b.(bool) //nolint:errcheck,forcetypeassert // mismo rango: mismo tipo
		switch {
		case !x && y:
			return -1, ""
		case x && !y:
			return 1, ""
		}
		return 0, ""
	}
	x, _ := a.(string)
	y, _ := b.(string)
	return cmpUTF16(jsUpper(x), jsUpper(y)), ""
}

var (
	mayusculas = cases.Upper(language.Und)
	minusculas = cases.Lower(language.Und)
)

// jsUpper es `toUpperCase()`: el mapeo COMPLETO de Unicode («ß» → «SS»), no el simple de
// strings.ToUpper.
func jsUpper(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return mayusculas.String(s)
		}
	}
	return strings.ToUpper(s) // ASCII: el camino de casi todas las llamadas
}

// jsLower es `toLowerCase()`.
func jsLower(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return minusculas.String(s)
		}
	}
	return strings.ToLower(s)
}

// jsEspacio es lo que `trim()` considera espacio: WhiteSpace y LineTerminator de
// ECMAScript. No es unicode.IsSpace: ese cuenta U+0085 y no cuenta U+FEFF.
func jsEspacio(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xA0, 0xFEFF, 0x2028, 0x2029:
		return true
	}
	return r > 0x7f && unicode.Is(unicode.Zs, r)
}

// jsTrim es `trim()`.
func jsTrim(s string) string { return strings.TrimFunc(s, jsEspacio) }

// cmpUTF16 compara como `a < b` en JavaScript: por unidades UTF-16, no por bytes ni por
// puntos de código. Solo se nota fuera del plano básico, donde un par subrogado
// (0xD800…) ordena por DEBAJO de U+E000–U+FFFF.
func cmpUTF16(a, b string) int {
	for a != "" && b != "" {
		ra, na := utf8.DecodeRuneInString(a)
		rb, nb := utf8.DecodeRuneInString(b)
		if ra != rb {
			if ua, ub := unidad(ra), unidad(rb); ua != ub {
				return cmpInt(int(ua), int(ub))
			}
			return cmpInt(int(ra), int(rb))
		}
		a, b = a[na:], b[nb:]
	}
	return cmpInt(len(a), len(b))
}

// unidad es la primera unidad UTF-16 de un punto de código.
func unidad(r rune) rune {
	if r >= 0x10000 {
		return 0xD800 + (r-0x10000)>>10
	}
	return r
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
