// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// Serializador ODF text (odt) del modelo rico de documento. Genera estilos
// automáticos (text:span para b/i/u/tachado/color, párrafos para títulos/cita/
// alineación, listas con viñeta/número) e hiperenlaces con xlink. Obra original
// de Sytue sobre OASIS ODF 1.3.

// odtWriter acumula el cuerpo ODF y su bloque <office:automatic-styles>.
type odtWriter struct {
	body      strings.Builder
	styles    strings.Builder
	textSty   map[string]string // firma de formato -> nombre Tn
	paraSty   map[string]string // firma de párrafo -> nombre Pn
	nT, nP    int
	lists     []string // pila de listas abiertas por nivel ("bullet"/"number")
	inItem    bool     // hay un <text:list-item> sin cerrar en el nivel más profundo
	media     map[string][]byte
	noteCount int
	stylesDoc string          // styles.xml: la página maestra con encabezado y pie, si los hay
	changes   strings.Builder // <text:changed-region> de las revisiones
	nChange   int
	comments  []xcomment
	byMark    map[string][]int
	pending   string // comentarios de punto, para el primer párrafo
}

func newODTWriter() *odtWriter {
	return &odtWriter{textSty: map[string]string{}, paraSty: map[string]string{}, media: map[string][]byte{}}
}

// odtTextBody produce el cuerpo <office:text> del modelo rico y registra sus
// estilos automáticos en w.styles.
func odtTextBody(title string, s *State, w *odtWriter, comments []Comment) string {
	w.body.WriteString(`<office:text>`)
	if title != "" {
		w.heading(xpara{style: "Heading1", runs: []xrun{{text: title}}}, 1)
	}
	doc := officeDocument(s, comments)
	w.stylesDoc = odtMasterStyles(doc)
	w.comments, w.byMark = doc.comments, doc.byMark
	for _, i := range doc.points {
		w.pending += w.annotation(i)
	}
	for _, blk := range doc.blocks {
		switch {
		case blk.para != nil:
			w.block(*blk.para)
		case blk.table != nil:
			w.closeList()
			w.table(blk.table)
		case blk.hr:
			w.closeList()
			w.body.WriteString(`<text:p text:style-name="` + w.paraStyle(xpara{style: "HR"}) + `"/>`)
		}
	}
	w.closeList()
	if w.pending != "" {
		w.body.WriteString(`<text:p>` + w.pending + `</text:p>`)
	}
	w.body.WriteString(`</office:text>`)
	body := w.body.String()
	if w.changes.Len() > 0 {
		body = strings.Replace(body, "<office:text>", "<office:text><text:tracked-changes>"+w.changes.String()+"</text:tracked-changes>", 1)
	}
	return body
}

func (w *odtWriter) block(p xpara) {
	if p.list != "" {
		w.listItem(p)
		return
	}
	w.closeList()
	if lvl := headingLevel(p.style); lvl > 0 {
		w.heading(p, lvl)
		return
	}
	w.body.WriteString(`<text:p text:style-name="` + w.paraStyle(p) + `">`)
	w.runs(&w.body, p.runs)
	w.body.WriteString(`</text:p>`)
}

func headingLevel(style string) int {
	if strings.HasPrefix(style, "Heading") {
		if n, err := strconv.Atoi(style[len("Heading"):]); err == nil {
			return n
		}
	}
	return 0
}

func (w *odtWriter) heading(p xpara, lvl int) {
	w.body.WriteString(`<text:h text:outline-level="` + strconv.Itoa(lvl) +
		`" text:style-name="` + w.paraStyle(p) + `">`)
	w.runs(&w.body, p.runs)
	w.body.WriteString(`</text:h>`)
}

// listItem agrupa items contiguos del mismo tipo y ANIDA por nivel: en ODF una
// sublista es un <text:list> dentro del <text:list-item> padre, así que el item
// vigente se deja abierto para alojarla. Sin esto un procedimiento con subpasos
// salía plano y la numeración de los pasos principales se corría.
func (w *odtWriter) listItem(p xpara) {
	lvl := p.level
	if lvl < 0 {
		lvl = 0
	}
	if lvl > len(w.lists) {
		lvl = len(w.lists) // sin saltos: cada nivel cuelga del item anterior
	}
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
	w.body.WriteString(`<text:list-item><text:p text:style-name="` + w.paraStyle(xpara{align: p.align}) + `">`)
	w.runs(&w.body, p.runs)
	w.body.WriteString(`</text:p>`)
	w.inItem = true
}

