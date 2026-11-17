// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PEGAR MARKDOWN. Es `markdownAHtml` de `client/markdown.ts`, letra por letra: lo que se
// pega de un `.md`, de Obsidian o de un asistente llega como texto lleno de marcas, y si
// «parece Markdown» (dos o más marcas de bloque) entra con formato —títulos, listas,
// tareas, citas, código, regla, negrita, cursiva, tachado y enlaces http(s)—. Lo demás
// entra como siempre, como texto.
//
// El resultado es HTML en el vocabulario del servicio y lo lee `Leer`, el mismo lector
// de las notas guardadas: no hay un segundo camino al modelo que pueda discrepar. Todo el
// texto sale escapado, así que lo que traiga de HTML el Markdown se queda en letras.
//
// Los casos son `testdata/markdown.json`, que lee también la web.

var (
	mdTitulo = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	mdTarea  = regexp.MustCompile(`^(?:[-*+]\s+)?\[([ xX]?)\](?:\s+(.*))?$`)
	mdVineta = regexp.MustCompile(`^[-*+]\s+(.*)$`)
	mdNumero = regexp.MustCompile(`^\d{1,9}[.)]\s+(.*)$`)
	mdCita   = regexp.MustCompile(`^>\s?(.*)$`)
	mdRegla  = regexp.MustCompile(`^(-{3,}|\*{3,})$`)
	mdValla  = regexp.MustCompile("^```")
	mdEnlace = regexp.MustCompile(`^\[([^\]]+)\]\((https?://[^\s)]+)\)`)
	mdBloque = []*regexp.Regexp{mdTitulo, mdTarea, mdVineta, mdNumero, mdCita, mdRegla, mdValla}
)

// marcasMinimas son las marcas de bloque que hacen falta para tratar un pegado como
// Markdown: una frase con un asterisco o un «#etiqueta» suelto no lo es.
const marcasMinimas = 2

func lineasMd(texto string) []string {
	texto = strings.ReplaceAll(texto, "\r\n", "\n")
	return strings.Split(strings.ReplaceAll(texto, "\r", "\n"), "\n")
}

// pareceMarkdown dice si el texto plano tiene al menos dos líneas con marca de bloque.
func pareceMarkdown(texto string) bool {
	marcas := 0
	for _, l := range lineasMd(texto) {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		for _, re := range mdBloque {
			if re.MatchString(t) {
				marcas++
				break
			}
		}
		if marcas >= marcasMinimas {
			return true
		}
	}
	return false
}

// escaparMd escapa como la web (`&`, `<`, `>`, `"`); `html.EscapeString` también
// escapa el apóstrofo y los casos comunes dejarían de coincidir.
var escaparMd = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace

func esPalabra(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }

// cierreMd dice dónde cierra la marca `d` abierta en `i`, o -1 si no es una marca.
func cierreMd(s string, i int, d string) int {
	tras, _ := utf8.DecodeRuneInString(s[i+len(d):])
	if i+len(d) >= len(s) || unicode.IsSpace(tras) {
		return -1
	}
	// «snake_case» no es cursiva: el guion bajo solo marca fuera de una palabra.
	bajo := d[0] == '_'
	if antes, _ := utf8.DecodeLastRuneInString(s[:i]); bajo && i > 0 && esPalabra(antes) {
		return -1
	}
	for desde := i + len(d) + 1; desde <= len(s); {
		k := strings.Index(s[desde:], d)
		if k < 0 {
			return -1
		}
		j := desde + k
		desde = j + 1
		previo, _ := utf8.DecodeLastRuneInString(s[:j])
		if unicode.IsSpace(previo) {
			continue
		}
		if len(d) == 1 && (s[j-1] == d[0] || (j+1 < len(s) && s[j+1] == d[0])) {
			continue
		}
		if despues, _ := utf8.DecodeRuneInString(s[j+len(d):]); bajo && j+len(d) < len(s) && esPalabra(despues) {
			continue
		}
		return j
	}
	return -1
}

var marcasEnLinea = []struct{ d, etiqueta string }{
	{"**", "b"}, {"__", "b"}, {"~~", "s"}, {"*", "i"}, {"_", "i"},
}

