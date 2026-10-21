// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Pruebas del importador (reverso del exportador Ola 1). Estrategia principal:
// round-trip export -> import -> mismo contenido, normalizando por parseHTMLBlocks
// (documentos) o comparando celdas/cajas (hojas/presentaciones). Más ficheros
// mínimos construidos a mano para cubrir rutas que el exportador no ejercita
// (p. ej. sharedStrings de xlsx) y la independencia respecto del exportador.

// roundTripHTML: documento rico SIN enlaces (el export decora los enlaces con
// subrayado/color, lo que rompería la igualdad estricta; los enlaces se prueban
// aparte). Cubre título, negrita/cursiva/subrayado/tachado, listas, tabla, regla
// y alineación con color.
const roundTripHTML = `<h1>Título</h1>` +
	`<p>Hola <strong>negrita</strong> <em>cursiva</em> <u>sub</u> <s>tach</s> normal</p>` +
	`<ul><li>uno</li><li>dos</li></ul>` +
	`<ol><li>primero</li><li>segundo</li></ol>` +
	`<table><tbody><tr><td>c1</td><td>c2</td></tr><tr><td>c3</td><td>c4</td></tr></tbody></table>` +
	`<hr>` +
	`<p style="text-align:center"><span style="color:#FF0000">centrado</span></p>`

// assertBlocksEqual compara dos HTML por su estructura de bloques normalizada
// (parseHTMLBlocks), tolerante a diferencias de serialización irrelevantes. Se
// descartan los párrafos vacíos: OOXML/ODF exigen un párrafo de relleno tras cada
// tabla y al final del cuerpo, que no forma parte del contenido del usuario.
func assertBlocksEqual(t *testing.T, where, want, got string) {
	t.Helper()
	wb, gb := nonEmptyBlocks(parseHTMLBlocks(want)), nonEmptyBlocks(parseHTMLBlocks(got))
	if !reflect.DeepEqual(wb, gb) {
		t.Fatalf("%s: bloques distintos\n want: %s\n  got: %s", where, want, got)
	}
}

// nonEmptyBlocks descarta los párrafos totalmente vacíos (sin runs, estilo, lista
// ni alineación): líneas en blanco estructurales que el formato de fichero añade.
func nonEmptyBlocks(blocks []xblock) []xblock {
	out := make([]xblock, 0, len(blocks))
	for _, b := range blocks {
		if b.para != nil && len(b.para.runs) == 0 && b.para.style == "" && b.para.list == "" && b.para.align == "" {
			continue
		}
		out = append(out, b)
	}
	return out
}

func TestImportDocxRoundTrip(t *testing.T) {
	s := stateWithContent(t, "doc:html", roundTripHTML)
	data, _, ext, err := Export(KindDocument, "", s, FormatOOXML)
	if err != nil || ext != "docx" {
		t.Fatalf("export docx: %v %s", err, ext)
	}
	res, err := Import("informe.docx", data)
	if err != nil {
		t.Fatalf("import docx: %v", err)
	}
	if res.Kind != KindDocument {
		t.Fatalf("kind = %q, quiero document", res.Kind)
	}
	if res.Title != "informe" {
		t.Fatalf("title = %q, quiero informe", res.Title)
	}
	assertBlocksEqual(t, "docx", roundTripHTML, res.Content)
}

func TestImportDocxHyperlink(t *testing.T) {
	const withLink = `<p>ver <a href="https://sytue.com/x">enlace</a> aquí</p>`
	s := stateWithContent(t, "doc:html", withLink)
	data, _, _, err := Export(KindDocument, "", s, FormatOOXML)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	res, err := Import("d.docx", data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	mustContain(t, "docx-link", res.Content, `href="https://sytue.com/x"`)
	mustContain(t, "docx-link", res.Content, "enlace")
}

func TestImportOdtRoundTrip(t *testing.T) {
	s := stateWithContent(t, "doc:html", roundTripHTML)
	data, _, ext, err := Export(KindDocument, "", s, FormatODF)
	if err != nil || ext != "odt" {
		t.Fatalf("export odt: %v %s", err, ext)
	}
	res, err := Import("informe.odt", data)
	if err != nil {
		t.Fatalf("import odt: %v", err)
	}
	if res.Kind != KindDocument {
		t.Fatalf("kind = %q", res.Kind)
	}
	assertBlocksEqual(t, "odt", roundTripHTML, res.Content)
}

// --- LISTAS ANIDADAS ---
//
// Perder el anidamiento convierte un procedimiento numerado con subpasos en una
// lista corrida: los pasos principales se renumeran (1..N contando los subpasos) y
// el documento deja de servir para lo que se escribió. Se prueba el VIAJE COMPLETO
// (contenido -> fichero -> importación) por cada formato, más ficheros ajenos al
// exportador (Word y LibreOffice escriben el anidamiento de otra manera).

// nestedListHTML: dos niveles de lista ordenada con una viñeta en el tercero.
const nestedListHTML = `<ol><li>Preparar<ol><li>Abrir la tapa<ul><li>con guantes</li></ul></li>` +
	`<li>Vaciar el depósito</li></ol></li><li>Montar</li></ol>`

func TestImportListasAnidadasRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name   string
		format BlobFormat
		ext    string
	}{
		{"docx", FormatOOXML, "docx"},
		{"odt", FormatODF, "odt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := stateWithContent(t, "doc:html", nestedListHTML)
			data, _, ext, err := Export(KindDocument, "", s, tc.format)
			if err != nil || ext != tc.ext {
				t.Fatalf("export %s: %v %s", tc.name, err, ext)
			}
			res, err := Import("guia."+ext, data)
			if err != nil {
				t.Fatalf("import %s: %v", tc.name, err)
			}
			assertBlocksEqual(t, "anidada-"+tc.name, nestedListHTML, res.Content)
		})
	}
}

