// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"html"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Lo que escribe la pestaña «Referencias» de la web (`ribbon/tabs/Referencias.tsx` y
// `insertToc` de canvas/exec.ts), con el MISMO HTML: el índice de títulos y las notas al
// pie.

// sangriaIndice es la de cada nivel del índice: h1 = 0 · h2 = 1,5 em · h3 = 3 em.
const sangriaIndice = 1.5

// entradasIndice son los <li> del índice a partir de los h1–h3 del documento: a cada
// título sin id le pone uno estable, `h-<n>`, saltando los ya ocupados, y enlaza a él
// (`itemsTdc`).
func (d *Documento) entradasIndice() string {
	usados := map[string]bool{}
	var titulos []*xhtml.Node
	var recorrer func(n *xhtml.Node, prof int)
	recorrer = func(n *xhtml.Node, prof int) {
		for h := n.FirstChild; h != nil && prof <= profundidadMax; h = h.NextSibling {
			if h.Type != xhtml.ElementNode {
				continue
			}
			if id := atributo(h, "id"); id != "" {
				usados[id] = true
			}
			if h.DataAtom == atom.H1 || h.DataAtom == atom.H2 || h.DataAtom == atom.H3 {
				titulos = append(titulos, h)
			}
			if h.DataAtom != atom.Nav || !tieneAtributo(h, "data-toc") {
				recorrer(h, prof+1)
			}
		}
	}
	recorrer(d.raiz, 0)
	var b strings.Builder
	n := 0
	for _, t := range titulos {
		id := atributo(t, "id")
		if id == "" {
			for n++; usados["h-"+strconv.Itoa(n)]; n++ {
			}
			id = "h-" + strconv.Itoa(n)
			usados[id] = true
			t.Attr = append(t.Attr, xhtml.Attribute{Key: "id", Val: id})
		}
		nivel := int(t.Data[1] - '0')
		sangria := strconv.FormatFloat(float64(nivel-1)*sangriaIndice, 'f', -1, 64)
		b.WriteString(`<li data-level="` + strconv.Itoa(nivel) + `" style="margin-left:` + sangria + `em">` +
			`<a href="#` + html.EscapeString(id) + `">` + html.EscapeString(textoDe(t)) + `</a></li>`)
	}
	return b.String()
}

// textoDe es el `textContent` de un nodo.
func textoDe(n *xhtml.Node) string {
	var b strings.Builder
	var ir func(n *xhtml.Node, prof int)
	ir = func(n *xhtml.Node, prof int) {
		for h := n.FirstChild; h != nil && prof <= profundidadMax; h = h.NextSibling {
			if h.Type == xhtml.TextNode {
				b.WriteString(h.Data)
			}
			ir(h, prof+1)
		}
	}
	ir(n, 0)
	return b.String()
}

// htmlIndice es el `<nav data-toc>` entero, con su título ya traducido.
func (d *Documento) htmlIndice(titulo string) string {
	return `<nav data-toc><p><b>` + html.EscapeString(titulo) + `</b></p><ol>` + d.entradasIndice() + `</ol></nav>`
}

// indice es el `<nav data-toc>` del documento, o nil.
func (d *Documento) indice() *xhtml.Node {
	var hallado *xhtml.Node
	var ir func(n *xhtml.Node, prof int)
	ir = func(n *xhtml.Node, prof int) {
		for h := n.FirstChild; h != nil && hallado == nil && prof <= profundidadMax; h = h.NextSibling {
			if h.DataAtom == atom.Nav && tieneAtributo(h, "data-toc") {
				hallado = h
				return
			}
			ir(h, prof+1)
		}
	}
	ir(d.raiz, 0)
	return hallado
}

// InsertarIndice es «Tabla de contenidos»: pone un índice nuevo donde está el cursor y
// quita el que hubiera (`insertToc`), con las anclas y la sangría de «Actualizar». Con el
// cursor dentro del índice, lo rehace en su sitio.
func (d *Documento) InsertarIndice(bloques []Bloque, i, pos int, titulo string) (Lugar, bool) {
	viejo := d.indice()
	if viejo != nil && i >= 0 && i < len(bloques) && bloques[i].Recuadro != nil {
		if !d.ActualizarIndice(titulo) {
			return Lugar{}, false
		}
		return d.primeroDesde(d.indice())
	}
	l, ok := d.InsertarHTML(bloques, i, pos, d.htmlIndice(titulo))
	if viejo != nil && viejo.Parent != nil {
		viejo.Parent.RemoveChild(viejo)
	}
	return l, ok
}

// ActualizarIndice es «Actualizar tabla»: rehace el índice donde está, y dice si había
// uno. Sin índice no hace nada: la cinta genera uno.
func (d *Documento) ActualizarIndice(titulo string) bool {
	nav := d.indice()
	if nav == nil {
		return false
	}
	nodos, err := xhtml.ParseFragment(strings.NewReader(d.htmlIndice(titulo)), cuerpo())
	if err != nil || len(nodos) == 0 {
		return false
	}
	nav.Parent.InsertBefore(nodos[0], nav)
	nav.Parent.RemoveChild(nav)
	return true
}

