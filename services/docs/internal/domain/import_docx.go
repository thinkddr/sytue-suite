// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"cmp"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"
)

// Importador OOXML WordprocessingML (.docx) -> modelo rico (xblock) -> doc:html.
// Reverso de export_docx.go: parsea word/document.xml preservando negrita/
// cursiva/subrayado/tachado, color, títulos (pStyle), listas (numPr + numbering),
// alineación (jc), enlaces (w:hyperlink + rels), tablas y reglas horizontales
// (pBdr). Clean-room sobre ECMA-376 con encoding/xml (stdlib); el streaming de
// tokens preserva el orden de runs y enlaces dentro de cada párrafo, algo que el
// unmarshalado a struct no garantiza.

// importDocxComments convierte los bytes de un .docx en (KindDocument, HTML) y, con comments, deja en el HTML el ancla de cada
// comentario (<mark data-comment-id>) y devuelve los comentarios para guardarlos.
func importDocxComments(data []byte, comments bool, p *presupuesto) (Kind, string, []ImportedComment, error) {
	files, err := readZip(data)
	if err != nil {
		return "", "", nil, err
	}
	principal := partePrincipal(files, "word/document.xml")
	doc := files[principal]
	if len(doc) == 0 {
		return "", "", nil, wrapFormat("docx: falta " + principal)
	}
	d := newDocxDec(files, doc, principal, nil, p)
	ancho, alto, izq, der, arriba := seccionDe(doc)
	d.hoja.margenIzq, d.hoja.margenArriba, d.hoja.altoPag = izq, arriba, alto
	if ancho > 0 {
		d.hoja.anchoTexto = ancho - izq - der
	}
	if comments {
		d.cm = parseDocxComments(files)
	}
	d.notes = parseDocxPart(files, "word/footnotes.xml", "footnote", d.hoja, p)
	for id, blk := range parseDocxPart(files, "word/endnotes.xml", "endnote", d.hoja, p) {
		d.notes["e"+id] = blk // las finales comparten numeración con las de pie
	}
	blocks, err := d.parseBody()
	if err != nil {
		return "", "", nil, err
	}
	blocks = d.withRegions(blocks)
	if p := d.capaPagina(); p != "" {
		blocks = append([]xblock{{raw: p}}, blocks...)
	}
	if sec := footnotesSection(d.used); sec != "" {
		blocks = append(blocks, xblock{raw: sec})
	}
	html := blocksToHTML(blocks)
	if html == "" {
		html = "<p></p>"
	}
	var imported []ImportedComment
	if d.cm != nil {
		imported = importedComments(d.cm.raw)
	}
	return KindDocument, html, imported, nil
}

type docxDec struct {
	dec     *xml.Decoder
	rels    map[string]string // rId -> destino (hiperenlaces e imágenes)
	ordered docxNumeracion    // numId -> formato de cada nivel
	files   map[string][]byte // partes del paquete (para los medios embebidos)
	styles  map[string]string // styleId localizado -> estilo semántico del editor
	hoja    *docxHoja         // docDefaults, estilos y tema (import_docx_formato.go)
	notes   map[string]string // id de nota -> su texto en HTML
	used    []string          // notas citadas, en orden de llamada (numeración final)
	sect    docxSect          // encabezado y pie de la última sección
	fld     docxField         // campo complejo abierto (fldChar/instrText)
	cm      *docxComments     // comentarios del fichero, si se piden
	paraN   int               // párrafos leídos del cuerpo
	par     docxParrafo       // el párrafo que se está leyendo
	cajas   []docxCaja        // cuadros de texto del párrafo en curso, para después de él
	finPag  bool              // el párrafo en curso tiene un salto de página (<w:br w:type="page"/>)
	ultimo  docxUltimo        // estilo y contextualSpacing del último párrafo leído
	tabla   docxTablaCtx      // el estilo de la tabla en la que se está, si se está en una
	// presupuesto corta la lectura de esta parte y de las que cuelgan de ella (notas,
	// encabezado y pie) en cuanto lo convertido no cabe (import_presupuesto.go).
	presupuesto *presupuesto
}

// newDocxDec prepara la lectura de una parte del paquete. Con hoja nil la lee de styles.xml.
func newDocxDec(files map[string][]byte, data []byte, part string, hoja *docxHoja, p *presupuesto) *docxDec {
	if hoja == nil {
		hoja = parseDocxHoja(files)
	}
	return &docxDec{
		dec:         p.decodificador(data),
		rels:        parseRels(files["word/_rels/"+path.Base(part)+".rels"]),
		ordered:     parseNumbering(files["word/numbering.xml"], hoja),
		files:       files,
		styles:      parseDocxStyles(files["word/styles.xml"]),
		hoja:        hoja,
		presupuesto: p,
	}
}

// docxParrafo es el contexto del párrafo en curso: su pPr ya heredado y lo que da a su texto.
type docxParrafo struct {
	p     docxPPr // docDefaults + estilo de tabla + estilo de párrafo + lo directo
	rEst  docxRPr // rPr del estilo de párrafo
	base  docxRPr // rDef + estilo de tabla + estilo de párrafo: lo que el bloque pone en su style
	listo bool
}

// docxTablaCtx es lo que el estilo de la tabla en curso da a sus párrafos y celdas. p y r son los
// de la celda en curso: los de toda la tabla (tp y tr) con el formato condicional de su parte.
type docxTablaCtx struct {
	celda  docxCeldaPr
	p      docxPPr
	r      docxRPr
	tp     docxPPr
	tr     docxRPr
	cond   map[string]docxCond
	look   int
	bandaF int
	bandaC int
	ncols  int
	nfilas int
	fila   int // filas leídas
}

// lookPorDefecto es el tblLook de una tabla que no lo dice: el que pone Word al insertar una
// (encabezado, primera columna y filas en bandas), y el que supone LibreOffice.
const lookPorDefecto = lookPrimeraFila | lookPrimeraCol | lookSinBandasC

func nuevaTablaCtx(c docxCeldaPr, p docxPPr, r docxRPr, cond map[string]docxCond, pr docxCeldaPr) docxTablaCtx {
	t := docxTablaCtx{celda: c, p: p, r: r, tp: p, tr: r, cond: cond, look: lookPorDefecto, bandaF: c.bandaF, bandaC: c.bandaC}
	if pr.look != nil {
		t.look = *pr.look
	}
	t.bandaF, t.bandaC = cmp.Or(pr.bandaF, t.bandaF), cmp.Or(pr.bandaC, t.bandaC)
	return t
}

// regiones son las partes del formato condicional (tblStylePr, §17.7.6) en las que cae una celda,
// de menos a más prioridad: bandas de filas y de columnas, primera y última columna, primera y
// última fila y las esquinas. Es el orden en que las pinta LibreOffice, usado como referencia: la
// columna en bandas tapa a la fila, y el encabezado, a la primera columna. Sin tblLook vale el de
// Word (04A0), como en LibreOffice. nfilas < 0 es que aún no se sabe cuántas hay.
func (t docxTablaCtx) regiones(fila, nfilas, col, span int) []string {
	if len(t.cond) == 0 {
		return nil
	}
	l := t.look
	primeraF := l&lookPrimeraFila != 0 && fila == 0
	ultimaF := l&lookUltimaFila != 0 && nfilas > 0 && fila == nfilas-1
	primeraC := l&lookPrimeraCol != 0 && col == 0
	ultimaC := l&lookUltimaCol != 0 && t.ncols > 0 && col+span >= t.ncols
	var out []string
	if l&lookSinBandasF == 0 && !primeraF && !ultimaF {
		out = append(out, t.banda("Horz", fila, t.bandaF, lookPrimeraFila))
	}
	if l&lookSinBandasC == 0 && !primeraC && !ultimaC {
		out = append(out, t.banda("Vert", col, t.bandaC, lookPrimeraCol))
	}
	for _, r := range []struct {
		si   bool
		tipo string
	}{
		{primeraC, "firstCol"},
		{ultimaC, "lastCol"},
		{primeraF, "firstRow"},
		{ultimaF, "lastRow"},
		{primeraF && primeraC, "nwCell"},
		{primeraF && ultimaC, "neCell"},
		{ultimaF && primeraC, "swCell"},
		{ultimaF && ultimaC, "seCell"},
	} {
		if r.si {
			out = append(out, r.tipo)
		}
	}
	return out
}

// banda es la banda (band1 o band2) de la fila o columna k, contando sin el encabezado.
func (t docxTablaCtx) banda(eje string, k, tam, encabezado int) string {
	if t.look&encabezado != 0 {
		k--
	}
	return "band" + strconv.Itoa(k/max(tam, 1)%2+1) + eje
}

// condicionar da a lo que se lee de la celda (col, span) de la fila en curso la letra y el
// párrafo de su parte de la tabla.
func (d *docxDec) condicionar(col, span int) {
	t := &d.tabla
	t.p, t.r = t.tp, t.tr
	for _, reg := range t.regiones(t.fila, t.nfilas, col, span) {
		c := t.cond[reg]
		t.p.sobre(c.p)
		t.r.sobre(c.r)
	}
}

// readTracked lee los runs de un <w:ins>/<w:del> hasta su cierre.
func (d *docxDec) readTracked(until, link string) []xrun {
	var out []xrun
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return out
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "r" {
				out = append(out, d.readRun(link)...)
			} else {
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == until {
				return out
			}
		}
	}
}