// TestImportListasAnidadasDocxAjeno: .docx a mano con w:ilvl (como lo escribe
// Word), independiente de nuestro exportador.
func TestImportListasAnidadasDocxAjeno(t *testing.T) {
	const numbering = `<?xml version="1.0"?><w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:abstractNum w:abstractNumId="0"><w:lvl w:ilvl="0"><w:numFmt w:val="decimal"/></w:lvl>` +
		`<w:lvl w:ilvl="1"><w:numFmt w:val="lowerLetter"/></w:lvl></w:abstractNum>` +
		`<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num></w:numbering>`
	item := func(ilvl, text string) string {
		return `<w:p><w:pPr><w:numPr><w:ilvl w:val="` + ilvl + `"/><w:numId w:val="1"/></w:numPr></w:pPr>` +
			`<w:r><w:t>` + text + `</w:t></w:r></w:p>`
	}
	doc := `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		item("0", "Uno") + item("1", "Uno.a") + item("1", "Uno.b") + item("0", "Dos") + `</w:body></w:document>`
	res, err := Import("word.docx", zipStore(t, map[string]string{
		"word/document.xml":  doc,
		"word/numbering.xml": numbering,
	}))
	if err != nil {
		t.Fatalf("import docx ajeno: %v", err)
	}
	assertBlocksEqual(t, "docx-ajeno",
		`<ol><li>Uno<ol><li>Uno.a</li><li>Uno.b</li></ol></li><li>Dos</li></ol>`, res.Content)
}

// TestImportListasAnidadasOdtAjeno: .odt como lo escribe LibreOffice: sublista
// DENTRO del <text:list-item> y el tipo declarado en <text:list-style> por nivel
// (los nombres de estilo son "L1", no dicen si es viñeta o número).
func TestImportListasAnidadasOdtAjeno(t *testing.T) {
	content := xmlHeader +
		`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
		`xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" ` +
		`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">` +
		`<office:automatic-styles><text:list-style style:name="L1">` +
		`<text:list-level-style-number text:level="1" style:num-format="1"/>` +
		`<text:list-level-style-bullet text:level="2" text:bullet-char="•"/>` +
		`</text:list-style></office:automatic-styles>` +
		`<office:body><office:text><text:list text:style-name="L1">` +
		`<text:list-item><text:p>Uno</text:p>` +
		`<text:list text:style-name="L1"><text:list-item><text:p>Uno.a</text:p></text:list-item></text:list>` +
		`</text:list-item>` +
		`<text:list-item><text:p>Dos</text:p></text:list-item>` +
		`</text:list></office:text></office:body></office:document-content>`
	res, err := Import("libre.odt", zipStore(t, map[string]string{
		"mimetype":    "application/vnd.oasis.opendocument.text",
		"content.xml": content,
	}))
	if err != nil {
		t.Fatalf("import odt ajeno: %v", err)
	}
	assertBlocksEqual(t, "odt-ajeno",
		`<ol><li>Uno<ul><li>Uno.a</li></ul></li><li>Dos</li></ol>`, res.Content)
}

// TestImportMarkdownListaAnidada: en Markdown el nivel lo da la SANGRÍA, y cada
// editor sangra distinto (2 espacios, 4 o tabulador).
func TestImportMarkdownListaAnidada(t *testing.T) {
	for _, md := range []string{
		"1. Uno\n  1. Uno.a\n  2. Uno.b\n2. Dos\n",     // 2 espacios
		"1. Uno\n    1. Uno.a\n    2. Uno.b\n2. Dos\n", // 4 espacios
		"1. Uno\n\t1. Uno.a\n\t2. Uno.b\n2. Dos\n",     // tabulador
	} {
		res, err := Import("guia.md", []byte(md))
		if err != nil {
			t.Fatalf("import md: %v", err)
		}
		assertBlocksEqual(t, "md-anidada",
			`<ol><li>Uno<ol><li>Uno.a</li><li>Uno.b</li></ol></li><li>Dos</li></ol>`, res.Content)
	}
}

