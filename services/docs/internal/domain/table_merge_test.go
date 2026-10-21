// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

const tablaCombinada = `<table><tbody>` +
	`<tr><td colspan="2" rowspan="2">Bloque</td><td>C</td></tr>` +
	`<tr><td>D</td></tr>` +
	`<tr><td>E</td><td>F</td><td rowspan="2">G</td></tr>` +
	`<tr><td colspan="2">H</td></tr>` +
	`</tbody></table>`

func tablaIdaYVuelta(t *testing.T, html string, formato BlobFormat) string {
	t.Helper()
	data, _, ext, err := Export(KindDocument, "", stateWithContent(t, "doc:html", html), formato)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Import("vuelta."+ext, data)
	if err != nil {
		t.Fatal(err)
	}
	return res.Content
}

func TestTablaCombinadaIdaYVuelta(t *testing.T) {
	for nombre, formato := range map[string]BlobFormat{"docx": FormatOOXML, "odt": FormatODF} {
		got := tablaIdaYVuelta(t, tablaCombinada, formato)
		assertBlocksEqual(t, nombre, `<table><tbody>`+
			`<tr><td colspan="2" rowspan="2"><p>Bloque</p></td><td><p>C</p></td></tr>`+
			`<tr><td><p>D</p></td></tr>`+
			`<tr><td><p>E</p></td><td><p>F</p></td><td rowspan="2"><p>G</p></td></tr>`+
			`<tr><td colspan="2"><p>H</p></td></tr>`+
			`</tbody></table>`, got)
	}
}

// Word escribe la combinación vertical con vMerge y la horizontal con gridSpan, sin celdas
// de relleno, y las continuaciones pueden llevar su propio gridSpan.
func TestTablaCombinadaDeWord(t *testing.T) {
	data, err := zipPackage(map[string]string{
		"word/document.xml": `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:tbl><w:tblGrid/>` +
			`<w:tr><w:tc><w:tcPr><w:gridSpan w:val="2"/><w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>A</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>B</w:t></w:r></w:p></w:tc></w:tr>` +
			`<w:tr><w:tc><w:tcPr><w:gridSpan w:val="2"/><w:vMerge/></w:tcPr><w:p/></w:tc><w:tc><w:p><w:r><w:t>C</w:t></w:r></w:p></w:tc></w:tr>` +
			`<w:tr><w:tc><w:tcPr><w:vMerge/></w:tcPr><w:p/></w:tc></w:tr>` + // continúa a las de arriba; su columna 2 queda vacía
			`</w:tbl></w:body></w:document>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := Import("w.docx", data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, `<td colspan="2" rowspan="3">A</td><td>B</td></tr><tr><td>C</td></tr><tr><td></td></tr>`) {
		t.Fatalf("vMerge/gridSpan mal leídos: %s", res.Content)
	}
}

// Una combinación hostil (rowspan/colspan enormes, solapadas) no reserva memoria ni
// rompe la rejilla: sale rectangular y acotada.
func TestTablaCombinadaHostil(t *testing.T) {
	html := `<table><tr><td colspan="999999999999999999999" rowspan="4000000000">X</td><td rowspan="5">Y</td></tr>` +
		`<tr><td colspan="3">Z</td><td rowspan="9">W</td></tr><tr><td>V</td></tr></table>`
	out := tablaIdaYVuelta(t, html, FormatOOXML)
	if !strings.Contains(out, "<table>") {
		t.Fatalf("la tabla se perdió: %s", out)
	}
	tbl := parseTableFromHTML(t, html)
	for r, row := range tbl.rows {
		if len(row) != tbl.cols {
			t.Fatalf("fila %d no rectangular: %d != %d", r, len(row), tbl.cols)
		}
	}
	if tbl.cols > maxTableCols {
		t.Fatalf("ancho sin acotar: %d", tbl.cols)
	}
}

func parseTableFromHTML(t *testing.T, s string) *xtable {
	t.Helper()
	for _, b := range parseHTMLBlocks(s) {
		if b.table != nil {
			return b.table
		}
	}
	t.Fatal("sin tabla")
	return nil
}
