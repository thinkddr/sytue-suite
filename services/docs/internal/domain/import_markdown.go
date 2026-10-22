// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strconv"
	"strings"
)

// Importador Markdown -> modelo rico (xblock) -> doc:html. Cubre el subconjunto
// CommonMark habitual y el que emite el exportador (export.go: título + párrafos):
// encabezados ATX, reglas horizontales, citas, listas (viñeta y ordenada) y el
// inline **negrita**, *cursiva*/_cursiva_, ~~tachado~~, `código`, [texto](url).
// Compacto y sin dependencias (rung ponytail: unas líneas antes que un parser).
// ponytail: no cubre tablas GFM ni bloques de código cercados; ampliar si se pide.

func importMarkdown(src string) string {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	var blocks []xblock
	var indents []int // sangrías de las listas abiertas: su tamaño es el nivel actual

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])

		switch {
		case trimmed == "":
			continue
		case isHR(trimmed):
			indents = nil
			blocks = append(blocks, xblock{hr: true})
			continue
		}
		// Encabezado ATX: 1..6 almohadillas + espacio.
		if lvl, rest, ok := atxHeading(trimmed); ok {
			indents = nil
			blocks = append(blocks, xblock{para: &xpara{
				style: "Heading" + strconv.Itoa(lvl),
				runs:  parseInline(rest),
			}})
			continue
		}
		// Cita.
		if q, ok := strings.CutPrefix(trimmed, ">"); ok {
			indents = nil
			blocks = append(blocks, xblock{para: &xpara{
				style: "Quote",
				runs:  parseInline(strings.TrimSpace(q)),
			}})
			continue
		}
		// Elemento de lista (viñeta u ordenada). El agrupado en <ul>/<ol> —y el
		// anidamiento— lo hace blocksToHTML; aquí se marca el tipo de cada ítem y su
		// nivel, que en Markdown lo da la SANGRÍA de la línea.
		if kind, content, ok := listItem(trimmed); ok {
			var level int
			indents, level = listLevel(indents, indentOf(lines[i]))
			blocks = append(blocks, xblock{para: &xpara{list: kind, level: level, runs: parseInline(content)}})
			continue
		}
		indents = nil
		// Párrafo: acumula líneas contiguas (salto simple = espacio).
		para := trimmed
		for i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			if next == "" || isHR(next) || isListStart(next) || strings.HasPrefix(next, ">") {
				break
			}
			if _, _, ok := atxHeading(next); ok {
				break
			}
			para += " " + next
			i++
		}
		blocks = append(blocks, xblock{para: &xpara{runs: parseInline(para)}})
	}
	if len(blocks) == 0 {
		return "<p></p>"
	}
	return blocksToHTML(blocks)
}

// indentOf mide la sangría de una línea en columnas (tabulador = 4).
func indentOf(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ':
			n++
		case '\t':
			n += 4
		default:
			return n
		}
	}
	return n
}

// listLevel sitúa un ítem con sangría `width` en la pila de sangrías abiertas y
// devuelve la pila actualizada y su nivel (0-based). Se compara con las sangrías
// vistas en lugar de dividir por un ancho fijo: así valen 2 espacios, 4 o un
// tabulador, que es como sangra cada editor.
func listLevel(indents []int, width int) ([]int, int) {
	for len(indents) > 0 && width < indents[len(indents)-1] {
		indents = indents[:len(indents)-1]
	}
	if len(indents) == 0 || width > indents[len(indents)-1] {
		indents = append(indents, width)
	}
	return indents, len(indents) - 1
}

