// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Lo que escribe la pestaña «Insertar» de la web (`ribbon/tabs/Insertar.tsx` y
// `canvas/exec.ts`), con el MISMO HTML: una tabla, un salto de página, una regla, una
// imagen o un enlace, y las órdenes de tabla. Todo cambia el árbol en el sitio, como el
// resto del paquete: lo de alrededor no se toca.

// HTMLTabla es la tabla de «Insertar tabla»: `filas` × `cols` celdas `<br>`, la primera
// fila en <th> si `cabecera`, y el párrafo de escape detrás (`tableHtml` de Insertar.tsx).
func HTMLTabla(filas, cols int, cabecera bool) string {
	fila := func(etiqueta string) string {
		s := "<tr>"
		for range cols {
			s += "<" + etiqueta + "><br></" + etiqueta + ">"
		}
		return s + "</tr>"
	}
	cuerpo := ""
	for f := range filas {
		if f == 0 && cabecera {
			cuerpo += fila("th")
		} else {
			cuerpo += fila("td")
		}
	}
	return "<table><tbody>" + cuerpo + "</tbody></table><p><br></p>"
}

// HTMLSaltoPagina es el `PAGE_BREAK_HTML` de Insertar.tsx.
const HTMLSaltoPagina = `<div data-page-break="" contenteditable="false" ` +
	`style="page-break-after:always;break-after:page;border-top:2px dashed #cbd5e1;margin:1.5em 0"></div>` +
	`<p><br></p>`

// HTMLRegla es lo que deja `insertHorizontalRule`.
const HTMLRegla = `<hr>`

// InsertarHTML mete un trozo de HTML de BLOQUE en la posición `pos` del bloque i, como el
// `insertHTML` de la web: en un párrafo vacío lo sustituye; al principio va delante, al
// final detrás y en medio lo parte. En una celda va detrás de la tabla —una tabla dentro
// de otra no se edita—, y en una lista o una línea suelta, detrás de ella entera. Devuelve
// el lugar del primer bloque de texto de lo insertado, o del de detrás, para el cursor.
//
// ponytail: Chromium parte la lista en dos si se inserta en medio; aquí va detrás.
func (d *Documento) InsertarHTML(bloques []Bloque, i, pos int, fragmento string) (Lugar, bool) {
	nodos, err := html.ParseFragment(strings.NewReader(fragmento), cuerpo())
	if err != nil || len(nodos) == 0 {
		return Lugar{}, false
	}
	padre, antes := d.raiz, (*html.Node)(nil)
	var sobra *html.Node
	if i >= 0 && i < len(bloques) {
		b := bloques[i]
		e := b.Editar()
		arriba := superiorDe(b.cont)
		switch {
		case b.Celda != nil:
			arriba = b.Celda.Tabla.nodo
			antes = arriba.NextSibling
		case b.cont == nil || b.cont.DataAtom == atom.Body:
			arriba, antes = nil, b.ultimo.NextSibling
		case arriba != b.cont || !e.cubre():
			antes = arriba.NextSibling
		case len(e.runas) == 0:
			antes, sobra = arriba.NextSibling, arriba
		case pos <= 0:
			antes = arriba
		case pos >= len(e.runas):
			antes = arriba.NextSibling
		default:
			antes = arriba.NextSibling
			if l, ok := e.Partir(pos); ok {
				antes = l.cont
			}
		}
		if arriba != nil && arriba.Parent != nil {
			padre = arriba.Parent
		}
	}
	for _, n := range nodos {
		padre.InsertBefore(n, antes)
	}
	if sobra != nil {
		sobra.Parent.RemoveChild(sobra)
	}
	return d.primeroDesde(nodos[0])
}

func superiorDe(n *html.Node) *html.Node {
	for n != nil && n.Parent != nil && n.Parent.DataAtom != atom.Body {
		n = n.Parent
	}
	return n
}

