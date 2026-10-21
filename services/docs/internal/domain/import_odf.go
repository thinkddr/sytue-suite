// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/xml"
	"strconv"
	"strings"
)

// Importadores OASIS ODF (.odt/.ods/.odp) -> modelos del editor. Reverso de
// export_odt.go / export_sheet.go / export_slides.go: parsean content.xml (con
// los estilos automáticos para el formato) y reconstruyen doc:html / doc:sheet /
// doc:slides. Clean-room sobre OASIS ODF 1.3 con encoding/xml.

// --- ODT (texto) -> doc:html ---

// importOdtComments convierte los bytes de un .odt en (KindDocument, HTML) y, con comments, deja el ancla de cada comentario en el
// HTML y devuelve los comentarios (ver comments_office.go).
func importOdtComments(data []byte, comments bool, p *presupuesto) (Kind, string, []ImportedComment, error) {
	files, err := readZip(data)
	if err != nil {
		return "", "", nil, err
	}
	content := files["content.xml"]
	if len(content) == 0 {
		return "", "", nil, wrapFormat("odt: falta content.xml")
	}
	styles := parseODFStyles(content)
	blocks, raw := parseODTBody(content, styles, files, comments, p)
	htmlStr := blocksToHTML(blocks)
	if htmlStr == "" {
		htmlStr = "<p></p>"
	}
	var imported []ImportedComment
	if comments {
		imported = importedComments(raw)
	}
	return KindDocument, htmlStr, imported, nil
}

// odfStyleInfo resume el formato de un estilo automático ODF.
type odfStyleInfo struct {
	run    xrun   // formato de carácter (familia text)
	align  string // alineación (familia paragraph)
	hr     bool   // párrafo-regla (border-bottom)
	quote  bool   // hereda del estilo de cita de LibreOffice
	fill   string // relleno #RRGGBB (familia graphic)
	stroke string // trazo #RRGGBB (familia graphic)
}

// parseODFStyles lee <office:automatic-styles> a un mapa nombre->formato.
func parseODFStyles(content []byte) map[string]odfStyleInfo {
	out := map[string]odfStyleInfo{}
	var doc struct {
		Styles []struct {
			Name      string `xml:"name,attr"`
			Family    string `xml:"family,attr"`
			Parent    string `xml:"parent-style-name,attr"`
			TextProps struct {
				FontWeight  string `xml:"font-weight,attr"`
				FontStyle   string `xml:"font-style,attr"`
				Underline   string `xml:"text-underline-style,attr"`
				LineThrough string `xml:"text-line-through-style,attr"`
				Color       string `xml:"color,attr"`
			} `xml:"text-properties"`
			ParaProps struct {
				TextAlign    string `xml:"text-align,attr"`
				BorderBottom string `xml:"border-bottom,attr"`
			} `xml:"paragraph-properties"`
			GraphicProps struct {
				Fill        string `xml:"fill-color,attr"`
				FillType    string `xml:"fill,attr"`
				StrokeColor string `xml:"stroke-color,attr"`
			} `xml:"graphic-properties"`
		} `xml:"automatic-styles>style"`
	}
	if xml.Unmarshal(content, &doc) != nil {
		return out
	}
	for _, s := range doc.Styles {
		info := odfStyleInfo{}
		info.run.b = s.TextProps.FontWeight == "bold"
		info.run.i = s.TextProps.FontStyle == "italic"
		info.run.u = s.TextProps.Underline != "" && s.TextProps.Underline != "none"
		info.run.strike = s.TextProps.LineThrough != "" && s.TextProps.LineThrough != "none"
		info.run.color = normalizeColor(s.TextProps.Color)
		info.align = odfAlignToInternal(s.ParaProps.TextAlign)
		info.hr = strings.Contains(s.ParaProps.BorderBottom, "solid")
		info.quote = s.Parent == odfQuoteStyle
		info.fill = normalizeColor(s.GraphicProps.Fill)
		info.stroke = normalizeColor(s.GraphicProps.StrokeColor)
		out[s.Name] = info
	}
	return out
}

