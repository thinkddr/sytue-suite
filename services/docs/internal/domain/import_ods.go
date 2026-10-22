// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/xml"
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// --- ODS (hoja) -> doc:sheet ---

func importOds(data []byte, p *presupuesto) (Kind, string, error) {
	files, err := readZip(data)
	if err != nil {
		return "", "", err
	}
	l, err := abrirLibroOds(files)
	if err != nil {
		return "", "", err
	}
	model := l.modelo(p)
	if p.agotado() {
		return KindSpreadsheet, "", nil // ya no cabe: importar devuelve ErrDemasiadoGrande
	}
	return KindSpreadsheet, marshalSheetModel(model), nil
}

// libroOds es lo que se lee una vez de un .ods: sus estilos, sus formatos, los objetos y cada
// hoja (<table:table>) ya separada de content.xml. modelo lo convierte, y recortarLibro lo
// convierte varias veces con distinto tope de filas sin volver a leer estilos ni las filas
// descartadas.
type libroOds struct {
	estilos  *odsEstilos
	formatos map[string]*numFmtSpec
	objs     []objetosHoja
	tablas   [][]byte // cada <table:table>…</table:table> de content.xml
	resto    []byte   // <office:body> sin las tablas: validaciones, filtros, nombres y dinámicas
}

func abrirLibroOds(files map[string][]byte) (*libroOds, error) {
	content := files["content.xml"]
	if len(content) == 0 {
		return nil, wrapFormat("ods: falta content.xml")
	}
	styles := files["styles.xml"]
	tablas, resto := partirCuerpoODS(content)
	return &libroOds{
		estilos:  parseODSEstilos(styles, content),
		formatos: parseODSFormatos(styles, content),
		objs:     objetosODS(files),
		tablas:   tablas,
		resto:    resto,
	}, nil
}

// modelo convierte el libro con el presupuesto (y el tope de filas) de `p`; nil si se agota.
func (l *libroOds) modelo(p *presupuesto) *sheetModel {
	ctx := &odsCtx{estilos: l.estilos, validaciones: map[string]dataValidation{}, formatos: l.formatos, presupuesto: p}
	model := &sheetModel{}
	filtros, nombresLibro := leerRestoODS(l.resto, model, ctx)
	for tablas, raw := range l.tablas {
		sd, ok := parseODSTableSlice(raw, len(model.Sheets)+1, &model.Names, ctx)
		if p.agotado() {
			return nil
		}
		if !ok {
			continue
		}
		if sd.escenarioODS != nil && len(model.Sheets) > 0 {
			// Calc guarda un escenario como una hoja oculta tras la hoja a la que pertenece.
			base := &model.Sheets[len(model.Sheets)-1]
			base.Scenarios = sanearEscenarios(append(base.Scenarios, sd.escenarioODS.escenario(sd.Name, sd.Cells)))
			continue
		}
		model.Sheets = append(model.Sheets, sd)
		model.indiceTabla = append(model.indiceTabla, tablas)
	}
	model.Names = append(model.Names, nombresLibro...)
	for _, f := range filtros {
		for i := range model.Sheets {
			if model.Sheets[i].Name == f.hoja && model.Sheets[i].AutoFilter == nil {
				model.Sheets[i].AutoFilter = autoFilterDeODS(f, model.Sheets[i].Cells)
			}
		}
	}
	aplicarObjetosODS(model, l.objs)
	if len(model.Sheets) == 0 {
		model.Sheets = []sheetData{{Name: "Hoja1", Cells: map[string]string{}}}
	}
	resolverListas(model)
	attachOdsPivots(l.resto, model)
	return model
}

// leerRestoODS lee lo de `<office:body>` que no son las tablas: el candado de la hoja de
// cálculo, las validaciones, los autofiltros y los nombres globales del libro.
func leerRestoODS(resto []byte, model *sheetModel, ctx *odsCtx) ([]filtroODS, []definedName) {
	if len(resto) == 0 {
		return nil, nil
	}
	var filtros []filtroODS
	var nombresLibro []definedName
	dec := xml.NewDecoder(bytes.NewReader(resto))
	for {
		tok, err := dec.Token()
		if err != nil {
			return filtros, nombresLibro
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "spreadsheet":
			model.Protect = lockDeODS(se)
		case "content-validations":
			leerValidacionesODS(dec, ctx)
		case "database-range":
			if f, valido := leerFiltroODS(dec, se); valido {
				filtros = append(filtros, f)
			}
		default:
			if n, esNombre := nombreDeODS(se, ""); esNombre {
				nombresLibro = append(nombresLibro, n) // del libro: van detrás de las tablas en ODF
			}
		}
	}
}

// aplicarObjetosODS adjunta los objetos de cada hoja (import_ods_objetos.go) y borra las
// celdas de las filas de totales, que el editor calcula al pintar.
func aplicarObjetosODS(model *sheetModel, objs []objetosHoja) {
	for i := range model.Sheets {
		if i >= len(model.indiceTabla) || model.indiceTabla[i] >= len(objs) {
			break
		}
		obj := objs[model.indiceTabla[i]]
		model.Sheets[i].objetosHoja = obj
		for _, t := range obj.Tables {
			if r, ok := leerRango(t.Range); ok && t.TotalsRow {
				for c := r.c1; c <= r.c2; c++ {
					delete(model.Sheets[i].Cells, A1Ref(r.r2, c))
				}
			}
		}
	}
}

// odsCtx es lo que una tabla necesita saber del resto del documento: los estilos de celda y
// las validaciones, que se declaran fuera de ella.
type odsCtx struct {
	estilos      *odsEstilos
	validaciones map[string]dataValidation
	formatos     map[string]*numFmtSpec // estilo de datos -> formato de número del editor
	presupuesto  *presupuesto           // cada celda colocada, también las repetidas
}

