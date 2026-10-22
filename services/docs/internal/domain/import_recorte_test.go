// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strconv"
	"strings"
	"testing"
)

// Un .xlsx que no cabe entero en un documento se abre con las primeras N filas de cada hoja
// —la N más alta con la que cabe—, una hoja «Aviso» delante en el idioma de quien abre, y
// `Truncated`, que es lo que hace que no se ancle al fichero (open-from-drive.ts). Antes
// daba «no se pudo abrir» con «Formatos admitidos… máx. 25 MB», aunque el fichero pesara 3.
func TestLibroQueNoCabeSeAbreRecortado(t *testing.T) {
	for _, nombre := range []string{"grande.xlsx", "grande.ods"} {
		res, err := ImportLocalized(nombre, ficheroDeTexto(t, nombre, 40_000), false, "es")
		if err != nil {
			t.Fatalf("%s: %v", nombre, err)
		}
		if !res.Truncated || res.Kind != KindSpreadsheet {
			t.Fatalf("%s: quiero un libro recortado, tengo truncated=%v kind=%s", nombre, res.Truncated, res.Kind)
		}
		if tam := tamanoSembrado(res.Content); tam > MaxContenidoSembrado || tam < MaxContenidoSembrado*97/100 {
			t.Errorf("%s: lo sembrado ocupa %d: tiene que caber y quedarse junto al tope (la N más alta)", nombre, tam)
		}
		m := parseSheetModelJSON(t, res.Content)
		if len(m.Sheets) != 2 || m.Sheets[0].Name != "Aviso" || m.Active != 0 {
			t.Fatalf("%s: quiero la hoja Aviso delante y activa, tengo %d hojas: %q", nombre, len(m.Sheets), m.Sheets[0].Name)
		}
		datos := m.Sheets[1]
		n := 0
		for ref := range datos.Cells {
			if r, _, ok := ParseA1(ref); ok {
				n = max(n, r)
			}
		}
		if n < 1000 || n >= 40_000 || len(datos.Cells) != n {
			t.Errorf("%s: %d filas y %d celdas: quiero las primeras N enteras", nombre, n, len(datos.Cells))
		}
		aviso := m.Sheets[0].Cells
		if !strings.Contains(aviso["A2"], "primeras "+strconv.Itoa(n)+" filas") || aviso["A3"] != "Datos" {
			t.Errorf("%s: el aviso no dice cuántas filas ni qué hoja: %q %q", nombre, aviso["A2"], aviso["A3"])
		}
	}
}

// Las hojas que caben enteras no se recortan ni salen en el aviso; y un libro que solo no
// cabía para el presupuesto, que carga por arriba, se abre entero y sin aviso.
func TestRecorteSoloEnLasHojasQueNoCaben(t *testing.T) {
	grande := ficheroDeTexto(t, "grande.xlsx", 40_000)
	files, err := readZip(grande)
	if err != nil {
		t.Fatal(err)
	}
	libro := map[string]string{}
	for k, v := range files {
		libro[k] = string(v)
	}
	libro["xl/workbook.xml"] = `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>` +
		`<sheet name="Corta" r:id="rId2"/><sheet name="Datos" r:id="rId1"/></sheets></workbook>`
	libro["xl/_rels/workbook.xml.rels"] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Target="worksheets/sheet2.xml"/></Relationships>`
	libro["xl/worksheets/sheet2.xml"] = `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>` +
		`<row r="1"><c r="A1" t="inlineStr"><is><t>uno</t></is></c></row><row r="2"><c r="A2"><v>2</v></c></row></sheetData></worksheet>`
	res, err := Import("dos.xlsx", zipStore(t, libro))
	if err != nil || !res.Truncated {
		t.Fatalf("quiero el libro recortado: %v", err)
	}
	m := parseSheetModelJSON(t, res.Content)
	if len(m.Sheets) != 3 || m.Sheets[1].Name != "Corta" || len(m.Sheets[1].Cells) != 2 {
		t.Fatalf("la hoja corta, entera: %+v", m.Sheets[1])
	}
	if a := m.Sheets[0].Cells; a["A3"] != "Datos" || a["A4"] != "" {
		t.Errorf("el aviso solo nombra la hoja recortada: %q %q", a["A3"], a["A4"])
	}
	// Uno que cabe entero no lleva aviso ni se marca.
	res, err = Import("pequeno.xlsx", ficheroDeTexto(t, "grande.xlsx", 100))
	if err != nil || res.Truncated || strings.Contains(res.Content, `"Aviso"`) {
		t.Errorf("un libro que cabe: truncated=%v err=%v", res != nil && res.Truncated, err)
	}
}

// Un .docx o un .pptx que no cabe entero también se abre recortado como copia aparte
// (`Truncated`), con el aviso delante en el idioma de quien lo abre y dentro del tope.
func TestDocxYPptxQueNoCabenSeAbrenRecortados(t *testing.T) {
	// .docx grande: aviso delante en el idioma pedido, primeros párrafos presentes y sin los últimos.
	docRes, err := ImportLocalized("grande.docx", ficheroDeTexto(t, "grande.docx", 40_000), false, "es")
	if err != nil {
		t.Fatalf("grande.docx: %v", err)
	}
	if !docRes.Truncated || docRes.Kind != KindDocument {
		t.Fatalf("grande.docx: quiero documento recortado, tengo truncated=%v kind=%s", docRes.Truncated, docRes.Kind)
	}
	if tam := tamanoSembrado(docRes.Content); tam > MaxContenidoSembrado || tam < MaxContenidoSembrado*95/100 {
		t.Errorf("grande.docx: lo sembrado ocupa %d: tiene que caber y quedarse junto al tope", tam)
	}
	if !strings.HasPrefix(docRes.Content, "<blockquote><p><strong>Conversión parcial</strong></p>") ||
		!strings.Contains(docRes.Content, "Es una copia aparte") ||
		!strings.Contains(docRes.Content, "0 Lorem ipsum") ||
		strings.Contains(docRes.Content, "39999 Lorem ipsum") {
		t.Errorf("grande.docx: falta el aviso o no recorta por el final")
	}

	// .pptx grande (una diapositiva con texto que desborda): diapositiva de aviso delante y la primera recortada.
	pptRes, err := ImportLocalized("grande.pptx", ficheroDeTexto(t, "grande.pptx", 40_000), false, "fr")
	if err != nil {
		t.Fatalf("grande.pptx: %v", err)
	}
	if !pptRes.Truncated || pptRes.Kind != KindPresentation {
		t.Fatalf("grande.pptx: quiero presentación recortada, tengo truncated=%v kind=%s", pptRes.Truncated, pptRes.Kind)
	}
	if tam := tamanoSembrado(pptRes.Content); tam > MaxContenidoSembrado || tam < MaxContenidoSembrado*90/100 {
		t.Errorf("grande.pptx: lo sembrado ocupa %d: tiene que caber y aprovechar el tope", tam)
	}
	sm := parseSlidesModelJSON(t, pptRes.Content)
	if len(sm.Slides) != 2 || len(sm.Slides[0].Boxes) == 0 ||
		!strings.Contains(sm.Slides[0].Boxes[0].HTML, "Conversion partielle") ||
		!strings.Contains(sm.Slides[1].Boxes[0].HTML, "0 Lorem ipsum") ||
		strings.Contains(sm.Slides[1].Boxes[0].HTML, "39999 Lorem ipsum") {
		t.Errorf("grande.pptx: quiero diapositiva de aviso delante y la primera recortada: %d diapositivas", len(sm.Slides))
	}
}
