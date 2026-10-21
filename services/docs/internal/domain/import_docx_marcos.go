// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/xml"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Marcos y cuadros de texto (framePr, §17.3.1.11; wp:anchor con wps:txbx, §20.4.2.3). El editor no
// tiene cajas que floten sobre la página, pero sí CSS: un marco es un <div data-marco> con lo que
// lleva dentro (los párrafos seguidos con el mismo marco, o el texto del cuadro), flotando a su
// lado con su ancho y sus distancias. Así las etiquetas de la columna izquierda de un currículum de
// Pages quedan a la izquierda de su texto, y la capitular de una novela, al principio del párrafo.
//
// ponytail: la posición vertical no se lee; el marco va en el flujo, donde está su ancla. Un cuadro
// colocado respecto a su párrafo o a su línea flota a su lado (también «delante» o «detrás» del
// texto, wrapNone: su texto quedaría tapado, y así se lee). Uno colocado respecto a la página (el
// encabezado de una plantilla de Pages, anclado en el párrafo que toque) no está junto a su ancla:
// va en su renglón, con su ancho y su posición en horizontal, y no se mete entre las líneas de al
// lado.

// docxMarco es la forma de un marco, de la que sale su CSS (y a la que vuelve la exportación).
type docxMarco struct {
	flota          string // left, right o "" (en su renglón: wrap notBeside o topAndBottom)
	ancho          float64
	izq, der, bajo float64 // márgenes en px: la posición a la izquierda y las distancias al texto
	centrado       bool
}

// css es el estilo del <div data-marco>, siempre en el mismo orden y en twips enteros (un cuadro
// anclado se mide en EMU): la exportación lo escribe en twips, y la vuelta tiene que dar lo mismo.
func (m docxMarco) css() string {
	twip := func(px float64) float64 { return math.Round(px*15) / 15 }
	m.ancho, m.izq, m.der, m.bajo = twip(m.ancho), twip(m.izq), twip(m.der), twip(m.bajo)
	var css docxCSS
	if m.flota != "" {
		css.add("float", m.flota)
	}
	if m.ancho > 0 {
		css.add("width", docxNum(m.ancho)+"px")
	}
	if m.centrado {
		css.add("margin-left", "auto")
		css.add("margin-right", "auto")
	} else {
		if m.izq != 0 {
			css.add("margin-left", docxNum(m.izq)+"px")
		}
		if m.der != 0 {
			css.add("margin-right", docxNum(m.der)+"px")
		}
	}
	if m.bajo != 0 {
		css.add("margin-bottom", docxNum(m.bajo)+"px")
	}
	return css.String()
}

// abrirMarco es el principio del bloque de un marco.
func abrirMarco(css string) xblock {
	return xblock{raw: `<div data-marco="" style="` + escaparAtributo.Replace(css) + `">`}
}

var cerrarMarco = xblock{raw: "</div>"}

// juntarMarcos junta dos marcos seguidos iguales en uno: es lo que hace Word con los párrafos
// seguidos de mismo marco (§17.3.1.11), y lo que volvería de exportar dos cuadros iguales.
func juntarMarcos(blocks []xblock) []xblock {
	out := blocks[:0:0]
	ultimo := "" // el principio del último marco abierto
	for i := 0; i < len(blocks); i++ {
		b := blocks[i]
		if strings.HasPrefix(b.raw, "<div data-marco") {
			ultimo = b.raw
		}
		if b.raw == cerrarMarco.raw && i+1 < len(blocks) && blocks[i+1].raw == ultimo {
			i++ // ni el cierre ni el nuevo principio
			continue
		}
		out = append(out, b)
	}
	return out
}

// marcoCSS es el CSS del marco de un párrafo (framePr), o "" si no lo es: el de Pages en
// pPrDefault (ancho y alto 0, a la izquierda, sin ajuste) no es un marco sino su ausencia.
func (h *docxHoja) marcoCSS(t xml.StartElement) string {
	num := func(a string) float64 {
		n, err := strconv.ParseFloat(attrLocal(t, a), 64)
		if err != nil || math.Abs(n) > 2*31680 {
			return 0
		}
		return n
	}
	w, alto, x, y := num("w"), num("h"), num("x"), num("y")
	xAlign, wrap, capitular := attrLocal(t, "xAlign"), attrLocal(t, "wrap"), attrLocal(t, "dropCap")
	if marcoNeutro(w, alto, x, y, xAlign, wrap, capitular) {
		return ""
	}
	m := docxMarco{ancho: w / 15, bajo: num("vSpace") / 15}
	m.flota, m.centrado = ladoDeMarco(xAlign, wrap), xAlign == "center" && m.ancho > 0
	espacio := num("hSpace") / 15
	switch {
	case m.flota == "right":
		m.izq = espacio
	case m.flota == "left" || m.flota == "" && !m.centrado:
		if xAlign == "" || xAlign == "left" {
			m.izq = x / 15
			if attrLocal(t, "hAnchor") == "page" {
				m.izq -= h.margenIzq / 15
			}
		}
		if m.flota != "" {
			m.der = espacio
		}
	}
	return m.css()
}