// odfQuoteStyle es el estilo de cita de LibreOffice: su nombre interno no se traduce.
const odfQuoteStyle = "Quotations"

func odfAlignToInternal(a string) string {
	switch a {
	case "center":
		return "center"
	case "end", "right":
		return "right"
	case "justify":
		return "justify"
	}
	return ""
}

// parseODTBody recorre <office:text> reconstruyendo bloques de documento.
func parseODTBody(content []byte, styles map[string]odfStyleInfo, files map[string][]byte, comments bool, p *presupuesto) ([]xblock, []*rawComment) {
	dec := p.decodificador(content)
	d := &odfDec{dec: dec, styles: styles, listSty: parseODFListStyles(content), files: files, doc: true}
	if comments {
		d.cm = newODFComments(content)
	}
	// Avanzar hasta <office:text>.
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, nil
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "text" {
			blocks := d.readTextBlocks("text", "", 0)
			header, footer, firstDiff := odtRegions(files)
			if header != "" {
				first := ""
				if firstDiff {
					first = " data-first-page-different"
				}
				blocks = append([]xblock{{raw: `<div data-doc-header` + first + ` style="` + docHeaderStyle + `">` + header + `</div>`}}, blocks...)
			}
			if footer != "" {
				blocks = append(blocks, xblock{raw: `<div data-doc-footer style="` + docFooterStyle + `">` + footer + `</div>`})
			}
			if sec := footnotesSection(d.notes); sec != "" {
				blocks = append(blocks, xblock{raw: sec})
			}
			return blocks, d.cm.rawList()
		}
	}
}

type odfDec struct {
	dec     *xml.Decoder
	styles  map[string]odfStyleInfo
	listSty map[string][]string  // <text:list-style> -> tipo de lista por nivel
	files   map[string][]byte    // partes del paquete (para los medios embebidos)
	notes   []string             // notas al pie citadas, en orden de llamada
	depth   int                  // profundidad de recursión (listas/spans/tablas anidados)
	doc     bool                 // texto, no presentación: los campos de página son campos
	changes map[string]odfChange // text:changed-region por id (control de cambios)
	cm      *odfComments         // comentarios del fichero, si se piden
	track   *xtrack              // inserción abierta entre text:change-start y text:change-end
	// paginaW y paginaH son el ancho y el alto de la diapositiva en cm: las cajas de una
	// presentación van en % de ellos.
	paginaW, paginaH float64
	odpState         // lo propio de una presentación (import_odp.go)
}

// maxODFDepth acota el anidamiento recorrido en content.xml (entrada no confiable:
// listas/spans/tablas anidados en profundidad podrían desbordar la pila).
const maxODFDepth = 500

// descend ejecuta fn incrementando el contador de profundidad; si se supera el
// tope, salta el subárbol del elemento actual en lugar de recursar.
func (d *odfDec) descend(fn func()) {
	if d.depth >= maxODFDepth {
		_ = d.dec.Skip()
		return
	}
	d.depth++
	fn()
	d.depth--
}

