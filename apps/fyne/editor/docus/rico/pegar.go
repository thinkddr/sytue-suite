// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// # Pegar con formato
//
// Lo que llega del portapapeles de otra aplicación (Word, LibreOffice, un navegador) es
// HTML ajeno, ya pasado por la regla de saneado del documento (`apps/nucleo/saneado`): sin
// scripts ni manejadores, pero con todo lo demás que esa aplicación escribe —clases de
// Word, `mso-*`, `<div>` anidados, márgenes—. ReducirPegado lo deja en lo que este paquete
// ENTIENDE, para que el documento siga siendo editable después de pegar: párrafos,
// títulos, citas, listas, tablas, reglas y, dentro de la línea, negrita, cursiva,
// subrayado, tachado, super y subíndice, enlaces, colores, letra y tamaño, saltos e
// imágenes `data:`. Lo que no se entiende se DESENVUELVE (se queda su texto), nunca entra.

// Pegado es lo que queda de un HTML pegado, listo para meterlo en el documento.
type Pegado struct {
	// HTML son los bloques que pegar (`<p>`, `<h2>`, `<ul>`, `<table>`…).
	HTML string
	// EnLinea dice que es UN párrafo corriente: se pega dentro del párrafo del cursor, sin
	// partirlo, como hace un navegador con lo copiado de en medio de una frase.
	EnLinea bool
}

// tiposDeImagenPegada son las imágenes `data:` que entran: las de mapa de bits que pinta
// cualquier cliente. Un SVG no, que es por donde se cuela un script; y una `https:` no,
// que delataría a quien abre el documento cada vez que se pintara.
var tiposDeImagenPegada = []string{"data:image/png;", "data:image/jpeg;", "data:image/gif;", "data:image/webp;"}

// contenedores son los elementos que solo agrupan bloques: se desenvuelven y sus hijos se
// leen como bloques. Un `<div>` de Chrome o de Word es casi siempre uno de estos.
var contenedores = map[atom.Atom]bool{
	atom.Div: true, atom.Section: true, atom.Article: true, atom.Header: true, atom.Footer: true,
	atom.Main: true, atom.Aside: true, atom.Nav: true, atom.Figure: true, atom.Figcaption: true,
	atom.Center: true, atom.Address: true, atom.Details: true, atom.Summary: true, atom.Dl: true,
	atom.Dd: true, atom.Dt: true, atom.Hgroup: true, atom.Body: true, atom.Caption: true,
}

// bloqueEnLinea son los elementos que, dentro de una línea, empiezan otra: se desenvuelven con
// un salto delante.
var bloqueEnLinea = map[atom.Atom]bool{
	atom.P: true, atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Blockquote: true, atom.Pre: true, atom.Ul: true, atom.Ol: true, atom.Li: true, atom.Tr: true,
	atom.Table: true, atom.Hr: true,
}

// formatoEnLinea son las marcas que entran tal cual (sin atributos, con su `style` reducido).
// `<del>` e `<ins>` sin la marca del control de cambios son tachado y subrayado.
var formatoEnLinea = map[atom.Atom]string{
	atom.B: "b", atom.Strong: "strong", atom.I: "i", atom.Em: "em", atom.U: "u", atom.S: "s",
	atom.Strike: "strike", atom.Sup: "sup", atom.Sub: "sub", atom.Del: "s", atom.Ins: "u",
}

// ReducirPegado deja un HTML ya saneado en lo que el documento entiende. Vacío si no queda
// nada que pegar.
func ReducirPegado(saneado string) Pegado {
	nodos, err := html.ParseFragment(strings.NewReader(saneado), cuerpo())
	if err != nil {
		return Pegado{}
	}
	fuente := cuerpo()
	for _, n := range nodos {
		fuente.AppendChild(n)
	}
	r := &reductor{}
	destino := cuerpo()
	r.bloques(fuente, destino, 0, false)
	r.cerrarParrafo()
	for n := destino.FirstChild; n != nil; n = n.NextSibling {
		if n.Type == html.ElementNode && (n.DataAtom == atom.Ul || n.DataAtom == atom.Ol) {
			n.Attr = nil // la marca de las listas de Word, que solo servía para juntarlas
		}
	}
	var b strings.Builder
	for n := destino.FirstChild; n != nil; n = n.NextSibling {
		escribir(&b, n)
	}
	unico := destino.FirstChild
	enLinea := unico != nil && unico.NextSibling == nil && unico.DataAtom == atom.P && len(unico.Attr) == 0
	return Pegado{HTML: b.String(), EnLinea: enLinea}
}

