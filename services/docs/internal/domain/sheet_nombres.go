// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/xml"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Los NOMBRES DEFINIDOS del libro (`SheetModel.names` del editor, sheets/nombres.ts):
// «Ventas» en vez de «Hoja1!$B$2:$B$40». Sin ellos, un .xlsx con `=SUM(Ventas)` llegaba
// con la fórmula y sin el nombre, y la hoja se llenaba de #NAME?. Aquí se traducen a lo
// que escriben Excel (`<definedName>` en workbook.xml, con `localSheetId` si es de una
// hoja) y Calc (`table:named-range` / `table:named-expression`, dentro de la tabla si
// es de una hoja), y de vuelta.

// definedName es el `DefinedName` del editor. Sheet vacío = ámbito libro.
type definedName struct {
	Name    string `json:"name"`
	Ref     string `json:"ref"`
	Sheet   string `json:"sheet,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// xlsxDefinedName es un <definedName> de workbook.xml.
type xlsxDefinedName struct {
	Name    string `xml:"name,attr"`
	Local   *int   `xml:"localSheetId,attr"`
	Comment string `xml:"comment,attr"`
	Ref     string `xml:",chardata"`
}

// nombresDeXlsx convierte los <definedName> en nombres del editor. El localSheetId es
// el índice de la hoja en <sheets>. Fuera los que Excel se reserva (`_xlnm.`: área de
// impresión, títulos, la base del filtro…), que no son de la persona.
func nombresDeXlsx(defs []xlsxDefinedName, hojas []string) []definedName {
	var out []definedName
	for _, d := range defs {
		ref := strings.TrimPrefix(strings.TrimSpace(d.Ref), "=")
		if d.Name == "" || ref == "" || strings.HasPrefix(d.Name, "_xlnm.") {
			continue
		}
		n := definedName{Name: d.Name, Ref: ref, Comment: d.Comment}
		if d.Local != nil {
			if *d.Local < 0 || *d.Local >= len(hojas) {
				continue // de una hoja que no está
			}
			n.Sheet = hojas[*d.Local]
		}
		out = append(out, n)
	}
	return out
}

// nombresXlsx es el <definedNames> de workbook.xml ("" si no hay ninguno). Va entre
// <sheets> y <calcPr>, que es donde lo pide el esquema.
func nombresXlsx(names []definedName, hojas []string, extra string) string {
	var b strings.Builder
	b.WriteString(extra)
	for _, n := range names {
		local := ""
		if n.Sheet != "" {
			i := slices.Index(hojas, n.Sheet)
			if i < 0 {
				continue
			}
			local = ` localSheetId="` + strconv.Itoa(i) + `"`
		}
		b.WriteString(`<definedName name="` + xmlEscape(n.Name) + `"` + local)
		if n.Comment != "" {
			b.WriteString(` comment="` + xmlEscape(n.Comment) + `"`)
		}
		b.WriteString(`>` + xmlEscape(n.Ref) + `</definedName>`)
	}
	if b.Len() == 0 {
		return ""
	}
	return `<definedNames>` + b.String() + `</definedNames>`
}

// reRangoNombre es una definición que es SOLO un rango, con su hoja o sin ella.
var reRangoNombre = regexp.MustCompile(`^(?:('(?:[^']|'')*'|[A-Za-z_][A-Za-z0-9_.]*)!)?(\$?[A-Za-z]{1,3}\$?[0-9]+(?::\$?[A-Za-z]{1,3}\$?[0-9]+)?)$`)

// nombresODS es el <table:named-expressions> de los nombres de ámbito `hoja` ("" = los
// del libro); base es la hoja contra la que se leen sus refs sin hoja (la suya, o la
// primera para los del libro). Un rango va como
// `table:named-range` y lo demás como `table:named-expression`, que es como lo escribe
// Calc. Las direcciones van en absoluto: en Calc una relativa se mueve con la celda.
func nombresODS(names []definedName, hoja, base string) string {
	var b strings.Builder
	for _, n := range names {
		if n.Sheet != hoja {
			continue
		}
		celdaBase := `$` + hojaODS(base) + `.$A$1`
		if m := reRangoNombre.FindStringSubmatch(n.Ref); m != nil {
			if r, ok := leerRango(m[2]); ok {
				h := base
				if m[1] != "" {
					h = sinComillas(m[1])
				}
				dir := `$` + hojaODS(h) + `.` + celdaAbs(r.r1, r.c1)
				if r.c1 != r.c2 || r.r1 != r.r2 {
					dir += `:.` + celdaAbs(r.r2, r.c2)
				}
				b.WriteString(`<table:named-range table:name="` + xmlEscape(n.Name) + `" table:base-cell-address="` +
					xmlEscape(celdaBase) + `" table:cell-range-address="` + xmlEscape(dir) + `"/>`)
				continue
			}
		}
		// Sin el «of:=» de table:formula: Calc no lo escribe aquí y, si lo encuentra, lo
		// arrastra tal cual al guardar en .xlsx (medido con LibreOffice 7.4).
		expr := a1AODF(n.Ref, true)
		b.WriteString(`<table:named-expression table:name="` + xmlEscape(n.Name) + `" table:base-cell-address="` +
			xmlEscape(celdaBase) + `" table:expression="` + xmlEscape(expr) + `"/>`)
	}
	if b.Len() == 0 {
		return ""
	}
	return `<table:named-expressions>` + b.String() + `</table:named-expressions>`
}

func celdaAbs(fila, col int) string { return `$` + ColumnToLetters(col) + `$` + strconv.Itoa(fila) }

// hojaODS es el nombre de hoja de una dirección de ODF, siempre entre comillas (Calc las
// acepta siempre y así no hay que decidir cuándo hacen falta).
func hojaODS(h string) string { return `'` + strings.ReplaceAll(h, `'`, `''`) + `'` }

// sinComillas quita las comillas de un nombre de hoja citado ('Mi hoja' → Mi hoja).
func sinComillas(h string) string {
	if len(h) >= 2 && h[0] == '\'' && h[len(h)-1] == '\'' {
		return strings.ReplaceAll(h[1:len(h)-1], `''`, `'`)
	}
	return h
}

// hojaEditor es el prefijo de hoja como lo escribe el EDITOR (`sheetPrefix` de
// sheets/format.ts): sin comillas solo si es una palabra ASCII, que es lo que su
// tokenizador lee sin ellas. No es hojaCitada: esa es para Excel y Calc, que sí leen
// letras con tilde sin comillas.
func hojaEditor(h string) string {
	if reHojaSimple.MatchString(h) {
		return h
	}
	return "'" + strings.ReplaceAll(h, "'", "''") + "'"
}

var reHojaSimple = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.$]*$`)

// nombreDeODS lee un <table:named-range> o <table:named-expression>; hoja es su ámbito
// ("" si cuelga del libro).
func nombreDeODS(se xml.StartElement, hoja string) (definedName, bool) {
	n := definedName{Name: attrLocal(se, "name"), Sheet: hoja}
	switch se.Name.Local {
	case "named-range":
		partes := strings.Split(attrLocal(se, "cell-range-address"), ":")
		for i, p := range partes {
			partes[i] = odfRefToA1(p)
		}
		n.Ref = strings.Join(partes, ":")
	case "named-expression":
		n.Ref = odfFormulaToA1(attrLocal(se, "expression"))
	default:
		return n, false
	}
	return n, n.Name != "" && n.Ref != ""
}

// definirNombres carga los nombres en el motor (antes que las celdas, como el lienzo).
func definirNombres(eng *formula.Engine, names []definedName) {
	for _, n := range names {
		if n.Sheet == "" {
			eng.DefineName(n.Name, n.Ref, "")
		} else {
			eng.DefineSheetName(n.Name, n.Ref, n.Sheet)
		}
	}
}