// popList cierra el nivel de lista más profundo (y su item, si sigue abierto).
func (w *odtWriter) popList() {
	if w.inItem {
		w.body.WriteString(`</text:list-item>`)
	}
	w.body.WriteString(`</text:list>`)
	w.lists = w.lists[:len(w.lists)-1]
	w.inItem = len(w.lists) > 0 // el item padre seguía abierto alojando la sublista
}

func (w *odtWriter) closeList() {
	for len(w.lists) > 0 {
		w.popList()
	}
}

// runs escribe los tramos en dst (reutilizable por odt y odp), agrupando enlaces.
func (w *odtWriter) runs(dst *strings.Builder, runs []xrun) {
	dst.WriteString(w.pending)
	w.pending = ""
	i := 0
	for i < len(runs) {
		r := runs[i]
		if r.mark != "" {
			// El ancla de un comentario: la anotación va delante y su fin detrás del texto.
			j := i
			for j+1 < len(runs) && runs[j+1].mark == r.mark {
				j++
			}
			inner := slices.Clone(runs[i : j+1])
			for k := range inner {
				inner[k].mark = ""
			}
			ids := w.byMark[r.mark]
			for _, c := range ids {
				dst.WriteString(w.annotation(c))
			}
			w.runs(dst, inner)
			for _, c := range ids {
				dst.WriteString(`<office:annotation-end office:name="` + annotationName(c) + `"/>`)
			}
			i = j + 1
			continue
		}
		if r.link != "" {
			href := r.link
			dst.WriteString(`<text:a xlink:type="simple" xlink:href="` + xmlEscape(href) + `">`)
			for i < len(runs) && runs[i].link == href {
				w.run(dst, runs[i])
				i++
			}
			dst.WriteString(`</text:a>`)
			continue
		}
		w.run(dst, r)
		i++
	}
}

func (w *odtWriter) run(dst *strings.Builder, r xrun) {
	if r.field != "" {
		if r.field == "PAGE" {
			dst.WriteString(`<text:page-number text:select-page="current">` + xmlEscape(r.text) + `</text:page-number>`)
		} else {
			dst.WriteString(`<text:page-count>` + xmlEscape(r.text) + `</text:page-count>`)
		}
		return
	}
	if r.note != nil {
		w.noteCount++
		id := strconv.Itoa(w.noteCount)
		dst.WriteString(`<text:note text:id="fn-` + id + `" text:note-class="` + r.note.class + `"><text:note-citation>` + id + `</text:note-citation><text:note-body><text:p>`)
		w.runs(dst, r.note.runs)
		dst.WriteString(`</text:p></text:note-body></text:note>`)
		return
	}
	if r.image != nil {
		img := r.image.embedded()
		if img == nil {
			return
		}
		name := "Pictures/inline" + strconv.Itoa(len(w.media)+1) + "." + img.ext
		w.media[name] = img.data
		// Un dibujo lleva primero su SVG y después el PNG: ODF pinta la primera imagen del marco
		// que entienda, así que el PNG queda de respaldo.
		vectorial := ""
		if len(r.image.svg) > 0 {
			svgName := "Pictures/inline" + strconv.Itoa(len(w.media)+1) + ".svg"
			w.media[svgName] = r.image.svg
			vectorial = `<draw:image xlink:href="` + svgName + `" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad" draw:mime-type="image/svg+xml"/>`
		}
		width := strconv.FormatFloat(float64(img.width)/96, 'f', 8, 64) + "in"
		height := strconv.FormatFloat(float64(img.height)/96, 'f', 8, 64) + "in"
		dst.WriteString(`<draw:frame text:anchor-type="as-char" svg:width="` + width + `" svg:height="` + height + `">` + vectorial + `<draw:image xlink:href="` + name + `" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad"/><svg:title>` + xmlEscape(r.image.alt) + `</svg:title></draw:frame>`)
		return
	}
	if r.brk {
		dst.WriteString(`<text:line-break/>`)
		return
	}
	if t := r.track; t != nil {
		// El borrado deja su texto en la región y un punto en el cuerpo; la inserción lo
		// envuelve entre dos marcas.
		w.nChange++
		id := "ct" + strconv.Itoa(w.nChange)
		info := `<office:change-info><dc:creator>` + xmlEscape(t.author) + `</dc:creator>`
		if t.ts != 0 {
			info += `<dc:date>` + time.UnixMilli(t.ts).UTC().Format("2006-01-02T15:04:05") + `</dc:date>`
		}
		info += `</office:change-info>`
		if t.kind == "del" {
			w.changes.WriteString(`<text:changed-region text:id="` + id + `"><text:deletion>` + info + `<text:p>` + xmlEscape(r.text) + `</text:p></text:deletion></text:changed-region>`)
			dst.WriteString(`<text:change text:change-id="` + id + `"/>`)
			return
		}
		w.changes.WriteString(`<text:changed-region text:id="` + id + `"><text:insertion>` + info + `</text:insertion></text:changed-region>`)
		r.track = nil
		dst.WriteString(`<text:change-start text:change-id="` + id + `"/>`)
		w.run(dst, r)
		dst.WriteString(`<text:change-end text:change-id="` + id + `"/>`)
		return
	}
	if !r.hasFmt() {
		dst.WriteString(xmlEscape(r.text))
		return
	}
	dst.WriteString(`<text:span text:style-name="` + w.textStyle(r) + `">` + xmlEscape(r.text) + `</text:span>`)
}