// reductor lleva el párrafo que se está llenando con el texto suelto entre bloques.
type reductor struct {
	parrafo *html.Node
	destino *html.Node
}

func (r *reductor) cerrarParrafo() {
	if r.parrafo != nil && sinContenido(r.parrafo) {
		r.parrafo.Parent.RemoveChild(r.parrafo)
	}
	r.parrafo = nil
}

// parrafoSuelto es el párrafo donde va el texto suelto de `destino`, abierto si hace falta.
func (r *reductor) parrafoSuelto(destino *html.Node) *html.Node {
	if r.parrafo == nil || r.destino != destino {
		r.cerrarParrafo()
		r.parrafo, r.destino = elemento("p"), destino
		destino.AppendChild(r.parrafo)
	}
	return r.parrafo
}

// bloques lee los hijos de `fuente` como bloques y los escribe, reducidos, en `destino`.
// `enCelda` es que se está dentro de una celda: una tabla ahí no se edita y se aplana.
//
//nolint:gocyclo // una rama por clase de bloque
func (r *reductor) bloques(fuente, destino *html.Node, prof int, enCelda bool) {
	if prof > profundidadMax {
		return
	}
	for h := fuente.FirstChild; h != nil; h = h.NextSibling {
		switch {
		case h.Type == html.TextNode:
			if strings.TrimSpace(h.Data) == "" && (r.parrafo == nil || r.destino != destino) {
				continue // el salto de línea entre </p> y <p> no es un párrafo
			}
			r.enLinea(h, r.parrafoSuelto(destino), prof, false, true)
		case h.Type != html.ElementNode:
		case contenedores[h.DataAtom] || h.DataAtom == atom.Li && !enLista(h):
			r.cerrarParrafo()
			r.bloques(h, destino, prof+1, enCelda)
			r.cerrarParrafo()
		case h.DataAtom == atom.Table && !enCelda:
			r.cerrarParrafo()
			r.tabla(h, destino, prof)
		case h.DataAtom == atom.Hr:
			r.cerrarParrafo()
			destino.AppendChild(elemento("hr"))
		case h.DataAtom == atom.Ul || h.DataAtom == atom.Ol:
			r.cerrarParrafo()
			r.lista(h, destino, prof)
		case h.DataAtom == atom.Blockquote:
			r.cerrarParrafo()
			cita := conEstilo(elemento("blockquote"), h, true)
			destino.AppendChild(cita)
			r.bloques(h, cita, prof+1, enCelda)
			r.cerrarParrafo()
			if cita.FirstChild == nil {
				destino.RemoveChild(cita)
			}
		case h.DataAtom == atom.P && esDeListaDeWord(h):
			r.cerrarParrafo()
			r.elementoDeWord(h, destino, prof)
		case esTitulo(h) || h.DataAtom == atom.P || h.DataAtom == atom.Pre:
			r.cerrarParrafo()
			etiqueta := h.Data
			if h.DataAtom == atom.Pre {
				etiqueta = "p"
			}
			p := conEstilo(elemento(etiqueta), h, true)
			r.enLinea(h, p, prof+1, h.DataAtom == atom.Pre, false)
			if !sinContenido(p) {
				destino.AppendChild(p)
			}
		default: // en línea (o una tabla dentro de una celda, que se aplana en líneas)
			r.enLinea(h, r.parrafoSuelto(destino), prof, false, true)
		}
	}
}

// Las listas de Word no son `<ul>`: cada elemento es un `<p class=MsoListParagraph>` con
// `mso-list` en su estilo y la viñeta escrita a mano en un `<span style="mso-list:Ignore">`.
// Los elementos seguidos se juntan en una lista; numerada si la marca empieza por una
// cifra o una letra con su punto o su paréntesis.

func esDeListaDeWord(n *html.Node) bool {
	estilo := strings.ToLower(strings.ReplaceAll(atributo(n, "style"), " ", ""))
	return strings.Contains(estilo, "mso-list:") && !strings.Contains(estilo, "mso-list:none")
}

func esMarcaDeWord(n *html.Node) bool {
	return strings.Contains(strings.ToLower(strings.ReplaceAll(atributo(n, "style"), " ", "")), "mso-list:ignore")
}

