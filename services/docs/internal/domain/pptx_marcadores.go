// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/xml"
	"path"
	"strconv"
	"strings"
)

// Marcadores de posición de PresentationML. Una diapositiva de PowerPoint casi nunca
// trae la geometría de su título ni de su cuerpo: `<p:spPr/>` vacío, y la hereda del
// diseño (`slideLayout`) y, si el diseño tampoco la dice, del patrón (`slideMaster`).
// Sin resolverla, esas cajas salían en (0, 0) con el tamaño de por defecto.

// phGeoms es la geometría de los marcadores de una parte, por índice (`idx`) y por tipo, y
// sus listas de estilos de texto (`a:lstStyle`), por lo mismo.
type phGeoms struct {
	byIdx, byType         map[string]xfrmPos
	estiloIdx, estiloTipo map[string]nivelesTexto
}

// phTipo lleva los tipos de marcador al que tienen en el patrón: un marcador sin tipo
// es de contenido (`obj`), y el patrón lo llama `body`; el título y el subtítulo de
// portada son el título y el cuerpo.
func phTipo(t string) string {
	switch t {
	case "", "obj", "subTitle":
		return "body"
	case "ctrTitle":
		return "title"
	}
	return t
}

// readPhGeoms lee los marcadores de un diseño o de un patrón: la geometría de los que la
// dicen y la lista de estilos de texto de los que la traen.
func readPhGeoms(raw []byte) phGeoms {
	g := phGeoms{
		byIdx: map[string]xfrmPos{}, byType: map[string]xfrmPos{},
		estiloIdx: map[string]nivelesTexto{}, estiloTipo: map[string]nivelesTexto{},
	}
	var root xnode
	if xml.Unmarshal(raw, &root) != nil {
		return g
	}
	root.each("sp", func(sp *xnode) {
		ph, spPr := sp.first("ph"), sp.kid("spPr")
		if ph == nil {
			return
		}
		if tx := sp.kid("txBody"); tx != nil && tx.kid("lstStyle") != nil {
			n := nivelesDe(tx.kid("lstStyle"))
			if i := ph.attr("idx"); i != "" {
				g.estiloIdx[i] = n
			}
			if t := phTipo(ph.attr("type")); !g.estiloTipo[t].hay() {
				g.estiloTipo[t] = n
			}
		}
		if spPr == nil || spPr.kid("xfrm") == nil {
			return
		}
		off, ext := spPr.kid("xfrm").kid("off"), spPr.kid("xfrm").kid("ext")
		if off == nil || ext == nil {
			return
		}
		pos := xfrmPos{
			float64(atoiDefault(off.attr("x"), 0)), float64(atoiDefault(off.attr("y"), 0)),
			float64(atoiDefault(ext.attr("cx"), 0)), float64(atoiDefault(ext.attr("cy"), 0)),
		}
		if pos.cx == 0 && pos.cy == 0 {
			return
		}
		if i := ph.attr("idx"); i != "" {
			g.byIdx[i] = pos
		}
		if t := phTipo(ph.attr("type")); g.byType[t] == (xfrmPos{}) {
			g.byType[t] = pos
		}
	})
	return g
}

func (g phGeoms) find(typ, idx string) (xfrmPos, bool) {
	if p, ok := g.byIdx[idx]; ok && idx != "" {
		return p, true
	}
	p, ok := g.byType[phTipo(typ)]
	return p, ok
}

// estilo es la lista de estilos de texto del marcador, con el mismo orden de búsqueda que su
// geometría: por índice y, si no, por tipo.
func (g phGeoms) estilo(typ, idx string) (nivelesTexto, bool) {
	if n, ok := g.estiloIdx[idx]; ok && idx != "" {
		return n, true
	}
	n, ok := g.estiloTipo[phTipo(typ)]
	return n, ok
}

// placeholder es la geometría de un marcador sin la suya: la del diseño y, si no, la
// del patrón; `def` si ninguno la dice.
func (t *pptxTheme) placeholder(typ, idx string, def xfrmPos) xfrmPos {
	if t == nil {
		return def
	}
	for _, g := range []phGeoms{t.layout, t.master} {
		if p, ok := g.find(typ, idx); ok {
			return p
		}
	}
	return def
}

// layoutGeoms es la geometría de los marcadores del diseño de una diapositiva, leída una
// vez por diseño (`cache`).
func layoutGeoms(slidePath string, rels map[string]string, files map[string][]byte, cache map[string]phGeoms) phGeoms {
	for _, target := range rels {
		if !strings.Contains(target, "slideLayout") {
			continue
		}
		name := path.Clean(path.Dir(slidePath) + "/" + target)
		g, ok := cache[name]
		if !ok {
			g = readPhGeoms(files[name])
			cache[name] = g
		}
		return g
	}
	return phGeoms{}
}