// ---- notas al pie ----

// seccionNotas es la `<section data-footnotes>` del final del documento, o nil.
func (d *Documento) seccionNotas() *xhtml.Node {
	for h := d.raiz.FirstChild; h != nil; h = h.NextSibling {
		if h.DataAtom == atom.Section && tieneAtributo(h, "data-footnotes") {
			return h
		}
	}
	return nil
}

// InsertarNotaPie es «Nota al pie» (`insertarNotaAlPie`): el marcador
// `<sup data-footnote-ref id="fnref-N"><a href="#fn-N">N</a></sup>` en el cursor y la nota
// `<li id="fn-N" value="N">` en la sección de notas del final, que se crea si falta. Un
// cursor dentro de las propias notas pone el marcador al final del cuerpo. Devuelve el
// lugar de la nota, para escribirla.
func (d *Documento) InsertarNotaPie(bloques []Bloque, i, pos int, provisional string) (Lugar, bool) {
	notas := d.seccionNotas()
	num := strconv.Itoa(d.siguienteNota())
	enNotas := func(b Bloque) bool {
		for p := b.cont; p != nil && notas != nil; p = p.Parent {
			if p == notas {
				return true
			}
		}
		return false
	}
	if i < 0 || i >= len(bloques) || !bloques[i].Tipo.EsTexto() || enNotas(bloques[i]) {
		// Al final del cuerpo: el último bloque de texto antes de las notas.
		i = -1
		for k := len(bloques) - 1; k >= 0 && i < 0; k-- {
			if bloques[k].Tipo.EsTexto() && !enNotas(bloques[k]) {
				i, pos = k, len([]rune(bloques[k].Texto()))
			}
		}
	}
	if i < 0 {
		return Lugar{}, false
	}
	bloques[i].Editar().marcadorNota(pos, num)
	li := elemento("li", xhtml.Attribute{Key: "id", Val: "fn-" + num}, xhtml.Attribute{Key: "value", Val: num})
	li.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: provisional})
	d.listaNotas(notas).AppendChild(li)
	return Lugar{cont: li, primero: li.FirstChild, ultimo: li.LastChild}, true
}

// siguienteNota es el número de la nota nueva: los marcadores que hay más uno, saltando
// los ids que aún ocupa una nota borrada a medias.
func (d *Documento) siguienteNota() int {
	usados := map[string]bool{}
	marcas := 0
	var recorrer func(n *xhtml.Node, prof int)
	recorrer = func(n *xhtml.Node, prof int) {
		for h := n.FirstChild; h != nil && prof <= profundidadMax; h = h.NextSibling {
			if id := atributo(h, "id"); id != "" {
				usados[id] = true
			}
			if h.DataAtom == atom.Sup && tieneAtributo(h, "data-footnote-ref") {
				marcas++
			}
			recorrer(h, prof+1)
		}
	}
	recorrer(d.raiz, 0)
	n := marcas + 1
	for usados["fn-"+strconv.Itoa(n)] || usados["fnref-"+strconv.Itoa(n)] {
		n++
	}
	return n
}

// marcadorNota escribe en `pos` el marcador de la nota `num`.
func (e *Edicion) marcadorNota(pos int, num string) {
	sup := elemento("sup", xhtml.Attribute{Key: "data-footnote-ref", Val: num}, xhtml.Attribute{Key: "id", Val: "fnref-" + num})
	e.marcador(pos, num, sup, elemento("a", xhtml.Attribute{Key: "href", Val: "#fn-" + num}))
}

// listaNotas es el <ol> de la sección de notas, que se crea (o se rehace, si llega sin
// lista) al final del documento.
func (d *Documento) listaNotas(notas *xhtml.Node) *xhtml.Node {
	if notas != nil {
		for h := notas.FirstChild; h != nil; h = h.NextSibling {
			if h.DataAtom == atom.Ol {
				return h
			}
		}
		notas.Parent.RemoveChild(notas) // sección sin <ol>: se rehace
	}
	notas = elemento("section", xhtml.Attribute{Key: "data-footnotes", Val: ""})
	notas.AppendChild(elemento("hr"))
	lista := elemento("ol")
	notas.AppendChild(lista)
	d.raiz.AppendChild(notas)
	return lista
}

// fueraDeMarcador es la cadena sin enlaces ni marcadores de nota: un marcador nuevo no
// va dentro de otro enlace ni de otro marcador, aunque se escriba justo detrás.
func fueraDeMarcador(c []*xhtml.Node) []*xhtml.Node {
	out := make([]*xhtml.Node, 0, len(c))
	for _, n := range c {
		if n.DataAtom != atom.A && (n.DataAtom != atom.Sup || !tieneAtributo(n, "data-footnote-ref") && !tieneAtributo(n, "data-endnote-ref")) {
			out = append(out, n)
		}
	}
	return out
}