// parseODSTableSlice lee el XML de una <table:table> ya aislada.
func parseODSTableSlice(raw []byte, numHoja int, names *[]definedName, ctx *odsCtx) (sheetData, bool) {
	dec := ctx.presupuesto.decodificador(raw)
	for {
		tok, err := dec.Token()
		if err != nil {
			return sheetData{}, false
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "table" {
			continue
		}
		name := attrLocal(se, "name")
		if name == "" {
			name = "Hoja" + strconv.Itoa(numHoja)
		}
		sd := parseODSTable(dec, raw, name, names, ctx)
		sd.Protect = protectDeODS(se)
		sd.TabColor = ctx.estilos.pestanas[attrLocal(se, "style-name")]
		return sd, true
	}
}

// parseODSTable lee una <table:table> a un SheetData contando repeticiones de
// filas/columnas para restituir las referencias A1 correctas.
// Los nombres de la hoja (su <table:named-expressions>) se añaden a names.
//
//nolint:gocognit,gocyclo // recorrido de un solo paso sobre el XML de ODF: un caso por elemento, y trocearlo reparte el estado
func parseODSTable(dec *xml.Decoder, raw []byte, name string, names *[]definedName, ctx *odsCtx) sheetData {
	sd := sheetData{Name: name, Cells: map[string]string{}}
	row, col := 0, 0
	var filaGrupos, colGrupos []grupoAbierto
	var colDef []tramoEstiloODS    // estilo por defecto de las columnas (`table:default-cell-style-name`)
	validas := map[string]string{} // celda -> nombre de la validación que la cubre
	fin := func() sheetData {
		sd.Validations = validacionesDeCeldas(validas, ctx.validaciones)
		sd.RowGroups, sd.ColGroups = sanearGrupos(sd.RowGroups, maxFilasHoja), sanearGrupos(sd.ColGroups, maxColumnasHoja)
		sd.Merges = combinadasDeRangos(sanearCombinadas(sd.Merges))
		return sd
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return fin()
		}
		switch e := tok.(type) {
		case xml.StartElement:
			if e.Name.Local == "table-row" {
				rep := repeticionesODS(attrLocal(e, "number-rows-repeated"))
				if m := ctx.estilos.medida(attrLocal(e, "style-name")); m.alto > 0 && !m.optimo {
					for k := 0; k < min(rep, maxFilasConAlto) && row+k < maxFilasHoja && !ctx.presupuesto.recorta(row+k+1); k++ {
						sd.fijarAlto(row+k, m.alto)
					}
				}
				// ponytail: una fila repetida con contenido se escribe solo en su
				// primera posición; las filas vacías repetidas solo avanzan el índice.
				if !parseODSRow(dec, &sd, row+1, ctx, validas, colDef) {
					for len(filaGrupos) > 0 {
						filaGrupos = cerrarGrupoODS(filaGrupos, row, &sd.RowGroups)
					}
					if f := ultimoCierreXML(raw, "table-row"); f > 0 {
						cola := saltarCierresXML(raw[f:])
						if bytes.Contains(cola, []byte("conditional-formats")) || bytes.Contains(cola, []byte("named-expressions")) {
							dec = ctx.presupuesto.decodificador(cola)
							continue
						}
					}
					return fin()
				}
				row += rep
			} else if e.Name.Local == "table-column" {
				rep := repeticionesODS(attrLocal(e, "number-columns-repeated"))
				if m := ctx.estilos.medida(attrLocal(e, "style-name")); m.ancho > 0 {
					for k := 0; k < min(rep, maxColumnasConAncho) && col+k < maxColumnasHoja; k++ {
						sd.fijarAncho(col+k, m.ancho)
					}
				}
				if d := attrLocal(e, "default-cell-style-name"); d != "" && len(colDef) < maxColumnasConAncho {
					colDef = append(colDef, tramoEstiloODS{col, col + rep, d})
				}
				col += rep
				_ = dec.Skip()
			} else if e.Name.Local == "table-row-group" {
				filaGrupos = append(filaGrupos, grupoAbierto{row, attrLocal(e, "display") == "false"})
			} else if e.Name.Local == "table-column-group" {
				colGrupos = append(colGrupos, grupoAbierto{col, attrLocal(e, "display") == "false"})
			} else if esContenedorODS[e.Name.Local] {
				continue // grupos y cabeceras de filas y columnas: lo de dentro es de la tabla
			} else if e.Name.Local == "scenario" {
				sd.escenarioODS = &escenarioODS{rangos: attrLocal(e, "scenario-ranges")}
			} else if e.Name.Local == "conditional-formats" {
				sd.CondFormats = append(sd.CondFormats, leerFormatosCondicionalesODS(dec, ctx)...)
			} else if n, ok := nombreDeODS(e, name); ok {
				*names = append(*names, n)
			} else if e.Name.Local != "named-expressions" { // ese se recorre: dentro van los nombres
				_ = dec.Skip()
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "table":
				return fin()
			case "table-row-group":
				filaGrupos = cerrarGrupoODS(filaGrupos, row, &sd.RowGroups)
			case "table-column-group":
				colGrupos = cerrarGrupoODS(colGrupos, col, &sd.ColGroups)
			}
		}
	}
}

// grupoAbierto es un `table:table-row-group` o `table-column-group` sin cerrar: dónde
// empezó (0-based) y si está colapsado.
type grupoAbierto struct {
	inicio    int
	colapsado bool
}

// cerrarGrupoODS cierra el grupo más interno y lo añade al esquema; `siguiente` es la
// primera unidad que ya no es suya. El nivel es la profundidad (1 = el más externo).
func cerrarGrupoODS(pila []grupoAbierto, siguiente int, destino *[]sheetGroup) []grupoAbierto {
	if len(pila) == 0 {
		return pila
	}
	g := pila[len(pila)-1]
	pila = pila[:len(pila)-1]
	if siguiente-1 >= g.inicio && len(*destino) < maxGruposHoja {
		*destino = append(*destino, sheetGroup{Start: g.inicio, End: siguiente - 1, Level: len(pila) + 1, Collapsed: g.colapsado})
	}
	return pila
}

// tramoEstiloODS es el estilo por defecto de las columnas [desde, hasta).
type tramoEstiloODS struct {
	desde, hasta int
	estilo       string
}

func estiloDeColumna(tramos []tramoEstiloODS, col int) string {
	for _, t := range tramos {
		if col >= t.desde && col < t.hasta {
			return t.estilo
		}
	}
	return ""
}

