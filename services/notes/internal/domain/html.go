// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"

	"golang.org/x/net/html"
)

// SANEADO DE TEXTO ENRIQUECIDO
//
// El editor del navegador es un `contenteditable`, así que lo que llega es HTML.
// Ese HTML lo pinta después el navegador de OTRA persona (un cuaderno de
// organización, un enlace público), de modo que saneado en cliente no es saneado:
// se hace AQUÍ, en el lado que no se puede saltar reenviando la petición a mano.
//
// La política es una ALLOWLIST cerrada —etiquetas conocidas, atributos contados—
// y todo lo demás desaparece. Un `<script>` no se escapa: se descarta con su
// contenido, que es lo que evita que reaparezca al copiar y pegar la nota.
//
// Se tokeniza con golang.org/x/net/html, que ya está en el módulo (lo usa el
// unfurl de Rooms): un saneador de allowlist son cuarenta líneas sobre él y no
// justifica traer una dependencia más (CONVENTIONS §3, cadena de suministro).

// allowedTags son las etiquetas que sobreviven al saneado. Es el vocabulario de
// un editor de notas: dar formato, listas, encabezados, cita, código y enlaces.
var allowedTags = map[string]bool{
	"p": true, "br": true, "div": true, "span": true,
	"b": true, "strong": true, "i": true, "em": true, "u": true,
	"s": true, "strike": true, "del": true, "mark": true,
	"code": true, "pre": true, "blockquote": true, "hr": true,
	"ul": true, "ol": true, "li": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"a": true,
	// Tablas: una nota de reunión o un comparativo son una tabla, y sin estas seis
	// etiquetas el saneador tiraba la rejilla y dejaba las celdas como texto
	// corrido. El exportador a texto plano ya las contemplaba (`tr` en cortaLinea).
	"table": true, "thead": true, "tbody": true, "tr": true, "th": true, "td": true,
	// Tareas: casillas de verificación interactivas en listas de tareas.
	"input": true,
}

// voidTags no llevan cierre.
var voidTags = map[string]bool{"br": true, "hr": true, "input": true}

// droppedSubtrees son las etiquetas cuyo CONTENIDO también se tira. Escapar el
// texto de un <script> lo dejaría visible como código en la nota; borrarlo entero
// es lo que espera quien pega desde otra página.
var droppedSubtrees = map[string]bool{
	"script": true, "style": true, "iframe": true, "object": true,
	"embed": true, "template": true, "noscript": true, "svg": true, "math": true,
}

// saneador lleva el estado del recorrido: lo emitido, la pila de etiquetas
// abiertas y la profundidad dentro de un subárbol descartado.
//
// Existe para que los dos casos con miga —abrir y cerrar— salgan del `switch` y
// se lean por separado. Estaban en línea y el bucle acumulaba una complejidad
// cognitiva de 49, casi el doble del límite: la parte de cerrar, que es la que
// tiene la búsqueda hacia atrás en la pila, quedaba enterrada bajo la de abrir.
type saneador struct {
	out      strings.Builder
	abiertas []string
	saltando int
}

// abre atiende una etiqueta de apertura (o autocerrada).
func (s *saneador) abre(z *html.Tokenizer, autocerrada bool) {
	name, hasAttr := z.TagName()
	tag := string(name)
	if droppedSubtrees[tag] {
		// Una autocerrada no abre subárbol; una normal sí.
		if !autocerrada {
			s.saltando++
		}
		return
	}
	if s.saltando > 0 || !allowedTags[tag] {
		return
	}
	s.out.WriteString("<" + tag)
	s.out.WriteString(atributosPermitidos(z, tag, hasAttr))
	s.out.WriteString(">")
	if !voidTags[tag] && !autocerrada {
		s.abiertas = append(s.abiertas, tag)
	}
}

