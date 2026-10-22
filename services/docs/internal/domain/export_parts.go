// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strconv"
	"strings"
)

// Partes estáticas (boilerplate) de los paquetes OPC (OOXML) y ODF: las piezas
// estructurales mínimas que ECMA-376 y OASIS ODF exigen para que el fichero abra
// en una suite ofimática. Los paquetes de hoja y presentación se generan de forma
// dinámica (número variable de hojas/diapositivas) en export_sheet.go y
// export_slides.go; aquí quedan las partes fijas del documento de texto y del ODF.
// Obra original de Sytue sobre las especificaciones públicas.

// --- OPC raíz / relaciones ---

const relsRoot = xmlHeader +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`</Relationships>`

const relsRootWorkbook = xmlHeader +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
	`</Relationships>`

const relsRootPresentation = xmlHeader +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>` +
	`</Relationships>`

// --- docx: [Content_Types].xml + estilos + numeración ---

const contentTypesDocx = xmlHeader +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
	`<Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/>` +
	`</Types>`

// docxStyles son los estilos del .docx exportado, y reflejan el folio del editor (editor.css):
// texto de 12 pt (16 px) con interlineado 1,5 y medio renglón después de cada párrafo; títulos 1
// a 3 en negrita, de 24, 18 y 14 pt, con su margen; de 4 a 6, como el texto; la cita en cursiva y
// gris, y los elementos de lista sin espacio entre ellos. La letra es Calibri: la del editor no
// tiene por qué estar en quien abre el fichero. Lo que el documento dice de cada párrafo va
// directo en él y pisa esto. El importador reconoce estos estilos y no los aplica.
var docxStyles = xmlHeader +
	`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
	`<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/><w:sz w:val="24"/></w:rPr></w:rPrDefault></w:docDefaults>` +
	`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:pPr><w:spacing w:after="120" w:line="300" w:lineRule="auto"/></w:pPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="240"/></w:pPr><w:rPr><w:b/><w:sz w:val="56"/></w:rPr></w:style>` +
	docxHeadingStyle(1, "322", "168", true, "48") +
	docxHeadingStyle(2, "299", "126", true, "36") +
	docxHeadingStyle(3, "281", "98", true, "28") +
	docxHeadingStyle(4, "0", "0", false, "24") +
	docxHeadingStyle(5, "0", "0", false, "24") +
	docxHeadingStyle(6, "0", "0", false, "24") +
	`<w:style w:type="paragraph" w:styleId="Quote"><w:name w:val="Quote"/><w:basedOn w:val="Normal"/><w:pPr><w:ind w:left="720" w:right="720"/></w:pPr><w:rPr><w:i/><w:color w:val="475569"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="ListParagraph"><w:name w:val="List Paragraph"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="0"/></w:pPr></w:style>` +
	`</w:styles>`

// docxStylesAnteriores son los estilos que escribían versiones anteriores de la exportación: el
// importador también los reconoce como suyos (parseDocxHoja).
var docxStylesAnteriores = []string{xmlHeader +
	`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
	`<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/><w:sz w:val="22"/></w:rPr></w:rPrDefault></w:docDefaults>` +
	`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="240"/></w:pPr><w:rPr><w:b/><w:sz w:val="56"/></w:rPr></w:style>` +
	docxHeadingStyleV1(1, "0", "48") +
	docxHeadingStyleV1(2, "1", "36") +
	docxHeadingStyleV1(3, "2", "28") +
	docxHeadingStyleV1(4, "3", "26") +
	docxHeadingStyleV1(5, "4", "24") +
	docxHeadingStyleV1(6, "5", "22") +
	`<w:style w:type="paragraph" w:styleId="Quote"><w:name w:val="Quote"/><w:basedOn w:val="Normal"/><w:pPr><w:ind w:left="720" w:right="720"/></w:pPr><w:rPr><w:i/></w:rPr></w:style>` +
	`</w:styles>`}

// docxHeadingStyle es el estilo del título n: su espacio antes y después (twips), si va en negrita
// y su tamaño en medios puntos.
func docxHeadingStyle(n int, antes, despues string, negrita bool, halfPt string) string {
	id := "Heading" + strconv.Itoa(n)
	b := ""
	if negrita {
		b = `<w:b/>`
	}
	return `<w:style w:type="paragraph" w:styleId="` + id + `"><w:name w:val="heading ` + strconv.Itoa(n) +
		`"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="` + antes + `" w:after="` + despues +
		`"/><w:outlineLvl w:val="` + strconv.Itoa(n-1) + `"/></w:pPr><w:rPr>` + b + `<w:sz w:val="` + halfPt + `"/></w:rPr></w:style>`
}

func docxHeadingStyleV1(n int, outline, halfPt string) string {
	id := "Heading" + strconv.Itoa(n)
	return `<w:style w:type="paragraph" w:styleId="` + id + `"><w:name w:val="heading ` + strconv.Itoa(n) +
		`"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="240" w:after="60"/><w:outlineLvl w:val="` + outline +
		`"/></w:pPr><w:rPr><w:b/><w:sz w:val="` + halfPt + `"/></w:rPr></w:style>`
}

// docxNumbering define dos listas (viñeta = numId 1, ordenada = numId 2) con 9
// niveles de anidamiento cada una.
var docxNumbering = buildDocxNumbering()

