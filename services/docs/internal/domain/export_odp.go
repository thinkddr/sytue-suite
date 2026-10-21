// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"cmp"
	"math"
	"strconv"
	"strings"
)

// Exportador .odp (OASIS ODF 1.3) del modelo doc:slides: la misma fidelidad que el
// .pptx de export_slides.go, escrita contra ODF y no traducida de OOXML.
//
//   - Tema: el fondo, la tinta y la fuente viven en el estilo «standard» y en el estilo
//     de página de la maestra (ODF no tiene objeto «tema»); al importar se deducen de
//     ahí (themeIDFor).
//   - Patrón: las cajas del patrón del editor son los objetos de la página maestra, con
//     el número de diapositiva y el total como campos (`text:page-number`/`text:page-count`).
//   - Fondo propio, ocultar el patrón y transición: el estilo `drawing-page` de cada
//     diapositiva. El morph no existe en ODF: sale como fundido y se anota con un
//     atributo propio (`sytue:transition`), que cualquier otro lector ignora.
//   - Animaciones: el árbol `anim:par` de ODF, con los mismos pasos y grupos que
//     animSteps y los presets `ooo-*` que LibreOffice reconoce.
//   - Medios: `draw:plugin` con su póster, enlazado o incrustado en `Media/`.
//   - Conectores: `draw:connector`, anclados a la forma por su punto de unión.
//
// El lienzo mide lo mismo que el del .pptx (13,333 in de ancho): 1 px del escenario del
// editor es 1 pt y los tamaños de letra no dependen de la proporción.

// Espacio de nombres propio para lo que ODF no sabe expresar.
const odpNSSytue = "urn:sytue:names:odf:1.0"

// odpPuntoUnion es el punto de unión por defecto de un objeto ODF en cada lado
// (arriba, derecha, abajo, izquierda).
var odpPuntoUnion = map[string]string{"top": "0", "right": "1", "bottom": "2", "left": "3"}

// odpShapeTypes son las geometrías ODF de las formas que no son ni rectángulo, ni
// elipse ni línea.
var odpShapeTypes = map[string]string{
	"arrow": "right-arrow", "triangle": "isosceles-triangle", "diamond": "diamond",
	"pentagon": "pentagon", "hexagon": "hexagon", "star": "star5",
	"callout": "rectangular-callout",
}

// odpCylinder es el cilindro con su trazado propio (LibreOffice no trae uno por nombre):
// el cuerpo con la tapa y el borde delantero de la tapa, sobre un lienzo de 21600.
const odpCylinder = `draw:type="non-primitive" draw:text-areas="0 7200 21600 18000" draw:enhanced-path="` +
	`M 0 3600 C 0 -1200 21600 -1200 21600 3600 L 21600 18000 C 21600 22800 0 22800 0 18000 Z N ` +
	`F M 0 3600 C 0 8400 21600 8400 21600 3600 N"`

type odpFx struct{ class, id, sub string }

// odpFxOf son los presets de ODF de cada efecto del editor (los de animFxOf).
var odpFxOf = map[string]odpFx{
	"appear":      {"entrance", "ooo-entrance-appear", ""},
	"fade":        {"entrance", "ooo-entrance-fade-in", ""},
	"fly":         {"entrance", "ooo-entrance-fly-in", "from-bottom"},
	"zoom":        {"entrance", "ooo-entrance-fade-in-and-zoom", "in"},
	"pulse":       {"emphasis", "ooo-emphasis-flash-bulb", ""},
	"spin":        {"emphasis", "ooo-emphasis-spin", ""},
	"grow-shrink": {"emphasis", "ooo-emphasis-grow-and-shrink", ""},
	"disappear":   {"exit", "ooo-exit-disappear", ""},
	"fade-out":    {"exit", "ooo-exit-fade-out", ""},
	"fly-out":     {"exit", "ooo-exit-fly-out", "from-bottom"},
	"zoom-out":    {"exit", "ooo-exit-fade-out-and-zoom", "in"},
}

// odpBuild lleva el estado de una exportación: lienzo, medios y numeración.
type odpBuild struct {
	model         slidesModel
	th            slideTheme
	font          string
	pw, ph        float64 // página, en cm
	media         map[string][]byte
	imgN, medN    int
	idN, nG, nH   int
	nD            int
	subN          int // subtítulos (.vtt) escritos
	layouts       []slideLayout
	maestras      map[string]string // id de diseño → su página maestra, si tiene formas propias
	total         int
	posterWritten bool
	headSty       map[string]string // firma de título → estilo, por fichero
}

func cmStr(v float64) string {
	return strconv.FormatFloat(math.Round(v*1000)/1000, 'f', -1, 64) + "cm"
}