// esContenedorODS son los elementos que solo agrupan filas o columnas (esquema, cabeceras
// repetidas): no se saltan, porque sus filas y columnas son de la tabla.
var esContenedorODS = map[string]bool{
	"table-row-group": true, "table-header-rows": true, "table-rows": true,
	"table-column-group": true, "table-header-columns": true, "table-columns": true,
}

// maxFilasConAlto acota las filas a las que se expande un `number-rows-repeated` con alto.
const maxFilasConAlto = 1024

// parseODSRow lee una <table:table-row> escribiendo sus celdas en la fila dada
// (1-based). Devuelve false si la fila cae fuera del recorte y tiene contenido.
//
//nolint:gocognit,gocyclo // traductor de un formato a otro: un caso por tipo, y trocearlo reparte la tabla sin simplificarla
func parseODSRow(dec *xml.Decoder, sd *sheetData, rowNum int, ctx *odsCtx, validas map[string]string, colDef []tramoEstiloODS) bool {
	col := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return true
		}
		switch e := tok.(type) {
		case xml.StartElement:
			if e.Name.Local == "table-cell" || e.Name.Local == "covered-table-cell" {
				rep := repeticionesODS(attrLocal(e, "number-columns-repeated"))
				nombreEstilo := attrLocal(e, "style-name")
				validacion := attrLocal(e, "content-validation-name")
				cs, rs := repeticionesODS(attrLocal(e, "number-columns-spanned")), repeticionesODS(attrLocal(e, "number-rows-spanned"))
				combinada := (cs > 1 || rs > 1) && rep == 1 && len(sd.Merges) < maxCombinadas
				text, tipo := odsCellText(dec, e)
				if nombreEstilo == "" && text != "" {
					// Calc escribe el formato de una columna entera una sola vez y la celda lo hereda.
					nombreEstilo = estiloDeColumna(colDef, col)
				}
				estilo := ctx.estilos.visual(nombreEstilo)
				spec := ctx.formatos[estilo.datos]
				if spec == nil && text != "" {
					spec = formatoPorTipoODS(tipo)
				}
				escribe := text != "" || (rep <= maxRepeticionVacia && (estilo.visible() || validacion != ""))
				if (combinada || escribe) && ctx.presupuesto.recorta(rowNum) {
					sd.recortada = true
					return false
				}
				if combinada {
					sd.Merges = append(sd.Merges, rango{c1: col + 1, r1: rowNum, c2: col + cs, r2: rowNum + rs - 1}.a1())
				}
				if escribe {
					for k := 0; k < min(rep, 1024) && col+k < maxColumnasHoja && rowNum <= maxFilasHoja; k++ {
						ref := A1Ref(rowNum, col+k+1)
						if text != "" {
							sd.Cells[ref] = envolverCelda(html.EscapeString(text), estilo.negrita, estilo.cursiva, estilo.alinear)
							ctx.presupuesto.gastar(len(ref) + len(sd.Cells[ref]))
						}
						if estilo.visible() && len(sd.Styles) < maxEstilosHoja {
							if sd.Styles == nil {
								sd.Styles = map[string]cellStyle{}
							}
							sd.Styles[ref] = estilo.estilo
						}
						if spec != nil && text != "" {
							if sd.Formats == nil {
								sd.Formats = map[string]numFmtSpec{}
							}
							sd.Formats[ref] = *spec
						}
						if validacion != "" && len(validas) < maxCeldasVacias*4 {
							validas[ref] = validacion
						}
					}
				}
				col += rep
			} else {
				_ = dec.Skip()
			}
		case xml.EndElement:
			if e.Name.Local == "table-row" {
				return true
			}
		}
	}
}

// partirCuerpoODS separa cada <table:table>…</table:table> de <office:body> y devuelve
// además el cuerpo sin ellas (validaciones, filtros, nombres del libro y tablas dinámicas).
func partirCuerpoODS(content []byte) (tablas [][]byte, resto []byte) {
	pos := max(inicioElementoXML(content, "body"), 0)
	cuerpo := content[pos:]
	var fuera [][]byte
	for len(cuerpo) > 0 {
		ini := inicioElementoXML(cuerpo, "table")
		if ini < 0 {
			fuera = append(fuera, cuerpo)
			break
		}
		if ini > 0 {
			fuera = append(fuera, cuerpo[:ini])
		}
		cuerpo = cuerpo[ini:]
		finApertura := bytes.IndexByte(cuerpo, '>')
		if finApertura < 0 {
			tablas = append(tablas, cuerpo)
			break
		}
		if finApertura > 0 && cuerpo[finApertura-1] == '/' {
			tablas = append(tablas, cuerpo[:finApertura+1])
			cuerpo = cuerpo[finApertura+1:]
			continue
		}
		fin := primerCierreXML(cuerpo[finApertura+1:], "table")
		if fin <= 0 {
			tablas = append(tablas, cuerpo)
			break
		}
		total := finApertura + 1 + fin
		tablas = append(tablas, cuerpo[:total])
		cuerpo = cuerpo[total:]
	}
	return tablas, bytes.Join(fuera, nil)
}