// readTextBlocks lee bloques de texto ODF hasta el cierre de `until`. `list` es
// el tipo de lista activo ("bullet"/"number"/"") heredado del <text:list> padre y
// `depth` cuántos <text:list> hay abiertos (0 = fuera de lista): el nivel de
// anidamiento del ítem es depth-1. En ODF las sublistas son un <text:list> DENTRO
// de un <text:list-item>, así que la recursión ya marca el nivel.
func (d *odfDec) readTextBlocks(until, list string, depth int) []xblock {
	var blocks []xblock
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return blocks
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "h":
				lvl := atoiDefault(attrLocal(t, "outline-level"), 1)
				if lvl < 1 || lvl > 6 {
					lvl = 1
				}
				p := xpara{style: "Heading" + strconv.Itoa(lvl), align: d.paraAlign(t)}
				p.runs = d.readRuns("h", xrun{})
				pp := p
				blocks = append(blocks, xblock{para: &pp})
			case "p":
				blocks = append(blocks, d.readODFPara(t, list, depth))
			case "list":
				kind := d.listKind(attrLocal(t, "style-name"), depth+1, list)
				d.descend(func() { blocks = append(blocks, d.readTextBlocks("list", kind, depth+1)...) })
			case "list-item":
				d.descend(func() { blocks = append(blocks, d.readTextBlocks("list-item", list, depth)...) })
			case "table":
				d.descend(func() {
					if tbl := d.readODFTable(); tbl != nil {
						blocks = append(blocks, xblock{table: tbl})
					}
				})
			case "tracked-changes":
				d.readChanges()
			default:
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == until {
				return blocks
			}
		}
	}
}

// readODFPara lee un <text:p>: de una lista, una regla horizontal, una cita o un párrafo.
func (d *odfDec) readODFPara(t xml.StartElement, list string, depth int) xblock {
	info := d.styles[attrLocal(t, "style-name")]
	runs := d.readRuns("p", xrun{})
	switch {
	case list != "":
		return xblock{para: &xpara{list: list, level: depth - 1, align: info.align, runs: runs}}
	case info.hr && len(runs) == 0:
		return xblock{hr: true}
	}
	p := xpara{align: info.align, runs: runs}
	if info.quote || attrLocal(t, "style-name") == odfQuoteStyle {
		p.style = "Quote"
	}
	return xblock{para: &p}
}

// paraAlign lee la alineación del estilo de un elemento (por style-name).
func (d *odfDec) paraAlign(se xml.StartElement) string {
	return d.styles[attrLocal(se, "style-name")].align
}

// readRuns recopila los tramos con formato de un párrafo/título ODF hasta el
// cierre de `until`, aplicando el formato heredado f (spans anidados) y enlaces.
func (d *odfDec) readRuns(until string, f xrun) []xrun {
	d.cm.paraStart(until)
	var out []xrun
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return out
		}
		switch t := tok.(type) {
		case xml.CharData:
			if s := string(t); s != "" {
				r := f
				r.text, r.track, r.mark = s, d.track, d.cm.current()
				d.cm.quote(r.mark, s)
				out = append(out, r)
			}
		case xml.StartElement:
			switch t.Name.Local {
			case "change-start", "change-end", "change", "page-number", "page-count", "annotation", "annotation-end":
				if r, ok := d.marker(t, f); ok {
					out = append(out, r)
				}
			case "span":
				nf := applyODFStyle(f, d.styles[attrLocal(t, "style-name")])
				d.descend(func() { out = append(out, d.readRuns("span", nf)...) })
			case "a":
				nf := f
				nf.link = attrLocal(t, "href")
				d.descend(func() { out = append(out, d.readRuns("a", nf)...) })
			case "note": // <text:note>: nota al pie/final, con su texto dentro
				if txt := d.readODFNote(); txt != "" {
					d.notes = append(d.notes, txt)
					r := f
					r.raw = footnoteMarkerClass(len(d.notes), attrLocal(t, "note-class"))
					out = append(out, r)
				}
			case "frame": // <draw:frame> anclado al párrafo: imagen embebida
				if img := d.readODFImage(t); img != "" {
					r := f
					r.raw = img
					out = append(out, r)
				}
			case "line-break":
				out = append(out, xrun{brk: true})
				_ = d.dec.Skip()
			case "s": // espacios múltiples
				r := f
				r.text = strings.Repeat(" ", atoiDefault(attrLocal(t, "c"), 1))
				out = append(out, r)
				_ = d.dec.Skip()
			case "tab":
				r := f
				r.text = "\t"
				out = append(out, r)
				_ = d.dec.Skip()
			default:
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == until {
				return out
			}
		}
	}
}

