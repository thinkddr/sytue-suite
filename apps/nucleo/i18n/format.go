// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package i18n

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// formato.json lo genera gen-i18n.ts preguntándole a Intl: separador de miles, coma
// decimal, signo menos y a partir de cuántas cifras se agrupa. Sale de ahí y no de
// golang.org/x/text/message porque x/text lleva CLDR 32 y la web ya no formatea así:
// «1234» en español va sin punto y el francés agrupa con U+202F.
//
//go:embed formato.json
var formatoJSON []byte

type formatoNumero struct {
	Grupo    string `json:"grupo"`
	Decimal  string `json:"decimal"`
	Menos    string `json:"menos"`
	MinGrupo int    `json:"min_grupo"`
	// Porcentaje es el patrón con `{n}` donde va el número: «{n}%» en inglés y
	// «{n}\u00a0%» en los otros veinticinco. El SITIO del signo y lo que hay delante
	// son del idioma, y por eso es un patrón y no un sufijo.
	Porcentaje string `json:"porcentaje"`
	// Moneda y MonedaMenos son los patrones del importe en EUR, el positivo y el
	// negativo. Son DOS porque el menos no siempre va donde empieza el número: en
	// inglés el símbolo va delante («€1,234.56») y el menos delante del símbolo
	// («-€1,234.56»), así que con un solo patrón un abono saldría «€-1,234.56».
	Moneda      string `json:"moneda"`
	MonedaMenos string `json:"moneda_menos"`
	// Monedas son los patrones de las monedas del EEE y Suiza que no son el euro, por
	// su código ISO: dónde va el código —en inglés delante, «SEK 250.00», y en español
	// detrás— y cuántos decimales lleva (la corona islandesa, ninguno).
	Monedas map[string]formatoMoneda `json:"monedas"`
	// ListaPar va entre dos elementos de una enumeración; con tres o más, ListaEntre
	// va entre los primeros y ListaFinal antes del último (`Intl.ListFormat`).
	ListaPar   string `json:"lista_par"`
	ListaEntre string `json:"lista_entre"`
	ListaFinal string `json:"lista_final"`
}

var formatos = func() map[Locale]formatoNumero {
	var m map[Locale]formatoNumero
	if err := json.Unmarshal(formatoJSON, &m); err != nil {
		panic("i18n: formato.json: " + err.Error())
	}
	return m
}()

// FormatNumber es `value.toLocaleString(tag)`: separadores del idioma y hasta tres
// decimales, sin ceros de relleno.
func FormatNumber(l Locale, n float64) string { return formatear(l, n, 3, false) }

// FormatFixed es `toLocaleString(tag, { minimumFractionDigits: d, maximumFractionDigits:
// d })`: exactamente d decimales, con sus ceros («1,0 GB»).
func FormatFixed(l Locale, n float64, d int) string { return formatear(l, n, d, true) }

// FormatBytes es `formatBytes` de @sytue/ui: un tamaño de fichero como lo escribe cada
// idioma. Un decimal por debajo de 10 («1,5 GB») y ninguno por encima («512 MB»); las
// unidades son símbolos y no se traducen. Lo que no se sabe —un negativo, un NaN— es «—».
func FormatBytes(l Locale, n float64) string {
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return "—"
	}
	if n < 1024 {
		return FormatNumber(l, n) + " B"
	}
	unidades := []string{"KB", "MB", "GB", "TB"}
	v, i := n/1024, 0
	for v >= 1024 && i < len(unidades)-1 {
		v /= 1024
		i++
	}
	decimales := 0
	if v < 10 {
		decimales = 1
	}
	return FormatFixed(l, v, decimales) + " " + unidades[i]
}

// FormatPercent es `formatPercent` de format.ts: `value.toLocaleString(tag, { style:
// 'percent', maximumFractionDigits: 1 })`. El argumento es la FRACCIÓN (0,42 → «42 %»),
// igual que en la web.
//
// El signo no se pega a mano: en inglés va pegado al número y en los otros veinticinco
// idiomas de la suite lleva delante un espacio duro (U+00A0). Un «%» escrito en el
// código sería correcto en uno de veintiséis.
func FormatPercent(l Locale, fraccion float64) string {
	return strings.ReplaceAll(formatos[l].Porcentaje, "{n}", formatear(l, fraccion*100, 1, false))
}

