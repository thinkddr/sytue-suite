// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	xhtml "golang.org/x/net/html"
)

// Importador clean-room: el REVERSO del exportador (export.go). Abre ficheros
// ofimáticos ABIERTOS y los convierte al modelo inicial de un documento nuevo de
// Sytue Docs/Sheets/Slides:
//
//   - .docx/.odt/.txt/.md/.html -> KindDocument     (contenido = HTML, doc:html)
//   - .xlsx/.ods/.csv           -> KindSpreadsheet  (contenido = JSON SheetModel, doc:sheet)
//   - .pptx/.odp                -> KindPresentation (contenido = JSON SlidesModel, doc:slides)
//
// Se usa exclusivamente la stdlib de Go (archive/zip, encoding/xml, encoding/csv)
// y el tokenizador HTML de golang.org/x/net/html (BSD, ya dependencia). NO se
// porta código AGPL (OnlyOffice/LibreOffice quedan fuera): el mapeo OOXML/ODF es
// obra original de Sytue sobre las especificaciones públicas ECMA-376 y OASIS ODF.
//
// El resultado es el valor que el editor guarda en el registro LWW del tipo
// (CONTENT_KEY del frontend); el llamante crea un documento del kind devuelto y
// siembra ese contenido con una única operación `set`.

// MaxImportBytes es el tamaño máximo aceptado de un fichero a importar. Cota la
// memoria del parseo (los paquetes OPC/ODF se leen completos a memoria). 25 MiB
// cubre documentos ofimáticos reales holgadamente.
const MaxImportBytes = 25 << 20

// ImportResult es el modelo inicial de un documento nuevo tras importar.
type ImportResult struct {
	Kind    Kind   `json:"kind"`
	Title   string `json:"title"`
	Content string `json:"content"` // valor de CONTENT_KEY[kind]: HTML | JSON(SheetModel) | JSON(SlidesModel)
	// Comments solo los trae ImportWithComments (ver comments_office.go).
	Comments []ImportedComment `json:"comments,omitempty"`
	// Template: el fichero era una plantilla (.dotx .xltx .potx .ott .ots .otp). Abrirla
	// crea un documento nuevo cada vez y no se ancla al fichero (import_plantilla.go).
	Template bool `json:"template,omitempty"`
	// Hashes son las contraseñas de protección del fichero, ya derivadas, por ámbito: el
	// cliente las devuelve en `proteccion.hashes` al sembrar el contenido (proteccion_fichero.go).
	Hashes map[string]ClaveWire `json:"hashes,omitempty"`
	// Truncated: el libro no cabía entero y se ha abierto recortado, con una hoja de aviso
	// delante (recortarLibro). Como una plantilla, abrirlo crea un documento aparte que NO se
	// ancla al fichero: guardar no puede sobrescribir el original con la copia recortada.
	Truncated bool `json:"truncated,omitempty"`
}

// Import detecta el formato por extensión (con respaldo por firma/magic),
// convierte el fichero y devuelve el modelo inicial. `filename` da el título y
// la pista de extensión; `data` es el contenido completo del fichero.
func Import(filename string, data []byte) (*ImportResult, error) {
	return importar(filename, data, false, "")
}

// ImportWithComments es Import que además devuelve los comentarios del fichero y deja su
// ancla en el HTML. Solo lo llama quien va a guardarlos (ver comments_office.go).
func ImportWithComments(filename string, data []byte) (*ImportResult, error) {
	return importar(filename, data, true, "")
}

// ImportLocalized es Import (o ImportWithComments, con `comments`) para quien sabe el idioma
// de la persona que abre el fichero: los avisos que el importador escribe dentro del
// documento salen en `locale`, con el inglés de respaldo (import_avisos.go).
func ImportLocalized(filename string, data []byte, comments bool, locale string) (*ImportResult, error) {
	return importar(filename, data, comments, locale)
}

func importar(filename string, data []byte, comments bool, locale string) (*ImportResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: fichero vacío", ErrInvalidFormat)
	}
	if len(data) > MaxImportBytes {
		return nil, fmt.Errorf("%w: fichero demasiado grande (máx %d MiB)", ErrInvalidFormat, MaxImportBytes>>20)
	}
	title := titleFromFilename(filename)
	f := detectFormat(filename, data)
	// El presupuesto corta la conversión en cuanto lo convertido ya no puede caber (ver
	// import_presupuesto.go): antes que cualquier error de lectura, que el corte provoca.
	p := &presupuesto{}
	kind, content, imported, err := convert(f, data, comments, locale, p)
	if p.agotado() {
		err = ErrDemasiadoGrande
	}
	var hashes map[string]ClaveWire
	if err == nil {
		if kind == KindSpreadsheet {
			content, hashes = extraerClavesImportadas(content)
		}
		// Lo convertido se siembra con una sola operación: tiene que caber (ver
		// import_presupuesto.go).
		content, err = caberEnElDocumento(content)
	}
	recortado := false
	if errors.Is(err, ErrDemasiadoGrande) {
		switch f { //nolint:exhaustive // solo los formatos de paquete que se recortan como copia aparte
		case fmtXlsx, fmtOds:
			// Un libro que no cabe se abre recortado, como copia aparte (recortarLibro).
			kind, imported = KindSpreadsheet, nil
			content, hashes, recortado, err = recortarLibro(f, data, locale)
		case fmtDocx:
			kind, imported = KindDocument, nil
			content, recortado, err = recortarDocx(content, locale)
		case fmtPptx:
			kind, imported = KindPresentation, nil
			content, recortado, err = recortarPptx(content, locale)
		}
	}
	if err != nil {
		return nil, err
	}
	return &ImportResult{
		Kind: kind, Title: title, Content: content, Comments: imported,
		Template: esPlantillaDeOficina(filename, data), Hashes: hashes, Truncated: recortado,
	}, nil
}