// cierra atiende una etiqueta de cierre.
func (s *saneador) cierra(z *html.Tokenizer) {
	name, _ := z.TagName()
	tag := string(name)
	if droppedSubtrees[tag] {
		if s.saltando > 0 {
			s.saltando--
		}
		return
	}
	if s.saltando > 0 || !allowedTags[tag] || voidTags[tag] {
		return
	}
	// Cierra hasta la apertura correspondiente. Un cierre HUÉRFANO —</p> sin
	// <p>— se descarta: emitirlo dejaría el HTML desbalanceado y el navegador
	// lo «arregla» tragándose lo que viene detrás.
	idx := -1
	for i := len(s.abiertas) - 1; i >= 0; i-- {
		if s.abiertas[i] == tag {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	for i := len(s.abiertas) - 1; i >= idx; i-- {
		s.out.WriteString("</" + s.abiertas[i] + ">")
	}
	s.abiertas = s.abiertas[:idx]
}

// termina cierra lo que quedara abierto y devuelve el resultado.
func (s *saneador) termina() string {
	for i := len(s.abiertas) - 1; i >= 0; i-- {
		s.out.WriteString("</" + s.abiertas[i] + ">")
	}
	return strings.TrimSpace(s.out.String())
}

// SanitizeHTML devuelve el HTML de entrada reducido a la allowlist. Nunca
// devuelve error: lo que no está permitido, no sale.
func SanitizeHTML(in string) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return ""
	}
	var s saneador
	z := html.NewTokenizer(strings.NewReader(in))
	for {
		switch tt := z.Next(); tt {
		case html.ErrorToken:
			return s.termina()
		case html.TextToken:
			if s.saltando == 0 {
				s.out.WriteString(html.EscapeString(string(z.Text())))
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			s.abre(z, tt == html.SelfClosingTagToken)
		case html.EndTagToken:
			s.cierra(z)
		default:
			// Comentarios y doctype: fuera sin más.
		}
	}
}

// atributosPermitidos devuelve los atributos que sobreviven para esa etiqueta, ya
// serializados y escapados. La lista es corta a propósito: `href` en enlaces (solo
// http/https/mailto) y `style` reducido a color de texto y de fondo.
func atributosPermitidos(z *html.Tokenizer, tag string, hasAttr bool) string {
	if !hasAttr {
		return ""
	}
	var b strings.Builder
	for {
		k, v, more := z.TagAttr()
		clave, valor := string(k), string(v)
		switch {
		case clave == "href" && tag == "a":
			if ValidWebURL(valor) || strings.HasPrefix(valor, "mailto:") {
				b.WriteString(` href="` + html.EscapeString(valor) + `"`)
				// Un enlace de una nota ajena abre en otra pestaña SIN darle acceso a
				// esta: `noopener` cierra window.opener, `noreferrer` no filtra de
				// dónde viene.
				b.WriteString(` target="_blank" rel="noopener noreferrer"`)
			}
		case clave == "type" && tag == "input":
			if strings.EqualFold(valor, "checkbox") {
				b.WriteString(` type="checkbox"`)
			}
		case (clave == "checked" || clave == "disabled") && tag == "input":
			b.WriteString(` ` + html.EscapeString(clave))
		case clave == "class":
			if clasePermitida(tag, valor) {
				b.WriteString(` class="` + html.EscapeString(valor) + `"`)
			}
		case clave == "style":
			if s := estiloPermitido(valor); s != "" {
				b.WriteString(` style="` + html.EscapeString(s) + `"`)
			}
		}
		if !more {
			return b.String()
		}
	}
}

// clasePermitida dice si una clase sobrevive: la fila y la casilla de una tarea, y
// `plegada` en un `<li>`, que es la lista plegada (como en Twos): se guarda con la nota y
// la lectura esconde su sublista.
func clasePermitida(tag, valor string) bool {
	return valor == "task-row" || valor == "task-checkbox" || (valor == "plegada" && tag == "li")
}

// estiloPermitido reduce un `style` a las declaraciones que un editor de
// notas necesita —color y fondo, solo con colores #RRGGBB, alineación del
// bloque, solo con las cuatro palabras, y cursor interactivo—. Todo lo demás (`url(...)`, `position`,
// `expression`) se cae por no estar en la lista, no por filtrarse.
func estiloPermitido(raw string) string {
	var partes []string
	for _, decl := range strings.Split(raw, ";") {
		prop, valor, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		prop = strings.ToLower(strings.TrimSpace(prop))
		valor = strings.TrimSpace(valor)
		if (prop == "color" || prop == "background-color") && ValidHexColor(valor) {
			partes = append(partes, prop+":"+valor)
		}
		if prop == "text-align" && alineaciones[strings.ToLower(valor)] {
			partes = append(partes, prop+":"+strings.ToLower(valor))
		}
		if prop == "cursor" && valor == "pointer" {
			partes = append(partes, "cursor:pointer")
		}
	}
	return strings.Join(partes, ";")
}

// alineaciones son los únicos valores que admite `text-align`. La barra del editor
// ofrece justo estos cuatro; cualquier otra cosa en esa propiedad no viene de ella.
var alineaciones = map[string]bool{"left": true, "center": true, "right": true, "justify": true}
