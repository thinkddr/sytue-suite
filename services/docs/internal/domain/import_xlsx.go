// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/xml"
	"html"
	"path"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Importador OOXML SpreadsheetML (.xlsx) -> SheetModel -> doc:sheet. Reverso de
// export_sheet.go: lee xl/workbook.xml (hojas), xl/sharedStrings.xml (cadenas
// compartidas), xl/styles.xml (formatos de número) y cada xl/worksheets/sheetN.xml
// (valores, fórmulas, formatos). Clean-room sobre ECMA-376 con encoding/xml. Las
// celdas se guardan como HTML escapado (igual que el modelo del editor); las
// fórmulas con prefijo "=".

// importXlsx convierte los bytes de un .xlsx en (KindSpreadsheet, JSON SheetModel).
func importXlsx(data []byte, p *presupuesto) (Kind, string, error) {
	files, err := readZip(data)
	if err != nil {
		return "", "", err
	}
	l, err := abrirLibroXlsx(files, p)
	if err != nil || p.agotado() {
		return KindSpreadsheet, "", err // agotado: importar devuelve ErrDemasiadoGrande
	}
	model := l.modelo(p)
	if p.agotado() {
		return KindSpreadsheet, "", nil
	}
	return KindSpreadsheet, marshalSheetModel(model), nil
}

// libroXlsx es lo que se lee una vez de un .xlsx: sus partes, la tabla de cadenas, los estilos
// y la lista de hojas. modelo lo convierte, y recortarLibro lo convierte varias veces, con
// distinto tope de filas, sin volver a leer nada de esto.
type libroXlsx struct {
	files     map[string][]byte
	principal string // la parte del libro: xl/workbook.xml casi siempre (partePrincipal)
	shared    []string
	estilos   *xlsxEstilos
	rels      map[string]string
	wb        struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			RID  string `xml:"id,attr"`
		} `xml:"sheets>sheet"`
		Names []xlsxDefinedName `xml:"definedNames>definedName"`
	}
}

func abrirLibroXlsx(files map[string][]byte, p *presupuesto) (*libroXlsx, error) {
	principal := partePrincipal(files, "xl/workbook.xml")
	if len(files[principal]) == 0 {
		return nil, wrapFormat("xlsx: falta el libro (" + principal + ")")
	}
	// Las cadenas, los estilos y el tema, donde dicen las relaciones del libro; si no las
	// tiene, donde los pone Excel.
	partes := map[string]string{"sharedStrings": "xl/sharedStrings.xml", "styles": "xl/styles.xml", "theme": "xl/theme/theme1.xml"}
	l := &libroXlsx{files: files, principal: principal, rels: map[string]string{}}
	for _, r := range parseRelsTyped(files[relsPath(principal)]) {
		l.rels[r.ID] = r.Target
		if _, ok := partes[path.Base(r.Type)]; ok {
			partes[path.Base(r.Type)] = destinoRel(principal, r.Target)
		}
	}
	if l.shared = parseSharedStrings(files[partes["sharedStrings"]], p); p.agotado() {
		return l, nil
	}
	l.estilos = parseXlsxStyles(files[partes["styles"]], files[partes["theme"]])
	if xml.Unmarshal(files[principal], &l.wb) != nil {
		return nil, wrapFormat("xlsx: workbook.xml inválido")
	}
	return l, nil
}