// noteRun es la llamada a una nota citada: el marcador del editor, con la nota apuntada
// para la sección del final.
func (d *docxDec) noteRun(t xml.StartElement, base xrun) (xrun, bool) {
	key := attrLocal(t, "id")
	if t.Name.Local == "endnoteReference" {
		key = "e" + key
	}
	txt, ok := d.notes[key]
	if !ok {
		return base, false
	}
	d.used = append(d.used, txt)
	base.raw = footnoteMarkerClass(len(d.used), strings.TrimSuffix(t.Name.Local, "Reference"))
	return base, true
}

// fieldChar avanza el campo complejo y devuelve PAGE o NUMPAGES al cerrarse uno de esos.
func (d *docxDec) fieldChar(typ string) string {
	switch typ {
	case "begin":
		d.fld = docxField{open: true}
	case "separate":
		d.fld.result = true
	case "end":
		f := d.fld.page()
		d.fld = docxField{}
		return f
	}
	return ""
}

// docxSect son las referencias del <w:sectPr> final: rId del encabezado y del pie por
// defecto, el del encabezado de la primera página y si esa página es distinta (titlePg).
type docxSect struct {
	header, footer, firstHeader string
	titlePg                     bool
	ancho, alto                 float64    // pgSz, en twips; 0 es que no lo dice
	margenes                    [4]float64 // pgMar: top, right, bottom, left, en twips
	conMargenes                 bool
	fondo                       string // w:background del documento, RRGGBB
}

// docxField sigue un campo complejo de Word entre sus fldChar. Solo PAGE y NUMPAGES se
// entienden: el resultado en caché de esos se descarta; el de los demás es texto del documento.
type docxField struct {
	open, result bool
	instr        string
}

func (f docxField) page() string {
	if i := strings.Fields(strings.ToUpper(f.instr)); len(i) > 0 && (i[0] == "PAGE" || i[0] == "NUMPAGES") {
		return i[0]
	}
	return ""
}

// parseBody avanza hasta <w:body> y lee sus bloques (párrafos, tablas, reglas).
func (d *docxDec) parseBody() ([]xblock, error) {
	for {
		tok, err := d.dec.Token()
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		if err != nil {
			return nil, wrapFormat("docx: XML inválido")
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "body" {
			return d.readBlocks("body")
		} else if ok && se.Name.Local == "background" {
			d.sect.fondo = docxHex(attrLocal(se, "color"))
		}
	}
}

// readBlocks lee párrafos y tablas hasta el cierre de `until`.
func (d *docxDec) readBlocks(until string) ([]xblock, error) {
	var blocks []xblock
	var previo *xpara    // el párrafo de justo antes, para contextualSpacing
	var enPag []docxCaja // marcos y cuadros colocados respecto a la página en curso
	inicioPag := 0
	abierto := "" // el marco de texto en el que van los últimos párrafos
	enCuerpo := until == "body"
	fin := func() ([]xblock, error) {
		blocks = cerrarMarcoAbierto(blocks, abierto)
		if enCuerpo {
			blocks, _ = d.cerrarPagina(blocks, enPag, inicioPag)
		}
		return juntarMarcos(blocks), nil
	}
	for {
		tok, err := d.dec.Token()
		if err != nil {
			//nolint:nilerr // fin de stream/EOF: importación best-effort, devuelve los bloques ya leídos
			return fin()
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				blocks, previo, abierto, enPag, inicioPag = d.anadirParrafo(blocks, previo, abierto, enCuerpo, enPag, inicioPag)
			case "tbl":
				previo = nil
				blocks, abierto = d.anadirTabla(blocks, abierto)
			case "sectPr":
				if enCuerpo {
					d.readSect()
				} else {
					_ = d.dec.Skip()
				}
			case "sdt", "sdtContent", "customXml":
				// Controles de contenido y datos: su contenido es del documento (las portadas y
				// los índices de Word van dentro de uno). Se entra y se leen sus bloques.
			default:
				d.h().descartaContenido("body", t.Name.Local)
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == until {
				return fin()
			}
		}
	}
}

func (d *docxDec) anadirTabla(blocks []xblock, abierto string) ([]xblock, string) {
	tbl := d.readTable()
	if tbl == nil {
		return blocks, abierto
	}
	if marco := marcoDeTabla(tbl); marco != abierto {
		blocks = cerrarMarcoAbierto(blocks, abierto)
		abierto = ""
		if marco != "" {
			blocks, abierto = append(blocks, abrirMarco(marco)), marco
		}
	}
	return append(blocks, xblock{table: tbl}), abierto
}

// cerrarMarcoAbierto cierra el marco de texto abierto, si lo hay.
func cerrarMarcoAbierto(blocks []xblock, abierto string) []xblock {
	if abierto != "" {
		blocks = append(blocks, cerrarMarco)
	}
	return blocks
}

// marcoDeTabla es el marco que comparten todos los párrafos de una tabla (§17.3.1.11): el que
// escribe la exportación en las celdas de una tabla que iba dentro de un <div data-marco>.
func marcoDeTabla(t *xtable) string {
	paras := parrafosDeTabla(t)
	if len(paras) == 0 || paras[0].marco == "" {
		return ""
	}
	m := paras[0].marco
	for _, p := range paras[1:] {
		if p.marco != m {
			return ""
		}
	}
	return m
}

// anadirParrafo lee un <w:p> y lo pone tras blocks, con los cuadros de texto que ancla detrás (o
// en enPag si van colocados respecto a la página). previo es el párrafo de justo antes (nil si no
// lo hay), para contextualSpacing.
func (d *docxDec) anadirParrafo(blocks []xblock, previo *xpara, abierto string, enCuerpo bool, enPag []docxCaja, inicioPag int) ([]xblock, *xpara, string, []docxCaja, int) {
	antes := d.ultimo
	d.finPag = false
	p, hr := d.readParagraph()
	if enCuerpo && d.par.p.antesPag {
		blocks, abierto, previo = cerrarMarcoAbierto(blocks, abierto), "", nil
		blocks, inicioPag = d.cerrarPagina(blocks, enPag, inicioPag)
		enPag = nil
	}
	if enCuerpo && !hr && p.marco != "" && d.par.p.marcoAncla.enPagina() {
		blocks, abierto, previo = cerrarMarcoAbierto(blocks, abierto), "", nil
		enPag = append(enPag, docxCaja{ancla: d.par.p.marcoAncla, marco: p.marco, bloques: []xblock{{para: &p}}})
	} else {
		blocks, previo, abierto = d.ponerParrafo(blocks, previo, abierto, p, hr, antes)
	}
	blocks, previo, abierto, enPag = d.repartirCajas(blocks, previo, abierto, enCuerpo, enPag)
	if enCuerpo && (d.finPag || d.par.p.finPag) {
		blocks, abierto, previo = cerrarMarcoAbierto(blocks, abierto), "", nil
		blocks, inicioPag = d.cerrarPagina(blocks, enPag, inicioPag)
		enPag = nil
	}
	return blocks, previo, abierto, enPag, inicioPag
}

func (d *docxDec) ponerParrafo(blocks []xblock, previo *xpara, abierto string, p xpara, hr bool, antes docxUltimo) ([]xblock, *xpara, string) {
	// Los párrafos seguidos con el mismo marco van en el mismo (§17.3.1.11).
	if marco := p.marco; hr || marco != abierto {
		blocks = cerrarMarcoAbierto(blocks, abierto)
		abierto = ""
		if !hr && marco != "" {
			blocks, abierto = append(blocks, abrirMarco(marco)), marco
		}
	}
	if hr {
		return append(blocks, xblock{hr: true}), nil, abierto
	}
	if previo != nil && antes.estilo == d.ultimo.estilo {
		contextual(previo, &p, antes.contextual, d.ultimo.contextual)
	}
	return append(blocks, xblock{para: &p}), &p, abierto
}

func (d *docxDec) repartirCajas(blocks []xblock, previo *xpara, abierto string, enCuerpo bool, enPag []docxCaja) ([]xblock, *xpara, string, []docxCaja) {
	if len(d.cajas) == 0 {
		return blocks, previo, abierto, enPag
	}
	var locales []docxCaja
	for _, c := range d.cajas {
		if enCuerpo && c.anclado && c.ancla.enPagina() {
			enPag = append(enPag, c)
		} else {
			locales = append(locales, c)
		}
	}
	d.cajas = nil
	if len(locales) > 0 {
		blocks = cerrarMarcoAbierto(blocks, abierto)
		blocks, abierto, previo = append(blocks, d.bloquesDeCajas(locales)...), "", nil
	}
	return blocks, previo, abierto, enPag
}

// readParagraph lee un <w:p>: sus propiedades (pPr) y sus runs/enlaces en orden.
// Devuelve el párrafo y si es en realidad una regla horizontal (borde inferior sin texto).
func (d *docxDec) readParagraph() (xpara, bool) {
	d.paraN++
	p := xpara{}
	var directo docxPPr
	d.par = docxParrafo{}
	var curLink string
	aparte := "" // la alineación de la ecuación aparte (m:oMathPara) que lleva, si lleva una
	for {
		tok, err := d.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "pPr" {
				d.h().leerPPr(d.dec, &directo)
				continue
			}
			d.empezarParrafo(directo)
			switch t.Name.Local {
			case "hyperlink":
				curLink = d.rels[attrLocal(t, "id")]
			case "r":
				p.runs = append(p.runs, d.readRun(curLink)...)
			case "ins", "del":
				tr := &xtrack{kind: t.Name.Local, author: attrLocal(t, "author"), ts: parseTrackTime(attrLocal(t, "date"))}
				for _, r := range d.readTracked(t.Name.Local, curLink) {
					r.track = tr
					p.runs = append(p.runs, r)
				}
			case "commentRangeStart", "commentRangeEnd":
				d.cm.span(t.Name.Local == "commentRangeStart", attrLocal(t, "id"), d.paraN)
				_ = d.dec.Skip()
			case "fldSimple":
				// PAGE y NUMPAGES son el campo del editor; de los demás (una fecha, un campo de
				// combinación) vale su resultado, que son los tramos de dentro: se entra.
				if f := (docxField{instr: attrLocal(t, "instr")}).page(); f != "" {
					p.runs = append(p.runs, xrun{field: f, text: "1"})
					_ = d.dec.Skip()
				}
			case "smartTag", "customXml", "sdt", "sdtContent":
				// Envoltorios de tramos (etiquetas inteligentes, datos, controles de contenido):
				// se entra y se leen sus tramos como del párrafo.
			case "oMath", "oMathPara":
				if al := d.leerEcuacion(&p, t, curLink); al != "" {
					aparte = al
				}
			default:
				d.h().descartaContenido("p", t.Name.Local)
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "hyperlink":
				curLink = ""
			case "p":
				return d.cerrarConEcuacion(p, directo, aparte)
			}
		}
	}
	return d.cerrarConEcuacion(p, directo, aparte)
}

