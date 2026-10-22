// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"html"
	"io"
	"path"
	"regexp"
	"strings"
	"testing"
)

// --- utilidades de test ---

// stateWithContent crea un State con el modelo del editor en su clave CONTENT_KEY
// (doc:html / doc:sheet / doc:slides), replicando el doble-encoding del cliente:
// el registro LWW guarda una CADENA JSON (JSON.stringify del modelo).
func stateWithContent(t *testing.T, key, content string) *State {
	t.Helper()
	s := NewState()
	v, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := s.Apply(Op{Kind: OpSet, Key: key, Value: v, Dot: Dot{Actor: "A", Counter: 1}, Lamport: 1}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	return s
}

// zipParts abre un ZIP y devuelve name->contenido; valida que todo .xml/.rels sea
// XML bien formado (recorrido de tokens). Es la comprobación de "ZIP OOXML/ODF válido".
func zipParts(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip inválido: %v", err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("abrir %s: %v", f.Name, err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(b)
		if strings.HasSuffix(f.Name, ".xml") || strings.HasSuffix(f.Name, ".rels") {
			dec := xml.NewDecoder(bytes.NewReader(b))
			for {
				_, err := dec.Token()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("XML mal formado en %s: %v", f.Name, err)
				}
			}
		}
	}
	return out
}

func mustContain(t *testing.T, where, hay, needle string) {
	t.Helper()
	if !strings.Contains(hay, needle) {
		t.Fatalf("%s: falta %q en:\n%s", where, needle, hay)
	}
}

const richDocHTML = `<h1>Título</h1>` +
	`<p>Hola <b>negrita</b> <i>cursiva</i> <u>subrayado</u> <s>tachado</s> ` +
	`<a href="https://sytue.com/ruta">enlace</a></p>` +
	`<ul><li>uno</li><li>dos</li></ul>` +
	`<ol><li>primero</li></ol>` +
	`<table><tr><td>c1</td><td>c2</td></tr><tr><td>c3</td><td>c4</td></tr></table>` +
	`<p style="text-align:center"><span style="color:#ff0000">centrado rojo</span></p>`

// --- DOCUMENTO: docx ---

func TestExportDocxPreservesFormatting(t *testing.T) {
	s := stateWithContent(t, "doc:html", richDocHTML)
	data, _, ext, err := Export(KindDocument, "MiDoc", s, FormatOOXML)
	if err != nil || ext != "docx" {
		t.Fatalf("docx: %v %s", err, ext)
	}
	parts := zipParts(t, data)
	for _, p := range []string{"[Content_Types].xml", "word/document.xml", "word/styles.xml", "word/numbering.xml", "word/_rels/document.xml.rels"} {
		if _, ok := parts[p]; !ok {
			t.Fatalf("falta parte OPC %q", p)
		}
	}
	doc := parts["word/document.xml"]
	mustContain(t, "docx", doc, `<w:pStyle w:val="Title"/>`)
	mustContain(t, "docx", doc, `<w:pStyle w:val="Heading1"/>`)
	mustContain(t, "docx", doc, "negrita")
	mustContain(t, "docx", doc, `<w:b/>`)
	mustContain(t, "docx", doc, `<w:i/>`)
	mustContain(t, "docx", doc, `<w:u w:val="single"/>`)
	mustContain(t, "docx", doc, `<w:strike/>`)
	mustContain(t, "docx", doc, `<w:numPr>`)
	mustContain(t, "docx", doc, `<w:tbl>`)
	mustContain(t, "docx", doc, "c3")
	mustContain(t, "docx", doc, `<w:jc w:val="center"/>`)
	mustContain(t, "docx", doc, `<w:color w:val="FF0000"/>`)
	mustContain(t, "docx", doc, `<w:hyperlink`)
	mustContain(t, "docx-rels", parts["word/_rels/document.xml.rels"], "https://sytue.com/ruta")
	mustContain(t, "docx-rels", parts["word/_rels/document.xml.rels"], `TargetMode="External"`)
}

// --- DOCUMENTO: odt ---

