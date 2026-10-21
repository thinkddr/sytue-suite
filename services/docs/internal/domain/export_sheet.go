// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"html"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/numfmt"
)

// Exportadores de HOJA DE CÁLCULO (xlsx/ods) a partir del modelo doc:sheet del
// editor (SheetModel: hojas, celdas HTML, formatos de número). Cada celda se
// clasifica en número / fórmula / texto y se mapea con FIDELIDAD: valores
// tipados, fórmulas (<f> en OOXML, table:formula en ODF) y formatos de número
// (styles.xml en OOXML, estilos de datos en ODF). Obra original de Sytue sobre
// ECMA-376 (SpreadsheetML) y OASIS ODF.

// --- Modelo doc:sheet (espejo de packages/office/src/editor/types.ts) ---

type sheetModel struct {
	Active int         `json:"active"`
	Sheets []sheetData `json:"sheets"`
	// Names son los nombres definidos del libro (sheet_nombres.go).
	Names []definedName `json:"names,omitempty"`
	// Protect es la protección de estructura del libro (proteccion_fichero.go).
	Protect json.RawMessage `json:"protect,omitempty"`
	// indiceTabla es, por hoja, su posición entre las `table:table` de un .ods leído (las hojas
	// de escenario no son hojas del editor y se saltan).
	indiceTabla []int
}

