// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Las TABLAS DINÁMICAS de una hoja, hacia fuera: en .xlsx como pivotCacheDefinition +
// pivotTableDefinition (ECMA-376 Parte 1, §18.10) y en .ods como
// <table:data-pilot-table> (OpenDocument 1.3, §9.6). Las celdas que la tabla ya
// escribió viajan como celdas normales; esto añade la DEFINICIÓN, para que Excel y
// LibreOffice la reconozcan como tabla dinámica y la puedan actualizar.
//
// Ida y vuelta sin pérdida: la definición entera de Sytue (agrupar fechas, intervalos,
// «mostrar valores como», formatos) va además, tal cual, en una extensión que Excel y
// Calc ignoran (`extLst` en OOXML, un atributo de espacio propio en ODF). Al importar,
// si está, manda ella; si no —un fichero hecho en Excel o en Calc—, se reconstruye de
// las partes normalizadas.

// pivotSpec es lo que el servicio necesita de un `SheetPivot` (sheets/pivot.ts). El
// resto de campos viaja en `raw`, intacto.
type pivotSpec struct {
	Name        string           `json:"name"`
	Source      string           `json:"source"`
	SourceSheet string           `json:"sourceSheet,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	Anchor      string           `json:"anchor"`
	Range       string           `json:"range,omitempty"`
	Rows        []pivotFieldSpec `json:"rows"`
	Cols        []pivotFieldSpec `json:"cols"`
	Values      []pivotValueSpec `json:"values"`
	Filters     []pivotFieldSpec `json:"filters,omitempty"`
	// HiddenFilters filtran sin estar en la tabla (los de una segmentación): en Excel, un
	// pivotField sin eje con elementos ocultos; en Calc, un campo «hidden» con miembros.
	HiddenFilters []pivotFieldSpec `json:"hiddenFilters,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	GrandRows     *bool            `json:"grandRows,omitempty"`     //nolint:tagliatelle // el nombre lo fija la web
	GrandCols     *bool            `json:"grandCols,omitempty"`     //nolint:tagliatelle // el nombre lo fija la web
	Subtotals     *bool            `json:"subtotals,omitempty"`
	raw           json.RawMessage
}

type pivotFieldSpec struct {
	Field  int      `json:"field"`
	Hidden []string `json:"hidden,omitempty"`
}

type pivotValueSpec struct {
	Field int    `json:"field"`
	Fn    string `json:"fn"`
	Show  string `json:"show,omitempty"`
	Name  string `json:"name,omitempty"`
}

// UnmarshalJSON guarda también el objeto entero, para la extensión sin pérdida.
func (p *pivotSpec) UnmarshalJSON(b []byte) error {
	type plano pivotSpec
	var x plano
	if err := json.Unmarshal(b, &x); err != nil {
		return err
	}
	*p = pivotSpec(x)
	p.raw = append(json.RawMessage(nil), b...)
	return nil
}

// MarshalJSON devuelve el objeto tal cual entró (o el reconstruido, si no hay original).
func (p pivotSpec) MarshalJSON() ([]byte, error) {
	if len(p.raw) > 0 {
		return p.raw, nil
	}
	type plano pivotSpec
	x := plano(p)
	if x.Rows == nil {
		x.Rows = []pivotFieldSpec{}
	}
	if x.Cols == nil {
		x.Cols = []pivotFieldSpec{}
	}
	if x.Values == nil {
		x.Values = []pivotValueSpec{}
	}
	return json.Marshal(x)
}

// Las funciones de resumen en OOXML (`ST_DataConsolidateFunction`) y en ODF.
var (
	pivotFnXlsx = map[string]string{
		"sum": "sum", "count": "count", "avg": "average", "max": "max", "min": "min", "product": "product",
		"countNums": "countNums", "stdev": "stdDev", "stdevp": "stdDevp", "var": "var", "varp": "varp",
	}
	pivotShowXlsx = map[string]string{
		"pctGrand": "percentOfTotal", "pctCol": "percentOfCol", "pctRow": "percentOfRow", "pctParent": "percentOfParentRow",
	}
	pivotFnOds = map[string]string{
		"sum": "sum", "count": "count", "avg": "average", "max": "max", "min": "min", "product": "product",
		"countNums": "countnums", "stdev": "stdev", "stdevp": "stdevp", "var": "var", "varp": "varp",
	}
)