func TestExportOdtPreservesFormatting(t *testing.T) {
	s := stateWithContent(t, "doc:html", richDocHTML)
	data, _, ext, err := Export(KindDocument, "MiDoc", s, FormatODF)
	if err != nil || ext != "odt" {
		t.Fatalf("odt: %v %s", err, ext)
	}
	parts := zipParts(t, data)
	c := parts["content.xml"]
	mustContain(t, "odt", c, `<text:h`)
	mustContain(t, "odt", c, "negrita")
	mustContain(t, "odt", c, `fo:font-weight="bold"`)
	mustContain(t, "odt", c, `fo:font-style="italic"`)
	mustContain(t, "odt", c, `style:text-underline-style="solid"`)
	mustContain(t, "odt", c, `style:text-line-through-style="solid"`)
	mustContain(t, "odt", c, `<text:list`)
	mustContain(t, "odt", c, `<table:table`)
	mustContain(t, "odt", c, "c4")
	mustContain(t, "odt", c, `fo:text-align="center"`)
	mustContain(t, "odt", c, `fo:color="#FF0000"`)
	mustContain(t, "odt", c, `<text:a`)
	mustContain(t, "odt", c, "https://sytue.com/ruta")
}

// --- HOJA: xlsx ---

func sheetJSON(t *testing.T) string {
	t.Helper()
	m := map[string]any{
		"active": 0,
		"sheets": []map[string]any{
			{
				"name": "Datos",
				"cells": map[string]string{
					"A1": "Nombre", "B1": "<b>10</b>", "A2": "Total", "B2": "=B1*2",
				},
				"formats": map[string]any{
					"B1": map[string]any{"id": "currency"},
					"B2": map[string]any{"id": "number", "decimals": 1},
				},
			},
			{"name": "Hoja2", "cells": map[string]string{"A1": "x"}},
		},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal sheet: %v", err)
	}
	return string(b)
}

func TestExportXlsxValuesFormulasFormats(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", sheetJSON(t))
	data, _, ext, err := Export(KindSpreadsheet, "Libro", s, FormatOOXML)
	if err != nil || ext != "xlsx" {
		t.Fatalf("xlsx: %v %s", err, ext)
	}
	parts := zipParts(t, data)
	sheet1 := parts["xl/worksheets/sheet1.xml"]
	mustContain(t, "xlsx", sheet1, `<f>B1*2</f>`) // fórmula
	mustContain(t, "xlsx", sheet1, `<v>10</v>`)   // valor numérico
	mustContain(t, "xlsx", sheet1, "Nombre")      // texto
	if _, ok := parts["xl/worksheets/sheet2.xml"]; !ok {
		t.Fatal("falta la segunda hoja (sheet2.xml)")
	}
	wb := parts["xl/workbook.xml"]
	mustContain(t, "xlsx", wb, `name="Datos"`)
	mustContain(t, "xlsx", wb, `name="Hoja2"`)
	mustContain(t, "xlsx", wb, `fullCalcOnLoad="1"`)
	styles := parts["xl/styles.xml"]
	mustContain(t, "xlsx", styles, "€") // formato moneda
	mustContain(t, "xlsx", styles, `applyNumberFormat="1"`)
}

// --- HOJA: ods ---

func TestExportOdsValuesFormulasFormats(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", sheetJSON(t))
	data, _, ext, err := Export(KindSpreadsheet, "Libro", s, FormatODF)
	if err != nil || ext != "ods" {
		t.Fatalf("ods: %v %s", err, ext)
	}
	parts := zipParts(t, data)
	c := parts["content.xml"]
	mustContain(t, "ods", c, `table:formula="of:=[.B1]*2"`)
	mustContain(t, "ods", c, `office:value="10"`)
	mustContain(t, "ods", c, `table:name="Datos"`)
	mustContain(t, "ods", c, `table:name="Hoja2"`)
	mustContain(t, "ods", c, `<number:currency-symbol`)
}

// --- PRESENTACIÓN: pptx ---