// marcoNeutro dice si un framePr es el que Pages pone en pPrDefault: sin ancho ni alto, a la
// izquierda, sin desplazar y sin ajuste ni capitular. No es un marco sino su ausencia.
func marcoNeutro(w, alto, x, y float64, xAlign, wrap, capitular string) bool {
	return w == 0 && alto == 0 && x == 0 && y == 0 && (xAlign == "" || xAlign == "left") && wrap == "" &&
		(capitular == "" || capitular == "none")
}

// ladoDeMarco es hacia dónde flota un marco: a ninguno si va en su renglón (notBeside, none) o
// centrado, a la derecha si se alinea ahí y a la izquierda si no.
func ladoDeMarco(xAlign, wrap string) string {
	switch {
	case xAlign == "center", wrap == "notBeside", wrap == "none":
		return ""
	case xAlign == "right" || xAlign == "outside":
		return "right"
	}
	return "left"
}

// docxAncla es lo que importa de un wp:anchor para colocar su cuadro de texto.
type docxAncla struct {
	ajuste           string  // wrapSquare, wrapNone…
	desde, alinea    string  // positionH: relativeFrom y align
	desdeV           string  // positionV: relativeFrom
	x                float64 // positionH/posOffset, en px
	distIzq, distDer float64 // distL y distR, en px
	distBajo         float64
	conX             bool
}

// leerAncla lee, del elemento en curso de un wp:anchor, lo que coloca su cuadro.
func (d *docxDec) leerAncla(t xml.StartElement, a *docxAncla, enH bool) {
	emu := func(v string) float64 { n, _ := strconv.ParseFloat(v, 64); return n / 9525 }
	switch t.Name.Local {
	case "anchor":
		a.distIzq, a.distDer, a.distBajo = emu(attrLocal(t, "distL")), emu(attrLocal(t, "distR")), emu(attrLocal(t, "distB"))
	case "positionH":
		a.desde = attrLocal(t, "relativeFrom")
	case "positionV":
		a.desdeV = attrLocal(t, "relativeFrom")
	case "posOffset":
		if enH {
			a.x, a.conX = emu(strings.TrimSpace(d.readText("posOffset"))), true
		}
	case "align":
		if enH {
			a.alinea = strings.TrimSpace(d.readText("align"))
		}
	case "wrapNone", "wrapSquare", "wrapTight", "wrapThrough", "wrapTopAndBottom":
		a.ajuste = t.Name.Local
	}
}

// cuadroCSS es el CSS del marco de un cuadro de texto anclado de ancho anchoPx.
func (d *docxDec) cuadroCSS(a docxAncla, anchoPx float64) string {
	h := d.h()
	x := a.x
	if a.desde == "page" || a.desde == "leftMargin" {
		x -= h.margenIzq / 15
	}
	texto := h.anchoTexto / 15
	if texto > 0 {
		// Una caja más ancha que el texto (la de una plantilla de maquetación de Pages, que
		// ocupa la página) se queda en su ancho, y dentro de la página.
		anchoPx = math.Min(anchoPx, texto)
		x = math.Max(-h.margenIzq/15, math.Min(x, texto-anchoPx))
	}
	m := docxMarco{ancho: anchoPx}
	derecha := a.alinea == "right" || a.alinea == "outside" || a.alinea == "" && a.conX && texto > 0 && x+anchoPx/2 > texto/2
	enSuRenglon := a.ajuste == "wrapTopAndBottom" || a.desdeV != "" && a.desdeV != "paragraph" && a.desdeV != "line"
	switch {
	case a.alinea == "center":
		m.centrado = anchoPx > 0
	case enSuRenglon:
		if !derecha {
			m.izq = x
		}
	case derecha:
		m.flota = "right"
		if a.ajuste != "wrapNone" {
			m.izq = a.distIzq
		}
	default:
		m.flota, m.izq = "left", x
		if a.ajuste != "wrapNone" {
			m.der = a.distDer
		}
	}
	if a.ajuste != "wrapNone" {
		m.bajo = a.distBajo
	}
	return m.css()
}

var (
	rePgSz  = regexp.MustCompile(`<(?:\w+:)?pgSz\b[^>]*>`)
	rePgMar = regexp.MustCompile(`<(?:\w+:)?pgMar\b[^>]*>`)
	reAttrW = regexp.MustCompile(`(?:\w+:)?(w|left|right)="(-?\d+(?:\.\d+)?)"`)
)

// seccionDe lee de document.xml, antes de recorrerlo, el ancho de página y los márgenes laterales
// de la última sección (los de todo el documento salvo saltos de sección), en twips: un cuadro
// anclado a la página se coloca desde el margen, y su sección va al final.
func seccionDe(doc []byte) (anchoPag, izq, der float64) {
	atributos := func(tag []byte) map[string]float64 {
		out := map[string]float64{}
		for _, m := range reAttrW.FindAllSubmatch(tag, -1) {
			n, _ := strconv.ParseFloat(string(m[2]), 64)
			out[string(m[1])] = n
		}
		return out
	}
	if t := rePgSz.FindAll(doc, -1); len(t) > 0 {
		anchoPag = atributos(t[len(t)-1])["w"]
	}
	izq, der = 1440, 1440
	if t := rePgMar.FindAll(doc, -1); len(t) > 0 {
		a := atributos(t[len(t)-1])
		izq, der = math.Abs(a["left"]), math.Abs(a["right"])
	}
	return anchoPag, izq, der
}
