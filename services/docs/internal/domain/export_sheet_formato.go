// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"slices"
	"strconv"
	"strings"
)

// Exportación a OOXML del formato de una hoja (ver sheet_formato.go): el registro de
// estilos de `xl/styles.xml` (fuentes, rellenos, bordes, cellXfs y dxfs), y en la hoja el
// formato condicional y las validaciones de datos.

// xfKey es lo que distingue un `cellXf` de otro.
type xfKey struct {
	num          string // formatCode ("" = General)
	font, fill   int
	border       int
	horiz, vert  string
	ajustarTexto bool
	// protección de celda: desbloqueada (locked="0") y con la fórmula oculta
	desbloqueada, oculta bool
}

// xlsxStyles es el registro de `xl/styles.xml`: cada fuente, relleno, borde, formato de
// número y combinación de ellos se escribe una sola vez.
type xlsxStyles struct {
	codes   []string       // formatCode personalizados (numFmtId 164+)
	codeID  map[string]int // formatCode -> numFmtId
	fonts   []string
	fills   []string
	borders []string
	xfs     []xfKey
	dxfs    []string
	idx     map[string]int // «f:<xml>», «r:<xml>», «b:<xml>», «d:<xml>» -> índice en su lista
	xfIdx   map[xfKey]int
}

const (
	fuenteBase  = `<font><sz val="11"/><name val="Calibri"/></font>`
	rellenoNada = `<fill><patternFill patternType="none"/></fill>`
	rellenoGris = `<fill><patternFill patternType="gray125"/></fill>`
	bordeNada   = `<border><left/><right/><top/><bottom/><diagonal/></border>`
)

func newXlsxStyles() *xlsxStyles {
	x := &xlsxStyles{
		codeID:  map[string]int{},
		fonts:   []string{fuenteBase},
		fills:   []string{rellenoNada, rellenoGris}, // Excel exige estos dos primeros
		borders: []string{bordeNada},
		xfs:     []xfKey{{}},
		idx:     map[string]int{},
		xfIdx:   map[xfKey]int{{}: 0},
	}
	return x
}

// en devuelve el índice de s en lista, añadiéndolo si no está.
func (x *xlsxStyles) en(lista *[]string, pref, s string) int {
	if i, ok := x.idx[pref+s]; ok {
		return i
	}
	*lista = append(*lista, s)
	x.idx[pref+s] = len(*lista) - 1
	return len(*lista) - 1
}

// numFmtID es el id del formato de número; los dos «date» y compañía se escriben como
// código personalizado, que es lo que entienden Excel y LibreOffice.
func (x *xlsxStyles) numFmtID(code string) int {
	if code == "" {
		return 0
	}
	if id, ok := x.codeID[code]; ok {
		return id
	}
	id := 164 + len(x.codes)
	x.codes = append(x.codes, code)
	x.codeID[code] = id
	return id
}