// modelo convierte el libro con el presupuesto (y el tope de filas) de `p`; nil si se agota.
func (l *libroXlsx) modelo(p *presupuesto) *sheetModel {
	model := &sheetModel{}
	parts := make([]string, 0, len(l.wb.Sheets))
	for i, sh := range l.wb.Sheets {
		target := l.rels[sh.RID]
		if target == "" {
			target = "worksheets/sheet" + strconv.Itoa(i+1) + ".xml"
		}
		parte := destinoRel(l.principal, target)
		parts = append(parts, parte)
		name := sh.Name
		if name == "" {
			name = "Hoja" + strconv.Itoa(i+1)
		}
		sd, objetos := parseWorksheet(name, l.files[parte], l.shared, l.estilos, p)
		if p.agotado() {
			return nil
		}
		objetosDeHojaXlsx(l.files, parte, &sd, objetos) // tablas, gráficos, minigráficos…
		model.Sheets = append(model.Sheets, sd)
	}
	if len(model.Sheets) == 0 {
		model.Sheets = []sheetData{{Name: "Hoja1", Cells: map[string]string{}}}
	}
	hojas := make([]string, len(model.Sheets))
	for i, sd := range model.Sheets {
		hojas[i] = sd.Name
	}
	resolverListas(model)
	model.Names = nombresDeXlsx(sinNombresDeSegmentador(l.wb.Names), hojas) // localSheetId = índice en <sheets>
	if bloqueado, h := lockDeXlsx(l.files[l.principal]); bloqueado {
		model.Protect = protectConClave(map[string]any{}, h)
	}
	attachXlsxPivots(l.files, model, parts)
	attachXlsxSlicers(l.files, l.principal, model, parts) // segmentadores de Excel (x14/x15); tras las dinámicas
	return model
}

// celdaXlsx es un `<c>` de la hoja.
type celdaXlsx struct {
	Ref     string `xml:"r,attr"`
	Type    string `xml:"t,attr"`
	Style   string `xml:"s,attr"`
	Formula struct {
		Texto string `xml:",chardata"`
		Tipo  string `xml:"t,attr"`
		Si    string `xml:"si,attr"`
	} `xml:"f"`
	V  string `xml:"v"`
	Is struct {
		T string `xml:"t"`
		R []struct {
			T string `xml:"t"`
		} `xml:"r"`
	} `xml:"is"`
}

// formulaDeCelda es la fórmula de una celda. Una compartida (`t="shared"`) lleva el texto solo
// en la maestra; las demás la heredan con las referencias relativas desplazadas.
func formulaDeCelda(c celdaXlsx, maestras map[string]celdaXlsx) string {
	f := strings.TrimSpace(c.Formula.Texto)
	if c.Formula.Tipo != "shared" || c.Formula.Si == "" {
		return f
	}
	if f != "" {
		maestras[c.Formula.Si] = c
		return f
	}
	m, ok := maestras[c.Formula.Si]
	if !ok {
		return ""
	}
	r0, c0, ok0 := ParseA1(m.Ref)
	r1, c1, ok1 := ParseA1(c.Ref)
	if !ok0 || !ok1 {
		return ""
	}
	if s := formula.ShiftRefs("="+strings.TrimSpace(m.Formula.Texto), c1-c0, r1-r0); strings.HasPrefix(s, "=") {
		return s[1:]
	}
	return "" // #REF!: la fórmula se sale de la hoja
}

// verdadero lee un booleano de OOXML («1», «true»).
func verdadero(s string) bool { return s == "1" || strings.EqualFold(s, "true") }

// hojaXlsx es lo que se lee de una hoja fuera de sus celdas.
type hojaXlsx struct {
	Cols []struct {
		Min         string `xml:"min,attr"`
		Max         string `xml:"max,attr"`
		Width       string `xml:"width,attr"`
		CustomWidth string `xml:"customWidth,attr"`
		Outline     string `xml:"outlineLevel,attr"`
		Hidden      string `xml:"hidden,attr"`
	} `xml:"col"`
	Combinadas []struct {
		Ref string `xml:"ref,attr"`
	} `xml:"mergeCell"`
	Pestana       *colorXlsx              `xml:"tabColor"`
	Validaciones  []validacionXlsxEntrada `xml:"dataValidation"`
	Escenarios    *xlsxEscenarios         `xml:"-"`
	Condicionales []formatoCondXlsx       `xml:"-"`
	Proteccion    *xlsxSheetProtection    `xml:"-"`
}

