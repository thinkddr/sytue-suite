// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

func TestOfficeNotesSemanticRoundTrip(t *testing.T) {
	h := `<p>Antes` + footnoteMarker(7) + `Después</p><table><tr><td>Celda` + footnoteMarkerClass(9, "endnote") + `</td></tr></table>` +
		`<section data-footnotes><ol><li id="fn-7">Nota <strong>importante</strong> &amp; segura<br>Segundo párrafo</li><li id="fn-9">Final <a href="https://sytue.eu">enlace</a></li></ol></section>`
	for _, format := range []BlobFormat{FormatOOXML, FormatODF} {
		t.Run(string(format), func(t *testing.T) {
			state := stateWithContent(t, "doc:html", h)
			for vuelta := 0; vuelta < 2; vuelta++ {
				data, _, _, err := Export(KindDocument, "", state, format)
				if err != nil {
					t.Fatal(err)
				}
				files, err := readZip(data)
				if err != nil {
					t.Fatal(err)
				}
				ext := "odt"
				if format == FormatOOXML {
					ext = "docx"
					for _, name := range []string{"word/footnotes.xml", "word/endnotes.xml", "word/_rels/endnotes.xml.rels"} {
						if len(files[name]) == 0 {
							t.Fatalf("falta la parte semántica %s", name)
						}
					}
					if strings.Contains(string(files["word/document.xml"]), "importante") {
						t.Fatal("se duplicó el cuerpo de la nota en el documento")
					}
				} else {
					mustContain(t, "nota ODF", string(files["content.xml"]), `text:note-class="endnote"`)
				}
				res, err := Import("notas."+ext, data)
				if err != nil {
					t.Fatal(err)
				}
				for _, want := range []string{"Antes", "Después", "Celda", `data-note-class="endnote"`, "<strong>importante</strong>", "Segundo párrafo", `href="https://sytue.eu"`} {
					mustContain(t, "notas ida y vuelta", res.Content, want)
				}
				if strings.Count(res.Content, "importante") != 1 || strings.Count(res.Content, "data-footnote-ref=") != 2 {
					t.Fatalf("se perdieron o duplicaron notas: %s", res.Content)
				}
				state = stateWithContent(t, "doc:html", res.Content)
			}
		})
	}
	var plain strings.Builder
	for _, block := range parseHTMLBlocks(h) {
		if block.para != nil {
			plain.WriteString(block.para.plain())
		}
	}
	mustContain(t, "otros destinos conservan las notas", plain.String(), "importante")
}

func TestOfficeNotesBrokenReferencesKeepText(t *testing.T) {
	h := `<p>Texto` + footnoteMarker(3) + `</p><section data-footnotes><ol><li id="fn-2">Nota huérfana</li></ol></section>`
	for _, format := range []BlobFormat{FormatOOXML, FormatODF} {
		data, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", h), format)
		if err != nil {
			t.Fatal(err)
		}
		files, err := readZip(data)
		if err != nil {
			t.Fatal(err)
		}
		if len(files["word/footnotes.xml"]) != 0 {
			t.Fatal("se inventó una nota para una referencia rota")
		}
		var all strings.Builder
		for _, content := range files {
			all.Write(content)
		}
		mustContain(t, "nota huérfana", all.String(), "Nota huérfana")
	}
	// El parser de los otros destinos no consume las notas semánticas Office.
	blocks := parseHTMLBlocks(h)
	var plain strings.Builder
	for _, block := range blocks {
		if block.para != nil {
			plain.WriteString(block.para.plain())
		}
	}
	mustContain(t, "texto/PDF", plain.String(), "Nota huérfana")
}