func (w *odtWriter) table(t *xtable) {
	w.body.WriteString(`<table:table table:name="Tabla">`)
	w.body.WriteString(`<table:table-column table:number-columns-repeated="` + strconv.Itoa(t.cols) + `"/>`)
	for _, row := range t.rows {
		w.body.WriteString(`<table:table-row>`)
		for _, cell := range row {
			if cell.covered {
				w.body.WriteString(`<table:covered-table-cell/>`)
				continue
			}
			w.body.WriteString(`<table:table-cell office:value-type="string"`)
			cs, rs := cell.span()
			if cs > 1 {
				w.body.WriteString(` table:number-columns-spanned="` + strconv.Itoa(cs) + `"`)
			}
			if rs > 1 {
				w.body.WriteString(` table:number-rows-spanned="` + strconv.Itoa(rs) + `"`)
			}
			w.body.WriteString(`>`)
			if len(cell.paras) > 0 {
				for _, p := range cell.paras {
					w.body.WriteString(`<text:p text:style-name="` + w.paraStyle(xpara{align: p.align}) + `">`)
					w.runs(&w.body, p.runs)
					w.body.WriteString(`</text:p>`)
				}
			} else {
				w.body.WriteString(`<text:p/>`)
			}
			w.body.WriteString(`</table:table-cell>`)
		}
		w.body.WriteString(`</table:table-row>`)
	}
	w.body.WriteString(`</table:table>`)
}

// --- registro de estilos automáticos ---

func (w *odtWriter) textStyle(r xrun) string {
	sig := boolc(r.b) + boolc(r.i) + boolc(r.u) + boolc(r.strike) + r.color
	if name, ok := w.textSty[sig]; ok {
		return name
	}
	w.nT++
	name := "T" + strconv.Itoa(w.nT)
	w.textSty[sig] = name
	var props strings.Builder
	if r.b {
		props.WriteString(`fo:font-weight="bold" `)
	}
	if r.i {
		props.WriteString(`fo:font-style="italic" `)
	}
	if r.u {
		props.WriteString(`style:text-underline-style="solid" style:text-underline-width="auto" style:text-underline-color="font-color" `)
	}
	if r.strike {
		props.WriteString(`style:text-line-through-style="solid" `)
	}
	if r.color != "" {
		props.WriteString(`fo:color="#` + r.color + `" `)
	}
	w.styles.WriteString(`<style:style style:name="` + name + `" style:family="text"><style:text-properties ` +
		strings.TrimSpace(props.String()) + `/></style:style>`)
	return name
}

// paraStyle registra un estilo de párrafo para (estilo de bloque, alineación).
func (w *odtWriter) paraStyle(p xpara) string {
	sig := p.style + "|" + p.align
	if name, ok := w.paraSty[sig]; ok {
		return name
	}
	w.nP++
	name := "P" + strconv.Itoa(w.nP)
	w.paraSty[sig] = name

	var paraProps, textProps strings.Builder
	if a := odfAlign(p.align); a != "" {
		paraProps.WriteString(`fo:text-align="` + a + `" `)
	}
	if lvl := headingLevel(p.style); lvl > 0 {
		textProps.WriteString(`fo:font-weight="bold" fo:font-size="` + headingPt(lvl) + `pt" `)
	}
	if p.style == "Quote" {
		paraProps.WriteString(`fo:margin-left="1cm" `)
		textProps.WriteString(`fo:font-style="italic" `)
	}
	if p.style == "HR" {
		paraProps.WriteString(`fo:border-bottom="0.5pt solid #000000" `)
	}
	parent := ""
	if p.style == "Quote" {
		// El estilo de cita de LibreOffice: así lo reconoce al abrir y así lo recuperamos.
		parent = ` style:parent-style-name="` + odfQuoteStyle + `"`
	}
	w.styles.WriteString(`<style:style style:name="` + name + `" style:family="paragraph"` + parent + `><style:paragraph-properties ` +
		strings.TrimSpace(paraProps.String()) + `/><style:text-properties ` + strings.TrimSpace(textProps.String()) + `/></style:style>`)
	return name
}