type sheetData struct {
	Name    string                `json:"name"`
	Cells   map[string]string     `json:"cells"`
	Formats map[string]numFmtSpec `json:"formats"`
	// TabColor es el color de la pestaña de la hoja (#RRGGBB); vacío = sin color.
	TabColor string `json:"tabColor,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	objetosHoja
	formatoHoja
	proteccionHoja
	// Pivots son las tablas dinámicas (export_pivot.go).
	Pivots []pivotSpec `json:"pivots,omitempty"`
	// AutoFilter es el autofiltro con sus condiciones (sheet_filtro.go).
	AutoFilter *sheetAutoFilter `json:"autofilter,omitempty"`
	// filtro es el autofiltro ya evaluado, solo al exportar a .xlsx.
	filtro *filtroCalculado
	// escenarioODS es lo que dice el `table:scenario` de una hoja de escenario de ODF leída.
	escenarioODS *escenarioODS
	// recortada: al leerla para un libro recortado, tenía más filas que el tope.
	recortada bool
}

type numFmtSpec struct {
	ID       string `json:"id"`
	Decimals *int   `json:"decimals"`
	Mask     string `json:"mask"` // solo con id "custom"
}

// sheetModelOf obtiene el SheetModel del registro doc:sheet (JSON.stringify en
// una cadena LWW). Si no existe, reconstruye un modelo desde los registros de
// celda "cell:<hoja>!<f>:<c>" (representación CRDT de bajo nivel / compat tests).
func sheetModelOf(s *State) *sheetModel {
	if raw := s.contentString("doc:sheet"); raw != "" {
		var m sheetModel
		if json.Unmarshal([]byte(raw), &m) == nil && len(m.Sheets) > 0 {
			return &m
		}
	}
	return sheetModelFromRegisters(s)
}

// sheetModelFromRegisters agrupa los registros LWW de celda por hoja.
func sheetModelFromRegisters(s *State) *sheetModel {
	bySheet := map[string]map[string]string{}
	var order []string
	for k, r := range s.Registers {
		sheet, row, col, ok := ParseCellKey(k)
		if !ok {
			continue
		}
		var v any
		_ = json.Unmarshal(r.Value, &v)
		if _, seen := bySheet[sheet]; !seen {
			bySheet[sheet] = map[string]string{}
			order = append(order, sheet)
		}
		bySheet[sheet][A1Ref(row, col)] = html.EscapeString(anyToString(v))
	}
	if len(order) == 0 {
		return nil
	}
	sort.Strings(order)
	m := &sheetModel{}
	for _, name := range order {
		m.Sheets = append(m.Sheets, sheetData{Name: name, Cells: bySheet[name]})
	}
	return m
}

func anyToString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "TRUE"
		}
		return "FALSE"
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// --- Clasificación de celda ---

type cellVal struct {
	ref     string // A1
	row     int
	col     int
	raw     string      // texto crudo (sin HTML)
	fmt     *numFmtSpec // formato de número (o nil)
	isNum   bool
	isForm  bool
	numText string // representación numérica canónica (si isNum)
	formula string // expresión sin '=' (si isForm)
	// soloMarco es una celda vacía que solo existe para colgar el marco de un gráfico.
	soloMarco bool
	// soloFormato es una celda vacía que solo existe por su estilo (un relleno, un borde).
	soloFormato bool
	// validacion es el nombre de la validación de ODF que cubre la celda.
	validacion string
	// cubierta es una celda tapada por una combinada (ODF: `table:covered-table-cell`) y span, el
	// rango que combina un ancla.
	cubierta bool
	span     *rango
	// estilo, negrita, cursiva y alinear son el formato visual de la celda: lo primero del
	// mapa `styles` de la hoja, y lo demás del HTML de la propia celda.
	estilo           cellStyle
	negrita, cursiva bool
	alinear          string
}

// stripHTML reduce el HTML de una celda a su texto crudo (para clasificar valor).
func stripHTML(s string) string {
	s = reLineBreak.ReplaceAllString(s, " ")
	s = reAnyTag.ReplaceAllString(s, "")
	return strings.TrimSpace(html.UnescapeString(s))
}

func classifyCell(ref, cellHTML string, fmtSpec *numFmtSpec) cellVal {
	row, col, _ := ParseA1(ref)
	c := cellVal{ref: ref, row: row, col: col, raw: stripHTML(cellHTML), fmt: fmtSpec}
	switch {
	case strings.HasPrefix(c.raw, "="):
		c.isForm = true
		c.formula = strings.TrimPrefix(c.raw, "=")
	case c.raw != "":
		if f, err := strconv.ParseFloat(c.raw, 64); err == nil {
			c.isNum = true
			c.numText = strconv.FormatFloat(f, 'f', -1, 64)
		}
	}
	return c
}

// sortedCells devuelve las celdas no vacías de una hoja ordenadas por fila/col.
func sortedCells(sd sheetData) []cellVal {
	out := make([]cellVal, 0, len(sd.Cells))
	for ref, cellHTML := range sd.Cells {
		row, col, ok := ParseA1(ref)
		if !ok {
			continue
		}
		var fmtp *numFmtSpec
		if f, ok := sd.Formats[ref]; ok {
			fc := f
			fmtp = &fc
		}
		c := classifyCell(ref, cellHTML, fmtp)
		c.row, c.col = row, col
		if c.raw == "" {
			continue
		}
		partes := formula.LeerCelda(cellHTML)
		c.negrita, c.cursiva, c.alinear = partes.Negrita, partes.Cursiva, partes.Alinear
		c.estilo = sd.Styles[ref]
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].row != out[j].row {
			return out[i].row < out[j].row
		}
		return out[i].col < out[j].col
	})
	return out
}

// --- OOXML: xlsx ---

func buildXlsx(s *State) ([]byte, error) {
	model := sheetModelOf(s)
	if model == nil || len(model.Sheets) == 0 {
		model = &sheetModel{Sheets: []sheetData{{Name: "Hoja1", Cells: map[string]string{}}}}
	}

	styles := newXlsxStyles()
	files := map[string]string{
		"[Content_Types].xml": "",
		"_rels/.rels":         relsRootWorkbook,
	}

	var sheetsXML, wbRels strings.Builder
	var overrides, basesDeFiltro strings.Builder
	var cuenta contadorObjetos
	hojas := make([]string, len(model.Sheets))
	eng, nombresHoja := motorParaFiltros(model)
	for i, sd := range model.Sheets {
		sd.RowGroups, sd.ColGroups = sanearGrupos(sd.RowGroups, maxFilasHoja), sanearGrupos(sd.ColGroups, maxColumnasHoja)
		n := i + 1
		part := "xl/worksheets/sheet" + strconv.Itoa(n) + ".xml"
		name := sd.Name
		if name == "" {
			name = "Hoja" + strconv.Itoa(n)
		}
		hojas[i] = name
		// Las tablas, los minigráficos, los gráficos y las segmentaciones (ver
		// export_sheet_objetos.go): sus partes, sus relaciones y su cola en la hoja.
		if sd.AutoFilter != nil {
			nombreMotor := name // el motor conoce la hoja por el nombre con el que se cargó
			if eng != nil {
				nombreMotor = nombresHoja[i]
			}
			sd.filtro = calcularFiltro(sd.AutoFilter, sd, eng, nombreMotor, filasDeTotales(sd))
		}
		obj := objetosXlsx(sd, name, &cuenta)
		sd.Cells = celdasConTablas(sd, false)
		filtroHoja := autoFilterHoja(sd)
		if filtroHoja != "" {
			basesDeFiltro.WriteString(baseDeFiltroXlsx(i, name, sd.filtro.ref))
		}
		// La protección va justo tras los datos y antes de los escenarios y del autofiltro, en el
		// orden del esquema.
		proteccion := sheetProtectionXlsx(sd.Protect)
		files[part] = strings.Replace(xlsxSheet(sd, styles), `</sheetData></worksheet>`, `</sheetData>`+proteccion+escenariosXlsx(sd)+filtroHoja+formatoHojaXlsx(sd, styles)+obj.colaHoja.String()+`</worksheet>`, 1)
		for ruta, contenido := range obj.partes {
			files[ruta] = contenido
		}
		for _, o := range obj.overrides {
			overrides.WriteString(o)
		}
		if len(obj.rels) > 0 {
			files["xl/worksheets/_rels/sheet"+strconv.Itoa(n)+".xml.rels"] = xmlHeader +
				`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(obj.rels, "") + `</Relationships>`
		}
		sheetsXML.WriteString(`<sheet name="` + xmlEscape(name) + `" sheetId="` + strconv.Itoa(n) +
			`" r:id="rId` + strconv.Itoa(n) + `"/>`)
		wbRels.WriteString(`<Relationship Id="rId` + strconv.Itoa(n) +
			`" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet` +
			strconv.Itoa(n) + `.xml"/>`)
		overrides.WriteString(`<Override PartName="/` + part +
			`" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`)
	}
	stylesRel := "rId" + strconv.Itoa(len(model.Sheets)+1)

	files["xl/workbook.xml"] = xmlHeader +
		`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
		workbookProtectionXlsx(model.Protect) +
		`<sheets>` + sheetsXML.String() + `</sheets>` + nombresXlsx(model.Names, hojas, basesDeFiltro.String()) +
		`<calcPr calcId="0" fullCalcOnLoad="1"/></workbook>`

	files["xl/_rels/workbook.xml.rels"] = xmlHeader +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		wbRels.String() +
		`<Relationship Id="` + stylesRel + `" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
		`</Relationships>`

	files["xl/styles.xml"] = styles.xml()

	files["[Content_Types].xml"] = xmlHeader +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
		`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
		overrides.String() +
		`</Types>`

	addXlsxPivots(files, model)
	return zipPackage(files)
}

// tabColorXlsx es el `sheetPr` con el color de la pestaña (va antes que todo lo demás de la hoja).
func tabColorXlsx(c string) string {
	if c = colorHex(c); c == "" {
		return ""
	}
	return `<sheetPr><tabColor rgb="` + argb(c) + `"/></sheetPr>`
}

func xlsxSheet(sd sheetData, styles *xlsxStyles) string {
	var rows strings.Builder
	cells := celdasConFormato(sd)
	altos := filasExtra(sd)
	i, k := 0, 0 // la celda y la fila vacía por las que se va
	for i < len(cells) || k < len(altos) {
		fila := maxFilasHoja + 1
		if i < len(cells) {
			fila = cells[i].row
		}
		if k < len(altos) && altos[k] < fila {
			fila = altos[k]
		}
		rows.WriteString(`<row r="` + strconv.Itoa(fila) + `"` + altoFilaXlsx(sd, fila) + atributosFilaXlsx(sd, fila) + `>`)
		for ; i < len(cells) && cells[i].row == fila; i++ {
			rows.WriteString(xlsxCell(cells[i], styles))
		}
		rows.WriteString(`</row>`)
		for k < len(altos) && altos[k] <= fila {
			k++
		}
	}
	// La rejilla de serie de Calcus (96 × 24 px) es la que declara el libro: lo que no dice
	// su medida se ve igual en Excel que en el editor.
	return xmlHeader +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
		tabColorXlsx(sd.TabColor) + `<sheetFormatPr defaultColWidth="` + strconv.FormatFloat(anchoExcel(anchoSeriePx), 'f', -1, 64) +
		`" defaultRowHeight="` + strconv.FormatFloat(altoSeriePx*0.75, 'f', -1, 64) + `" customHeight="1"` + niveles(sd) + `/>` +
		colsXlsx(sd) + `<sheetData>` + rows.String() + `</sheetData></worksheet>`
}