// primeroDesde es el lugar del primer bloque de texto que empieza en n —en él o dentro— o
// después, en orden de documento.
func (d *Documento) primeroDesde(n *html.Node) (Lugar, bool) {
	bloques := d.Bloques()
	ancla := make(map[*html.Node]int, len(bloques))
	for k := len(bloques) - 1; k >= 0; k-- {
		b := bloques[k]
		a := b.cont
		if a == nil || a.DataAtom == atom.Body || (!b.cubre() && b.primero != nil) {
			a = b.primero
		}
		ancla[a] = k
	}
	var hallado *Bloque
	var recorrer func(n *html.Node) bool
	recorrer = func(n *html.Node) bool {
		if k, ok := ancla[n]; ok && bloques[k].Tipo.EsTexto() {
			hallado = &bloques[k]
			return true
		}
		for h := n.FirstChild; h != nil; h = h.NextSibling {
			if recorrer(h) {
				return true
			}
		}
		return false
	}
	for h := n; h != nil; {
		if recorrer(h) {
			return hallado.Lugar, true
		}
		for h.NextSibling == nil { // lo siguiente en orden de documento tras el subárbol de h
			if h = h.Parent; h == nil || h == d.raiz {
				return Lugar{}, false
			}
		}
		h = h.NextSibling
	}
	return Lugar{}, false
}

// EsTexto dice si en un bloque de este tipo se escribe: una regla, un salto de página y
// una figura no.
func (t Tipo) EsTexto() bool { return t != Regla && t != SaltoPagina && t != Figura }

// InsertarImagen mete un <img src> en `pos`: lo que deja `insertImage` de la web, sin más
// atributos (la web los pone cuando se toca la imagen).
func (e *Edicion) InsertarImagen(pos int, src string) {
	img := elemento("img", html.Attribute{Key: "src", Val: src})
	pos = max(0, min(pos, len(e.runas)))
	cadena := append(slices.Clip(e.hereda(pos)), img)
	e.runas = slices.Insert(e.runas, pos, ObjetoRuna)
	e.cadenas = slices.Insert(e.cadenas, pos, cadena)
	e.hayPendiente = false
	e.reescribir()
}

// InsertarEnlace sustituye [desde, hasta) por `texto` enlazado a `href`: el `<a href>` de
// EnlacePanel. Un enlace no va dentro de otro: lo de alrededor pierde el suyo.
func (e *Edicion) InsertarEnlace(desde, hasta int, href, texto string) {
	desde, hasta = max(0, min(desde, len(e.runas))), max(0, min(hasta, len(e.runas)))
	desde, hasta = min(desde, hasta), max(desde, hasta)
	a := elemento("a", html.Attribute{Key: "href", Val: href})
	cadena := slices.DeleteFunc(slices.Clone(e.hereda(desde)), func(n *html.Node) bool { return n.DataAtom == atom.A })
	cadena = append(cadena, a)
	nuevo := limpiar(texto)
	cadenas := slices.Clone(e.cadenas[:desde])
	for range nuevo {
		cadenas = append(cadenas, cadena)
	}
	e.cadenas = append(cadenas, e.cadenas[hasta:]...)
	e.runas = slices.Concat(e.runas[:desde], nuevo, e.runas[hasta:])
	e.hayPendiente = false
	e.reescribir()
}

// ---- las órdenes de tabla (`tableOp`, `mergeCells`, `splitCell`, `setCellProp`) ----

// OrdenTabla es una de las órdenes de fila, columna o tabla de la cinta.
type OrdenTabla uint8

// Las de `TableCommand` de canvas/exec.ts.
const (
	FilaEncima OrdenTabla = iota
	FilaDebajo
	ColIzquierda
	ColDerecha
	BorrarFila
	BorrarColumna
	BorrarTabla
)

// Tabla aplica una orden a la tabla de la celda `c`, como `tableOp` de la web, y devuelve
// dónde queda el cursor.
func (d *Documento) Tabla(c *Celda, o OrdenTabla) (Lugar, bool) {
	if c == nil || c.nodo.Parent == nil {
		return Lugar{}, false
	}
	td, tr, tabla := c.nodo, c.nodo.Parent, c.Tabla.nodo
	switch o {
	case FilaEncima, FilaDebajo:
		nueva := elemento("tr")
		for range celdasDe(tr) {
			nueva.AppendChild(celdaVaciaNueva("td"))
		}
		if o == FilaEncima {
			tr.Parent.InsertBefore(nueva, tr)
		} else {
			tr.Parent.InsertBefore(nueva, tr.NextSibling)
		}
		return d.primeroDesde(nueva.FirstChild)
	case ColIzquierda, ColDerecha:
		en := slices.Index(celdasDe(tr), td)
		if o == ColDerecha {
			en++
		}
		for _, f := range filasDe(tabla) {
			cs := celdasDe(f)
			var ref *html.Node
			if en < len(cs) {
				ref = cs[en]
			}
			f.InsertBefore(celdaVaciaNueva("td"), ref)
		}
		return d.primeroDesde(td)
	case BorrarFila:
		return d.borrarFila(tabla, tr)
	case BorrarColumna:
		return d.borrarColumna(tabla, tr, td)
	case BorrarTabla:
		return d.quitarTabla(tabla), true
	}
	return Lugar{}, false
}