// pivotSheetOf es la hoja del modelo con ese nombre (o la de índice i si el nombre va vacío).
func pivotSourceSheet(m *sheetModel, i int, p pivotSpec) (sheetData, string, bool) {
	if p.SourceSheet == "" {
		return m.Sheets[i], sheetName(m, i), true
	}
	for j, sd := range m.Sheets {
		if sheetName(m, j) == p.SourceSheet {
			return sd, p.SourceSheet, true
		}
	}
	return sheetData{}, "", false
}

func sheetName(m *sheetModel, i int) string {
	if n := m.Sheets[i].Name; n != "" {
		return n
	}
	return "Hoja" + strconv.Itoa(i+1)
}

// pivotColumn son los valores de un campo del origen (sin el encabezado): el texto
// crudo de cada celda, y si es un número.
type pivotItem struct {
	text  string
	isNum bool
	num   float64
}

// pivotSourceGrid lee el origen: los nombres de los campos y, por campo, sus valores.
func pivotSourceGrid(sd sheetData, source string) (names []string, cols [][]pivotItem, ok bool) {
	r1, c1, r2, c2, ok := parseA1Range(source)
	if !ok {
		return nil, nil, false
	}
	for c := c1; c <= c2; c++ {
		ref := A1Ref(r1, c)
		name := stripHTML(sd.Cells[ref])
		if strings.TrimSpace(name) == "" {
			name = "Columna" + strconv.Itoa(c-c1+1)
		}
		names = append(names, name)
		var col []pivotItem
		for r := r1 + 1; r <= r2; r++ {
			cv := classifyCell(A1Ref(r, c), sd.Cells[A1Ref(r, c)], nil)
			it := pivotItem{text: cv.raw}
			if cv.isNum {
				it.isNum = true
				it.num, _ = strconv.ParseFloat(cv.numText, 64)
			}
			if cv.isForm {
				it.text = "" // el servicio no calcula: una fórmula del origen va como vacía
			}
			col = append(col, it)
		}
		cols = append(cols, col)
	}
	return names, cols, true
}

// parseA1Range lee «A1:C10» (filas y columnas 1-based, como A1Ref).
func parseA1Range(s string) (r1, c1, r2, c2 int, ok bool) {
	a, b, hay := strings.Cut(strings.ToUpper(strings.TrimSpace(s)), ":")
	if !hay {
		b = a
	}
	r1, c1, ok1 := ParseA1(a)
	r2, c2, ok2 := ParseA1(b)
	if !ok1 || !ok2 {
		return 0, 0, 0, 0, false
	}
	return min(r1, r2), min(c1, c2), max(r1, r2), max(c1, c2), true
}

// distinctItems son los valores distintos de un campo, en orden de aparición, con la
// clave de Sytue de cada uno (la de `hidden`).
func distinctItems(col []pivotItem) (items []pivotItem, keys []string) {
	seen := map[string]bool{}
	for _, it := range col {
		k := pivotKey(it)
		if !seen[k] {
			seen[k] = true
			items = append(items, it)
			keys = append(keys, k)
		}
	}
	return items, keys
}

// pivotKey es la clave de un elemento como la escribe la tabla de Sytue: el número en
// su forma corta o el texto.
//
// ponytail: la web usa `formatNumber` (15 cifras significativas) y aquí va la forma más
// corta exacta; solo difieren en números con más de 15 cifras, que no son etiquetas.
func pivotKey(it pivotItem) string {
	if it.isNum {
		return strconv.FormatFloat(it.num, 'f', -1, 64)
	}
	return it.text
}