// objetosLeidosXlsx son los elementos de una hoja que apuntan a otras partes (el dibujo, las
// tablas) o que llevan extensiones (minigráficos, segmentaciones), y su autofiltro: los lee
// objetosDeHojaXlsx. Se recogen en la misma pasada que las celdas, en vez de volver a leer la
// hoja entera para ellos.
type objetosLeidosXlsx struct {
	Drawing struct {
		ID string `xml:"id,attr"`
	} `xml:"-"`
	Tablas []struct {
		ID string `xml:"id,attr"`
	} `xml:"tablePart"`
	AutoFilter *xlsxAutoFilter `xml:"-"`
	Ext        []struct {
		URI   string `xml:"uri,attr"`
		Inner []byte `xml:",innerxml"`
	} `xml:"ext"`
}

// finDeDatos es dónde termina el cierre de `sheetData` (con prefijo o sin él) en el XML de una
// hoja; 0 si no se encuentra. Una búsqueda de bytes, no una expresión regular: se hace en cada
// intento del recorte, sobre hojas de decenas de MB.
func finDeDatos(raw []byte) int {
	i := bytes.LastIndex(raw, []byte("sheetData>"))
	if i < 0 {
		return 0
	}
	j := i - 1
	for j >= 0 && (raw[j] == ':' || raw[j] == '_' || raw[j] == '-' || raw[j] == '.' ||
		raw[j] >= 'a' && raw[j] <= 'z' || raw[j] >= 'A' && raw[j] <= 'Z' || raw[j] >= '0' && raw[j] <= '9') {
		j--
	}
	if j < 1 || raw[j] != '/' || raw[j-1] != '<' {
		return 0
	}
	return i + len("sheetData>")
}

