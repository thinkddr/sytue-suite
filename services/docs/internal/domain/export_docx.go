// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Serializador OOXML WordprocessingML (docx) del modelo rico de documento. Mapea
// cada xblock a párrafos/tablas con formato REAL: rPr (b/i/u/strike/color),
// estilos de párrafo (títulos, cita), numeración (viñetas/ordenada) e hiperenlaces.
// Obra original de Sytue sobre ECMA-376.

// docxWriter acumula el cuerpo del documento y las relaciones (hiperenlaces).
type docxWriter struct {
	body       strings.Builder
	relOf      map[string]string // href -> rId
	relDefs    []string          // <Relationship .../> de hiperenlaces
	nextRel    int               // siguiente rId libre (styles=1, numbering=2)
	media      map[string][]byte
	imageTypes map[string]string
	notes      []*xnote
	noteIDs    map[*xnote]int
	nRev       int // revisiones escritas (para su w:id)
	comments   []xcomment
	byMark     map[string][]int
	pending    string   // comentarios de punto, para el primer párrafo
	vinetas    []string // viñetas propias de las listas, por orden de aparición: numId 3, 4…
}

// partesExtra son settings.xml y fontTable.xml, por su nombre, si hacen falta: el tabulador por
// defecto, el color de página visible y la genérica de cada fuente. Sin ellos, volver a importar
// el fichero daría otro tab-size y quitaría la genérica de cada font-family.
func partesExtra(doc xdoc, notas []*xnote) map[string]string {
	extra := map[string]string{}
	var ajustes string
	if doc.pagina != nil && doc.pagina.color != "" {
		ajustes += `<w:displayBackgroundShape/>` // sin él, Word no enseña el color de la página
	}
	if tab := tabuladorDocx(doc.blocks); tab != "" {
		ajustes += `<w:defaultTabStop w:val="` + tab + `"/>`
	}
	if ajustes != "" {
		extra["settings"] = xmlHeader + `<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` + ajustes + `</w:settings>`
	}
	if fuentes := fuentesDocx(doc.blocks, notas); len(fuentes) > 0 {
		var ft strings.Builder
		ft.WriteString(xmlHeader + `<w:fonts xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`)
		for _, nombre := range slices.Sorted(maps.Keys(fuentes)) {
			ft.WriteString(`<w:font w:name="` + xmlEscape(nombre) + `"><w:family w:val="` + fuentes[nombre] + `"/></w:font>`)
		}
		ft.WriteString(`</w:fonts>`)
		extra["fontTable"] = ft.String()
	}
	return extra
}

// numVineta es el numId de las listas con esa viñeta propia (las dos de siempre son 1 y 2).
func (w *docxWriter) numVineta(v string) string {
	i := slices.Index(w.vinetas, v)
	if i < 0 {
		w.vinetas = append(w.vinetas, v)
		i = len(w.vinetas) - 1
	}
	return strconv.Itoa(i + 3)
}

func newDocxWriter() *docxWriter {
	return &docxWriter{relOf: map[string]string{}, nextRel: 3, media: map[string][]byte{}, imageTypes: map[string]string{}, noteIDs: map[*xnote]int{}}
}

// hyperlinkID reserva/reutiliza un rId para un href externo.
func (w *docxWriter) hyperlinkID(href string) string {
	if id, ok := w.relOf[href]; ok {
		return id
	}
	id := "rId" + strconv.Itoa(w.nextRel)
	w.nextRel++
	w.relOf[href] = id
	w.relDefs = append(w.relDefs,
		`<Relationship Id="`+id+`" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="`+
			xmlEscape(href)+`" TargetMode="External"/>`)
	return id
}