// pixel PNG 1x1 en base64 (data URL) para probar el embebido de imágenes.
const onePxPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func slidesJSON(t *testing.T) string {
	t.Helper()
	m := map[string]any{
		"slides": []map[string]any{
			{
				"id": "s1",
				"boxes": []map[string]any{
					{"x": 10, "y": 10, "w": 80, "h": 20, "kind": "text", "html": "<h1>Portada</h1><p><b>subtítulo</b></p>"},
					{"x": 10, "y": 40, "w": 30, "h": 30, "kind": "shape", "shape": "ellipse", "fill": "#00FF00", "stroke": "#000000", "html": ""},
					{"x": 50, "y": 40, "w": 30, "h": 30, "kind": "image", "src": onePxPNG, "html": ""},
				},
				"notes": "Hablar despacio",
			},
			{
				"id":    "s2",
				"boxes": []map[string]any{{"x": 5, "y": 5, "w": 90, "h": 90, "kind": "text", "html": "<p>Segunda diapositiva</p>"}},
			},
		},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal slides: %v", err)
	}
	return string(b)
}

func TestExportPptxAllSlides(t *testing.T) {
	s := stateWithContent(t, "doc:slides", slidesJSON(t))
	data, _, ext, err := Export(KindPresentation, "Charla", s, FormatOOXML)
	if err != nil || ext != "pptx" {
		t.Fatalf("pptx: %v %s", err, ext)
	}
	parts := zipParts(t, data)
	sl1, ok1 := parts["ppt/slides/slide1.xml"]
	sl2, ok2 := parts["ppt/slides/slide2.xml"]
	if !ok1 || !ok2 {
		t.Fatal("faltan diapositivas (deben salir TODAS, no solo la primera)")
	}
	mustContain(t, "pptx", sl1, "Portada")
	mustContain(t, "pptx", sl1, "subtítulo")
	mustContain(t, "pptx", sl1, `prst="ellipse"`)
	mustContain(t, "pptx", sl1, `val="00FF00"`)
	mustContain(t, "pptx", sl1, "<p:pic>")
	mustContain(t, "pptx", sl2, "Segunda diapositiva")
	if _, ok := parts["ppt/media/image1.png"]; !ok {
		t.Fatal("la imagen de la diapositiva no se embebió")
	}
	mustContain(t, "pptx", parts["ppt/notesSlides/notesSlide1.xml"], "Hablar despacio")
	if _, ok := parts["ppt/notesMasters/notesMaster1.xml"]; !ok {
		t.Fatal("falta el notesMaster requerido por las notas")
	}
	pres := parts["ppt/presentation.xml"]
	if strings.Count(pres, "<p:sldId ") != 2 {
		t.Fatalf("presentation.xml debe listar 2 diapositivas: %s", pres)
	}
	// master + layout + theme presentes (validez PowerPoint)
	for _, p := range []string{"ppt/slideMasters/slideMaster1.xml", "ppt/slideLayouts/slideLayout1.xml", "ppt/theme/theme1.xml"} {
		if _, ok := parts[p]; !ok {
			t.Fatalf("falta parte requerida %q", p)
		}
	}
}

// --- PRESENTACIÓN: odp ---

func TestExportOdpAllSlides(t *testing.T) {
	s := stateWithContent(t, "doc:slides", slidesJSON(t))
	data, _, ext, err := Export(KindPresentation, "Charla", s, FormatODF)
	if err != nil || ext != "odp" {
		t.Fatalf("odp: %v %s", err, ext)
	}
	parts := zipParts(t, data)
	c := parts["content.xml"]
	if strings.Count(c, "<draw:page ") != 2 {
		t.Fatalf("odp debe tener 2 páginas: %s", c)
	}
	mustContain(t, "odp", c, "Portada")
	mustContain(t, "odp", c, "Segunda diapositiva")
	mustContain(t, "odp", c, "<draw:ellipse")
	mustContain(t, "odp", c, "<presentation:notes>")
	mustContain(t, "odp", c, "Hablar despacio")
	if _, ok := parts["Pictures/img1.png"]; !ok {
		t.Fatal("la imagen no se embebió en el odp")
	}
	if _, ok := parts["styles.xml"]; !ok {
		t.Fatal("odp necesita styles.xml (página maestra)")
	}
	mustContain(t, "odp", parts["META-INF/manifest.xml"], "Pictures/img1.png")
}

// --- HOJA: Markdown, HTML y PDF (los valores, no las fórmulas) ---