func (b *odpBuild) pos(box slideBox) string {
	return `svg:x="` + cmStr(box.X/100*b.pw) + `" svg:y="` + cmStr(box.Y/100*b.ph) +
		`" svg:width="` + cmStr(box.W/100*b.pw) + `" svg:height="` + cmStr(box.H/100*b.ph) + `"`
}

func odpID(n int) string { return "id" + strconv.Itoa(n) }

func odpIDAttrs(n int) string { return `xml:id="` + odpID(n) + `" draw:id="` + odpID(n) + `"` }

// odpPackage devuelve los estilos automáticos y el cuerpo de content.xml, el styles.xml
// completo y los binarios incrustados.
func odpPackage(title string, s *State) (styles, body, stylesDoc string, media map[string][]byte) {
	m := slidesModelOf(title, s)
	b := &odpBuild{model: m, th: m.theme(), media: map[string][]byte{}, total: len(m.Slides)}
	b.font = b.th.Font
	if b.font == "" {
		b.font = pptxFontDefault
	}
	wEMU, hEMU := m.slideEMU()
	b.pw, b.ph = float64(wEMU)/360000, float64(hEMU)/360000
	if m.Master != nil {
		b.layouts = disenosValidos(m.Master.Layouts)
	}

	// Maestra: patrón del editor. Sus estilos van en styles.xml.
	mw := newODTWriter()
	b.headSty = map[string]string{}
	var mst strings.Builder
	var masterBoxes []slideBox
	if m.Master != nil {
		masterBoxes = m.Master.Boxes
	}
	masterShapes, _ := b.boxes(masterBoxes, mw, &mst, true)
	// Un diseño con formas propias (o que oculta el patrón) es en ODF otra página maestra: las
	// cajas del patrón y, encima, las suyas, en un grupo marcado que la importación devuelve
	// al diseño. ODF no tiene diseños con formas: sus `presentation-page-layout` solo llevan
	// marcadores.
	var maestras strings.Builder
	b.maestras = map[string]string{}
	for _, l := range b.layouts {
		if len(l.Decor) == 0 && len(l.Connectors) == 0 && !l.HideMaster {
			continue
		}
		deco, ids := b.boxes(l.Decor, mw, &mst, true)
		deco += b.connectors(slideT{Boxes: l.Decor, Connectors: l.Connectors}, ids, &mst)
		base := masterShapes
		oculta := ""
		if l.HideMaster {
			base, oculta = "", ` sytue:hideMaster="true"`
		}
		nombre := "M_" + l.ID
		b.maestras[l.ID] = nombre
		maestras.WriteString(`<style:master-page style:name="` + nombre + `" style:page-layout-name="PL1" draw:style-name="Mdp1" sytue:layout="` +
			xmlEscape(l.ID) + `"` + oculta + `>` + base + `<draw:g sytue:decor="true">` + deco + `</draw:g></style:master-page>`)
	}
	stylesDoc = b.stylesXML(mw.styles.String()+mst.String(), masterShapes, maestras.String())

	// Cuerpo: una página por diapositiva.
	w := newODTWriter()
	b.headSty = map[string]string{}
	var st strings.Builder
	st.WriteString(odpFrameStyle + odpListStyles(b.font))
	var out strings.Builder
	out.WriteString(`<office:presentation>`)
	for i, sl := range m.Slides {
		dp := b.pageStyle(&st, sl)
		maestra := cmp.Or(b.maestras[sl.Layout], "Default")
		out.WriteString(`<draw:page draw:name="Diapositiva ` + strconv.Itoa(i+1) + `" draw:master-page-name="` + xmlEscape(maestra) + `"` + b.layoutAttr(sl.Layout))
		if dp != "" {
			out.WriteString(` draw:style-name="` + dp + `"`)
		}
		out.WriteString(`>`)
		shapes, ids := b.boxes(sl.Boxes, w, &st, false)
		out.WriteString(shapes)
		out.WriteString(b.connectors(sl, ids, &st))
		out.WriteString(odpTiming(sl.Boxes, ids))
		if strings.TrimSpace(sl.Notes) != "" {
			out.WriteString(`<presentation:notes><draw:frame presentation:class="notes" svg:x="2.1cm" svg:y="14.107cm" ` +
				`svg:width="16.799cm" svg:height="13.364cm"><draw:text-box>`)
			for _, line := range splitParagraphs(sl.Notes) {
				out.WriteString(`<text:p>` + xmlEscape(line) + `</text:p>`)
			}
			out.WriteString(`</draw:text-box></draw:frame></presentation:notes>`)
		}
		out.WriteString(`</draw:page>`)
	}
	out.WriteString(`</office:presentation>`)
	for name, data := range w.media {
		b.media[name] = data
	}
	for name, data := range mw.media {
		b.media[name] = data
	}
	return w.styles.String() + st.String(), out.String(), stylesDoc, b.media
}

// --- Cajas ---