// parseWorksheet convierte una hoja OOXML en SheetData (celdas + formatos).
//
// La hoja se lee en flujo, celda a celda, cargando cada una al presupuesto: deshacerla
// entera en estructuras (xml.Unmarshal) multiplicaba por veinte lo que ocupa su XML antes
// de poder mirar si cabía (import_presupuesto.go). Del resto de la hoja solo se descodifican
// los hijos directos de <worksheet> que se usan; lo de `extLst` no se toca, como antes.
//
//nolint:gocognit,gocyclo // traductor de un formato a otro: un caso por tipo, y trocearlo reparte la tabla sin simplificarla
func parseWorksheet(name string, raw []byte, shared []string, estilos *xlsxEstilos, p *presupuesto) (sheetData, objetosLeidosXlsx) {
	sd := sheetData{Name: name, Cells: map[string]string{}}
	var objetos objetosLeidosXlsx
	if len(raw) == 0 {
		return sd, objetos
	}
	var ws hojaXlsx
	filaNivel, filaOculta := map[int]int{}, map[int]bool{}
	var formats map[string]numFmtSpec
	compartidas := map[string]celdaXlsx{} // fórmulas compartidas: la celda maestra por `si`
	colocar := func(c celdaXlsx) {
		row, col, ok := ParseA1(c.Ref)
		if !ok || row > maxFilasHoja || col > maxColumnasHoja {
			return
		}
		if p.recorta(row) { // la celda dice otra fila que la suya (un fichero raro u hostil)
			sd.recortada = true
			return
		}
		isText := c.Is.T
		if isText == "" && len(c.Is.R) > 0 {
			var b strings.Builder
			for _, r := range c.Is.R {
				b.WriteString(r.T)
			}
			isText = b.String()
		}
		text := xlsxCellText(c.Type, c.V, isText, formulaDeCelda(c, compartidas), shared)
		xf, conEstilo := estilos.xf(c.Style)
		if text == "" {
			// Una celda vacía que solo lleva un relleno o un borde: el estilo se conserva.
			if conEstilo && !xf.estilo.vacio() && len(sd.Styles) < maxEstilosHoja {
				if sd.Styles == nil {
					sd.Styles = map[string]cellStyle{}
				}
				sd.Styles[c.Ref] = xf.estilo
			}
			return
		}
		celda := html.EscapeString(text)
		if conEstilo {
			celda = envolverCelda(celda, xf.negrita, xf.cursiva, xf.alinear)
			if !xf.estilo.vacio() && len(sd.Styles) < maxEstilosHoja {
				if sd.Styles == nil {
					sd.Styles = map[string]cellStyle{}
				}
				sd.Styles[c.Ref] = xf.estilo
			}
			if xf.num != nil {
				if formats == nil {
					formats = map[string]numFmtSpec{}
				}
				formats[c.Ref] = *xf.num
			}
		}
		if _, repetida := sd.Cells[c.Ref]; !repetida {
			p.gastar(len(c.Ref) + len(celda))
		}
		sd.Cells[c.Ref] = celda
	}
	// fila lee una <row> ya abierta: su alto y su nivel de esquema, y sus celdas. false si
	// queda fuera del tope de filas de un libro recortado (y no la lee).
	n := 0
	fila := func(dec *xml.Decoder, f xml.StartElement) bool {
		// `r` es opcional en la fila y en la celda (ECMA-376 §18.3.1.73 y §18.3.1.4): sin él,
		// cada una va detrás de la anterior. Lo omiten escritores en flujo y los de .NET, y
		// sin esto la hoja se abría vacía.
		if r, err := strconv.Atoi(attrLocal(f, "r")); err == nil {
			n = r
		} else {
			n++
		}
		if p.recorta(n) {
			return false
		}
		if n >= 1 && n <= maxFilasHoja {
			if verdadero(attrLocal(f, "customHeight")) {
				if ht, err := strconv.ParseFloat(attrLocal(f, "ht"), 64); err == nil {
					sd.fijarAlto(n-1, ht*96/72)
				}
			}
			if l, err := strconv.Atoi(attrLocal(f, "outlineLevel")); err == nil && l > 0 && len(filaNivel) < maxFilasEsquema {
				filaNivel[n-1], filaOculta[n-1] = l, verdadero(attrLocal(f, "hidden"))
			}
		}
		col := 0
		for {
			tok, err := dec.Token()
			if err != nil {
				return true
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local != "c" {
					_ = dec.Skip()
					continue
				}
				var c celdaXlsx
				if dec.DecodeElement(&c, &t) != nil {
					return true
				}
				if _, k, ok := ParseA1(c.Ref); ok {
					col = k
				} else {
					col++
					c.Ref = A1Ref(n, col)
				}
				colocar(c)
			case xml.EndElement:
				return true // </row>
			}
		}
	}
	dec := p.decodificador(raw)
	nivel := 0 // 1 = hijos de <worksheet>
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if _, ok := tok.(xml.EndElement); ok {
			nivel--
			continue
		}
		t, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if nivel == 0 {
			nivel = 1 // <worksheet>
			continue
		}
		switch t.Name.Local {
		case "sheetData":
			for {
				tok, err := dec.Token()
				if err != nil {
					break
				}
				if r, ok := tok.(xml.StartElement); ok && r.Name.Local == "row" {
					if !fila(dec, r) {
						// Un libro recortado: las filas de más no se leen. Se sigue tras el cierre
						// de sheetData, que se busca en los bytes: leerlas para saltarlas costaba
						// tanto como importarlas, en cada intento del recorte.
						sd.recortada = true
						if fin := finDeDatos(raw); fin > 0 {
							dec = p.decodificador(raw[fin:])
						} else {
							_ = dec.Skip() // la fila
							_ = dec.Skip() // y el resto de sheetData
						}
						break
					}
				} else if ok {
					_ = dec.Skip()
				} else if _, fin := tok.(xml.EndElement); fin {
					break
				}
			}
		case "drawing":
			objetos.Drawing.ID = attrLocal(t, "id")
			_ = dec.Skip()
		case "tableParts", "extLst":
			_ = dec.DecodeElement(&objetos, &t)
		case "autoFilter":
			objetos.AutoFilter = &xlsxAutoFilter{}
			_ = dec.DecodeElement(objetos.AutoFilter, &t)
		case "sheetProtection":
			ws.Proteccion = &xlsxSheetProtection{}
			_ = dec.DecodeElement(ws.Proteccion, &t)
		case "cols", "mergeCells", "sheetPr", "dataValidations":
			_ = dec.DecodeElement(&ws, &t) // cada uno rellena sus campos de hojaXlsx
		case "scenarios":
			ws.Escenarios = &xlsxEscenarios{}
			_ = dec.DecodeElement(ws.Escenarios, &t)
		case "conditionalFormatting":
			var cf formatoCondXlsx
			if dec.DecodeElement(&cf, &t) == nil {
				ws.Condicionales = append(ws.Condicionales, cf)
			}
		default:
			_ = dec.Skip()
		}
	}
	if p.agotado() {
		return sd, objetos
	}
	colNivel, colOculta := map[int]int{}, map[int]bool{}
	for _, c := range ws.Cols {
		lo, e1 := strconv.Atoi(c.Min)
		hi, e2 := strconv.Atoi(c.Max)
		if e1 != nil || e2 != nil || lo < 1 || hi < lo {
			continue
		}
		hi = min(hi, maxColumnasHoja)
		if w, err := strconv.ParseFloat(c.Width, 64); err == nil && verdadero(c.CustomWidth) {
			for col := lo; col <= hi && col <= maxColumnasConAncho; col++ {
				sd.fijarAncho(col-1, pxDeAnchoExcel(w))
			}
		}
		if l, err := strconv.Atoi(c.Outline); err == nil && l > 0 {
			for col := lo; col <= hi; col++ {
				colNivel[col-1], colOculta[col-1] = l, verdadero(c.Hidden)
			}
		}
	}
	sd.RowGroups = gruposDeNiveles(filaNivel, filaOculta, maxFilasHoja)
	sd.ColGroups = gruposDeNiveles(colNivel, colOculta, maxColumnasHoja)
	if formats != nil {
		sd.Formats = formats
	}
	sd.Scenarios = escenariosDeXlsx(ws.Escenarios)
	if ws.Pestana != nil {
		sd.TabColor = estilos.resolver(*ws.Pestana) // con o sin hoja de estilos: tema e indexados
	}
	for _, m := range ws.Combinadas {
		sd.Merges = append(sd.Merges, m.Ref)
	}
	sd.Merges = combinadasDeRangos(sanearCombinadas(sd.Merges))
	sd.CondFormats = reglasDeXlsx(ws.Condicionales, estilos)
	sd.Validations = validacionesDeXlsx(ws.Validaciones)
	if protect, h := protectDeXlsx(ws.Proteccion); protect != nil {
		sd.Protect = protectConClave(protect, h)
	}
	return sd, objetos
}