// TestExportSheetShowsValues: las tres descargas de lectura de una hoja son una tabla
// por hoja con lo que se VE —el valor calculado con su formato de número—, no la
// fórmula ni un documento vacío. Los números son los de Calcus y la web: «10,00 €»
// (moneda es-ES con espacio duro, y en negrita como la celda) y «20,0» (número con un
// decimal).
func TestExportSheetShowsValues(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", sheetJSON(t))
	md, _, _, err := Export(KindSpreadsheet, "Libro", s, FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, "md", string(md), "## Datos\n\n| Nombre | **10,00\u00a0€** |\n| --- | --- |\n| Total | 20,0 |")
	mustContain(t, "md", string(md), "## Hoja2\n\n| x |")
	htmlOut, _, _, err := Export(KindSpreadsheet, "Libro", s, FormatHTML)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, "html", string(htmlOut), "<h2>Datos</h2>")
	mustContain(t, "html", string(htmlOut), "<thead><tr><th>Nombre</th><th><b>10,00\u00a0€</b></th></tr></thead>")
	mustContain(t, "html", string(htmlOut), "<tr><td>Total</td><td>20,0</td></tr>")
	pdf, _, _, err := Export(KindSpreadsheet, "Libro", s, FormatPDF)
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range [][]byte{md, htmlOut, pdf} {
		if bytes.Contains(out, []byte("B1*2")) {
			t.Errorf("una descarga de lectura enseña la fórmula en vez de su valor:\n%s", out)
		}
	}
	if !bytes.Contains(pdf, []byte("(Total | 20,0)")) {
		t.Errorf("el PDF no lleva la fila con su valor: %q", pdf)
	}
}

// TestExportSheetEvaluatesTheWholeBook: una fórmula que lee de otra hoja se calcula, un
// literal con ceros delante sale tal cual y una máscara personalizada se aplica.
func TestExportSheetEvaluatesTheWholeBook(t *testing.T) {
	const doc = `{"active":1,"sheets":[{"name":"Datos","cells":{"A1":"5","A2":"007"}},` +
		`{"name":"Resumen","cells":{"A1":"=Datos!A1*3","B1":"1234.5","C1":"=1/0"},` +
		`"formats":{"B1":{"id":"custom","mask":"#,##0.00"}}}]}`
	s := stateWithContent(t, "doc:sheet", doc)
	md, _, _, err := Export(KindSpreadsheet, "", s, FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, "md", string(md), "| 5 |\n| --- |\n| 007 |")
	mustContain(t, "md", string(md), "| 15 | 1.234,50 | #DIV/0! |")
}

// TestExportSheetStructuredRefsAndStyles: `Tabla[Columna]` se calcula con las tablas del
// libro (antes salía #REF!), y la negrita, la cursiva y la alineación de la celda llegan
// al Markdown y al HTML.
func TestExportSheetStructuredRefsAndStyles(t *testing.T) {
	const doc = `{"active":0,"sheets":[{"name":"Datos",` +
		`"cells":{"A1":"<b>Precio</b>","B1":"<i>Nota</i>","A2":"<div style=\"text-align: right;\">4</div>",` +
		`"A3":"<div style=\"text-align: right;\">6</div>","B2":"x","D1":"=SUM(Ventas[Precio])"},` +
		`"tables":[{"name":"Ventas","range":"A1:B3"}]}]}`
	s := stateWithContent(t, "doc:sheet", doc)
	md, _, _, err := Export(KindSpreadsheet, "", s, FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, "md", string(md), "| **Precio** | *Nota* |  | 10 |\n| ---: | --- | --- | --- |\n| 4 | x |  |  |")
	htmlOut, _, _, err := Export(KindSpreadsheet, "", s, FormatHTML)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, "html", string(htmlOut), "<th><b>Precio</b></th><th><i>Nota</i></th><th></th><th>10</th>")
	mustContain(t, "html", string(htmlOut), `<td style="text-align: right;">4</td>`)
}

// --- PDF ---