// boxes escribe las cajas como objetos de una página (o de la maestra) y devuelve, por
// caja, el número de id que recibió (0 si no salió: una imagen o un medio sin origen
// válido). Los estilos gráficos nuevos van a st.
func (b *odpBuild) boxes(boxes []slideBox, w *odtWriter, st *strings.Builder, fields bool) (string, []int) {
	var out strings.Builder
	ids := make([]int, len(boxes))
	for i, box := range boxes {
		var x string
		n := b.idN + 1
		switch box.Kind {
		case "image":
			ext, data, ok := decodeDataURL(box.Src)
			if !ok {
				continue
			}
			b.imgN++
			name := "Pictures/img" + strconv.Itoa(b.imgN) + "." + ext
			b.media[name] = data
			x = `<draw:frame draw:style-name="Sytue_20_marco" ` + odpIDAttrs(n) + ` ` + b.pos(box) + `>` +
				`<draw:image xlink:href="` + name + `" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad"/></draw:frame>`
		case "media":
			x = b.mediaFrame(n, box)
		case "shape":
			x = b.shape(n, box, w, st, fields)
		default: // texto
			x = `<draw:frame draw:style-name="Sytue_20_marco" ` + odpIDAttrs(n) + ` ` + b.pos(box) + `><draw:text-box>` +
				b.paras(w, box.HTML, fields) + `</draw:text-box></draw:frame>`
		}
		if x == "" {
			continue
		}
		b.idN = n
		ids[i] = n
		out.WriteString(x)
	}
	return out.String(), ids
}

// shape es una forma con su relleno, su trazo y su texto.
func (b *odpBuild) shape(n int, box slideBox, w *odtWriter, st *strings.Builder, fields bool) string {
	b.nG++
	sty := "gr" + strconv.Itoa(b.nG)
	var gp strings.Builder
	if c := shapeColor(box.Fill, shapeFillDefault); c != "" && box.Shape != "line" {
		gp.WriteString(`draw:fill="solid" draw:fill-color="#` + c + `" `)
	} else {
		gp.WriteString(`draw:fill="none" `)
	}
	if c := shapeColor(box.Stroke, shapeStrokeDefault); c != "" {
		gp.WriteString(`draw:stroke="solid" svg:stroke-width="0.053cm" svg:stroke-color="#` + c + `" `)
	} else {
		gp.WriteString(`draw:stroke="none" `)
	}
	st.WriteString(`<style:style style:name="` + sty + `" style:family="graphic" style:parent-style-name="standard">` +
		`<style:graphic-properties ` + gp.String() + `draw:textarea-vertical-align="middle" draw:auto-grow-height="false" ` +
		`fo:padding-top="0.127cm" fo:padding-bottom="0.127cm" fo:padding-left="0.254cm" fo:padding-right="0.254cm" fo:wrap-option="wrap"/></style:style>`)
	head := ` draw:style-name="` + sty + `" ` + odpIDAttrs(n) + ` `
	switch box.Shape {
	case "line":
		return `<draw:line` + head + `svg:x1="` + cmStr(box.X/100*b.pw) + `" svg:y1="` + cmStr(box.Y/100*b.ph) +
			`" svg:x2="` + cmStr((box.X+box.W)/100*b.pw) + `" svg:y2="` + cmStr((box.Y+box.H)/100*b.ph) + `"><text:p/></draw:line>`
	case "ellipse":
		return `<draw:ellipse` + head + b.pos(box) + `>` + b.paras(w, box.HTML, fields) + `</draw:ellipse>`
	}
	geom := ""
	if typ := odpShapeTypes[box.Shape]; typ != "" {
		geom = `draw:type="` + typ + `"`
	} else if box.Shape == "cylinder" {
		geom = odpCylinder
	}
	if geom != "" {
		return `<draw:custom-shape` + head + b.pos(box) + `>` + b.paras(w, box.HTML, fields) +
			`<draw:enhanced-geometry svg:viewBox="0 0 21600 21600" ` + geom + `/></draw:custom-shape>`
	}
	return `<draw:rect` + head + b.pos(box) + `>` + b.paras(w, box.HTML, fields) + `</draw:rect>`
}

