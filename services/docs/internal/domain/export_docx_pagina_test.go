// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"regexp"
	"strings"
	"testing"
)

// La página de un .docx (pgSz, pgMar y w:background) entra como la capa data-sytue-page y sale
// igual al exportar: una carta apaisada con márgenes distintos y papel de color vuelve como era.
// Antes se perdía al importar y la exportación no decía tamaño: Word la abría en su papel.
func TestDocxPaginaIdaYVuelta(t *testing.T) {
	cuerpo := `<w:p><w:r><w:t>Hola</w:t></w:r></w:p><w:sectPr><w:pgSz w:w="15840" w:h="12240" w:orient="landscape"/>` +
		`<w:pgMar w:top="720" w:right="1080" w:bottom="-1440" w:left="1800" w:header="708" w:footer="708" w:gutter="0"/></w:sectPr>`
	files := map[string]string{"word/document.xml": `<w:document ` + wNS + `><w:background w:color="FFF2CC"/><w:body>` + cuerpo + `</w:body></w:document>`}
	data, err := zipPackage(files)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Import("pagina.docx", data)
	if err != nil {
		t.Fatal(err)
	}
	capa := `<div data-sytue-page="" data-width="1056" data-height="816" data-margins="48 72 96 120" data-color="#FFF2CC" contenteditable="false" aria-hidden="true"></div>`
	if !strings.HasPrefix(res.Content, capa) {
		t.Fatalf("la página no entra como capa:\n%s", res.Content)
	}
	if limpio := SanearContenido("doc:html", res.Content); limpio != res.Content {
		t.Errorf("la capa cambia al sanearla, %s", difiere(res.Content, limpio))
	}

	salida, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", res.Content), FormatOOXML)
	if err != nil {
		t.Fatal(err)
	}
	partes, err := readZip(salida)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(partes["word/document.xml"])
	for _, q := range []string{
		`<w:background w:color="FFF2CC"/>`, `<w:pgSz w:w="15840" w:h="12240" w:orient="landscape"/>`,
		`<w:pgMar w:top="720" w:right="1080" w:bottom="1440" w:left="1800"`,
	} {
		mustContain(t, "página exportada", doc, q)
	}
	mustContain(t, "color de página visible en Word", string(partes["word/settings.xml"]), `<w:displayBackgroundShape/>`)
	otra, err := Import("vuelta.docx", salida)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(otra.Content, capa) {
		t.Errorf("la ida y vuelta cambia la página:\n%s", otra.Content)
	}

	// Sin capa, la exportación lleva el folio del editor: A4 con 96 px de margen.
	salida, _, _, err = Export(KindDocument, "", stateWithContent(t, "doc:html", "<p>Hola</p>"), FormatOOXML)
	if err != nil {
		t.Fatal(err)
	}
	partes, _ = readZip(salida)
	mustContain(t, "página por defecto", string(partes["word/document.xml"]), `<w:pgSz w:w="11910" w:h="16845"/><w:pgMar w:top="1440"`)
}

// La tipografía, el tamaño y el formato de párrafo, de celda y de lista viajan a .docx y vuelven:
// Docus -> .docx -> Docus no pierde ninguna declaración. Antes la exportación solo leía el color
// de los tramos y la alineación, y todo lo demás se quedaba en el editor.
func TestDocxFormatoIdaYVuelta(t *testing.T) {
	h := `<h1 style="font-family:Georgia,serif;font-size:20pt;font-weight:400">Título fino</h1>` +
		`<p style="font-family:'Avenir Next',sans-serif;font-size:11pt;color:#5F5F5F;margin-top:12px;margin-bottom:6px;line-height:1.38;margin-left:48px;text-indent:-24px;background-color:#FFF2CC;border-bottom:1px solid #000000;white-space:pre-wrap;tab-size:48px;--sytue-tabs:0px left, 200px right dot">` +
		`1.	Uno <span style="font-size:14pt;letter-spacing:1pt;text-transform:uppercase;background-color:#FFFF00">dos</span><sup>3</sup>	fin</p>` +
		`<p style="font-family:'Avenir Next',sans-serif;font-size:9pt;line-height:12pt"><br></p>` +
		`<ul style="list-style-type:'- '"><li>uno</li></ul>` +
		`<table style="width:400px;margin-left:24px"><tbody><tr><td style="border-top:2px solid #4472C4;border-bottom:none;background-color:#D9E2F3;padding:2px 4px;width:200px;vertical-align:middle">celda</td>` +
		`<td style="width:200px">otra</td></tr></tbody></table>`
	salida, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", h), FormatOOXML)
	if err != nil {
		t.Fatal(err)
	}
	partes, err := readZip(salida)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(partes["word/document.xml"])
	for _, q := range []string{
		`<w:rFonts w:ascii="Georgia"`, `<w:b w:val="0"/>`, `<w:sz w:val="40"/>`, // el título, sin negrita
		`<w:pBdr><w:bottom w:val="single" w:sz="6" w:space="0" w:color="000000"/></w:pBdr>`,
		`<w:shd w:val="clear" w:color="auto" w:fill="FFF2CC"/>`,
		`<w:tabs><w:tab w:val="right" w:leader="dot" w:pos="3720"/></w:tabs>`, // (200 + 48) px desde el margen
		`<w:spacing w:before="180" w:after="90" w:line="276" w:lineRule="auto"/>`, `<w:ind w:left="720" w:hanging="360"/>`,
		`<w:caps/>`, `<w:spacing w:val="20"/>`, `<w:sz w:val="28"/>`, `w:fill="FFFF00"`, `<w:vertAlign w:val="superscript"/>`,
		`<w:tab/>`, `<w:spacing w:line="240" w:lineRule="exact"/>`, // el párrafo vacío: su letra va en la marca
		`<w:pStyle w:val="ListParagraph"/>`,
		`<w:tblW w:w="6000" w:type="dxa"/>`, `<w:tblInd w:w="360" w:type="dxa"/>`,
		`<w:tcBorders><w:top w:val="single" w:sz="12" w:space="0" w:color="4472C4"/><w:bottom w:val="nil"/></w:tcBorders>`,
		`<w:vAlign w:val="center"/>`,
	} {
		mustContain(t, "formato exportado", doc, q)
	}
	mustContain(t, "viñeta propia", string(partes["word/numbering.xml"]), `<w:lvlText w:val="-"/>`)
	mustContain(t, "genérica de la fuente", string(partes["word/fontTable.xml"]), `<w:font w:name="Avenir Next"><w:family w:val="swiss"/>`)
	mustContain(t, "tabulador por defecto", string(partes["word/settings.xml"]), `<w:defaultTabStop w:val="720"/>`)

	vuelta, err := Import("vuelta.docx", salida)
	if err != nil {
		t.Fatal(err)
	}
	antes, despues := declaracionesDe(h), declaracionesDe(vuelta.Content)
	for d, n := range antes {
		if despues[d] < n {
			t.Errorf("la ida y vuelta pierde %q (%d de %d)", d, despues[d], n)
		}
	}
	if strings.Count(vuelta.Content, "\t") != 2 {
		t.Errorf("los tabuladores no vuelven:\n%s", vuelta.Content)
	}
}