// marker resuelve los elementos que no son texto: las marcas de revisión, los campos de
// número y total de páginas y los comentarios. Consume el elemento entero.
func (d *odfDec) marker(t xml.StartElement, f xrun) (xrun, bool) {
	switch {
	case t.Name.Local == "annotation":
		d.readAnnotation(t)
	case t.Name.Local == "annotation-end":
		d.cm.end(attrLocal(t, "name"))
		_ = d.dec.Skip()
	case strings.HasPrefix(t.Name.Local, "change"):
		defer func() { _ = d.dec.Skip() }()
		return d.changeRun(t, f)
	default:
		defer func() { _ = d.dec.Skip() }()
		return d.pageField(t.Name.Local, f)
	}
	return f, false
}

// odfChange es una región de cambios de ODF: quién, cuándo, de qué tipo y, si es un
// borrado, qué texto se quitó.
type odfChange struct {
	track xtrack
	text  string
}

// readChanges lee <text:tracked-changes> (recién abierto) a d.changes. // ponytail: del
// borrado solo se guarda el texto plano de sus párrafos, unidos por espacios.
func (d *odfDec) readChanges() {
	d.changes = map[string]odfChange{}
	var id string
	var cur odfChange
	var para strings.Builder
	paras := make([]string, 0, 1)
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "changed-region":
				id, cur, paras = attrLocal(t, "id"), odfChange{}, paras[:0]
			case "insertion":
				cur.track.kind = "ins"
			case "deletion":
				cur.track.kind = "del"
			case "creator":
				cur.track.author = d.readTextOf("creator")
			case "date":
				cur.track.ts = parseTrackTime(strings.TrimSpace(d.readTextOf("date")))
			case "p":
				para.Reset()
				for _, r := range d.readRuns("p", xrun{}) {
					para.WriteString(r.text)
				}
				paras = append(paras, para.String())
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "changed-region":
				cur.text = strings.Join(paras, " ")
				d.changes[id] = cur
			case "tracked-changes":
				return
			}
		}
	}
}

// readTextOf lee el texto de un elemento hasta su cierre.
func (d *odfDec) readTextOf(local string) string {
	var b strings.Builder
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return b.String()
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			if t.Name.Local == local {
				return b.String()
			}
		}
	}
}

// changeRun entiende las marcas de revisión del cuerpo: change-start y change-end abren y
// cierran una inserción (que afecta al texto de en medio) y change es el punto de un
// borrado, cuyo texto se trae de su región.
func (d *odfDec) changeRun(t xml.StartElement, f xrun) (xrun, bool) {
	c, known := d.changes[attrLocal(t, "change-id")]
	switch {
	case t.Name.Local == "change-end" || !known:
		d.track = nil
	case t.Name.Local == "change-start" && c.track.kind == "ins":
		tr := c.track
		d.track = &tr
	case t.Name.Local == "change" && c.track.kind == "del" && c.text != "":
		tr := c.track
		f.text, f.track = c.text, &tr
		return f, true
	}
	return f, false
}

// pageField es el campo de número o total de páginas: de patrón en una presentación, de
// documento en un texto; en el cuerpo de una diapositiva no es nada.
func (d *odfDec) pageField(name string, f xrun) (xrun, bool) {
	switch {
	case d.master:
		f.text = map[string]string{"page-number": "{n}", "page-count": "{total}"}[name]
	case !d.doc:
		return f, false
	case name == "page-number":
		f.field, f.text = "PAGE", "1"
	default:
		f.field, f.text = "NUMPAGES", "1"
	}
	return f, true
}

// readODFNote lee un <text:note> y devuelve el texto de su cuerpo en HTML. La
// cita (el número original) se descarta: la numeración se rehace por orden de
// llamada, que es lo que hace el editor.
func (d *odfDec) readODFNote() string {
	var paras []string
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return strings.Join(paras, "<br>")
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				if h := runsToHTML(d.readRuns("p", xrun{})); h != "" {
					paras = append(paras, h)
				}
			case "note-citation":
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == "note" {
				return strings.Join(paras, "<br>")
			}
		}
	}
}