// mediaFrame es un vídeo o un audio: el objeto multimedia de ODF con su póster. "" si
// el origen no vale (ni data:URL de audio/vídeo ni URL http/https).
func (b *odpBuild) mediaFrame(n int, box slideBox) string {
	var href string
	if ext, _, data, ok := decodeMediaDataURL(box.Src); ok {
		b.medN++
		href = "Media/media" + strconv.Itoa(b.medN) + "." + ext
		b.media[href] = data
	} else if u := strings.TrimSpace(box.Src); strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
		href = u
	} else {
		return ""
	}
	var poster string
	if ext, data, ok := decodeDataURL(box.Poster); ok {
		b.imgN++
		poster = "Pictures/img" + strconv.Itoa(b.imgN) + "." + ext
		b.media[poster] = data
	} else {
		poster = "Pictures/poster.png"
		if !b.posterWritten {
			b.posterWritten = true
			b.media[poster] = posterPNG()
		}
	}
	return `<draw:frame draw:style-name="Sytue_20_marco" ` + odpIDAttrs(n) + ` ` + b.pos(box) + `>` +
		`<draw:plugin xlink:href="` + xmlEscape(href) + `" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad" ` +
		`draw:mime-type="application/vnd.sun.star.media"><draw:param draw:name="Loop" draw:value="` + strconv.FormatBool(box.Loop) + `"/>` +
		`<draw:param draw:name="Mute" draw:value="false"/><draw:param draw:name="VolumeDB" draw:value="0"/>` +
		b.trackElems(box) + `</draw:plugin>` +
		`<draw:image xlink:href="` + poster + `" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad"/></draw:frame>`
}

// trackElems son los subtítulos de un medio: ODF no tiene pistas de texto, así que cada
// una va como un elemento propio (`sytue:track`) dentro del `draw:plugin`, con su .vtt en
// `Media/`. Cualquier otro lector ignora lo que no conoce; LibreOffice lo descarta al
// guardar.
func (b *odpBuild) trackElems(box slideBox) string {
	var out strings.Builder
	for _, t := range pistasValidas(box.Tracks) {
		b.subN++
		href := "Media/subtitles" + strconv.Itoa(b.subN) + ".vtt"
		b.media[href] = []byte(t.VTT)
		out.WriteString(`<sytue:track xlink:href="` + href + `" sytue:lang="` + xmlEscape(t.Lang) + `"`)
		if t.Label != "" {
			out.WriteString(` sytue:label="` + xmlEscape(t.Label) + `"`)
		}
		if t.Default {
			out.WriteString(` sytue:default="true"`)
		}
		out.WriteString(`/>`)
	}
	return out.String()
}

// --- Texto ---

// paras escribe el HTML de una caja como párrafos ODF. Los títulos llevan el tamaño del
// editor (44/30/24 pt); el resto hereda los 20 pt del estilo «standard». Con `fields`
// (la maestra), {n} y {total} salen como campos.
func (b *odpBuild) paras(w *odtWriter, htmlStr string, fields bool) string {
	w.body.Reset()
	w.lists, w.inItem = nil, false
	n := 0
	for _, blk := range bloquesDeCaja(htmlStr) {
		if blk.para == nil {
			continue
		}
		n++
		p := *blk.para
		switch {
		case p.list == "" && headingLevel(p.style) > 0:
			w.closeList()
			w.body.WriteString(`<text:h text:outline-level="` + strconv.Itoa(headingLevel(p.style)) + `" text:style-name="` + b.headStyle(w, p) + `">`)
			b.runs(w, p.runs, fields)
			w.body.WriteString(`</text:h>`)
		case p.list == "" && fields && paraHasField(p):
			w.closeList()
			w.body.WriteString(`<text:p text:style-name="` + w.paraStyle(p) + `">`)
			b.runs(w, p.runs, true)
			w.body.WriteString(`</text:p>`)
		case p.list != "":
			// ponytail: dentro de una viñeta del patrón {n} no es un campo.
			b.listItem(w, p)
		default:
			w.closeList()
			w.block(p)
		}
	}
	w.closeList()
	if n == 0 {
		return `<text:p/>`
	}
	return w.body.String()
}

func paraHasField(p xpara) bool {
	for _, r := range p.runs {
		if reMasterField.MatchString(r.text) {
			return true
		}
	}
	return false
}

// listItem escribe un elemento de lista anidando por nivel (una sublista es un
// `text:list` dentro del elemento padre). La sangría la da el estilo de lista.
func (b *odpBuild) listItem(w *odtWriter, p xpara) {
	lvl := max(p.level, 0)
	lvl = min(lvl, len(w.lists)) // sin saltos: cada nivel cuelga del elemento anterior
	for len(w.lists) > lvl+1 {
		w.popList()
	}
	if len(w.lists) == lvl+1 && w.lists[lvl] != p.list {
		w.popList()
	}
	if len(w.lists) == lvl {
		style := "Lbullet"
		if p.list == "number" {
			style = "Lnumber"
		}
		w.body.WriteString(`<text:list text:style-name="` + style + `">`)
		w.lists = append(w.lists, p.list)
		w.inItem = false
	}
	if w.inItem {
		w.body.WriteString(`</text:list-item>`)
	}
	w.body.WriteString(`<text:list-item><text:p text:style-name="` + b.listParaStyle(w, p.align) + `">`)
	w.runs(&w.body, p.runs)
	w.body.WriteString(`</text:p>`)
	w.inItem = true
}

func (b *odpBuild) listParaStyle(w *odtWriter, align string) string {
	return w.paraStyle(xpara{align: align})
}