// xlsxCellText resuelve el texto crudo de una celda: fórmula (prefijo "="),
// cadena compartida, cadena inline, booleano o número.
func xlsxCellText(typ, v, isText, fx string, shared []string) string {
	if strings.TrimSpace(fx) != "" {
		return "=" + strings.TrimSpace(fx)
	}
	switch typ {
	case "s":
		if idx, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && idx >= 0 && idx < len(shared) {
			return shared[idx]
		}
		return ""
	case "inlineStr":
		return isText
	case "b":
		if strings.TrimSpace(v) == "1" {
			return "TRUE"
		}
		return "FALSE"
	default: // "n" (número), "str" (resultado de fórmula) o vacío
		return v
	}
}

// parseSharedStrings lee xl/sharedStrings.xml a una tabla índice->texto. Concatena
// los runs de texto enriquecido de cada <si>; la guía fonética (`rPh`) no es texto de la
// celda. En flujo y con el presupuesto de la importación, como la hoja: deshecha en
// estructuras, una tabla de cadenas grande ocupaba varias veces su XML.
func parseSharedStrings(data []byte, p *presupuesto) []string {
	if len(data) == 0 {
		return nil
	}
	var out []string
	dec := p.decodificador(data)
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		if si, ok := tok.(xml.StartElement); ok && si.Name.Local == "si" {
			out = append(out, cadenaCompartida(dec))
		}
	}
}

