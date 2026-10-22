// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ia

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
	"time"
)

// Hoja es una pestaña del libro.
type Hoja struct {
	Nombre string
	Tabla  Tabla
}

// EscribirXLSX escribe un libro de Excel (SpreadsheetML) con una hoja por tabla.
//
// Lo justo para que el fichero SIRVA en Excel, no solo que se abra: cabecera en
// negrita y fija, números como números, importes con dos decimales y fechas como
// fechas de verdad —se pueden sumar, filtrar y ordenar—. El texto va como cadena
// en línea, que Excel no evalúa: un cliente llamado «=HYPERLINK(…)» no es una
// fórmula aquí, al revés que en un CSV.
//
// No es el exportador de Sytue Docs, y a propósito: aquel reproduce una hoja
// editada (fórmulas, formatos por celda, su gemelo ODS); esto vuelca tablas.
func EscribirXLSX(w io.Writer, hojas []Hoja) error {
	zw := zip.NewWriter(w)
	var libro, relaciones, tipos strings.Builder
	usados := map[string]bool{}
	for i, h := range hojas {
		n := strconv.Itoa(i + 1)
		libro.WriteString(`<sheet name="` + escapar(nombreDeHoja(h.Nombre, usados)) + `" sheetId="` + n + `" r:id="rId` + n + `"/>`)
		relaciones.WriteString(`<Relationship Id="rId` + n + `" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet` + n + `.xml"/>`)
		tipos.WriteString(`<Override PartName="/xl/worksheets/sheet` + n + `.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`)
		if err := parte(zw, "xl/worksheets/sheet"+n+".xml", hoja(h.Tabla)); err != nil {
			return err
		}
	}
	estilos := "rId" + strconv.Itoa(len(hojas)+1)
	partes := [][2]string{
		{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
			`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
			tipos.String() + `</Types>`},
		{"_rels/.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<sheets>` + libro.String() + `</sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + relaciones.String() +
			`<Relationship Id="` + estilos + `" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`},
		// Estilos: 0 normal, 1 cabecera en negrita, 2 importe (#,##0.00), 3 fecha.
		{"xl/styles.xml", `<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
			`<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>` +
			`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
			`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
			`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
			`<cellXfs count="4"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>` +
			`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/>` +
			`<xf numFmtId="4" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
			`<xf numFmtId="14" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/></cellXfs></styleSheet>`},
	}
	for _, p := range partes {
		if err := parte(zw, p[0], p[1]); err != nil {
			return err
		}
	}
	return zw.Close()
}

func parte(zw *zip.Writer, nombre, cuerpo string) error {
	f, err := zw.Create(nombre)
	if err == nil {
		_, err = io.WriteString(f, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\n"+cuerpo)
	}
	return err
}

func hoja(t Tabla) string {
	var b strings.Builder
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews><sheetData>`)
	b.WriteString(`<row r="1">`)
	for i, c := range t.Columnas {
		b.WriteString(`<c r="` + columna(i) + `1" t="inlineStr" s="1"><is><t>` + escapar(c.Etiqueta) + `</t></is></c>`)
	}
	b.WriteString(`</row>`)
	for f, fila := range t.Filas {
		r := strconv.Itoa(f + 2)
		b.WriteString(`<row r="` + r + `">`)
		for i, v := range fila {
			tipo := ""
			if i < len(t.Columnas) {
				tipo = t.Columnas[i].Tipo
			}
			b.WriteString(celdaXML(columna(i)+r, tipo, v))
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

// excelCero es el día 0 de las fechas de Excel (con su 29 de febrero de 1900).
var excelCero = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

func celdaXML(ref, tipo string, v any) string {
	numero := func(s, estilo string) string { return `<c r="` + ref + `"` + estilo + `><v>` + s + `</v></c>` }
	switch x := v.(type) {
	case nil:
		return ""
	case json.Number:
		if tipo == TipoDinero {
			return numero(x.String(), ` s="2"`)
		}
		return numero(x.String(), "")
	case float64:
		return numero(strconv.FormatFloat(x, 'f', -1, 64), "")
	case bool:
		if x {
			return `<c r="` + ref + `" t="b"><v>1</v></c>`
		}
		return `<c r="` + ref + `" t="b"><v>0</v></c>`
	case string:
		if tipo == TipoFecha {
			if t, err := time.Parse(time.DateOnly, x); err == nil {
				return numero(strconv.Itoa(int(t.Sub(excelCero).Hours()/24)), ` s="3"`)
			}
		}
		return `<c r="` + ref + `" t="inlineStr"><is><t xml:space="preserve">` + escapar(x) + `</t></is></c>`
	}
	return ""
}

// columna es la letra de la columna i (0 → A, 26 → AA).
func columna(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('A'+(i-1)%26)) + s
	}
	return s
}

// nombreDeHoja cumple lo que Excel exige: 31 caracteres, sin []:*?/\ y sin repetir.
func nombreDeHoja(nombre string, usados map[string]bool) string {
	limpio := recortar(strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) {
			return '-'
		}
		return r
	}, nombre), 31)
	if limpio == "" {
		limpio = "Hoja"
	}
	final := limpio
	for n := 2; usados[strings.ToLower(final)]; n++ {
		sufijo := " (" + strconv.Itoa(n) + ")"
		final = recortar(limpio, 31-len(sufijo)) + sufijo
	}
	usados[strings.ToLower(final)] = true
	return final
}

// escapar deja el texto válido en XML; lo que XML 1.0 no admite sale como U+FFFD.
func escapar(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
