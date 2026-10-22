// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"html"
	"slices"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// El ÍNDICE ALFABÉTICO de «Referencias» (`alphaindex.ts`, `markIndexEntry` y
// `regenerarIndice` de Referencias.tsx), con el mismo HTML: cada término marcado es un
// `<span data-index-entry data-index-sub>` que no cambia cómo se ve el texto, y el índice
// es una `<section data-index>` con los términos en orden alfabético, sus subentradas y
// un enlace a cada aparición.

// Las listas del índice, sin viñetas: la de los términos y la de sus subentradas, con
// sangría (`LIST_STYLE` y `SUB_STYLE`).
const (
	estiloListaIndice = "list-style:none;padding-left:0;margin:.25em 0"
	estiloSubIndice   = "list-style:none;padding-left:1.5em;margin:0"
)

func esEntradaIndice(n *xhtml.Node) bool {
	return n != nil && n.DataAtom == atom.Span && n.Namespace == "" && tieneAtributo(n, "data-index-entry")
}

// enIndice dice si n está dentro de la sección del índice.
func enIndice(n *xhtml.Node) bool {
	for p := n; p != nil; p = p.Parent {
		if esSeccionIndice(p) {
			return true
		}
	}
	return false
}

func esSeccionIndice(n *xhtml.Node) bool {
	return n != nil && n.DataAtom == atom.Section && tieneAtributo(n, "data-index")
}

// enSeccion dice si n está dentro de una sección generada (notas, índice…): ahí no se
// marca ni se ancla nada (`rangoEnCuerpo`).
func enSeccion(n *xhtml.Node) bool {
	for p := n; p != nil; p = p.Parent {
		if p.DataAtom == atom.Section {
			return true
		}
	}
	return false
}

// TerminoIndice es `parseIndexTerm`: «término» o «término:subtérmino», partido por los
// primeros dos puntos. ok es false sin término.
func TerminoIndice(crudo string) (termino, sub string, ok bool) {
	s := strings.TrimSpace(crudo)
	if s == "" {
		return "", "", false
	}
	termino, sub, _ = strings.Cut(s, ":")
	termino, sub = strings.TrimSpace(termino), strings.TrimSpace(sub)
	return termino, sub, termino != ""
}

// MarcarIndice es `markIndexEntry`: envuelve [desde, hasta) en el `<span data-index-entry>`
// del término, por dentro del formato; sin selección deja el marcador vacío en desde. Dice
// si lo ha hecho: dentro de una sección generada, no.
func (e *Edicion) MarcarIndice(desde, hasta int, termino, sub string) bool {
	if enSeccion(e.cont) {
		return false
	}
	atributos := []xhtml.Attribute{{Key: "data-index-entry", Val: termino}}
	if sub != "" {
		atributos = append(atributos, xhtml.Attribute{Key: "data-index-sub", Val: sub})
	}
	marca := elemento("span", atributos...)
	desde, hasta = max(0, min(desde, len(e.runas))), max(0, min(hasta, len(e.runas)))
	if desde >= hasta {
		// El marcador vacío es un objeto: UNA runa sin ancho, que no se pierde al reescribir.
		cadena := append(slices.Clip(e.hereda(desde)), marca)
		e.runas = slices.Insert(e.runas, desde, ObjetoRuna)
		e.cadenas = slices.Insert(e.cadenas, desde, cadena)
		e.hayPendiente = false
		e.reescribir()
		return true
	}
	e.aplicar(desde, hasta, func(c []*xhtml.Node) []*xhtml.Node { return append(slices.Clip(c), marca) })
	return true
}

// entradaIndice es una aparición marcada, ya numerada (`IndexEntry`).
type entradaIndice struct{ termino, sub, ref string }

// grupoIndice es un término con sus referencias directas y sus subentradas (`IndexTerm`).
type grupoIndice struct {
	termino string
	refs    []string
	subs    []subIndice
}

type subIndice struct {
	sub  string
	refs []string
}

var ordenIndice = collate.New(language.Spanish)

// agruparIndice es `buildIndex`: agrupa por término —exacto— y subtérmino, y ordena los
// dos alfabéticamente como `localeCompare(…, 'es')`; las referencias, en el orden del
// documento y sin repetir.
func agruparIndice(entradas []entradaIndice) []grupoIndice {
	var grupos []grupoIndice
	pos := map[string]int{}
	for _, e := range entradas {
		t := strings.TrimSpace(e.termino)
		if t == "" {
			continue
		}
		i, ok := pos[t]
		if !ok {
			i = len(grupos)
			pos[t] = i
			grupos = append(grupos, grupoIndice{termino: t})
		}
		g := &grupos[i]
		sub := strings.TrimSpace(e.sub)
		if sub == "" {
			if !slices.Contains(g.refs, e.ref) {
				g.refs = append(g.refs, e.ref)
			}
			continue
		}
		k := slices.IndexFunc(g.subs, func(s subIndice) bool { return s.sub == sub })
		if k < 0 {
			k = len(g.subs)
			g.subs = append(g.subs, subIndice{sub: sub})
		}
		if !slices.Contains(g.subs[k].refs, e.ref) {
			g.subs[k].refs = append(g.subs[k].refs, e.ref)
		}
	}
	for i := range grupos {
		slices.SortStableFunc(grupos[i].subs, func(a, b subIndice) int { return ordenIndice.CompareString(a.sub, b.sub) })
	}
	slices.SortStableFunc(grupos, func(a, b grupoIndice) int { return ordenIndice.CompareString(a.termino, b.termino) })
	return grupos
}