// readODFImage lee un <draw:frame> de un párrafo y devuelve su imagen embebida
// como <img> con data:URL, o "" si el marco no lleva imagen (formas, objetos
// OLE). Consume el marco entero.
func (d *odfDec) readODFImage(se xml.StartElement) string {
	w := cmToPx(parseLenCm(attrLocal(se, "width")))
	h := cmToPx(parseLenCm(attrLocal(se, "height")))
	src, alt := "", ""
	var svg []byte
	fin := func() string {
		// Un dibujo exportado por Sytue (SVG con su escena + PNG de respaldo) vuelve como dibujo.
		if fig := dibujoDesdeSVG(svg, src, w, h, alt); fig != "" {
			return fig
		}
		return imgTag(src, w, h)
	}
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return fin()
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "image":
				href := strings.TrimPrefix(attrLocal(t, "href"), "./")
				if strings.HasSuffix(strings.ToLower(href), ".svg") && svg == nil {
					svg = d.files[href]
				} else if src == "" {
					src = dataURLFromZip(href, d.files)
				}
				_ = d.dec.Skip()
			case "title":
				alt = d.readTextOf("title")
			default:
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == "frame" {
				return fin()
			}
		}
	}
}

// cmToPx convierte centímetros a píxeles CSS (1 in = 2.54 cm = 96 px).
func cmToPx(cm float64) int { return int(cm/2.54*96 + 0.5) }

// readODFTable lee una <table:table> a un xtable (celdas = sus párrafos).
func (d *odfDec) readODFTable() *xtable {
	t := &xtable{}
read:
	for {
		tok, err := d.dec.Token()
		if err != nil {
			break
		}
		switch e := tok.(type) {
		case xml.StartElement:
			if e.Name.Local == "table-row" {
				if row := d.readODFRow(); len(row) > 0 {
					t.rows = append(t.rows, row)
				}
			} else {
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			if e.Name.Local == "table" {
				break read
			}
		}
	}
	if len(t.rows) == 0 {
		return nil
	}
	t.normalize()
	return t
}

func (d *odfDec) readODFRow() []xcell {
	var row []xcell
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return row
		}
		switch e := tok.(type) {
		case xml.StartElement:
			if e.Name.Local == "table-cell" {
				c := xcell{
					colspan: atoiDefault(attrLocal(e, "number-columns-spanned"), 1),
					rowspan: atoiDefault(attrLocal(e, "number-rows-spanned"), 1),
				}
				for _, blk := range d.readTextBlocks("table-cell", "", 0) {
					if blk.para != nil {
						c.paras = append(c.paras, *blk.para)
					}
				}
				if len(c.paras) == 0 {
					c.paras = []xpara{{}}
				}
				row = append(row, c)
			} else {
				// «covered-table-cell» no se lee: las posiciones cubiertas las rehace normalize.
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			if e.Name.Local == "table-row" {
				return row
			}
		}
	}
}

// applyODFStyle combina el formato heredado con el de un estilo de texto.
func applyODFStyle(f xrun, info odfStyleInfo) xrun {
	if info.run.b {
		f.b = true
	}
	if info.run.i {
		f.i = true
	}
	if info.run.u {
		f.u = true
	}
	if info.run.strike {
		f.strike = true
	}
	if info.run.color != "" {
		f.color = info.run.color
	}
	return f
}