// buildDocx genera un .docx con fidelidad de formato a partir del modelo rico.
func buildDocx(title string, s *State, comments []Comment) ([]byte, error) {
	w := newDocxWriter()
	doc := officeDocument(s, comments)
	sect, regions, regionTypes := w.regionParts(doc)
	w.comments, w.byMark = doc.comments, doc.byMark
	w.pending = w.commentRefs(doc.points)
	w.body.WriteString(xmlHeader)
	w.body.WriteString(`<w:document ` +
		`xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
		`xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" ` +
		`xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
		`xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)
	if doc.pagina != nil && doc.pagina.color != "" {
		w.body.WriteString(`<w:background w:color="` + doc.pagina.color + `"/>`)
	}
	w.body.WriteString(`<w:body>`)

	w.bloquesCuerpo(title, doc.blocks)
	w.body.WriteString(sect + `</w:body></w:document>`)
	document := w.body.String()
	noteParts, overrides := w.noteParts()
	maps.Copy(noteParts, regions)
	overrides += regionTypes
	commentParts, commentTypes := w.commentParts()
	overrides += commentTypes
	extra := partesExtra(doc, w.notes)
	for _, nombre := range slices.Sorted(maps.Keys(extra)) {
		id := "rId" + strconv.Itoa(w.nextRel)
		w.nextRel++
		w.relDefs = append(w.relDefs, `<Relationship Id="`+id+`" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/`+
			nombre+`" Target="`+nombre+`.xml"/>`)
		overrides += `<Override PartName="/word/` + nombre + `.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.` +
			nombre + `+xml"/>`
	}

	rels := xmlHeader +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/>` +
		strings.Join(w.relDefs, "") +
		`</Relationships>`

	var defaults strings.Builder
	for _, ext := range []string{"png", "jpg", "gif", "svg"} {
		if mime := w.imageTypes[ext]; mime != "" {
			defaults.WriteString(`<Default Extension="` + ext + `" ContentType="` + mime + `"/>`)
		}
	}
	parts := map[string]string{
		"[Content_Types].xml":          strings.Replace(contentTypesDocx, "</Types>", defaults.String()+overrides+"</Types>", 1),
		"_rels/.rels":                  relsRoot,
		"word/document.xml":            document,
		"word/_rels/document.xml.rels": rels,
		"word/styles.xml":              docxStyles,
		"word/numbering.xml":           numeracionDocx(w.vinetas),
	}
	maps.Copy(parts, commentParts)
	for nombre, xml := range extra {
		parts["word/"+nombre+".xml"] = xml
	}
	for name, content := range noteParts {
		parts[name] = content
		var mediaRels strings.Builder
		for _, definition := range w.relDefs {
			if strings.Contains(definition, `/hyperlink"`) || strings.Contains(definition, `/image"`) {
				mediaRels.WriteString(definition)
			}
		}
		parts["word/_rels/"+strings.TrimPrefix(name, "word/")+".rels"] = xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + mediaRels.String() + `</Relationships>`
	}
	files := make(map[string][]byte, len(parts)+len(w.media))
	for name, content := range parts {
		files[name] = []byte(content)
	}
	for name, content := range w.media {
		files[name] = content
	}
	return zipPackageBytes(files)
}

func (w *docxWriter) bloquesCuerpo(title string, blocks []xblock) {
	if title != "" {
		w.para(xpara{style: "Title", runs: []xrun{{text: title}}})
	}
	for _, blk := range blocks {
		switch {
		case blk.para != nil:
			w.para(*blk.para)
		case blk.table != nil:
			w.table(blk.table)
		case blk.hr:
			w.horizontalRule()
		}
	}
	if w.pending != "" { // un documento sin párrafos: los comentarios de punto van en uno propio
		w.body.WriteString(`<w:p>` + w.pending + `</w:p>`)
	} else if n := len(blocks); n > 0 && blocks[n-1].table != nil {
		// Word exige un párrafo tras una tabla al final del cuerpo; entre tablas no hace falta, y
		// uno vacío sin framePr partiría un marco con dos tablas seguidas.
		w.body.WriteString(`<w:p/>`)
	}
}

func (w *docxWriter) para(p xpara) {
	w.body.WriteString(`<w:p>`)
	w.paraProps(p)
	w.body.WriteString(w.pending)
	w.pending = ""
	w.runs(p.runs)
	w.body.WriteString(`</w:p>`)
}

func (w *docxWriter) paraProps(p xpara) {
	var pr strings.Builder
	switch {
	case p.style != "":
		pr.WriteString(`<w:pStyle w:val="` + p.style + `"/>`)
	case p.list != "":
		pr.WriteString(`<w:pStyle w:val="ListParagraph"/>`) // sin espacio entre elementos, como en el editor
	}
	pr.WriteString(framePrDocx(p.marco))
	if p.list != "" {
		numID := "1" // viñeta
		switch {
		case p.list == "number":
			numID = "2"
		case p.vineta != "":
			numID = w.numVineta(p.vineta)
		}
		pr.WriteString(`<w:numPr><w:ilvl w:val="` + strconv.Itoa(p.level) + `"/><w:numId w:val="` + numID + `"/></w:numPr>`)
	}
	medio, sangria := pPrDocx(p.css, p.list != "")
	pr.WriteString(medio + sangria)
	if jc := docxAlign(p.align); jc != "" {
		pr.WriteString(`<w:jc w:val="` + jc + `"/>`)
	}
	if p.css != "" && !tramosConTexto(p.runs) {
		// Un párrafo sin texto mide lo que su marca (§17.3.1.29): la letra del bloque va en ella.
		if rpr := rPrDocx(xrun{}.conCSS(p.css, true), false); rpr != "" {
			pr.WriteString(`<w:rPr>` + rpr + `</w:rPr>`)
		}
	}
	if pr.Len() > 0 {
		w.body.WriteString(`<w:pPr>` + pr.String() + `</w:pPr>`)
	}
}

// tramosConTexto dice si el párrafo tiene algún tramo de texto corriente. Uno sin ninguno (vacío,
// o solo con una imagen, un campo o una llamada a nota) lleva la letra del bloque en su marca, que
// es de donde la saca el importador (tramosConLetra).
func tramosConTexto(runs []xrun) bool {
	for _, r := range runs {
		if r.text != "" && r.image == nil && r.field == "" {
			return true
		}
	}
	return false
}

func docxAlign(a string) string {
	switch a {
	case "center":
		return "center"
	case "right":
		return "right"
	case "justify":
		return "both"
	case "left":
		return "left"
	}
	return ""
}

func (w *docxWriter) runs(runs []xrun) {
	i := 0
	for i < len(runs) {
		r := runs[i]
		if r.mark != "" {
			// Una racha de tramos con el mismo ancla va entre commentRangeStart y End; el
			// ancla de un comentario que ya no existe se queda en texto corriente.
			j := i
			for j+1 < len(runs) && runs[j+1].mark == r.mark {
				j++
			}
			inner := slices.Clone(runs[i : j+1])
			for k := range inner {
				inner[k].mark = ""
			}
			ids := w.byMark[r.mark]
			w.body.WriteString(w.commentStarts(ids))
			w.runs(inner)
			w.body.WriteString(w.commentEnds(ids))
			i = j + 1
			continue
		}
		if r.link != "" {
			// Agrupar runs contiguos del mismo enlace dentro de un <w:hyperlink>.
			href := r.link
			id := w.hyperlinkID(href)
			w.body.WriteString(`<w:hyperlink r:id="` + id + `">`)
			for i < len(runs) && runs[i].link == href {
				w.run(runs[i], true)
				i++
			}
			w.body.WriteString(`</w:hyperlink>`)
			continue
		}
		w.run(r, false)
		i++
	}
}

func (w *docxWriter) run(r xrun, isLink bool) {
	if r.field != "" {
		w.body.WriteString(`<w:fldSimple w:instr=" ` + r.field + ` "><w:r><w:t>` + xmlEscape(r.text) + `</w:t></w:r></w:fldSimple>`)
		return
	}
	if r.note != nil {
		id, exists := w.noteIDs[r.note]
		if !exists {
			id = len(w.notes) + 1
			w.notes = append(w.notes, r.note)
			w.noteIDs[r.note] = id
		}
		w.body.WriteString(`<w:r><w:` + r.note.class + `Reference w:id="` + strconv.Itoa(id) + `"/></w:r>`)
		return
	}
	if r.image != nil {
		w.picture(r.image)
		return
	}
	if r.brk {
		if r.track != nil {
			w.body.WriteString(w.revisionOpen(r.track) + `<w:r><w:br/></w:r></w:` + r.track.kind + `>`)
			return
		}
		w.body.WriteString(`<w:r><w:br/></w:r>`)
		return
	}
	if r.track != nil {
		w.body.WriteString(w.revisionOpen(r.track))
	}
	w.body.WriteString(`<w:r>`)
	if rpr := rPrDocx(r, isLink); rpr != "" {
		w.body.WriteString(`<w:rPr>` + rpr + `</w:rPr>`)
	}
	tag := "t"
	if r.track != nil && r.track.kind == "del" {
		tag = "delText"
	}
	// Un tabulador es su elemento (§17.3.3.32), no un carácter del texto.
	for i, trozo := range strings.Split(r.text, "\t") {
		if i > 0 {
			w.body.WriteString(`<w:tab/>`)
		}
		if trozo != "" || i == 0 && !strings.Contains(r.text, "\t") {
			w.body.WriteString(`<w:` + tag + ` xml:space="preserve">` + xmlEscape(trozo) + `</w:` + tag + `>`)
		}
	}
	w.body.WriteString(`</w:r>`)
	if r.track != nil {
		w.body.WriteString(`</w:` + r.track.kind + `>`)
	}
}

// revisionOpen abre un <w:ins>/<w:del> con su autor y su fecha. // ponytail: una marca por
// tramo (Word las junta al abrir); el formato dentro de una revisión viaja, pero un salto
// o una imagen revisados salen sin la marca.
func (w *docxWriter) revisionOpen(t *xtrack) string {
	w.nRev++
	date := ""
	if t.ts != 0 {
		date = ` w:date="` + time.UnixMilli(t.ts).UTC().Format("2006-01-02T15:04:05Z") + `"`
	}
	return `<w:` + t.kind + ` w:id="` + strconv.Itoa(1000+w.nRev) + `" w:author="` + xmlEscape(t.author) + `"` + date + `>`
}

// picture guarda únicamente bytes embebidos en el documento. No hace red ni
// acepta SVG activo; reutiliza el decodificador compartido con Diapos.
func (w *docxWriter) picture(img *ximage) {
	embedded := img.embedded()
	if embedded == nil {
		return
	}
	ext, data, width, height := embedded.ext, embedded.data, embedded.width, embedded.height
	id := strconv.Itoa(w.nextRel)
	rid := "rId" + id
	w.nextRel++
	name := "image" + id + "." + ext
	w.media["word/media/"+name] = data
	w.imageTypes[ext] = mediaTypeOf(name)
	w.relDefs = append(w.relDefs, `<Relationship Id="`+rid+`" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/`+name+`"/>`)
	// Un dibujo lleva además su SVG: Word 2016+ y LibreOffice pintan el vectorial (extensión
	// svgBlip de Office) y los demás, el PNG del blip.
	svgExt := ""
	if len(img.svg) > 0 {
		sid := strconv.Itoa(w.nextRel)
		w.nextRel++
		w.media["word/media/image"+sid+".svg"] = img.svg
		w.imageTypes["svg"] = "image/svg+xml"
		w.relDefs = append(w.relDefs, `<Relationship Id="rId`+sid+`" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image`+sid+`.svg"/>`)
		svgExt = `<a:extLst><a:ext uri="{96DAC541-7B7A-43D3-8B79-37D633B846F1}"><asvg:svgBlip xmlns:asvg="http://schemas.microsoft.com/office/drawing/2016/SVG/main" r:embed="rId` + sid + `"/></a:ext></a:extLst>`
	}
	cx, cy := strconv.Itoa(width*9525), strconv.Itoa(height*9525)
	w.body.WriteString(`<w:r><w:drawing><wp:inline><wp:extent cx="` + cx + `" cy="` + cy + `"/><wp:docPr id="` + id + `" name="` + name + `" descr="` + xmlEscape(img.alt) + `"/>` +
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="0" name="` + name + `"/><pic:cNvPicPr/></pic:nvPicPr>` +
		`<pic:blipFill><a:blip r:embed="` + rid + `">` + svgExt + `</a:blip><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="` + cx + `" cy="` + cy + `"/></a:xfrm>` +
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`)
}

