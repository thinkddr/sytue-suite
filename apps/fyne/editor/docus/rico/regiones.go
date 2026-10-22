// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"slices"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// El ENCABEZADO y el PIE de «Insertar» (`EncabezadoPieGroup` de `ribbon/tabs/Insertar.tsx`),
// con el mismo HTML: un `<div data-doc-header>` al principio del folio y un
// `<div data-doc-footer>` al final (antes de las notas al pie), con su posición en línea
// —van fuera de la corriente, en el margen—; «Primera página distinta» es
// `data-first-page-different` en el encabezado, que entonces no se ve; y el número de
// página es un `<span data-page-field>` cuyo texto pone la vista.

// Los estilos de `HEADER_STYLE` y `FOOTER_STYLE`: van en línea porque el HTML es el modelo
// y viaja a las exportaciones. Vuelven tal cual; no se leen más que para la letra.
const (
	EstiloEncabezado = "position:absolute;left:var(--doc-page-margin,96px);right:var(--doc-page-margin,96px);" +
		"top:calc(var(--doc-page-margin,96px) - 30px);" +
		"display:flex;align-items:flex-end;gap:8px;min-height:20px;padding-bottom:6px;" +
		"border-bottom:1px solid rgba(15,23,42,0.16);font-size:12px"
	EstiloPie = "position:absolute;left:var(--doc-page-margin,96px);right:var(--doc-page-margin,96px);" +
		"bottom:calc(var(--doc-page-margin,96px) - 30px);" +
		"display:flex;align-items:flex-start;gap:8px;min-height:20px;padding-top:6px;" +
		"border-top:1px solid rgba(15,23,42,0.16);font-size:12px"
	// EstiloCampoPagina es el del `<span data-page-field>` de `insertPageField`.
	EstiloCampoPagina = "white-space:nowrap"
)

// Encabezado y Pie son los dos valores de [Bloque.Region].
const (
	Encabezado = "header"
	Pie        = "footer"
)

// tintaRegion es el `color:#475569` que `ensureHeaderFooterStyle` pone a las dos regiones.
var tintaRegion = tintaCita

// regionDe dice si n es el encabezado o el pie, y cuál.
func regionDe(n *html.Node) string {
	if n == nil || n.Type != html.ElementNode || n.DataAtom != atom.Div || n.Namespace != "" {
		return ""
	}
	switch {
	case tieneAtributo(n, "data-doc-header"):
		return Encabezado
	case tieneAtributo(n, "data-doc-footer"):
		return Pie
	}
	return ""
}

// esCampoPagina dice si n es el `<span data-page-field>` del número de página.
func esCampoPagina(n *html.Node) bool {
	return n != nil && n.Type == html.ElementNode && n.DataAtom == atom.Span && n.Namespace == "" && tieneAtributo(n, "data-page-field")
}

// objetoAlLeer es esObjeto al recorrer el ÁRBOL, donde además el marcador del índice sin
// texto (el de una selección vacía) es un objeto: una runa sin ancho que no se pierde al
// reescribir. En las cadenas de una edición el objeto se reconoce por su runa.
func objetoAlLeer(n *html.Node) bool {
	return esObjeto(n) || esEntradaIndice(n) && n.FirstChild == nil
}

// esObjeto dice si n ocupa UNA runa en el texto: una imagen, algo de otro espacio de nombres,
// el campo del número de página —que se pinta con el texto que le toque— o el enlace de
// vuelta de una nota al final.
func esObjeto(n *html.Node) bool {
	return n != nil && (objetos[n.DataAtom] || n.Namespace != "" || esCampoPagina(n) || esVuelta(n) || esCita(n) || esCampoCombinacion(n))
}

// entenderRegion apunta lo que impide editar una región: que no cuelgue del folio (se
// pinta con respecto a él) o que lleve bloques dentro, que la web colocaría uno al lado del
// otro (`display: flex`) y aquí no se sabe.
func entenderRegion(n *html.Node, motivos map[string]bool) {
	if n.Parent == nil || n.Parent.DataAtom != atom.Body {
		motivos["div[data-doc-"+regionDe(n)+"] dentro de otro"] = true
	}
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		if !enLinea(h, 0) {
			motivos["div[data-doc-"+regionDe(n)+"] con bloques"] = true
		}
	}
}