// TestImportPptxListaAnidada: viñetas con subniveles en una diapositiva (a:pPr lvl).
func TestImportPptxListaAnidada(t *testing.T) {
	const boxHTML = `<ul><li>Punto<ul><li>Subpunto</li></ul></li><li>Otro</li></ul>`
	model := `{"slides":[{"id":"s1","boxes":[{"x":10,"y":10,"w":80,"h":40,"kind":"text","html":"` +
		strings.ReplaceAll(boxHTML, `"`, `\"`) + `"}]}]}`
	s := stateWithContent(t, "doc:slides", model)
	data, _, _, err := Export(KindPresentation, "Charla", s, FormatOOXML)
	if err != nil {
		t.Fatalf("export pptx: %v", err)
	}
	res, err := Import("charla.pptx", data)
	if err != nil {
		t.Fatalf("import pptx: %v", err)
	}
	got := parseSlidesModelJSON(t, res.Content)
	if len(got.Slides) != 1 || len(got.Slides[0].Boxes) != 1 {
		t.Fatalf("modelo = %#v", got)
	}
	assertBlocksEqual(t, "pptx-anidada", boxHTML, got.Slides[0].Boxes[0].HTML)
}

// --- IMÁGENES ANCLADAS ---
//
// Una imagen que no se lee desaparece del documento y deja huérfano al texto que
// la cita («como se ve en la figura»). El editor guarda cada imagen como un único
// <img> con data:URL, así que la importación puede restituirla entera.

// pngBytes son los bytes crudos del PNG de 1x1 usado en las pruebas.
func pngBytes(t *testing.T) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(onePxPNG, "data:image/png;base64,"))
	if err != nil {
		t.Fatalf("png de prueba: %v", err)
	}
	return raw
}

func TestImportDocxImagenAnclada(t *testing.T) {
	doc := `<?xml version="1.0"?><w:document ` +
		`xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
		`xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
		`xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"><w:body>` +
		`<w:p><w:r><w:t>Figura 1:</w:t></w:r>` +
		`<w:r><w:drawing><wp:inline><wp:extent cx="1905000" cy="952500"/><a:graphic><a:graphicData>` +
		`<pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">` +
		`<pic:blipFill><a:blip r:embed="rId5"/></pic:blipFill></pic:pic>` +
		`</a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>` +
		`<w:p><w:r><w:t>texto posterior</w:t></w:r></w:p></w:body></w:document>`
	rels := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId5" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image1.png"/>` +
		`</Relationships>`
	res, err := Import("informe.docx", zipStore(t, map[string]string{
		"word/document.xml":            doc,
		"word/_rels/document.xml.rels": rels,
		"word/media/image1.png":        string(pngBytes(t)),
	}))
	if err != nil {
		t.Fatalf("import docx con imagen: %v", err)
	}
	mustContain(t, "docx-img", res.Content, `<img src="`+onePxPNG+`"`)
	// Tamaño de maqueta: 1905000 EMU = 200 px; 952500 EMU = 100 px.
	mustContain(t, "docx-img", res.Content, `data-w="200" data-h="100"`)
	mustContain(t, "docx-img", res.Content, `width:200px;height:100px`)
	// La imagen va en su sitio: dentro del párrafo que la cita, no al final.
	if i, j := strings.Index(res.Content, "Figura 1:"), strings.Index(res.Content, "<img"); i < 0 || j < i {
		t.Fatalf("la imagen no quedó tras su texto: %s", res.Content)
	}
	mustContain(t, "docx-img", res.Content, "texto posterior")
}

func TestImportOdtImagenAnclada(t *testing.T) {
	content := xmlHeader +
		`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
		`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" ` +
		`xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" ` +
		`xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0" ` +
		`xmlns:xlink="http://www.w3.org/1999/xlink">` +
		`<office:body><office:text>` +
		`<text:p>Figura 1:<draw:frame svg:width="5.08cm" svg:height="2.54cm">` +
		`<draw:image xlink:href="Pictures/uno.png"/></draw:frame></text:p>` +
		`<text:p>texto posterior</text:p>` +
		`</office:text></office:body></office:document-content>`
	res, err := Import("informe.odt", zipStore(t, map[string]string{
		"mimetype":         "application/vnd.oasis.opendocument.text",
		"content.xml":      content,
		"Pictures/uno.png": string(pngBytes(t)),
	}))
	if err != nil {
		t.Fatalf("import odt con imagen: %v", err)
	}
	mustContain(t, "odt-img", res.Content, `<img src="`+onePxPNG+`"`)
	mustContain(t, "odt-img", res.Content, `data-w="192" data-h="96"`) // 5.08cm = 2in = 192px
	mustContain(t, "odt-img", res.Content, "texto posterior")
}