func xlsxCell(c cellVal, styles *xlsxStyles) string {
	sAttr := ""
	if idx := styles.xfFor(c); idx != 0 {
		sAttr = ` s="` + strconv.Itoa(idx) + `"`
	}
	ref := ` r="` + c.ref + `"`
	switch {
	case c.soloFormato:
		return `<c` + ref + sAttr + `/>`
	case c.isForm:
		// La fórmula se recalcula al abrir (calcPr fullCalcOnLoad="1").
		return `<c` + ref + sAttr + `><f>` + xmlEscape(c.formula) + `</f></c>`
	case c.isNum:
		return `<c` + ref + sAttr + `><v>` + c.numText + `</v></c>`
	default:
		return `<c` + ref + sAttr + ` t="inlineStr"><is><t xml:space="preserve">` + xmlEscape(c.raw) + `</t></is></c>`
	}
}

// defaultDecimals refleja los presets del editor (sheets/format.ts).
func fmtDecimals(spec numFmtSpec) int {
	if spec.Decimals != nil {
		d := *spec.Decimals
		if d < 0 {
			d = 0
		}
		if d > 8 {
			d = 8
		}
		return d
	}
	switch spec.ID {
	case "number", "thousands", "currency":
		return 2
	default: // percent, date, general
		return 0
	}
}

func decZeros(d int) string {
	if d > 0 {
		return "." + strings.Repeat("0", d)
	}
	return ""
}

