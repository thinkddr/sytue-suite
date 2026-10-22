// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"testing"
)

// Un libro cuya parte principal no está en xl/workbook.xml —en la raíz del paquete, con otro
// nombre o con otra caja— es tan válido como cualquiera (lo que manda es la relación
// officeDocument de _rels/.rels, y OPC no distingue mayúsculas en los nombres de parte), y no
// se abría: «falta xl/workbook.xml». Las cadenas y los estilos van donde dicen las relaciones
// del libro.
func TestLibroConLaParteEnOtroSitio(t *testing.T) {
	rel := func(id, tipo, destino string) string {
		return `<Relationship Id="` + id + `" Type="` + tipoR + tipo + `" Target="` + destino + `"/>`
	}
	rels := func(r ...string) string {
		out := `<Relationships ` + nsRels + `>`
		for _, x := range r {
			out += x
		}
		return out + `</Relationships>`
	}
	libro := `<workbook ` + nsHoja + `><sheets><sheet name="Datos" sheetId="1" r:id="h1"/></sheets></workbook>`
	hoja := `<worksheet ` + nsHoja + `><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" s="1"><v>7</v></c></row></sheetData></worksheet>`
	cadenas := `<sst ` + nsHoja + ` count="1" uniqueCount="1"><si><t>hola</t></si></sst>`
	estilos := `<styleSheet ` + nsHoja + `><fonts count="1"><font/></fonts>` +
		`<fills count="3"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill>` +
		`<fill><patternFill patternType="solid"><fgColor rgb="FFFFC000"/></patternFill></fill></fills>` +
		`<borders count="1"><border/></borders><cellXfs count="2"><xf/><xf fillId="2" applyFill="1"/></cellXfs></styleSheet>`
	casos := map[string]map[string]string{
		"en la raíz, como xlnt y calamine": {
			"_rels/.rels":             rels(rel("r", "officeDocument", "workbook.xml")),
			"workbook.xml":            libro,
			"_rels/workbook.xml.rels": rels(rel("h1", "worksheet", "sheet1.xml"), rel("s", "sharedStrings", "strings.xml"), rel("e", "styles", "stylesheet.xml")),
			"sheet1.xml":              hoja,
			"strings.xml":             cadenas,
			"stylesheet.xml":          estilos,
		},
		"con destino absoluto y otro nombre, como el Open XML SDK": {
			"_rels/.rels":                 rels(rel("r", "officeDocument", "/xl/workbook2.xml")),
			"xl/workbook2.xml":            libro,
			"xl/_rels/workbook2.xml.rels": rels(rel("h1", "worksheet", "/xl/worksheets/sheet1.xml"), rel("s", "sharedStrings", "sharedStrings.xml"), rel("e", "styles", "styles.xml")),
			"xl/worksheets/sheet1.xml":    hoja,
			"xl/sharedStrings.xml":        cadenas,
			"xl/styles.xml":               estilos,
		},
		"con otra caja": {
			"_rels/.rels":                rels(rel("r", "officeDocument", "xl/workbook.xml")),
			"xl/WorkBook.xml":            libro,
			"xl/_rels/WorkBook.xml.rels": rels(rel("h1", "worksheet", "worksheets/sheet1.xml"), rel("s", "sharedStrings", "sharedStrings.xml"), rel("e", "styles", "styles.xml")),
			"xl/WorkSheets/SHEET1.xml":   hoja,
			"xl/SharedStrings.xml":       cadenas,
			"xl/STYLES.XML":              estilos,
		},
	}
	for nombre, files := range casos {
		res, err := Import("libro.xlsx", zipStore(t, files))
		if err != nil {
			t.Errorf("%s: %v", nombre, err)
			continue
		}
		m := parseSheetModelJSON(t, res.Content)
		if h := m.Sheets[0]; h.Name != "Datos" || h.Cells["A1"] != "hola" || h.Cells["B1"] != "7" || h.Styles["B1"].Fill == "" {
			t.Errorf("%s: hoja %q, A1 %q, B1 %q con relleno %q", nombre, h.Name, h.Cells["A1"], h.Cells["B1"], h.Styles["B1"].Fill)
		}
	}
	// Sin xl/workbook.xml ni _rels/.rels pero con hojas en xl/_rels/workbook.xml.rels y
	// xl/worksheets/sheet1.xml, como missing-bits.xlsx de exceljs: se abre con sus hojas.
	sinLibroConHoja := map[string]string{
		"xl/_rels/workbook.xml.rels": rels(rel("rId4", "worksheet", "worksheets/sheet1.xml")),
		"xl/worksheets/sheet1.xml":   `<worksheet ` + nsHoja + `><sheetData><row r="2"><c r="N2"><v>42</v></c></row></sheetData></worksheet>`,
	}
	if res, err := Import("missing-bits.xlsx", zipStore(t, sinLibroConHoja)); err != nil {
		t.Errorf("sin workbook.xml pero con hoja (missing-bits): %v", err)
	} else if m := parseSheetModelJSON(t, res.Content); len(m.Sheets) != 1 || m.Sheets[0].Name != "Hoja1" || m.Sheets[0].Cells["N2"] != "42" {
		t.Errorf("missing-bits: hojas = %+v", m.Sheets)
	}
	// Sin parte principal ni hojas no hay libro (como EmptyZipFile.xlsx de ExcelDataReader,
	// que solo lleva test.txt).
	vacio := map[string]string{"test.txt": "no es un libro"}
	if _, err := Import("vacio.xlsx", zipStore(t, vacio)); !errors.Is(err, ErrInvalidFormat) {
		t.Errorf("un ZIP sin libro: err = %v, quiero formato inválido", err)
	}
}