// headStyle es el estilo de un título: tamaño del editor, negrita y alineación.
func (b *odpBuild) headStyle(w *odtWriter, p xpara) string {
	sig := p.style + "|" + p.align
	if name, ok := b.headSty[sig]; ok {
		return name
	}
	b.nH++
	name := "SH" + strconv.Itoa(b.nH)
	b.headSty[sig] = name
	para := ""
	if a := odfAlign(p.align); a != "" {
		para = `<style:paragraph-properties fo:text-align="` + a + `"/>`
	}
	w.styles.WriteString(`<style:style style:name="` + name + `" style:family="paragraph">` + para +
		`<style:text-properties fo:font-size="` + strconv.Itoa(slideRunSize(p.style)/100) + `pt" fo:font-weight="bold"/></style:style>`)
	return name
}

// runs escribe los tramos de un párrafo; con `fields`, {n} y {total} salen como campos
// (que heredan el formato del tramo).
func (b *odpBuild) runs(w *odtWriter, runs []xrun, fields bool) {
	dst := &w.body
	if !fields {
		w.runs(dst, runs)
		return
	}
	for _, r := range runs {
		if r.brk || r.image != nil || !reMasterField.MatchString(r.text) {
			w.runs(dst, []xrun{r})
			continue
		}
		emit := func(s string) {
			if s != "" {
				c := r
				c.text = s
				w.runs(dst, []xrun{c})
			}
		}
		last := 0
		for _, m := range reMasterField.FindAllStringIndex(r.text, -1) {
			emit(r.text[last:m[0]])
			f := `<text:page-count>` + strconv.Itoa(b.total) + `</text:page-count>`
			if r.text[m[0]:m[1]] == "{n}" {
				f = `<text:page-number text:select-page="current">` + slideNumCachedPh + `</text:page-number>`
			}
			if r.hasFmt() {
				f = `<text:span text:style-name="` + w.textStyle(r) + `">` + f + `</text:span>`
			}
			dst.WriteString(f)
			last = m[1]
		}
		emit(r.text[last:])
	}
}

// --- Diapositiva: fondo, patrón visible y transición ---

// pageStyle es el estilo `drawing-page` de una diapositiva; "" si no se sale de lo
// normal (fondo del tema, patrón visible, sin transición).
func (b *odpBuild) pageStyle(st *strings.Builder, sl slideT) string {
	var p strings.Builder
	bg := normalizeColor(sl.Bg)
	if bg != "" {
		p.WriteString(`draw:fill="solid" draw:fill-color="#` + bg + `" `)
	}
	if sl.HideMaster {
		p.WriteString(`presentation:background-objects-visible="false" `)
	}
	switch sl.Transition {
	case "fade":
		p.WriteString(`presentation:transition-speed="fast" smil:type="fade" smil:subtype="crossfade" `)
	case "slide":
		p.WriteString(`presentation:transition-speed="fast" smil:type="pushWipe" smil:subtype="fromRight" `)
	case "morph":
		// ODF no tiene morph: fundido para quien no lo entiende, y la marca propia.
		p.WriteString(`presentation:transition-speed="fast" smil:type="fade" smil:subtype="crossfade" sytue:transition="morph" `)
	}
	if p.Len() == 0 {
		return ""
	}
	b.nD++
	name := "dp" + strconv.Itoa(b.nD)
	st.WriteString(`<style:style style:name="` + name + `" style:family="drawing-page"><style:drawing-page-properties ` +
		strings.TrimSpace(p.String()) + `/></style:style>`)
	return name
}

// --- Conectores ---