// srcFormat es el formato de origen detectado.
type srcFormat int

const (
	fmtUnknown srcFormat = iota
	fmtDocx
	fmtXlsx
	fmtPptx
	fmtOdt
	fmtOds
	fmtOdp
	fmtCSV
	fmtText
	fmtMarkdown
	fmtHTML
	fmtPDF
	// iWork (import_iwork.go): fmtIWork es un paquete de iWork de aplicación desconocida.
	fmtPages
	fmtNumbers
	fmtKeynote
	fmtIWork
)

// convert despacha al parser del formato y devuelve (kind, contenido).
func convert(f srcFormat, data []byte, comments bool, locale string, p *presupuesto) (Kind, string, []ImportedComment, error) {
	if esIWork(f) {
		k, c := importIWork(f, data, locale)
		return k, c, nil, nil
	}
	if f == fmtDocx {
		return importDocxComments(data, comments, p)
	}
	if f == fmtOdt {
		return importOdtComments(data, comments, p)
	}
	k, c, err := convertPlano(f, data, p)
	return k, c, nil, err
}

// convertPlano es convert para lo que no lleva comentarios.
func convertPlano(f srcFormat, data []byte, p *presupuesto) (Kind, string, error) {
	switch f {
	case fmtXlsx:
		return importXlsx(data, p)
	case fmtOds:
		return importOds(data, p)
	case fmtCSV:
		return KindSpreadsheet, importCSV(data), nil
	case fmtPptx:
		return importPptx(data, p)
	case fmtPDF:
		return importPDF(data)
	case fmtOdp:
		return importOdp(data, p)
	case fmtMarkdown:
		return KindDocument, importMarkdown(string(data)), nil
	case fmtHTML:
		return KindDocument, importHTMLDoc(data), nil
	case fmtText:
		return KindDocument, importPlainText(string(data)), nil
	default:
		return "", "", fmt.Errorf("%w: formato de importación no reconocido", ErrInvalidFormat)
	}
}

// detectFormat clasifica por extensión y, si es desconocida o ambigua, olfatea
// la firma del contenido (paquete ZIP OPC/ODF vs texto/HTML).
func detectFormat(filename string, data []byte) srcFormat {
	// La FIRMA manda sobre la extensión siempre que sea inequívoca. Un fichero
	// binario leído como texto plano volcaría su contenido crudo dentro del
	// documento: un destrozo silencioso, y de los que nadie relaciona con el nombre
	// del fichero. Pasaba con el PDF y pasaba igual con todo el resto: un .docx
	// renombrado a .txt (o un .xlsx adjuntado sin extensión) entraba como texto y
	// el documento quedaba lleno de «PK…». La firma solo decide cuando es
	// concluyente (cabecera PDF, o paquete ZIP con partes ofimáticas dentro); si
	// no, manda la extensión como antes.
	if bytes.HasPrefix(data, []byte("%PDF-")) {
		return fmtPDF
	}
	if isZipPackage(data) {
		if f := sniffIWork(filename, data); f != fmtUnknown {
			return f
		}
		if f := sniffZip(data); f != fmtUnknown {
			return f
		}
		// ZIP que no declara partes ofimáticas: solo la extensión puede salvarlo
		// (p. ej. un .odt sin entrada «mimetype»), y solo si es de un formato
		// empaquetado. Cualquier otra cosa se rechaza: leer un ZIP como texto
		// llenaba el documento de «PK…».
		if f := extFormat(filename); isZipFormat(f) {
			return f
		}
		return fmtUnknown
	}
	// Fuera de un ZIP, «.key» es casi siempre una clave PEM, no una presentación: no se
	// le hace caso a esa extensión y el fichero entra como texto, como hasta ahora.
	if f := extFormat(filename); f != fmtUnknown && f != fmtKeynote {
		return f
	}
	return sniffFormat(data)
}

// extFormat clasifica por la extensión del nombre de fichero.
func extFormat(filename string) srcFormat {
	if f, ok := extPlantilla[strings.ToLower(path.Ext(filename))]; ok {
		return f
	}
	switch strings.ToLower(path.Ext(filename)) {
	case ".docx":
		return fmtDocx
	case ".xlsx":
		return fmtXlsx
	case ".pptx":
		return fmtPptx
	case ".odt":
		return fmtOdt
	case ".ods":
		return fmtOds
	case ".odp":
		return fmtOdp
	case ".csv", ".tsv":
		return fmtCSV
	case ".md", ".markdown":
		return fmtMarkdown
	case ".html", ".htm":
		return fmtHTML
	case ".txt", ".text":
		return fmtText
	case ".pdf":
		return fmtPDF
	}
	return extIWork(filename)
}