// cadenaCompartida lee un <si> ya abierto: su texto, o el de sus runs si los tiene.
func cadenaCompartida(dec *xml.Decoder) string {
	var t, runs strings.Builder
	conRuns := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if _, fin := tok.(xml.EndElement); fin {
			break // </si>
		}
		e, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch e.Name.Local {
		case "t":
			t.WriteString(readElementText(dec, "t"))
		case "r":
			conRuns = true
			runs.WriteString(textoDeRun(dec))
		default:
			_ = dec.Skip()
		}
	}
	if conRuns {
		return runs.String()
	}
	return t.String()
}

// textoDeRun es el texto de un <r> ya abierto de una cadena enriquecida.
func textoDeRun(dec *xml.Decoder) string {
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return b.String()
		}
		switch e := tok.(type) {
		case xml.StartElement:
			if e.Name.Local == "t" {
				b.WriteString(readElementText(dec, "t"))
			} else {
				_ = dec.Skip()
			}
		case xml.EndElement:
			return b.String() // </r>
		}
	}
}

// numFmtSpecFor deduce el preset del editor a partir del numFmtId estándar o del
// formatCode personalizado. Reverso de ooxmlNumFmtCode (export_sheet.go).
func numFmtSpecFor(numFmtID, code string) *numFmtSpec {
	// Estándar ECMA-376 (§18.8.30) de uso común.
	switch numFmtID {
	case "", "0":
		return nil // General
	case "1":
		return &numFmtSpec{ID: "number", Decimals: ptr(0)}
	case "2":
		return &numFmtSpec{ID: "number", Decimals: ptr(2)}
	case "3":
		return &numFmtSpec{ID: "thousands", Decimals: ptr(0)}
	case "4":
		return &numFmtSpec{ID: "thousands", Decimals: ptr(2)}
	case "9":
		return &numFmtSpec{ID: "percent", Decimals: ptr(0)}
	case "10":
		return &numFmtSpec{ID: "percent", Decimals: ptr(2)}
	case "14", "15", "16", "17", "22":
		return &numFmtSpec{ID: "date"}
	}
	return codeToNumFmtSpec(code)
}

// codeToNumFmtSpec clasifica un formatCode arbitrario en un preset del editor.
func codeToNumFmtSpec(code string) *numFmtSpec {
	c := strings.TrimSpace(code)
	if c == "" || strings.EqualFold(c, "general") {
		return nil
	}
	lower := strings.ToLower(c)
	switch {
	case strings.Contains(c, "€") || strings.Contains(c, "$") || strings.Contains(lower, "usd") || strings.Contains(lower, "eur"):
		return &numFmtSpec{ID: "currency", Decimals: ptr(decimalsInCode(c))}
	case strings.Contains(c, "%"):
		return &numFmtSpec{ID: "percent", Decimals: ptr(decimalsInCode(c))}
	case strings.ContainsAny(lower, "dmy") && (strings.Contains(lower, "d") || strings.Contains(lower, "y")):
		return &numFmtSpec{ID: "date"}
	case strings.Contains(c, "#,##0") || strings.Contains(c, "#,###"):
		return &numFmtSpec{ID: "thousands", Decimals: ptr(decimalsInCode(c))}
	case strings.Contains(c, "0"):
		return &numFmtSpec{ID: "number", Decimals: ptr(decimalsInCode(c))}
	}
	return nil
}

// decimalsInCode cuenta los dígitos '0'/'#' tras el primer punto decimal del
// código (fuera de literales entrecomillados).
func decimalsInCode(code string) int {
	// Descarta literales entre comillas para no contar puntos de texto.
	var b strings.Builder
	inQuote := false
	for i := 0; i < len(code); i++ {
		if code[i] == '"' {
			inQuote = !inQuote
			continue
		}
		if !inQuote {
			b.WriteByte(code[i])
		}
	}
	c := b.String()
	dot := strings.IndexByte(c, '.')
	if dot < 0 {
		return 0
	}
	n := 0
	for i := dot + 1; i < len(c); i++ {
		if c[i] == '0' || c[i] == '#' {
			n++
		} else {
			break
		}
	}
	return n
}

func ptr(i int) *int { return &i }