// marcaDeWord es el texto de la viñeta de un elemento de lista de Word.
func marcaDeWord(n *html.Node, prof int) string {
	if prof > profundidadMax {
		return ""
	}
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		if h.Type == html.ElementNode && esMarcaDeWord(h) {
			return strings.TrimSpace(strings.ReplaceAll(textoDe(h), "\u00a0", " "))
		}
		if m := marcaDeWord(h, prof+1); m != "" {
			return m
		}
	}
	return ""
}

func (r *reductor) elementoDeWord(fuente, destino *html.Node, prof int) {
	etiqueta := "ul"
	if m := []rune(marcaDeWord(fuente, 0)); len(m) >= 2 && (m[len(m)-1] == '.' || m[len(m)-1] == ')') {
		etiqueta = "ol"
	}
	l := destino.LastChild
	if l == nil || l.Type != html.ElementNode || l.Data != etiqueta || atributo(l, "data-word") == "" {
		l = elemento(etiqueta, html.Attribute{Key: "data-word", Val: "1"})
		destino.AppendChild(l)
	}
	li := elemento("li")
	r.enLinea(fuente, li, prof+1, false, false)
	if !sinContenido(li) {
		l.AppendChild(li)
	}
	if l.FirstChild == nil {
		destino.RemoveChild(l)
	}
}

// lista copia una `<ul>`/`<ol>` con sus elementos. Lo suelto dentro de la lista (lo que no
// va en un `<li>`) se lleva al elemento de antes, o a uno nuevo.
func (r *reductor) lista(fuente, destino *html.Node, prof int) {
	if prof > profundidadMax {
		return
	}
	l := elemento(fuente.Data)
	var li *html.Node
	nuevo := func() *html.Node {
		li = elemento("li")
		l.AppendChild(li)
		return li
	}
	for h := fuente.FirstChild; h != nil; h = h.NextSibling {
		switch {
		case h.Type == html.ElementNode && h.DataAtom == atom.Li:
			r.elementoDeLista(h, nuevo(), prof+1)
		case h.Type == html.ElementNode && (h.DataAtom == atom.Ul || h.DataAtom == atom.Ol):
			if li == nil {
				nuevo()
			}
			r.lista(h, li, prof+1)
		case h.Type == html.TextNode && strings.TrimSpace(h.Data) == "":
		default:
			if li == nil {
				nuevo()
			}
			r.enLinea(h, li, prof+1, false, true)
		}
	}
	for h := l.FirstChild; h != nil; {
		sig := h.NextSibling
		if sinContenido(h) {
			l.RemoveChild(h)
		}
		h = sig
	}
	if l.FirstChild != nil {
		destino.AppendChild(l)
	}
}

// elementoDeLista llena un `<li>`: su texto en línea (los `<p>` de Word y de Google Docs
// dentro de cada elemento, desenvueltos) y sus listas anidadas.
func (r *reductor) elementoDeLista(fuente, li *html.Node, prof int) {
	for h := fuente.FirstChild; h != nil; h = h.NextSibling {
		if h.Type == html.ElementNode && (h.DataAtom == atom.Ul || h.DataAtom == atom.Ol) {
			r.lista(h, li, prof+1)
			continue
		}
		r.enLinea(h, li, prof+1, false, true)
	}
}

// tabla copia una tabla fila a fila: sus grupos de filas, sus filas y sus celdas con
// `colspan` y `rowspan`. Lo que va en cada celda se lee como bloques, sin tablas dentro.
func (r *reductor) tabla(fuente, destino *html.Node, prof int) {
	t := elemento("table")
	var cuerpoActual *html.Node
	var filas func(n, grupo *html.Node, prof int)
	filas = func(n, grupo *html.Node, prof int) {
		if prof > profundidadMax {
			return
		}
		for h := n.FirstChild; h != nil; h = h.NextSibling {
			if h.Type != html.ElementNode {
				continue
			}
			switch h.DataAtom {
			case atom.Thead, atom.Tbody, atom.Tfoot:
				g := elemento(h.Data)
				t.AppendChild(g)
				filas(h, g, prof+1)
			case atom.Tr:
				if grupo == nil {
					if cuerpoActual == nil {
						cuerpoActual = elemento("tbody")
						t.AppendChild(cuerpoActual)
					}
					grupo = cuerpoActual
				}
				grupo.AppendChild(r.fila(h, prof+1))
			case atom.Caption:
				// El título de la tabla, delante como un párrafo: dentro no se entiende.
				p := elemento("p")
				r.enLinea(h, p, prof+1, false, false)
				if !sinContenido(p) {
					destino.AppendChild(p)
				}
			default: // `<colgroup>` y lo demás: medidas de columna que no se pintan
			}
		}
	}
	filas(fuente, nil, prof+1)
	for g := t.FirstChild; g != nil; {
		sig := g.NextSibling
		if g.FirstChild == nil {
			t.RemoveChild(g)
		}
		g = sig
	}
	if t.FirstChild != nil {
		destino.AppendChild(t)
	}
}