// borrarFila quita la fila; la última se lleva la tabla, como en la web.
func (d *Documento) borrarFila(tabla, tr *html.Node) (Lugar, bool) {
	if len(filasDe(tabla)) <= 1 {
		return d.quitarTabla(tabla), true
	}
	vecina := siguienteElemento(tr)
	if vecina == nil {
		vecina = anteriorElemento(tr)
	}
	tr.Parent.RemoveChild(tr)
	if vecina != nil && vecina.DataAtom == atom.Tr && len(celdasDe(vecina)) > 0 {
		return d.primeroDesde(celdasDe(vecina)[0])
	}
	return d.primeroDesde(tabla)
}

// borrarColumna quita la columna de `td` en cada fila; la última se lleva la tabla.
func (d *Documento) borrarColumna(tabla, tr, td *html.Node) (Lugar, bool) {
	propias := celdasDe(tr)
	if len(propias) <= 1 {
		return d.quitarTabla(tabla), true
	}
	en := slices.Index(propias, td)
	for _, f := range filasDe(tabla) {
		if cs := celdasDe(f); len(cs) > 1 {
			f.RemoveChild(cs[min(en, len(cs)-1)])
		}
	}
	propias = celdasDe(tr)
	return d.primeroDesde(propias[min(en, len(propias)-1)])
}

// celdasDe son las celdas de una fila.
func celdasDe(tr *html.Node) []*html.Node {
	var out []*html.Node
	for h := tr.FirstChild; h != nil; h = h.NextSibling {
		if esCelda(h) {
			out = append(out, h)
		}
	}
	return out
}

// celdaVaciaNueva es la celda que crea la web: con su `<br>`.
func celdaVaciaNueva(etiqueta string) *html.Node {
	n := elemento(etiqueta)
	n.AppendChild(elemento("br"))
	return n
}

// quitarTabla es `removeTable`: la tabla se va y, si el documento se queda sin nada, se
// le pone el `<p><br></p>` donde seguir escribiendo. El cursor va a la vecina.
func (d *Documento) quitarTabla(tabla *html.Node) Lugar {
	vecina := siguienteElemento(tabla)
	if vecina == nil {
		vecina = anteriorElemento(tabla)
	}
	tabla.Parent.RemoveChild(tabla)
	if d.Asegurar() || vecina == nil {
		vecina = d.raiz.FirstChild
	}
	l, _ := d.primeroDesde(vecina)
	return l
}

func filasDe(tabla *html.Node) []*html.Node {
	var out []*html.Node
	for h := tabla.FirstChild; h != nil; h = h.NextSibling {
		switch h.DataAtom {
		case atom.Thead, atom.Tbody, atom.Tfoot:
			out = append(out, filasDe(h)...)
		case atom.Tr:
			out = append(out, h)
		default:
		}
	}
	return out
}

func siguienteElemento(n *html.Node) *html.Node {
	for h := n.NextSibling; h != nil; h = h.NextSibling {
		if h.Type == html.ElementNode {
			return h
		}
	}
	return nil
}

func anteriorElemento(n *html.Node) *html.Node {
	for h := n.PrevSibling; h != nil; h = h.PrevSibling {
		if h.Type == html.ElementNode {
			return h
		}
	}
	return nil
}