func TestExportPDFAllKinds(t *testing.T) {
	cases := []struct {
		kind    Kind
		key     string
		content string
		token   string
	}{
		{KindDocument, "doc:html", "<h1>Informe</h1><p>Contenido del documento</p>", "Contenido"},
		{KindSpreadsheet, "doc:sheet", sheetJSON(t), "Nombre"},
		{KindPresentation, "doc:slides", slidesJSON(t), "Portada"},
	}
	for _, tc := range cases {
		s := stateWithContent(t, tc.key, tc.content)
		data, ct, ext, err := Export(tc.kind, "Doc", s, FormatPDF)
		if err != nil || ext != "pdf" || ct != "application/pdf" {
			t.Fatalf("%s pdf: %v %s %s", tc.kind, err, ct, ext)
		}
		if !bytes.HasPrefix(data, []byte("%PDF-1.")) {
			t.Fatalf("%s: cabecera PDF ausente", tc.kind)
		}
		if !bytes.Contains(data, []byte("%%EOF")) {
			t.Fatalf("%s: sin marca %%EOF", tc.kind)
		}
		if !bytes.Contains(data, []byte("startxref")) {
			t.Fatalf("%s: sin tabla xref", tc.kind)
		}
		if !bytes.Contains(data, []byte(tc.token)) {
			t.Fatalf("%s: el PDF no contiene el texto %q", tc.kind, tc.token)
		}
	}
}

// HTML patológicamente anidado (miles de <div> o <table>) no debe desbordar la
// pila: el tope maxWalkDepth lo corta. Entrada no confiable (blob del editor).
func TestParseHTMLBlocksDeepNestingNoPanic(t *testing.T) {
	deepDiv := strings.Repeat("<div>", 20000) + "hola" + strings.Repeat("</div>", 20000)
	if blocks := parseHTMLBlocks(deepDiv); len(blocks) == 0 {
		t.Skip("sin bloques (aceptable: se cortó por profundidad)")
	}
	deepTable := strings.Repeat("<table><tr><td>", 20000) + "x"
	_ = parseHTMLBlocks(deepTable) // no debe hacer panic
	// Un docx a partir de ello sigue siendo un ZIP válido.
	s := stateWithContent(t, "doc:html", deepDiv)
	data, _, _, err := Export(KindDocument, "T", s, FormatOOXML)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	zipParts(t, data)
}

// --- conversor de fórmula A1 -> OpenFormula (ODF) ---

func TestA1FormulaToODF(t *testing.T) {
	cases := map[string]string{
		"A1+B1":           "of:=[.A1]+[.B1]",
		"SUM(A1:B2)":      "of:=SUM([.A1:.B2])",
		"A1*2":            "of:=[.A1]*2",
		"IF(A1>0;A1;0)":   "of:=IF([.A1]>0;[.A1];0)",
		`CONCAT("A1";B2)`: `of:=CONCAT("A1";[.B2])`, // "A1" dentro de cadena no se toca
		"Hoja2!C3":        "of:=[Hoja2.C3]",
		"'Mi hoja'!C3":    "of:=['Mi hoja'.C3]", // la comilla se queda: sin ella no es ODF
		"AB1_x+1":         "of:=AB1_x+1",        // un nombre con forma de celda al principio
	}
	for in, want := range cases {
		if got := a1FormulaToODF(in); got != want {
			t.Errorf("a1FormulaToODF(%q) = %q; quiero %q", in, got, want)
		}
	}
}

// El HTML de una presentación son sus LÁMINAS, con cada caja en su sitio: es lo
// que pinta la vista pública `/s/{token}`. Antes salía texto plano, sin láminas.
func TestExportHTMLPresentacionPintaLaminas(t *testing.T) {
	s := stateWithContent(t, "doc:slides", slidesJSON(t))
	data, _, _, err := Export(KindPresentation, "Doc", s, FormatHTML)
	if err != nil {
		t.Fatal(err)
	}
	h := string(data)
	if n := strings.Count(h, `<section class="lamina"`); n != 2 {
		t.Fatalf("láminas = %d, quiero 2", n)
	}
	for _, want := range []string{
		"<h1>Portada</h1>", "left:10.000%;top:10.000%;width:80.000%;height:20.000%",
		"background:#00FF00", "border-radius:50%", `<img alt="" src="data:image/png`, "Segunda diapositiva",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("falta %q", want)
		}
	}
	if strings.Contains(h, "Hablar despacio") {
		t.Error("las notas del ponente no se publican")
	}
}

// --- PRESENTACIÓN: la batería de fidelidad de .pptx y .odp ---
//
// Cada prueba recorre los DOS formatos con el mismo modelo, de modo que lo que se mide en
// uno se mide en el otro: el paquete íntegro (nada colgando ni sin declarar), el modelo de
// todo a la vez como punto fijo del viaje de ida y vuelta, el texto con caracteres que
// rompen XML en cada campo libre, y la entrada hostil.

