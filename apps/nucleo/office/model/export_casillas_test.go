// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"strings"
	"testing"
)

// Una lista de casillas del editor (ul[data-checklist] > li[data-checked]) sale en cada
// formato con su marca, hecha o sin hacer, y no como una viñeta más.
const htmlCasillas = `<p>Antes</p><ul data-checklist=""><li data-checked="1">hecho</li>` +
	`<li data-checked="0">pendiente</li></ul><ul><li>viñeta</li></ul>`

func TestLasCasillasEnElModeloRico(t *testing.T) {
	var got []string
	for _, b := range parseHTMLBlocks(htmlCasillas) {
		if b.para != nil {
			got = append(got, b.para.list+"|"+b.para.casilla+"|"+b.para.vineta+"|"+b.para.plain())
		}
	}
	want := []string{"||" + "|Antes", "bullet|1|☑|hecho", "bullet|0|☐|pendiente", "bullet|||viñeta"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("bloques:\n%s\nquiero:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	p := parseHTMLBlocks(htmlCasillas)[2].para.conCasillaEnTexto("□")
	if p.list != "" || p.plain() != "□ pendiente" {
		t.Errorf("con la marca escrita: lista %q, texto %q", p.list, p.plain())
	}
}

func TestLasCasillasAlExportar(t *testing.T) {
	s := stateWithContent(t, "doc:html", htmlCasillas)

	md, _, _, err := Export(KindDocument, "", s, FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "- [x] hecho\n\n- [ ] pendiente\n\nviñeta") {
		t.Errorf("Markdown sin las tareas de GFM:\n%s", md)
	}

	h, _, _, err := Export(KindDocument, "", s, FormatHTML)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(h), `ul[data-checklist]>li[data-checked="1"]::before{content:"\2611"}`) ||
		!strings.Contains(string(h), `<li data-checked="0">pendiente</li>`) {
		t.Errorf("HTML sin la marca de las casillas:\n%s", h)
	}
	// Sin casillas, la cabecera no cambia.
	h, _, _, _ = Export(KindDocument, "", stateWithContent(t, "doc:html", "<p>x</p>"), FormatHTML)
	if strings.Contains(string(h), "<style>") {
		t.Errorf("un documento sin casillas lleva la hoja de las casillas:\n%s", h)
	}

	docx, _, _, err := Export(KindDocument, "", s, FormatOOXML)
	if err != nil {
		t.Fatal(err)
	}
	num := unzipPart(t, docx, "word/numbering.xml")
	for _, m := range []string{`<w:lvlText w:val="☑"/>`, `<w:lvlText w:val="☐"/>`} {
		if !strings.Contains(num, m) {
			t.Errorf("numbering.xml sin la viñeta %s", m)
		}
	}

	odt, _, _, err := Export(KindDocument, "", s, FormatODF)
	if err != nil {
		t.Fatal(err)
	}
	content := unzipPart(t, odt, "content.xml")
	if !strings.Contains(content, "☑ hecho") || !strings.Contains(content, "☐ pendiente") {
		t.Errorf("content.xml sin las casillas escritas:\n%s", content)
	}
	if strings.Count(content, "<text:list ") != 1 {
		t.Errorf("en ODT solo la viñeta es lista; las casillas van con su marca:\n%s", content)
	}

	// En una celda, ODT tampoco tiene lista: la casilla va escrita.
	odt, _, _, err = Export(KindDocument, "", stateWithContent(t, "doc:html",
		`<table><tr><td><ul data-checklist=""><li data-checked="1">en celda</li></ul></td></tr></table>`), FormatODF)
	if err != nil {
		t.Fatal(err)
	}
	if content := unzipPart(t, odt, "content.xml"); !strings.Contains(content, "☑ en celda") {
		t.Errorf("la casilla de una celda sale sin marca en ODT:\n%s", content)
	}

	pdf, _, _, err := Export(KindDocument, "", s, FormatPDF)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pdf), "[x] hecho") || !strings.Contains(string(pdf), "[ ] pendiente") {
		t.Errorf("el PDF simple no lleva la marca de las casillas")
	}
	if _, _, err := ExportPDFAccesible(KindDocument, "", "es", s); err != nil {
		t.Fatalf("PDF accesible: %v", err)
	}
}
