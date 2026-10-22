// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Las NOTAS AL FINAL de «Referencias» (`endnotes.ts` e `insertarNotaAlFinal` de
// Referencias.tsx), con el mismo HTML: el marcador `<sup data-endnote-ref>` con su número
// romano, y la nota en la `<section data-endnotes>` del final, con el enlace de vuelta al
// texto delante —que no se edita— y la lista sin viñetas.

// estiloListaNotasFinal es el `ENDNOTES_SHELL`: la lista sin viñetas, sin sangría y con un
// cuarto de em arriba y abajo.
const estiloListaNotasFinal = "list-style:none;padding-left:0;margin:.25em 0"

// Romano es `romanNumeral`: en minúscula, de 1 en adelante.
func Romano(n int) string {
	n = max(n, 1)
	var b strings.Builder
	for _, s := range []struct {
		v int
		s string
	}{{1000, "m"}, {900, "cm"}, {500, "d"}, {400, "cd"}, {100, "c"}, {90, "xc"}, {50, "l"}, {40, "xl"}, {10, "x"}, {9, "ix"}, {5, "v"}, {4, "iv"}, {1, "i"}} {
		for n >= s.v {
			b.WriteString(s.s)
			n -= s.v
		}
	}
	return b.String()
}

// esSeccionFinal dice si n es la `<section data-endnotes>`.
func esSeccionFinal(n *xhtml.Node) bool {
	return n != nil && n.DataAtom == atom.Section && tieneAtributo(n, "data-endnotes")
}

// esVuelta dice si n es el enlace de vuelta de una nota al final: no se edita, se salta.
func esVuelta(n *xhtml.Node) bool {
	return n != nil && n.DataAtom == atom.A && n.Namespace == "" && tieneAtributo(n, "data-endnote-back")
}

func (d *Documento) seccionFinal() *xhtml.Node {
	for h := d.raiz.FirstChild; h != nil; h = h.NextSibling {
		if esSeccionFinal(h) {
			return h
		}
	}
	return nil
}

// marcadoresFinal son los `<sup data-endnote-ref>` en orden de documento.
func (d *Documento) marcadoresFinal() []*xhtml.Node {
	var out []*xhtml.Node
	var recorrer func(n *xhtml.Node, prof int)
	recorrer = func(n *xhtml.Node, prof int) {
		for h := n.FirstChild; h != nil && prof <= profundidadMax; h = h.NextSibling {
			if h.DataAtom == atom.Sup && tieneAtributo(h, "data-endnote-ref") {
				out = append(out, h)
				continue
			}
			recorrer(h, prof+1)
		}
	}
	recorrer(d.raiz, 0)
	return out
}

// InsertarNotaFinal es «Insertar nota al final»: el marcador en el cursor —o al final del
// cuerpo, si el cursor está en las notas— y la nota al final con el texto provisional; y
// lo renumera todo en orden de documento. `vuelta` es el `title` del enlace de vuelta, ya
// traducido. Devuelve el lugar de la nota nueva, para escribirla.
func (d *Documento) InsertarNotaFinal(bloques []Bloque, i, pos int, provisional, vuelta string) (Lugar, bool) {
	if i, pos = alCuerpo(bloques, i, pos); i < 0 {
		return Lugar{}, false
	}
	usados := d.idsUsados()
	n := len(d.marcadoresFinal())
	for n++; usados["en-"+strconv.Itoa(n)] || usados["enref-"+strconv.Itoa(n)]; n++ {
	}
	num := strconv.Itoa(n)
	sup := elemento("sup", xhtml.Attribute{Key: "data-endnote-ref", Val: num}, xhtml.Attribute{Key: "id", Val: "enref-" + num})
	a := elemento("a", xhtml.Attribute{Key: "href", Val: "#en-" + num})
	bloques[i].Editar().marcador(pos, "i", sup, a)
	d.RenumerarNotasFinal(provisional, vuelta)
	final := strings.TrimPrefix(atributo(sup, "id"), "enref-")
	for _, b := range d.Bloques() {
		if b.cubre() && b.cont.DataAtom == atom.Li && atributo(b.cont, "id") == "en-"+final {
			return b.Lugar, true
		}
	}
	return Lugar{}, false
}

// alCuerpo es dónde va un marcador de nota: en (i, pos) si es texto del cuerpo, y si no
// —en una sección generada o en una región— al final del último bloque de texto del cuerpo
// (`caretAlFinalDelCuerpo`). -1 si no hay ninguno.
func alCuerpo(bloques []Bloque, i, pos int) (int, int) {
	delCuerpo := func(b Bloque) bool { return b.Tipo.EsTexto() && b.Region == "" && !enSeccion(b.cont) }
	if i >= 0 && i < len(bloques) && delCuerpo(bloques[i]) {
		return i, pos
	}
	for k := len(bloques) - 1; k >= 0; k-- {
		if delCuerpo(bloques[k]) {
			return k, len([]rune(bloques[k].Texto()))
		}
	}
	return -1, 0
}

