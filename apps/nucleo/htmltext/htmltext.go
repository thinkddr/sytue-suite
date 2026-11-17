// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package htmltext saca el TEXTO de un fragmento de HTML, para enseñarlo donde no hay
// quien pinte HTML: el resumen de una nota, el cuerpo de un correo en una lista.
//
// No interpreta nada: lo que devuelve se pinta como texto, así que un <script> es, como
// mucho, una palabra que no sale. No es un saneador y no debe usarse como tal.
package htmltext

import (
	"html"
	"regexp"
	"strings"
	"unicode"
)

var (
	reOculto   = regexp.MustCompile(`(?is)<(script|style|head|template)\b.*?</(script|style|head|template)\s*>`)
	reBloque   = regexp.MustCompile(`(?i)<(br\s*/?|/p|/div|/li|/h[1-6]|/tr|/blockquote|/pre)\s*>`)
	reVineta   = regexp.MustCompile(`(?i)<li\b[^>]*>`)
	reEtiqueta = regexp.MustCompile(`(?s)<[^>]*>`)
	reBlancos  = regexp.MustCompile(`[ \t\f\v\x{a0}]+`)
	reSaltos   = regexp.MustCompile(`\n{3,}`)
)

// HasMarkup dice si el contenido trae formato: algo que editar como texto plano perdería.
func HasMarkup(s string) bool { return reEtiqueta.MatchString(s) }

// Text devuelve el texto legible: los bloques como líneas, las viñetas con su «•», las
// entidades resueltas.
func Text(s string) string {
	s = reOculto.ReplaceAllString(s, "")
	s = reVineta.ReplaceAllString(s, "• ")
	s = reBloque.ReplaceAllString(s, "\n")
	s = html.UnescapeString(reEtiqueta.ReplaceAllString(s, ""))
	s = reBlancos.ReplaceAllString(s, " ")
	lineas := strings.Split(s, "\n")
	for i := range lineas {
		lineas[i] = strings.TrimSpace(lineas[i])
	}
	return strings.TrimSpace(reSaltos.ReplaceAllString(strings.Join(lineas, "\n"), "\n\n"))
}

// reEntidad son las CINCO entidades que deshace Raw, en el orden de la web: `&amp;` va
// la última para que `&amp;lt;` vuelva como el texto «&lt;» y no como «<».
var reEntidad = strings.NewReplacer("&nbsp;", " ", "&lt;", "<", "&gt;", ">", "&quot;", `"`)

// Escape es el `escapeHtml` de packages/office/src/editor/sheets/format.ts: &, <, > y ".
//
// No es html.EscapeString, que además convierte el apóstrofo en `&#39;`: eso volvería
// distinto de como lo escribe la web, y el mismo documento se lee en las dos.
func Escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// Raw es el `stripHtml` de la misma fuente: el texto crudo de UNA celda. No es Text: en
// una celda un <br> no es un salto de línea —una celda es una línea—, y solo se deshacen
// las entidades que escribe Escape, porque son las únicas que la web pone.
//
// Tiene que dar exactamente lo que da la web: es lo que entra en el motor de fórmulas, y
// dos resultados distintos sobre el mismo documento serían dos hojas distintas.
func Raw(s string) string {
	return strings.ReplaceAll(reEntidad.Replace(reEtiqueta.ReplaceAllString(s, "")), "&amp;", "&")
}

// Summary es el texto en UNA línea, recortado a n runas: el resumen de una lista.
func Summary(s string, n int) string {
	t := strings.Join(strings.Fields(Text(s)), " ")
	if r := []rune(t); len(r) > n {
		return strings.TrimSpace(string(r[:n])) + "…"
	}
	return t
}

// TextStats son las cuentas de «Información» de una nota.
type TextStats struct {
	Palabras   int `json:"palabras"`
	Caracteres int `json:"caracteres"`
	// Minutos de lectura a PalabrasPorMinuto, redondeando hacia arriba.
	Minutos int `json:"minutos"`
}