// leerEcuacion lee un m:oMath o un m:oMathPara a tramos del párrafo, uno por línea en formato
// lineal (import_docx_omml.go), y devuelve la alineación de una ecuación aparte ("" si no lo es).
func (d *docxDec) leerEcuacion(p *xpara, t xml.StartElement, link string) string {
	ec := leerMath(d.dec, t)
	for i, linea := range ecuaciones(ec) {
		if i > 0 {
			p.runs = append(p.runs, xrun{brk: true})
		}
		r := xrun{link: link}
		d.formatoTramo(&r, docxRPr{}, "")
		r.text, r.css = linea, conDeclaracion(r.css, "font-family", familiaEcuacion)
		p.runs = append(p.runs, r)
	}
	if t.Name.Local == "oMathPara" {
		return alineacionEcuacion(ec)
	}
	return ""
}

// cerrarConEcuacion cierra el párrafo con la alineación de su ecuación aparte, si lleva una: Word
// la pinta con la suya (oMathParaPr/jc), no con la del párrafo.
func (d *docxDec) cerrarConEcuacion(p xpara, directo docxPPr, aparte string) (xpara, bool) {
	d.empezarParrafo(directo)
	p, hr := d.cerrarParrafo(p, directo)
	switch aparte {
	case "center", "right":
		p.align = aparte
	case "left":
		p.align = ""
	}
	return p, hr
}

// empezarParrafo resuelve, una vez por párrafo, su pPr y el formato que da a su texto. El pPr
// va siempre primero (CT_P), así que basta con hacerlo ante el primer hijo que no lo es.
func (d *docxDec) empezarParrafo(directo docxPPr) {
	if d.par.listo {
		return
	}
	h := d.h()
	estilo, rEst := h.estiloP(directo.pStyle)
	pp := h.pDef
	pp.sobre(d.tabla.p)
	pp.sobre(estilo)
	pp.sobre(directo)
	pp.pStyle = cmpOr(directo.pStyle, h.defecto["paragraph"])
	d.par = docxParrafo{p: pp, rEst: rEst, base: h.resolverR(d.tabla.r, rEst, docxRPr{}, docxRPr{}), listo: true}
}

// cerrarParrafo da al párrafo su estilo semántico, su lista, su alineación y su CSS.
func (d *docxDec) cerrarParrafo(p xpara, directo docxPPr) (xpara, bool) {
	pp := d.par.p
	if pp.marco != nil {
		p.marco = *pp.marco
	}
	d.ultimo = docxUltimo{estilo: pp.pStyle, contextual: pp.contextual != nil && *pp.contextual}
	if directo.pStyle != "" {
		var declared bool
		p.style, declared = d.styles[directo.pStyle]
		if !declared {
			p.style = docxStyleToBlock(directo.pStyle)
		}
	} else if def := d.h().defecto["paragraph"]; def != "" {
		p.style = d.styles[def]
	}
	if pp.numID != nil && headingLevel(p.style) == 0 {
		// Un título numerado sigue siendo un título: el editor no tiene títulos en listas.
		lvl := 0
		if pp.ilvl != nil {
			lvl = min(max(*pp.ilvl, 0), 8)
		}
		if kind := d.ordered.tipo(*pp.numID, lvl); kind != "" {
			p.list, p.level, p.vineta = kind, lvl, d.ordered.vineta(*pp.numID, lvl)
		}
	}
	p.align = docxJcToAlign(pp.jc)
	p.runs = fundirTramos(p.runs)
	if len(p.runs) == 0 && pp.bordes[2] != nil && *pp.bordes[2] != "none" {
		return p, true
	}
	base := d.par.base
	if !tramosConLetra(p.runs) {
		// Un párrafo sin texto (vacío, o solo con una imagen) mide lo que su marca: es la que da
		// la altura de la línea.
		base = d.h().resolverR(d.tabla.r, d.par.rEst, d.h().estiloR(""), pp.marca)
	}
	p.css = d.cssParrafo(pp, base, baseUsada(base, p.runs), p.list != "", headingLevel(p.style) > 0, tieneTab(p.runs))
	subirComun(&p)
	if l := headingLevel(p.style); l >= 1 && l <= 3 {
		// El editor ya pinta en negrita h1, h2 y h3 (de h4 a h6 el reset los deja como el
		// texto): la negrita del estilo no se repite en cada tramo, y un título de Word que no
		// la lleva la quita.
		negritaDeTitulo(&p, base)
	}
	return p, false
}

// cssParrafo es el style de un bloque: lo que da a su texto, su espaciado, sus sangrías, su
// sombreado y sus bordes. Solo lo que algún nivel de la herencia dice.
func (d *docxDec) cssParrafo(pp docxPPr, base, baseTexto docxRPr, lista, titulo, tab bool) string {
	var css docxCSS
	d.h().cssBase(&css, baseTexto, titulo)
	if pp.antes != nil && (*pp.antes != 0 || titulo) {
		// Un <p> del editor no tiene margen de arriba; un título, sí.
		css.add("margin-top", docxPx(*pp.antes))
	}
	if pp.despues != nil {
		css.add("margin-bottom", docxPx(*pp.despues))
	}
	if lh := interlineado(pp, base); lh != "" {
		css.add("line-height", lh)
	}
	if !lista {
		// En una lista las sangrías las pone la lista del editor, no la del nivel de Word.
		if pp.izq != nil && *pp.izq > 0 {
			css.add("margin-left", docxPx(*pp.izq))
		}
		if pp.der != nil && *pp.der > 0 {
			css.add("margin-right", docxPx(*pp.der))
		}
		if pp.primera != nil && *pp.primera != 0 {
			css.add("text-indent", docxPx(*pp.primera))
		}
	}
	if pp.shd != nil && *pp.shd != "" {
		css.add("background-color", "#"+*pp.shd)
	}
	for i, lado := range []string{"top", "right", "bottom", "left"} {
		if b := pp.bordes[i]; b != nil && *b != "none" {
			css.add("border-"+lado, *b)
		}
	}
	if tab {
		// Sin paradas propias, el tabulador avanza a la siguiente por defecto (w:defaultTabStop,
		// 1,25 cm en casi todo documento): eso es tab-size. Las propias van en --sytue-tabs, que
		// pintan la web (canvas/tabulaciones.ts) y rico; las dos cuentan desde el borde del texto.
		css.add("white-space", "pre-wrap")
		css.add("tab-size", docxPx(d.h().tabulador))
		if v := paradasCSS(pp, lista); v != "" {
			css.add("--sytue-tabs", v)
		}
	}
	return css.String()
}

// paradasCSS es el valor de --sytue-tabs: «120px left, 600px right dot», en px desde el borde
// izquierdo del texto (Word las cuenta desde el margen: se les resta la sangría, y una parada a
// la izquierda de la sangría queda negativa, que vale para la primera línea de una francesa).
// Con sangría francesa, la sangría es una parada más: «1.» + tabulador + texto alinea el texto
// con la segunda línea (§17.3.1.12).
func paradasCSS(pp docxPPr, lista bool) string {
	izq := 0.0
	if pp.izq != nil && !lista {
		izq = *pp.izq
	}
	tabs := slices.DeleteFunc(slices.Clone(pp.tabs), func(t docxTab) bool { return t.val == "clear" || t.pos <= 0 })
	if !lista && pp.primera != nil && *pp.primera < 0 {
		tabs = mezclarTabs(tabs, []docxTab{{pos: izq, val: "left"}})
	}
	out := make([]string, 0, len(tabs))
	for _, t := range tabs {
		out = append(out, strings.TrimSpace(docxNum((t.pos-izq)/15)+"px "+t.val+" "+t.leader))
	}
	return strings.Join(out, ", ")
}

// interlineado es el line-height de un párrafo, o "" si ningún nivel lo dice.
func interlineado(pp docxPPr, base docxRPr) string {
	if pp.linea == nil {
		return ""
	}
	switch pp.reglaLinea {
	case "exact":
		return docxNum(*pp.linea/20) + "pt"
	case "atLeast":
		// ponytail: CSS no tiene interlineado mínimo; vale el mayor entre el mínimo y el
		// natural de la letra del bloque (1,2 veces su tamaño).
		return docxNum(math.Max(*pp.linea/20, 1.2*base.sz)) + "pt"
	}
	// «auto»: 240 avos de la línea natural de la fuente, que en CSS es `normal` y ronda 1,2
	// veces el tamaño. ponytail: 1,2 fijo; varía un 5 % entre fuentes.
	return docxNum(*pp.linea / 240 * 1.2)
}