// isZipPackage indica si los bytes empiezan por la firma de un ZIP (OPC/ODF).
func isZipPackage(data []byte) bool {
	return bytes.HasPrefix(data, []byte("PK\x03\x04")) || bytes.HasPrefix(data, []byte("PK\x05\x06"))
}

// isZipFormat indica si el formato viaja dentro de un paquete ZIP.
func isZipFormat(f srcFormat) bool {
	switch f {
	case fmtDocx, fmtXlsx, fmtPptx, fmtOdt, fmtOds, fmtOdp, fmtPages, fmtNumbers, fmtKeynote, fmtIWork:
		return true
	default:
		// El resto —texto, CSV, Markdown, HTML, PDF y el desconocido— son
		// ficheros sueltos: no hay paquete que abrir. El `default` es explícito
		// para que añadir un formato nuevo obligue a pasar por aquí y decidir
		// de qué lado cae, en vez de heredar el «no» por descarte.
		return false
	}
}

// sniffFormat deduce el formato de un fichero sin extensión reconocida ni firma
// binaria concluyente: HTML o texto plano.
func sniffFormat(data []byte) srcFormat {
	head := strings.ToLower(strings.TrimSpace(string(firstN(data, 512))))
	if strings.HasPrefix(head, "<!doctype html") || strings.HasPrefix(head, "<html") {
		return fmtHTML
	}
	return fmtText
}