// modeloTotal es una presentación con TODO lo que el editor guarda a la vez: un tema propio,
// el patrón con sus campos y dos diseños, fondo propio, patrón oculto, las tres transiciones,
// animaciones de cada clase, conectores, formas, listas, notas, y un vídeo y un audio con sus
// subtítulos.
func modeloTotal(t *testing.T) slidesModel {
	t.Helper()
	var m slidesModel
	if err := json.Unmarshal([]byte(modeloCompleto(t)), &m); err != nil {
		t.Fatal(err)
	}
	m.Theme = "custom-1"
	m.Themes = []customTheme{{ID: "custom-1", Name: "Cooperativa", Bg: "#F4F1EA", Ink: "#1B2A41", Font: "Georgia"}}
	m.Master.Layouts = []slideLayout{
		{ID: "portada", Name: "Portada", Boxes: []slideBox{{ID: "p", X: 10, Y: 35, W: 80, H: 20, Kind: "text", HTML: "<h1>Título de la portada</h1>"}}},
		{ID: "dos", Name: "Dos columnas", Boxes: []slideBox{
			{ID: "a", X: 8, Y: 8, W: 84, H: 16, Kind: "text", HTML: "<h1>Título</h1>"},
			{ID: "b", X: 8, Y: 30, W: 40, H: 60, Kind: "text", HTML: "<p>Izquierda</p>"},
		}},
	}
	m.Slides[0].Layout, m.Slides[1].Layout = "portada", "dos"
	m.Slides[0].Notes = "Hablar despacio.\nPausa tras el dato."
	m.Slides[1].Boxes[0].Tracks = []slideTrack{{Lang: "es", VTT: vttPrueba, Default: true}, {Lang: "en", VTT: vttPrueba}}
	m.Slides[1].Boxes[1].Tracks = []slideTrack{{Lang: "fr", VTT: vttPrueba, Default: true}}
	m.Slides[2].Boxes = append(m.Slides[2].Boxes, slideBox{
		ID: "l", X: 10, Y: 30, W: 80, H: 50, Kind: "text", HTML: "<ul><li>uno<ul><li>anidado</li></ul></li></ul><ol><li>primero</li></ol>",
	})
	return m
}

func exportarPresentacion(t *testing.T, formato BlobFormat, m slidesModel) []byte {
	t.Helper()
	if formato == FormatODF {
		return exportarOdp(t, jsonDe(t, m))
	}
	return exportarPptx(t, jsonDe(t, m))
}

var formatosPresentacion = map[string]struct {
	formato BlobFormat
	ext     string
}{"pptx": {FormatOOXML, "pptx"}, "odp": {FormatODF, "odp"}}