func (w *docxWriter) table(t *xtable) {
	ancho, jc, sangria := tblPrDocx(t.css)
	if ancho == "" {
		ancho = `<w:tblW w:w="0" w:type="auto"/>`
	}
	w.body.WriteString(`<w:tbl><w:tblPr>` + ancho + jc + sangria + bordesDeTabla(t) + `</w:tblPr>`)
	w.body.WriteString(`<w:tblGrid>`)
	for i := 0; i < t.cols; i++ {
		w.body.WriteString(`<w:gridCol/>`)
	}
	w.body.WriteString(`</w:tblGrid>`)
	for r, row := range t.rows {
		w.body.WriteString(`<w:tr>`)
		for c, cell := range row {
			span, vm := cell.colspan, ""
			if cell.covered {
				// Word no escribe la celda cubierta por la de su izquierda; la de arriba se
				// marca vMerge (sin valor = continúa) con el ancho de la combinada.
				if cell.orow == r || cell.ocol != c {
					continue
				}
				span, vm = t.rows[cell.orow][cell.ocol].colspan, `<w:vMerge/>`
			} else if cell.rowspan > 1 {
				vm = `<w:vMerge w:val="restart"/>`
			}
			celda := cell
			if cell.covered {
				celda = t.rows[cell.orow][cell.ocol]
			}
			w.celda(cell, celda.css, span, vm)
		}
		w.body.WriteString(`</w:tr>`)
	}
	w.body.WriteString(`</w:tbl>`)
}