// sniffZip abre un paquete ZIP y decide el formato por su mimetype (ODF) o por
// las partes presentes (OOXML: word/ xl/ ppt/). Solo mira el directorio del ZIP y la
// entrada «mimetype»: descomprimir el paquete entero para olfatearlo duplicaba la
// memoria de cada importación (import_presupuesto.go).
func sniffZip(data []byte) srcFormat {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmtUnknown
	}
	nombres := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		nombre := path.Clean(strings.ReplaceAll(f.Name, `\`, "/"))
		nombres[nombre] = nil
		if nombre == "mimetype" {
			if rc, err := f.Open(); err == nil {
				nombres[nombre], _ = io.ReadAll(io.LimitReader(rc, 256))
				rc.Close()
			}
		}
	}
	mime := strings.TrimSpace(string(nombres["mimetype"]))
	if f, ok := mimetypesOdfPlantilla[mime]; ok {
		return f
	}
	switch mime {
	case "application/vnd.oasis.opendocument.text":
		return fmtOdt
	case "application/vnd.oasis.opendocument.spreadsheet":
		return fmtOds
	case "application/vnd.oasis.opendocument.presentation":
		return fmtOdp
	}
	switch {
	case hasPrefixKey(nombres, "word/"):
		return fmtDocx
	case hasPrefixKey(nombres, "xl/"):
		return fmtXlsx
	case hasPrefixKey(nombres, "ppt/"):
		return fmtPptx
	}
	return fmtUnknown
}

func hasPrefixKey(m map[string][]byte, prefix string) bool {
	for k := range m {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

func firstN(b []byte, n int) []byte {
	if len(b) < n {
		return b
	}
	return b[:n]
}

// titleFromFilename deriva un título del nombre de fichero (sin ruta ni
// extensión). Vacío -> "Sin título" (el servicio lo normaliza igualmente).
func titleFromFilename(filename string) string {
	base := path.Base(strings.ReplaceAll(filename, "\\", "/"))
	if ext := path.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return "Sin título"
	}
	return base
}

// --- Texto plano / Markdown / HTML -> doc:html ---

// importPlainText envuelve cada línea no vacía en un párrafo. Las líneas en
// blanco separan párrafos (se colapsan saltos consecutivos).
func importPlainText(text string) string {
	var b strings.Builder
	for _, para := range splitParagraphs(text) {
		para = strings.TrimRight(para, "\r")
		if strings.TrimSpace(para) == "" {
			continue
		}
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(para))
		b.WriteString("</p>")
	}
	if b.Len() == 0 {
		return "<p></p>"
	}
	return b.String()
}

// importHTMLDoc normaliza un HTML externo al subconjunto del editor: se parsea
// con el tokenizador (reusa rtBuilder del exportador) y se re-serializa con
// blocksToHTML. Así entra cualquier HTML y sale el modelo canónico doc:html.
func importHTMLDoc(data []byte) string {
	doc, err := xhtml.Parse(bytes.NewReader(data))
	if err != nil {
		return importPlainText(string(data))
	}
	b := &rtBuilder{}
	if body := findNode(doc, "body"); body != nil {
		for c := body.FirstChild; c != nil; c = c.NextSibling {
			b.walk(c, xrun{})
		}
	} else {
		b.walk(doc, xrun{})
	}
	b.flush()
	out := blocksToHTML(b.blocks)
	if out == "" {
		return "<p></p>"
	}
	return out
}

// findNode busca en profundidad el primer elemento con el nombre dado.
func findNode(n *xhtml.Node, name string) *xhtml.Node {
	if n.Type == xhtml.ElementNode && n.Data == name {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if got := findNode(c, name); got != nil {
			return got
		}
	}
	return nil
}

// --- CSV -> doc:sheet ---

// importCSV parsea un CSV/TSV a un SheetModel de una hoja. El separador se
// autodetecta (coma/punto y coma/tabulador) sobre la primera línea con datos.
func importCSV(data []byte) string {
	text := strings.TrimPrefix(string(data), "\uFEFF") // BOM UTF-8
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = detectDelimiter(text)
	r.FieldsPerRecord = -1 // filas de ancho variable
	r.LazyQuotes = true
	rows, err := r.ReadAll()
	if err != nil || len(rows) == 0 {
		return marshalSheetModel(&sheetModel{Sheets: []sheetData{{Name: "Hoja1", Cells: map[string]string{}}}})
	}
	cells := map[string]string{}
	for ri, row := range rows {
		for ci, val := range row {
			if val == "" {
				continue
			}
			cells[A1Ref(ri+1, ci+1)] = html.EscapeString(val)
		}
	}
	return marshalSheetModel(&sheetModel{Sheets: []sheetData{{Name: "Hoja1", Cells: cells}}})
}

// detectDelimiter elige el separador más frecuente en la primera línea con
// contenido entre coma, punto y coma y tabulador (coma por defecto).
func detectDelimiter(text string) rune {
	line := text
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		line = text[:i]
	}
	best, bestN := ',', -1
	for _, d := range []rune{',', ';', '\t'} {
		if n := strings.Count(line, string(d)); n > bestN {
			best, bestN = d, n
		}
	}
	return best
}

// marshalSheetModel serializa un SheetModel al valor del registro doc:sheet.
func marshalSheetModel(m *sheetModel) string {
	b, err := json.Marshal(m)
	if err != nil {
		return `{"active":0,"sheets":[{"name":"Hoja1","cells":{}}]}`
	}
	return string(b)
}

// marshalSlidesModel serializa un SlidesModel al valor del registro doc:slides.
func marshalSlidesModel(m *slidesModel) string {
	b, err := json.Marshal(m)
	if err != nil {
		return `{"slides":[]}`
	}
	return string(b)
}

// --- Escritor de HTML del editor (reverso de parseHTMLBlocks) ---

// blocksToHTML serializa el modelo rico (xblock) al HTML canónico del editor
// (doc:html): títulos <h1..6>, cita <blockquote>, listas <ul>/<ol>, tablas y
// runs con <strong>/<em>/<u>/<s>/<span color>/<a>. Es el inverso de
// parseHTMLBlocks (export_richtext.go), de modo que import->export->import es
// estable.
func blocksToHTML(blocks []xblock) string {
	var b strings.Builder
	ls := listStack{b: &b}
	for _, blk := range blocks {
		switch {
		case blk.para != nil && blk.para.list != "":
			ls.item(*blk.para)
		case blk.para != nil:
			ls.closeAll()
			b.WriteString(paraToHTML(*blk.para))
		case blk.table != nil:
			ls.closeAll()
			b.WriteString(tableToHTML(blk.table))
		case blk.hr:
			ls.closeAll()
			b.WriteString("<hr>")
		case blk.raw != "":
			ls.closeAll()
			b.WriteString(blk.raw)
		}
	}
	ls.closeAll()
	return b.String()
}

// --- Notas al pie ---
//
// El editor las guarda en el propio HTML del folio: un marcador superíndice donde
// va la llamada y UNA sección al final con la lista de notas (Referencias.tsx).
// Sin restituirlas, el texto de la nota —a menudo la letra pequeña que sostiene
// una afirmación— se perdía entero al importar.

// El estilo en línea del encabezado y el pie, el mismo de Insertar.tsx (HEADER_STYLE y
// FOOTER_STYLE) y de apps/fyne/editor/docus/rico/regiones.go: el HTML es el modelo y el
// editor lo reconoce por él.
const (
	docHeaderStyle = "position:absolute;left:var(--doc-page-margin,96px);right:var(--doc-page-margin,96px);" +
		"top:calc(var(--doc-page-margin,96px) - 30px);" +
		"display:flex;align-items:flex-end;gap:8px;min-height:20px;padding-bottom:6px;" +
		"border-bottom:1px solid rgba(15,23,42,0.16);font-size:12px"
	docFooterStyle = "position:absolute;left:var(--doc-page-margin,96px);right:var(--doc-page-margin,96px);" +
		"bottom:calc(var(--doc-page-margin,96px) - 30px);" +
		"display:flex;align-items:flex-start;gap:8px;min-height:20px;padding-top:6px;" +
		"border-top:1px solid rgba(15,23,42,0.16);font-size:12px"
)

// footnoteMarker arma el marcador superíndice de la nota n (1-based).
func footnoteMarker(n int) string {
	return footnoteMarkerClass(n, "footnote")
}

func footnoteMarkerClass(n int, class string) string {
	s := strconv.Itoa(n)
	extra := ""
	if class == "endnote" {
		extra = ` data-note-class="endnote"`
	}
	return `<sup data-footnote-ref="` + s + `"` + extra + ` id="fnref-` + s + `"><a href="#fn-` + s + `">` + s + `</a></sup>`
}

// footnotesSection arma la sección final con las notas ya serializadas a HTML
// (en el orden de sus llamadas); "" si no hay ninguna.
func footnotesSection(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section data-footnotes><hr><ol>`)
	for i, n := range notes {
		s := strconv.Itoa(i + 1)
		b.WriteString(`<li id="fn-` + s + `" value="` + s + `">` + n + `</li>`)
	}
	b.WriteString(`</ol></section>`)
	return b.String()
}