func (b *odpBuild) connectors(sl slideT, ids []int, st *strings.Builder) string {
	byID := map[string]int{}
	for i, bx := range sl.Boxes {
		if bx.ID != "" {
			byID[bx.ID] = i
		}
	}
	resolve := func(e connectorEnd) connEnd {
		if i, ok := byID[e.BoxID]; ok && ids[i] != 0 {
			bx := sl.Boxes[i]
			return connEnd{
				rect:   &crect{bx.X / 100 * b.pw, bx.Y / 100 * b.ph, bx.W / 100 * b.pw, bx.H / 100 * b.ph},
				anchor: e.Anchor, spid: ids[i], sites: cxnSites(bx),
			}
		}
		var x, y float64
		if e.X != nil {
			x = *e.X
		}
		if e.Y != nil {
			y = *e.Y
		}
		return connEnd{pt: cpt{x / 100 * b.pw, y / 100 * b.ph}}
	}
	var out strings.Builder
	for _, c := range sl.Connectors {
		from, to := resolve(c.From), resolve(c.To)
		a, z := from.at(to.target()), to.at(from.target())
		b.nG++
		sty := "gr" + strconv.Itoa(b.nG)
		color := b.th.Ink
		if v := normalizeColor(c.Stroke); v != "" {
			color = v
		}
		marks := ""
		if c.StartArrow {
			marks += `draw:marker-start="Sytue_20_flecha" draw:marker-start-width="0.25cm" draw:marker-start-center="false" `
		}
		if c.EndArrow {
			marks += `draw:marker-end="Sytue_20_flecha" draw:marker-end-width="0.25cm" draw:marker-end-center="false" `
		}
		st.WriteString(`<style:style style:name="` + sty + `" style:family="graphic" style:parent-style-name="standard">` +
			`<style:graphic-properties draw:stroke="solid" svg:stroke-width="0.053cm" svg:stroke-color="#` + color + `" ` +
			marks + `draw:fill="none"/></style:style>`)
		typ := ""
		switch c.Kind {
		case "curved":
			typ = ` draw:type="curve"`
		case "straight", "":
			typ = ` draw:type="line"`
		}
		glue := ""
		if from.rect != nil && from.sites != nil {
			glue += ` draw:start-shape="` + odpID(from.spid) + `" draw:start-glue-point="` + odpPuntoUnion[sideOf(*from.rect, a)] + `"`
		}
		if to.rect != nil && to.sites != nil {
			glue += ` draw:end-shape="` + odpID(to.spid) + `" draw:end-glue-point="` + odpPuntoUnion[sideOf(*to.rect, z)] + `"`
		}
		out.WriteString(`<draw:connector draw:style-name="` + sty + `"` + typ + ` svg:x1="` + cmStr(a.x) + `" svg:y1="` + cmStr(a.y) +
			`" svg:x2="` + cmStr(z.x) + `" svg:y2="` + cmStr(z.y) + `"` + glue + `><text:p/></draw:connector>`)
	}
	return out.String()
}

// --- Animaciones ---

func secs(ms int) string {
	return strconv.FormatFloat(float64(ms)/1000, 'f', -1, 64) + "s"
}

// odpTiming es el árbol de animaciones de una diapositiva; "" si no tiene animaciones
// ni medios que arranquen solos.
func odpTiming(boxes []slideBox, ids []int) string {
	steps := timedSteps(boxes, ids)
	var auto []int
	for i, bx := range boxes {
		if bx.Kind == "media" && bx.Autoplay && ids[i] != 0 {
			auto = append(auto, ids[i])
		}
	}
	if len(steps) == 0 && len(auto) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<anim:par smil:dur="indefinite" smil:restart="never" presentation:node-type="timing-root">` +
		`<anim:seq smil:dur="indefinite" presentation:node-type="main-sequence">`)
	if len(auto) > 0 {
		b.WriteString(`<anim:par smil:begin="0s" smil:fill="hold"><anim:par smil:begin="0s" smil:fill="hold">`)
		for k, id := range auto {
			node := "with-previous"
			if k == 0 {
				node = "after-previous"
			}
			b.WriteString(`<anim:par smil:begin="0s" smil:fill="hold" presentation:node-type="` + node +
				`" presentation:preset-class="media-call" presentation:preset-id=""><anim:command smil:dur="0.001s" smil:fill="hold" ` +
				`smil:targetElement="` + odpID(id) + `" anim:command="play"/></anim:par>`)
		}
		b.WriteString(`</anim:par></anim:par>`)
	}
	for _, st := range steps {
		b.WriteString(`<anim:par smil:begin="next" smil:fill="hold">`)
		delay := 0
		for gi, g := range st {
			begin := "0s"
			if gi > 0 {
				begin = secs(delay)
			}
			b.WriteString(`<anim:par smil:begin="` + begin + `" smil:fill="hold">`)
			longest := 0
			for ei, r := range g {
				node := "with-previous"
				if ei == 0 {
					node = "after-previous"
					if gi == 0 {
						node = "on-click"
					}
				}
				x, dur := odpEffect(r.anim, ids[r.box], node)
				b.WriteString(x)
				longest = max(longest, dur)
			}
			b.WriteString(`</anim:par>`)
			delay += longest
		}
		b.WriteString(`</anim:par>`)
	}
	b.WriteString(`</anim:seq></anim:par>`)
	return b.String()
}