// --- NOTAS AL PIE ---
//
// El texto de la nota es contenido: si no se lee, desaparece del documento sin
// dejar rastro (ni siquiera la llamada). Se restituye con el marcado que ya usa el
// editor: marcador superíndice + una sección de notas al final del folio.

func TestImportDocxNotasAlPie(t *testing.T) {
	doc := `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:r><w:t>Beneficio anual</w:t></w:r><w:r><w:footnoteReference w:id="2"/></w:r>` +
		`<w:r><w:t> y previsión</w:t></w:r><w:r><w:footnoteReference w:id="3"/></w:r>` +
		`<w:r><w:t> (método</w:t></w:r><w:r><w:endnoteReference w:id="2"/></w:r><w:r><w:t>)</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	endnotes := `<?xml version="1.0"?><w:endnotes xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:endnote w:id="2"><w:p><w:r><w:t>Detalle metodológico.</w:t></w:r></w:p></w:endnote></w:endnotes>`
	notes := `<?xml version="1.0"?><w:footnotes xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:footnote w:id="0" w:type="separator"><w:p><w:r><w:t>---</w:t></w:r></w:p></w:footnote>` +
		`<w:footnote w:id="2"><w:p><w:r><w:t>Cifras sin auditar.</w:t></w:r></w:p></w:footnote>` +
		`<w:footnote w:id="3"><w:p><w:r><w:t>Estimación interna.</w:t></w:r></w:p></w:footnote>` +
		`</w:footnotes>`
	res, err := Import("cuentas.docx", zipStore(t, map[string]string{
		"word/document.xml":  doc,
		"word/footnotes.xml": notes,
		"word/endnotes.xml":  endnotes,
	}))
	if err != nil {
		t.Fatalf("import docx con notas: %v", err)
	}
	// El texto de las dos notas está, y numerado por orden de llamada.
	mustContain(t, "docx-nota", res.Content, "Cifras sin auditar.")
	mustContain(t, "docx-nota", res.Content, "Estimación interna.")
	mustContain(t, "docx-nota", res.Content, `<sup data-footnote-ref="1" id="fnref-1"><a href="#fn-1">1</a></sup>`)
	mustContain(t, "docx-nota", res.Content, `<sup data-footnote-ref="2" id="fnref-2"><a href="#fn-2">2</a></sup>`)
	mustContain(t, "docx-nota", res.Content, `<section data-footnotes><hr><ol><li id="fn-1"`)
	// Las notas al final comparten numeración: la tercera llamada es la nota 3.
	mustContain(t, "docx-nota", res.Content, "Detalle metodológico.")
	// y se marca como nota final (data-note-class) para que el export la devuelva a endnotes.xml;
	// las notas al pie no llevan la marca.
	mustContain(t, "docx-nota", res.Content, `<sup data-footnote-ref="3" data-note-class="endnote" id="fnref-3">`)
	if strings.Contains(res.Content, `data-footnote-ref="1" data-note-class`) {
		t.Fatalf("una nota al pie no debe llevar data-note-class: %s", res.Content)
	}
	// El separador que Word guarda como nota 0 no es una nota del autor.
	if strings.Contains(res.Content, "---") {
		t.Fatalf("el separador de Word se coló como nota: %s", res.Content)
	}
	// La llamada va donde estaba, no al final del párrafo.
	if i, j := strings.Index(res.Content, "Beneficio anual"), strings.Index(res.Content, "fnref-1"); j < i {
		t.Fatalf("marcador fuera de sitio: %s", res.Content)
	}
}

func TestImportOdtNotasAlPie(t *testing.T) {
	content := xmlHeader +
		`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
		`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">` +
		`<office:body><office:text>` +
		`<text:p>Beneficio anual<text:note text:id="ftn0" text:note-class="footnote">` +
		`<text:note-citation>1</text:note-citation>` +
		`<text:note-body><text:p>Cifras sin auditar.</text:p></text:note-body>` +
		`</text:note> y previsión.</text:p>` +
		`</office:text></office:body></office:document-content>`
	res, err := Import("cuentas.odt", zipStore(t, map[string]string{
		"mimetype":    "application/vnd.oasis.opendocument.text",
		"content.xml": content,
	}))
	if err != nil {
		t.Fatalf("import odt con notas: %v", err)
	}
	mustContain(t, "odt-nota", res.Content, "Cifras sin auditar.")
	mustContain(t, "odt-nota", res.Content, `<sup data-footnote-ref="1" id="fnref-1">`)
	mustContain(t, "odt-nota", res.Content, `<section data-footnotes>`)
	// El texto de la nota NO debe quedarse incrustado en medio del párrafo.
	// El corte se hace con el índice COMPROBADO: `Index` devuelve -1 cuando no
	// encuentra, y `[:−1]` no falla, entra en pánico —que oculta qué se rompió.
	corte := strings.Index(res.Content, "<section")
	if corte < 0 {
		t.Fatalf("no hay sección de notas que separar del cuerpo: %s", res.Content)
	}
	body := res.Content[:corte]
	if strings.Contains(body, "Cifras sin auditar.") {
		t.Fatalf("la nota se coló en el cuerpo: %s", body)
	}
	mustContain(t, "odt-nota", body, "y previsión.")
}

