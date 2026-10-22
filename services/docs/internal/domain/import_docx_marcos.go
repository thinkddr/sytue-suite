// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"cmp"
	"encoding/xml"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Marcos y cuadros de texto (framePr, §17.3.1.11; wp:anchor con wps:txbx, §20.4.2.3; VML w:pict con
// v:shape/v:textbox). El editor no tiene cajas que floten sobre la página, pero sí CSS: un marco es
// un <div data-marco> con lo que lleva dentro (los párrafos y tablas seguidos con el mismo marco, o
// el contenido del cuadro), flotando a su lado con su ancho y sus distancias. Así las etiquetas de
// la columna izquierda de un currículum de Pages quedan a la izquierda de su texto, y la capitular
// de una novela, al principio del párrafo.
//
// La posición vertical respecto a la página o a sus márgenes ordena los cuadros y marcos de cada
// página de arriba abajo (y de izquierda a derecha cuando comparten renglón) y lleva al principio o
// al final de la página los anclados en un párrafo intermedio. Dos cuadros que comparten banda
// vertical en columnas distintas flotan en paralelo.

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

// marcoCSS es el CSS del marco de un párrafo (framePr), con su ancla si va colocado respecto a la
// página, o "" si no lo es: el de Pages en pPrDefault (ancho y alto 0, a la izquierda, sin ajuste)
// no es un marco sino su ausencia.
func (h *docxHoja) marcoCSS(t xml.StartElement) (string, docxAncla) {
	num := func(a string) float64 {
		n, err := strconv.ParseFloat(attrLocal(t, a), 64)
		if err != nil || math.Abs(n) > 2*31680 {
			return 0
		}
		return n
	}
	w, alto, x, y := num("w"), num("h"), num("x"), num("y")
	xAlign, yAlign := attrLocal(t, "xAlign"), attrLocal(t, "yAlign")
	hAnchor, vAnchor := attrLocal(t, "hAnchor"), attrLocal(t, "vAnchor")
	wrap, capitular := attrLocal(t, "wrap"), attrLocal(t, "dropCap")
	if marcoNeutro(w, alto, x, y, xAlign, yAlign, wrap, capitular) {
		return "", docxAncla{}
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
			if hAnchor == "page" {
				m.izq -= h.margenIzq / 15
			}
		}
		if m.flota != "" {
			m.der = espacio
		}
	}
	var a docxAncla
	if m.flota == "" {
		a = docxAncla{
			desde: hAnchor, alinea: xAlign,
			desdeV: vAnchor, alineaV: yAlign,
			x: x / 15, y: y / 15,
			conX: attrLocal(t, "x") != "",
		}
	}
	return m.css(), a
}