// listKind decide si un <text:list> es de viñeta o numerada: primero su
// declaración <text:list-style> PARA ESE NIVEL (lo que escriben Word y
// LibreOffice), luego la heurística por nombre de estilo (Lbullet/Lnumber, que es
// lo único que escribe nuestro exportador), luego el tipo heredado del padre
// (sublistas que no declaran estilo) y, en último término, viñeta.
//
// Sin la primera regla, una lista numerada de LibreOffice (estilos "L1", "L2"…)
// se importaba SIEMPRE como viñetas: un procedimiento perdía la numeración de sus
// pasos, que es justo lo que hace legible un procedimiento.
func (d *odfDec) listKind(styleName string, level int, parent string) string {
	if lv := d.listSty[styleName]; level >= 1 && level <= len(lv) && lv[level-1] != "" {
		return lv[level-1]
	}
	if k := odfListKind(styleName); k != "" {
		return k
	}
	if parent != "" {
		return parent
	}
	return "bullet"
}

// odfListKind deduce el tipo por el nombre de estilo (Lnumber/Lbullet del
// exportador); "" si el nombre no lo dice.
func odfListKind(styleName string) string {
	s := strings.ToLower(styleName)
	switch {
	case strings.Contains(s, "number"):
		return "number"
	case strings.Contains(s, "bullet"):
		return "bullet"
	}
	return ""
}

// parseODFListStyles lee las <text:list-style> de content.xml a
// nombre -> tipo por nivel (índice 0 = text:level="1").
func parseODFListStyles(content []byte) map[string][]string {
	out := map[string][]string{}
	var doc struct {
		Lists []struct {
			Name   string `xml:"name,attr"`
			Number []struct {
				Level string `xml:"level,attr"`
			} `xml:"list-level-style-number"`
			Bullet []struct {
				Level string `xml:"level,attr"`
			} `xml:"list-level-style-bullet"`
		} `xml:"automatic-styles>list-style"`
	}
	if xml.Unmarshal(content, &doc) != nil {
		return out
	}
	for _, ls := range doc.Lists {
		var levels []string
		set := func(lvl, kind string) {
			n := atoiDefault(lvl, 1)
			if n < 1 || n > 10 {
				return
			}
			for len(levels) < n {
				levels = append(levels, "")
			}
			levels[n-1] = kind
		}
		for _, n := range ls.Number {
			set(n.Level, "number")
		}
		for _, b := range ls.Bullet {
			set(b.Level, "bullet")
		}
		if len(levels) > 0 {
			out[ls.Name] = levels
		}
	}
	return out
}

// odtRegions lee el encabezado y el pie de la primera página maestra de styles.xml, cada uno
// como HTML en línea (los párrafos de más, unidos con <br>), y si la primera página lleva
// un encabezado propio vacío (style:header-first), que es «primera página distinta».
// ponytail: una sola página maestra; pares/impares y encabezados de primera página con
// texto no se leen.
func odtRegions(files map[string][]byte) (header, footer string, firstDiff bool) {
	styles := files["styles.xml"]
	if len(styles) == 0 {
		return "", "", false
	}
	d := &odfDec{dec: xml.NewDecoder(bytes.NewReader(styles)), styles: parseODFStyles(styles), files: files, doc: true}
	inMaster := false
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return header, footer, firstDiff
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch name := t.Name.Local; {
			case name == "master-page":
				inMaster = true
			case inMaster && name == "header":
				header = d.regionHTML(name)
			case inMaster && name == "footer":
				footer = d.regionHTML(name)
			case inMaster && name == "header-first":
				firstDiff = d.regionHTML(name) == ""
			}
		case xml.EndElement:
			if t.Name.Local == "master-page" {
				return header, footer, firstDiff
			}
		}
	}
}

// regionHTML lee el encabezado o el pie que acaba de abrirse como HTML en línea.
func (d *odfDec) regionHTML(name string) string {
	var paras []string
	for _, blk := range d.readTextBlocks(name, "", 0) {
		if blk.para == nil {
			continue
		}
		if h := runsToHTML(blk.para.runs); h != "" {
			paras = append(paras, h)
		}
	}
	return strings.Join(paras, "<br>")
}