// RenumerarNotasFinal es `renumberEndnotesDom`: numera los marcadores en orden de documento
// (1…N, en romano) y rehace la lista de notas con lo que ya decía cada una. Una nota sin
// marcador se va; sin marcadores, se va la sección. Dice si había algo que hacer.
func (d *Documento) RenumerarNotasFinal(provisional, vuelta string) bool {
	marcas := d.marcadoresFinal()
	seccion := d.seccionFinal()
	contenido := map[string][]*xhtml.Node{}
	if seccion != nil {
		for _, li := range elementosDe(seccion, atom.Li) {
			if id := atributo(li, "id"); strings.HasPrefix(id, "en-") {
				contenido[id[3:]] = contenidoNotaFinal(li)
			}
		}
	}
	if len(marcas) == 0 {
		if seccion == nil {
			return false
		}
		seccion.Parent.RemoveChild(seccion)
		return true
	}
	var plan [][]*xhtml.Node
	for k, m := range marcas {
		plan = append(plan, contenido[atributo(m, "data-endnote-ref")])
		seq := strconv.Itoa(k + 1)
		ponerAtributo(m, "data-endnote-ref", seq)
		ponerAtributo(m, "id", "enref-"+seq)
		if a := primerHijo(m, atom.A); a != nil {
			ponerAtributo(a, "href", "#en-"+seq)
			vaciar(a)
			a.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: Romano(k + 1)})
		}
	}
	if seccion == nil {
		seccion = elemento("section", xhtml.Attribute{Key: "data-endnotes"})
		d.raiz.AppendChild(seccion)
	}
	ol := primerHijo(seccion, atom.Ol)
	if ol == nil {
		vaciar(seccion)
		seccion.AppendChild(elemento("hr"))
		ol = elemento("ol", xhtml.Attribute{Key: "style", Val: estiloListaNotasFinal})
		seccion.AppendChild(ol)
	}
	vaciar(ol)
	for k, c := range plan {
		seq := strconv.Itoa(k + 1)
		li := elemento("li", xhtml.Attribute{Key: "id", Val: "en-" + seq})
		back := elemento("a", xhtml.Attribute{Key: "href", Val: "#enref-" + seq}, xhtml.Attribute{Key: "data-endnote-back"},
			xhtml.Attribute{Key: "contenteditable", Val: "false"}, xhtml.Attribute{Key: "title", Val: vuelta})
		back.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: Romano(k+1) + "."})
		li.AppendChild(back)
		li.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: " "})
		if len(c) == 0 {
			c = []*xhtml.Node{{Type: xhtml.TextNode, Data: provisional}}
		}
		for _, n := range c {
			li.AppendChild(n)
		}
		fundirTextos(li)
		ol.AppendChild(li)
	}
	return true
}

// contenidoNotaFinal es `contenidoNota`: lo que dice la nota sin su enlace de vuelta, sin
// los blancos de los extremos (`innerHTML.trim()`). Son los nodos, sacados de la nota.
func contenidoNotaFinal(li *xhtml.Node) []*xhtml.Node {
	var out []*xhtml.Node
	for h := li.FirstChild; h != nil; h = li.FirstChild {
		li.RemoveChild(h)
		if !esVuelta(h) {
			out = append(out, h)
		}
	}
	for len(out) > 0 && out[0].Type == xhtml.TextNode {
		if out[0].Data = strings.TrimLeft(out[0].Data, " \t\n\r\f"); out[0].Data != "" {
			break
		}
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1].Type == xhtml.TextNode {
		u := out[len(out)-1]
		if u.Data = strings.TrimRight(u.Data, " \t\n\r\f"); u.Data != "" {
			break
		}
		out = out[:len(out)-1]
	}
	return out
}

// idsUsados son todos los `id` del documento.
func (d *Documento) idsUsados() map[string]bool {
	usados := map[string]bool{}
	var recorrer func(n *xhtml.Node, prof int)
	recorrer = func(n *xhtml.Node, prof int) {
		for h := n.FirstChild; h != nil && prof <= profundidadMax; h = h.NextSibling {
			if id := atributo(h, "id"); id != "" {
				usados[id] = true
			}
			recorrer(h, prof+1)
		}
	}
	recorrer(d.raiz, 0)
	return usados
}

func elementosDe(n *xhtml.Node, a atom.Atom) []*xhtml.Node {
	var out []*xhtml.Node
	var recorrer func(n *xhtml.Node, prof int)
	recorrer = func(n *xhtml.Node, prof int) {
		for h := n.FirstChild; h != nil && prof <= profundidadMax; h = h.NextSibling {
			if h.DataAtom == a {
				out = append(out, h)
			}
			recorrer(h, prof+1)
		}
	}
	recorrer(n, 0)
	return out
}

func primerHijo(n *xhtml.Node, a atom.Atom) *xhtml.Node {
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		if h.DataAtom == a {
			return h
		}
	}
	return nil
}

func vaciar(n *xhtml.Node) {
	for h := n.FirstChild; h != nil; h = n.FirstChild {
		n.RemoveChild(h)
	}
}

// marcador escribe en `pos` un marcador de nota: `texto` dentro de `sup > a`, fuera de
// cualquier otro enlace o marcador.
func (e *Edicion) marcador(pos int, texto string, sup, a *xhtml.Node) {
	pos = max(0, min(pos, len(e.runas)))
	cadena := append(fueraDeMarcador(e.hereda(pos)), sup, a)
	runas := []rune(texto)
	cadenas := make([][]*xhtml.Node, 0, len(e.cadenas)+len(runas))
	cadenas = append(cadenas, e.cadenas[:pos]...)
	for range runas {
		cadenas = append(cadenas, cadena)
	}
	e.cadenas = append(cadenas, e.cadenas[pos:]...)
	e.runas = append(append(append([]rune{}, e.runas[:pos]...), runas...), e.runas[pos:]...)
	e.hayPendiente = false
	e.reescribir()
}
