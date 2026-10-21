// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
	"time"
)

// El encabezado y el pie del editor (div[data-doc-header|footer], con el número de página
// en un span[data-page-field]) viajan a su sitio en OOXML y ODF —no al cuerpo— y vuelven
// con el mismo HTML, también con «primera página distinta».
func TestOfficeRegionsRoundTrip(t *testing.T) {
	h := `<div data-doc-header data-first-page-different style="` + docHeaderStyle + `">Cooperativa <strong>La Vega</strong></div>` +
		`<p>Cuerpo del informe</p>` +
		`<div data-doc-footer style="` + docFooterStyle + `">Pie del informe <span data-page-field style="white-space:nowrap">Página 1 de 1</span></div>`
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
				ext, cuerpo := "odt", string(files["content.xml"])
				if format == FormatOOXML {
					ext, cuerpo = "docx", string(files["word/document.xml"])
					for _, parte := range []string{"word/header1.xml", "word/footer1.xml"} {
						if len(files[parte]) == 0 {
							t.Fatalf("falta %s", parte)
						}
					}
					debeContener(t, "campos de Word", string(files["word/footer1.xml"]), `w:instr=" PAGE "`, `w:instr=" NUMPAGES "`)
					debeContener(t, "primera página", cuerpo, "<w:titlePg/>")
				} else {
					debeContener(t, "campos de ODF", string(files["styles.xml"]), "<text:page-number", "<text:page-count", "<style:header-first>")
				}
				if strings.Contains(cuerpo, "Cooperativa") || strings.Contains(cuerpo, "Pie del informe") {
					t.Fatalf("el encabezado o el pie se quedaron en el cuerpo: %s", cuerpo)
				}
				res, err := Import("regiones."+ext, data)
				if err != nil {
					t.Fatal(err)
				}
				debeContener(t, "ida y vuelta", res.Content,
					`<div data-doc-header data-first-page-different style=`, `Cooperativa <strong>La Vega</strong></div>`,
					`<p>Cuerpo del informe</p>`, `<div data-doc-footer style=`,
					`Pie del informe <span data-page-field style="white-space:nowrap">Página 1 de 1</span></div>`)
				state = stateWithContent(t, "doc:html", res.Content)
			}
		})
	}
	// El PDF y el texto plano siguen viendo el encabezado y el pie en el folio.
	var plain strings.Builder
	for _, b := range parseHTMLBlocks(h) {
		if b.para != nil {
			plain.WriteString(b.para.plain())
		}
	}
	debeContener(t, "PDF", plain.String(), "Cooperativa", "Pie del informe")
}

func debeContener(t *testing.T, donde, hay string, quiere ...string) {
	t.Helper()
	for _, q := range quiere {
		mustContain(t, donde, hay, q)
	}
}

// El control de cambios (ins/del[data-track] con autor y sello) viaja como revisión de
// Word y de LibreOffice, no como subrayado y tachado, y vuelve igual.
func TestOfficeTrackedChangesRoundTrip(t *testing.T) {
	h := `<p>Se dejó <ins data-track data-author="Lucía Prieto" data-author-id="u1" data-ts="1790000000000" style="--sytue-track:var(--sytue-avatar-2)">un párrafo <strong>nuevo</strong></ins>` +
		` y se quitó <del data-track data-author="Lucía Prieto" data-author-id="u1" data-ts="1790000300000" style="--sytue-track:var(--sytue-avatar-2)">el resto</del>.</p>`
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
				ext, cuerpo := "odt", string(files["content.xml"])
				if format == FormatOOXML {
					ext, cuerpo = "docx", string(files["word/document.xml"])
					debeContener(t, "revisiones de Word", cuerpo, `<w:ins `, `w:author="Lucía Prieto"`, `<w:delText xml:space="preserve">el resto</w:delText>`, `w:date="2026-09-21T`)
				} else {
					debeContener(t, "revisiones de ODF", cuerpo, `<text:tracked-changes>`, `<text:change-start `, `<text:change-end `, `<dc:creator>Lucía Prieto</dc:creator>`, `<text:deletion>`)
				}
				if strings.Contains(cuerpo, "<w:u ") || strings.Contains(cuerpo, "<w:strike") {
					t.Fatalf("la revisión se exportó además como subrayado o tachado: %s", cuerpo)
				}
				res, err := Import("revisiones."+ext, data)
				if err != nil {
					t.Fatal(err)
				}
				debeContener(t, "ida y vuelta", res.Content,
					`<ins data-track data-author="Lucía Prieto" data-author-id="Lucía Prieto" data-ts="1790000000000" style="--sytue-track:var(--sytue-avatar-`,
					`un párrafo <strong>nuevo</strong></ins>`,
					`<del data-track data-author="Lucía Prieto" data-author-id="Lucía Prieto" data-ts="1790000300000"`, `>el resto</del>.`)
				state = stateWithContent(t, "doc:html", res.Content)
			}
		})
	}
	// PDF y texto plano siguen tratándolas como subrayado y tachado.
	if !strings.Contains(parseHTMLDoc(h, false).blocks[0].para.plain(), "un párrafo nuevo") {
		t.Fatal("el PDF perdió la inserción")
	}
}