func fuenteXlsx(c cellVal) string {
	st := c.estilo
	if !c.negrita && !c.cursiva && st.Font == "" && st.Size == 0 && st.Color == "" && !st.U && !st.S {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<font>`)
	if c.negrita {
		b.WriteString(`<b/>`)
	}
	if c.cursiva {
		b.WriteString(`<i/>`)
	}
	if st.S {
		b.WriteString(`<strike/>`)
	}
	if st.U {
		b.WriteString(`<u/>`)
	}
	tam := 11
	if st.Size > 0 {
		tam = st.Size
	}
	b.WriteString(`<sz val="` + strconv.Itoa(tam) + `"/>`)
	if col := colorHex(st.Color); col != "" {
		b.WriteString(`<color rgb="` + argb(col) + `"/>`)
	}
	nombre := "Calibri"
	if st.Font != "" {
		nombre = st.Font
	}
	b.WriteString(`<name val="` + xmlEscape(nombre) + `"/></font>`)
	return b.String()
}

func bordeXlsx(b *cellBorder) string {
	if b == nil || *b == (cellBorder{}) {
		return ""
	}
	lado := func(tag string, on bool, trazo, color string) string {
		if !on {
			return `<` + tag + `/>`
		}
		estilo, c := "thin", `<color auto="1"/>`
		if estilosBorde[trazo] {
			estilo = trazo
		}
		if col := colorHex(color); col != "" {
			c = `<color rgb="` + argb(col) + `"/>`
		}
		return `<` + tag + ` style="` + estilo + `">` + c + `</` + tag + `>`
	}
	return `<border>` + lado("left", b.L, b.LS, b.LC) + lado("right", b.R, b.RS, b.RC) + lado("top", b.T, b.TS, b.TC) +
		lado("bottom", b.B, b.BS, b.BC) + `<diagonal/></border>`
}

// xfFor es el índice de cellXfs de una celda: su formato de número, su fuente (negrita y
// cursiva vienen del HTML de la celda), su relleno, sus bordes y su alineación. 0 = el
// estilo por defecto.
func (x *xlsxStyles) xfFor(c cellVal) int {
	k := xfKey{}
	if c.fmt != nil {
		k.num = ooxmlNumFmtCode(*c.fmt)
		x.numFmtID(k.num)
	}
	if f := fuenteXlsx(c); f != "" {
		k.font = x.en(&x.fonts, "f:", f)
	}
	if col := colorHex(c.estilo.Fill); col != "" {
		k.fill = x.en(&x.fills, "r:", `<fill><patternFill patternType="solid"><fgColor rgb="`+argb(col)+`"/><bgColor indexed="64"/></patternFill></fill>`)
	}
	if b := bordeXlsx(c.estilo.Border); b != "" {
		k.border = x.en(&x.borders, "b:", b)
	}
	if c.alinear == "center" || c.alinear == "right" {
		k.horiz = c.alinear
	}
	k.vert = map[string]string{"top": "top", "middle": "center", "bottom": "bottom"}[c.estilo.Valign]
	k.ajustarTexto = c.estilo.Wrap
	k.desbloqueada, k.oculta = c.estilo.desbloqueada(), c.estilo.Hidden
	if i, ok := x.xfIdx[k]; ok {
		return i
	}
	x.xfs = append(x.xfs, k)
	x.xfIdx[k] = len(x.xfs) - 1
	return len(x.xfs) - 1
}

// dxfFor es el índice de dxfs de lo que pinta una regla de formato condicional: su color de
// letra y su relleno (cualquiera de los dos puede faltar).
func (x *xlsxStyles) dxfFor(fill, tinta string) int {
	var b strings.Builder
	b.WriteString(`<dxf>`)
	if tinta != "" {
		b.WriteString(`<font><color rgb="` + argb(tinta) + `"/></font>`)
	}
	if fill != "" {
		col := argb(fill)
		b.WriteString(`<fill><patternFill patternType="solid"><fgColor rgb="` + col + `"/><bgColor rgb="` + col + `"/></patternFill></fill>`)
	}
	b.WriteString(`</dxf>`)
	return x.en(&x.dxfs, "d:", b.String())
}

//nolint:gocognit,gocyclo // serializa cada tabla de estilos de styles.xml: un caso por atributo, y trocearlo reparte la tabla sin simplificarla
func (x *xlsxStyles) xml() string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	if len(x.codes) > 0 {
		b.WriteString(`<numFmts count="` + strconv.Itoa(len(x.codes)) + `">`)
		for i, code := range x.codes {
			b.WriteString(`<numFmt numFmtId="` + strconv.Itoa(164+i) + `" formatCode="` + xmlEscape(code) + `"/>`)
		}
		b.WriteString(`</numFmts>`)
	}
	lista := func(tag string, items []string) {
		b.WriteString(`<` + tag + ` count="` + strconv.Itoa(len(items)) + `">` + strings.Join(items, "") + `</` + tag + `>`)
	}
	lista("fonts", x.fonts)
	lista("fills", x.fills)
	lista("borders", x.borders)
	b.WriteString(`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>`)
	b.WriteString(`<cellXfs count="` + strconv.Itoa(len(x.xfs)) + `">`)
	for _, k := range x.xfs {
		b.WriteString(`<xf numFmtId="` + strconv.Itoa(x.numFmtID(k.num)) + `" fontId="` + strconv.Itoa(k.font) +
			`" fillId="` + strconv.Itoa(k.fill) + `" borderId="` + strconv.Itoa(k.border) + `" xfId="0"`)
		if k.num != "" {
			b.WriteString(` applyNumberFormat="1"`)
		}
		if k.font != 0 {
			b.WriteString(` applyFont="1"`)
		}
		if k.fill != 0 {
			b.WriteString(` applyFill="1"`)
		}
		if k.border != 0 {
			b.WriteString(` applyBorder="1"`)
		}
		alinea := k.horiz != "" || k.vert != "" || k.ajustarTexto
		protege := k.desbloqueada || k.oculta
		if !alinea && !protege {
			b.WriteString(`/>`)
			continue
		}
		if alinea {
			b.WriteString(` applyAlignment="1"`)
		}
		if protege {
			b.WriteString(` applyProtection="1"`)
		}
		b.WriteString(`>`)
		if alinea {
			b.WriteString(`<alignment`)
			if k.horiz != "" {
				b.WriteString(` horizontal="` + k.horiz + `"`)
			}
			if k.vert != "" {
				b.WriteString(` vertical="` + k.vert + `"`)
			}
			if k.ajustarTexto {
				b.WriteString(` wrapText="1"`)
			}
			b.WriteString(`/>`)
		}
		if protege {
			b.WriteString(`<protection locked="` + unoOCero(!k.desbloqueada) + `" hidden="` + unoOCero(k.oculta) + `"/>`)
		}
		b.WriteString(`</xf>`)
	}
	b.WriteString(`</cellXfs>`)
	b.WriteString(`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>`)
	if len(x.dxfs) > 0 {
		lista("dxfs", x.dxfs)
	}
	b.WriteString(`</styleSheet>`)
	return b.String()
}