// odpEffect es el `anim:par` de un efecto con sus comportamientos y lo que dura (ms).
// Duraciones y desplazamientos son los del .pptx (effectXML), de modo que se ven igual.
func odpEffect(a slideAnim, id int, node string) (string, int) {
	tgt := ` smil:targetElement="` + odpID(id) + `"`
	fx, ok := animFxOf[a.Type]
	if !ok { // trayectoria
		fx = animFx{class: "path", dur: motionDurMS}
	}
	set := func(val string, atMS int) string {
		return `<anim:set smil:begin="` + secs(atMS) + `" smil:dur="0.001s" smil:fill="hold"` + tgt +
			` smil:attributeName="visibility" smil:to="` + val + `"/>`
	}
	fade := func(mode string) string {
		m := ""
		if mode == "out" {
			m = ` smil:mode="out"`
		}
		return `<anim:transitionFilter smil:dur="` + secs(fx.dur) + `"` + tgt + ` smil:type="fade" smil:subtype="crossfade"` + m + `/>`
	}
	prop := func(attr, from, to string) string {
		return `<anim:animate smil:dur="` + secs(fx.dur) + `" smil:fill="hold"` + tgt + ` smil:attributeName="` + attr +
			`" smil:values="` + from + `;` + to + `" smil:keyTimes="0;1"/>`
	}
	scale := func(by string) string {
		return `<anim:animateTransform smil:dur="` + secs(fx.dur/2) + `" smil:fill="hold" smil:autoReverse="true"` + tgt +
			` smil:by="` + by + `" smil:attributeName="transform" svg:type="scale"/>`
	}
	hide := set("hidden", fx.dur-1)
	var beh string
	switch a.Type {
	case "appear":
		beh = set("visible", 0)
	case "fade":
		beh = set("visible", 0) + fade("in")
	case "fly":
		beh = set("visible", 0) + prop("y", "y+0.05", "y") + fade("in")
	case "zoom":
		beh = set("visible", 0) + prop("width", "width*0.82", "width") + prop("height", "height*0.82", "height") + fade("in")
	case "pulse":
		beh = scale("0.06,0.06")
	case "grow-shrink":
		beh = scale("0.22,0.22")
	case "spin":
		beh = `<anim:animateTransform smil:dur="` + secs(fx.dur) + `" smil:fill="hold"` + tgt +
			` smil:by="360" smil:attributeName="transform" svg:type="rotate"/>`
	case "disappear":
		beh = set("hidden", 0)
	case "fade-out":
		beh = fade("out") + hide
	case "fly-out":
		beh = prop("y", "y", "y+0.05") + fade("out") + hide
	case "zoom-out":
		beh = prop("width", "width", "width*0.82") + prop("height", "height", "height*0.82") + fade("out") + hide
	default:
		path := strings.TrimSuffix(motionPath(motionPointsOf(a)), " E")
		beh = `<anim:animateMotion smil:dur="` + secs(fx.dur) + `" smil:fill="hold"` + tgt + ` svg:path="` + path + `"/>`
	}
	class, preset, sub := "motion-path", "", ""
	if o, ok := odpFxOf[a.Type]; ok {
		class, preset, sub = o.class, o.id, o.sub
	}
	if sub != "" {
		sub = ` presentation:preset-sub-type="` + sub + `"`
	}
	return `<anim:par smil:begin="0s" smil:fill="hold" presentation:node-type="` + node + `" presentation:preset-class="` + class +
		`" presentation:preset-id="` + preset + `"` + sub + `>` + beh + `</anim:par>`, fx.dur
}

// --- Estilos y maestra (styles.xml) ---

// odpFrameStyle es el estilo de los marcos de texto, imagen y medio: sin relleno ni
// trazo, con el margen interior de un cuadro de texto de PowerPoint.
const odpFrameStyle = `<style:style style:name="Sytue_20_marco" style:family="graphic" style:parent-style-name="standard">` +
	`<style:graphic-properties draw:fill="none" draw:stroke="none" draw:textarea-vertical-align="top" draw:auto-grow-height="false" ` +
	`fo:padding-top="0.127cm" fo:padding-bottom="0.127cm" fo:padding-left="0.254cm" fo:padding-right="0.254cm" fo:wrap-option="wrap"/></style:style>`

const odpStylesNS = `xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
	`xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" ` +
	`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" ` +
	`xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" ` +
	`xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" ` +
	`xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0" ` +
	`xmlns:xlink="http://www.w3.org/1999/xlink" ` +
	`xmlns:presentation="urn:oasis:names:tc:opendocument:xmlns:presentation:1.0" ` +
	`xmlns:smil="urn:oasis:names:tc:opendocument:xmlns:smil-compatible:1.0" ` +
	`xmlns:sytue="` + odpNSSytue + `"`