func (r *reductor) fila(fuente *html.Node, prof int) *html.Node {
	tr := elemento("tr")
	for h := fuente.FirstChild; h != nil; h = h.NextSibling {
		if h.Type != html.ElementNode || (h.DataAtom != atom.Td && h.DataAtom != atom.Th) {
			continue
		}
		var attr []html.Attribute
		for _, k := range []string{"colspan", "rowspan"} {
			if v, err := strconv.Atoi(strings.TrimSpace(atributo(h, k))); err == nil && v > 1 && v <= 1000 {
				attr = append(attr, html.Attribute{Key: k, Val: strconv.Itoa(v)})
			}
		}
		celda := elemento(h.Data, attr...)
		r.cerrarParrafo()
		r.bloques(h, celda, prof+1, true)
		r.cerrarParrafo()
		if celda.FirstChild == nil {
			celda.AppendChild(elemento("br")) // la celda vacía de la web: `<td><br></td>`
		}
		tr.AppendChild(celda)
	}
	if tr.FirstChild == nil {
		tr.AppendChild(func() *html.Node { c := elemento("td"); c.AppendChild(elemento("br")); return c }())
	}
	return tr
}

// enLinea copia `fuente` (o sus hijos, si `hijos`) como contenido en línea de `destino`.
// Lo que en la fuente era un bloque dentro de la línea (un `<p>` en un `<li>`, una fila
// de una tabla dentro de una celda) se desenvuelve con un salto delante.
//
//nolint:gocyclo,gocognit // una rama por elemento en línea
func (r *reductor) enLinea(fuente, destino *html.Node, prof int, pre, solo bool) {
	if prof > profundidadMax {
		return
	}
	copiar := func(h *html.Node) {
		switch {
		case h.Type == html.TextNode:
			r.texto(h.Data, destino, pre)
		case h.Type != html.ElementNode:
		case h.DataAtom == atom.Br:
			destino.AppendChild(elemento("br"))
		case h.DataAtom == atom.Img:
			if img := imagenPegada(h); img != nil {
				destino.AppendChild(img)
			}
		case formatoEnLinea[h.DataAtom] != "":
			r.envolver(h, conEstilo(elemento(formatoEnLinea[h.DataAtom]), h, false), destino, prof, pre)
		case h.DataAtom == atom.A:
			if href := strings.TrimSpace(atributo(h, "href")); href != "" {
				r.envolver(h, elemento("a", html.Attribute{Key: "href", Val: href}), destino, prof, pre)
			} else {
				r.enLinea(h, destino, prof+1, pre, false)
			}
		case h.DataAtom == atom.Span && esMarcaDeWord(h):
			// La viñeta o el número que Word escribe delante de cada elemento de lista: la
			// lista ya la pinta el documento.
		case h.DataAtom == atom.Span:
			r.envolver(h, conEstilo(elemento("span"), h, false), destino, prof, pre)
		case h.DataAtom == atom.Font:
			if c := atributo(h, "color"); c != "" {
				if _, ok := colorCSS(c); ok {
					r.envolver(h, elemento("font", html.Attribute{Key: "color", Val: c}), destino, prof, pre)
					return
				}
			}
			r.enLinea(h, destino, prof+1, pre, false)
		case bloqueEnLinea[h.DataAtom] || contenedores[h.DataAtom] || h.DataAtom == atom.Td || h.DataAtom == atom.Th:
			// Un bloque dentro de la línea: en su propio renglón, salvo una celda, que va
			// detrás de la de antes con un espacio.
			if !sinContenido(destino) {
				if h.DataAtom == atom.Td || h.DataAtom == atom.Th {
					r.texto(" ", destino, false)
				} else if !acabaEnSalto(destino) && h.DataAtom != atom.Table && h.DataAtom != atom.Ul && h.DataAtom != atom.Ol {
					destino.AppendChild(elemento("br"))
				}
			}
			if h.DataAtom == atom.Hr {
				return
			}
			r.enLinea(h, destino, prof+1, pre || h.DataAtom == atom.Pre, false)
		default: // `<code>`, `<small>`, `<mark>`, `<abbr>`…: se queda su texto
			r.enLinea(h, destino, prof+1, pre, false)
		}
	}
	if solo {
		copiar(fuente)
		return
	}
	for h := fuente.FirstChild; h != nil; h = h.NextSibling {
		copiar(h)
	}
}