// celdasConFormato son las celdas de la hoja que se escriben: las que tienen contenido y
// las vacías que solo llevan un relleno, un borde u otro estilo.
func celdasConFormato(sd sheetData) []cellVal {
	cells := sortedCells(sd)
	hay := make(map[string]bool, len(cells))
	for _, c := range cells {
		hay[c.ref] = true
	}
	extra := 0
	for ref, st := range sd.Styles {
		if hay[ref] || st.vacio() || extra >= maxCeldasVacias {
			continue
		}
		row, col, ok := ParseA1(ref)
		if !ok || row > maxFilasHoja || col > maxColumnasHoja {
			continue
		}
		extra++
		cells = append(cells, cellVal{ref: ref, row: row, col: col, estilo: st, soloFormato: true})
	}
	slices.SortFunc(cells, func(a, b cellVal) int {
		if a.row != b.row {
			return a.row - b.row
		}
		return a.col - b.col
	})
	return cells
}

// operadorCelda es el `operator` de un `cfRule` `cellIs` y el de una validación.
var operadorCelda = map[string]string{
	"gt": "greaterThan", "ge": "greaterThanOrEqual", "lt": "lessThan", "le": "lessThanOrEqual",
	"eq": "equal", "ne": "notEqual", "between": "between", "notBetween": "notBetween",
}

// literalFormula es un valor del editor (texto es-ES) como operando de una fórmula de
// OOXML: un número con punto decimal, o el texto entre comillas.
func literalFormula(valor string) string {
	if n, ok := parseNumEs(valor); ok {
		return numOOXML(n)
	}
	return `"` + strings.ReplaceAll(strings.TrimSpace(valor), `"`, `""`) + `"`
}

var iconosXlsx = map[string]string{"arrows": "3Arrows", "traffic": "3TrafficLights1", "symbols": "3Symbols"}

func cfvo(tipo, val string) string {
	if val == "" {
		return `<cfvo type="` + tipo + `"/>`
	}
	return `<cfvo type="` + tipo + `" val="` + val + `"/>`
}