// ooxmlNumFmtCode traduce el preset del editor al código de formato ECMA-376
// (convención es-ES: € tras el número, fecha dd/mm/yyyy). "" = General (sin estilo).
func ooxmlNumFmtCode(spec numFmtSpec) string {
	d := fmtDecimals(spec)
	switch spec.ID {
	case "number":
		return "0" + decZeros(d)
	case "thousands":
		return "#,##0" + decZeros(d)
	case "currency":
		return "#,##0" + decZeros(d) + `" €"`
	case "percent":
		return "0" + decZeros(d) + "%"
	case "date":
		return "dd/mm/yyyy"
	default:
		return ""
	}
}

// --- ODF: ods ---

// odsBody devuelve (estilos automáticos, cuerpo, styles.xml) de la hoja para content.xml, y los
// objetos incrustados (los gráficos) que van en el paquete.
func odsBody(s *State) (string, string, string, map[string][]byte) {
	model := sheetModelOf(s)
	if model == nil || len(model.Sheets) == 0 {
		model = &sheetModel{Sheets: []sheetData{{Name: "Hoja1", Cells: map[string]string{}}}}
	}
	styles := newOdsStyles()
	validaciones := &validacionesODS{}
	media := map[string][]byte{}
	eng, nombresHoja := motorParaFiltros(model)
	nombresUsados := map[string]bool{} // los de todas las hojas, para que un escenario no repita ninguno
	for i, sd := range model.Sheets {
		n := sd.Name
		if n == "" {
			n = "Hoja" + strconv.Itoa(i+1)
		}
		nombresUsados[strings.ToLower(n)] = true
	}
	objetos := 0
	var body strings.Builder
	primera := ""
	for i, sd := range model.Sheets {
		sd.RowGroups, sd.ColGroups = sanearGrupos(sd.RowGroups, maxFilasHoja), sanearGrupos(sd.ColGroups, maxColumnasHoja)
		name := sd.Name
		if name == "" {
			name = "Hoja" + strconv.Itoa(i+1)
		}
		if i == 0 {
			primera = name
		}
		// Los objetos de la hoja (export_ods_objetos.go): los gráficos cuelgan de su
		// celda, y los minigráficos y las segmentaciones van al final de la tabla.
		if sd.AutoFilter != nil {
			nombreMotor := name
			if eng != nil {
				nombreMotor = nombresHoja[i]
			}
			sd.filtro = calcularFiltro(sd.AutoFilter, sd, eng, nombreMotor, filasDeTotales(sd))
			model.Sheets[i].filtro = sd.filtro
		}
		marcos := marcosODS(sd, name, &objetos, media)
		sd.Cells = celdasConTablas(sd, true)
		body.WriteString(`<table:table table:name="` + xmlEscape(name) + `"` + styles.atributoPestana(sd.TabColor) + atributosTablaODS(sd.Protect) + `>`)
		odsTable(&body, sd, styles, marcos, validaciones.registrar(sd, name))
		body.WriteString(colaTablaODS(sd, name, styles) + nombresODS(model.Names, name, name) + `</table:table>`)
		hojasEscenarioODS(sd, nombresUsados, styles, &body)
	}
	// El esquema de ODF pide los nombres antes que los rangos de base de datos, y estos
	// antes que las tablas dinámicas.
	body.WriteString(nombresODS(model.Names, "", primera) + rangosODS(model.Sheets) + odsPivots(model) + `</office:spreadsheet>`)
	if len(media) == 0 {
		media = nil
	}
	// Las validaciones van antes de las tablas, a las que sus celdas apuntan por nombre.
	return styles.defs.String(), `<office:spreadsheet` + atributosLibroODS(model.Protect) + `>` + validaciones.cabecera() + body.String(), styles.documentoEstilos(), media
}