func odfAlign(a string) string {
	switch a {
	case "center":
		return "center"
	case "right":
		return "end"
	case "justify":
		return "justify"
	case "left":
		return "start"
	}
	return ""
}

func headingPt(lvl int) string {
	switch lvl {
	case 1:
		return "24"
	case 2:
		return "18"
	case 3:
		return "14"
	case 4:
		return "13"
	case 5:
		return "12"
	default:
		return "11"
	}
}

func boolc(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// odtMasterStyles es el styles.xml de un documento con encabezado o pie: la página maestra
// «Standard» los lleva, y «primera página distinta» es un style:header-first vacío (el pie
// se repite, que es lo que hace el editor). Sin ninguno de los dos no hay styles.xml.
// ponytail: A4 y 2 cm fijos —el editor no tiene tamaño de página—, sin imágenes en las
// regiones (sus Pictures/ irían a otro paquete) ni pares/impares.
func odtMasterStyles(doc xdoc) string {
	if doc.header == nil && doc.footer == nil {
		return ""
	}
	rw := newODTWriter()
	region := func(tag string, p *xpara) string {
		if p == nil {
			return ""
		}
		var b strings.Builder
		runs := slices.DeleteFunc(slices.Clone(p.runs), func(r xrun) bool { return r.image != nil })
		for i := range runs {
			runs[i].track = nil // las revisiones viven en el cuerpo, no en styles.xml
		}
		rw.runs(&b, runs)
		return `<style:` + tag + `><text:p>` + b.String() + `</text:p></style:` + tag + `>`
	}
	var master strings.Builder
	master.WriteString(region("header", doc.header))
	if doc.firstDiff && doc.header != nil {
		master.WriteString(`<style:header-first><text:p/></style:header-first>`)
	}
	master.WriteString(region("footer", doc.footer))
	if doc.firstDiff && doc.header != nil && doc.footer != nil {
		master.WriteString(region("footer-first", doc.footer))
	}
	return xmlHeader + `<office:document-styles ` +
		`xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
		`xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" ` +
		`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" ` +
		`xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" ` +
		`xmlns:xlink="http://www.w3.org/1999/xlink" ` +
		`xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0" ` +
		`xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" office:version="1.3">` +
		`<office:automatic-styles>` + rw.styles.String() +
		`<style:page-layout style:name="pm1"><style:page-layout-properties fo:page-width="21cm" fo:page-height="29.7cm" ` +
		`fo:margin-top="2cm" fo:margin-bottom="2cm" fo:margin-left="2cm" fo:margin-right="2cm"/>` +
		`<style:header-style><style:header-footer-properties fo:min-height="0.6cm" fo:margin-bottom="0.3cm"/></style:header-style>` +
		`<style:footer-style><style:header-footer-properties fo:min-height="0.6cm" fo:margin-top="0.3cm"/></style:footer-style>` +
		`</style:page-layout></office:automatic-styles>` +
		`<office:master-styles><style:master-page style:name="Standard" style:page-layout-name="pm1">` +
		master.String() + `</style:master-page></office:master-styles></office:document-styles>`
}

func annotationName(i int) string { return "__Annotation__" + strconv.Itoa(i) }

// annotation es el <office:annotation> del comentario i. Lleva el id de su padre en
// loext:parent-name (LibreOffice lo usa para el hilo) y loext:resolved.
func (w *odtWriter) annotation(i int) string {
	c := w.comments[i]
	var b strings.Builder
	b.WriteString(`<office:annotation office:name="` + annotationName(i) + `"`)
	if c.parent >= 0 {
		b.WriteString(` loext:parent-name="` + annotationName(c.parent) + `"`)
	}
	if c.resolved {
		b.WriteString(` loext:resolved="true"`)
	}
	b.WriteString(`><dc:creator>` + xmlEscape(c.author) + `</dc:creator>`)
	if !c.date.IsZero() {
		b.WriteString(`<dc:date>` + c.date.UTC().Format("2006-01-02T15:04:05") + `</dc:date>`)
	}
	for _, p := range c.paras {
		b.WriteString(`<text:p>` + xmlEscape(p) + `</text:p>`)
	}
	b.WriteString(`</office:annotation>`)
	return b.String()
}