// --- HOJA ---

// sheetForImport es un modelo de hoja SIN formato HTML en celdas (para igualdad
// exacta): texto, número y fórmula.
func sheetForImport() sheetData {
	return sheetData{
		Name: "Datos",
		Cells: map[string]string{
			"A1": "Nombre", "B1": "Precio",
			"A2": "Café", "B2": "1.5",
			"A3": "Total", "B3": "=SUM(B2:B2)",
		},
	}
}

func TestImportXlsxRoundTrip(t *testing.T) {
	src := sheetForImport()
	src.Formats = map[string]numFmtSpec{"B2": {ID: "currency", Decimals: ptr(2)}}
	mj, _ := json.Marshal(sheetModel{Sheets: []sheetData{src}})
	s := stateWithContent(t, "doc:sheet", string(mj))

	data, _, ext, err := Export(KindSpreadsheet, "Libro", s, FormatOOXML)
	if err != nil || ext != "xlsx" {
		t.Fatalf("export xlsx: %v %s", err, ext)
	}
	res, err := Import("libro.xlsx", data)
	if err != nil {
		t.Fatalf("import xlsx: %v", err)
	}
	if res.Kind != KindSpreadsheet {
		t.Fatalf("kind = %q", res.Kind)
	}
	got := parseSheetModelJSON(t, res.Content)
	if len(got.Sheets) != 1 {
		t.Fatalf("hojas = %d, quiero 1", len(got.Sheets))
	}
	assertCellsEqual(t, "xlsx", src.Cells, got.Sheets[0].Cells)
	if f := got.Sheets[0].Formats["B2"]; f.ID != "currency" {
		t.Fatalf("formato B2 = %q, quiero currency", f.ID)
	}
}

func TestImportOdsRoundTrip(t *testing.T) {
	src := sheetForImport()
	mj, _ := json.Marshal(sheetModel{Sheets: []sheetData{src}})
	s := stateWithContent(t, "doc:sheet", string(mj))

	data, _, ext, err := Export(KindSpreadsheet, "Libro", s, FormatODF)
	if err != nil || ext != "ods" {
		t.Fatalf("export ods: %v %s", err, ext)
	}
	res, err := Import("libro.ods", data)
	if err != nil {
		t.Fatalf("import ods: %v", err)
	}
	got := parseSheetModelJSON(t, res.Content)
	if len(got.Sheets) != 1 {
		t.Fatalf("hojas = %d, quiero 1", len(got.Sheets))
	}
	assertCellsEqual(t, "ods", src.Cells, got.Sheets[0].Cells)
}

func TestImportCSVDelimiterAndCells(t *testing.T) {
	// Punto y coma (frecuente en es-ES): debe autodetectarse.
	csv := "Nombre;Precio\nCafé;1,50\nTé;2,00\n"
	res, err := Import("datos.csv", []byte(csv))
	if err != nil {
		t.Fatalf("import csv: %v", err)
	}
	if res.Kind != KindSpreadsheet {
		t.Fatalf("kind = %q", res.Kind)
	}
	got := parseSheetModelJSON(t, res.Content)
	cells := got.Sheets[0].Cells
	for ref, want := range map[string]string{"A1": "Nombre", "B1": "Precio", "A2": "Café", "B2": "1,50", "A3": "Té"} {
		if cells[ref] != want {
			t.Errorf("csv celda %s = %q, quiero %q", ref, cells[ref], want)
		}
	}
}

// TestImportXlsxSharedStrings cubre la ruta sharedStrings + resolución de rels
// que el exportador (inlineStr) NO ejercita: fichero xlsx mínimo hecho a mano.
func TestImportXlsxSharedStrings(t *testing.T) {
	files := map[string]string{
		"[Content_Types].xml":        `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>`,
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Hoja" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>Hola</t></si><si><t>Mundo</t></si></sst>`,
		"xl/worksheets/sheet1.xml":   `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1"><v>42</v></c></row></sheetData></worksheet>`,
	}
	res, err := Import("real.xlsx", zipStore(t, files))
	if err != nil {
		t.Fatalf("import xlsx shared: %v", err)
	}
	got := parseSheetModelJSON(t, res.Content)
	cells := got.Sheets[0].Cells
	if cells["A1"] != "Hola" || cells["B1"] != "Mundo" || cells["C1"] != "42" {
		t.Fatalf("celdas = %#v", cells)
	}
	if got.Sheets[0].Name != "Hoja" {
		t.Fatalf("nombre hoja = %q", got.Sheets[0].Name)
	}
}