// listStack serializa listas ANIDADAS a partir del nivel de cada párrafo
// (xpara.level): mantiene la pila de <ul>/<ol> abiertos y deja el <li> vigente SIN
// cerrar, de modo que la sublista se emita dentro de él
// (<li>Uno<ol><li>Uno.a</li></ol></li>).
//
// Antes se ignoraba el nivel y todo se aplanaba en una única lista: un
// procedimiento numerado con subpasos se importaba como 1..N corrido —la
// numeración de los pasos principales dejaba de coincidir con el original— y el
// documento perdía su estructura. El nivel ya venía informado desde el docx
// (w:ilvl) y desde el HTML; solo se tiraba aquí, que es por donde pasan TODOS los
// importadores (docx, odt, html, markdown, pptx, odp).
type listStack struct {
	b    *strings.Builder
	open []string // tipo de lista abierta por nivel ("bullet"/"number")
	inLI bool     // hay un <li> sin cerrar en el nivel más profundo
}

// item emite un <li> en el nivel del párrafo, abriendo o cerrando las sublistas
// necesarias para llegar a él.
func (s *listStack) item(p xpara) {
	lvl := p.level
	if lvl < 0 {
		lvl = 0
	}
	if lvl > len(s.open) {
		lvl = len(s.open) // sin saltos de nivel: un <ul> hijo directo de <ul> no es válido
	}
	for len(s.open) > lvl+1 {
		s.pop()
	}
	if len(s.open) == lvl+1 && s.open[lvl] != p.list {
		s.pop() // cambio viñeta <-> número en el mismo nivel: lista nueva
	}
	if len(s.open) == lvl {
		s.push(p.list, p.vineta)
	}
	if s.inLI {
		s.b.WriteString("</li>")
		s.inLI = false
	}
	s.b.WriteString("<li" + estiloBloque(p) + ">")
	s.b.WriteString(runsToHTML(p.runs))
	s.inLI = true
}

func (s *listStack) push(kind, vineta string) {
	switch {
	case kind == "number":
		s.b.WriteString("<ol>")
	case vineta != "":
		// La viñeta propia de una lista importada, como cadena de CSS (sin comillas ni barras:
		// lo garantiza el importador).
		s.b.WriteString(`<ul style="list-style-type:'` + escaparAtributo.Replace(vineta) + ` '">`)
	default:
		s.b.WriteString("<ul>")
	}
	s.open = append(s.open, kind)
	s.inLI = false
}

func (s *listStack) pop() {
	if s.inLI {
		s.b.WriteString("</li>")
	}
	if s.open[len(s.open)-1] == "number" {
		s.b.WriteString("</ol>")
	} else {
		s.b.WriteString("</ul>")
	}
	s.open = s.open[:len(s.open)-1]
	s.inLI = len(s.open) > 0 // el <li> padre sigue abierto: alojaba la sublista
}

func (s *listStack) closeAll() {
	for len(s.open) > 0 {
		s.pop()
	}
}

// paraToHTML emite un párrafo/título/cita con su alineación.
func paraToHTML(p xpara) string {
	tag := "p"
	if lvl := headingLevel(p.style); lvl >= 1 && lvl <= 6 {
		tag = "h" + string(rune('0'+lvl))
	} else if p.style == "Quote" {
		tag = "blockquote"
	}
	inner := runsToHTML(p.runs)
	if inner == "" && p.css != "" {
		inner = "<br>" // la línea vacía de un fichero ocupa su alto, como en el editor
	}
	return "<" + tag + estiloBloque(p) + ">" + inner + "</" + tag + ">"
}

// escaparAtributo escapa el valor de un atributo como lo serializa el navegador (innerHTML): la
// comilla simple de un font-family ('Times New Roman') se queda tal cual. Con &#39; el editor
// reescribía cada estilo al montar el documento, y el registro y la secuencia de caracteres de
// doc:html divergían en el primer guardado sin que nadie hubiera tocado nada.
var escaparAtributo = strings.NewReplacer("&", "&amp;", "\u00a0", "&nbsp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;")

// estiloBloque es el atributo style de un bloque: su alineación y el CSS que calculó el
// importador. Lo escapa: el CSS no lleva comillas, pero el valor de una fuente viene de fuera.
func estiloBloque(p xpara) string {
	st := p.css
	if a := cssAlign(p.align); a != "" {
		st = strings.TrimPrefix(st+";text-align:"+a, ";")
	}
	if st == "" {
		return ""
	}
	return ` style="` + escaparAtributo.Replace(st) + `"`
}

// cssAlign mapea la alineación interna al valor CSS text-align (left es el
// defecto: no se emite para no ensuciar el HTML).
func cssAlign(a string) string {
	switch a {
	case "center", "right", "justify":
		return a
	}
	return ""
}