// dentro es la tabla sin los campos que no existen en un origen de n columnas: un
// índice fuera haría un fichero que Excel no abre. Tampoco lleva los filtros ocultos
// que no filtran: el de un campo que ya está en un eje duerme (`computePivotTable`).
// La extensión conserva el original.
func (p pivotSpec) dentro(n int) pivotSpec {
	campos := func(l []pivotFieldSpec) []pivotFieldSpec {
		var out []pivotFieldSpec
		for _, f := range l {
			if f.Field >= 0 && f.Field < n {
				out = append(out, f)
			}
		}
		return out
	}
	p.Rows, p.Cols, p.Filters = campos(p.Rows), campos(p.Cols), campos(p.Filters)
	enEje := pivotAxisFields(p)
	var sueltos []pivotFieldSpec
	for _, f := range campos(p.HiddenFilters) {
		if !enEje[f.Field] && len(f.Hidden) > 0 {
			sueltos = append(sueltos, f)
		}
	}
	p.HiddenFilters = sueltos
	var vs []pivotValueSpec
	for _, v := range p.Values {
		if v.Field >= 0 && v.Field < n {
			vs = append(vs, v)
		}
	}
	p.Values = vs
	return p
}

// ---- OOXML ----

// addXlsxPivots añade al paquete las tablas dinámicas de cada hoja: las partes de la
// caché y de la tabla, sus relaciones y sus tipos, y las <pivotCaches> del libro.
func addXlsxPivots(files map[string]string, m *sheetModel) {
	cacheID := 0
	var caches, wbRels, types strings.Builder
	for i, sd := range m.Sheets {
		var sheetRels strings.Builder
		for _, p := range sd.Pivots {
			src, srcName, ok := pivotSourceSheet(m, i, p)
			if !ok {
				continue
			}
			names, cols, ok := pivotSourceGrid(src, p.Source)
			if !ok {
				continue
			}
			p = p.dentro(len(names))
			cacheID++
			n := strconv.Itoa(cacheID)
			defPart := "xl/pivotCache/pivotCacheDefinition" + n + ".xml"
			tabPart := "xl/pivotTables/pivotTable" + n + ".xml"
			files[defPart] = xlsxPivotCache(p, srcName, names, cols)
			files[tabPart] = xlsxPivotTable(p, n, names, cols)
			files["xl/pivotTables/_rels/pivotTable"+n+".xml.rels"] = xmlHeader +
				`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
				`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/pivotCacheDefinition" Target="../pivotCache/pivotCacheDefinition` + n + `.xml"/>` +
				`</Relationships>`
			caches.WriteString(`<pivotCache cacheId="` + n + `" r:id="rIdPc` + n + `"/>`)
			wbRels.WriteString(`<Relationship Id="rIdPc` + n + `" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/pivotCacheDefinition" Target="pivotCache/pivotCacheDefinition` + n + `.xml"/>`)
			sheetRels.WriteString(`<Relationship Id="rIdPt` + n + `" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/pivotTable" Target="../pivotTables/pivotTable` + n + `.xml"/>`)
			types.WriteString(`<Override PartName="/` + defPart + `" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.pivotCacheDefinition+xml"/>` +
				`<Override PartName="/` + tabPart + `" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.pivotTable+xml"/>`)
		}
		if sheetRels.Len() == 0 {
			continue
		}
		relsPart := "xl/worksheets/_rels/sheet" + strconv.Itoa(i+1) + ".xml.rels"
		if cur, ok := files[relsPart]; ok && strings.Contains(cur, "</Relationships>") {
			files[relsPart] = strings.Replace(cur, "</Relationships>", sheetRels.String()+"</Relationships>", 1)
		} else {
			files[relsPart] = xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
				sheetRels.String() + `</Relationships>`
		}
	}
	if cacheID == 0 {
		return
	}
	// <pivotCaches> va tras <calcPr> en CT_Workbook: justo antes del cierre.
	files["xl/workbook.xml"] = strings.Replace(files["xl/workbook.xml"], "</workbook>", "<pivotCaches>"+caches.String()+"</pivotCaches></workbook>", 1)
	files["xl/_rels/workbook.xml.rels"] = strings.Replace(files["xl/_rels/workbook.xml.rels"], "</Relationships>", wbRels.String()+"</Relationships>", 1)
	files["[Content_Types].xml"] = strings.Replace(files["[Content_Types].xml"], "</Types>", types.String()+"</Types>", 1)
}

