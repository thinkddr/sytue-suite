// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/xml"
	"math"
	"strconv"
	"strings"
)

// Animaciones y conectores de un .odp: el reverso de odpTiming y de odpBuild.connectors
// (export_odp.go). Comparten la geometría de los conectores con el .pptx
// (slides_conector.go) y el árbol de tiempos con él (pptx_anim.go): aquí solo cambian
// los nombres de ODF.

// odpTypeOfID es el efecto del editor de cada preset de ODF.
var odpTypeOfID = func() map[string]string {
	m := make(map[string]string, len(odpFxOf))
	for typ, fx := range odpFxOf {
		m[fx.id] = typ
	}
	return m
}()

// readOdpTiming aplica a las cajas de la diapositiva (`draw:id` → índice de caja) las
// animaciones de la secuencia principal y la reproducción automática de sus medios. Lo
// que no apunta a una caja conocida (secuencias interactivas, objetos que no se
// importaron) se ignora.
func readOdpTiming(root *xnode, ids map[string]int, boxes []slideBox) {
	order := 0
	root.each("seq", func(main *xnode) {
		if main.attr("node-type") != "main-sequence" {
			return
		}
		for i := range main.Kids {
			if main.Kids[i].XMLName.Local == "par" {
				readOdpStep(&main.Kids[i], ids, boxes, &order)
			}
		}
	})
}

// readOdpStep lee un paso (un clic) de la secuencia principal: sus grupos y, en cada
// uno, sus efectos. Un paso que no espera al clic (`begin` distinto de «next») es el de
// los medios que se reproducen solos.
func readOdpStep(step *xnode, ids map[string]int, boxes []slideBox, order *int) {
	begin := step.attr("begin")
	auto := begin != "" && begin != "next" && begin != "indefinite"
	gi := 0
	for g := range step.Kids {
		if step.Kids[g].XMLName.Local != "par" {
			continue
		}
		ei := 0
		for e := range step.Kids[g].Kids {
			if step.Kids[g].Kids[e].XMLName.Local != "par" {
				continue
			}
			readOdpEffect(&step.Kids[g].Kids[e], gi, ei, auto, ids, boxes, order)
			ei++
		}
		gi++
	}
}

func readOdpEffect(e *xnode, gi, ei int, auto bool, ids map[string]int, boxes []slideBox, order *int) {
	bi, ok := ids[targetOf(e)]
	if !ok {
		return
	}
	if e.attr("preset-class") == "media-call" {
		if auto && boxes[bi].Kind == "media" {
			boxes[bi].Autoplay = true
		}
		return
	}
	a, ok := odpAnimFrom(e)
	if !ok {
		return
	}
	*order++
	a.Order = *order
	switch node := e.attr("node-type"); {
	case node == "with-previous" || (node == "" && ei > 0):
		a.Trigger = "with"
	case node == "after-previous" || (node == "" && gi > 0):
		a.Trigger = "after"
	}
	boxes[bi].Anims = append(boxes[bi].Anims, a)
}

// targetOf es el objeto al que apunta el efecto: el `targetElement` del primer
// comportamiento que lo lleve.
func targetOf(n *xnode) string {
	if t := n.attr("targetElement"); t != "" {
		return t
	}
	for i := range n.Kids {
		if t := targetOf(&n.Kids[i]); t != "" {
			return t
		}
	}
	return ""
}

// odpAnimFrom es la animación del editor de un efecto: por su preset; uno que el editor
// no tiene cae en el más parecido de su clase, y una trayectoria se reconoce por su
// geometría o queda como propia.
func odpAnimFrom(e *xnode) (slideAnim, bool) {
	class, id := e.attr("preset-class"), e.attr("preset-id")
	if class == "motion-path" {
		m := e.first("animateMotion")
		if m == nil {
			return slideAnim{}, false
		}
		pts := parseMotionPath(m.attr("path"))
		if len(pts) < 2 {
			return slideAnim{}, false
		}
		if typ := motionTypeFor(pts); typ != "" {
			return slideAnim{Type: typ}, true
		}
		return slideAnim{Type: "motion-custom", Points: pts}, true
	}
	if typ, ok := odpTypeOfID[id]; ok {
		return slideAnim{Type: typ}, true
	}
	has := func(s string) bool { return strings.Contains(id, s) }
	switch class {
	case "entrance":
		switch {
		case has("appear"):
			return slideAnim{Type: "appear"}, true
		case has("fly"):
			return slideAnim{Type: "fly"}, true
		case has("zoom"):
			return slideAnim{Type: "zoom"}, true
		}
		return slideAnim{Type: "fade"}, true
	case "exit":
		switch {
		case has("disappear"):
			return slideAnim{Type: "disappear"}, true
		case has("fly"):
			return slideAnim{Type: "fly-out"}, true
		case has("zoom"):
			return slideAnim{Type: "zoom-out"}, true
		}
		return slideAnim{Type: "fade-out"}, true
	case "emphasis":
		switch {
		case has("spin"):
			return slideAnim{Type: "spin"}, true
		case has("grow"):
			return slideAnim{Type: "grow-shrink"}, true
		}
		return slideAnim{Type: "pulse"}, true
	}
	return slideAnim{}, false
}