// TestImportMinimalDocx prueba un .docx mínimo hecho a mano (independiente del
// exportador): un párrafo con negrita.
func TestImportMinimalDocx(t *testing.T) {
	doc := `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Hola</w:t></w:r><w:r><w:t xml:space="preserve"> mundo</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	res, err := Import("min.docx", zipStore(t, map[string]string{"word/document.xml": doc}))
	if err != nil {
		t.Fatalf("import min docx: %v", err)
	}
	assertBlocksEqual(t, "min-docx", `<p><strong>Hola</strong> mundo</p>`, res.Content)
}

// --- PRESENTACIÓN ---

func TestImportPptxRoundTrip(t *testing.T) {
	s := stateWithContent(t, "doc:slides", slidesJSON(t))
	data, _, ext, err := Export(KindPresentation, "Charla", s, FormatOOXML)
	if err != nil || ext != "pptx" {
		t.Fatalf("export pptx: %v %s", err, ext)
	}
	res, err := Import("charla.pptx", data)
	if err != nil {
		t.Fatalf("import pptx: %v", err)
	}
	if res.Kind != KindPresentation {
		t.Fatalf("kind = %q", res.Kind)
	}
	got := parseSlidesModelJSON(t, res.Content)
	if len(got.Slides) != 2 {
		t.Fatalf("diapositivas = %d, quiero 2", len(got.Slides))
	}
	s0 := got.Slides[0]
	if len(s0.Boxes) != 3 {
		t.Fatalf("cajas diapositiva 1 = %d, quiero 3", len(s0.Boxes))
	}
	// Caja de texto: HTML equivalente y posición restituida.
	txt := s0.Boxes[0]
	if txt.Kind != "text" {
		t.Fatalf("caja 0 kind = %q", txt.Kind)
	}
	assertBlocksEqual(t, "pptx-text", "<h1>Portada</h1><p><b>subtítulo</b></p>", txt.HTML)
	if txt.X != 10 || txt.Y != 10 {
		t.Fatalf("posición caja 0 = (%v,%v), quiero (10,10)", txt.X, txt.Y)
	}
	// Forma elipse con relleno/borde.
	shape := s0.Boxes[1]
	if shape.Kind != "shape" || shape.Shape != "ellipse" || shape.Fill != "#00FF00" {
		t.Fatalf("forma = %#v", shape)
	}
	// Imagen: dataURL preservado byte a byte.
	img := s0.Boxes[2]
	if img.Kind != "image" || img.Src != onePxPNG {
		t.Fatalf("imagen src = %q", img.Src)
	}
	if s0.Notes != "Hablar despacio" {
		t.Fatalf("notas = %q", s0.Notes)
	}
}

func TestImportOdpRoundTrip(t *testing.T) {
	s := stateWithContent(t, "doc:slides", slidesJSON(t))
	data, _, ext, err := Export(KindPresentation, "Charla", s, FormatODF)
	if err != nil || ext != "odp" {
		t.Fatalf("export odp: %v %s", err, ext)
	}
	res, err := Import("charla.odp", data)
	if err != nil {
		t.Fatalf("import odp: %v", err)
	}
	got := parseSlidesModelJSON(t, res.Content)
	if len(got.Slides) != 2 {
		t.Fatalf("diapositivas = %d, quiero 2", len(got.Slides))
	}
	// Primera diapositiva: al menos texto + forma + imagen.
	kinds := map[string]int{}
	for _, b := range got.Slides[0].Boxes {
		kinds[b.Kind]++
	}
	if kinds["text"] == 0 || kinds["image"] == 0 {
		t.Fatalf("tipos de caja = %#v", kinds)
	}
	if got.Slides[0].Notes != "Hablar despacio" {
		t.Fatalf("notas = %q", got.Slides[0].Notes)
	}
}

func TestImportOdpGroupedAndCustomShape(t *testing.T) {
	data := zipStore(t, map[string]string{
		"mimetype": "application/vnd.oasis.opendocument.presentation",
		"styles.xml": `<?xml version="1.0" encoding="UTF-8"?>
<office:document-styles xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0"
  xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0">
  <office:styles>
    <style:style style:name="gr1" style:family="graphic">
      <style:graphic-properties draw:fill-color="#003366" draw:stroke-color="#ffffff"/>
    </style:style>
  </office:styles>
</office:document-styles>`,
		"content.xml": `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"
  xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0">
  <office:body>
    <office:presentation>
      <draw:page draw:name="page1">
        <draw:g draw:name="Group1">
          <draw:custom-shape draw:style-name="gr1" svg:x="2cm" svg:y="2cm" svg:width="12cm" svg:height="4cm">
            <text:p>Título en custom shape</text:p>
          </draw:custom-shape>
          <draw:frame svg:x="2cm" svg:y="7cm" svg:width="15cm" svg:height="5cm">
            <text:p>Párrafo directo en frame</text:p>
          </draw:frame>
        </draw:g>
      </draw:page>
    </office:presentation>
  </office:body>
</office:document-content>`,
	})

	res, err := Import("presentacion.odp", data)
	if err != nil {
		t.Fatalf("import odp: %v", err)
	}
	if res.Kind != KindPresentation {
		t.Fatalf("res.Kind = %q, want presentation", res.Kind)
	}
	got := parseSlidesModelJSON(t, res.Content)
	if len(got.Slides) != 1 {
		t.Fatalf("diapositivas = %d, want 1", len(got.Slides))
	}
	boxes := got.Slides[0].Boxes
	if len(boxes) != 2 {
		t.Fatalf("boxes = %d, want 2", len(boxes))
	}
	if !strings.Contains(boxes[0].HTML, "Título en custom shape") {
		t.Errorf("box 0 HTML = %q, want 'Título en custom shape'", boxes[0].HTML)
	}
	if !strings.Contains(boxes[1].HTML, "Párrafo directo en frame") {
		t.Errorf("box 1 HTML = %q, want 'Párrafo directo en frame'", boxes[1].HTML)
	}
	if boxes[0].Kind != "text" || boxes[1].Kind != "text" {
		t.Errorf("box kinds = %q, %q, want text, text", boxes[0].Kind, boxes[1].Kind)
	}
}

// --- Texto / Markdown / HTML ---

func TestImportMarkdown(t *testing.T) {
	md := "# Título\n\nHola **negrita** y *cursiva* con [enlace](https://sytue.com).\n\n- uno\n- dos\n\n1. primero\n2. segundo\n"
	res, err := Import("notas.md", []byte(md))
	if err != nil {
		t.Fatalf("import md: %v", err)
	}
	if res.Kind != KindDocument {
		t.Fatalf("kind = %q", res.Kind)
	}
	blocks := parseHTMLBlocks(res.Content)
	// Debe haber un h1, un párrafo con formatos y enlace, y dos listas.
	if blocks[0].para == nil || blocks[0].para.style != "Heading1" {
		t.Fatalf("primer bloque no es h1: %#v", blocks[0].para)
	}
	mustContain(t, "md", res.Content, "<strong>negrita</strong>")
	mustContain(t, "md", res.Content, "<em>cursiva</em>")
	mustContain(t, "md", res.Content, `href="https://sytue.com"`)
	mustContain(t, "md", res.Content, "<ul><li>uno</li><li>dos</li></ul>")
	mustContain(t, "md", res.Content, "<ol><li>primero</li><li>segundo</li></ol>")
}

func TestImportPlainText(t *testing.T) {
	res, err := Import("nota.txt", []byte("Línea uno\n\nLínea dos\n"))
	if err != nil {
		t.Fatalf("import txt: %v", err)
	}
	if res.Content != "<p>Línea uno</p><p>Línea dos</p>" {
		t.Fatalf("txt content = %q", res.Content)
	}
}

func TestImportHTMLNormalizes(t *testing.T) {
	full := `<!doctype html><html><head><title>x</title></head><body><h2>Cabecera</h2><p>Cuerpo <b>fuerte</b></p></body></html>`
	res, err := Import("pagina.html", []byte(full))
	if err != nil {
		t.Fatalf("import html: %v", err)
	}
	assertBlocksEqual(t, "html", "<h2>Cabecera</h2><p>Cuerpo <b>fuerte</b></p>", res.Content)
}

// --- Detección / límites ---

func TestDetectFormatByMagic(t *testing.T) {
	// Sin extensión reconocida: se olfatea el contenido.
	if f := detectFormat("data.bin", []byte("<html><body>hola</body></html>")); f != fmtHTML {
		t.Errorf("magic html = %v", f)
	}
	if f := detectFormat("data", []byte("solo texto plano")); f != fmtText {
		t.Errorf("magic texto = %v", f)
	}
	// Un docx real (por firma ZIP + parte word/).
	docxBytes := zipStore(t, map[string]string{"word/document.xml": `<w:document xmlns:w="x"><w:body/></w:document>`})
	if f := detectFormat("noext", docxBytes); f != fmtDocx {
		t.Errorf("magic docx = %v", f)
	}
}

// TestDetectFormatFirmaSobreExtension: la FIRMA manda sobre la extensión. Un
// binario ofimático con la extensión equivocada (adjunto renombrado, descarga sin
// extensión) se leía como texto plano y volcaba el ZIP crudo —«PK…»— dentro del
// documento: destrozo silencioso, el mismo agujero que ya se cerró para el PDF.
func TestDetectFormatFirmaSobreExtension(t *testing.T) {
	docx := zipStore(t, map[string]string{
		"word/document.xml": `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
			`<w:body><w:p><w:r><w:t>Contenido real</w:t></w:r></w:p></w:body></w:document>`,
	})
	xlsx := zipStore(t, map[string]string{
		"xl/workbook.xml": `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"/>`,
	})
	for _, tc := range []struct {
		name string
		data []byte
		want srcFormat
	}{
		{"informe.txt", docx, fmtDocx}, // .docx renombrado a .txt
		{"informe.csv", docx, fmtDocx}, // ...o a .csv
		{"adjunto", xlsx, fmtXlsx},     // .xlsx sin extensión
		{"libro.docx", xlsx, fmtXlsx},  // extensión ofimática pero equivocada
		{"nota.txt", []byte("hola"), fmtText},
	} {
		if got := detectFormat(tc.name, tc.data); got != tc.want {
			t.Errorf("detectFormat(%q) = %v, quiero %v", tc.name, got, tc.want)
		}
	}
	// Viaje completo: el .docx llamado .txt entra como documento, no como volcado.
	res, err := Import("informe.txt", docx)
	if err != nil {
		t.Fatalf("import docx renombrado: %v", err)
	}
	if res.Kind != KindDocument {
		t.Fatalf("kind = %q, quiero document", res.Kind)
	}
	assertBlocksEqual(t, "docx-renombrado", "<p>Contenido real</p>", res.Content)
	if strings.Contains(res.Content, "PK") {
		t.Fatalf("el ZIP crudo se coló en el documento: %q", res.Content)
	}
	// Un ZIP que no es ofimático se rechaza aunque se llame .txt (antes se volcaba).
	blob := zipStore(t, map[string]string{"cualquiera.bin": "x"})
	if _, err := Import("cosa.txt", blob); err == nil {
		t.Error("ZIP no ofimático llamado .txt debería fallar, no volcarse como texto")
	}
}

