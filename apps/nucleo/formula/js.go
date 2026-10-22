// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"math"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Lo que el original pide a `Math` y a `Number` y Go hace distinto. Cada función dice en
// qué difiere; donde no difiere se usa `math` a secas.

// jsPow es Math.pow: como math.Pow salvo que un exponente NaN da NaN aunque la base sea
// 1, y (±1)^±Infinity da NaN y no 1.
func jsPow(x, y float64) float64 {
	if math.IsNaN(y) || (math.IsInf(y, 0) && math.Abs(x) == 1) {
		return math.NaN()
	}
	return math.Pow(x, y)
}

// jsRound es Math.round: la mitad va hacia +∞ (Math.round(-2.5) = -2), no «lejos del
// cero» como math.Round. Conserva el -0 de Math.round(-0.2).
func jsRound(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	f := math.Floor(x)
	if x-f >= 0.5 {
		f++
	}
	if f == 0 && math.Signbit(x) {
		return math.Copysign(0, -1)
	}
	return f
}

// jsSign es Math.sign.
func jsSign(x float64) float64 {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return x // ±0 y NaN se devuelven tal cual
}

// prec15 es `Number(x.toPrecision(15))`: lo que limpia el ruido binario de un float64.
func prec15(x float64) float64 {
	if x == 0 {
		return 0 // también -0: la cadena es «0.00000000000000», sin signo
	}
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	n, err := strconv.ParseFloat(aPrecision15(x), 64)
	if err != nil {
		return x
	}
	return n
}

// jsToFixed es `x.toFixed(d)` para 0 ≤ d ≤ 100: redondea la mitad HACIA ARRIBA en
// magnitud sobre la expansión decimal exacta, donde strconv redondea al par. Solo
// difieren en un empate exacto (0.5, 2.5, 0.125…), y solo entonces se paga la exacta.
func jsToFixed(x float64, d int) string {
	if math.IsNaN(x) {
		return "NaN"
	}
	if math.Abs(x) >= 1e21 {
		return jsNumero(x)
	}
	abs := math.Abs(x)
	s := strconv.FormatFloat(abs, 'f', d, 64)
	// Un empate real es x·10^d = k+½, exacto en float64: el producto lo delata siempre.
	if y := abs * math.Pow(10, float64(d)); y-math.Floor(y) == 0.5 {
		s = mitadArriba(new(big.Float).SetPrec(2200).SetFloat64(abs).Text('f', 1080), d)
	}
	if x < 0 {
		return "-" + s // también «-0.00» para -0.001; solo el -0 sale sin signo
	}
	return s
}

var (
	reJSDecimal = regexp.MustCompile(`^[+-]?(Infinity|\d+\.?\d*([eE][+-]?\d+)?|\.\d+([eE][+-]?\d+)?)$`)
	reJSEntero  = regexp.MustCompile(`^0([xX][0-9a-fA-F]+|[oO][0-7]+|[bB][01]+)$`)
)

// jsNumberDe es `Number(texto)` entero: recorta, "" vale 0, admite Infinity con signo y
// los literales 0x, 0o y 0b (sin signo), y lo demás es NaN. strconv.ParseFloat acepta
// cosas que JavaScript no («inf», «nan», «1_000», hexadecimales con exponente): por eso
// va la forma validada por delante.
func jsNumberDe(s string) float64 {
	t := jsTrim(s)
	switch {
	case t == "":
		return 0
	case reJSEntero.MatchString(t):
		base := map[byte]int{'x': 16, 'o': 8, 'b': 2}[t[1]|0x20]
		n, _ := new(big.Int).SetString(t[2:], base)
		f, _ := new(big.Float).SetInt(n).Float64()
		return f
	case !reJSDecimal.MatchString(t):
		return math.NaN()
	}
	n, _ := strconv.ParseFloat(strings.Replace(t, "Infinity", "Inf", 1), 64)
	return n // un desbordamiento ya viene como ±Inf
}

/* ---------- Texto en unidades UTF-16 ----------
 * LEN, LEFT, MID, FIND… miden en unidades UTF-16, como una hoja real y como el original:
 * un emoji cuenta 2. Con texto ASCII —casi siempre— las unidades son los bytes y no se
 * convierte nada.
 * ponytail: cortar un par subrogado por la mitad da U+FFFD aquí y medio par en
 * JavaScript; un texto de Go no puede llevar medio par.
 */

func esASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// jsLen es `s.length`.
func jsLen(s string) int {
	if esASCII(s) {
		return len(s)
	}
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// indice normaliza un índice de `slice`: negativo cuenta desde el final, y se acota.
func indice(i float64, n int) int {
	switch {
	case math.IsNaN(i):
		return 0
	case i < 0:
		return int(math.Max(float64(n)+math.Trunc(i), 0))
	}
	return int(math.Min(math.Trunc(i), float64(n)))
}

// jsSlice es `s.slice(ini, fin)`.
func jsSlice(s string, ini, fin float64) string {
	if esASCII(s) {
		a, b := indice(ini, len(s)), indice(fin, len(s))
		if a >= b {
			return ""
		}
		return s[a:b]
	}
	u := utf16.Encode([]rune(s))
	a, b := indice(ini, len(u)), indice(fin, len(u))
	if a >= b {
		return ""
	}
	return string(utf16.Decode(u[a:b]))
}

// jsDesde es `s.slice(ini)`.
func jsDesde(s string, ini float64) string { return jsSlice(s, ini, math.Inf(1)) }

// jsIndexOf es `s.indexOf(aguja, desde)`, en unidades UTF-16.
func jsIndexOf(s, aguja string, desde float64) int {
	if esASCII(s) {
		d := indice(math.Max(desde, 0), len(s))
		if i := strings.Index(s[d:], aguja); i >= 0 {
			return d + i
		}
		return -1
	}
	u, a := utf16.Encode([]rune(s)), utf16.Encode([]rune(aguja))
	for i := indice(math.Max(desde, 0), len(u)); i+len(a) <= len(u); i++ {
		if slices.Equal(u[i:i+len(a)], a) {
			return i
		}
	}
	return -1
}

// esEntero es Number.isInteger.
func esEntero(x float64) bool { return !math.IsInf(x, 0) && x == math.Trunc(x) }

// esFinito es Number.isFinite.
func esFinito(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// mitadArriba recorta una expansión decimal exacta a d cifras redondeando la mitad
// hacia arriba.
func mitadArriba(exacto string, d int) string {
	entero, frac, _ := strings.Cut(exacto, ".")
	frac += strings.Repeat("0", d+1)
	digitos := []byte(entero + frac[:d])
	if frac[d] >= '5' {
		i := len(digitos) - 1
		for ; i >= 0 && digitos[i] == '9'; i-- {
			digitos[i] = '0'
		}
		if i < 0 {
			digitos = append([]byte{'1'}, digitos...)
		} else {
			digitos[i]++
		}
	}
	corte := len(digitos) - d
	if d == 0 {
		return string(digitos)
	}
	return string(digitos[:corte]) + "." + string(digitos[corte:])
}