//nolint:gocognit,gocyclo // recorre filas, celdas, medidas y marcos a la vez en el orden en que ODF los pide
func odsTable(b *strings.Builder, sd sheetData, styles *odsStyles, marcos map[[2]int]string, validas map[string]string) {
	cells := celdasConFormato(sd)
	hay := map[string]bool{}
	for i := range cells {
		hay[cells[i].ref] = true
		cells[i].validacion = validas[cells[i].ref]
	}
	// Las celdas vacías que solo llevan una validación también se escriben.
	for ref, nombre := range validas {
		if row, col, ok := ParseA1(ref); ok && !hay[ref] {
			cells = append(cells, cellVal{ref: ref, row: row, col: col, soloFormato: true, validacion: nombre})
		}
	}
	cells = combinarODS(cells, sanearCombinadas(sd.Merges))
	// Una celda que solo lleva el marco de un gráfico entra vacía en el recorrido.
	for pos := range marcos {
		if _, hay := sd.Cells[A1Ref(pos[0], pos[1])]; !hay {
			cells = append(cells, cellVal{ref: A1Ref(pos[0], pos[1]), row: pos[0], col: pos[1], soloMarco: true})
		}
	}
	slices.SortFunc(cells, func(a, b cellVal) int {
		if a.row != b.row {
			return a.row - b.row
		}
		return a.col - b.col
	})
	maxCol := 1
	for _, c := range cells {
		maxCol = max(maxCol, c.col)
	}
	for _, i := range medidasOrdenadas(sd.ColWidths, maxColumnasHoja) {
		maxCol = max(maxCol, i+1)
	}
	grupos := gruposAnidados(sd.RowGroups, maxFilasHoja)
	maxCol = max(maxCol, ultimaDe(gruposAnidados(sd.ColGroups, maxColumnasHoja)))
	pila := nuevaPilaODS(grupos, "table-row-group")
	cs := cortes(grupos)
	finGrupos := ultimaDe(grupos)
	b.WriteString(columnasODS(sd, maxCol, styles))
	altos := filasConAlto(sd)
	i, k := 0, 0 // la celda y el alto por los que se va
	prevRow := 0
	vacias := func(desde, hasta int) { // filas vacías de serie, partidas donde cambia el esquema
		for _, t := range trozos(desde, hasta, cs) {
			pila.avanzar(t[0], b)
			b.WriteString(`<table:table-row table:style-name="` + styles.estiloFila(altoSeriePx) + `"` + colapsoODS(sd.RowGroups, t[0]-1) +
				` table:number-rows-repeated="` + strconv.Itoa(t[1]-t[0]+1) + `"/>`)
		}
	}
	for i < len(cells) || k < len(altos) {
		row := maxFilasHoja + 1
		if i < len(cells) {
			row = cells[i].row
		}
		if k < len(altos) && altos[k] < row {
			row = altos[k]
		}
		if row-prevRow > 1 {
			vacias(prevRow+1, row-1)
		}
		pila.avanzar(row, b)
		alto := altoSeriePx
		if px, ok := medidaValida(sd.RowHeights[strconv.Itoa(row-1)], maxAltoPx); ok {
			alto = px
		}
		b.WriteString(`<table:table-row table:style-name="` + styles.estiloFila(alto) + `"` + colapsoODS(sd.RowGroups, row-1) + filtradaODS(sd, row) + `>`)
		nextCol := 1
		for ; i < len(cells) && cells[i].row == row; i++ {
			if cells[i].col > nextCol {
				b.WriteString(`<table:table-cell table:number-columns-repeated="` + strconv.Itoa(cells[i].col-nextCol) + `"/>`)
			}
			celda := odsCell(cells[i], styles)
			if m := marcos[[2]int{cells[i].row, cells[i].col}]; m != "" {
				// El marco va dentro de la celda, delante de su texto, como lo escribe Calc.
				if cells[i].soloMarco {
					celda = `<table:table-cell>` + m + `</table:table-cell>`
				} else {
					celda = strings.Replace(celda, `>`, `>`+m, 1)
				}
			}
			b.WriteString(celda)
			nextCol = cells[i].col + 1
		}
		b.WriteString(`</table:table-row>`)
		prevRow = row
		for k < len(altos) && altos[k] <= row {
			k++
		}
	}
	if finGrupos > prevRow {
		vacias(prevRow+1, finGrupos)
	}
	pila.cerrar(b)
}

func odsCell(c cellVal, styles *odsStyles) string {
	attrs := ""
	if name := styles.styleFor(c); name != "" {
		attrs = ` table:style-name="` + name + `"`
	}
	if c.validacion != "" {
		attrs += ` table:content-validation-name="` + c.validacion + `"`
	}
	if c.cubierta {
		return `<table:covered-table-cell/>`
	}
	if c.span != nil {
		attrs += ` table:number-columns-spanned="` + strconv.Itoa(c.span.c2-c.span.c1+1) + `" table:number-rows-spanned="` + strconv.Itoa(c.span.r2-c.span.r1+1) + `"`
	}
	switch {
	case c.soloFormato:
		return `<table:table-cell` + attrs + `/>`
	case c.isForm:
		return `<table:table-cell` + attrs + ` table:formula="` + xmlEscape(a1FormulaToODF(c.formula)) +
			`" office:value-type="float" office:value="0"><text:p>` + xmlEscape(c.raw) + `</text:p></table:table-cell>`
	case c.isNum && c.fmt != nil && c.fmt.ID == "date":
		// Un día es una fecha de ODF (Calc la trata como tal); si el número no es un día, float.
		if n, err := strconv.ParseFloat(c.numText, 64); err == nil {
			if f, ok := fechaODS(n); ok {
				return `<table:table-cell` + attrs + ` office:value-type="date" office:date-value="` + f +
					`"><text:p>` + xmlEscape(c.raw) + `</text:p></table:table-cell>`
			}
		}
		return `<table:table-cell` + attrs + ` office:value-type="float" office:value="` + c.numText +
			`"><text:p>` + xmlEscape(c.raw) + `</text:p></table:table-cell>`
	case c.isNum:
		return `<table:table-cell` + attrs + ` office:value-type="float" office:value="` + c.numText +
			`"><text:p>` + xmlEscape(c.raw) + `</text:p></table:table-cell>`
	default:
		return `<table:table-cell` + attrs + ` office:value-type="string"><text:p>` +
			xmlEscape(c.raw) + `</text:p></table:table-cell>`
	}
}