func TestImportRejectsEmptyAndOversize(t *testing.T) {
	if _, err := Import("x.txt", nil); err == nil {
		t.Error("fichero vacío debería fallar")
	}
	big := make([]byte, MaxImportBytes+1)
	if _, err := Import("x.txt", big); err == nil {
		t.Error("fichero sobredimensionado debería fallar")
	}
}

func TestImportUnknownFormat(t *testing.T) {
	// ZIP válido pero sin partes ofimáticas reconocibles.
	blob := zipStore(t, map[string]string{"random.txt": "nada"})
	if _, err := Import("cosa.bin", blob); err == nil {
		t.Error("formato desconocido debería fallar")
	}
}

// TestImportODFDeepNestingNoPanic comprueba que un content.xml con spans muy
// anidados (entrada maliciosa) no desborda la pila: el tope de profundidad corta
// la recursión. La prueba supera el test simplemente completando sin panic.
func TestImportODFDeepNestingNoPanic(t *testing.T) {
	const n = 5000
	inner := strings.Repeat(`<text:span>`, n) + "hola" + strings.Repeat(`</text:span>`, n)
	content := xmlHeader +
		`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
		`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">` +
		`<office:body><office:text><text:p>` + inner + `</text:p></office:text></office:body></office:document-content>`
	odt := zipStore(t, map[string]string{
		"mimetype":    "application/vnd.oasis.opendocument.text",
		"content.xml": content,
	})
	res, err := Import("profundo.odt", odt)
	if err != nil {
		t.Fatalf("import odt profundo: %v", err)
	}
	if res.Kind != KindDocument {
		t.Fatalf("kind = %q", res.Kind)
	}
}