func esNombreXML(b byte) bool {
	return b == '_' || b == '-' || b == '.' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// inicioElementoXML devuelve el índice del `<` donde abre el primer elemento de nombre local
// `local` (con o sin prefijo de espacio de nombres), o -1 si no está.
func inicioElementoXML(raw []byte, local string) int {
	aguja := []byte(local)
	base := 0
	for {
		i := bytes.Index(raw[base:], aguja)
		if i < 0 {
			return -1
		}
		i += base
		fin := i + len(aguja)
		if fin < len(raw) {
			switch raw[fin] {
			case ' ', '\t', '\r', '\n', '/', '>':
				if ini := prefijoEtiquetaXML(raw, i, false); ini >= 0 {
					return ini
				}
			}
		}
		base = i + 1
	}
}

// primerCierreXML devuelve el índice tras el `>` del primer cierre `</…:local>` o `</local>`, o 0.
func primerCierreXML(raw []byte, local string) int {
	aguja := []byte(local)
	base := 0
	for {
		i := bytes.Index(raw[base:], aguja)
		if i < 0 {
			return 0
		}
		i += base
		if fin := finDeCierreXML(raw, i, len(aguja)); fin > 0 && prefijoEtiquetaXML(raw, i, true) >= 0 {
			return fin
		}
		base = i + 1
	}
}

// ultimoCierreXML devuelve el índice tras el `>` del último cierre `</…:local>` o `</local>`, o 0.
func ultimoCierreXML(raw []byte, local string) int {
	aguja := []byte(local)
	hasta := len(raw)
	for {
		i := bytes.LastIndex(raw[:hasta], aguja)
		if i < 0 {
			return 0
		}
		if fin := finDeCierreXML(raw, i, len(aguja)); fin > 0 && prefijoEtiquetaXML(raw, i, true) >= 0 {
			return fin
		}
		hasta = i
	}
}

func finDeCierreXML(raw []byte, i, n int) int {
	k := i + n
	for k < len(raw) && (raw[k] == ' ' || raw[k] == '\t' || raw[k] == '\r' || raw[k] == '\n') {
		k++
	}
	if k < len(raw) && raw[k] == '>' {
		return k + 1
	}
	return 0
}

func prefijoEtiquetaXML(raw []byte, i int, cierre bool) int {
	if i < 1 {
		return -1
	}
	j := i - 1
	if raw[j] == ':' {
		j--
		if j < 0 || !esNombreXML(raw[j]) {
			return -1
		}
		for j >= 0 && esNombreXML(raw[j]) {
			j--
		}
	}
	if cierre {
		if j >= 1 && raw[j] == '/' && raw[j-1] == '<' {
			return j - 1
		}
		return -1
	}
	if j >= 0 && raw[j] == '<' {
		return j
	}
	return -1
}

// saltarCierresXML descarta etiquetas de cierre sueltas al comienzo de `raw` (p. ej.
// `</table:table-row-group>` o `</table:table>`) para que un decodificador nuevo solo vea
// elementos completos.
func saltarCierresXML(raw []byte) []byte {
	for {
		s := bytes.TrimLeft(raw, " \t\r\n")
		if !bytes.HasPrefix(s, []byte("</")) {
			return s
		}
		i := bytes.IndexByte(s, '>')
		if i < 0 {
			return nil
		}
		raw = s[i+1:]
	}
}

// repeticionesODS lee un `number-*-repeated`: al menos 1 y, aunque el fichero diga una
// barbaridad, sin desbordar el contador de filas o columnas.
func repeticionesODS(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 {
		return 1
	}
	return min(n, 1<<24)
}

// maxRepeticionVacia es la repetición máxima de una celda vacía que se escribe por su
// estilo: LibreOffice rellena el resto de una fila con una sola celda repetida miles de
// veces, y eso no es formato de nadie.
const maxRepeticionVacia = 64

// odsCellText resuelve el texto crudo de una celda ODS: fórmula (prefijo "="),
// valor numérico (office:value) o texto (<text:p>). Consume el elemento celda.
func odsCellText(dec *xml.Decoder, se xml.StartElement) (string, string) {
	formula := attrLocal(se, "formula")
	valueType := attrLocal(se, "value-type")
	officeValue := attrLocal(se, "value")
	boolValue := attrLocal(se, "boolean-value")
	fecha := attrLocal(se, "date-value")
	if valueType == "time" {
		fecha = attrLocal(se, "time-value")
	}

	// Texto interno de los <text:p> de la celda (para tipo string).
	var text strings.Builder
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.CharData:
			if depth > 0 {
				text.Write(t)
			}
		case xml.StartElement:
			if t.Name.Local == "p" {
				if text.Len() > 0 {
					text.WriteByte('\n')
				}
				depth++
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "p":
				if depth > 0 {
					depth--
				}
			case "table-cell", "covered-table-cell":
				return odsResolveValue(formula, valueType, officeValue, boolValue, fecha, text.String()), valueType
			}
		}
	}
	return odsResolveValue(formula, valueType, officeValue, boolValue, fecha, text.String()), valueType
}

func odsResolveValue(formula, valueType, officeValue, boolValue, fecha, text string) string {
	if strings.TrimSpace(formula) != "" {
		return "=" + odfFormulaToA1(formula)
	}
	switch valueType {
	case "float", "percentage", "currency":
		if officeValue != "" {
			return officeValue
		}
	case "date", "time":
		if n, ok := serialDeODS(valueType, fecha); ok {
			return textoSerial(n)
		}
	case "boolean":
		if boolValue == "true" {
			return "TRUE"
		}
		if boolValue == "false" {
			return "FALSE"
		}
	}
	return strings.TrimSpace(text)
}

var reODFRef = regexp.MustCompile(`\[([^\]]*)\]`)

// odfFormulaToA1 convierte una fórmula OpenFormula (of:=[.A1]+SUM([.B1:.B3])) a
// estilo A1 (A1+SUM(B1:B3)). Reverso de a1FormulaToODF (export_sheet.go).
func odfFormulaToA1(f string) string {
	f = strings.TrimSpace(f)
	f = strings.TrimPrefix(f, "of:")
	f = strings.TrimPrefix(f, "=")
	return reODFRef.ReplaceAllStringFunc(f, func(m string) string {
		inner := m[1 : len(m)-1] // sin corchetes
		parts := strings.Split(inner, ":")
		for i, p := range parts {
			parts[i] = odfRefToA1(p)
		}
		return strings.Join(parts, ":")
	})
}

// odfRefToA1 convierte una referencia OpenFormula (".A1", "Hoja.A1", "$Hoja.$A$1" o
// "$'Mi hoja'.A1") a A1 ("A1", "Hoja!A1", "Hoja!$A$1" o "'Mi hoja'!A1"). El «$» de la
// hoja (hoja absoluta) no tiene equivalente en A1 y se quita; las comillas se ponen
// como las lee el editor.
func odfRefToA1(ref string) string {
	dot := strings.LastIndexByte(ref, '.')
	if dot < 0 {
		return ref
	}
	sheet := sinComillas(strings.TrimPrefix(ref[:dot], "$"))
	cell := ref[dot+1:]
	if sheet == "" {
		return cell
	}
	return hojaEditor(sheet) + "!" + cell
}