// FormatList es `formatList` de format.ts (`Intl.ListFormat`, conjunción larga):
// «a, b y c» en español, «a, b and c» en inglés, «a, b und c» en alemán. Unirlos con
// una «y» del código sería correcto en un idioma de veintiséis.
//
// El español lleva la regla que ICU aplica a mano: la «y» pasa a «e» delante de un
// sonido /i/ —«i» o «hi», pero no «hia», «hie», «hio» ni «hiu»—: «Eva e Iker».
func FormatList(l Locale, xs []string) string {
	f := formatos[l]
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	}
	ultimo := xs[len(xs)-1]
	sep := f.ListaFinal
	if len(xs) == 2 {
		sep = f.ListaPar
	}
	if l == "es" && sep == " y " && suenaI(ultimo) {
		sep = " e "
	}
	return strings.Join(xs[:len(xs)-1], f.ListaEntre) + sep + ultimo
}

// suenaI es el caso de la «e» del español: empieza por «i» o por «hi» sin diptongo.
func suenaI(s string) bool {
	s = strings.ToLower(s)
	switch {
	case strings.HasPrefix(s, "i"):
		return true
	case strings.HasPrefix(s, "hi"):
		return len(s) == 2 || !strings.ContainsAny(s[2:3], "aeou")
	}
	return false
}

// FormatCurrency es `formatCurrency` de format.ts —`toLocaleString(tag, { style:
// 'currency', currency: 'EUR' })`— sobre un importe en CÉNTIMOS enteros, que es como lo
// guarda SyERP y como viaja por su API (services/syerp/internal/domain).
//
// No pasa por `float64` en ningún punto: la parte entera y los dos decimales salen de
// dividir el entero, así que no hay un 1234,56 que se convierta en 1234,5599999. Un
// céntimo perdido por factura es lo que esto evita.
//
// Las demás monedas van por FormatCurrencyIn.
func FormatCurrency(l Locale, centimos int64) string {
	f := formatos[l]
	return monedaCon(f, f.Moneda, f.MonedaMenos, 2, centimos)
}

// formatoMoneda es el patrón de una moneda que no es el euro en un idioma.
type formatoMoneda struct {
	Moneda      string `json:"moneda"`
	MonedaMenos string `json:"moneda_menos"`
	Decimales   int    `json:"decimales"`
}

// FormatCurrencyIn es `formatearDinero(centimos, locale, moneda)` del ERP —`Intl.
// NumberFormat(tag, { style: 'currency', currency })`— para cualquier moneda del EEE y
// de Suiza, con el importe en CÉNTIMOS (la centésima parte, también en las coronas).
//
// Una moneda sin decimales se REDONDEA como Intl, la mitad hacia arriba en valor
// absoluto: 1234,56 ISK son «1235 ISK». Un código que `formato.json` no trae —que no
// es de ningún país del ERP— sale con el número del idioma y el código detrás, que es
// lo que hace Intl en la mayoría de idiomas.
func FormatCurrencyIn(l Locale, centimos int64, codigo string) string {
	f := formatos[l]
	if codigo == "" || codigo == "EUR" {
		return FormatCurrency(l, centimos)
	}
	m, hay := f.Monedas[codigo]
	if !hay {
		return FormatFixed(l, float64(centimos)/100, 2) + "\u00a0" + codigo
	}
	return monedaCon(f, m.Moneda, m.MonedaMenos, m.Decimales, centimos)
}

// monedaCon escribe un importe en céntimos con un patrón y sus decimales (0 o 2) sin
// pasar por `float64`: la parte entera y los decimales salen de dividir el entero.
func monedaCon(f formatoNumero, patron, patronMenos string, decimales int, centimos int64) string {
	abs := centimos
	if centimos < 0 {
		patron, abs = patronMenos, -centimos
	}
	var n string
	if decimales == 0 {
		n = agrupar(f, strconv.FormatInt((abs+50)/100, 10))
	} else {
		n = agrupar(f, strconv.FormatInt(abs/100, 10)) + f.Decimal + fmt.Sprintf("%02d", abs%100)
	}
	return strings.ReplaceAll(patron, "{n}", n)
}

// agrupar mete el separador de millar del idioma en una parte entera ya escrita. Nueve
// idiomas no agrupan por debajo de 10.000 (`min_grupo`), y por eso no es un bucle de
// tres en tres a secas.
func agrupar(f formatoNumero, entero string) string {
	if len(entero) < 3+f.MinGrupo {
		return entero
	}
	var b strings.Builder
	for i, c := range entero {
		if i > 0 && (len(entero)-i)%3 == 0 {
			b.WriteString(f.Grupo)
		}
		b.WriteRune(c)
	}
	return b.String()
}