// reglaXlsx es el `cfRule` de una regla del editor; "" si no se puede escribir.
//
//nolint:gocognit,gocyclo // traductor de un formato a otro: un caso por tipo, y trocearlo reparte la tabla sin simplificarla
func reglaXlsx(r condRule, rg rango, prio int, st *xlsxStyles) string {
	p := ` priority="` + strconv.Itoa(prio) + `"`
	conRelleno := func() (string, bool) {
		col, tinta := colorHex(r.Fill), colorHex(r.TextColor)
		if col == "" && tinta == "" {
			return "", false
		}
		return ` dxfId="` + strconv.Itoa(st.dxfFor(col, tinta)) + `"`, true
	}
	celda := A1Ref(rg.r1, rg.c1) // las fórmulas de la regla van relativas a la primera celda
	switch r.Kind {
	case "", "cell":
		d, ok := conRelleno()
		if !ok || r.Value == "" {
			return ""
		}
		if r.Op == "contains" {
			t := strings.TrimSpace(r.Value)
			return `<cfRule type="containsText"` + d + p + ` operator="containsText" text="` + xmlEscape(t) + `"><formula>NOT(ISERROR(SEARCH("` +
				xmlEscape(strings.ReplaceAll(t, `"`, `""`)) + `",` + celda + `)))</formula></cfRule>`
		}
		op := operadorCelda[r.Op]
		if op == "" {
			return ""
		}
		return `<cfRule type="cellIs"` + d + p + ` operator="` + op + `"><formula>` + xmlEscape(literalFormula(r.Value)) + `</formula></cfRule>`
	case "formula":
		d, ok := conRelleno()
		f := strings.TrimPrefix(strings.TrimSpace(r.Formula), "=")
		if !ok || f == "" {
			return ""
		}
		return `<cfRule type="expression"` + d + p + `><formula>` + xmlEscape(f) + `</formula></cfRule>`
	case "top", "bottom":
		d, ok := conRelleno()
		if !ok || r.Rank <= 0 {
			return ""
		}
		x := `<cfRule type="top10"` + d + p + ` rank="` + strconv.Itoa(r.Rank) + `"`
		if r.Percent {
			x += ` percent="1"`
		}
		if r.Kind == "bottom" {
			x += ` bottom="1"`
		}
		return x + `/>`
	case "dup", "unique":
		d, ok := conRelleno()
		if !ok {
			return ""
		}
		return `<cfRule type="` + map[string]string{"dup": "duplicateValues", "unique": "uniqueValues"}[r.Kind] + `"` + d + p + `/>`
	case "dataBar":
		col := colorHex(r.Color)
		if col == "" {
			return ""
		}
		return `<cfRule type="dataBar"` + p + `><dataBar>` + cfvo("min", "") + cfvo("max", "") + `<color rgb="` + argb(col) + `"/></dataBar></cfRule>`
	case "colorScale":
		if len(r.Scale) != 2 && len(r.Scale) != 3 {
			return ""
		}
		v, c := cfvo("min", ""), ""
		if len(r.Scale) == 3 {
			v += cfvo("percentile", "50")
		}
		v += cfvo("max", "")
		for _, col := range r.Scale {
			h := colorHex(col)
			if h == "" {
				return ""
			}
			c += `<color rgb="` + argb(h) + `"/>`
		}
		return `<cfRule type="colorScale"` + p + `><colorScale>` + v + c + `</colorScale></cfRule>`
	case "iconSet":
		set := iconosXlsx[r.Icons]
		if set == "" {
			set = iconosXlsx["arrows"]
		}
		return `<cfRule type="iconSet"` + p + `><iconSet iconSet="` + set + `">` + cfvo("percent", "0") + cfvo("percent", "33") + cfvo("percent", "67") + `</iconSet></cfRule>`
	}
	return ""
}