// celda escribe un w:tc: su tcPr (con el formato de css, el de la celda que la abarca si está
// cubierta) y sus párrafos.
func (w *docxWriter) celda(cell xcell, css string, span int, vm string) {
	tcW, resto := tcPrDocx(css)
	if tcW == "" {
		tcW = `<w:tcW w:w="0" w:type="auto"/>`
	}
	w.body.WriteString(`<w:tc><w:tcPr>` + tcW)
	if span > 1 {
		w.body.WriteString(`<w:gridSpan w:val="` + strconv.Itoa(span) + `"/>`)
	}
	w.body.WriteString(vm + resto + `</w:tcPr>`)
	if len(cell.paras) == 0 {
		w.body.WriteString(`<w:p/>`)
	}
	for _, p := range cell.paras {
		w.para(p)
	}
	w.body.WriteString(`</w:tc>`)
}

// bordesDeTabla es el tblBorders de una tabla. Una del editor lleva los que pinta editor.css. Una
// cuyas celdas dicen los suyos (la de un .docx importado) no lleva ninguno: el lado que su celda no
// dice tampoco tenía borde en el original, y con los de la tabla volvería con uno que no era.
func bordesDeTabla(t *xtable) string {
	for _, fila := range t.rows {
		for _, c := range fila {
			if strings.Contains(c.css, "border") {
				return ""
			}
		}
	}
	return `<w:tblBorders>` +
		`<w:top w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
		`<w:left w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
		`<w:bottom w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
		`<w:right w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
		`<w:insideH w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
		`<w:insideV w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
		`</w:tblBorders>`
}