// pivotAxisFields son los campos del origen que van en un eje (filas, columnas o
// filtros): Excel les pide sus elementos en la caché.
func pivotAxisFields(p pivotSpec) map[int]bool {
	out := map[int]bool{}
	for _, l := range [][]pivotFieldSpec{p.Rows, p.Cols, p.Filters} {
		for _, f := range l {
			out[f.Field] = true
		}
	}
	return out
}

// xlsxPivotCache es la definición de la caché: el origen y, por campo, sus elementos.
// Sin registros y con `refreshOnLoad`: Excel la rellena al abrir.
func xlsxPivotCache(p pivotSpec, srcSheet string, names []string, cols [][]pivotItem) string {
	axis := pivotAxisFields(p)
	for _, f := range p.HiddenFilters { // sus ocultos se dicen por elemento, como los de un eje
		axis[f.Field] = true
	}
	var b strings.Builder
	b.WriteString(xmlHeader + `<pivotCacheDefinition xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" refreshOnLoad="1" saveData="0" ` +
		`createdVersion="6" refreshedVersion="6" minRefreshableVersion="3" recordCount="` + strconv.Itoa(len(cols[0])) + `">` +
		`<cacheSource type="worksheet"><worksheetSource ref="` + xmlEscape(strings.ToUpper(p.Source)) + `" sheet="` + xmlEscape(srcSheet) + `"/></cacheSource>` +
		`<cacheFields count="` + strconv.Itoa(len(names)) + `">`)
	for f, name := range names {
		b.WriteString(`<cacheField name="` + xmlEscape(name) + `" numFmtId="0">`)
		b.WriteString(xlsxSharedItems(cols[f], axis[f]))
		b.WriteString(`</cacheField>`)
	}
	b.WriteString(`</cacheFields></pivotCacheDefinition>`)
	return b.String()
}

// xlsxSharedItems describe los valores de un campo: con sus elementos si va en un eje.
func xlsxSharedItems(col []pivotItem, withItems bool) string {
	attrs := xlsxSharedAttrs(col)
	if !withItems {
		return `<sharedItems` + attrs + `/>`
	}
	items, _ := distinctItems(col)
	var b strings.Builder
	b.WriteString(`<sharedItems` + attrs + ` count="` + strconv.Itoa(len(items)) + `">`)
	for _, it := range items {
		switch {
		case it.isNum:
			b.WriteString(`<n v="` + strconv.FormatFloat(it.num, 'g', -1, 64) + `"/>`)
		case it.text == "":
			b.WriteString(`<m/>`)
		default:
			b.WriteString(`<s v="` + xmlEscape(it.text) + `"/>`)
		}
	}
	b.WriteString(`</sharedItems>`)
	return b.String()
}

// xlsxSharedAttrs son los atributos de <sharedItems>: qué tipos lleva y su rango.
func xlsxSharedAttrs(col []pivotItem) string {
	hasNum, hasStr, hasBlank := false, false, false
	lo, hi := 0.0, 0.0
	for _, it := range col {
		switch {
		case it.isNum:
			if !hasNum {
				lo, hi = it.num, it.num
			}
			lo, hi, hasNum = min(lo, it.num), max(hi, it.num), true
		case it.text == "":
			hasBlank = true
		default:
			hasStr = true
		}
	}
	attrs := ""
	if !hasStr {
		attrs += ` containsSemiMixedTypes="0" containsString="0"`
	}
	if hasNum {
		attrs += ` containsNumber="1"`
		if !hasStr && !hasBlank && isIntegral(lo) && isIntegral(hi) {
			attrs += ` containsInteger="1"`
		}
		attrs += ` minValue="` + strconv.FormatFloat(lo, 'g', -1, 64) + `" maxValue="` + strconv.FormatFloat(hi, 'g', -1, 64) + `"`
	}
	if hasBlank {
		attrs += ` containsBlank="1"`
	}
	if hasNum && hasStr {
		attrs += ` containsMixedTypes="1"`
	}
	return attrs
}

func isIntegral(x float64) bool { return x == float64(int64(x)) }