// --- Unidad: parser inline de Markdown (ruta crítica) ---

func TestParseInlineMarkdown(t *testing.T) {
	runs := parseInline("a **b** c *d* `e` [f](u) ~~g~~ **sin cierre")
	// Reconstruye por texto+flags para una aserción legible.
	got := make([]string, 0, len(runs))
	for _, r := range runs {
		tag := ""
		if r.b {
			tag += "b"
		}
		if r.i {
			tag += "i"
		}
		if r.strike {
			tag += "s"
		}
		if r.link != "" {
			tag += "L"
		}
		got = append(got, tag+":"+r.text)
	}
	want := []string{":a ", "b:b", ": c ", "i:d", ": ", ":e", ": ", "L:f", ": ", "s:g", ": **sin cierre"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inline runs:\n got %#v\nwant %#v", got, want)
	}
}

// --- helpers ---

func parseSheetModelJSON(t *testing.T, content string) *sheetModel {
	t.Helper()
	var m sheetModel
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		t.Fatalf("sheet JSON inválido: %v\n%s", err, content)
	}
	return &m
}

func parseSlidesModelJSON(t *testing.T, content string) *slidesModel {
	t.Helper()
	var m slidesModel
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		t.Fatalf("slides JSON inválido: %v\n%s", err, content)
	}
	return &m
}

func assertCellsEqual(t *testing.T, where string, want, got map[string]string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s: nº de celdas want=%d got=%d\n want=%v\n got=%v", where, len(want), len(got), want, got)
	}
	for ref, w := range want {
		if got[ref] != w {
			t.Errorf("%s: celda %s = %q, quiero %q", where, ref, got[ref], w)
		}
	}
}

// zipStore construye un ZIP en memoria con los ficheros dados (sin comprimir).
func zipStore(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}