// formatoPorTipoODS es el formato de una celda cuyo estilo no trae uno (o no se entiende) y
// que ODF tipa como fecha, moneda o porcentaje.
func formatoPorTipoODS(tipo string) *numFmtSpec {
	switch tipo {
	case "date":
		return &numFmtSpec{ID: "date"}
	case "currency": // LibreOffice no escribe estilo para la moneda por defecto
		return &numFmtSpec{ID: "currency", Decimals: ptr(2)}
	case "percentage":
		return &numFmtSpec{ID: "percent", Decimals: ptr(0)}
	}
	return nil
}

// textoSerial es un número de serie como el texto crudo de una celda («46096», «45000.5»).
func textoSerial(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) }

// --- Estilos de celda de ODF ---

// odsVisual es un estilo de celda ya traducido al modelo del editor.
type odsVisual struct {
	estilo           cellStyle
	negrita, cursiva bool
	alinear          string
	datos            string // nombre del estilo de datos (el formato de número)
}

// visible dice si el estilo trae algo que el editor guarde en `styles` (la negrita, la
// cursiva y la alineación viajan en el HTML de la celda, y el formato de número aparte).
func (v odsVisual) visible() bool { return !v.estilo.vacio() }

// odsEstilos son los estilos de celda de un documento ODF.
type odsEstilos struct {
	crudo   map[string]*odsEstiloCrudo
	alias   map[string]string // style:display-name -> style:name
	cache   map[string]odsVisual
	medidas map[string]*odsMedida // estilos de columna y de fila
	// pestanas es el color de la pestaña de cada estilo de tabla (`tab-color`).
	pestanas map[string]string
}

// odsMedida es el ancho de un estilo de columna o el alto de uno de fila, en px; `optimo`
// dice que el alto lo calcula la aplicación (no es del usuario).
type odsMedida struct {
	ancho, alto float64
	optimo      bool
}

// medida es la de un estilo de columna o de fila; la vacía si no hay.
func (e *odsEstilos) medida(nombre string) odsMedida {
	if e == nil || e.medidas[nombre] == nil {
		return odsMedida{}
	}
	return *e.medidas[nombre]
}

type odsEstiloCrudo struct {
	padre, datos string
	props        map[string]string
}

// parseODSEstilos lee los `style:style` de los documentos dados: los de celda, el ancho y el
// alto de los de columna y de fila, y la pestaña de los de tabla.
//
//nolint:gocognit,gocyclo // traductor de un formato a otro: un caso por tipo, y trocearlo reparte la tabla sin simplificarla
func parseODSEstilos(docs ...[]byte) *odsEstilos {
	e := &odsEstilos{crudo: map[string]*odsEstiloCrudo{}, alias: map[string]string{}, cache: map[string]odsVisual{}, medidas: map[string]*odsMedida{}, pestanas: map[string]string{}}
	for _, doc := range docs {
		dec := xml.NewDecoder(bytes.NewReader(doc))
		var actual *odsEstiloCrudo
		var medida *odsMedida
		tabla := "" // el estilo de tabla abierto
		for fin := false; !fin; {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			switch t := tok.(type) {
			case xml.StartElement:
				switch t.Name.Local {
				case "body":
					fin = true // los estilos automáticos van siempre antes de <office:body>
				case "style":
					actual, medida, tabla = nil, nil, ""
					if attrLocal(t, "family") == "table" {
						tabla = attrLocal(t, "name")
						continue
					}
					if fam := attrLocal(t, "family"); fam == "table-column" || fam == "table-row" {
						medida = &odsMedida{}
						e.medidas[attrLocal(t, "name")] = medida
						continue
					}
					if attrLocal(t, "family") != "table-cell" || len(e.crudo) >= maxEstilosHoja {
						continue
					}
					nombre := attrLocal(t, "name")
					actual = &odsEstiloCrudo{padre: attrLocal(t, "parent-style-name"), datos: attrLocal(t, "data-style-name"), props: map[string]string{}}
					e.crudo[nombre] = actual
					if d := attrLocal(t, "display-name"); d != "" {
						e.alias[d] = nombre
					}
				case "table-properties":
					// La pestaña: `tableooo:tab-color`, el que escribe LibreOffice, o el de ODF 1.3.
					if c := colorHex(attrLocal(t, "tab-color")); tabla != "" && c != "" && len(e.pestanas) < maxEstilosHoja {
						e.pestanas[tabla] = c
					}
				case "table-column-properties", "table-row-properties":
					if medida != nil {
						medida.ancho, _ = medidaValida(longitudPx(attrLocal(t, "column-width")), maxAnchoPx)
						medida.alto, _ = medidaValida(longitudPx(attrLocal(t, "row-height")), maxAltoPx)
						medida.optimo = attrLocal(t, "use-optimal-row-height") == "true"
					}
				case "table-cell-properties", "text-properties", "paragraph-properties":
					if actual != nil {
						for _, a := range t.Attr {
							actual.props[a.Name.Local] = a.Value
						}
					}
				}
			case xml.EndElement:
				if t.Name.Local == "style" {
					actual, medida, tabla = nil, nil, ""
				}
			}
		}
	}
	return e
}

// propiedades son las propiedades de un estilo con las de sus padres debajo.
func (e *odsEstilos) propiedades(nombre string, prof int) (map[string]string, string) {
	if e == nil {
		return nil, ""
	}
	if n, ok := e.alias[nombre]; ok && e.crudo[nombre] == nil {
		nombre = n
	}
	c := e.crudo[nombre]
	if c == nil || prof > 8 {
		return map[string]string{}, ""
	}
	props, datos := e.propiedades(c.padre, prof+1)
	out := make(map[string]string, len(props)+len(c.props))
	for k, v := range props {
		out[k] = v
	}
	for k, v := range c.props {
		out[k] = v
	}
	if c.datos != "" {
		datos = c.datos
	}
	return out, datos
}