func (w *docxWriter) horizontalRule() {
	w.body.WriteString(`<w:p><w:pPr><w:pBdr>` +
		`<w:bottom w:val="single" w:sz="6" w:space="1" w:color="auto"/>` +
		`</w:pBdr></w:pPr></w:p>`)
}

// regionParts escribe el encabezado y el pie como partes header1/footer1 y devuelve el
// <w:sectPr> que las cita, las partes y sus tipos de contenido. Va ANTES del cuerpo: usa
// w.body como borrador y los rId de las partes salen de la misma cuenta que los del cuerpo.
// «Primera página distinta» es titlePg con un encabezado vacío para esa página; el pie
// sigue siendo el mismo, que es lo que hace el editor.
// ponytail: un único juego de partes por documento; pares/impares y secciones, cuando el
// editor los tenga.
func (w *docxWriter) regionParts(doc xdoc) (sect string, parts map[string]string, types string) {
	parts = map[string]string{}
	var refs, ct strings.Builder
	add := func(kind string, p *xpara, n int, kinds ...string) {
		name := kind + strconv.Itoa(n) + ".xml"
		w.body.Reset()
		if p != nil {
			w.para(*p)
		} else {
			w.body.WriteString(`<w:p/>`)
		}
		root := "w:hdr"
		if kind == "footer" {
			root = "w:ftr"
		}
		parts["word/"+name] = xmlHeader + `<` + root + ` xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` + w.body.String() + `</` + root + `>`
		w.body.Reset()
		id := "rId" + strconv.Itoa(w.nextRel)
		w.nextRel++
		w.relDefs = append(w.relDefs, `<Relationship Id="`+id+`" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/`+kind+`" Target="`+name+`"/>`)
		ct.WriteString(`<Override PartName="/word/` + name + `" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.` + kind + `+xml"/>`)
		for _, k := range kinds {
			refs.WriteString(`<w:` + kind + `Reference w:type="` + k + `" r:id="` + id + `"/>`)
		}
	}
	first := doc.firstDiff && doc.header != nil
	if doc.header != nil {
		add("header", doc.header, 1, "default")
		if first {
			add("header", nil, 2, "first")
		}
	}
	if doc.footer != nil {
		kinds := []string{"default"}
		if first {
			kinds = append(kinds, "first")
		}
		add("footer", doc.footer, 1, kinds...)
	}
	tail := paginaSect(doc.pagina)
	if first {
		tail += `<w:titlePg/>`
	}
	return `<w:sectPr>` + refs.String() + tail + `</w:sectPr>`, parts, ct.String()
}