func buildDocxNumbering() string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`)
	// abstractNum 0: viñetas.
	b.WriteString(`<w:abstractNum w:abstractNumId="0">`)
	for lvl := 0; lvl < 9; lvl++ {
		ind := strconv.Itoa(720 * (lvl + 1))
		b.WriteString(`<w:lvl w:ilvl="` + strconv.Itoa(lvl) + `"><w:start w:val="1"/><w:numFmt w:val="bullet"/>` +
			`<w:lvlText w:val="&#8226;"/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="` + ind + `" w:hanging="360"/></w:pPr>` +
			`<w:rPr><w:rFonts w:ascii="Symbol" w:hAnsi="Symbol"/></w:rPr></w:lvl>`)
	}
	b.WriteString(`</w:abstractNum>`)
	// abstractNum 1: numerada.
	b.WriteString(`<w:abstractNum w:abstractNumId="1">`)
	for lvl := 0; lvl < 9; lvl++ {
		ind := strconv.Itoa(720 * (lvl + 1))
		b.WriteString(`<w:lvl w:ilvl="` + strconv.Itoa(lvl) + `"><w:start w:val="1"/><w:numFmt w:val="decimal"/>` +
			`<w:lvlText w:val="%` + strconv.Itoa(lvl+1) + `."/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="` + ind + `" w:hanging="360"/></w:pPr></w:lvl>`)
	}
	b.WriteString(`</w:abstractNum>`)
	b.WriteString(`<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>`)
	b.WriteString(`<w:num w:numId="2"><w:abstractNumId w:val="1"/></w:num>`)
	b.WriteString(`</w:numbering>`)
	return b.String()
}

// numeracionDocx es numbering.xml con, además de las dos listas de siempre, una de viñetas por
// cada viñeta propia (numId 3, 4…), con ese carácter en sus nueve niveles: en CSS la viñeta de
// una lista la heredan las anidadas.
func numeracionDocx(vinetas []string) string {
	if len(vinetas) == 0 {
		return docxNumbering
	}
	var b strings.Builder
	for i, v := range vinetas {
		b.WriteString(`<w:abstractNum w:abstractNumId="` + strconv.Itoa(i+2) + `">`)
		for lvl := 0; lvl < 9; lvl++ {
			ind := strconv.Itoa(720 * (lvl + 1))
			b.WriteString(`<w:lvl w:ilvl="` + strconv.Itoa(lvl) + `"><w:start w:val="1"/><w:numFmt w:val="bullet"/>` +
				`<w:lvlText w:val="` + xmlEscape(v) + `"/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="` + ind + `" w:hanging="360"/></w:pPr></w:lvl>`)
		}
		b.WriteString(`</w:abstractNum>`)
	}
	// Los abstractNum van todos antes que los num (CT_Numbering).
	abstractos := strings.Replace(docxNumbering, `<w:num w:numId="1">`, b.String()+`<w:num w:numId="1">`, 1)
	var nums strings.Builder
	for i := range vinetas {
		nums.WriteString(`<w:num w:numId="` + strconv.Itoa(i+3) + `"><w:abstractNumId w:val="` + strconv.Itoa(i+2) + `"/></w:num>`)
	}
	return strings.Replace(abstractos, `</w:numbering>`, nums.String()+`</w:numbering>`, 1)
}

// --- ODF: manifest ---

func odfManifest(mimetype string, hasStyles bool, media map[string][]byte) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.3">`)
	b.WriteString(`<manifest:file-entry manifest:full-path="/" manifest:media-type="` + xmlEscape(mimetype) + `"/>`)
	b.WriteString(`<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>`)
	if hasStyles {
		b.WriteString(`<manifest:file-entry manifest:full-path="styles.xml" manifest:media-type="text/xml"/>`)
	}
	for _, path := range sortedBytesKeys(media) {
		// Un gráfico incrustado de una hoja (`Object N/content.xml`) declara además su
		// carpeta como documento de gráfico, que es lo que LibreOffice busca.
		if dir, ok := strings.CutSuffix(path, "content.xml"); ok && strings.HasPrefix(dir, "Object ") {
			b.WriteString(`<manifest:file-entry manifest:full-path="` + xmlEscape(dir) +
				`" manifest:version="1.3" manifest:media-type="application/vnd.oasis.opendocument.chart"/>`)
		}
		tipo := mediaTypeOf(path)
		if strings.HasSuffix(path, ".svg") {
			// Solo aquí: mediaTypeOf no declara SVG como imagen para que un SVG de un fichero de
			// fuera no acabe en un <img> del editor.
			tipo = "image/svg+xml"
		}
		b.WriteString(`<manifest:file-entry manifest:full-path="` + xmlEscape(path) +
			`" manifest:media-type="` + tipo + `"/>`)
	}
	b.WriteString(`</manifest:manifest>`)
	return b.String()
}

// mediaTypeOf deriva el media-type de un binario embebido por su extensión.
func mediaTypeOf(path string) string {
	switch {
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".jpg"), strings.HasSuffix(path, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(path, ".gif"):
		return "image/gif"
	case strings.HasSuffix(path, ".xml"):
		return "text/xml"
	case strings.HasSuffix(path, ".vtt"):
		return "text/vtt"
	default:
		// Audio y vídeo incrustados (Media/…): su MIME, no un binario anónimo.
		if i := strings.LastIndexByte(path, '.'); i >= 0 {
			if m := mediaExts[path[i+1:]]; m != "" {
				return m
			}
		}
		return "application/octet-stream"
	}
}