// negritaDeTitulo reparte la negrita de un título 1 a 3, que el editor ya pinta en negrita: si
// todo su texto la lleva, sale de los tramos; si el estilo la quita, o la lleva y algún tramo de
// texto no, el título va con font-weight:400 y cada tramo con la suya (un título con una palabra
// en negrita, o uno que la quita en sus tramos).
func negritaDeTitulo(p *xpara, base docxRPr) {
	heredada := base.b != nil && *base.b
	todos := heredada
	for _, r := range p.runs {
		if r.text != "" && r.raw == "" && r.field == "" && !r.b {
			todos = false
		}
	}
	for i := range p.runs {
		if todos {
			p.runs[i].b = false
		}
	}
	if base.b != nil && !*base.b || heredada && !todos && tramosConLetra(p.runs) {
		p.css = strings.TrimPrefix(p.css+";font-weight:400", ";")
	}
}

// tramosConLetra dice si algún tramo es texto corriente: no una imagen, un campo ni una llamada a
// nota, que no llevan la letra del párrafo a su bloque (subirComun no los mira).
func tramosConLetra(runs []xrun) bool {
	for _, r := range runs {
		if r.text != "" && r.raw == "" && r.field == "" {
			return true
		}
	}
	return false
}

// baseUsada deja en base solo las propiedades de carácter que algún tramo de texto del párrafo
// hereda del bloque: si todos los tramos de texto traen la suya propia (distinta entre sí), la
// del bloque no pinta ninguna letra y se perdería al exportar y reimportar.
func baseUsada(base docxRPr, runs []xrun) docxRPr {
	if !tramosConLetra(runs) {
		return base
	}
	var usaFont, usaSz, usaColor bool
	for _, r := range runs {
		if r.text == "" || r.raw != "" || r.field != "" || r.image != nil {
			continue
		}
		if !tienePropCSS(r.css, "font-family") {
			usaFont = true
		}
		if !tienePropCSS(r.css, "font-size") {
			usaSz = true
		}
		if r.color == "" {
			usaColor = true
		}
	}
	if !usaFont {
		base.font = ""
	}
	if !usaSz {
		base.sz = 0
	}
	if !usaColor {
		base.color = nil
	}
	return base
}

func tienePropCSS(css, prop string) bool {
	for _, d := range strings.Split(css, ";") {
		if k, _, ok := strings.Cut(d, ":"); ok && k == prop {
			return true
		}
	}
	return false
}

func tieneTab(runs []xrun) bool {
	for _, r := range runs {
		if strings.Contains(r.text, "\t") {
			return true
		}
	}
	return false
}

// fundirTramos junta los tramos seguidos con el mismo formato: Pages parte el texto en
// fragmentos de una o dos letras («20», «2», «6») y cada uno sería su propio <span>.
func fundirTramos(runs []xrun) []xrun {
	out := runs[:0:0]
	for _, r := range runs {
		if n := len(out); n > 0 && r.text != "" && out[n-1].text != "" && mismoFormato(out[n-1], r) {
			out[n-1].text += r.text
			continue
		}
		out = append(out, r)
	}
	return out
}

func mismoFormato(a, b xrun) bool {
	if a.brk || b.brk || a.field != "" || b.field != "" || a.raw != "" || b.raw != "" || a.image != nil || b.image != nil || a.note != nil || b.note != nil {
		return false
	}
	a.text, b.text = "", ""
	return a == b
}

// readRun lee un <w:r>: formato (rPr) y contenido (texto/saltos), en orden.
//
//nolint:gocyclo,gocognit // una rama por elemento de CT_R: la ramificación ES la tabla
func (d *docxDec) readRun(link string) []xrun {
	base := xrun{link: link, mark: d.cm.current(d.paraN)}
	d.formatoTramo(&base, docxRPr{}, "")
	var out []xrun
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return out
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "rPr":
				d.readRPr(&base)
			case "fldChar":
				if f := d.fieldChar(attrLocal(t, "fldCharType")); f != "" {
					out = append(out, xrun{field: f, text: "1"})
				}
				_ = d.dec.Skip()
			case "instrText":
				d.fld.instr += d.readText("instrText")
			case "t", "delText":
				if txt := d.readText(t.Name.Local); txt != "" && (!d.fld.result || d.fld.page() == "") {
					d.cm.quote(base.mark, txt)
					r := base
					r.text = txt
					out = append(out, r)
				}
			case "tab":
				r := base
				r.text = "\t"
				out = append(out, r)
				_ = d.dec.Skip()
			case "br", "cr":
				if t.Name.Local == "br" && attrLocal(t, "type") == "page" {
					d.finPag = true
				}
				out = append(out, xrun{brk: true})
				_ = d.dec.Skip()
			case "footnoteReference", "endnoteReference":
				if r, ok := d.noteRun(t, base); ok {
					out = append(out, r)
				}
				_ = d.dec.Skip()
			case "drawing", "pict", "AlternateContent", "object":
				// Imagen anclada: se pierde por completo si no se lee (el fichero
				// entra sin las figuras y el texto que las cita se queda huérfano).
				if img := d.readRunImage(t.Name.Local); img != "" {
					r := base
					r.raw = img
					out = append(out, r)
				}
			case "noBreakHyphen", "softHyphen", "sym", "ptab":
				// Caracteres que Word guarda como elemento y no como texto (§17.3.3).
				if txt := docxCaracter(t); txt != "" {
					r := base
					r.text = txt
					out = append(out, r)
				}
				_ = d.dec.Skip()
			default:
				d.h().descartaContenido("r", t.Name.Local)
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == "r" {
				return out
			}
		}
	}
}

// readRPr lee el rPr de un tramo y le pone su formato ya heredado.
func (d *docxDec) readRPr(r *xrun) {
	var directo docxRPr
	estilo := ""
	for {
		tok, err := d.dec.Token()
		if err != nil {
			break
		}
		if ee, ok := tok.(xml.EndElement); ok && ee.Name.Local == "rPr" {
			break
		}
		if t, ok := tok.(xml.StartElement); ok {
			if t.Name.Local == "rStyle" {
				estilo = attrLocal(t, "val")
			}
			d.h().propiedadR(t, &directo)
			_ = d.dec.Skip()
		}
	}
	d.formatoTramo(r, directo, estilo)
}

// formatoTramo resuelve el formato de un tramo (§17.7.2) y lo escribe en el modelo: las marcas
// de siempre y, en css, lo que cambia frente a su bloque.
func (d *docxDec) formatoTramo(r *xrun, directo docxRPr, estilo string) {
	h := d.h()
	d.empezarParrafo(docxPPr{})
	res := h.resolverR(d.tabla.r, d.par.rEst, h.estiloR(estilo), directo)
	si := func(b *bool) bool { return b != nil && *b }
	r.b, r.i, r.u, r.strike = si(res.b), si(res.i), si(res.u), si(res.strike)
	r.color = ""
	if res.color != nil && *res.color != "" && (d.par.base.color == nil || *d.par.base.color != *res.color) {
		r.color = *res.color
	}
	r.css = h.cssTramo(res, d.par.base)
	switch res.vert {
	case "superscript":
		r.vert = "sup"
	case "subscript":
		r.vert = "sub"
	default:
		r.vert = ""
	}
}

// parseDocxPart lee una parte con elementos de párrafos (footnotes, endnotes, header, footer)
// a id -> texto en HTML; los hdr/ftr no llevan id, así que su texto queda en la clave "".
// Solo llegan al documento las notas CITADAS, así que las entradas de servicio de
// Word (el separador con id 0, que nadie referencia) se quedan fuera solas.
func parseDocxPart(files map[string][]byte, part, elem string, hoja *docxHoja, p *presupuesto) map[string]string {
	out := map[string]string{}
	data := files[part]
	if len(data) == 0 {
		return out
	}
	nd := newDocxDec(files, data, part, hoja, p)
	for {
		tok, err := nd.dec.Token()
		if err != nil {
			return out
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != elem {
			continue
		}
		id := attrLocal(se, "id")
		blocks, _ := nd.readBlocks(elem)
		var paras []string
		for _, blk := range blocks {
			if blk.para == nil {
				continue
			}
			if h := runsToHTML(blk.para.runs); h != "" {
				paras = append(paras, h)
			}
		}
		if len(paras) > 0 {
			out[id] = strings.Join(paras, "<br>")
		}
	}
}

// readDrawing lee un <w:drawing> (DrawingML) o <w:pict> (VML antiguo) y devuelve
// la imagen embebida como <img> con data:URL, o "" si no hay ninguna (formas,
// gráficos y diagramas quedan fuera de alcance: no hay dónde ponerlos).
func (d *docxDec) readDrawing(until string) string {
	var rID, rSVG, alt string
	var cxEMU, cyEMU int
	var ancla docxAncla // la de un cuadro de texto anclado (wp:anchor), para colocarlo
	var grupo, forma docxVML
	var leidas []docxCaja
	anclado, enH, enV, enVML := false, false, false, false
	// El primero de cada cosa manda (un dibujo de Word repite extensiones y blips).
	primero := func(dst *string, v string) {
		if *dst == "" {
			*dst = v
		}
	}
	for {
		tok, err := d.dec.Token()
		if err != nil {
			d.cajas = append(d.cajas, leidas...)
			return ""
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "blip": // <a:blip r:embed="rId5">
				primero(&rID, attrLocal(t, "embed"))
			case "svgBlip": // extensión de Office: la versión vectorial de la imagen
				primero(&rSVG, attrLocal(t, "embed"))
			case "docPr":
				alt = attrLocal(t, "descr")
			case "imagedata": // <v:imagedata r:id="rId5"> (VML)
				primero(&rID, attrLocal(t, "id"))
			case "extent", "ext": // tamaño de maqueta en EMU (el primero manda)
				if cxEMU == 0 {
					cxEMU = atoiDefault(attrLocal(t, "cx"), 0)
					cyEMU = atoiDefault(attrLocal(t, "cy"), 0)
				}
			case "group", "shape", "rect", "roundrect", "oval", "wrap":
				elementoVML(t, &forma, &grupo, &enVML, leidas)
			case "txbxContent":
				if c, ok := d.cajaLeida(anclado, ancla, cxEMU, cyEMU, enVML, forma); ok {
					leidas = append(leidas, c)
				}
			case "anchor", "positionH", "positionV", "posOffset", "align", "wrapNone", "wrapSquare", "wrapTight", "wrapThrough", "wrapTopAndBottom":
				d.leerAncla(t, &ancla, &anclado, &enH, &enV)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "positionH":
				enH = false
			case "positionV":
				enV = false
			case "group":
				grupo = docxVML{}
			case until:
				d.cajas = append(d.cajas, leidas...)
				return d.imagenLeida(rID, rSVG, alt, emuToPx(cxEMU), emuToPx(cyEMU))
			}
		}
	}
}