// ladoDeODS lee un `fo:border*` de ODF («0.74pt solid #000000»): si el lado se ve, su trazo
// del editor (por el estilo y el grosor: desde 1,25 pt es «medium» y desde 2,25, «thick», los
// cortes de Calc entre fino, medio y grueso) y su color. El negro es el de serie.
func ladoDeODS(v string) (bool, string, string) {
	v = strings.TrimSpace(v)
	if v == "" || v == "none" || strings.HasPrefix(v, "none ") || strings.HasPrefix(v, "hidden") {
		return false, "", ""
	}
	estilo, color, pt := "solid", "", 0.0
	for _, t := range strings.Fields(v) {
		switch {
		case strings.HasPrefix(t, "#"):
			color = colorHex(t)
		case t == "dashed" || t == "dotted" || t == "double" || t == "solid":
			estilo = t
		default:
			if px := longitudPx(t); px > 0 {
				pt = px * 0.75
			}
		}
	}
	if color == "#000000" {
		color = ""
	}
	trazo := ""
	switch {
	case estilo != "solid":
		trazo = estilo
	case pt >= 2.25:
		trazo = "thick"
	case pt >= 1.25:
		trazo = "medium"
	}
	return true, trazo, color
}

func longitudPt(s string) int {
	s = strings.TrimSuffix(strings.TrimSpace(s), "pt")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int(f + 0.5)
}

// visual es el estilo de celda por su nombre, ya traducido; el de una celda sin estilo es
// el vacío.
//
//nolint:gocognit,gocyclo // traductor de un formato a otro: un caso por tipo, y trocearlo reparte la tabla sin simplificarla
func (e *odsEstilos) visual(nombre string) odsVisual {
	if e == nil || nombre == "" {
		return odsVisual{}
	}
	if v, ok := e.cache[nombre]; ok {
		return v
	}
	p, datos := e.propiedades(nombre, 0)
	base, _ := e.propiedades("Default", 0)
	v := odsVisual{datos: datos}
	if bg := p["background-color"]; bg != "transparent" {
		v.estilo.Fill = colorHex(bg)
	}
	if c := colorHex(p["color"]); c != "#000000" {
		v.estilo.Color = c
	}
	v.negrita = p["font-weight"] == "bold" || (p["font-weight"] != "" && atoiDefault(p["font-weight"], 0) >= 600)
	v.cursiva = p["font-style"] == "italic" || p["font-style"] == "oblique"
	v.estilo.U = p["text-underline-style"] != "" && p["text-underline-style"] != "none"
	v.estilo.S = p["text-line-through-style"] != "" && p["text-line-through-style"] != "none"
	nombreFuente := func(m map[string]string) string {
		if m["font-name"] != "" {
			return m["font-name"]
		}
		return m["font-family"]
	}
	if f := nombreFuente(p); f != "" && !strings.EqualFold(f, nombreFuente(base)) {
		for _, fam := range fuentesDelEditor {
			if strings.EqualFold(strings.Trim(f, `'"`), fam) {
				v.estilo.Font = fam
			}
		}
	}
	if t := longitudPt(p["font-size"]); t > 0 && t != longitudPt(base["font-size"]) {
		v.estilo.Size = min(96, max(6, t))
	}
	lado := func(k string) (bool, string, string) {
		b, ok := p["border-"+k]
		if !ok {
			b = p["border"]
		}
		return ladoDeODS(b)
	}
	var b cellBorder
	b.T, b.TS, b.TC = lado("top")
	b.R, b.RS, b.RC = lado("right")
	b.B, b.BS, b.BC = lado("bottom")
	b.L, b.LS, b.LC = lado("left")
	if b != (cellBorder{}) {
		v.estilo.Border = &b
	}
	v.estilo.Valign = map[string]string{"top": "top", "middle": "middle", "bottom": "bottom"}[p["vertical-align"]]
	v.estilo.Wrap = p["wrap-option"] == "wrap"
	// Protección de celda: por omisión protegida; «none» y «formula-hidden» la desbloquean.
	switch cp := p["cell-protect"]; {
	case cp == "none" || cp == "formula-hidden":
		no := false
		v.estilo.Locked = &no
		v.estilo.Hidden = cp == "formula-hidden"
	case strings.Contains(cp, "hidden"):
		v.estilo.Hidden = true
	}
	switch p["text-align"] {
	case "center":
		v.alinear = "center"
	case "end", "right":
		v.alinear = "right"
	}
	e.cache[nombre] = v
	return v
}

// --- Validaciones de ODF ---

var (
	reODSTipoNum = regexp.MustCompile(`^cell-content-is-(whole-number|decimal-number|date)\(\)\s+and\s+(.*)$`)
	reODSCmp     = regexp.MustCompile(`^(cell-content|cell-content-text-length)\(\)\s*(>=|<=|!=|<>|>|<|=)\s*(.+)$`)
	reODSEntre   = regexp.MustCompile(`^(cell-content-is-between|cell-content-is-not-between|cell-content-text-length-is-between|cell-content-text-length-is-not-between)\((.*)\)$`)
	reODSLista   = regexp.MustCompile(`^cell-content-is-in-list\((.*)\)$`)
	reODSFormula = regexp.MustCompile(`^is-true-formula\((.*)\)$`)
	reODSCadenas = regexp.MustCompile(`"((?:[^"]|"")*)"`)
	opDeODS      = map[string]string{">": "gt", ">=": "ge", "<": "lt", "<=": "le", "=": "eq", "!=": "ne", "<>": "ne"}
)

// argumentosODS parte los argumentos de una función de OpenFormula por `;` (LibreOffice a
// veces escribe `,`: un decimal usa siempre punto, así que no hay ambigüedad).
func argumentosODS(s string) []string {
	sep := ";"
	if !strings.Contains(s, ";") {
		sep = ","
	}
	partes := strings.Split(s, sep)
	for i := range partes {
		partes[i] = strings.TrimSpace(partes[i])
	}
	return partes
}

