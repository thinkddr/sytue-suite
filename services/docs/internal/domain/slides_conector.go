// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"math"
	"strconv"
)

// Conectores de diapositiva en DrawingML (`p:cxnSp`), de ida y de vuelta.
//
// La geometría es la de slides/connector.ts: cada extremo anclado a una caja sale por
// el centro de un lado o, en 'auto', por donde el rayo del centro de su caja hacia el
// otro extremo corta el borde; uno libre es su punto. El formato guarda el conector
// como una forma de su recuadro con giro y volteos —el codo va primero en horizontal
// en su marco, así que uno que empieza en vertical va girado 90°—, y el anclaje como
// `stCxn`/`endCxn` al sitio de conexión de la forma, que es lo que PowerPoint usa para
// seguirla cuando se mueve y LibreOffice para dibujarlo.
//
// Los sitios dependen de la geometría (los del rectángulo no son los del hexágono:
// visto en LibreOffice, el «0» de un hexágono es su vértice derecho). Solo se ancla a
// las formas que tienen un sitio en el centro de cada lado, que es donde el editor
// ancla: rectángulos (y cajas de texto, imágenes y medios), rombos y elipses.
// ponytail: a las demás el conector va suelto, con su trazado tal cual; anclarlas
// pide la tabla de sitios de cada geometría del formato.

type cpt struct{ x, y float64 }

type crect struct{ x, y, w, h float64 }

func (r crect) center() cpt { return cpt{r.x + r.w/2, r.y + r.h/2} }

// Lados en el orden de los sitios de conexión de un rectángulo.
var cxnSides = [4]string{"top", "left", "bottom", "right"}

// cxnSites es, por geometría, el índice del sitio de conexión en el centro de cada
// lado (arriba, izquierda, abajo, derecha); nil = no se ancla. Una elipse tiene ocho
// sitios, empezando arriba y en sentido antihorario.
func cxnSites(b slideBox) []int {
	if b.Kind != "shape" {
		return []int{0, 1, 2, 3}
	}
	switch b.Shape {
	case "", "rect", "diamond":
		return []int{0, 1, 2, 3}
	case "ellipse":
		return []int{0, 2, 4, 6}
	}
	return nil
}

func sideMid(r crect, side string) cpt {
	switch side {
	case "top":
		return cpt{r.x + r.w/2, r.y}
	case "bottom":
		return cpt{r.x + r.w/2, r.y + r.h}
	case "left":
		return cpt{r.x, r.y + r.h/2}
	case "right":
		return cpt{r.x + r.w, r.y + r.h/2}
	}
	return r.center()
}

// rayExit es la salida por el borde del rayo desde el centro hacia `to`.
func rayExit(r crect, to cpt) cpt {
	c := r.center()
	dx, dy := to.x-c.x, to.y-c.y
	if dx == 0 && dy == 0 {
		return cpt{r.x + r.w, c.y}
	}
	tx, ty := math.Inf(1), math.Inf(1)
	if dx != 0 {
		tx = r.w / 2 / math.Abs(dx)
	}
	if dy != 0 {
		ty = r.h / 2 / math.Abs(dy)
	}
	t := math.Min(tx, ty)
	return cpt{c.x + dx*t, c.y + dy*t}
}

// sideOf es el lado del rectángulo en el que cae un punto de su borde.
func sideOf(r crect, p cpt) string {
	d := []float64{math.Abs(p.y - r.y), math.Abs(p.x - r.x), math.Abs(p.y - (r.y + r.h)), math.Abs(p.x - (r.x + r.w))}
	best := 0
	for i := range d {
		if d[i] < d[best] {
			best = i
		}
	}
	return cxnSides[best]
}

// connEnd es un extremo resuelto: la caja (y su id de forma) o el punto libre.
type connEnd struct {
	rect   *crect
	anchor string
	spid   int
	sites  []int
	pt     cpt
}

func (e connEnd) target() cpt {
	if e.rect != nil {
		return e.rect.center()
	}
	return e.pt
}

func (e connEnd) at(to cpt) cpt {
	if e.rect == nil {
		return e.pt
	}
	if e.anchor == "" || e.anchor == "auto" {
		return rayExit(*e.rect, to)
	}
	return sideMid(*e.rect, e.anchor)
}

// connectorsXML escribe los conectores de la diapositiva, con ids de forma desde `next`.
func (pb *pptxBuild) connectorsXML(sl slideT, spids []int, next int) string {
	w, h := float64(pb.w), float64(pb.h)
	byID := map[string]int{}
	for i, b := range sl.Boxes {
		if b.ID != "" {
			byID[b.ID] = i
		}
	}
	resolve := func(e connectorEnd) connEnd {
		if i, ok := byID[e.BoxID]; ok && spids[i] != 0 {
			b := sl.Boxes[i]
			return connEnd{rect: &crect{b.X / 100 * w, b.Y / 100 * h, b.W / 100 * w, b.H / 100 * h}, anchor: e.Anchor, spid: spids[i], sites: cxnSites(b)}
		}
		var x, y float64
		if e.X != nil {
			x = *e.X
		}
		if e.Y != nil {
			y = *e.Y
		}
		return connEnd{pt: cpt{x / 100 * w, y / 100 * h}}
	}
	out := ""
	for _, c := range sl.Connectors {
		from, to := resolve(c.From), resolve(c.To)
		a, b := from.at(to.target()), to.at(from.target())
		out += pptxConnector(next, c, from, to, a, b)
		next++
	}
	return out
}