func (d *docxDec) cajaLeida(anclado bool, ancla docxAncla, cxEMU, cyEMU int, enVML bool, forma docxVML) (docxCaja, bool) {
	bloques := d.leerCuadro()
	if len(bloques) == 0 {
		return docxCaja{}, false
	}
	switch {
	case anclado:
		return docxCaja{ancla: ancla, ancho: float64(cxEMU) / 9525, alto: float64(cyEMU) / 9525, anclado: cxEMU > 0, bloques: bloques}, true
	case enVML:
		return docxCaja{ancla: forma.ancla, ancho: forma.ancho, alto: forma.alto, anclado: forma.anclado && forma.ancho > 0, bloques: bloques}, true
	default:
		return docxCaja{bloques: bloques}, true
	}
}

// imagenLeida es el HTML de una imagen del documento: un dibujo editable si es uno exportado
// por Sytue (su SVG lleva la escena) y, si no, el <img> de siempre.
func (d *docxDec) imagenLeida(rID, rSVG, alt string, w, h int) string {
	src := dataURLFromZip(docxRelTarget(d.rels[rID]), d.files)
	if rSVG != "" {
		if fig := dibujoDesdeSVG(d.files[docxRelTarget(d.rels[rSVG])], src, w, h, alt); fig != "" {
			return fig
		}
	}
	return imgTag(src, w, h)
}

// LibreOffice envuelve la misma imagen DrawingML y su respaldo VML en
// mc:AlternateContent. Elegimos la primera representación que entendemos y
// consumimos todo el contenedor: el respaldo no debe duplicar la imagen.
func (d *docxDec) readAlternateImage() string {
	var selected string
	var cuadro bool
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return selected
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			if tok.Name.Local == "drawing" || tok.Name.Local == "pict" {
				if selected == "" && !cuadro {
					antes := len(d.cajas)
					selected = d.readDrawing(tok.Name.Local)
					cuadro = len(d.cajas) > antes // un cuadro de texto ya leído: su respaldo VML lo repetiría
				} else {
					_ = d.dec.Skip()
				}
			}
		case xml.EndElement:
			if tok.Name.Local == "AlternateContent" {
				return selected
			}
		}
	}
}

func (d *docxDec) readRunImage(container string) string {
	if container == "AlternateContent" {
		return d.readAlternateImage()
	}
	return d.readDrawing(container)
}

// docxRelTarget resuelve el destino de una relación de word/document.xml a su
// ruta dentro del paquete ("media/image1.png" -> "word/media/image1.png"). Las
// referencias externas (http…) no se traen: solo se embebe lo que viaja dentro.
func docxRelTarget(target string) string {
	if target == "" || strings.Contains(target, "://") {
		return ""
	}
	return path.Clean("word/" + strings.TrimPrefix(target, "/"))
}

// emuToPx convierte una longitud OOXML en EMU a píxeles CSS (914400 EMU = 1 in = 96 px).
func emuToPx(emu int) int { return emu / 9525 }

// imgTag arma el <img> del editor de documentos: el data:URL como src y el tamaño
// de maqueta en data-w/data-h (fuente de verdad del editor) reflejado en el style
// para que se vea igual antes de tocar la imagen.
func imgTag(src string, w, h int) string {
	if src == "" {
		return ""
	}
	tag := `<img src="` + src + `"`
	if w > 0 && h > 0 {
		wpx, hpx := strconv.Itoa(w), strconv.Itoa(h)
		tag += ` data-w="` + wpx + `" data-h="` + hpx + `"` +
			` style="width:` + wpx + `px;height:` + hpx + `px;max-width:100%"`
	}
	return tag + ">"
}

// readText acumula el CharData de un elemento de texto hasta su cierre.
func (d *docxDec) readText(local string) string {
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

// docxCell es una <w:tc> con su columna de rejilla y su combinación vertical (vMerge).
type docxCell struct {
	xcell
	col  int
	vm   string // "", "restart" o "continue"
	drop bool   // «continue» ya fundida en la celda de arriba
	pr   docxCeldaPr
	ex   docxCeldaPr // excepciones de la fila (tblPrEx)
}

// docxTabla es lo leído de un <w:tbl>: su tblPr, su rejilla y sus filas.
type docxTabla struct {
	pr     docxCeldaPr
	rejill []float64 // anchos de gridCol en px
	filas  [][]docxCell
}

// readRows lee las filas de un <w:tbl> hasta su cierre. Con lookUltimaFila activo retiene la
// última <w:tr> leída hasta ver si hay otra detrás: así condicionar sabe cuál es la fila final y
// aplica el rPr y el pPr de lastRow, swCell y seCell sin guardar la tabla entera en memoria.
func (d *docxDec) readRows(t *docxTabla) {
	var pendiente []xml.Token
	vaciar := func(ultima bool) {
		if len(pendiente) == 0 {
			return
		}
		d.tabla.nfilas = -1
		if ultima {
			d.tabla.nfilas = d.tabla.fila + 1
		}
		dec := d.dec
		d.dec = xml.NewTokenDecoder(&lectorTokens{toks: pendiente})
		pendiente = nil
		if row := d.readRow(); len(row) > 0 {
			t.filas = append(t.filas, row)
			d.tabla.fila++
		}
		d.dec = dec
	}
	for {
		tok, err := d.dec.Token()
		if err != nil {
			break
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "tr":
				if d.tabla.look&lookUltimaFila == 0 || len(d.tabla.cond) == 0 {
					if row := d.readRow(); len(row) > 0 {
						t.filas = append(t.filas, row)
						d.tabla.fila++
					}
				} else if toks := d.leerTokensTr(); len(toks) > 0 {
					vaciar(false)
					pendiente = toks
				}
			case "tblPr":
				d.h().leerCeldaPr(d.dec, "tblPr", &t.pr)
				// El estilo de la tabla se aplica a todo lo que hay dentro (§17.7.6).
				c, p, r, cond := d.h().estiloT(t.pr.estilo)
				d.tabla = nuevaTablaCtx(c, p, r, cond, t.pr)
			case "tblGrid":
				t.rejill = d.readGrid()
				d.tabla.ncols = len(t.rejill)
			case "sdt", "sdtContent", "customXml":
				// controles de contenido alrededor de filas: se entra
			default:
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			if e.Name.Local == "tbl" {
				vaciar(true)
				return
			}
		}
	}
	vaciar(true)
}

func (d *docxDec) leerTokensTr() []xml.Token {
	var toks []xml.Token
	prof, profTbl, tieneTc := 1, 0, false
	for prof > 0 {
		tok, err := d.dec.Token()
		if err != nil {
			break
		}
		toks = append(toks, xml.CopyToken(tok))
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "tr":
				prof++
			case "tbl":
				profTbl++
			case "tc":
				if profTbl == 0 {
					tieneTc = true
				}
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "tr":
				prof--
			case "tbl":
				profTbl--
			}
		}
	}
	if !tieneTc {
		return nil
	}
	return toks
}

type lectorTokens struct {
	toks []xml.Token
}

func (l *lectorTokens) Token() (xml.Token, error) {
	if len(l.toks) == 0 {
		return nil, io.EOF
	}
	t := l.toks[0]
	l.toks = l.toks[1:]
	return t, nil
}

func (d *docxDec) readGrid() []float64 {
	var out []float64
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return out
		}
		switch e := tok.(type) {
		case xml.StartElement:
			if e.Name.Local == "gridCol" {
				n, err := strconv.ParseFloat(attrLocal(e, "w"), 64)
				if err != nil || n < 0 || n > 31680 {
					n = 0
				}
				out = append(out, n/15)
			}
			_ = d.dec.Skip()
		case xml.EndElement:
			if e.Name.Local == "tblGrid" {
				return out
			}
		}
	}
}

