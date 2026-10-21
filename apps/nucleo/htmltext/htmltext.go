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