// stylesXML es styles.xml: la fuente, el estilo «standard» (tinta y fuente del tema), las
// listas, la flecha de los conectores, el tamaño de página y la página maestra con el
// fondo del tema y las cajas del patrón.
func (b *odpBuild) stylesXML(autoStyles, masterShapes, maestrasDiseno string) string {
	font := xmlEscape(b.font)
	ink, bg := b.th.Ink, b.th.Bg
	return xmlHeader + `<office:document-styles ` + odpStylesNS + ` office:version="1.3">` +
		`<office:font-face-decls><style:font-face style:name="` + font + `" svg:font-family="&apos;` + font + `&apos;"/></office:font-face-decls>` +
		`<office:styles>` +
		`<style:style style:name="standard" style:family="graphic"><style:graphic-properties draw:fill="none" draw:stroke="none"/>` +
		`<style:text-properties style:font-name="` + font + `" fo:font-size="20pt" fo:color="#` + ink + `" fo:language="es" fo:country="ES"/></style:style>` +
		`<draw:marker draw:name="Sytue_20_flecha" svg:viewBox="0 0 20 30" svg:d="m10 0-10 30h20z"/>` +
		odpListStyles(b.font) + b.layoutStyles() +
		`</office:styles><office:automatic-styles>` +
		`<style:page-layout style:name="PL1"><style:page-layout-properties fo:page-width="` + cmStr(b.pw) + `" fo:page-height="` + cmStr(b.ph) +
		`" fo:margin-top="0cm" fo:margin-bottom="0cm" fo:margin-left="0cm" fo:margin-right="0cm" style:print-orientation="landscape"/></style:page-layout>` +
		`<style:style style:name="Mdp1" style:family="drawing-page"><style:drawing-page-properties presentation:background-visible="true" ` +
		`presentation:background-objects-visible="true" draw:fill="solid" draw:fill-color="#` + bg + `"/></style:style>` +
		odpFrameStyle + autoStyles + `</office:automatic-styles>` +
		`<office:master-styles><style:master-page style:name="Default"` + b.themeName() + ` style:page-layout-name="PL1" draw:style-name="Mdp1">` +
		masterShapes + `</style:master-page>` + maestrasDiseno + `</office:master-styles></office:document-styles>`
}

// odpListStyles son las dos listas del editor: con viñeta y numerada.
func odpListStyles(font string) string {
	return odpListStyle("Lbullet", false, xmlEscape(font)) + odpListStyle("Lnumber", true, xmlEscape(font))
}

// odpListStyle es la lista con viñeta o numerada de nueve niveles (la sangría del
// .pptx: 0,79 cm por nivel).
func odpListStyle(name string, numbered bool, font string) string {
	var s strings.Builder
	s.WriteString(`<text:list-style style:name="` + name + `">`)
	for lvl := 1; lvl <= 9; lvl++ {
		// Modo clásico (espacio antes + ancho de la etiqueta): es el que Impress aplica
		// a la sangría; el de «alineación de etiqueta» lo ignora en un cuadro de texto.
		props := `<style:list-level-properties text:space-before="` + cmStr(0.8*float64(lvl-1)) + `" text:min-label-width="0.8cm"/>`
		l := strconv.Itoa(lvl)
		if numbered {
			s.WriteString(`<text:list-level-style-number text:level="` + l + `" style:num-suffix="." style:num-format="1">` + props +
				`<style:text-properties fo:font-size="100%"/></text:list-level-style-number>`)
		} else {
			s.WriteString(`<text:list-level-style-bullet text:level="` + l + `" text:bullet-char="•" text:bullet-relative-size="100%">` + props +
				`<style:text-properties style:font-name="` + font + `"/></text:list-level-style-bullet>`)
		}
	}
	s.WriteString(`</text:list-style>`)
	return s.String()
}

// --- Diseños y tema propio ---

func odpLayoutName(id string) string { return "AL_" + id }

// layoutAttr es la referencia de una diapositiva a su diseño ("" si no lleva ninguno).
func (b *odpBuild) layoutAttr(id string) string {
	for _, l := range b.layouts {
		if l.ID == id {
			return ` presentation:presentation-page-layout-name="` + odpLayoutName(id) + `"`
		}
	}
	return ""
}

// layoutStyles son los diseños del modelo como `style:presentation-page-layout` de ODF: sus
// marcadores de posición (el título y el resto como esquema). El id y el nombre del diseño
// y la muestra de cada marcador van como atributos propios, que ODF deja pasar y los demás
// lectores ignoran.
func (b *odpBuild) layoutStyles() string {
	var out strings.Builder
	for _, l := range b.layouts {
		out.WriteString(`<style:presentation-page-layout style:name="` + odpLayoutName(l.ID) + `" sytue:id="` + xmlEscape(l.ID) +
			`" sytue:name="` + xmlEscape(l.Name) + `">`)
		for i, bx := range l.Boxes {
			obj := "outline"
			if i == 0 && esTitulo(bx.HTML) {
				obj = "title"
			}
			out.WriteString(`<presentation:placeholder presentation:object="` + obj + `" ` + b.pos(bx) +
				` sytue:html="` + xmlEscape(bx.HTML) + `"/>`)
		}
		out.WriteString(`</style:presentation-page-layout>`)
	}
	return out.String()
}

// themeName es el nombre visible de la página maestra: el del tema propio, si lo hay.
func (b *odpBuild) themeName() string {
	if b.th.Name == "" {
		return ""
	}
	return ` style:display-name="` + xmlEscape(b.th.Name) + `"`
}