// readTable lee un <w:tbl> a un xtable (celdas = sus párrafos). gridSpan pasa a
// colspan; las celdas vMerge «continue» se funden en la «restart» de su columna como rowspan.
func (d *docxDec) readTable() *xtable {
	fuera := d.tabla
	defer func() { d.tabla = fuera }()
	d.tabla = docxTablaCtx{}
	c, p, r, cond := d.h().estiloT("")
	d.tabla = nuevaTablaCtx(c, p, r, cond, docxCeldaPr{})
	var tb docxTabla
	d.readRows(&tb)
	rows := tb.filas
	if len(rows) == 0 {
		return nil
	}
	fundirVMerge(rows)
	t := &xtable{css: d.cssTabla(tb)}
	for i, row := range rows {
		var fila []xcell
		for _, c := range row {
			if !c.drop {
				c.css = d.cssCelda(tb, c, i, len(rows))
				fila = append(fila, c.xcell)
			}
		}
		t.rows = append(t.rows, fila)
	}
	t.normalize()
	return t
}

// cssTabla es el style de la tabla: su ancho y su sangría.
func (d *docxDec) cssTabla(tb docxTabla) string {
	var css docxCSS
	ancho := tb.pr.tblAnch
	if ancho == 0 {
		for _, w := range tb.rejill {
			ancho += w
		}
	}
	if ancho > 0 {
		css.add("width", docxNum(ancho)+"px")
	}
	ind := d.tabla.celda.tblInd
	if tb.pr.tblInd != nil {
		ind = tb.pr.tblInd
	}
	switch {
	case tb.pr.tblJc == "center":
		css.add("margin-left", "auto")
		css.add("margin-right", "auto")
	case tb.pr.tblJc == "right" || tb.pr.tblJc == "end":
		css.add("margin-left", "auto")
	case ind != nil && *ind > 0:
		css.add("margin-left", docxNum(*ind)+"px")
	}
	return css.String()
}

// cssCelda es el style de una celda: bordes, sombreado, márgenes, ancho y alineación vertical.
// Cada nivel pisa al anterior: estilo de la tabla, tblPr, excepciones de la fila y tcPr. Los
// bordes de la tabla van al exterior; insideH e insideV, a los lados que dan a otra celda.
func (d *docxDec) cssCelda(tb docxTabla, c docxCell, fila, nfilas int) string {
	pr := d.tabla.celda
	pr.sobre(tb.pr)
	pr.sobre(c.ex)
	primera, ultima := fila == 0, fila+max(c.rowspan, 1) >= nfilas
	ultimaCol := c.col+max(c.colspan, 1) >= len(tb.rejill)
	lado := bordesCelda(pr, c, primera, ultima, ultimaCol)
	shd, margen, valign := pr.shd, pr.margen, c.pr.valign
	lado, shd = d.tabla.condicionalCelda(lado, shd, fila, nfilas, c, ultimaCol)
	for k := range lado {
		if c.pr.bordes[k] != nil {
			lado[k] = c.pr.bordes[k]
		}
	}
	if c.pr.shd != nil && *c.pr.shd != "" {
		// Un sombreado de celda «auto» deja ver el de la tabla: no es blanco.
		shd = c.pr.shd
	}
	for i := range margen {
		if c.pr.margen[i] != nil {
			margen[i] = c.pr.margen[i]
		}
	}
	var css docxCSS
	cssBordes(&css, lado)
	if shd != nil && *shd != "" {
		css.add("background-color", "#"+*shd)
	}
	if r := cssRelleno(margen); r != "" {
		css.add("padding", r)
	}
	ancho := c.pr.ancho
	if ancho == 0 {
		for k := c.col; k < c.col+max(c.colspan, 1) && k < len(tb.rejill); k++ {
			ancho += tb.rejill[k]
		}
	}
	if ancho > 0 {
		css.add("width", docxNum(ancho)+"px")
	}
	switch valign {
	case "center":
		css.add("vertical-align", "middle")
	case "bottom":
		css.add("vertical-align", "bottom")
	}
	return css.String()
}

// cssBordes escribe los cuatro lados de una celda; si son iguales, en una sola declaración:
// una tabla tiene miles de celdas.
func cssBordes(css *docxCSS, lado [4]*string) {
	if lado[0] != nil && lado[1] != nil && lado[2] != nil && lado[3] != nil && *lado[0] == *lado[1] && *lado[1] == *lado[2] && *lado[2] == *lado[3] {
		css.add("border", *lado[0])
		return
	}
	for i, nombre := range []string{"top", "right", "bottom", "left"} {
		if lado[i] != nil {
			css.add("border-"+nombre, *lado[i])
		}
	}
}

// cssRelleno es el padding de unos márgenes de celda en twips, abreviado como en CSS.
func cssRelleno(margen [4]*float64) string {
	if margen == [4]*float64{} {
		return ""
	}
	var v [4]string
	for i, m := range margen {
		v[i] = "0"
		if m != nil {
			v[i] = docxPx(*m)
		}
	}
	switch {
	case v[0] == v[2] && v[1] == v[3] && v[0] == v[1]:
		return v[0]
	case v[0] == v[2] && v[1] == v[3]:
		return v[0] + " " + v[1]
	}
	return strings.Join(v[:], " ")
}

// bordesCelda son los cuatro lados de una celda: los de la tabla por fuera e insideH e insideV
// por dentro. Lo que diga su tcBorders va encima, después del formato condicional.
func bordesCelda(pr docxCeldaPr, c docxCell, primera, ultima, ultimaCol bool) [4]*string {
	lado := [4]*string{pr.bordes[0], pr.bordes[1], pr.bordes[2], pr.bordes[3]}
	interior := map[int]bool{0: !primera, 1: !ultimaCol, 2: !ultima, 3: c.col > 0}
	for i := range lado {
		if interior[i] {
			lado[i] = pr.bordes[4+i%2] // arriba y abajo, insideH; derecha e izquierda, insideV
		}
	}
	return lado
}

// condicionalCelda pone sobre los bordes y el sombreado de una celda los de las partes de la
// tabla en las que cae.
func (t docxTablaCtx) condicionalCelda(lado [4]*string, shd *string, fila, nfilas int, c docxCell, ultimaCol bool) ([4]*string, *string) {
	primera, ultima := fila == 0, fila+max(c.rowspan, 1) >= nfilas
	for _, reg := range t.regiones(fila, nfilas, c.col, max(c.colspan, 1)) {
		cd := t.cond[reg].celda
		if cd.shd != nil && *cd.shd != "" {
			shd = cd.shd
		}
		// Los bordes de la parte: los suyos por fuera e insideH/insideV entre sus celdas.
		esFila := strings.HasSuffix(reg, "Horz") || reg == "firstRow" || reg == "lastRow"
		esCol := strings.HasSuffix(reg, "Vert") || reg == "firstCol" || reg == "lastCol"
		lado = bordesDeParte(lado, cd.bordes, [4]bool{!esCol || primera, !esFila || ultimaCol, !esCol || ultima, !esFila || c.col == 0})
	}
	return lado, shd
}

// bordesDeParte pone sobre lado los bordes de una parte de la tabla: en cada lado, el de fuera
// de la parte si lo es y el de dentro (insideH o insideV) si no.
func bordesDeParte(lado [4]*string, bordes [6]*string, fuera [4]bool) [4]*string {
	for k := range lado {
		b := bordes[k]
		if !fuera[k] {
			b = bordes[4+k%2]
		}
		if b != nil {
			lado[k] = b
		}
	}
	return lado
}

// fundirVMerge convierte cada celda vMerge «restart» y las «continue» que le siguen en su
// columna en un rowspan de la primera; las continuaciones quedan marcadas `drop`.
func fundirVMerge(rows [][]docxCell) {
	origen := map[int]*docxCell{} // columna → celda combinada que sigue hacia abajo
	for _, row := range rows {
		for i := range row {
			c := &row[i]
			if o := origen[c.col]; c.vm == "continue" && o != nil {
				o.rowspan++
				c.drop = true
				continue
			}
			if c.vm == "restart" {
				c.rowspan = 1
				origen[c.col] = c
				continue
			}
			for k := 0; k < max(c.colspan, 1) && k < maxTableCols; k++ {
				delete(origen, c.col+k)
			}
		}
	}
}

// readRow lee un <w:tr> a sus celdas. gridBefore (celdas vacías antes de la primera)
// no se interpreta: ponytail, rarísimo fuera de tablas pegadas desde otras fuentes.
func (d *docxDec) readRow() []docxCell {
	var row []docxCell
	var ex docxCeldaPr
	col := 0
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return row
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "tc":
				c := d.readCell(col)
				c.col, c.ex = col, ex
				col += max(c.colspan, 1)
				row = append(row, c)
			case "tblPrEx":
				d.h().leerCeldaPr(d.dec, "tblPrEx", &ex)
			case "sdt", "sdtContent", "customXml":
				// controles de contenido alrededor de las celdas: se entra
			default:
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			if e.Name.Local == "tr" {
				return row
			}
		}
	}
}

// readCell lee un <w:tc> a sus párrafos y a su tcPr. Una tabla anidada no cabe en la celda del
// editor: su texto entra como párrafos de la celda, fila a fila, para no perderlo.
func (d *docxDec) readCell(col int) docxCell {
	c := docxCell{}
	d.condicionar(col, 1)
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return c
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "p":
				p, _ := d.readParagraph()
				c.paras = append(c.paras, p)
				for _, caja := range d.cajas {
					for _, b := range caja.bloques {
						if b.para != nil {
							c.paras = append(c.paras, *b.para)
						}
						c.paras = append(c.paras, parrafosDeTabla(b.table)...)
					}
				}
				d.cajas = nil
			case "tcPr":
				c.colspan, c.vm, c.pr = d.readCellProps()
				d.condicionar(col, c.colspan)
			case "tbl":
				c.paras = append(c.paras, parrafosDeTabla(d.readTable())...)
			case "sdt", "sdtContent", "customXml":
			default:
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			if e.Name.Local == "tc" {
				if len(c.paras) == 0 {
					c.paras = []xpara{{}}
				}
				return c
			}
		}
	}
}

