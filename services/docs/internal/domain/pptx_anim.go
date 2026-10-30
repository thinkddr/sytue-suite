// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/xml"
	"math"
	"strconv"
	"strings"
)

// Animaciones de diapositiva en PresentationML (`p:timing`), de ida y de vuelta.
//
// El árbol es el de ECMA-376 §19.5: una raíz, la secuencia principal y, dentro, un
// `p:par` por PASO (clic); en cada paso, un `p:par` por grupo que se encadena «tras la
// anterior» y, en cada grupo, un `p:par` por efecto, que es lo que lleva su clase
// (entrada, énfasis, salida, trayectoria), su preset y su disparador (`nodeType`). Los
// pasos y grupos salen de animSteps (slides_model.go), que reparte la secuencia como lo
// hace el editor. Cada efecto lleva los comportamientos que lo reproducen —visibilidad,
// fundido, posición, escala, giro o trayectoria— con las duraciones del CSS del
// editor, de modo que se vea igual en PowerPoint y en LibreOffice; al importar se lee el
// preset y la clase (y la trayectoria), no los comportamientos.
//
// Los medios añaden su nodo `p:video`/`p:audio` (sin él PowerPoint no los reproduce en
// la presentación), con repetición si van en bucle, y los que se reproducen solos un
// primer paso que empieza con la diapositiva.

// animFx es lo que el formato sabe de cada efecto del editor: clase, preset y duración.
type animFx struct {
	class   string // entr, emph, exit, path
	preset  int
	subtype int
	dur     int // ms
}

var animFxOf = map[string]animFx{
	"appear":      {"entr", 1, 0, 0},
	"fly":         {"entr", 2, 4, 380},
	"fade":        {"entr", 10, 0, 320},
	"zoom":        {"entr", 53, 16, 300},
	"pulse":       {"emph", 26, 0, 620},
	"spin":        {"emph", 8, 0, 700},
	"grow-shrink": {"emph", 6, 0, 560},
	"disappear":   {"exit", 1, 0, 0},
	"fly-out":     {"exit", 2, 4, 380},
	"fade-out":    {"exit", 10, 0, 300},
	"zoom-out":    {"exit", 53, 16, 300},
}

const motionDurMS = 640

// tnIDs numera los cTn del árbol (únicos en la diapositiva).
type tnIDs struct{ n int }

func (t *tnIDs) next() string { t.n++; return strconv.Itoa(t.n) }

// pptxTiming es el `p:timing` de una diapositiva; "" si no tiene animaciones ni medios.
func pptxTiming(boxes []slideBox, spids []int) string {
	steps := timedSteps(boxes, spids)
	var media, auto []int
	for i, b := range boxes {
		if b.Kind == "media" && spids[i] != 0 {
			media = append(media, i)
			if b.Autoplay {
				auto = append(auto, i)
			}
		}
	}
	if len(steps) == 0 && len(media) == 0 {
		return ""
	}
	ids := &tnIDs{n: 2} // 1 = raíz, 2 = secuencia principal
	var seq strings.Builder
	if len(auto) > 0 {
		seq.WriteString(autoplayStep(ids, auto, spids))
	}
	for _, st := range steps {
		seq.WriteString(clickStep(ids, st, boxes, spids))
	}
	var b strings.Builder
	b.WriteString(`<p:timing><p:tnLst><p:par><p:cTn id="1" dur="indefinite" restart="never" nodeType="tmRoot"><p:childTnLst>`)
	if seq.Len() > 0 {
		b.WriteString(`<p:seq concurrent="1" nextAc="seek"><p:cTn id="2" dur="indefinite" nodeType="mainSeq"><p:childTnLst>` +
			seq.String() + `</p:childTnLst></p:cTn>` +
			`<p:prevCondLst><p:cond evt="onPrev" delay="0"><p:tgtEl><p:sldTgt/></p:tgtEl></p:cond></p:prevCondLst>` +
			`<p:nextCondLst><p:cond evt="onNext" delay="0"><p:tgtEl><p:sldTgt/></p:tgtEl></p:cond></p:nextCondLst></p:seq>`)
	}
	for _, i := range media {
		b.WriteString(mediaNode(ids, boxes[i], spids[i]))
	}
	b.WriteString(`</p:childTnLst></p:cTn></p:par></p:tnLst>` + buildList(steps, boxes, spids) + `</p:timing>`)
	return b.String()
}