// odfDataStyle genera el <number:*-style> ODF para el preset dado.
func odfDataStyle(name string, spec numFmtSpec) string {
	d := fmtDecimals(spec)
	num := `<number:number number:decimal-places="` + strconv.Itoa(d) + `" number:min-integer-digits="1"`
	switch spec.ID {
	case "number":
		return `<number:number-style style:name="` + name + `">` + num + `/></number:number-style>`
	case "thousands":
		return `<number:number-style style:name="` + name + `">` + num + ` number:grouping="true"/></number:number-style>`
	case "currency":
		return `<number:currency-style style:name="` + name + `">` + num + ` number:grouping="true"/>` +
			`<number:text> </number:text><number:currency-symbol number:language="es" number:country="ES">€</number:currency-symbol>` +
			`</number:currency-style>`
	case "percent":
		return `<number:percentage-style style:name="` + name + `">` + num + `/><number:text>%</number:text></number:percentage-style>`
	case "date":
		return `<number:date-style style:name="` + name + `">` +
			`<number:day number:style="long"/><number:text>/</number:text>` +
			`<number:month number:style="long"/><number:text>/</number:text>` +
			`<number:year number:style="long"/></number:date-style>`
	default:
		return ""
	}
}

// a1FormulaToODF convierte una fórmula estilo A1 (=A1+SUM(B1:B3)) a OpenFormula
// (of:=[.A1]+SUM([.B1:.B3])): envuelve referencias y rangos en corchetes ODF.
// Escáner (no regex suelta): salta literales de cadena y NO envuelve tokens
// seguidos de '(' (nombres de función tipo LOG10). RE2 no tiene lookahead, por
// eso el recorrido es manual.
// Un nombre definido con forma de celda al principio («AB1_x») no se toca: tras la
// referencia no puede seguir otra letra de palabra.
func a1FormulaToODF(expr string) string { return "of:=" + a1AODF(expr, false) }