// parrafosDeTabla son los párrafos de una tabla, celda a celda: lo que cabe de una tabla anidada.
func parrafosDeTabla(t *xtable) []xpara {
	if t == nil {
		return nil
	}
	var out []xpara
	for _, fila := range t.rows {
		for _, celda := range fila {
			if !celda.covered {
				out = append(out, celda.paras...)
			}
		}
	}
	return out
}

// readCellProps lee <w:tcPr>: gridSpan, vMerge ("restart", "continue" o "") y el formato.
func (d *docxDec) readCellProps() (int, string, docxCeldaPr) {
	var pr docxCeldaPr
	d.h().leerCeldaPr(d.dec, "tcPr", &pr)
	return max(pr.span, 1), pr.vm, pr
}

// --- Mapeos y helpers ---

// Word localiza styleId («Ttulo1», «Cita»): el identificador no es el
// significado. styles.xml conserva el nombre semántico, outlineLvl y basedOn.
// Resolverlos evita perder títulos al abrir documentos de otro idioma.
type docxStyleValue struct {
	Val string `xml:"val,attr"`
}
type docxParagraphStyle struct {
	ID      string          `xml:"styleId,attr"`
	Type    string          `xml:"type,attr"`
	Name    docxStyleValue  `xml:"name"`
	Base    docxStyleValue  `xml:"basedOn"`
	Outline *docxStyleValue `xml:"pPr>outlineLvl"`
}

func parseDocxStyles(data []byte) map[string]string {
	var doc struct {
		Styles []docxParagraphStyle `xml:"style"`
	}
	out := make(map[string]string)
	if xml.Unmarshal(data, &doc) != nil {
		return out
	}
	styles := make(map[string]docxParagraphStyle)
	for _, s := range doc.Styles {
		if s.Type == "paragraph" {
			styles[s.ID] = s
		}
	}
	// Iterativa, con ciclo y profundidad acotados: no recursión sobre entrada
	// ajena. Un estilo desconocido conserva el párrafo y nunca inventa un título.
	for id := range styles {
		out[id] = ""
		seen := make(map[string]bool)
		for cur, depth := id, 0; cur != "" && depth < 64 && !seen[cur]; depth++ {
			seen[cur] = true
			s, ok := styles[cur]
			if !ok {
				break
			}
			if block, resolved := docxParagraphMeaning(s); resolved {
				out[id] = block
				break
			}
			cur = s.Base.Val
		}
	}
	return out
}

func docxParagraphMeaning(s docxParagraphStyle) (string, bool) {
	if s.Outline != nil {
		// Nueve declara texto normal, incluso si hereda de un título.
		if s.Outline.Val == "9" {
			return "", true
		}
		if n, err := strconv.Atoi(s.Outline.Val); err == nil && n >= 0 && n <= 5 {
			return "Heading" + strconv.Itoa(n+1), true
		}
	}
	block := docxStyleToBlock(s.Name.Val)
	if block == "" {
		block = docxStyleToBlock(s.ID)
	}
	return block, block != ""
}

// docxStyleToBlock mapea un styleId de párrafo al estilo de bloque interno.
func docxStyleToBlock(styleID string) string {
	styleID = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(styleID)), " ", "")
	switch {
	case styleID == "title":
		return "Heading1"
	case styleID == "quote" || styleID == "intensequote" || styleID == "quotations":
		// «Quotations» es el estilo de cita de LibreOffice, que no lo traduce al guardar.
		return "Quote"
	case strings.HasPrefix(styleID, "heading"):
		if n := atoiDefault(styleID[len("heading"):], 0); n >= 1 && n <= 6 {
			return "Heading" + strconv.Itoa(n)
		}
	}
	return ""
}

// docxJcToAlign mapea w:jc a la alineación interna (both -> justify).
func docxJcToAlign(val string) string {
	switch val {
	case "center":
		return "center"
	case "right", "end":
		return "right"
	case "both", "distribute", "justify":
		return "justify"
	}
	return ""
}

// docxNumeracion es el formato de las listas de numbering.xml: numId -> formato por nivel.
type docxNumeracion map[string][9]docxNivel

// docxNivel es un nivel de lista: su numFmt y, en una de viñetas, la viñeta si es un carácter
// corriente («-» de Pages, «o»); la de una fuente de símbolos (Symbol, Wingdings) no tiene
// equivalente en Unicode y se queda la del editor.
type docxNivel struct{ fmt, vineta string }

// vineta es la viñeta propia del nivel lvl de numId, o "" si es la de siempre.
func (n docxNumeracion) vineta(numID string, lvl int) string {
	if v := n[numID][lvl]; v.fmt == "bullet" {
		return v.vineta
	}
	return ""
}

// tipo es "bullet" o "number" para el nivel lvl de numId, o "" si no hay lista (numId 0 quita
// la numeración heredada, §17.9.18).
func (n docxNumeracion) tipo(numID string, lvl int) string {
	if numID == "" || numID == "0" {
		return ""
	}
	fmts, ok := n[numID]
	if !ok {
		return ""
	}
	switch fmts[lvl].fmt {
	case "bullet":
		return "bullet"
	case "none":
		return "" // sin marca: la numeración de capítulos de LibreOffice en sus títulos
	}
	return "number"
}

// parseNumbering lee numbering.xml. El formato de cada nivel sale del abstractNum, de su
// numStyleLink (el abstractNum puede delegar en un estilo de numeración, que apunta a otro
// numId: así escriben las listas Pages y Word) y de los lvlOverride del num.
func parseNumbering(data []byte, h *docxHoja) docxNumeracion {
	out := docxNumeracion{}
	if len(data) == 0 {
		return out
	}
	type lvl struct {
		Ilvl   string `xml:"ilvl,attr"`
		NumFmt struct {
			Val string `xml:"val,attr"`
		} `xml:"numFmt"`
		Texto struct {
			Val string `xml:"val,attr"`
		} `xml:"lvlText"`
		Fuente struct {
			ASCII string `xml:"ascii,attr"`
		} `xml:"rPr>rFonts"`
	}
	var doc struct {
		Abstract []struct {
			ID   string `xml:"abstractNumId,attr"`
			Link struct {
				Val string `xml:"val,attr"`
			} `xml:"numStyleLink"`
			Lvl []lvl `xml:"lvl"`
		} `xml:"abstractNum"`
		Num []struct {
			NumID    string `xml:"numId,attr"`
			Abstract struct {
				Val string `xml:"val,attr"`
			} `xml:"abstractNumId"`
			Override []struct {
				Lvl []lvl `xml:"lvl"`
			} `xml:"lvlOverride"`
		} `xml:"num"`
	}
	if xml.Unmarshal(data, &doc) != nil {
		return out
	}
	poner := func(f *[9]docxNivel, ls []lvl) {
		for _, l := range ls {
			if i := atoiDefault(l.Ilvl, 0); i < 9 && l.NumFmt.Val != "" {
				f[i] = docxNivel{fmt: l.NumFmt.Val, vineta: vinetaPropia(l.Texto.Val, l.Fuente.ASCII)}
			}
		}
	}
	abs := map[string][9]docxNivel{}
	enlace := map[string]string{}
	for _, a := range doc.Abstract {
		var f [9]docxNivel
		poner(&f, a.Lvl)
		abs[a.ID] = f
		enlace[a.ID] = a.Link.Val
	}
	numAbs := map[string]string{}
	for _, n := range doc.Num {
		numAbs[n.NumID] = n.Abstract.Val
	}
	// Sigue numStyleLink: abstractNum -> estilo de numeración -> su numId -> su abstractNum.
	resolver := func(id string) [9]docxNivel {
		for range 8 {
			var e *docxEstilo
			if h != nil {
				e = h.estilos[enlace[id]]
			}
			if e == nil || e.p.numID == nil || numAbs[*e.p.numID] == "" || numAbs[*e.p.numID] == id {
				break
			}
			id = numAbs[*e.p.numID]
		}
		return abs[id]
	}
	for _, n := range doc.Num {
		f := resolver(n.Abstract.Val)
		for _, o := range n.Override {
			poner(&f, o.Lvl)
		}
		for i := 1; i < len(f); i++ {
			if f[i].fmt == "" {
				f[i] = f[i-1] // un nivel sin definir sigue al de arriba
			}
		}
		out[n.NumID] = f
	}
	return out
}