// envolver pone `nuevo` en `destino` con el contenido de `fuente`; si no le queda nada que
// decir (un `<span>` sin estilo), solo el contenido.
func (r *reductor) envolver(fuente, nuevo, destino *html.Node, prof int, pre bool) {
	if nuevo.DataAtom == atom.Span && len(nuevo.Attr) == 0 {
		r.enLinea(fuente, destino, prof+1, pre, false)
		return
	}
	r.enLinea(fuente, nuevo, prof+1, pre, false)
	if nuevo.FirstChild != nil {
		destino.AppendChild(nuevo)
	}
}

// texto añade texto. En un `<pre>` cada salto de línea es un `<br>`: el párrafo en el que
// se convierte no conserva los blancos.
func (r *reductor) texto(s string, destino *html.Node, pre bool) {
	if !pre {
		destino.AppendChild(&html.Node{Type: html.TextNode, Data: s})
		return
	}
	for i, linea := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if i > 0 {
			destino.AppendChild(elemento("br"))
		}
		if linea != "" {
			destino.AppendChild(&html.Node{Type: html.TextNode, Data: linea})
		}
	}
}

// imagenPegada es el `<img>` que entra de uno pegado: solo con un `data:` de mapa de bits,
// su texto alternativo y sus medidas.
func imagenPegada(n *html.Node) *html.Node {
	src := strings.TrimSpace(atributo(n, "src"))
	bajo := strings.ToLower(src)
	admitida := false
	for _, p := range tiposDeImagenPegada {
		admitida = admitida || strings.HasPrefix(bajo, p) && strings.Contains(bajo, ";base64,")
	}
	if !admitida {
		return nil
	}
	img := elemento("img", html.Attribute{Key: "src", Val: src})
	if alt := atributo(n, "alt"); alt != "" {
		img.Attr = append(img.Attr, html.Attribute{Key: "alt", Val: alt})
	}
	for _, k := range []string{"width", "height"} {
		if v, err := strconv.Atoi(strings.TrimSpace(atributo(n, k))); err == nil && v > 0 && v <= 10000 {
			img.Attr = append(img.Attr, html.Attribute{Key: k, Val: strconv.Itoa(v)})
		}
	}
	return img
}

// conEstilo le pone a `nuevo` las declaraciones del `style` de `fuente` que se entienden y
// se pintan; las demás (`mso-*`, márgenes, lo que solo se ignora) se quedan fuera.
func conEstilo(nuevo, fuente *html.Node, bloque bool) *html.Node {
	var quedan []string
	for decl := range strings.SplitSeq(atributo(fuente, "style"), ";") {
		prop, valor, ok := strings.Cut(decl, ":")
		prop, valor = strings.ToLower(strings.TrimSpace(prop)), strings.TrimSpace(valor)
		if !ok || prop == "" || valor == "" {
			continue
		}
		prueba := estiloBase
		if len(prueba.declarar(prop+": "+valor, bloque)) == 0 {
			quedan = append(quedan, prop+": "+valor)
		}
	}
	if len(quedan) > 0 {
		nuevo.Attr = append(nuevo.Attr, html.Attribute{Key: "style", Val: strings.Join(quedan, "; ")})
	}
	return nuevo
}

func esTitulo(n *html.Node) bool {
	switch n.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		return true
	default:
		return false
	}
}

func enLista(n *html.Node) bool {
	return n.Parent != nil && (n.Parent.DataAtom == atom.Ul || n.Parent.DataAtom == atom.Ol)
}

// sinContenido dice si n no tiene nada que se vea: ni texto, ni imagen, ni regla.
func sinContenido(n *html.Node) bool {
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		switch {
		case h.Type == html.TextNode && strings.TrimSpace(strings.ReplaceAll(h.Data, "\u00a0", "x")) != "":
			return false
		case h.Type == html.ElementNode && (h.DataAtom == atom.Img || h.DataAtom == atom.Hr || h.DataAtom == atom.Table):
			return false
		case h.Type == html.ElementNode && !sinContenido(h):
			return false
		}
	}
	return true
}

func acabaEnSalto(n *html.Node) bool {
	u := n.LastChild
	return u != nil && u.Type == html.ElementNode && u.DataAtom == atom.Br
}