// xlsxPivotTable es la definición de la tabla: dónde está, qué campo va en cada eje,
// los elementos ocultos, los campos de valores y, en su extensión, la definición de
// Sytue entera.
func xlsxPivotTable(p pivotSpec, cacheID string, names []string, cols [][]pivotItem) string {
	loc, pageRows := xlsxPivotLocation(p)
	var b strings.Builder
	b.WriteString(xmlHeader + `<pivotTableDefinition xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" name="` +
		xmlEscape(p.Name) + `" cacheId="` + cacheID + `" dataCaption="Valores" applyNumberFormats="0" applyBorderFormats="0" ` +
		`applyFontFormats="0" applyPatternFormats="0" applyAlignmentFormats="0" applyWidthHeightFormats="1" ` +
		`updatedVersion="6" minRefreshableVersion="3" createdVersion="6" useAutoFormatting="1" itemPrintTitles="1" ` +
		`indent="0" compact="0" compactData="0" outline="1" outlineData="1" multipleFieldFilters="0"`)
	if p.GrandRows != nil && !*p.GrandRows {
		b.WriteString(` rowGrandTotals="0"`)
	}
	if p.GrandCols != nil && !*p.GrandCols {
		b.WriteString(` colGrandTotals="0"`)
	}
	b.WriteString(`><location ref="` + xmlEscape(loc) + `" firstHeaderRow="1" firstDataRow="` +
		strconv.Itoa(len(p.Cols)+1) + `" firstDataCol="` + strconv.Itoa(max(len(p.Rows), 1)) + `"`)
	if pageRows > 0 {
		b.WriteString(` rowPageCount="` + strconv.Itoa(len(p.Filters)) + `" colPageCount="1"`)
	}
	b.WriteString(`/>`)
	xlsxPivotFields(&b, p, names, cols)
	xlsxAxisFields(&b, "rowFields", p.Rows, false)
	xlsxAxisFields(&b, "colFields", p.Cols, len(p.Values) > 1)
	if len(p.Filters) > 0 {
		b.WriteString(`<pageFields count="` + strconv.Itoa(len(p.Filters)) + `">`)
		for _, f := range p.Filters {
			b.WriteString(`<pageField fld="` + strconv.Itoa(f.Field) + `" hier="-1"/>`)
		}
		b.WriteString(`</pageFields>`)
	}
	xlsxDataFields(&b, p, names)
	def, _ := json.Marshal(p)
	b.WriteString(`<pivotTableStyleInfo name="PivotStyleLight16" showRowHeaders="1" showColHeaders="1" ` +
		`showRowStripes="0" showColStripes="0" showLastColumn="1"/>` +
		`<extLst><ext uri="` + pivotExtURI + `" xmlns:sy="` + pivotExtNS + `"><sy:pivot>` + xmlEscape(string(def)) +
		`</sy:pivot></ext></extLst></pivotTableDefinition>`)
	return b.String()
}

// xlsxPivotLocation es la ubicación de Excel —que no incluye los filtros del informe,
// que van encima— y cuántas filas ocupan esos filtros.
func xlsxPivotLocation(p pivotSpec) (string, int) {
	loc := strings.ToUpper(p.Range)
	if loc == "" {
		loc = strings.ToUpper(p.Anchor)
	}
	if len(p.Filters) == 0 {
		return loc, 0
	}
	pageRows := len(p.Filters) + 1
	if r1, c1, r2, c2, ok := parseA1Range(loc); ok && r1+pageRows <= r2 {
		loc = A1Ref(r1+pageRows, c1) + ":" + A1Ref(r2, c2)
	}
	return loc, pageRows
}