// a1AODF es a1FormulaToODF sin el «of:=». Con hojaAbs, la hoja de cada referencia va
// en absoluto («[$Datos.$A$1]»), que es lo que pide un nombre definido: Calc lee una
// hoja relativa dentro de un nombre como relativa a la celda que lo usa, y al guardar
// en .xlsx duplica el nombre en cada hoja (medido con LibreOffice 7.4).
func a1AODF(expr string, hojaAbs bool) string {
	var b strings.Builder
	for i := 0; i < len(expr); {
		ch := expr[i]
		switch {
		case ch == '"': // literal de cadena: copiar verbatim (con escape "")
			j := i + 1
			for j < len(expr) {
				if expr[j] == '"' {
					if j+1 < len(expr) && expr[j+1] == '"' {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			b.WriteString(expr[i:j])
			i = j
		case isRefStart(ch):
			raw, conv := readRefToken(expr[i:], hojaAbs)
			b.WriteString(conv)
			i += len(raw)
		default:
			b.WriteByte(ch)
			i++
		}
	}
	return b.String()
}

func letraDePalabra(ch byte) bool {
	return ch == '_' || ch == '.' || (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isRefStart(ch byte) bool {
	return ch == '$' || ch == '\'' || ch == '_' ||
		(ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

var (
	reODFSheetRef = regexp.MustCompile(`^(?:('[^']*'|[A-Za-z_][A-Za-z0-9_ ]*)!)?(\$?[A-Za-z]{1,3}\$?[0-9]+)(?::(\$?[A-Za-z]{1,3}\$?[0-9]+))?`)
	reIdent       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*`)
)

// readRefToken lee el token al inicio de s: devuelve el texto crudo consumido y
// su conversión (envuelto en [ ] si es una referencia/rango; verbatim si no).
func readRefToken(s string, hojaAbs bool) (raw, conv string) {
	if m := reODFSheetRef.FindStringSubmatch(s); m != nil {
		full := m[0]
		// Seguido de '(' => es un nombre de función que parece ref: no envolver. Seguido
		// de otra letra de palabra => es un nombre («AB1_x»), tampoco.
		if len(full) >= len(s) || (s[len(full)] != '(' && !letraDePalabra(s[len(full)])) {
			prefix := "."
			if m[1] != "" {
				// Las comillas se quedan: ODF las pide igual que A1 («['Mi hoja'.A1]»).
				prefix = m[1] + "."
				if hojaAbs {
					prefix = "$" + prefix
				}
			}
			if m[3] != "" {
				return full, "[" + prefix + m[2] + ":." + m[3] + "]"
			}
			return full, "[" + prefix + m[2] + "]"
		}
	}
	if id := reIdent.FindString(s); id != "" {
		return id, id
	}
	return s[:1], s[:1]
}

// --- Lo que se VE: Markdown, HTML y PDF ---

// sheetGrid es una hoja tal y como se ve: el rectángulo que va de la primera celda usada
// a la última, con el texto de cada celda ya CALCULADO y con su formato de número. Una
// fórmula sale por su valor, no por «=SUM(…)»: estas tres descargas son de lectura.
type sheetGrid struct {
	name string
	rows [][]string
	// estilos es, celda a celda, la alineación, la negrita y la cursiva del HTML de la
	// celda (su `Dentro` no se usa: el texto es el de rows).
	estilos [][]formula.PartesCelda
}

// rawText es `stripHtml` de `sheets/format.ts`: lo que el motor ve de una celda. No es
// stripHTML, que recorta y decodifica todas las entidades: el valor saldría distinto
// del que calcula la web.
var rawText = strings.NewReplacer("&nbsp;", " ", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&amp;", "&")

func cellRaw(cellHTML string) string { return rawText.Replace(reAnyTag.ReplaceAllString(cellHTML, "")) }

// motorDelLibro es el motor con el libro ENTERO cargado (tablas, nombres y celdas), y los
// nombres de hoja con los que se cargó.
func motorDelLibro(m *sheetModel) (*formula.Engine, []string) {
	names := make([]string, len(m.Sheets))
	for i := range m.Sheets {
		names[i] = sheetName(m, i) // el de reserva si viene vacío
	}
	tablas := tablasResolubles(m, names)
	eng := formula.New(formula.Options{Sheets: names, ResolveTable: func(q formula.TableQuery, aqui formula.Here) (*formula.RefRange, formula.Error) {
		return formula.ResolverEstructurada(tablas, q, aqui)
	}})
	definirNombres(eng, m.Names)
	for i, sd := range m.Sheets {
		for ref, h := range sd.Cells {
			if _, _, ok := ParseA1(ref); ok {
				_ = eng.SetCell(names[i], ref, cellRaw(h))
			}
		}
	}
	return eng, names
}

// sheetGrids evalúa el libro ENTERO con el motor de Sytue Sheets —el de Calcus, en
// `apps/nucleo/formula`— y devuelve cada hoja como se ve. Todas las hojas van al motor
// aunque se pinte una: una fórmula puede leer de otra.
// Las referencias estructuradas (`Tabla[Columna]`) se resuelven con las tablas del libro,
// como en el lienzo.
func sheetGrids(m *sheetModel) []sheetGrid {
	eng, names := motorDelLibro(m)
	out := make([]sheetGrid, len(m.Sheets))
	for i, sd := range m.Sheets {
		out[i] = sheetGrid{name: names[i]}
		cells := sortedCells(sd)
		if len(cells) == 0 {
			continue
		}
		r1, r2, c1, c2 := cells[0].row, cells[len(cells)-1].row, cells[0].col, cells[0].col
		for _, c := range cells {
			c1, c2 = min(c1, c.col), max(c2, c.col)
		}
		for r := r1; r <= r2; r++ {
			row := make([]string, c2-c1+1)
			estilos := make([]formula.PartesCelda, c2-c1+1)
			for c := c1; c <= c2; c++ {
				row[c-c1] = cellShown(eng, names[i], sd, A1Ref(r, c))
				estilos[c-c1] = formula.LeerCelda(sd.Cells[A1Ref(r, c)])
			}
			out[i].rows = append(out[i].rows, row)
			out[i].estilos = append(out[i].estilos, estilos)
		}
	}
	return out
}

// tablasResolubles son las tablas con formato de todas las hojas, con los nombres de sus
// columnas sacados del texto crudo de la cabecera.
func tablasResolubles(m *sheetModel, names []string) []formula.TablaResoluble {
	var out []formula.TablaResoluble
	for i, sd := range m.Sheets {
		for _, t := range sd.Tables {
			tr, ok := formula.NuevaTablaResoluble(t.Name, names[i], t.Range, t.Headers == nil || *t.Headers, t.TotalsRow,
				func(c, f int) string { return cellRaw(sd.Cells[A1Ref(f+1, c+1)]) }) // Rect va en base 0
			if ok {
				out = append(out, tr)
			}
		}
	}
	return out
}

// cellShown es `cellHtml` de SheetCanvas sin el HTML: una fórmula, por su valor con el
// formato de número; un literal que es un número y tiene formato, formateado; lo demás,
// el literal TAL CUAL —un «007» tecleado se ve «007», no el 7 que el motor lee de él—.
func cellShown(eng *formula.Engine, sheet string, sd sheetData, ref string) string {
	raw := cellRaw(sd.Cells[ref])
	f, conFmt := sd.Formats[ref]
	if !strings.HasPrefix(raw, "=") && (!conFmt || f.ID == "general") {
		return raw
	}
	v, _ := eng.GetValue(sheet, ref)
	n, esNum := v.(float64)
	switch {
	case esNum && conFmt:
		d := fmtDecimals(f)
		texto, _ := numfmt.Aplicar(f.ID, d, f.Mask, n)
		return texto
	case strings.HasPrefix(raw, "="):
		return formula.Display(v)
	}
	return raw // el formato solo re-pinta números
}

// sheetModelOrEmpty es el modelo de la hoja, o uno vacío.
func sheetModelOrEmpty(s *State) *sheetModel {
	if m := sheetModelOf(s); m != nil {
		return m
	}
	return &sheetModel{}
}

// markdownCell escapa lo que rompería una celda de una tabla de Markdown.
var markdownCell = strings.NewReplacer("|", `\|`, "\n", " ", "\r", "")

// sheetMarkdown es una tabla por hoja, con su nombre de título. La primera fila del
// rectángulo usado hace de cabecera, que es lo que Markdown exige.
func sheetMarkdown(b *strings.Builder, s *State) {
	for _, g := range sheetGrids(sheetModelOrEmpty(s)) {
		b.WriteString("## " + g.name + "\n\n")
		if len(g.rows) == 0 {
			continue
		}
		for i, row := range g.rows {
			b.WriteString("|")
			for j, c := range row {
				b.WriteString(" " + markdownEnfasis(markdownCell.Replace(c), g.estilos[i][j]) + " |")
			}
			b.WriteString("\n")
			if i == 0 {
				b.WriteString("|")
				for j := range row {
					b.WriteString(" " + markdownSeparador(g, j) + " |")
				}
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}
}

// markdownEnfasis envuelve el texto en `**` y `*` según la negrita y la cursiva de la
// celda. Markdown no admite el énfasis con espacios por dentro: se envuelve lo recortado.
func markdownEnfasis(texto string, p formula.PartesCelda) string {
	t := strings.TrimSpace(texto)
	if t == "" {
		return texto
	}
	if p.Cursiva {
		t = "*" + t + "*"
	}
	if p.Negrita {
		t = "**" + t + "**"
	}
	return t
}

// markdownSeparador es la alineación de la columna j. Markdown la fija por COLUMNA, no por
// celda: se usa la de las celdas con texto del cuerpo si todas llevan la misma, y la de
// la cabecera si el cuerpo no tiene ninguna.
func markdownSeparador(g sheetGrid, j int) string {
	alinear, hay, mixta := "", false, false
	for i := 1; i < len(g.rows); i++ {
		if strings.TrimSpace(g.rows[i][j]) == "" {
			continue
		}
		a := g.estilos[i][j].Alinear
		if hay && a != alinear {
			mixta = true
		}
		alinear, hay = a, true
	}
	if !hay {
		alinear = g.estilos[0][j].Alinear
	}
	switch {
	case mixta:
		return "---"
	case alinear == "center":
		return ":---:"
	case alinear == "right":
		return "---:"
	}
	return "---"
}

// sheetHTML es una `<table>` por hoja, con su nombre de título y la primera fila del
// rectángulo usado en `<thead>`.
func sheetHTML(b *strings.Builder, s *State) {
	for _, g := range sheetGrids(sheetModelOrEmpty(s)) {
		b.WriteString("<h2>" + html.EscapeString(g.name) + "</h2>\n")
		if len(g.rows) == 0 {
			continue
		}
		b.WriteString("<table>\n")
		for i, row := range g.rows {
			tag := "td"
			if i == 0 {
				tag = "th"
				b.WriteString("<thead>")
			}
			b.WriteString("<tr>")
			for j, c := range row {
				b.WriteString(htmlCelda(tag, c, g.estilos[i][j]))
			}
			b.WriteString("</tr>")
			if i == 0 {
				b.WriteString("</thead>\n<tbody>")
			}
			b.WriteString("\n")
		}
		b.WriteString("</tbody></table>\n")
	}
}

// htmlCelda es la celda con su alineación en `style` y su negrita y cursiva en `<b>` e
// `<i>`, como las escribe el editor.
func htmlCelda(tag, texto string, p formula.PartesCelda) string {
	t := html.EscapeString(texto)
	if p.Cursiva {
		t = "<i>" + t + "</i>"
	}
	if p.Negrita {
		t = "<b>" + t + "</b>"
	}
	abre := "<" + tag
	if p.Alinear != "" {
		abre += ` style="text-align: ` + p.Alinear + `;"`
	}
	return abre + ">" + t + "</" + tag + ">"
}