func pptxConnector(id int, c slideConnector, from, to connEnd, a, b cpt) string {
	prst := "straightConnector1"
	switch c.Kind {
	case "elbow":
		prst = "bentConnector3"
	case "curved":
		prst = "curvedConnector3"
	}
	dx, dy := b.x-a.x, b.y-a.y
	// El codo y la curva salen en horizontal en su marco: si el trazado empieza en
	// vertical, el marco va girado 90° (y entonces el ancho del marco es el alto).
	vertical := c.Kind != "straight" && c.Kind != "" && math.Abs(dy) > math.Abs(dx)
	cx, cy := (a.x+b.x)/2, (a.y+b.y)/2
	var ext cpt
	var rot string
	var flipH, flipV bool
	if vertical {
		ext = cpt{math.Abs(dy), math.Abs(dx)}
		rot = ` rot="5400000"`
		// Girado 90° en sentido horario, +x del marco baja y +y va a la izquierda.
		flipH = dy < 0
		flipV = dx > 0
	} else {
		ext = cpt{math.Abs(dx), math.Abs(dy)}
		flipH = dx < 0
		flipV = dy < 0
	}
	attrs := rot
	if flipH {
		attrs += ` flipH="1"`
	}
	if flipV {
		attrs += ` flipV="1"`
	}
	cxn := ""
	if from.rect != nil && from.sites != nil {
		cxn += `<a:stCxn id="` + strconv.Itoa(from.spid) + `" idx="` + cxnIdx(*from.rect, a, from.sites) + `"/>`
	}
	if to.rect != nil && to.sites != nil {
		cxn += `<a:endCxn id="` + strconv.Itoa(to.spid) + `" idx="` + cxnIdx(*to.rect, b, to.sites) + `"/>`
	}
	color := `<a:schemeClr val="tx1"/>` // la tinta del tema, como currentColor en el editor
	if v := normalizeColor(c.Stroke); v != "" {
		color = `<a:srgbClr val="` + v + `"/>`
	}
	ends := ""
	if c.StartArrow {
		ends += `<a:headEnd type="triangle"/>`
	}
	if c.EndArrow {
		ends += `<a:tailEnd type="triangle"/>`
	}
	sid := strconv.Itoa(id)
	r := func(v float64) string { return strconv.Itoa(int(math.Round(v))) }
	return `<p:cxnSp><p:nvCxnSpPr><p:cNvPr id="` + sid + `" name="Connector ` + sid + `"/><p:cNvCxnSpPr>` + cxn + `</p:cNvCxnSpPr><p:nvPr/></p:nvCxnSpPr>` +
		`<p:spPr><a:xfrm` + attrs + `><a:off x="` + r(cx-ext.x/2) + `" y="` + r(cy-ext.y/2) + `"/><a:ext cx="` + r(ext.x) + `" cy="` + r(ext.y) + `"/></a:xfrm>` +
		`<a:prstGeom prst="` + prst + `"><a:avLst/></a:prstGeom><a:ln w="19050"><a:solidFill>` + color + `</a:solidFill>` + ends + `</a:ln></p:spPr></p:cxnSp>`
}

// cxnIdx es el sitio de conexión del lado en el que cae el extremo.
func cxnIdx(r crect, p cpt, sites []int) string {
	s := sideOf(r, p)
	for i, side := range cxnSides {
		if side == s {
			return strconv.Itoa(sites[i])
		}
	}
	return strconv.Itoa(sites[0])
}

// cxnEnds son los extremos (inicio y fin) de un conector a partir de su recuadro, su
// giro (en 60000-ésimas de grado) y sus volteos: el inverso de pptxConnector.
func cxnEnds(off, ext cpt, rot float64, flipH, flipV bool) (cpt, cpt) {
	c := cpt{off.x + ext.x/2, off.y + ext.y/2}
	th := rot / 60000 * math.Pi / 180
	place := func(lx, ly float64) cpt {
		if flipH {
			lx = -lx
		}
		if flipV {
			ly = -ly
		}
		return cpt{c.x + lx*math.Cos(th) - ly*math.Sin(th), c.y + lx*math.Sin(th) + ly*math.Cos(th)}
	}
	return place(-ext.x/2, -ext.y/2), place(ext.x/2, ext.y/2)
}

// cxnAnchor es el lado de una caja al que va un extremo importado: el de su sitio de
// conexión si el extremo está en el centro de ese lado; si no, 'auto' (vacío), que es
// lo que escribe el editor cuando el anclaje es automático.
func cxnAnchor(b slideBox, r crect, idx string, p cpt, tol float64) string {
	i, err := strconv.Atoi(idx)
	if err != nil {
		return ""
	}
	for k, site := range cxnSites(b) {
		if site == i {
			if m := sideMid(r, cxnSides[k]); math.Hypot(m.x-p.x, m.y-p.y) <= tol {
				return cxnSides[k]
			}
		}
	}
	return ""
}