// Separadores son los del idioma tal y como los da Intl: el de millar y el decimal.
// Los necesita quien LEE lo que alguien teclea en un campo de número.
func Separadores(l Locale) (grupo, decimal string) {
	f := formatos[l]
	return f.Grupo, f.Decimal
}

// ParseFixed es la vuelta de FormatFixed: lee lo que alguien TECLEA en un campo de
// importe y lo devuelve en unidades de 10^-d —con d=2, céntimos—, sin pasar por float.
// 0,1 + 0,2 no es 0,3 y una factura no puede depender de eso: SyERP guarda el dinero en
// céntimos enteros justamente por eso (services/syerp/internal/domain).
//
// Admite el separador decimal del idioma y el otro, porque quien teclea en el bloque
// numérico de un portátil escribe el que traiga la tecla; los separadores de millar del
// idioma y los espacios finos; y el signo menos del idioma además del ASCII.
//
// Manda el separador DEL IDIOMA: si aparece, es el decimal y tiene que llevar detrás
// entre 1 y d cifras — «12,345 €» en español no son 12.345 €, son tres decimales, y se
// rechaza en vez de multiplicar por mil—. El otro carácter solo hace de decimal cuando
// el del idioma no está, aparece UNA vez y lleva detrás de 1 a d cifras; si no, es de
// millar y tiene que ir seguido de tres. Así «12.50» en español son 12,50 y «1.234»,
// mil doscientos treinta y cuatro.
func ParseFixed(l Locale, s string, d int) (int64, bool) {
	f := formatos[l]
	s = strings.TrimSpace(s)
	if f.Menos != "-" && f.Menos != "" {
		s = strings.ReplaceAll(s, f.Menos, "-")
	}
	for _, esp := range []string{" ", " ", " ", " ", "'"} {
		s = strings.ReplaceAll(s, esp, "")
	}
	negativo := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	if s == "" || len(s) > 24 {
		return 0, false
	}
	entero, decimales, ok := partirImporte(s, f.Decimal, d)
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseInt(entero+decimales+strings.Repeat("0", d-len(decimales)), 10, 64)
	if err != nil {
		return 0, false
	}
	if negativo {
		n = -n
	}
	return n, true
}

// partirImporte separa la parte entera (ya sin separadores de millar) de la decimal.
func partirImporte(s, decimal string, d int) (entero, decimales string, ok bool) {
	switch {
	case decimal != "" && strings.Contains(s, decimal):
		if strings.Count(s, decimal) != 1 {
			return "", "", false
		}
		corte := strings.Index(s, decimal)
		cola := s[corte+len(decimal):]
		if len(cola) < 1 || len(cola) > d || !soloCifras(cola) {
			return "", "", false
		}
		decimales, s = cola, s[:corte]
	default:
		if i := strings.LastIndexAny(s, ".,"); i >= 0 {
			cola, cual := s[i+1:], s[i:i+1]
			if len(cola) >= 1 && len(cola) <= d && soloCifras(cola) && strings.Count(s, cual) == 1 {
				decimales, s = cola, s[:i]
			}
		}
	}
	// Lo que queda son cifras y, como mucho, separadores de millar de tres en tres.
	for i, trozo := range strings.FieldsFunc(s, func(r rune) bool { return r == '.' || r == ',' }) {
		if i > 0 && len(trozo) != 3 {
			return "", "", false
		}
		entero += trozo
	}
	if entero == "" && decimales == "" {
		return "", "", false
	}
	if entero == "" {
		entero = "0"
	}
	if !soloCifras(entero) {
		return "", "", false
	}
	return entero, decimales, true
}

func soloCifras(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func formatear(l Locale, n float64, maxDec int, fijos bool) string {
	f := formatos[l]
	// Redondeo «mitad hacia fuera», como Intl; FormatFloat redondea al par.
	escala := math.Pow10(maxDec)
	unidades := math.Round(math.Abs(n) * escala)
	entero := strconv.FormatFloat(math.Floor(unidades/escala), 'f', 0, 64)
	decimales := ""
	if maxDec > 0 {
		decimales = strconv.FormatFloat(escala+math.Mod(unidades, escala), 'f', 0, 64)[1:]
		if !fijos {
			decimales = strings.TrimRight(decimales, "0")
		}
	}
	milesimas := unidades // el signo: «-0» no lleva menos

	var b strings.Builder
	if n < 0 && milesimas != 0 {
		b.WriteString(f.Menos)
	}
	// min_grupo 2: se agrupa a partir de cinco cifras («1234» pero «12.345»).
	b.WriteString(agrupar(f, entero))
	if decimales != "" {
		b.WriteString(f.Decimal + decimales)
	}
	return b.String()
}