// marcoNeutro dice si un framePr es el que Pages pone en pPrDefault: sin ancho ni alto, a la
// izquierda, sin desplazar y sin ajuste ni capitular. No es un marco sino su ausencia.
func marcoNeutro(w, alto, x, y float64, xAlign, yAlign, wrap, capitular string) bool {
	return w == 0 && alto == 0 && x == 0 && y == 0 && (xAlign == "" || xAlign == "left") && yAlign == "" && wrap == "" &&
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

// docxAncla es lo que importa de un wp:anchor o de un cuadro VML para colocar su cuadro de texto.
type docxAncla struct {
	ajuste           string  // wrapSquare, wrapNone…
	desde, alinea    string  // positionH: relativeFrom y align
	desdeV, alineaV  string  // positionV: relativeFrom y align
	x, y             float64 // posOffset horizontal y vertical, en px
	distIzq, distDer float64 // distL y distR, en px
	distBajo         float64
	conX             bool
}

// enPagina dice si la posición vertical viene dada respecto a la página o a sus márgenes (no
// respecto al párrafo o a la línea donde está el ancla).
func (a docxAncla) enPagina() bool {
	switch a.desdeV {
	case "page", "margin", "topMargin", "bottomMargin":
		return true
	}
	switch a.alineaV {
	case "top", "center", "bottom", "inside", "outside":
		return true
	}
	return false
}

// docxCaja es un cuadro de texto o un marco de página pendiente de colocar.
type docxCaja struct {
	ancla       docxAncla
	ancho, alto float64
	anclado     bool
	marco       string // CSS ya calculado cuando viene de w:framePr
	bloques     []xblock
}

// leerAncla lee, del elemento en curso de un wp:anchor, lo que coloca su cuadro.
func (d *docxDec) leerAncla(t xml.StartElement, a *docxAncla, anclado, enH, enV *bool) {
	emu := func(v string) float64 { n, _ := strconv.ParseFloat(v, 64); return n / 9525 }
	switch t.Name.Local {
	case "anchor":
		*anclado = true
		a.distIzq, a.distDer, a.distBajo = emu(attrLocal(t, "distL")), emu(attrLocal(t, "distR")), emu(attrLocal(t, "distB"))
	case "positionH":
		*enH = true
		a.desde = attrLocal(t, "relativeFrom")
	case "positionV":
		*enV = true
		a.desdeV = attrLocal(t, "relativeFrom")
	case "posOffset":
		v := emu(strings.TrimSpace(d.readText("posOffset")))
		if *enH {
			a.x, a.conX = v, true
		} else if *enV {
			a.y = v
		}
	case "align":
		v := strings.TrimSpace(d.readText("align"))
		if *enH {
			a.alinea = v
		} else if *enV {
			a.alineaV = v
		}
	case "wrapNone", "wrapSquare", "wrapTight", "wrapThrough", "wrapTopAndBottom":
		a.ajuste = t.Name.Local
	}
}

// docxVML es la geometría de una forma o de un grupo VML (<v:shape>, <v:rect>, <v:group>…).
type docxVML struct {
	ancla          docxAncla
	ancho, alto    float64
	anclado        bool
	ox, oy, cw, ch float64 // coordorigin y coordsize de un <v:group>
}

// leerFormaVML lee el atributo style (y coordsize/coordorigin en un <v:group>) de una forma VML.
// Si está dentro de un <v:group>, escala sus coordenadas al tamaño del grupo en la página.
func leerFormaVML(t xml.StartElement, grupo docxVML) docxVML {
	f := estiloFormaVML(attrLocal(t, "style"))
	if grupo.cw > 0 && grupo.ch > 0 && grupo.ancho > 0 && grupo.alto > 0 && t.Name.Local != "group" {
		left, top, width, height := f.ancla.x, f.ancla.y, f.ancho, f.alto
		f.anclado = grupo.anclado || f.anclado
		f.ancho = width / grupo.cw * grupo.ancho
		f.alto = height / grupo.ch * grupo.alto
		f.ancla = grupo.ancla
		f.ancla.x = grupo.ancla.x + (left-grupo.ox)/grupo.cw*grupo.ancho
		f.ancla.y = grupo.ancla.y + (top-grupo.oy)/grupo.ch*grupo.alto
		f.ancla.conX = true
	} else if f.ancla.ajuste == "" {
		f.ancla.ajuste = "wrapNone"
	}
	if t.Name.Local == "group" {
		f.cw, f.ch = parVML(attrLocal(t, "coordsize"))
		f.ox, f.oy = parVML(attrLocal(t, "coordorigin"))
	}
	return f
}

func estiloFormaVML(style string) (f docxVML) {
	for _, decl := range strings.Split(style, ";") {
		k, v, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.ToLower(strings.TrimSpace(v))
		switch k {
		case "position":
			f.anclado = v == "absolute"
		case "width":
			f.ancho = pxDeVML(v)
		case "height":
			f.alto = pxDeVML(v)
		case "margin-left", "left":
			f.ancla.x, f.ancla.conX = pxDeVML(v), true
		case "margin-top", "top":
			f.ancla.y = pxDeVML(v)
		case "mso-position-horizontal", "mso-position-vertical":
			if v != "absolute" {
				if strings.HasSuffix(k, "horizontal") {
					f.ancla.alinea = v
				} else {
					f.ancla.alineaV = v
				}
			}
		case "mso-position-horizontal-relative":
			f.ancla.desde = origenVML(v)
		case "mso-position-vertical-relative":
			f.ancla.desdeV = origenVML(v)
		case "mso-wrap-distance-left":
			f.ancla.distIzq = pxDeVML(v)
		case "mso-wrap-distance-right":
			f.ancla.distDer = pxDeVML(v)
		case "mso-wrap-distance-bottom":
			f.ancla.distBajo = pxDeVML(v)
		}
	}
	return f
}

// elementoVML procesa un elemento VML (<v:group>, <v:shape>… o <w10:wrap>) dentro de readDrawing.
func elementoVML(t xml.StartElement, forma, grupo *docxVML, enVML *bool, leidas []docxCaja) {
	switch t.Name.Local {
	case "group":
		*grupo = leerFormaVML(t, docxVML{})
	case "shape", "rect", "roundrect", "oval":
		*forma = leerFormaVML(t, *grupo)
		*enVML = true
	case "wrap":
		aplicarWrapVML(t, &forma.ancla)
		aplicarWrapVML(t, &grupo.ancla)
		for i := range leidas {
			aplicarWrapVML(t, &leidas[i].ancla)
			if leidas[i].ancho > 0 {
				leidas[i].anclado = true
			}
		}
	}
}

// aplicarWrapVML incorpora un <w10:wrap> (que en VML va después de <v:textbox>) al ancla.
func aplicarWrapVML(t xml.StartElement, a *docxAncla) {
	switch attrLocal(t, "type") {
	case "square":
		a.ajuste = "wrapSquare"
	case "tight":
		a.ajuste = "wrapTight"
	case "through":
		a.ajuste = "wrapThrough"
	case "topAndBottom":
		a.ajuste = "wrapTopAndBottom"
	case "none":
		a.ajuste = "wrapNone"
	}
	if x := origenVML(attrLocal(t, "anchorx")); x != "" && a.desde == "" {
		a.desde = x
	}
	if y := origenVML(attrLocal(t, "anchory")); y != "" && a.desdeV == "" {
		a.desdeV = y
	}
}

func origenVML(s string) string {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "page":
		return "page"
	case "margin":
		return "margin"
	case "top-margin-area":
		return "topMargin"
	case "bottom-margin-area":
		return "bottomMargin"
	case "left-margin-area":
		return "leftMargin"
	case "right-margin-area":
		return "rightMargin"
	case "text", "char":
		return "text"
	case "line":
		return "line"
	}
	return ""
}