// validacionXlsx es el `dataValidation` de una validación del editor; "" si no se puede
// escribir.
//
//nolint:gocognit,gocyclo // traductor de un formato a otro: un caso por tipo, y trocearlo reparte la tabla sin simplificarla
func validacionXlsx(v dataValidation, rg rango) string {
	attrs := ` allowBlank="1" showInputMessage="1" showErrorMessage="1"`
	if !v.Strict {
		attrs += ` errorStyle="warning"`
	}
	if e := strings.TrimSpace(v.Error); e != "" {
		attrs += ` error="` + xmlEscape(truncarRunas(e, 255)) + `"`
	}
	sqref := ` sqref="` + rg.a1() + `"`
	var f1, f2, tipo, op string
	switch v.Type {
	case "", "list":
		tipo = "list"
		lit := ""
		for _, val := range v.Values {
			val = strings.ReplaceAll(val, `"`, `""`)
			if len(lit)+len(val)+1 > 253 { // el literal de Excel cabe en 255 caracteres
				break
			}
			if lit != "" {
				lit += ","
			}
			lit += val
		}
		if lit == "" {
			return ""
		}
		f1 = `"` + lit + `"`
	case "formula":
		tipo = "custom"
		f1 = strings.TrimPrefix(strings.TrimSpace(v.Formula), "=")
		if f1 == "" {
			return ""
		}
	case "number", "date", "textLength":
		tipo = map[string]string{"number": "decimal", "date": "date", "textLength": "textLength"}[v.Type]
		if v.Type == "number" && v.Integer {
			tipo = "whole"
		}
		op = operadorCelda[v.Op]
		a, ok := operandoValidacion(v.Type, v.A)
		if op == "" || !ok {
			return ""
		}
		f1 = a
		if v.Op == "between" || v.Op == "notBetween" {
			b, ok := operandoValidacion(v.Type, v.B)
			if !ok {
				return ""
			}
			f2 = b
		}
	default:
		return ""
	}
	x := `<dataValidation type="` + tipo + `"`
	if op != "" && op != "between" {
		x += ` operator="` + op + `"` // `between` es el valor por defecto
	}
	x += attrs + sqref + `><formula1>` + xmlEscape(f1) + `</formula1>`
	if f2 != "" {
		x += `<formula2>` + xmlEscape(f2) + `</formula2>`
	}
	return x + `</dataValidation>`
}

// operandoValidacion es un operando del editor como número de la fórmula: un día es su
// serial.
func operandoValidacion(tipo, texto string) (string, bool) {
	var n float64
	var ok bool
	if tipo == "date" {
		n, ok = parseFechaEs(texto)
	} else {
		n, ok = parseNumEs(texto)
	}
	if !ok {
		return "", false
	}
	return numOOXML(n), true
}

func truncarRunas(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// formatoHojaXlsx son los `conditionalFormatting` y los `dataValidations` de la hoja, en
// el sitio que les da el esquema (tras el autofiltro y las celdas combinadas, y antes de
// los dibujos y las tablas).
func formatoHojaXlsx(sd sheetData, st *xlsxStyles) string {
	var b strings.Builder
	b.WriteString(mergeCellsXlsx(sd)) // va antes que el formato condicional
	prio := 0
	for _, r := range sd.CondFormats {
		rg, ok := leerRango(r.Range)
		if !ok || !rg.valido() {
			continue
		}
		prio++
		if x := reglaXlsx(r, rg, prio, st); x != "" {
			b.WriteString(`<conditionalFormatting sqref="` + rg.a1() + `">` + x + `</conditionalFormatting>`)
		} else {
			prio--
		}
	}
	var dv strings.Builder
	n := 0
	for _, v := range sd.Validations {
		rg, ok := leerRango(v.Range)
		if !ok || !rg.valido() {
			continue
		}
		if x := validacionXlsx(v, rg); x != "" {
			dv.WriteString(x)
			n++
		}
	}
	if n > 0 {
		b.WriteString(`<dataValidations count="` + strconv.Itoa(n) + `">` + dv.String() + `</dataValidations>`)
	}
	return b.String()
}