// PalabrasPorMinuto es el ritmo de lectura silenciosa que usan Medium, Bear o Notion.
const PalabrasPorMinuto = 200

// Stats es `estadisticas` de apps/notes/src/client/extracto.ts sobre un texto PLANO
// (el de Flat): palabras, caracteres y minutos de lectura. Los vectores de
// testdata/estadisticas.json los recorren las dos, y tienen que dar lo mismo.
//
// Los caracteres son runas del texto con los blancos juntados en un espacio. Una
// palabra es una racha de letras y cifras; un apóstrofo o un guion entre letras
// («don't», «e-mail») y un punto o una coma entre cifras («1.250») no la cortan. Una
// marca combinante (un acento suelto, el dakuten de un kana descompuesto, el selector
// de variante de «❤️») va con lo que tiene delante y nunca empieza una palabra: si no,
// un emoji o un «が» en NFD contarían de más. Han, hiragana y katakana se escriben sin
// espacios y cada carácter cuenta como una palabra, como en Word; el hangul separa con
// espacios y va como los demás.
func Stats(texto string) TextStats {
	rs := []rune(strings.Join(camposJS(texto), " "))
	palabras, enPalabra := 0, false
	for i, c := range rs {
		switch {
		case unicode.IsMark(c):
			// Sigue lo que hubiera: ni empieza ni corta una palabra.
		case esCJK(c):
			palabras++
			enPalabra = false
		case esLetra(c):
			if !enPalabra {
				palabras++
			}
			enPalabra = true
		case enPalabra && i+1 < len(rs) && une(rs[i-1], c, rs[i+1]):
			// Sigue la misma palabra.
		default:
			enPalabra = false
		}
	}
	return TextStats{
		Palabras:   palabras,
		Caracteres: len(rs),
		Minutos:    (palabras + PalabrasPorMinuto - 1) / PalabrasPorMinuto,
	}
}

func esCJK(c rune) bool {
	return unicode.In(c, unicode.Han, unicode.Hiragana, unicode.Katakana)
}

func esLetra(c rune) bool {
	return unicode.IsLetter(c) || unicode.IsNumber(c)
}

// esBlancoJS es el `\s` de JavaScript, que es lo que juntan textoDeNota y estadisticas
// en la web. No es unicode.IsSpace (el de strings.Fields): JavaScript cuenta como blanco
// U+FEFF (el BOM que llega pegado de otros sitios) y no U+0085, y con Fields los dos
// darían números distintos del mismo texto.
func esBlancoJS(c rune) bool {
	return c == '\uFEFF' || (c != '\u0085' && unicode.IsSpace(c))
}

func camposJS(s string) []string { return strings.FieldsFunc(s, esBlancoJS) }

func une(antes, c, despues rune) bool {
	if esCJK(despues) || !esLetra(despues) {
		return false
	}
	switch c {
	case '\'', '’', '-', '‐':
		return true
	case '.', ',':
		return unicode.IsDigit(antes) && unicode.IsDigit(despues)
	}
	return false
}

var (
	reOcultoPlano = regexp.MustCompile(`(?is)<(script|style|head|template|noscript|iframe|object)\b.*?</(script|style|head|template|noscript|iframe|object)\s*>`)
	reBloquePlano = regexp.MustCompile(`(?i)</?(br|hr|p|div|li|ul|ol|blockquote|pre|h[1-6]|table|thead|tbody|tfoot|tr|td|th)\b[^>]*>`)
)

// Flat es `textoDeNota` de extracto.ts: el texto de una nota en UNA línea, sin las
// viñetas que pone Text, con cada bloque separado por un espacio. Es lo que se cuenta
// con Stats; con Text, el «•» de cada viñeta sería un carácter que la web no ve.
func Flat(s string) string {
	s = reOcultoPlano.ReplaceAllString(s, "")
	s = reBloquePlano.ReplaceAllString(s, " ")
	s = html.UnescapeString(reEtiqueta.ReplaceAllString(s, ""))
	return strings.Join(camposJS(s), " ")
}