// escaparWeb es el `escapeHtml` de la web: también las comillas simples.
var escaparWeb = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")

func enlacesIndice(refs []string) string {
	partes := make([]string, len(refs))
	for i, r := range refs {
		partes[i] = `<a href="#` + escaparWeb.Replace("idx-"+r) + `">` + escaparWeb.Replace(r) + `</a>`
	}
	return strings.Join(partes, ", ")
}

// htmlIndiceAlfa es `buildIndexHtml`: la lista de términos con sus enlaces y sus
// subentradas; "" sin entradas.
func htmlIndiceAlfa(entradas []entradaIndice) string {
	grupos := agruparIndice(entradas)
	if len(grupos) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<ul style="` + estiloListaIndice + `">`)
	for _, g := range grupos {
		b.WriteString(`<li data-index-term>` + escaparWeb.Replace(g.termino))
		if len(g.refs) > 0 {
			b.WriteString(" " + enlacesIndice(g.refs))
		}
		if len(g.subs) > 0 {
			b.WriteString(`<ul style="` + estiloSubIndice + `">`)
			for _, s := range g.subs {
				b.WriteString(`<li data-index-sub>` + escaparWeb.Replace(s.sub) + " " + enlacesIndice(s.refs) + `</li>`)
			}
			b.WriteString(`</ul>`)
		}
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul>`)
	return b.String()
}

// GenerarIndiceAlfa es «Insertar o actualizar índice» (`regenerarIndice`): numera las
// apariciones en orden de documento —su `id` es el ancla del enlace—, y rehace la sección
// en su sitio o la pone tras el bloque i (o antes de las notas, o al final). `titulo` y
// `vacio` son `ref.index` y `ref.index.empty`, ya traducidos. Devuelve el lugar del título.
func (d *Documento) GenerarIndiceAlfa(bloques []Bloque, i int, titulo, vacio string) (Lugar, bool) {
	marcas := d.marcasIndiceAlfa()
	entradas := make([]entradaIndice, 0, len(marcas))
	for k, span := range marcas {
		ref := strconv.Itoa(k + 1)
		ponerAtributo(span, "id", "idx-"+ref)
		entradas = append(entradas, entradaIndice{atributo(span, "data-index-entry"), atributo(span, "data-index-sub"), ref})
	}
	lista := htmlIndiceAlfa(entradas)
	if lista == "" {
		lista = `<p style="color:#64748b"><i>` + html.EscapeString(vacio) + `</i></p>`
	}
	nodos, err := xhtml.ParseFragment(strings.NewReader(`<p><b>`+html.EscapeString(titulo)+`</b></p>`+lista), cuerpo())
	if err != nil {
		return Lugar{}, false
	}
	seccion := d.seccionIndice()
	if seccion == nil {
		seccion = elemento("section", xhtml.Attribute{Key: "data-index"})
		d.colocarSeccion(bloques, i, seccion)
	}
	vaciar(seccion)
	for _, n := range nodos {
		seccion.AppendChild(n)
	}
	return d.primeroDesde(seccion)
}

// marcasIndiceAlfa son los marcadores del índice, en orden de documento.
func (d *Documento) marcasIndiceAlfa() []*xhtml.Node {
	var out []*xhtml.Node
	var recorrer func(n *xhtml.Node, prof int)
	recorrer = func(n *xhtml.Node, prof int) {
		for h := n.FirstChild; h != nil && prof <= profundidadMax; h = h.NextSibling {
			if esEntradaIndice(h) {
				out = append(out, h)
			}
			recorrer(h, prof+1)
		}
	}
	recorrer(d.raiz, 0)
	return out
}

func (d *Documento) seccionIndice() *xhtml.Node {
	for h := d.raiz.FirstChild; h != nil; h = h.NextSibling {
		if esSeccionIndice(h) {
			return h
		}
	}
	return nil
}

// colocarSeccion pone una sección generada NUEVA tras el bloque de arriba del todo del
// cursor (`colocarSeccionIndice`) o, si el cursor no está en el cuerpo, antes de las notas
// que cierran el folio, o al final.
func (d *Documento) colocarSeccion(bloques []Bloque, i int, seccion *xhtml.Node) {
	if i >= 0 && i < len(bloques) && bloques[i].cont != nil && !enSeccion(bloques[i].cont) && bloques[i].Region == "" {
		arriba := superiorDe(bloques[i].cont)
		if bloques[i].cont.DataAtom == atom.Body {
			arriba = bloques[i].ultimo
		}
		if arriba != nil && arriba.Parent == d.raiz {
			d.raiz.InsertBefore(seccion, arriba.NextSibling)
			return
		}
	}
	for h := d.raiz.FirstChild; h != nil; h = h.NextSibling {
		if h.DataAtom == atom.Section && (tieneAtributo(h, "data-footnotes") || esSeccionFinal(h)) {
			d.raiz.InsertBefore(seccion, h)
			return
		}
	}
	d.raiz.AppendChild(seccion)
}