// Region es `ensureRegion`: el lugar del encabezado (o del pie) para llevar allí el cursor,
// creándolo si no hay. El encabezado va lo primero del folio y el pie lo último, delante de
// las notas al pie.
func (d *Documento) Region(cual string) (Lugar, bool) {
	var hallada *html.Node
	for h := d.raiz.FirstChild; h != nil; h = h.NextSibling {
		if regionDe(h) == cual {
			hallada = h
			break
		}
	}
	if hallada == nil {
		switch cual {
		case Encabezado:
			hallada = elemento("div", html.Attribute{Key: "data-doc-header"}, html.Attribute{Key: "style", Val: EstiloEncabezado})
			d.raiz.InsertBefore(hallada, d.raiz.FirstChild)
		case Pie:
			hallada = elemento("div", html.Attribute{Key: "data-doc-footer"}, html.Attribute{Key: "style", Val: EstiloPie})
			d.raiz.InsertBefore(hallada, d.seccionNotas())
		default:
			return Lugar{}, false
		}
	}
	for _, b := range d.Bloques() {
		if b.cont == hallada {
			return b.Lugar, true
		}
	}
	return Lugar{}, false
}

// AlternarPrimeraDistinta es `toggleFirstPageDifferent`: pone o quita
// `data-first-page-different` en el encabezado. Sin encabezado no hace nada.
func (d *Documento) AlternarPrimeraDistinta() bool {
	for h := d.raiz.FirstChild; h != nil; h = h.NextSibling {
		if regionDe(h) != Encabezado {
			continue
		}
		if i := slices.IndexFunc(h.Attr, func(a html.Attribute) bool { return a.Key == "data-first-page-different" }); i >= 0 {
			h.Attr = slices.Delete(h.Attr, i, i+1)
		} else {
			h.Attr = append(h.Attr, html.Attribute{Key: "data-first-page-different"})
		}
		return true
	}
	return false
}

// InsertarCampoPagina es `insertPageField`: el `<span data-page-field>` en pos, con el
// texto que la web le pone al insertarlo (`status.pageOf`, ya traducido).
func (e *Edicion) InsertarCampoPagina(pos int, texto string) {
	campo := elemento("span", html.Attribute{Key: "data-page-field"}, html.Attribute{Key: "style", Val: EstiloCampoPagina})
	campo.AppendChild(&html.Node{Type: html.TextNode, Data: texto})
	pos = max(0, min(pos, len(e.runas)))
	cadena := append(slices.Clip(e.hereda(pos)), campo)
	e.runas = slices.Insert(e.runas, pos, ObjetoRuna)
	e.cadenas = slices.Insert(e.cadenas, pos, cadena)
	e.hayPendiente = false
	e.reescribir()
}

// Vacio dice si el bloque no tiene nada dentro, ni un `<br>`: el `:empty` de CSS.
func (l Lugar) Vacio() bool { return l.primero == nil }

// EnRegion dice si el bloque abierto es el encabezado o el pie: ahí no se parte en
// párrafos ni se cambia de tipo, que la región es UNA línea.
func (e *Edicion) EnRegion() bool { return e.cubre() && regionDe(e.cont) != "" }

// CortesFlex son las runas donde empieza otro hijo del bloque: en una región, cada uno es
// un elemento de su `display: flex` y va separado del de al lado por el `gap: 8px`.
func (b Bloque) CortesFlex() []int {
	var out []int
	pos := 0
	var antes *html.Node
	primero := true
	for _, t := range b.Tramos {
		var hijo *html.Node
		if len(t.cadena) > 0 {
			hijo = t.cadena[0]
		} else if t.nodo != nil {
			hijo = t.nodo
		}
		if !primero && hijo != antes {
			out = append(out, pos)
		}
		antes, primero = hijo, false
		n := len([]rune(t.Texto))
		if t.Salto || t.Objeto != "" {
			n++
		}
		pos += n
	}
	return out
}