//nolint:gocognit // es un recorrido de marcas; partirlo esconde el orden en que se prueban.
func enLineaMd(s string) string {
	var out strings.Builder
	i := 0
sigue:
	for i < len(s) {
		c := s[i]
		if c == '`' {
			if j := strings.IndexByte(s[i+1:], '`'); j > 0 {
				out.WriteString("<code>" + escaparMd(s[i+1:i+1+j]) + "</code>")
				i += j + 2
				continue
			}
		}
		if c == '[' {
			if m := mdEnlace.FindStringSubmatch(s[i:]); m != nil {
				fmt.Fprintf(&out, `<a href="%s">%s</a>`, escaparMd(m[2]), enLineaMd(m[1]))
				i += len(m[0])
				continue
			}
		}
		for _, mc := range marcasEnLinea {
			if !strings.HasPrefix(s[i:], mc.d) {
				continue
			}
			j := cierreMd(s, i, mc.d)
			if j < 0 {
				continue
			}
			fmt.Fprintf(&out, "<%s>%s</%s>", mc.etiqueta, enLineaMd(s[i+len(mc.d):j]), mc.etiqueta)
			i = j + len(mc.d)
			continue sigue
		}
		// Una runa entera: cortar a medias una letra con tilde rompería el UTF-8.
		_, n := utf8.DecodeRuneInString(s[i:])
		out.WriteString(escaparMd(s[i : i+n]))
		i += n
	}
	return out.String()
}

// markdownAHTML convierte Markdown al HTML de una nota. Una línea suelta es un párrafo;
// las líneas en blanco solo separan.
//
//nolint:gocognit,gocyclo,cyclop // es el despacho por tipo de línea, igual que la web.
func markdownAHTML(texto string) string {
	var out []string
	var listaTipo string
	var items, cita, codigo []string
	enCita, enCodigo := false, false
	cerrar := func() {
		if listaTipo != "" {
			out = append(out, "<"+listaTipo+">"+strings.Join(items, "")+"</"+listaTipo+">")
		}
		if enCita {
			out = append(out, "<blockquote>"+strings.Join(cita, "<br>")+"</blockquote>")
		}
		listaTipo, items, enCita, cita = "", nil, false, nil
	}
	enLista := func(tipo, contenido string) {
		if listaTipo != tipo {
			cerrar()
			listaTipo = tipo
		}
		items = append(items, "<li>"+enLineaMd(contenido)+"</li>")
	}
	cerrarCodigo := func() {
		esc := make([]string, len(codigo))
		for k, l := range codigo {
			esc[k] = escaparMd(l)
		}
		out = append(out, "<pre>"+strings.Join(esc, "<br>")+"</pre>")
		enCodigo, codigo = false, nil
	}
	for _, linea := range lineasMd(texto) {
		t := strings.TrimSpace(linea)
		if enCodigo {
			if mdValla.MatchString(t) {
				cerrarCodigo()
			} else {
				codigo = append(codigo, linea)
			}
			continue
		}
		switch {
		case mdValla.MatchString(t):
			cerrar()
			enCodigo = true
		case t == "":
			cerrar()
		case mdRegla.MatchString(t):
			cerrar()
			out = append(out, "<hr>")
		case mdTitulo.MatchString(t):
			cerrar()
			m := mdTitulo.FindStringSubmatch(t)
			n := min(len(m[1]), 5)
			out = append(out, fmt.Sprintf("<h%d>%s</h%d>", n, enLineaMd(m[2]), n))
		case mdTarea.MatchString(t):
			cerrar()
			m := mdTarea.FindStringSubmatch(t)
			marcada := ""
			if m[1] == "x" || m[1] == "X" {
				marcada = " checked"
			}
			out = append(out, `<div class="task-row"><input type="checkbox"`+marcada+
				`> <span>`+enLineaMd(m[2])+`</span></div>`)
		case mdVineta.MatchString(t):
			enLista("ul", mdVineta.FindStringSubmatch(t)[1])
		case mdNumero.MatchString(t):
			enLista("ol", mdNumero.FindStringSubmatch(t)[1])
		case mdCita.MatchString(t):
			if !enCita {
				cerrar()
				enCita = true
			}
			cita = append(cita, enLineaMd(mdCita.FindStringSubmatch(t)[1]))
		default:
			cerrar()
			out = append(out, "<p>"+enLineaMd(t)+"</p>")
		}
	}
	if enCodigo {
		cerrarCodigo()
	}
	cerrar()
	return strings.Join(out, "")
}

// documentoDePegado es lo que pega una nota de `texto`: el Markdown convertido si lo
// parece, o nil si entra como texto plano.
func documentoDePegado(texto string) *Documento {
	if !pareceMarkdown(texto) {
		return nil
	}
	d := Leer(markdownAHTML(texto))
	if d.Vacio() {
		return nil
	}
	return d
}