// xlsxPivotFields son los <pivotField>: uno por campo del origen, con su eje, sus
// elementos (y los ocultos) si va en uno, y si es un campo de valores.
func xlsxPivotFields(b *strings.Builder, p pivotSpec, names []string, cols [][]pivotItem) {
	axisOf, hidden := map[int]string{}, map[int][]string{}
	for _, eje := range []struct {
		ax string
		l  []pivotFieldSpec
	}{{"axisRow", p.Rows}, {"axisCol", p.Cols}, {"axisPage", p.Filters}, {"", p.HiddenFilters}} {
		for _, f := range eje.l {
			axisOf[f.Field], hidden[f.Field] = eje.ax, f.Hidden
		}
	}
	isData := map[int]bool{}
	for _, v := range p.Values {
		isData[v.Field] = true
	}
	subtotals := p.Subtotals == nil || *p.Subtotals
	b.WriteString(`<pivotFields count="` + strconv.Itoa(len(names)) + `">`)
	for f := range names {
		b.WriteString(`<pivotField compact="0" outline="0" showAll="0"`)
		if isData[f] {
			b.WriteString(` dataField="1"`)
		}
		ax, onAxis := axisOf[f]
		if !onAxis {
			b.WriteString(`/>`)
			continue
		}
		if !subtotals {
			b.WriteString(` defaultSubtotal="0"`)
		}
		if ax != "" { // un filtro oculto no va en ningún eje, pero lleva sus elementos
			b.WriteString(` axis="` + ax + `"`)
		}
		b.WriteString(`>`)
		_, keys := distinctItems(cols[f])
		n := len(keys)
		if subtotals {
			n++
		}
		b.WriteString(`<items count="` + strconv.Itoa(n) + `">`)
		for x, k := range keys {
			h := ""
			if containsStr(hidden[f], k) {
				h = ` h="1"`
			}
			b.WriteString(`<item x="` + strconv.Itoa(x) + `"` + h + `/>`)
		}
		if subtotals {
			b.WriteString(`<item t="default"/>`)
		}
		b.WriteString(`</items></pivotField>`)
	}
	b.WriteString(`</pivotFields>`)
}

// xlsxAxisFields son los campos de un eje; `values` añade el pseudocampo «Valores» (-2),
// que Excel pide con varios campos de valores.
func xlsxAxisFields(b *strings.Builder, tag string, l []pivotFieldSpec, values bool) {
	if len(l) == 0 && !values {
		return
	}
	n := len(l)
	if values {
		n++
	}
	b.WriteString(`<` + tag + ` count="` + strconv.Itoa(n) + `">`)
	for _, f := range l {
		b.WriteString(`<field x="` + strconv.Itoa(f.Field) + `"/>`)
	}
	if values {
		b.WriteString(`<field x="-2"/>`)
	}
	b.WriteString(`</` + tag + `>`)
}

// xlsxDataFields son los campos de valores con su función y su «mostrar como».
func xlsxDataFields(b *strings.Builder, p pivotSpec, names []string) {
	if len(p.Values) == 0 {
		return
	}
	b.WriteString(`<dataFields count="` + strconv.Itoa(len(p.Values)) + `">`)
	for _, v := range p.Values {
		name := v.Name
		if name == "" && v.Field < len(names) {
			// Excel no admite que un valor se llame como un campo del origen: el espacio
			// del final es su propia salida para eso.
			name = names[v.Field] + " "
		}
		b.WriteString(`<dataField name="` + xmlEscape(name) + `" fld="` + strconv.Itoa(v.Field) + `"`)
		if fn := pivotFnXlsx[v.Fn]; fn != "" && fn != "sum" {
			b.WriteString(` subtotal="` + fn + `"`)
		}
		if s := pivotShowXlsx[v.Show]; s != "" {
			b.WriteString(` showDataAs="` + s + `"`)
		}
		b.WriteString(` baseField="0" baseItem="0"/>`)
	}
	b.WriteString(`</dataFields>`)
}

// La extensión con la definición de Sytue: un URI y un espacio de nombres propios, que
// Excel y Calc ignoran y conservan.
const (
	pivotExtURI = "{8F4C2A63-5B7E-4D1A-9C3E-7A1B2C3D4E5F}"
	pivotExtNS  = "urn:sytue:calcus:pivot"
)