// TestPresentacionPaqueteIntegro: el paquete no deja nada colgando ni sin declarar. En el
// .pptx, cada parte tiene tipo de contenido y cada relación apunta a una parte que existe; en
// el .odp, el manifiesto lista cada fichero y cada fichero listado existe, y `mimetype` va
// primero y sin comprimir. Los identificadores de forma no se repiten en una diapositiva.
func TestPresentacionPaqueteIntegro(t *testing.T) {
	m := modeloTotal(t)
	t.Run("pptx", func(t *testing.T) {
		data := exportarPresentacion(t, FormatOOXML, m)
		parts := zipParts(t, data)
		tipos := parts["[Content_Types].xml"]
		for name := range parts {
			if name == "[Content_Types].xml" {
				continue
			}
			ext := strings.TrimPrefix(path.Ext(name), ".")
			if !strings.Contains(tipos, `PartName="/`+name+`"`) && !strings.Contains(tipos, `Extension="`+ext+`"`) {
				t.Errorf("la parte %s no tiene tipo de contenido", name)
			}
		}
		for name, content := range parts {
			if !strings.HasSuffix(name, ".rels") {
				continue
			}
			dir := path.Dir(path.Dir(name)) // la carpeta de la parte que declara las relaciones
			for _, mt := range regexp.MustCompile(`Target="([^"]+)"( TargetMode="External")?`).FindAllStringSubmatch(content, -1) {
				if mt[2] != "" {
					continue
				}
				target := path.Clean(path.Join(dir, mt[1]))
				if strings.HasPrefix(mt[1], "/") {
					target = strings.TrimPrefix(mt[1], "/")
				}
				if _, ok := parts[target]; !ok {
					t.Errorf("%s apunta a %s, que no existe", name, target)
				}
			}
		}
		for name, content := range parts {
			if !strings.HasPrefix(name, "ppt/slides/slide") || !strings.HasSuffix(name, ".xml") {
				continue
			}
			vistos := map[string]bool{}
			for _, mt := range regexp.MustCompile(`<p:cNvPr id="(\d+)"`).FindAllStringSubmatch(content, -1) {
				if vistos[mt[1]] {
					t.Errorf("%s repite el id de forma %s", name, mt[1])
				}
				vistos[mt[1]] = true
			}
		}
	})
	t.Run("odp", func(t *testing.T) {
		data := exportarPresentacion(t, FormatODF, m)
		parts := zipParts(t, data)
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		if f := zr.File[0]; f.Name != "mimetype" || f.Method != zip.Store {
			t.Errorf("el primer fichero es %s (método %d), quiero mimetype sin comprimir", f.Name, f.Method)
		}
		manifiesto := parts["META-INF/manifest.xml"]
		listados := map[string]bool{}
		for _, mt := range regexp.MustCompile(`manifest:full-path="([^"]+)"`).FindAllStringSubmatch(manifiesto, -1) {
			listados[mt[1]] = true
			if _, ok := parts[mt[1]]; !ok && mt[1] != "/" && !strings.HasSuffix(mt[1], "/") {
				t.Errorf("el manifiesto lista %s, que no existe", mt[1])
			}
		}
		for name := range parts {
			if name != "mimetype" && name != "META-INF/manifest.xml" && !listados[name] {
				t.Errorf("el fichero %s no está en el manifiesto", name)
			}
		}
		vistos := map[string]bool{}
		for _, mt := range regexp.MustCompile(` draw:id="([^"]+)"`).FindAllStringSubmatch(parts["content.xml"], -1) {
			if vistos[mt[1]] {
				t.Errorf("content.xml repite el draw:id %s", mt[1])
			}
			vistos[mt[1]] = true
		}
	})
}

// TestPresentacionTotalEsPuntoFijo: abrir lo exportado y volver a exportarlo no cambia
// nada. Si tras una vuelta el modelo es otro, cada guardado desde LibreOffice o PowerPoint
// iría erosionando la presentación.
func TestPresentacionTotalEsPuntoFijo(t *testing.T) {
	m := modeloTotal(t)
	for nombre, f := range formatosPresentacion {
		t.Run(nombre, func(t *testing.T) {
			uno := reimportar(t, "uno."+f.ext, exportarPresentacion(t, f.formato, m))
			dos := reimportar(t, "dos."+f.ext, exportarPresentacion(t, f.formato, uno))
			a, b := jsonDe(t, uno), jsonDe(t, dos)
			if a != b {
				t.Errorf("la segunda vuelta cambia el modelo.\nprimera: %s\nsegunda: %s", a, b)
			}
			// Y la primera vuelta conserva lo que el formato sabe llevar.
			if len(uno.Slides) != 3 || len(uno.Master.Layouts) != 2 || len(uno.Themes) != 1 {
				t.Errorf("modelo tras la vuelta: %d diapositivas, patrón %+v, temas %+v", len(uno.Slides), uno.Master, uno.Themes)
			}
			if got := uno.Slides[1].Boxes[0].Tracks; len(got) != 2 {
				t.Errorf("subtítulos del vídeo = %+v", got)
			}
			if uno.Slides[0].Notes != "Hablar despacio.\nPausa tras el dato." {
				t.Errorf("notas = %q", uno.Slides[0].Notes)
			}
		})
	}
}