// vinetaPropia es la viñeta de un lvlText de viñetas si es un solo carácter corriente que no es
// el disco del editor; "" si no (la de siempre). ponytail: las de Symbol y Wingdings (el «·» de
// Word en su área privada) se quedan en el disco.
func vinetaPropia(texto, fuente string) string {
	f := strings.ToLower(fuente)
	if strings.Contains(f, "symbol") || strings.Contains(f, "wingdings") || strings.Contains(f, "webdings") {
		return ""
	}
	r := []rune(texto)
	if len(r) != 1 || r[0] == '•' || r[0] < 0x21 || r[0] >= 0xE000 && r[0] <= 0xF8FF || strings.ContainsRune(`'"\`, r[0]) {
		return ""
	}
	return texto
}

// attrLocal devuelve el valor del atributo con el nombre local dado.
func attrLocal(se xml.StartElement, local string) string {
	for _, a := range se.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// boolAttr interpreta un toggle OOXML (<w:b/>, <w:b w:val="0"/>, ...). Presencia
// sin val = activado; val "0"/"false"/"off" = desactivado.
func boolAttr(se xml.StartElement) bool {
	switch attrLocal(se, "val") {
	case "0", "false", "off":
		return false
	default:
		return true
	}
}

// normalizeHexColor normaliza un color OOXML/DrawingML ("FF0000", "auto") a
// "RRGGBB" en mayúsculas; "" si es auto/vacío/ inválido.
func normalizeHexColor(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.EqualFold(v, "auto") {
		return ""
	}
	return normalizeColor("#" + v)
}

func atoiDefault(s string, def int) int {
	n := 0
	if s == "" {
		return def
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return def
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}

func wrapFormat(msg string) error {
	return &formatError{msg: msg}
}

type formatError struct{ msg string }

func (e *formatError) Error() string { return e.msg }
func (e *formatError) Unwrap() error { return ErrInvalidFormat }

// readSect lee el <w:sectPr> final del cuerpo: las partes de encabezado y pie y titlePg.
// ponytail: solo la última sección y las partes «default»/«first»; sin pares/impares.
func (d *docxDec) readSect() {
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			id, typ := attrLocal(t, "id"), attrLocal(t, "type")
			switch {
			case t.Name.Local == "headerReference" && typ == "default":
				d.sect.header = id
			case t.Name.Local == "headerReference" && typ == "first":
				d.sect.firstHeader = id
			case t.Name.Local == "footerReference" && typ == "default":
				d.sect.footer = id
			case t.Name.Local == "titlePg":
				d.sect.titlePg = boolAttr(t)
			case t.Name.Local == "pgSz":
				d.sect.ancho, d.sect.alto = twipsDe(t, "w"), twipsDe(t, "h")
			case t.Name.Local == "pgMar":
				d.sect.conMargenes = true
				for i, lado := range []string{"top", "right", "bottom", "left"} {
					d.sect.margenes[i] = math.Abs(twipsDe(t, lado)) // negativo: el texto no empuja el margen
				}
			}
			_ = d.dec.Skip()
		case xml.EndElement:
			if t.Name.Local == "sectPr" {
				return
			}
		}
	}
}

func twipsDe(t xml.StartElement, attr string) float64 {
	n, err := strconv.ParseFloat(attrLocal(t, attr), 64)
	if err != nil || math.Abs(n) > 31680*2 {
		return 0
	}
	return n
}

// capaPagina es la página del documento (tamaño, márgenes y color del papel) como la capa
// que guardan el editor y rico (data-sytue-page, en px). Lo que la sección no dice es lo del
// editor: A4 y 96 px de margen.
func (d *docxDec) capaPagina() string {
	s := d.sect
	if s.ancho == 0 && s.alto == 0 && !s.conMargenes && s.fondo == "" {
		return ""
	}
	px := func(tw, defecto float64) string {
		if tw <= 0 {
			return docxNum(defecto)
		}
		return docxNum(tw / 15)
	}
	m := []string{"96", "96", "96", "96"}
	if s.conMargenes {
		for i, tw := range s.margenes {
			m[i] = docxNum(tw / 15)
		}
	}
	out := `<div data-sytue-page="" data-width="` + px(s.ancho, 794) + `" data-height="` + px(s.alto, 1123) +
		`" data-margins="` + strings.Join(m, " ") + `"`
	if s.fondo != "" {
		out += ` data-color="#` + s.fondo + `"`
	}
	return out + ` contenteditable="false" aria-hidden="true"></div>`
}

// regionHTML es el texto de la parte a la que apunta el rId, o "" si no hay.
func (d *docxDec) regionHTML(id, elem string) string {
	part := docxRelTarget(d.rels[id])
	if id == "" || part == "" {
		return ""
	}
	return parseDocxPart(d.files, part, elem, d.hoja, d.presupuesto)[""]
}

// withRegions pone el encabezado delante del cuerpo y el pie al final, con el HTML que el
// editor ya conoce (regiones.go): un div[data-doc-header|footer] con el texto en línea.
// Con titlePg y sin encabezado propio en la primera página, el encabezado se oculta en ella.
func (d *docxDec) withRegions(blocks []xblock) []xblock {
	if h := d.regionHTML(d.sect.header, "hdr"); h != "" {
		first := ""
		if d.sect.titlePg && d.regionHTML(d.sect.firstHeader, "hdr") == "" {
			first = " data-first-page-different"
		}
		blocks = append([]xblock{{raw: `<div data-doc-header` + first + ` style="` + docHeaderStyle + `">` + h + `</div>`}}, blocks...)
	}
	if f := d.regionHTML(d.sect.footer, "ftr"); f != "" {
		blocks = append(blocks, xblock{raw: `<div data-doc-footer style="` + docFooterStyle + `">` + f + `</div>`})
	}
	return blocks
}

// h es la hoja de estilos de la lectura; una lectura montada sin ella (las pruebas que leen un
// tramo suelto) usa una vacía.
func (d *docxDec) h() *docxHoja {
	if d.hoja == nil {
		d.hoja = parseDocxHoja(nil)
	}
	return d.hoja
}

// docxCaracter es el texto de un carácter que Word guarda como elemento: el guion que no parte,
// el guion opcional, el tabulador absoluto y un símbolo (w:sym) fuera del área privada de las
// fuentes de símbolos, que no tiene equivalente Unicode.
func docxCaracter(t xml.StartElement) string {
	switch t.Name.Local {
	case "noBreakHyphen":
		return "\u2011"
	case "softHyphen":
		return "\u00ad"
	case "ptab":
		return "\t"
	}
	n, err := strconv.ParseUint(attrLocal(t, "char"), 16, 32)
	if err != nil || n < 0x20 || n >= 0xF000 && n <= 0xF0FF || n > 0x10FFFF {
		return ""
	}
	return string(rune(n))
}

// docxUltimo es lo que contextualSpacing necesita del párrafo anterior.
type docxUltimo struct {
	estilo     string
	contextual bool
}

// contextual aplica contextualSpacing (§17.3.1.9) a dos párrafos seguidos del mismo estilo: el
// que lo lleva no pone su espacio hacia el otro. Es lo que junta los elementos de una lista de
// Word, que llevan un espacio después que solo vale tras el último.
func contextual(antes, ahora *xpara, ctxAntes, ctxAhora bool) {
	if ctxAntes {
		antes.css = conDeclaracion(antes.css, "margin-bottom", "0")
	}
	// Arriba solo un título tiene margen en el editor (editor.css): un párrafo sin el suyo no
	// necesita «margin-top:0», que además no volvería de una ida y vuelta por .docx.
	if ctxAhora && (headingLevel(ahora.style) > 0 || strings.Contains(";"+ahora.css, ";margin-top:")) {
		ahora.css = conDeclaracion(ahora.css, "margin-top", "0")
	}
}

// conDeclaracion pone prop:val en una lista de declaraciones de CSS, en su sitio si ya estaba.
func conDeclaracion(css, prop, val string) string {
	decl := strings.Split(css, ";")
	for i, d := range decl {
		if k, _, _ := strings.Cut(d, ":"); k == prop {
			decl[i] = prop + ":" + val
			return strings.Join(decl, ";")
		}
	}
	return strings.TrimPrefix(css+";"+prop+":"+val, ";")
}

// heredables son las propiedades de un tramo que se pueden subir a su bloque: las que el texto
// hereda en CSS.
var heredables = map[string]bool{"font-family": true, "font-size": true}

// subirComun sube al bloque la tipografía y el tamaño que comparten todos sus tramos de texto.
// Word repite el formato directo en cada tramo, y sin esto cada uno sería su propio <span> con
// lo mismo: el documento crecía hasta no caber, y lo que se escribiera después en el párrafo no
// tendría ese formato. El color no sube: la exportación a .docx lo lee de los tramos.
func subirComun(p *xpara) {
	texto := func(r xrun) bool { return r.text != "" && r.raw == "" && r.field == "" && r.image == nil }
	var comun map[string]string
	primero := true
	for _, r := range p.runs {
		if !texto(r) {
			continue
		}
		decl := map[string]string{}
		for _, d := range strings.Split(r.css, ";") {
			if k, v, ok := strings.Cut(d, ":"); ok && heredables[k] {
				decl[k] = v
			}
		}
		if primero {
			comun, primero = decl, false
			continue
		}
		for k, v := range comun {
			if decl[k] != v {
				delete(comun, k)
			}
		}
	}
	if len(comun) == 0 {
		return
	}
	for i := range p.runs {
		r := &p.runs[i]
		if !texto(*r) {
			continue
		}
		var quedan []string
		for _, d := range strings.Split(r.css, ";") {
			if k, _, _ := strings.Cut(d, ":"); d != "" && comun[k] == "" {
				quedan = append(quedan, d)
			}
		}
		r.css = strings.Join(quedan, ";")
	}
	for _, k := range []string{"font-family", "font-size"} {
		if v := comun[k]; v != "" {
			p.css = conDeclaracion(p.css, k, v)
		}
	}
}

// leerCuadro lee el contenido de un cuadro de texto (w:txbxContent, de DrawingML o de VML): su
// texto es del documento, y Pages pone en cuadros de texto la portada, el pie de una carta y, en
// sus plantillas de maquetación, el documento entero.
func (d *docxDec) leerCuadro() []xblock {
	par, ultimo, cajas, finPag := d.par, d.ultimo, d.cajas, d.finPag
	d.cajas = nil
	bloques, _ := d.readBlocks("txbxContent")
	d.par, d.ultimo, d.cajas, d.finPag = par, ultimo, cajas, finPag
	return bloques
}