// runsToHTML serializa los tramos inline con su formato. El anidamiento de
// etiquetas es indiferente para el re-parseo (parseHTMLBlocks acumula flags).
func runsToHTML(runs []xrun) string {
	var b strings.Builder
	runs = agruparCampos(runs)
	for i := 0; i < len(runs); i++ {
		r := runs[i]
		if r.mark != "" || r.track != nil {
			n := agrupado(&b, runs[i:])
			i += n - 1
			continue
		}
		if r.field != "" {
			b.WriteString(`<span data-page-field style="white-space:nowrap">` + html.EscapeString(r.text) + "</span>")
			continue
		}
		if r.brk {
			b.WriteString("<br>")
			continue
		}
		if r.raw != "" {
			// HTML que ARMA el importador (una imagen), no texto del fichero: va tal
			// cual. Lo construye imgTag a partir de un data:URL en base64 y un tamaño
			// numérico, nunca de una cadena del documento.
			b.WriteString(r.raw)
			continue
		}
		t := html.EscapeString(r.text)
		if st := estiloTramo(r); st != "" {
			t = `<span style="` + escaparAtributo.Replace(st) + `">` + t + "</span>"
		}
		if r.vert != "" {
			t = "<" + r.vert + ">" + t + "</" + r.vert + ">"
		}
		if r.strike {
			t = "<s>" + t + "</s>"
		}
		if r.u {
			t = "<u>" + t + "</u>"
		}
		if r.i {
			t = "<em>" + t + "</em>"
		}
		if r.b {
			t = "<strong>" + t + "</strong>"
		}
		if r.link != "" && enlaceSeguro(r.link) {
			t = `<a href="` + html.EscapeString(r.link) + `">` + t + "</a>"
		}
		b.WriteString(t)
	}
	return b.String()
}

// estiloTramo es el CSS en línea de un tramo: su color y lo que calculó el importador.
func estiloTramo(r xrun) string {
	if r.color == "" {
		return r.css
	}
	return strings.TrimSuffix("color:#"+r.color+";"+r.css, ";")
}

// agruparCampos funde «Página » + PAGE + « de » + NUMPAGES en UN campo: el del editor es la
// frase entera («Página 1 de 5», en el idioma de quien lo abre) y la vista reescribe todo su
// texto, así que la palabra de delante tiene que ir dentro o saldría duplicada.
// ponytail: «la palabra de delante» es la última del texto previo si este acaba en espacio;
// «de» es el único literal que se admite entre los dos campos.
func agruparCampos(runs []xrun) []xrun {
	plano := func(r xrun) bool {
		return r.field == "" && !r.brk && r.raw == "" && r.image == nil && r.note == nil && r.link == ""
	}
	var out []xrun
	for i := 0; i < len(runs); i++ {
		r := runs[i]
		if r.field == "" {
			out = append(out, r)
			continue
		}
		g := xrun{field: "PAGE", text: "1"}
		if n := len(out); n > 0 && plano(out[n-1]) && strings.HasSuffix(out[n-1].text, " ") {
			t := strings.TrimRight(out[n-1].text, " ")
			if k := strings.LastIndexByte(t, ' ') + 1; k < len(t) {
				g.text = t[k:] + " 1"
				out[n-1].text = t[:k]
			}
		}
		if i+2 < len(runs) && plano(runs[i+1]) && runs[i+2].field == "NUMPAGES" {
			g.text += runs[i+1].text + "1"
			i += 2
		}
		out = append(out, g)
	}
	return out
}

// agrupado escribe la racha de tramos que empieza en runs[0] y comparten su comentario (un
// <mark>) o su revisión (un <ins>/<del>), y dice cuántos tramos ha gastado. El comentario
// envuelve a la revisión.
func agrupado(b *strings.Builder, runs []xrun) int {
	r := runs[0]
	n := 1
	same := func(x xrun) bool { return x.mark == r.mark && (r.mark != "" || x.track != nil && *x.track == *r.track) }
	for n < len(runs) && same(runs[n]) {
		n++
	}
	inner := slices.Clone(runs[:n])
	for k := range inner {
		if r.mark != "" {
			inner[k].mark = ""
		} else {
			inner[k].track = nil
		}
	}
	if r.mark != "" {
		b.WriteString(`<mark data-comment-id="` + html.EscapeString(r.mark) + `">` + runsToHTML(inner) + "</mark>")
	} else {
		b.WriteString(trackOpen(r.track) + runsToHTML(inner) + "</" + r.track.kind + ">")
	}
	return n
}

// trackOpen abre la marca del control de cambios tal como la escribe el editor
// (`trackAttrs` de trackchanges.ts): bandera, autor, id de autor, sello y color. Sin cuenta
// de origen, el id del autor es su nombre y el color sale de él con el mismo hash que
// tokens.Persona (31·h + unidad UTF-16, módulo 6).
func trackOpen(t *xtrack) string {
	var h uint32
	for _, u := range utf16.Encode([]rune(t.author)) {
		h = h*31 + uint32(u)
	}
	ts := ""
	if t.ts != 0 {
		ts = ` data-ts="` + strconv.FormatInt(t.ts, 10) + `"`
	}
	a := html.EscapeString(t.author)
	return "<" + t.kind + ` data-track data-author="` + a + `" data-author-id="` + a + `"` + ts +
		` style="--sytue-track:var(--sytue-avatar-` + strconv.Itoa(int(h%6)+1) + `)">`
}