// La cita (blockquote) vuelve como cita tras exportar a DOCX y a ODT, y el estilo «Quotations»
// de LibreOffice se lee como cita.
func TestOfficeQuoteRoundTrip(t *testing.T) {
	for _, format := range []BlobFormat{FormatOOXML, FormatODF} {
		data, _, ext, err := Export(KindDocument, "", stateWithContent(t, "doc:html", `<p>Antes</p><blockquote>La tierra no se hereda</blockquote>`), format)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Import("cita."+ext, data)
		if err != nil {
			t.Fatal(err)
		}
		debeContener(t, string(format), res.Content, `<blockquote>La tierra no se hereda</blockquote>`)
	}
}

// Los comentarios van en su parte propia (comments.xml + commentsExtended.xml, o
// office:annotation) con su ancla, su hilo y su «resuelto», y vuelven a ImportWithComments
// con la autoría original delante del cuerpo. Import a secas no deja marks sin comentario.
func TestOfficeCommentsRoundTrip(t *testing.T) {
	h := `<p>Total <mark data-comment-id="m1">1200</mark> euros</p><table><tr><td><mark data-comment-id="m2">celda</mark></td></tr></table>`
	raiz := "c1"
	cs := []Comment{
		{ID: "c1", AuthorName: "Lucía Prieto", Body: "Revisar esta cifra\ncon tesorería", Anchor: []byte(`{"mark":"m1","text":"1200"}`), CreatedAt: time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)},
		{ID: "c2", ParentID: &raiz, AuthorName: "Marta Gil", Body: "Hecho", Resolved: true, CreatedAt: time.Date(2026, 9, 30, 11, 0, 0, 0, time.UTC)},
		{ID: "c3", AuthorName: "Ana", Body: "General", Anchor: []byte(`{"text":"suelto"}`), CreatedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)},
		{ID: "c4", AuthorName: "Ana", Body: "En la tabla", Anchor: []byte(`{"mark":"m2","text":"celda"}`), CreatedAt: time.Date(2026, 9, 30, 13, 0, 0, 0, time.UTC)},
	}
	for _, format := range []BlobFormat{FormatOOXML, FormatODF} {
		t.Run(string(format), func(t *testing.T) {
			data, _, ext, err := ExportWithComments(KindDocument, "", stateWithContent(t, "doc:html", h), format, cs)
			if err != nil {
				t.Fatal(err)
			}
			files, err := readZip(data)
			if err != nil {
				t.Fatal(err)
			}
			if format == FormatOOXML {
				debeContener(t, "comments.xml", string(files["word/comments.xml"]), `w:author="Lucía Prieto"`, `w:initials="LP"`, `con tesorería`, `w:date="2026-09-30T10:00:00Z"`)
				debeContener(t, "commentsExtended", string(files["word/commentsExtended.xml"]), `w15:paraIdParent=`, `w15:done="1"`)
				debeContener(t, "documento", string(files["word/document.xml"]), `<w:commentRangeStart w:id="0"/>`, `<w:commentReference w:id="1"/>`)
			} else {
				debeContener(t, "ODF", string(files["content.xml"]), `<office:annotation `, `<office:annotation-end `, `<dc:creator>Marta Gil</dc:creator>`, `loext:resolved="true"`)
			}
			// Sin pedir comentarios, el HTML no lleva marks que nadie vaya a guardar.
			plano, err := Import("c."+ext, data)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(plano.Content, "data-comment-id") || len(plano.Comments) != 0 {
				t.Fatalf("Import a secas dejó comentarios o anclas: %s", plano.Content)
			}
			res, err := ImportWithComments("c."+ext, data)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Comments) != 4 {
				t.Fatalf("comentarios: %d, quería 4: %+v", len(res.Comments), res.Comments)
			}
			por := map[string]ImportedComment{}
			for _, c := range res.Comments {
				por[c.Author+"|"+c.Body] = c
			}
			raizI := por["Lucía Prieto|Revisar esta cifra\ncon tesorería"]
			resp := por["Marta Gil|Hecho"]
			if raizI.Text != "Lucía Prieto, 2026-09-30: Revisar esta cifra\ncon tesorería" || !raizI.Anchored || raizI.Quote != "1200" {
				t.Fatalf("raíz mal: %+v", raizI)
			}
			if resp.ParentID != raizI.ID || !resp.Resolved || resp.Anchored {
				t.Fatalf("respuesta mal: %+v", resp)
			}
			if g := por["Ana|General"]; g.Anchored || g.Text != "Ana, 2026-09-30: General" {
				t.Fatalf("el comentario sin ancla salió anclado: %+v", g)
			}
			if tb := por["Ana|En la tabla"]; !tb.Anchored || tb.Quote != "celda" {
				t.Fatalf("el de la tabla perdió el ancla: %+v", tb)
			}
			// Un <mark> por comentario anclado, y ninguno suelto.
			if n := strings.Count(res.Content, "<mark data-comment-id="); n != 2 {
				t.Fatalf("marks: %d, quería 2: %s", n, res.Content)
			}
			for _, c := range res.Comments {
				if c.Anchored != strings.Contains(res.Content, `data-comment-id="`+c.ID+`"`) {
					t.Fatalf("ancla y HTML no coinciden para %+v: %s", c, res.Content)
				}
			}
		})
	}
}