// TestPresentacionTextoQueRompeXML: los caracteres que rompen XML, comillas incluidas, en
// cada campo libre —nombre del tema, del diseño y de una pista, notas, texto de una forma y
// de una caja— salen escapados (el paquete sigue bien formado) y vuelven tal cual.
func TestPresentacionTextoQueRompeXML(t *testing.T) {
	const raro = `<&"'>Ñandú ✓ 日本`
	m := modeloTotal(t)
	m.Themes[0].Name = "Tema " + raro
	m.Master.Layouts[0].Name = "Diseño " + raro
	m.Slides[1].Boxes[0].Tracks[0].Label = "Pista " + raro
	m.Slides[0].Notes = "Notas " + raro
	m.Slides[0].Boxes[0].HTML = "<p>" + html.EscapeString("Forma "+raro) + "</p>"
	for nombre, f := range formatosPresentacion {
		t.Run(nombre, func(t *testing.T) {
			got := reimportar(t, "raro."+f.ext, exportarPresentacion(t, f.formato, m)) // zipParts valida el XML
			if len(got.Themes) == 0 || got.Master == nil || len(got.Master.Layouts) == 0 {
				t.Fatalf("se perdió el tema o los diseños: %+v", got)
			}
			if got.Themes[0].Name != "Tema "+raro {
				t.Errorf("nombre del tema = %q", got.Themes[0].Name)
			}
			if got.Master.Layouts[0].Name != "Diseño "+raro {
				t.Errorf("nombre del diseño = %q", got.Master.Layouts[0].Name)
			}
			if got.Slides[1].Boxes[0].Tracks[0].Label != "Pista "+raro {
				t.Errorf("etiqueta de la pista = %q", got.Slides[1].Boxes[0].Tracks[0].Label)
			}
			if got.Slides[0].Notes != "Notas "+raro {
				t.Errorf("notas = %q", got.Slides[0].Notes)
			}
			assertBlocksEqual(t, "forma", m.Slides[0].Boxes[0].HTML, got.Slides[0].Boxes[0].HTML)
		})
	}
}

// TestPresentacionEntradaHostil: un paquete roto, truncado o con anidamientos enormes no
// tumba el servicio ni se cuelga: o se rechaza o se abre lo que se pueda.
func TestPresentacionEntradaHostil(t *testing.T) {
	buenos := map[string][]byte{
		"pptx": exportarPresentacion(t, FormatOOXML, modeloTotal(t)),
		"odp":  exportarPresentacion(t, FormatODF, modeloTotal(t)),
	}
	profundo := strings.Repeat("<p:par>", 40_000) + strings.Repeat("</p:par>", 40_000)
	grupos := strings.Repeat("<draw:g>", 40_000) + strings.Repeat("</draw:g>", 40_000)
	casos := map[string]func(parts map[string]string, ext string) map[string]string{
		"sin partes": func(map[string]string, string) map[string]string { return map[string]string{} },
		"truncado": func(p map[string]string, ext string) map[string]string {
			k := map[string]string{"pptx": "ppt/slides/slide1.xml", "odp": "content.xml"}[ext]
			p[k] = p[k][:len(p[k])/2]
			return p
		},
		"anidado": func(p map[string]string, ext string) map[string]string {
			if ext == "pptx" {
				p["ppt/slides/slide1.xml"] = strings.Replace(p["ppt/slides/slide1.xml"], "</p:cSld>", "</p:cSld><p:timing>"+profundo+"</p:timing>", 1)
			} else {
				p["content.xml"] = strings.Replace(p["content.xml"], "</draw:page>", grupos+"</draw:page>", 1)
			}
			return p
		},
		"relaciones a ninguna parte": func(p map[string]string, ext string) map[string]string {
			if ext == "pptx" {
				p["ppt/slides/_rels/slide1.xml.rels"] = strings.ReplaceAll(p["ppt/slides/_rels/slide1.xml.rels"], `Target="`, `Target="../no-existe/`)
			}
			return p
		},
	}
	for ext, bueno := range buenos {
		for nombre, rota := range casos {
			t.Run(ext+"/"+nombre, func(t *testing.T) {
				parts := zipParts(t, bueno)
				data := reempaquetar(t, rota(parts, ext))
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("pánico al importar: %v", r)
					}
				}()
				res, err := Import("hostil."+ext, data)
				if err == nil && res.Kind != KindPresentation {
					t.Errorf("lo importado no es una presentación: %s", res.Kind)
				}
			})
		}
	}
}

// reempaquetar vuelve a comprimir unas partes (name → contenido) como un ZIP.
func reempaquetar(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range parts {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
