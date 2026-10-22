// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package numfmt es el FORMATO DE NÚMERO de una celda de Sytue Sheets: los presets de
// `sheets/format.ts` en es-ES y las máscaras personalizadas de `sheets/numfmt-mask.ts`.
//
// Vive en el núcleo, al lado del motor de fórmulas, porque lo necesitan dos sitios que
// tienen que dar el MISMO texto: la celda que pinta Calcus y la hoja que descarga el
// servicio de Docs (Markdown, HTML y PDF). Dos copias de quinientas líneas de
// redondeos y fechas divergen en el primer caso raro (docs/CONVENTIONS.md §13.2).
package numfmt

import (
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Aplicar es `applyNumFmt` (más el color de `numFmtInnerHtml`): el número con el
// formato `id` —general, number, thousands, currency, percent, date o custom—, con `d`
// decimales y, en `custom`, la máscara. El color es el de un `[Rojo]` de máscara, o "".
func Aplicar(id string, d int, mascara string, n float64) (texto, color string) {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return formula.FormatNumber(n), ""
	}
	switch id {
	case "number":
		return NumeroES(n, d, 0, false, 0), ""
	case "thousands":
		return NumeroES(n, d, 0, true, 1), ""
	case "currency":
		return NumeroES(n, d, 0, true, 1) + "\u00a0€", ""
	case "percent":
		// El estilo `percent` de Intl multiplica por 100 el valor EXACTO, no el float
		// ya redondeado, y agrupa con el mínimo de es-ES (dos cifras): 1234 % sale
		// junto y 12.345 % separado.
		return NumeroES(n, d, 2, true, 2) + "\u00a0%", ""
	case "date":
		return fechaSerial(n), ""
	case "custom":
		if mascara == "" {
			return formula.FormatNumber(n), ""
		}
		return ConMascara(mascara, n)
	}
	return formula.FormatNumber(n), "" // general
}

// NumeroES es `Intl.NumberFormat('es-ES')` con d decimales fijos: coma decimal, punto
// de millares y el signo de los negativos (también el del cero negativo, como Intl).
//
// `agrupar` enciende los millares, y `minimo` es cuántas cifras tiene que haber por
// encima del primer grupo para separarlo: 1 es `useGrouping: 'always'` y 2 es el
// mínimo de es-ES (`'auto'`), por el que 1234 sale sin punto.
func NumeroES(n float64, d, esc int, agrupar bool, minimo int) string {
	signo := ""
	if n < 0 || math.Signbit(n) {
		signo = "-"
	}
	ent, dec := fijo(math.Abs(n), d, esc)
	if agrupar && len(ent) >= 3+minimo {
		ent = millares(ent, ".")
	}
	if dec != "" {
		return signo + ent + "," + dec
	}
	return signo + ent
}

// fijo es `x.toFixed(d)` sobre el valor EXACTO de x·10^esc, con x ≥ 0: la mitad se
// redondea hacia arriba, que es lo que hacen `toFixed` y el `halfExpand` de Intl. No
// vale `strconv`, que redondea los empates exactos al par (0,125 → «0.12»).
func fijo(x float64, d, esc int) (ent, dec string) {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		x = 0 // nunca llega (lo filtran quienes llaman), pero un pánico aquí cierra la app
	}
	const prec = 512 // 53 bits de mantisa + 10^108 caben de sobra
	v := new(big.Float).SetPrec(prec).SetFloat64(x)
	if k := d + esc; k > 0 {
		v.Mul(v, new(big.Float).SetPrec(prec).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(k)), nil)))
	}
	i, _ := v.Int(nil) // trunca: x ≥ 0
	resto := new(big.Float).SetPrec(prec).Sub(v, new(big.Float).SetPrec(prec).SetInt(i))
	if resto.Cmp(big.NewFloat(0.5)) >= 0 {
		i.Add(i, big.NewInt(1))
	}
	s := i.String()
	if d == 0 {
		return s, ""
	}
	if len(s) <= d {
		s = strings.Repeat("0", d-len(s)+1) + s
	}
	return s[:len(s)-d], s[len(s)-d:]
}

// millares mete el separador cada tres cifras desde la derecha.
func millares(cifras, sep string) string {
	var b strings.Builder
	for i, r := range cifras {
		if i > 0 && (len(cifras)-i)%3 == 0 {
			b.WriteString(sep)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// El sistema de fechas 1900 del motor: el serial 0 es el 31-12-1899 y el 60 es el
// «29-2-1900» que no existió (el error histórico de Lotus que Excel conserva).
var epocaFechas = time.Date(1899, 12, 31, 0, 0, 0, 0, time.UTC)

const maxSerialFecha = 2_958_465 // 9999-12-31

// fechaSerial es `formatDateSerial`: la parte de día del serial en «d/m/aaaa», que es
// lo que da `Intl.DateTimeFormat('es-ES')`. Fuera del sistema, el General.
func fechaSerial(n float64) string {
	dia := math.Trunc(n)
	if dia < 1 || dia > maxSerialFecha {
		return formula.FormatNumber(n)
	}
	if dia == 60 {
		return "29/2/1900"
	}
	dias := int(dia)
	if dias > 60 {
		dias--
	}
	t := epocaFechas.AddDate(0, 0, dias)
	return strconv.Itoa(t.Day()) + "/" + strconv.Itoa(int(t.Month())) + "/" + strconv.Itoa(t.Year())
}