// --- Conectores ---

// odpConn es un conector leído, a falta de resolver a qué cajas se ancla.
type odpConn struct {
	c                  slideConnector
	startID, endID     string
	startGlue, endGlue string
	a, b               cpt // extremos, en cm
}

// odpLado es el lado de cada punto de unión por defecto de un objeto de ODF (el inverso
// de odpPuntoUnion).
var odpLado = map[string]string{"0": "top", "1": "right", "2": "bottom", "3": "left"}

// readODPConnector lee un `draw:connector`: tipo, color y puntas y sus extremos (las
// formas a las que se ancla, con su punto de unión, y los puntos de su recuadro).
func (d *odfDec) readODPConnector(se xml.StartElement) odpConn {
	st := d.look.resolver(attrLocal(se, "style-name"))
	k := odpConn{
		startID: attrLocal(se, "start-shape"), endID: attrLocal(se, "end-shape"),
		startGlue: attrLocal(se, "start-glue-point"), endGlue: attrLocal(se, "end-glue-point"),
		a: cpt{parseLenCm(attrLocal(se, "x1")), parseLenCm(attrLocal(se, "y1"))},
		b: cpt{parseLenCm(attrLocal(se, "x2")), parseLenCm(attrLocal(se, "y2"))},
	}
	switch attrLocal(se, "type") {
	case "line":
		k.c.Kind = "straight"
	case "curve":
		k.c.Kind = "curved"
	default: // «standard» es el valor por defecto de ODF: líneas en ángulo recto
		k.c.Kind = "elbow"
	}
	k.c.StartArrow, k.c.EndArrow = st.markStart != "", st.markEnd != ""
	if c := normalizeColor(st.stroke); c != "" && st.strokeType != "none" && !strings.EqualFold(c, d.ink) {
		k.c.Stroke = "#" + c
	}
	_ = d.dec.Skip()
	return k
}

// resolveODPConnectors anexa a la diapositiva sus conectores ya anclados: a la caja por
// su `draw:id` y, con el lado, si el extremo cae en el punto medio de ese lado (si no,
// el conector es «automático» y se queda sin lado).
func (d *odfDec) resolveODPConnectors(sl *slideT, raw []odpConn) {
	tol := math.Max(d.paginaW, d.paginaH) / 100 // un 1 %: lo que redondean los % del modelo
	end := func(id, glue string, p cpt) connectorEnd {
		if i, ok := d.ids[id]; ok && id != "" {
			b := sl.Boxes[i]
			r := crect{b.X / 100 * d.paginaW, b.Y / 100 * d.paginaH, b.W / 100 * d.paginaW, b.H / 100 * d.paginaH}
			anchor := ""
			if side := odpLado[glue]; side != "" {
				if m := sideMid(r, side); math.Hypot(m.x-p.x, m.y-p.y) <= tol {
					anchor = side
				}
			}
			return connectorEnd{BoxID: b.ID, Anchor: anchor}
		}
		x := math.Min(100, math.Max(0, roundPct(p.x/d.paginaW*100)))
		y := math.Min(100, math.Max(0, roundPct(p.y/d.paginaH*100)))
		return connectorEnd{X: &x, Y: &y}
	}
	for i, k := range raw {
		k.c.ID = sl.ID + "c" + strconv.Itoa(i+1)
		k.c.From, k.c.To = end(k.startID, k.startGlue, k.a), end(k.endID, k.endGlue, k.b)
		sl.Connectors = append(sl.Connectors, k.c)
	}
}