// declaracionesDe cuenta las declaraciones de los atributos style de un HTML.
func declaracionesDe(h string) map[string]int {
	out := map[string]int{}
	for _, m := range regexp.MustCompile(`style="([^"]*)"`).FindAllStringSubmatch(h, -1) {
		for _, d := range strings.Split(m[1], ";") {
			if d = strings.TrimSpace(d); d != "" {
				out[d]++
			}
		}
	}
	return out
}

// La base de carácter de un párrafo que todos sus tramos pisan no se escribe muerta en <p>,
// los espacios duros (\u00a0), ideográficos (\u3000) o de sangría con formato no se tiran al
// exportar, y un salto de línea borrado con control de cambios (<del data-track><br></del>)
// vuelve íntegro tras exportar y reimportar.
func TestDocxIdaYVueltaSinPerdidasDeBaseNiEspacios(t *testing.T) {
	styles := `<w:styles ` + wNS + `><w:docDefaults><w:rPrDefault><w:rPr>` +
		`<w:rFonts w:ascii="Times New Roman"/><w:color w:val="5F5F5F"/><w:sz w:val="20"/>` +
		`</w:rPr></w:rPrDefault></w:docDefaults></w:styles>`
	cuerpo := `<w:p>` +
		`<w:r><w:rPr><w:rFonts w:ascii="Georgia"/><w:color w:val="112233"/><w:sz w:val="24"/></w:rPr><w:t>Uno </w:t></w:r>` +
		`<w:r><w:rPr><w:rFonts w:ascii="Arial"/><w:color w:val="445566"/><w:sz w:val="22"/></w:rPr><w:t>Dos</w:t></w:r>` +
		`</w:p>` +
		`<w:p><w:r><w:rPr><w:color w:val="0000FF"/></w:rPr><w:t xml:space="preserve"> </w:t></w:r><w:r><w:rPr><w:color w:val="112233"/></w:rPr><w:t>código</w:t></w:r></w:p>` +
		`<w:p><w:r><w:rPr><w:color w:val="333333"/></w:rPr><w:t>` + "\u00a0" + `</w:t></w:r></w:p>` +
		`<w:p><w:r><w:rPr><w:color w:val="000000"/></w:rPr><w:t>` + "\u3000\u3000" + `</w:t></w:r><w:r><w:rPr><w:color w:val="000000"/><w:b/></w:rPr><w:t>默示</w:t></w:r></w:p>` +
		`<w:p><w:del w:id="1" w:author="Ana" w:date="2026-05-01T10:00:00Z"><w:r><w:br/></w:r></w:del></w:p>` +
		`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr>`
	ida := importarDocx(t, cuerpo, map[string]string{"word/styles.xml": styles})
	noTiene(t, strings.Split(ida, "</p>")[0], "Times New Roman", "#5F5F5F", "10pt")
	raw, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", ida), FormatOOXML)
	if err != nil {
		t.Fatalf("exportar: %v", err)
	}
	res, err := Import("vuelta.docx", raw)
	if err != nil {
		t.Fatalf("reimportar: %v", err)
	}
	if res.Content != ida {
		t.Errorf("ida y vuelta difieren:\nida:    %s\nvuelta: %s", ida, res.Content)
	}
}