func pxDeVML(s string) float64 {
	s = strings.TrimSpace(strings.ToLower(s))
	k := 1.0
	switch {
	case strings.HasSuffix(s, "pt"):
		s, k = strings.TrimSuffix(s, "pt"), 96.0/72.0
	case strings.HasSuffix(s, "in"):
		s, k = strings.TrimSuffix(s, "in"), 96.0
	case strings.HasSuffix(s, "cm"):
		s, k = strings.TrimSuffix(s, "cm"), 96.0/2.54
	case strings.HasSuffix(s, "mm"):
		s, k = strings.TrimSuffix(s, "mm"), 96.0/25.4
	case strings.HasSuffix(s, "pc"):
		s, k = strings.TrimSuffix(s, "pc"), 16.0
	case strings.HasSuffix(s, "px"):
		s = strings.TrimSuffix(s, "px")
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.Abs(n*k) > 1e7 {
		return 0
	}
	return n * k
}

func parVML(s string) (float64, float64) {
	a, b, ok := strings.Cut(s, ",")
	if !ok {
		return 0, 0
	}
	x, _ := strconv.ParseFloat(strings.TrimSpace(a), 64)
	y, _ := strconv.ParseFloat(strings.TrimSpace(b), 64)
	return x, y
}

// cuadroCSS es el CSS del marco de un cuadro de texto anclado de ancho anchoPx. Con enParalelo
// (dos cuadros que comparten banda vertical en columnas distintas), flota a su lado aunque su
// posición vertical venga dada respecto a la página.
func (d *docxDec) cuadroCSS(a docxAncla, anchoPx float64, enParalelo bool) string {
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
	switch {
	case a.alinea == "center":
		m.centrado = anchoPx > 0
	case !enParalelo && a.enRenglon():
		m.izq = x
		if !a.conX && derecha && texto > anchoPx {
			m.izq = texto - anchoPx
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

func (a docxAncla) enRenglon() bool {
	return a.ajuste == "wrapTopAndBottom" || a.desdeV != "" && a.desdeV != "paragraph" && a.desdeV != "line" && a.desdeV != "text"
}

// xEnPagina es la coordenada horizontal del cuadro desde el margen izquierdo del texto, en px,
// para ordenar de izquierda a derecha los cuadros que comparten renglón.
func (h *docxHoja) xEnPagina(a docxAncla, anchoPx float64) float64 {
	texto := h.anchoTexto / 15
	if texto <= 0 {
		texto = 624
	}
	switch a.alinea {
	case "center":
		return (texto - anchoPx) / 2
	case "right", "outside":
		return texto - anchoPx
	}
	x := a.x
	if a.desde == "page" || a.desde == "leftMargin" {
		x -= h.margenIzq / 15
	}
	return x
}

// yEnPagina es la coordenada vertical del cuadro desde el borde superior de la página, en px.
func (h *docxHoja) yEnPagina(a docxAncla, altoPx float64) float64 {
	altoPag := h.altoPag / 15
	if altoPag <= 0 {
		altoPag = 1056
	}
	margen := h.margenArriba / 15
	if margen <= 0 {
		margen = 96
	}
	switch a.alineaV {
	case "top", "inside":
		if a.desdeV == "margin" || a.desdeV == "topMargin" {
			return margen
		}
		return 0
	case "center":
		return (altoPag - altoPx) / 2
	case "bottom", "outside":
		if a.desdeV == "margin" || a.desdeV == "bottomMargin" {
			return altoPag - margen - altoPx
		}
		return altoPag - altoPx
	}
	y := a.y
	switch a.desdeV {
	case "margin", "topMargin":
		y += margen
	case "bottomMargin":
		y += altoPag - margen
	}
	return y
}

// ordenarCajas ordena cajas de arriba abajo por su posición en la página y, cuando están a la
// misma altura (hasta 12 px de diferencia), de izquierda a derecha; devuelve además cuáles
// comparten banda vertical en columnas distintas para que floten en paralelo.
func (d *docxDec) ordenarCajas(cajas []docxCaja) []bool {
	h := d.h()
	slices.SortStableFunc(cajas, func(a, b docxCaja) int {
		return cmp.Compare(h.yEnPagina(a.ancla, a.alto), h.yEnPagina(b.ancla, b.alto))
	})
	for i := 0; i < len(cajas); {
		y0 := h.yEnPagina(cajas[i].ancla, cajas[i].alto)
		j := i + 1
		for j < len(cajas) && h.yEnPagina(cajas[j].ancla, cajas[j].alto)-y0 <= 12 {
			j++
		}
		if j-i > 1 {
			slices.SortStableFunc(cajas[i:j], func(a, b docxCaja) int {
				return cmp.Compare(h.xEnPagina(a.ancla, a.ancho), h.xEnPagina(b.ancla, b.ancho))
			})
		}
		i = j
	}
	paralelo := make([]bool, len(cajas))
	for i := range cajas {
		if !cajas[i].anclado {
			continue
		}
		yi, hi := h.yEnPagina(cajas[i].ancla, cajas[i].alto), math.Max(cajas[i].alto, 16)
		xi, wi := h.xEnPagina(cajas[i].ancla, cajas[i].ancho), cajas[i].ancho
		for j := range cajas {
			if i == j || !cajas[j].anclado {
				continue
			}
			yj, hj := h.yEnPagina(cajas[j].ancla, cajas[j].alto), math.Max(cajas[j].alto, 16)
			xj, wj := h.xEnPagina(cajas[j].ancla, cajas[j].ancho), cajas[j].ancho
			if yi < yj+hj && yj < yi+hi && (xi+wi <= xj+8 || xj+wj <= xi+8) {
				paralelo[i] = true
				break
			}
		}
	}
	return paralelo
}

// bloquesDeCaja envuelve los bloques de una caja en su <div data-marco>.
func (d *docxDec) bloquesDeCaja(c docxCaja, enParalelo bool) []xblock {
	if len(c.bloques) == 0 {
		return nil
	}
	css := c.marco
	if c.anclado {
		css = d.cuadroCSS(c.ancla, c.ancho, enParalelo)
	}
	if css == "" {
		return c.bloques
	}
	out := make([]xblock, 0, len(c.bloques)+2)
	out = append(out, abrirMarco(css))
	out = append(out, c.bloques...)
	out = append(out, cerrarMarco)
	return out
}

// bloquesDeCajas convierte una lista de cajas en sus bloques, ordenándolas si son varias.
func (d *docxDec) bloquesDeCajas(cajas []docxCaja) []xblock {
	paralelo := d.ordenarCajas(cajas)
	out := make([]xblock, 0, len(cajas)*3)
	for i, c := range cajas {
		out = append(out, d.bloquesDeCaja(c, paralelo[i])...)
	}
	return out
}

// cerrarPagina coloca en blocks[inicio:] las cajas y marcos de la página en curso (enPag): si la
// página tiene párrafos o tablas de flujo con contenido, los del margen superior van antes del
// primero de ellos y el resto al final de la página; si la página solo tiene párrafos vacíos de
// anclaje (plantillas de maquetación de Pages), van todos en su orden vertical y horizontal.
func (d *docxDec) cerrarPagina(blocks []xblock, enPag []docxCaja, inicio int) ([]xblock, int) {
	if len(enPag) == 0 {
		return blocks, len(blocks)
	}
	paralelo := d.ordenarCajas(enPag)
	primero := -1
	for i := inicio; i < len(blocks); i++ {
		if bloqueConContenido(blocks[i]) {
			primero = i
			break
		}
	}
	if primero < 0 {
		for i, c := range enPag {
			blocks = append(blocks, d.bloquesDeCaja(c, paralelo[i])...)
		}
		return blocks, len(blocks)
	}
	h := d.h()
	umbralArriba := math.Max(h.margenArriba/15, 96)
	var arriba, abajo []xblock
	for i, c := range enPag {
		a := c.ancla
		esArriba := a.alineaV == "top" || a.alineaV == "inside" || a.desdeV == "topMargin" ||
			a.alineaV == "" && h.yEnPagina(a, c.alto) <= umbralArriba
		if esArriba {
			arriba = append(arriba, d.bloquesDeCaja(c, paralelo[i])...)
		} else {
			abajo = append(abajo, d.bloquesDeCaja(c, paralelo[i])...)
		}
	}
	if len(arriba) > 0 {
		blocks = slices.Insert(blocks, primero, arriba...)
	}
	blocks = append(blocks, abajo...)
	return blocks, len(blocks)
}

func bloqueConContenido(b xblock) bool {
	if b.table != nil || b.hr || b.raw != "" {
		return true
	}
	if b.para == nil {
		return false
	}
	for _, r := range b.para.runs {
		if r.text != "" || r.raw != "" || r.field != "" || r.image != nil || r.note != nil {
			return true
		}
	}
	return false
}

var (
	rePgSz  = regexp.MustCompile(`<(?:\w+:)?pgSz\b[^>]*>`)
	rePgMar = regexp.MustCompile(`<(?:\w+:)?pgMar\b[^>]*>`)
	reAttrW = regexp.MustCompile(`(?:\w+:)?(w|h|left|right|top)="(-?\d+(?:\.\d+)?)"`)
)

// seccionDe lee de document.xml, antes de recorrerlo, el tamaño de página y los márgenes de la
// última sección (los de todo el documento salvo saltos de sección), en twips: un cuadro anclado a
// la página se coloca desde el margen, y su sección va al final.
func seccionDe(doc []byte) (anchoPag, altoPag, izq, der, arriba float64) {
	atributos := func(tag []byte) map[string]float64 {
		out := map[string]float64{}
		for _, m := range reAttrW.FindAllSubmatch(tag, -1) {
			n, _ := strconv.ParseFloat(string(m[2]), 64)
			out[string(m[1])] = n
		}
		return out
	}
	altoPag = 15840
	if t := rePgSz.FindAll(doc, -1); len(t) > 0 {
		a := atributos(t[len(t)-1])
		anchoPag = a["w"]
		if a["h"] > 0 {
			altoPag = a["h"]
		}
	}
	izq, der, arriba = 1440, 1440, 1440
	if t := rePgMar.FindAll(doc, -1); len(t) > 0 {
		a := atributos(t[len(t)-1])
		izq, der, arriba = math.Abs(a["left"]), math.Abs(a["right"]), math.Abs(a["top"])
	}
	return anchoPag, altoPag, izq, der, arriba
}