// validacionDeODS pasa un `table:condition` al modelo; false si no tiene equivalente.
func validacionDeODS(cond string) (dataValidation, bool) {
	c := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(cond), "of:"))
	var v dataValidation
	if m := reODSLista.FindStringSubmatch(c); m != nil {
		if cs := reODSCadenas.FindAllStringSubmatch(m[1], -1); len(cs) > 0 && !strings.Contains(reODSCadenas.ReplaceAllString(m[1], ""), "[") {
			for _, x := range cs {
				v.Values = append(v.Values, strings.ReplaceAll(x[1], `""`, `"`))
			}
			return v, true
		}
		v.listaDe = odfFormulaToA1(m[1]) // un rango («[.A1:.A9]»)
		return v, strings.Contains(m[1], ".")
	}
	if m := reODSFormula.FindStringSubmatch(c); m != nil {
		v.Type, v.Formula = "formula", "="+odfFormulaToA1(m[1])
		return v, true
	}
	tipo := ""
	if m := reODSTipoNum.FindStringSubmatch(c); m != nil {
		tipo, c = m[1], strings.TrimSpace(m[2])
	}
	v.Type = map[string]string{"": "number", "whole-number": "number", "decimal-number": "number", "date": "date"}[tipo]
	v.Integer = tipo == "whole-number"
	operando := func(s string) (string, bool) { return operandoDeValidacion(v.Type, s) }
	if m := reODSEntre.FindStringSubmatch(c); m != nil {
		args := argumentosODS(m[2])
		if len(args) != 2 {
			return v, false
		}
		if strings.Contains(m[1], "text-length") {
			v.Type = "textLength"
		}
		v.Op = "between"
		if strings.Contains(m[1], "not-between") {
			v.Op = "notBetween"
		}
		var ok1, ok2 bool
		v.A, ok1 = operando(args[0])
		v.B, ok2 = operando(args[1])
		return v, ok1 && ok2
	}
	if m := reODSCmp.FindStringSubmatch(c); m != nil {
		if m[1] == "cell-content-text-length" {
			v.Type = "textLength"
		}
		v.Op = opDeODS[m[2]]
		var ok bool
		v.A, ok = operando(m[3])
		return v, ok
	}
	return v, false
}

// leerValidacionesODS lee los `table:content-validation` hasta el cierre de su contenedor.
func leerValidacionesODS(dec *xml.Decoder, ctx *odsCtx) {
	var nombre string
	var v dataValidation
	var ok bool
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "content-validation":
				nombre = attrLocal(t, "name")
				v, ok = validacionDeODS(attrLocal(t, "condition"))
				v.Strict = false
			case "error-message":
				// LibreOffice enseña el aviso; solo `stop` rechaza.
				v.Strict = attrLocal(t, "message-type") == "stop" && attrLocal(t, "display") != "false"
				v.Error = textoHijoODS(dec)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "content-validation":
				if ok && nombre != "" && len(ctx.validaciones) < maxValidacionesHoja {
					ctx.validaciones[nombre] = v
				}
			case "content-validations":
				return
			}
		}
	}
}

// textoHijoODS lee el texto de los `text:p` hasta cerrar el elemento actual (que ya se abrió).
func textoHijoODS(dec *xml.Decoder) string {
	var b strings.Builder
	prof := 1
	for prof > 0 {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			prof++
		case xml.EndElement:
			prof--
		case xml.CharData:
			b.Write(t)
		}
	}
	return strings.TrimSpace(b.String())
}

// validacionesDeCeldas agrupa las celdas que cubre cada validación en rangos: por filas y
// después uniendo las filas contiguas con el mismo tramo.
func validacionesDeCeldas(por map[string]string, plantillas map[string]dataValidation) []dataValidation {
	if len(por) == 0 {
		return nil
	}
	type pos struct{ r, c int }
	porNombre := map[string][]pos{}
	for ref, n := range por {
		if _, ok := plantillas[n]; !ok {
			continue
		}
		if r, c, ok := ParseA1(ref); ok {
			porNombre[n] = append(porNombre[n], pos{r, c})
		}
	}
	nombres := make([]string, 0, len(porNombre))
	for n := range porNombre {
		nombres = append(nombres, n)
	}
	slices.Sort(nombres)
	var out []dataValidation
	for _, n := range nombres {
		celdas := porNombre[n]
		slices.SortFunc(celdas, func(a, b pos) int {
			if a.r != b.r {
				return a.r - b.r
			}
			return a.c - b.c
		})
		type tramo struct{ r1, r2, c1, c2 int }
		var tramos []tramo
		for i := 0; i < len(celdas); {
			j := i
			for j+1 < len(celdas) && celdas[j+1].r == celdas[i].r && celdas[j+1].c == celdas[j].c+1 {
				j++
			}
			t := tramo{celdas[i].r, celdas[i].r, celdas[i].c, celdas[j].c}
			if k := len(tramos) - 1; k >= 0 && tramos[k].c1 == t.c1 && tramos[k].c2 == t.c2 && tramos[k].r2 == t.r1-1 {
				tramos[k].r2 = t.r1
			} else {
				tramos = append(tramos, t)
			}
			i = j + 1
		}
		for _, t := range tramos {
			if len(out) >= maxValidacionesHoja {
				return out
			}
			v := plantillas[n]
			v.Range = rango{c1: t.c1, r1: t.r1, c2: t.c2, r2: t.r2}.a1()
			out = append(out, v)
		}
	}
	return out
}

// --- Formato condicional de ODF ---

// rangosDeDireccionODS lee una dirección de ODF (`Hoja.B2:Hoja.B4`, varias separadas por espacios).
func rangosDeDireccionODS(s string) []rango {
	var out []rango
	for _, d := range direccionesODS(s) {
		var celdas []string
		for _, p := range strings.Split(d, ":") {
			celdas = append(celdas, strings.ReplaceAll(p[strings.LastIndexByte(p, '.')+1:], "$", ""))
		}
		if r, ok := leerRango(strings.Join(celdas, ":")); ok && r.valido() {
			out = append(out, r)
		}
	}
	return out
}

