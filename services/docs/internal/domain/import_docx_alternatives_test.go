// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package domain

import (
	"encoding/xml"
	"os"
	"strings"
	"testing"
)

func TestDOCXAlternateImageSelectsOneRepresentation(t *testing.T) {
	for _, first := range []string{"image", "missing"} {
		t.Run(first, func(t *testing.T) {
			body := `<r><AlternateContent><Choice><drawing><blip embed="` + first + `"/><extent cx="914400" cy="457200"/></drawing></Choice><Fallback><pict><imagedata id="image"/></pict></Fallback></AlternateContent><t>Después</t></r>`
			d := docxDec{dec: xml.NewDecoder(strings.NewReader(body)), rels: map[string]string{"image": "media/a.png"}, files: map[string][]byte{"word/media/a.png": pngBytes(t)}}
			if _, err := d.dec.Token(); err != nil {
				t.Fatal(err)
			}
			runs := d.readRun("")
			var images int
			for _, run := range runs {
				if strings.Contains(run.raw, "<img") {
					images++
				}
			}
			if images != 1 || runs[len(runs)-1].text != "Después" {
				t.Fatalf("runs: %+v", runs)
			}
		})
	}
}

func TestDOCXLibreOfficeCorpusKeepsAlternateImage(t *testing.T) {
	data, err := os.ReadFile("testdata/fidelidad/libreoffice/documento.docx")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Import("documento.docx", data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(result.Content, "<img") != 1 {
		t.Fatal("imagen omitida o duplicada")
	}
}