// parseTrackTime lee la fecha de una revisión (OOXML la da con zona, ODF sin ella: UTC) en
// milisegundos, o 0 si no hay o no se entiende.
func parseTrackTime(s string) int64 {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

// tableToHTML emite una tabla como <table><tbody><tr><td>… con los párrafos de
// cada celda.
func tableToHTML(t *xtable) string {
	var b strings.Builder
	b.WriteString("<table")
	if t.css != "" {
		b.WriteString(` style="` + escaparAtributo.Replace(t.css) + `"`)
	}
	b.WriteString("><tbody>")
	for _, row := range t.rows {
		b.WriteString("<tr>")
		for _, c := range row {
			if c.covered {
				continue
			}
			b.WriteString("<td")
			cs, rs := c.span()
			if cs > 1 {
				b.WriteString(` colspan="` + strconv.Itoa(cs) + `"`)
			}
			if rs > 1 {
				b.WriteString(` rowspan="` + strconv.Itoa(rs) + `"`)
			}
			if c.css != "" {
				b.WriteString(` style="` + escaparAtributo.Replace(c.css) + `"`)
			}
			b.WriteString(">")
			b.WriteString(celdaHTML(c.paras))
			b.WriteString("</td>")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</tbody></table>")
	return b.String()
}

// celdaHTML es el contenido de una celda: el texto suelto si es un párrafo sin formato propio
// (lo que escribe el editor), y si no, sus párrafos como bloques, cada uno con su estilo.
func celdaHTML(paras []xpara) string {
	if len(paras) == 1 && paras[0].css == "" && paras[0].style == "" && paras[0].list == "" && paras[0].align == "" {
		return runsToHTML(paras[0].runs)
	}
	blocks := make([]xblock, len(paras))
	for i := range paras {
		blocks[i] = xblock{para: &paras[i]}
	}
	return blocksToHTML(blocks)
}

// --- Utilidades de paquete ZIP (OPC / ODF) ---

// readZip lee un paquete ZIP a un mapa nombre->bytes. Cota el total
// descomprimido a un múltiplo de MaxImportBytes para no amplificar en memoria un
// zip-bomb (entrada no confiable).
func readZip(data []byte) (map[string][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: paquete ZIP inválido: %w", ErrInvalidFormat, err)
	}
	const maxTotal = 4 * MaxImportBytes // tope de expansión frente a zip-bombs
	out := make(map[string][]byte, len(zr.File))
	var total int64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// El tamaño que declara la entrada lo hace cumplir archive/zip al leerla: se puede
		// rechazar antes de descomprimir nada y reservar de una vez lo justo, en vez de
		// dejar que io.ReadAll duplique el búfer hasta llegar (hasta tres veces la parte).
		if f.UncompressedSize64 > uint64(maxTotal-total) {
			return nil, fmt.Errorf("%w: contenido descomprimido excesivo", ErrInvalidFormat)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("%w: entrada %q: %w", ErrInvalidFormat, f.Name, err)
		}
		buf := bytes.NewBuffer(make([]byte, 0, f.UncompressedSize64+bytes.MinRead)) // +MinRead: ReadFrom crece si se llena antes del EOF
		_, err = buf.ReadFrom(io.LimitReader(rc, maxTotal-total+1))
		rc.Close()
		b := buf.Bytes()
		if err != nil {
			return nil, fmt.Errorf("%w: leer %q: %w", ErrInvalidFormat, f.Name, err)
		}
		total += int64(len(b))
		if total > maxTotal {
			return nil, fmt.Errorf("%w: contenido descomprimido excesivo", ErrInvalidFormat)
		}
		// Algunas herramientas de Windows escriben las entradas con «\» («xl\workbook.xml»):
		// el ZIP pide «/», pero Office las abre igual, y aquí no se encontraba ninguna parte.
		nombre := path.Clean(strings.ReplaceAll(f.Name, `\`, "/"))
		if strings.HasSuffix(nombre, ".xml") || strings.HasSuffix(nombre, ".rels") {
			b = aUTF8(b)
		}
		if strings.HasSuffix(nombre, ".rels") {
			b = relativizarDestinos(nombre, b)
		}
		out[nombre] = b
	}
	return out, nil
}

// reCodificacionXML es la codificación de la declaración XML.
var reCodificacionXML = regexp.MustCompile(`(?i)encoding\s*=\s*["']utf-16(?:le|be)?["']`)

// aUTF8 pasa a UTF-8 una parte XML escrita en UTF-16, con marca de orden de bytes o sin ella
// (XML 1.0 §4.3.3 y apéndice F): el decodificador de XML de Go solo lee UTF-8 y la parte se
// perdía entera. Lo demás se devuelve tal cual.
func aUTF8(b []byte) []byte {
	var be bool
	ini := 2
	switch {
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		be = true
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
	case bytes.HasPrefix(b, []byte{0, '<'}):
		be, ini = true, 0
	case bytes.HasPrefix(b, []byte{'<', 0}):
		ini = 0
	default:
		return b
	}
	u := make([]uint16, 0, len(b)/2)
	for i := ini; i+1 < len(b); i += 2 {
		if be {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			u = append(u, uint16(b[i+1])<<8|uint16(b[i]))
		}
	}
	s := string(utf16.Decode(u))
	// La declaración dice UTF-16 y ya no lo es: el decodificador se negaría a leerla.
	if fin := strings.Index(s, "?>"); strings.HasPrefix(s, "<?xml") && fin > 0 {
		s = reCodificacionXML.ReplaceAllString(s[:fin], `encoding="UTF-8"`) + s[fin:]
	}
	return []byte(s)
}

var (
	reRelacion     = regexp.MustCompile(`<(?:\w+:)?Relationship\b[^>]*>`)
	reDestinoRaiz  = regexp.MustCompile(`(\bTarget\s*=\s*["'])/+`)
	modoExternoRel = regexp.MustCompile(`\bTargetMode\s*=\s*["']External["']`)
)

// relativizarDestinos reescribe los destinos de relación absolutos (`Target="/xl/worksheets/
// sheet1.xml"`, desde la raíz del paquete) como relativos a la parte que los declara
// (`../xl/worksheets/sheet1.xml` desde xl/workbook.xml), que es como los resuelven todos los
// lectores de aquí. Los dos son válidos (OPC, ECMA-376 parte 2 §9.3) y los absolutos son los
// que escribe System.IO.Packaging de .NET —y con él el Open XML SDK, ClosedXML y los informes
// que generan—: sin esto, el libro o la presentación se abría vacío y sin avisar. Los destinos
// externos (`TargetMode="External"`) no son partes del paquete y no se tocan.
func relativizarDestinos(nombreRels string, data []byte) []byte {
	if !reDestinoRaiz.Match(data) {
		return data
	}
	// «xl/_rels/workbook.xml.rels» declara las relaciones de una parte de «xl».
	subir := ""
	if dir := path.Dir(path.Dir(nombreRels)); dir != "." && dir != "/" {
		subir = strings.Repeat("../", strings.Count(dir, "/")+1)
	}
	return reRelacion.ReplaceAllFunc(data, func(rel []byte) []byte {
		if modoExternoRel.Match(rel) {
			return rel
		}
		return reDestinoRaiz.ReplaceAll(rel, []byte("${1}"+subir))
	})
}

// partePrincipal es la parte principal del paquete, la que nombra la relación officeDocument de
// _rels/.rels (§11.3.10 de la Parte 1), o `porDefecto` si no la nombra o no está. Word en la
// web llama al documento «document22.xml», y hay libros con el suyo en la raíz del paquete o
// en «xl/workbook2.xml». Si solo está con otra caja, se alinea el paquete (alinearCaja).
func partePrincipal(files map[string][]byte, porDefecto string) string {
	for _, r := range parseRelsTyped(files["_rels/.rels"]) {
		if !strings.HasSuffix(r.Type, "/officeDocument") || r.Target == "" {
			continue
		}
		p := path.Clean(strings.TrimPrefix(r.Target, "/"))
		if len(files[p]) == 0 {
			alinearCaja(files)
		}
		if len(files[p]) > 0 {
			return p
		}
	}
	return porDefecto
}

// alinearCaja da a cada parte que el paquete cita con otra caja el nombre con el que la cita,
// como alias de los mismos bytes: los nombres de parte de OPC no distinguen mayúsculas (Parte
// 2, §9.1.1.1), y hay escritores que guardan «xl/WorkBook.xml» y la citan como
// «xl/workbook.xml». Recorre el paquete desde sus relaciones, de parte en parte.
// ponytail: solo se llama cuando la parte principal falta con su nombre; un paquete que la
// tenga bien y cite mal otra parte no se alinea.
func alinearCaja(files map[string][]byte) {
	porMinusculas := make(map[string]string, len(files))
	for n := range files {
		porMinusculas[strings.ToLower(n)] = n
	}
	alias := func(n string) bool {
		if _, ok := files[n]; ok {
			return true
		}
		r, ok := porMinusculas[strings.ToLower(n)]
		if ok {
			files[n] = files[r]
		}
		return ok
	}
	pendientes, vistas := []string{""}, map[string]bool{} // "": el paquete, con _rels/.rels
	for len(pendientes) > 0 {
		p := pendientes[len(pendientes)-1]
		pendientes = pendientes[:len(pendientes)-1]
		if vistas[p] || !alias(relsPath(p)) {
			continue
		}
		vistas[p] = true
		for _, r := range parseRelsTyped(files[relsPath(p)]) {
			if d := destinoRel(p, r.Target); alias(d) {
				pendientes = append(pendientes, d)
			}
		}
	}
}

// parseRels parsea un stream de relaciones OPC (_rels/*.rels) a Id->Target.
func parseRels(data []byte) map[string]string {
	var rels struct {
		Rel []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	out := map[string]string{}
	if xml.Unmarshal(data, &rels) != nil {
		return out
	}
	for _, r := range rels.Rel {
		out[r.ID] = r.Target
	}
	return out
}

// enlaceSeguro dice si un destino de enlace que viene de un fichero o de un documento ajeno
// puede ir a un `href` del editor: una dirección web, un correo, un teléfono, una ruta
// (`/`, `./`, `../`) o un ancla. Todo lo demás —`javascript:`, `data:`, `vbscript:`, `file:`…— se
// queda en texto sin enlace: el HTML que sale de aquí acaba en el DOM de quien lo abre.
// Lista de lo que SÍ vale, no de lo que no: un esquema nuevo no entra por olvido.
func enlaceSeguro(destino string) bool {
	for _, r := range destino {
		// Los navegadores quitan tabuladores y saltos de DENTRO del esquema («java\tscript:»)
		// y los controles de delante: con cualquiera de ellos, no es un enlace.
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	if _, err := url.Parse(strings.TrimSpace(destino)); err != nil {
		return false
	}
	// La regla de enlaces del saneador (saneado_html.go), que es la que se aplica al
	// guardar: lo que no la cumple se quitaría allí, y el documento se guardaría distinto
	// de como se importó. Deja fuera la ruta suelta («fonts.docx», un fichero vecino del
	// original), que en Docs no lleva a ningún sitio.
	return urlAdmitida(destino, false)
}