// timedSteps son los pasos de animSteps sin lo que no sale: efectos de cajas que no se
// exportaron y tipos que el formato no conoce.
func timedSteps(boxes []slideBox, spids []int) [][][]animRef {
	var steps [][][]animRef
	for _, st := range animSteps(boxes) {
		var grupos [][]animRef
		for _, g := range st {
			var keep []animRef
			for _, r := range g {
				if spids[r.box] != 0 && (animFxOf[r.anim.Type].class != "" || strings.HasPrefix(r.anim.Type, "motion-")) {
					keep = append(keep, r)
				}
			}
			if len(keep) > 0 {
				grupos = append(grupos, keep)
			}
		}
		if len(grupos) > 0 {
			steps = append(steps, grupos)
		}
	}
	return steps
}

// mediaNode es el nodo de un medio, sin el que PowerPoint no lo reproduce.
func mediaNode(ids *tnIDs, b slideBox, spid int) string {
	tag := "video"
	if b.MediaType == "audio" {
		tag = "audio"
	}
	rep := ""
	if b.Loop {
		rep = ` repeatCount="indefinite"`
	}
	return `<p:` + tag + `><p:cMediaNode vol="80000"><p:cTn id="` + ids.next() + `"` + rep + ` fill="hold">` +
		`<p:stCondLst><p:cond delay="indefinite"/></p:stCondLst></p:cTn>` + spTgt(spid) + `</p:cMediaNode></p:` + tag + `>`
}

// buildList dice que las formas animadas se animan como un todo (no párrafo a párrafo).
func buildList(steps [][][]animRef, boxes []slideBox, spids []int) string {
	seen := map[int]bool{}
	var bld strings.Builder
	for _, st := range steps {
		for _, g := range st {
			for _, r := range g {
				if k := boxes[r.box].Kind; (k == "" || k == "text" || k == "shape") && !seen[spids[r.box]] {
					seen[spids[r.box]] = true
					bld.WriteString(`<p:bldP spid="` + strconv.Itoa(spids[r.box]) + `" grpId="0" animBg="1"/>`)
				}
			}
		}
	}
	if bld.Len() == 0 {
		return ""
	}
	return `<p:bldLst>` + bld.String() + `</p:bldLst>`
}

func spTgt(spid int) string {
	return `<p:tgtEl><p:spTgt spid="` + strconv.Itoa(spid) + `"/></p:tgtEl>`
}

func cond(delay int) string {
	return `<p:stCondLst><p:cond delay="` + strconv.Itoa(delay) + `"/></p:stCondLst>`
}

// clickStep es un paso: sus grupos se encadenan (cada uno empieza cuando acaba el
// anterior) y los efectos de un grupo van a la vez.
func clickStep(ids *tnIDs, st [][]animRef, boxes []slideBox, spids []int) string {
	var b strings.Builder
	b.WriteString(`<p:par><p:cTn id="` + ids.next() + `" fill="hold"><p:stCondLst><p:cond delay="indefinite"/></p:stCondLst><p:childTnLst>`)
	delay := 0
	for gi, g := range st {
		b.WriteString(`<p:par><p:cTn id="` + ids.next() + `" fill="hold">` + cond(delay) + `<p:childTnLst>`)
		longest := 0
		for ei, r := range g {
			node := "withEffect"
			if ei == 0 {
				node = "afterEffect"
				if gi == 0 {
					node = "clickEffect"
				}
			}
			x, dur := effectXML(ids, r.anim, spids[r.box], node, boxes[r.box].Kind == "media")
			b.WriteString(x)
			longest = max(longest, dur)
		}
		b.WriteString(`</p:childTnLst></p:cTn></p:par>`)
		delay += longest
	}
	b.WriteString(`</p:childTnLst></p:cTn></p:par>`)
	return b.String()
}