func containsStr(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// ---- ODF ----

// odsPivots son las <table:data-pilot-table> de todas las hojas.
func odsPivots(m *sheetModel) string {
	var b strings.Builder
	for i, sd := range m.Sheets {
		for _, p := range sd.Pivots {
			src, srcName, ok := pivotSourceSheet(m, i, p)
			if !ok {
				continue
			}
			names, cols, ok := pivotSourceGrid(src, p.Source)
			if !ok {
				continue
			}
			p = p.dentro(len(names))
			b.WriteString(odsPivot(p, sheetName(m, i), srcName, names, cols))
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return `<table:data-pilot-tables>` + b.String() + `</table:data-pilot-tables>`
}

// odsAddress es «Hoja.A1:Hoja.C10» (el nombre entre comillas si hace falta).
func odsAddress(sheet, rng string) string {
	q := sheet
	if strings.ContainsAny(sheet, " .'-") {
		q = "'" + strings.ReplaceAll(sheet, "'", "''") + "'"
	}
	a, b, hay := strings.Cut(strings.ToUpper(rng), ":")
	if !hay {
		return q + "." + a
	}
	return q + "." + a + ":" + q + "." + b
}

// odsGrandTotal es `table:grand-total`: qué totales generales se ven. "" = los dos.
func odsGrandTotal(p pivotSpec) string {
	rows := p.GrandRows == nil || *p.GrandRows
	cols := p.GrandCols == nil || *p.GrandCols
	switch {
	case rows && cols:
		return ""
	case cols:
		return "column"
	case rows:
		return "row"
	}
	return "none"
}

func odsPivot(p pivotSpec, sheet, srcSheet string, names []string, cols [][]pivotItem) string {
	target := p.Range
	if target == "" {
		target = p.Anchor
	}
	def, _ := json.Marshal(p)
	var b strings.Builder
	b.WriteString(`<table:data-pilot-table table:name="` + xmlEscape(p.Name) + `" table:target-range-address="` +
		xmlEscape(odsAddress(sheet, target)) + `" xmlns:sy="` + pivotExtNS + `" sy:pivot="` + xmlEscape(string(def)) + `"`)
	if g := odsGrandTotal(p); g != "" {
		b.WriteString(` table:grand-total="` + g + `"`)
	}
	b.WriteString(`><table:source-cell-range table:cell-range-address="` + xmlEscape(odsAddress(srcSheet, p.Source)) + `"/>`)
	subtotals := p.Subtotals == nil || *p.Subtotals
	for _, grupo := range []struct {
		l  []pivotFieldSpec
		or string
	}{{p.Filters, "page"}, {p.Rows, "row"}, {p.Cols, "column"}, {p.HiddenFilters, "hidden"}} {
		for _, f := range grupo.l {
			odsFieldXML(&b, f, grupo.or, names, cols, subtotals)
		}
	}
	for _, v := range p.Values {
		if v.Field >= len(names) {
			continue
		}
		fn := pivotFnOds[v.Fn]
		if fn == "" {
			fn = "sum"
		}
		b.WriteString(`<table:data-pilot-field table:source-field-name="` + xmlEscape(names[v.Field]) +
			`" table:orientation="data" table:function="` + fn + `"/>`)
	}
	b.WriteString(`</table:data-pilot-table>`)
	return b.String()
}

// odsFieldXML es un campo de filas, columnas o filtros, con sus miembros ocultos.
func odsFieldXML(b *strings.Builder, f pivotFieldSpec, orientation string, names []string, cols [][]pivotItem, subtotals bool) {
	if f.Field >= len(names) {
		return
	}
	b.WriteString(`<table:data-pilot-field table:source-field-name="` + xmlEscape(names[f.Field]) +
		`" table:orientation="` + orientation + `">`)
	b.WriteString(`<table:data-pilot-level table:show-empty="false">`)
	if !subtotals {
		b.WriteString(`<table:data-pilot-subtotals/>`)
	}
	if len(f.Hidden) > 0 {
		b.WriteString(`<table:data-pilot-members>`)
		items, keys := distinctItems(cols[f.Field])
		for x, k := range keys {
			b.WriteString(`<table:data-pilot-member table:name="` + xmlEscape(pivotKey(items[x])) + `" table:display="` +
				strconv.FormatBool(!containsStr(f.Hidden, k)) + `"/>`)
		}
		b.WriteString(`</table:data-pilot-members>`)
	}
	b.WriteString(`</table:data-pilot-level></table:data-pilot-field>`)
}