// paginaSect son el pgSz y el pgMar de la sección: los de la capa de la página o, sin ella, los
// del folio del editor (A4 y 96 px), que es lo que se ve al editar.
func paginaSect(p *xpagina) string {
	if p == nil {
		p = &xpagina{ancho: 794, alto: 1123, margenes: [4]float64{96, 96, 96, 96}}
	}
	tw := func(px float64) string { return strconv.Itoa(int(math.Round(px * 15))) }
	orient := ""
	if p.ancho > p.alto {
		orient = ` w:orient="landscape"`
	}
	m := p.margenes
	return `<w:pgSz w:w="` + tw(p.ancho) + `" w:h="` + tw(p.alto) + `"` + orient + `/>` +
		`<w:pgMar w:top="` + tw(m[0]) + `" w:right="` + tw(m[1]) + `" w:bottom="` + tw(m[2]) + `" w:left="` + tw(m[3]) +
		`" w:header="708" w:footer="708" w:gutter="0"/>`
}

func (w *docxWriter) commentStarts(ids []int) string {
	var b strings.Builder
	for _, i := range ids {
		b.WriteString(`<w:commentRangeStart w:id="` + strconv.Itoa(i) + `"/>`)
	}
	return b.String()
}

func (w *docxWriter) commentEnds(ids []int) string {
	var b strings.Builder
	for _, i := range ids {
		n := strconv.Itoa(i)
		b.WriteString(`<w:commentRangeEnd w:id="` + n + `"/><w:r><w:commentReference w:id="` + n + `"/></w:r>`)
	}
	return b.String()
}

// commentRefs son los comentarios sin ancla en el texto, como rango vacío (que Word y
// LibreOffice enseñan como comentario del punto).
func (w *docxWriter) commentRefs(ids []int) string {
	return w.commentStarts(ids) + w.commentEnds(ids)
}

// paraID es el w14:paraId de un párrafo de comentario: 8 cifras hex por debajo de 0x80000000.
func paraID(comment, para int) string {
	return fmt.Sprintf("%08X", 0x10000000+comment*0x100+para)
}

// commentParts escribe comments.xml y commentsExtended.xml (hilos y resuelto), con sus
// relaciones y tipos de contenido. Nada si no hay comentarios.
func (w *docxWriter) commentParts() (parts map[string]string, types string) {
	if len(w.comments) == 0 {
		return nil, ""
	}
	const ns = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
		`xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" ` +
		`xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml" ` +
		`xmlns:w15="http://schemas.microsoft.com/office/word/2012/wordml" mc:Ignorable="w14 w15"`
	var cm, ex strings.Builder
	for i, c := range w.comments {
		date := ""
		if !c.date.IsZero() {
			date = ` w:date="` + c.date.UTC().Format("2006-01-02T15:04:05Z") + `"`
		}
		cm.WriteString(`<w:comment w:id="` + strconv.Itoa(i) + `" w:author="` + xmlEscape(c.author) + `"` + date +
			` w:initials="` + xmlEscape(initials(c.author)) + `">`)
		for k, p := range c.paras {
			cm.WriteString(`<w:p w14:paraId="` + paraID(i, k) + `">`)
			if k == 0 {
				cm.WriteString(`<w:r><w:annotationRef/></w:r>`)
			}
			cm.WriteString(`<w:r><w:t xml:space="preserve">` + xmlEscape(p) + `</w:t></w:r></w:p>`)
		}
		cm.WriteString(`</w:comment>`)
		last := paraID(i, len(c.paras)-1)
		parent := ""
		if c.parent >= 0 {
			parent = ` w15:paraIdParent="` + paraID(c.parent, len(w.comments[c.parent].paras)-1) + `"`
		}
		ex.WriteString(`<w15:commentEx w15:paraId="` + last + `"` + parent + ` w15:done="` + boolc1(c.resolved) + `"/>`)
	}
	parts = map[string]string{
		"word/comments.xml":         xmlHeader + `<w:comments ` + ns + `>` + cm.String() + `</w:comments>`,
		"word/commentsExtended.xml": xmlHeader + `<w15:commentsEx ` + ns + `>` + ex.String() + `</w15:commentsEx>`,
	}
	for _, d := range []struct{ rel, name string }{
		{"http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments", "comments.xml"},
		{"http://schemas.microsoft.com/office/2011/relationships/commentsExtended", "commentsExtended.xml"},
	} {
		id := "rId" + strconv.Itoa(w.nextRel)
		w.nextRel++
		w.relDefs = append(w.relDefs, `<Relationship Id="`+id+`" Type="`+d.rel+`" Target="`+d.name+`"/>`)
	}
	types = `<Override PartName="/word/comments.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.comments+xml"/>` +
		`<Override PartName="/word/commentsExtended.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.commentsExtended+xml"/>`
	return parts, types
}

func boolc1(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