// autoplayStep es el primer paso de una diapositiva con medios que se reproducen solos:
// empieza con la secuencia principal (cTn 2), sin clic.
func autoplayStep(ids *tnIDs, auto, spids []int) string {
	var b strings.Builder
	b.WriteString(`<p:par><p:cTn id="` + ids.next() + `" fill="hold"><p:stCondLst><p:cond delay="indefinite"/>` +
		`<p:cond evt="onBegin" delay="0"><p:tn val="2"/></p:cond></p:stCondLst><p:childTnLst>` +
		`<p:par><p:cTn id="` + ids.next() + `" fill="hold">` + cond(0) + `<p:childTnLst>`)
	for k, i := range auto {
		node := "withEffect"
		if k == 0 {
			node = "afterEffect"
		}
		b.WriteString(`<p:par><p:cTn id="` + ids.next() + `" presetID="1" presetClass="mediacall" presetSubtype="0" fill="hold" nodeType="` + node + `">` +
			cond(0) + `<p:childTnLst><p:cmd type="call" cmd="playFrom(0.0)"><p:cBhvr><p:cTn id="` + ids.next() + `" dur="1" fill="hold"/>` +
			spTgt(spids[i]) + `</p:cBhvr></p:cmd></p:childTnLst></p:cTn></p:par>`)
	}
	b.WriteString(`</p:childTnLst></p:cTn></p:par></p:childTnLst></p:cTn></p:par>`)
	return b.String()
}

// effectXML es el `p:par` de un efecto con sus comportamientos; devuelve también lo
// que dura.
func effectXML(ids *tnIDs, a slideAnim, spid int, node string, media bool) (string, int) {
	tgt := spTgt(spid)
	fx, ok := animFxOf[a.Type]
	if !ok { // trayectoria
		fx = animFx{class: "path", dur: motionDurMS}
	}
	set := func(val string, delay int) string {
		return `<p:set><p:cBhvr><p:cTn id="` + ids.next() + `" dur="1" fill="hold">` + cond(delay) + `</p:cTn>` + tgt +
			`<p:attrNameLst><p:attrName>style.visibility</p:attrName></p:attrNameLst></p:cBhvr><p:to><p:strVal val="` + val + `"/></p:to></p:set>`
	}
	fade := func(dir string) string {
		return `<p:animEffect transition="` + dir + `" filter="fade"><p:cBhvr><p:cTn id="` + ids.next() + `" dur="` + strconv.Itoa(fx.dur) + `"/>` +
			tgt + `</p:cBhvr></p:animEffect>`
	}
	prop := func(attr, from, to string) string {
		return `<p:anim calcmode="lin" valueType="num"><p:cBhvr additive="base"><p:cTn id="` + ids.next() + `" dur="` + strconv.Itoa(fx.dur) +
			`" fill="hold"/>` + tgt + `<p:attrNameLst><p:attrName>` + attr + `</p:attrName></p:attrNameLst></p:cBhvr><p:tavLst>` +
			`<p:tav tm="0"><p:val><p:strVal val="` + from + `"/></p:val></p:tav>` +
			`<p:tav tm="100000"><p:val><p:strVal val="` + to + `"/></p:val></p:tav></p:tavLst></p:anim>`
	}
	scale := func(pct int) string { // ida y vuelta
		by := strconv.Itoa(pct * 1000)
		return `<p:animScale><p:cBhvr><p:cTn id="` + ids.next() + `" dur="` + strconv.Itoa(fx.dur/2) + `" autoRev="1" fill="hold"/>` + tgt +
			`</p:cBhvr><p:by x="` + by + `" y="` + by + `"/></p:animScale>`
	}
	var beh string
	switch a.Type {
	case "appear":
		beh = set("visible", 0)
	case "fade":
		beh = set("visible", 0) + fade("in")
	case "fly":
		beh = set("visible", 0) + prop("ppt_y", "#ppt_y+0.05", "#ppt_y") + fade("in")
	case "zoom":
		beh = set("visible", 0) + prop("ppt_w", "#ppt_w*0.82", "#ppt_w") + prop("ppt_h", "#ppt_h*0.82", "#ppt_h") + fade("in")
	case "pulse":
		beh = scale(106)
	case "grow-shrink":
		beh = scale(122)
	case "spin":
		beh = `<p:animRot by="21600000"><p:cBhvr><p:cTn id="` + ids.next() + `" dur="` + strconv.Itoa(fx.dur) + `" fill="hold"/>` + tgt +
			`<p:attrNameLst><p:attrName>r</p:attrName></p:attrNameLst></p:cBhvr></p:animRot>`
	case "disappear":
		beh = set("hidden", 0)
	case "fade-out":
		beh = fade("out") + set("hidden", fx.dur-1)
	case "fly-out":
		beh = prop("ppt_y", "#ppt_y", "#ppt_y+0.05") + fade("out") + set("hidden", fx.dur-1)
	case "zoom-out":
		beh = prop("ppt_w", "#ppt_w", "#ppt_w*0.82") + prop("ppt_h", "#ppt_h", "#ppt_h*0.82") + fade("out") + set("hidden", fx.dur-1)
	default:
		beh = `<p:animMotion origin="layout" path="` + motionPath(motionPointsOf(a)) + `" pathEditMode="relative">` +
			`<p:cBhvr><p:cTn id="` + ids.next() + `" dur="` + strconv.Itoa(fx.dur) + `" fill="hold"/>` + tgt +
			`<p:attrNameLst><p:attrName>ppt_x</p:attrName><p:attrName>ppt_y</p:attrName></p:attrNameLst></p:cBhvr></p:animMotion>`
	}
	grp := ` grpId="0"`
	if media {
		grp = ""
	}
	return `<p:par><p:cTn id="` + ids.next() + `" presetID="` + strconv.Itoa(fx.preset) + `" presetClass="` + fx.class +
		`" presetSubtype="` + strconv.Itoa(fx.subtype) + `" fill="hold"` + grp + ` nodeType="` + node + `">` + cond(0) +
		`<p:childTnLst>` + beh + `</p:childTnLst></p:cTn></p:par>`, fx.dur
}