// Combinar es `mergeCells`: si las celdas forman un rectángulo sólido de la rejilla, la
// de arriba a la izquierda se queda con todo —lo que no está vacío de las demás, detrás
// de un espacio— y abarca el rectángulo; las demás se van. Si no, no hace nada.
func (d *Documento) Combinar(celdas []*Celda) (Lugar, bool) {
	if len(celdas) < 2 {
		return Lugar{}, false
	}
	r0, c0, r1, c1 := celdas[0].Fila, celdas[0].Col, -1, -1
	visto := map[[2]int]bool{}
	for _, c := range celdas {
		r0, c0 = min(r0, c.Fila), min(c0, c.Col)
		r1, c1 = max(r1, c.Fila+c.Filas-1), max(c1, c.Col+c.Cols-1)
		for f := c.Fila; f < c.Fila+c.Filas; f++ {
			for k := c.Col; k < c.Col+c.Cols; k++ {
				if visto[[2]int{f, k}] {
					return Lugar{}, false // se solapan
				}
				visto[[2]int{f, k}] = true
			}
		}
	}
	if len(visto) != (r1-r0+1)*(c1-c0+1) {
		return Lugar{}, false // no es un rectángulo limpio: no se estropea la tabla
	}
	var ancla *Celda
	for _, c := range celdas {
		if c.Fila == r0 && c.Col == c0 {
			ancla = c
		}
	}
	if ancla == nil {
		return Lugar{}, false
	}
	for _, c := range celdas {
		if c == ancla {
			continue
		}
		if !celdaVacia(c.nodo) {
			ancla.nodo.AppendChild(&html.Node{Type: html.TextNode, Data: " "})
			for h := c.nodo.FirstChild; h != nil; h = c.nodo.FirstChild {
				c.nodo.RemoveChild(h)
				ancla.nodo.AppendChild(h)
			}
		}
		c.nodo.Parent.RemoveChild(c.nodo)
	}
	fundirTextos(ancla.nodo)
	ponerAtributo(ancla.nodo, "rowspan", strconv.Itoa(r1-r0+1))
	ponerAtributo(ancla.nodo, "colspan", strconv.Itoa(c1-c0+1))
	return d.primeroDesde(ancla.nodo)
}

// fundirTextos junta los textos seguidos, que es como vuelve el árbol al leerlo: sin ello
// lo escrito no se leería como el mismo árbol y no se podría guardar.
func fundirTextos(n *html.Node) {
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		for h.Type == html.TextNode && h.NextSibling != nil && h.NextSibling.Type == html.TextNode {
			h.Data += h.NextSibling.Data
			n.RemoveChild(h.NextSibling)
		}
		fundirTextos(h)
	}
}

// celdaVacia es el `inner === ” || inner === '<br>'` de la web.
func celdaVacia(td *html.Node) bool {
	return td.FirstChild == nil || (td.FirstChild == td.LastChild && td.FirstChild.DataAtom == atom.Br && len(td.FirstChild.Attr) == 0)
}

// Dividir es `splitCell`: una celda que abarca varias vuelve a 1 × 1 y se rellenan con
// celdas vacías las casillas que deja libres.
func (d *Documento) Dividir(c *Celda) (Lugar, bool) {
	if c == nil || c.Filas == 1 && c.Cols == 1 {
		return Lugar{}, false
	}
	ponerAtributo(c.nodo, "rowspan", "1")
	ponerAtributo(c.nodo, "colspan", "1")
	filas := filasDe(c.Tabla.nodo)
	for df := range min(c.Filas, len(filas)-c.Fila) {
		tr := filas[c.Fila+df]
		for dc := range c.Cols {
			if df == 0 && dc == 0 {
				continue
			}
			// La rejilla se vuelve a leer en cada paso: refleja las celdas ya puestas. La
			// nueva va delante de la primera de su fila que queda a su derecha.
			var ref *html.Node
			for _, otra := range leerRejilla(c.Tabla.nodo, 0).Celdas {
				if otra.nodo.Parent == tr && otra.Col > c.Col+dc {
					ref = otra.nodo
					break
				}
			}
			tr.InsertBefore(celdaVaciaNueva(c.nodo.Data), ref)
		}
	}
	return d.primeroDesde(c.nodo)
}

// PonerCeldas es `setCellProp`: pone (o quita, con "" o «transparent») el fondo o la
// alineación vertical de las celdas. El color va como lo escribe el navegador,
// `rgb(r, g, b)`.
func (d *Documento) PonerCeldas(celdas []*Celda, prop, valor string) bool {
	if len(celdas) == 0 || (prop != "background-color" && prop != "vertical-align") {
		return false
	}
	if valor == "transparent" {
		valor = ""
	}
	if c, ok := colorCSS(valor); ok && prop == "background-color" && valor != "" {
		valor = fmt.Sprintf("rgb(%d, %d, %d)", c.R, c.G, c.B)
	}
	for _, c := range celdas {
		ponerDeclaracion(c.nodo, prop, valor)
	}
	return true
}

func ponerAtributo(n *html.Node, clave, valor string) {
	for i := range n.Attr {
		if n.Attr[i].Namespace == "" && n.Attr[i].Key == clave {
			n.Attr[i].Val = valor
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: clave, Val: valor})
}