// isHR indica una regla horizontal: 3+ de '-', '*' o '_' (con posibles espacios).
func isHR(s string) bool {
	compact := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	if len(compact) < 3 {
		return false
	}
	c := compact[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	return strings.Count(compact, string(c)) == len(compact)
}

// atxHeading parsea "# ...".."###### ...". Devuelve nivel y texto.
func atxHeading(s string) (level int, rest string, ok bool) {
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(s) || s[n] != ' ' {
		return 0, "", false
	}
	return n, strings.TrimSpace(strings.TrimRight(s[n+1:], " #")), true
}

// listItem detecta "- ", "* ", "+ " (viñeta) o "N. "/"N) " (ordenada).
func listItem(s string) (kind, content string, ok bool) {
	if len(s) >= 2 && (s[0] == '-' || s[0] == '*' || s[0] == '+') && s[1] == ' ' {
		return "bullet", strings.TrimSpace(s[2:]), true
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > 0 && i < len(s) && (s[i] == '.' || s[i] == ')') && i+1 < len(s) && s[i+1] == ' ' {
		return "number", strings.TrimSpace(s[i+2:]), true
	}
	return "", "", false
}

func isListStart(s string) bool {
	_, _, ok := listItem(s)
	return ok
}

// parseInline convierte el inline Markdown de una línea a tramos con formato.
// Reconoce enlaces, **/__ (negrita), */_ (cursiva), ~~ (tachado) y `código`
// (literal). Los delimitadores sin cierre se tratan como texto (robusto ante
// entrada arbitraria).
func parseInline(s string) []xrun {
	runs, _ := parseInlineUntil(s, xrun{}, "", 0)
	if len(runs) == 0 {
		return nil
	}
	return runs
}

// maxInlineDepth acota el anidamiento de énfasis/enlaces recorrido. La entrada es
// un fichero subido no confiable: sin tope, una línea con millones de
// delimitadores anidados desbordaría la pila (mismo motivo que maxWalkDepth en
// export_richtext.go).
const maxInlineDepth = 400

// parseInlineUntil recorre s aplicando el formato acumulado f y para al
// encontrar el delimitador de cierre `stop` (vacío = hasta el final). Devuelve
// los tramos producidos y el resto de s tras `stop`. Función pura sin estado
// compartido: cada emphasis/enlace recursa sobre su contenido (con tope de
// profundidad) y el llamante vuelca el buffer de texto antes de anexar los
// sub-tramos.
//
//nolint:gocognit // tokenizador de inline Markdown: ramificación inherente al formato
func parseInlineUntil(s string, f xrun, stop string, depth int) (out []xrun, rest string) {
	if depth > maxInlineDepth {
		// Demasiado anidamiento: el resto se emite como texto literal.
		if s != "" {
			nf := f
			nf.text = s
			out = append(out, nf)
		}
		return out, ""
	}
	var buf strings.Builder
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		nf := f
		nf.text = buf.String()
		buf.Reset()
		out = append(out, nf)
	}
	for s != "" {
		if stop != "" && strings.HasPrefix(s, stop) {
			flush()
			return out, s[len(stop):]
		}
		switch {
		case s[0] == '\\' && len(s) > 1: // escape: el siguiente byte es literal
			buf.WriteByte(s[1])
			s = s[2:]
		case s[0] == '`':
			if lit, n, ok := scanCode(s); ok {
				flush()
				nf := f
				nf.text = lit
				out = append(out, nf)
				s = s[n:]
			} else {
				buf.WriteByte('`')
				s = s[1:]
			}
		case s[0] == '[':
			if text, href, n, ok := scanLink(s); ok {
				flush()
				lf := f
				lf.link = href
				sub, _ := parseInlineUntil(text, lf, "", depth+1)
				out = append(out, sub...)
				s = s[n:]
			} else {
				buf.WriteByte('[')
				s = s[1:]
			}
		case strings.HasPrefix(s, "**"), strings.HasPrefix(s, "__"):
			if sub, r, ok := tryEmphasis(s, f, s[:2], func(nf *xrun) { nf.b = true }, depth); ok {
				flush()
				out, s = append(out, sub...), r
			} else {
				buf.WriteString(s[:2])
				s = s[2:]
			}
		case strings.HasPrefix(s, "~~"):
			if sub, r, ok := tryEmphasis(s, f, "~~", func(nf *xrun) { nf.strike = true }, depth); ok {
				flush()
				out, s = append(out, sub...), r
			} else {
				buf.WriteString("~~")
				s = s[2:]
			}
		case s[0] == '*' || s[0] == '_':
			if sub, r, ok := tryEmphasis(s, f, s[:1], func(nf *xrun) { nf.i = true }, depth); ok {
				flush()
				out, s = append(out, sub...), r
			} else {
				buf.WriteByte(s[0])
				s = s[1:]
			}
		default:
			buf.WriteByte(s[0])
			s = s[1:]
		}
	}
	flush()
	return out, ""
}

// tryEmphasis intenta parsear un tramo delimitado por `delim` (que abre s).
// ok=false si no hay delimitador de cierre (el llamante lo trata como literal).
func tryEmphasis(s string, f xrun, delim string, set func(*xrun), depth int) (out []xrun, rest string, ok bool) {
	inner := s[len(delim):]
	if !strings.Contains(inner, delim) {
		return nil, "", false
	}
	nf := f
	set(&nf)
	sub, rest := parseInlineUntil(inner, nf, delim, depth+1)
	return sub, rest, true
}

// scanCode lee `código` (o “código“). Devuelve el literal y los bytes consumidos.
func scanCode(s string) (string, int, bool) {
	ticks := 0
	for ticks < len(s) && s[ticks] == '`' {
		ticks++
	}
	fence := s[:ticks]
	end := strings.Index(s[ticks:], fence)
	if end < 0 {
		return "", 0, false
	}
	return strings.TrimSpace(s[ticks : ticks+end]), ticks + end + ticks, true
}

// scanLink lee [texto](href). Devuelve texto, href y bytes consumidos.
func scanLink(s string) (text, href string, n int, ok bool) {
	closeBr := strings.IndexByte(s, ']')
	if closeBr < 0 || closeBr+1 >= len(s) || s[closeBr+1] != '(' {
		return "", "", 0, false
	}
	closeParen := strings.IndexByte(s[closeBr+2:], ')')
	if closeParen < 0 {
		return "", "", 0, false
	}
	text = s[1:closeBr]
	href = strings.TrimSpace(s[closeBr+2 : closeBr+2+closeParen])
	return text, href, closeBr + 2 + closeParen + 1, true
}