// motionPath es la trayectoria en la notación del formato: fracciones del ancho y del
// alto de la diapositiva, relativas a la posición de la caja.
func motionPath(pts []motionPoint) string {
	var b strings.Builder
	for i, p := range pts {
		if i == 0 {
			b.WriteString("M ")
		} else {
			b.WriteString(" L ")
		}
		b.WriteString(fmtFrac(p.DX/100) + " " + fmtFrac(p.DY/100))
	}
	b.WriteString(" E")
	return b.String()
}

func fmtFrac(v float64) string {
	return strconv.FormatFloat(math.Round(v*1e5)/1e5, 'f', -1, 64)
}

// --- Importación ---

// xnode es un elemento XML genérico (nombre, atributos e hijos): el árbol de tiempos
// es recursivo y de muchos tipos de nodo, y recorrerlo así es más corto que modelarlo.
type xnode struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Text    string     `xml:",chardata"`
	Kids    []xnode    `xml:",any"`
}

func (n *xnode) attr(local string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// attrNS es el atributo `local` del espacio de nombres `ns` (los de relación, `r:id`, se
// llaman igual que otros atributos del elemento).
func (n *xnode) attrNS(ns, local string) string {
	for _, a := range n.Attrs {
		if a.Name.Space == ns && a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

func (n *xnode) kid(local string) *xnode {
	for i := range n.Kids {
		if n.Kids[i].XMLName.Local == local {
			return &n.Kids[i]
		}
	}
	return nil
}

// kids son los hijos de `local` > los nietos (p.ej. childTnLst > par).
func (n *xnode) kids(local string) []xnode {
	if k := n.kid(local); k != nil {
		return k.Kids
	}
	return nil
}

// first es el primer descendiente `local` (en orden de documento), o nil.
func (n *xnode) first(local string) *xnode {
	for i := range n.Kids {
		if n.Kids[i].XMLName.Local == local {
			return &n.Kids[i]
		}
		if f := n.Kids[i].first(local); f != nil {
			return f
		}
	}
	return nil
}

// each recorre todos los descendientes `local`.
func (n *xnode) each(local string, fn func(*xnode)) {
	for i := range n.Kids {
		if n.Kids[i].XMLName.Local == local {
			fn(&n.Kids[i])
		}
		n.Kids[i].each(local, fn)
	}
}

// readTiming aplica a las cajas de la diapositiva (`spid` → índice de caja) las
// animaciones de la secuencia principal, el bucle de sus medios y su reproducción
// automática. Lo que no es de una caja conocida (secuencias interactivas, efectos de
// párrafo de formas que no se importaron) se ignora.
func readTiming(t *xnode, spidBox map[string]int, boxes []slideBox) {
	order := 0
	t.each("cTn", func(main *xnode) {
		if main.attr("nodeType") != "mainSeq" {
			return
		}
		steps := main.kids("childTnLst")
		for i := range steps {
			readTimingStep(steps[i].kid("cTn"), spidBox, boxes, &order)
		}
	})
	t.each("cMediaNode", func(m *xnode) {
		tgt, tn := m.first("spTgt"), m.kid("cTn")
		if tgt == nil || tn == nil {
			return
		}
		if bi, ok := spidBox[tgt.attr("spid")]; ok && boxes[bi].Kind == "media" && tn.attr("repeatCount") == "indefinite" {
			boxes[bi].Loop = true
		}
	})
}

// readTimingStep lee un paso (un clic) de la secuencia principal: sus grupos y, en
// cada uno, sus efectos. Un paso que empieza con la diapositiva (`onBegin`) es el de
// los medios que se reproducen solos.
func readTimingStep(stepTn *xnode, spidBox map[string]int, boxes []slideBox, order *int) {
	if stepTn == nil {
		return
	}
	auto := false
	for _, c := range stepTn.kids("stCondLst") {
		auto = auto || c.attr("evt") == "onBegin"
	}
	grupos := stepTn.kids("childTnLst")
	for gi := range grupos {
		grpTn := grupos[gi].kid("cTn")
		if grpTn == nil {
			continue
		}
		efectos := grpTn.kids("childTnLst")
		for ei := range efectos {
			readTimingEffect(efectos[ei].kid("cTn"), gi, ei, auto, spidBox, boxes, order)
		}
	}
}

func readTimingEffect(e *xnode, gi, ei int, auto bool, spidBox map[string]int, boxes []slideBox, order *int) {
	if e == nil {
		return
	}
	tgt := e.first("spTgt")
	if tgt == nil {
		return
	}
	bi, ok := spidBox[tgt.attr("spid")]
	if !ok {
		return
	}
	if e.attr("presetClass") == "mediacall" {
		if auto && boxes[bi].Kind == "media" {
			boxes[bi].Autoplay = true
		}
		return
	}
	a, ok := animFrom(e)
	if !ok {
		return
	}
	*order++
	a.Order = *order
	switch node := e.attr("nodeType"); {
	case node == "withEffect" || (node == "" && ei > 0):
		a.Trigger = "with"
	case node == "afterEffect" || (node == "" && gi > 0):
		a.Trigger = "after"
	}
	boxes[bi].Anims = append(boxes[bi].Anims, a)
}

// animFrom es la animación del editor de un efecto: por su clase y su preset; uno que
// el editor no tiene cae en el más parecido de su clase (una entrada «barrido» es un
// fundido), y una trayectoria se reconoce por su geometría o queda como propia.
func animFrom(e *xnode) (slideAnim, bool) {
	class := e.attr("presetClass")
	preset, _ := strconv.Atoi(e.attr("presetID"))
	if class == "path" {
		m := e.first("animMotion")
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
	for typ, fx := range animFxOf {
		if fx.class == class && fx.preset == preset {
			return slideAnim{Type: typ}, true
		}
	}
	switch class {
	case "entr":
		return slideAnim{Type: "fade"}, true
	case "exit":
		return slideAnim{Type: "fade-out"}, true
	case "emph":
		return slideAnim{Type: "pulse"}, true
	}
	return slideAnim{}, false
}

// parseMotionPath lee una trayectoria del formato (M/L/C… en fracciones de la
// diapositiva) como desfases en % desde su primer punto. De las curvas solo toma el
// punto final de cada tramo: el editor dibuja polilíneas.
func parseMotionPath(d string) []motionPoint {
	f := strings.Fields(strings.NewReplacer(",", " ").Replace(d))
	var pts []motionPoint
	var nums []float64
	flush := func(cmd string) {
		if (cmd == "M" || cmd == "L" || cmd == "C") && len(nums) >= 2 {
			pts = append(pts, motionPoint{DX: nums[len(nums)-2] * 100, DY: nums[len(nums)-1] * 100})
		}
		nums = nums[:0]
	}
	cmd := ""
	for _, tok := range f {
		if v, err := strconv.ParseFloat(tok, 64); err == nil {
			nums = append(nums, v)
			continue
		}
		flush(cmd)
		cmd = strings.ToUpper(tok)
	}
	flush(cmd)
	if len(pts) == 0 {
		return nil
	}
	o := pts[0]
	for i := range pts {
		pts[i] = motionPoint{DX: math.Round((pts[i].DX-o.DX)*100) / 100, DY: math.Round((pts[i].DY-o.DY)*100) / 100}
	}
	return pts
}