// direccionesODS parte por espacios fuera de comillas.
func direccionesODS(s string) []string {
	var out []string
	var b strings.Builder
	q := false
	for _, r := range s {
		switch {
		case r == '\'':
			q = !q
			b.WriteRune(r)
		case r == ' ' && !q:
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

var (
	reODSOperador = regexp.MustCompile(`^(>=|<=|!=|<>|>|<|=)\s*(.+)$`)
	reODSFuncion  = regexp.MustCompile(`^([a-z-]+)\((.*)\)$`)
)

// operandoCondicionODS es un operando de una condición como texto del editor: un número, un
// texto entre comillas; false si es una referencia o una fórmula.
func operandoCondicionODS(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if m := reODSCadenas.FindStringSubmatch(s); len(m) > 1 && m[0] == s {
		return strings.ReplaceAll(m[1], `""`, `"`), true
	}
	if n, ok := parseNumEs(strings.ReplaceAll(s, ",", ".")); ok && !strings.ContainsAny(s, "[]") {
		return textoNumEs(n), true
	}
	return "", false
}

// reglaDeCondicionODS pasa un `calcext:condition` que pinta (con `pinta`, su relleno y su
// letra) al modelo; false si no tiene equivalente.
func reglaDeCondicionODS(valor string, rg rango, pinta condRule) (condRule, bool) {
	r := condRule{Range: rg.a1(), Fill: pinta.Fill, TextColor: pinta.TextColor}
	celda := A1Ref(rg.r1, rg.c1)
	v := strings.TrimSpace(valor)
	if m := reODSOperador.FindStringSubmatch(v); m != nil {
		op := opDeODS[m[1]]
		if o, ok := operandoCondicionODS(m[2]); ok {
			r.Op, r.Value = op, o
			return r, true
		}
		r.Kind, r.Formula = "formula", "="+celda+simboloOp[op]+odfFormulaToA1(m[2])
		return r, true
	}
	switch v {
	case "duplicate":
		r.Kind = "dup"
		return r, true
	case "unique":
		r.Kind = "unique"
		return r, true
	}
	m := reODSFuncion.FindStringSubmatch(v)
	if m == nil {
		return r, false
	}
	args := m[2]
	texto := func() (string, bool) { return operandoCondicionODS(args) }
	switch m[1] {
	case "contains-text":
		t, ok := texto()
		r.Op, r.Value = "contains", t
		return r, ok && t != ""
	case "begins-with", "ends-with", "not-contains-text":
		t, ok := texto()
		if !ok {
			return r, false
		}
		q := `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
		r.Kind = "formula"
		switch m[1] {
		case "begins-with":
			r.Formula = "=LEFT(" + celda + ",LEN(" + q + "))=" + q
		case "ends-with":
			r.Formula = "=RIGHT(" + celda + ",LEN(" + q + "))=" + q
		default:
			r.Formula = "=ISERROR(SEARCH(" + q + "," + celda + "))"
		}
		return r, true
	case "formula-is", "is-true-formula":
		r.Kind, r.Formula = "formula", "="+odfFormulaToA1(args)
		return r, true
	case "between", "not-between":
		a := argumentosODS(args)
		if len(a) != 2 {
			return r, false
		}
		x, y := odfFormulaToA1(a[0]), odfFormulaToA1(a[1])
		r.Kind = "formula"
		if m[1] == "between" {
			r.Formula = "=AND(" + celda + ">=" + x + "," + celda + "<=" + y + ")"
		} else {
			r.Formula = "=OR(" + celda + "<" + x + "," + celda + ">" + y + ")"
		}
		return r, true
	case "top-elements", "bottom-elements", "top-percent", "bottom-percent":
		n, err := strconv.Atoi(strings.TrimSpace(args))
		if err != nil || n <= 0 {
			return r, false
		}
		r.Kind = strings.SplitN(m[1], "-", 2)[0]
		r.Percent = strings.HasSuffix(m[1], "percent")
		r.Rank = min(n, map[bool]int{true: 100, false: 1000}[r.Percent])
		return r, true
	}
	return r, false
}

// leerFormatosCondicionalesODS lee los `calcext:conditional-format` hasta el cierre del
// contenedor.
//
//nolint:gocognit,gocyclo // traductor de un formato a otro: un caso por tipo, y trocearlo reparte la tabla sin simplificarla
func leerFormatosCondicionalesODS(dec *xml.Decoder, ctx *odsCtx) []condRule {
	var out []condRule
	var rangos []rango
	var escala []string
	var iconos string
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "conditional-format":
				rangos = rangosDeDireccionODS(attrLocal(t, "target-range-address"))
				escala, iconos = nil, ""
			case "condition":
				e := ctx.estilos.visual(attrLocal(t, "apply-style-name")).estilo
				pinta := condRule{Fill: e.Fill, TextColor: e.Color}
				if pinta.Fill == "" && pinta.TextColor == "" {
					break // no pinta nada: no tiene equivalente
				}
				for _, rg := range rangos {
					if r, ok := reglaDeCondicionODS(attrLocal(t, "value"), rg, pinta); ok && len(out) < maxReglasHoja {
						out = append(out, r)
					}
				}
			case "data-bar":
				if c := colorHex(attrLocal(t, "positive-color")); c != "" {
					for _, rg := range rangos {
						out = append(out, condRule{Range: rg.a1(), Kind: "dataBar", Color: c})
					}
				}
			case "color-scale-entry":
				if c := colorHex(attrLocal(t, "color")); c != "" {
					escala = append(escala, c)
				}
			case "icon-set":
				for _, i := range iconosDeXlsx {
					if strings.Contains(attrLocal(t, "icon-set-type"), i.trozo) {
						iconos = i.id
					}
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "color-scale":
				if len(escala) == 2 || len(escala) == 3 {
					for _, rg := range rangos {
						out = append(out, condRule{Range: rg.a1(), Kind: "colorScale", Scale: slices.Clone(escala)})
					}
				}
			case "icon-set":
				if iconos != "" {
					for _, rg := range rangos {
						out = append(out, condRule{Range: rg.a1(), Kind: "iconSet", Icons: iconos})
					}
				}
			case "conditional-formats":
				return out
			}
		}
	}
}