// # El formato que hereda el texto
//
// Un párrafo de una diapositiva casi nunca dice su tamaño, su alineación ni su color: los
// hereda, según su nivel (`a:pPr lvl`), de las listas de estilos que tiene por encima. Son,
// de la más cercana a la más lejana, la de su forma (`a:lstStyle`), la del marcador del diseño,
// la del marcador del patrón, la del patrón para su clase de texto (`p:txStyles`: título,
// cuerpo u otro) y la de la presentación (`p:defaultTextStyle`); gana la primera que lo diga
// (ECMA-376 §19.3.1.51 y §21.1.2.3). Sin esto, el título de una diapositiva de PowerPoint, que
// no lleva tamaño propio, salía a 20 px, alineado a la izquierda y con la tinta del tema.

// nivelesTexto son los nueve niveles de una lista de estilos (`a:lvl1pPr`…`a:lvl9pPr`).
type nivelesTexto [9]*xnode

func (n nivelesTexto) hay() bool {
	for _, x := range n {
		if x != nil {
			return true
		}
	}
	return false
}

// nivelesDe lee los niveles de una lista de estilos (`a:lstStyle`, `p:titleStyle`…).
func nivelesDe(lista *xnode) (n nivelesTexto) {
	if lista == nil {
		return n
	}
	for i := range lista.Kids {
		k := &lista.Kids[i]
		num, ok := strings.CutPrefix(k.XMLName.Local, "lvl")
		if !ok {
			continue
		}
		if num, ok = strings.CutSuffix(num, "pPr"); !ok {
			continue
		}
		if l, err := strconv.Atoi(num); err == nil && l >= 1 && l <= len(n) {
			n[l-1] = k
		}
	}
	return n
}

// cadenaTexto son las listas de estilos que hereda el texto de una forma, de la más cercana
// a la más lejana.
type cadenaTexto []nivelesTexto

// textoHeredado es lo que la cadena da a los párrafos de un nivel: su alineación (`algn`), y a
// su texto el tamaño (centésimas de punto) y el nodo que lleva su relleno (`a:defRPr`).
type textoHeredado struct {
	algn  string
	sz    int
	letra *xnode
}

// nivel es lo que hereda un párrafo de nivel `l` (0 el primero): de cada cosa, lo que diga la
// primera lista que la diga.
func (c cadenaTexto) nivel(l int) textoHeredado {
	var h textoHeredado
	l = min(max(l, 0), len(nivelesTexto{})-1)
	for _, ns := range c {
		n := ns[l]
		if n == nil {
			continue
		}
		if h.algn == "" {
			h.algn = n.attr("algn")
		}
		d := n.kid("defRPr")
		if d == nil {
			continue
		}
		if h.sz == 0 {
			h.sz = atoiDefault(d.attr("sz"), 0)
		}
		if h.letra == nil && tieneRelleno(d) {
			h.letra = d
		}
	}
	return h
}

// tieneRelleno dice si el nodo lleva un relleno (el color de su texto).
func tieneRelleno(n *xnode) bool {
	for i := range n.Kids {
		if esRellenoDML(n.Kids[i].XMLName.Local) {
			return true
		}
	}
	return false
}

// claseDeTexto es la lista de `p:txStyles` que hereda un marcador: la del título, la de
// «otros» para la fecha, el pie, el número y el encabezado, y la del cuerpo para el resto.
func claseDeTexto(typ string) string {
	switch typ {
	case "title", "ctrTitle":
		return "titleStyle"
	case "dt", "ftr", "sldNum", "hdr":
		return "otherStyle"
	}
	return "bodyStyle"
}

// cadenaDe es la cadena que hereda el texto de una forma, sin su propia lista (que se lee con
// su texto, delante de esta): la de su marcador en el diseño y en el patrón y la de su clase
// en el patrón, si es un marcador, y la de la presentación.
func (t *pptxTheme) cadenaDe(nv nvProps) cadenaTexto {
	if t == nil {
		return nil
	}
	var c cadenaTexto
	if nv.ph {
		for _, g := range []phGeoms{t.layout, t.master} {
			if n, ok := g.estilo(nv.phType, nv.phIdx); ok {
				c = append(c, n)
			}
		}
		c = append(c, t.txStyles[claseDeTexto(nv.phType)])
	}
	return append(c, t.defecto)
}

// leerTxStyles lee del patrón sus listas por clase de texto (`p:txStyles`).
func leerTxStyles(raw []byte) map[string]nivelesTexto {
	var root xnode
	if xml.Unmarshal(raw, &root) != nil {
		return nil
	}
	tx := root.kid("txStyles")
	if tx == nil {
		return nil
	}
	out := map[string]nivelesTexto{}
	for i := range tx.Kids {
		out[tx.Kids[i].XMLName.Local] = nivelesDe(&tx.Kids[i])
	}
	return out
}
