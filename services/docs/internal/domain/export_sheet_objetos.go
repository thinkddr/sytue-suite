// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"maps"
	"strconv"
	"strings"
)

// Exportación a OOXML de los objetos de una hoja (ver sheet_objetos.go):
//
//   - una TABLA con formato es un `xl/tables/tableN.xml` (ListObject) con su estilo, sus
//     columnas y su fila de totales, que en la hoja son fórmulas `SUBTOTAL` como las
//     escribe Excel;
//   - un MINIGRÁFICO es un `x14:sparklineGroup` en la extensión de la hoja;
//   - un GRÁFICO es un `xl/charts/chartN.xml` (DrawingML) anclado a su celda por un
//     `xl/drawings/drawingN.xml`, con todo lo que el editor sabe de él: tipo, series por
//     filas o columnas, apilado, eje secundario, títulos, leyenda, etiquetas, retícula,
//     colores y tamaño;
//   - las SEGMENTACIONES no tienen equivalente fuera de una tabla dinámica o una tabla
//     de Excel, así que viajan en una extensión propia (`urn:sytue:calcus`) que Excel y
//     Calc ignoran y que la importación lee.

const (
	nsMainX   = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	nsRelX    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsChart   = "http://schemas.openxmlformats.org/drawingml/2006/chart"
	nsDrawing = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsXdr     = "http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing"
	nsX14     = "http://schemas.microsoft.com/office/spreadsheetml/2009/9/main"
	nsXm      = "http://schemas.microsoft.com/office/excel/2006/main"
	nsSytue   = "urn:sytue:calcus"

	uriSparklines = "{05C60535-1F16-4fd2-B633-F4F36F0B64E0}"
	uriSytue      = "{7A1F9C2E-5B3D-4E8A-9C0F-5359545545A1}"

	relTabla   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/table"
	relDibujo  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/drawing"
	relGrafico = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart"

	ctTabla   = "application/vnd.openxmlformats-officedocument.spreadsheetml.table+xml"
	ctDibujo  = "application/vnd.openxmlformats-officedocument.drawing+xml"
	ctGrafico = "application/vnd.openxmlformats-officedocument.drawingml.chart+xml"
)

// estiloTablaExcel es el estilo de tabla de Excel más parecido a cada uno del editor, y
// la vuelta: los cinco van y vuelven sin perderse.
var estiloTablaExcel = map[string]string{
	"slate": "TableStyleMedium1", "blue": "TableStyleMedium2", "green": "TableStyleMedium7",
	"amber": "TableStyleMedium3", "plain": "TableStyleLight1",
}

// funcionTotalExcel es `totalsRowFunction` y el primer argumento de SUBTOTAL de cada
// función de la fila de totales. OJO: el `count` de Excel es CONTAR.A (no vacías) y su
// `countNums` es CONTAR (solo números), al revés de como suenan.
var funcionTotalExcel = map[string]struct {
	nombre   string
	subtotal int
}{
	"sum": {"sum", 109}, "average": {"average", 101}, "count": {"countNums", 102},
	"counta": {"count", 103}, "min": {"min", 105}, "max": {"max", 104},
}

// partesObjetos es lo que los objetos de una hoja añaden al paquete.
type partesObjetos struct {
	colaHoja  strings.Builder   // lo que va en la hoja tras <sheetData>: drawing, tableParts y extLst
	rels      []string          // las <Relationship> de la hoja
	partes    map[string]string // ruta → contenido
	overrides []string          // los <Override> de [Content_Types].xml
}

// contadorObjetos numera tablas, dibujos y gráficos en todo el libro.
type contadorObjetos struct{ tablas, dibujos, graficos int }

// celdasConTablas es la hoja con las celdas que sus tablas exigen en OOXML: la cabecera
// con los nombres únicos de columna (Excel da el fichero por dañado si no coinciden) y
// la fila de totales con sus fórmulas SUBTOTAL, que el editor no guarda porque la
// calcula al pintar.
//
// Con `odf`, la fórmula del total va con rangos A1 y `;` (OpenFormula), que es lo que
// LibreOffice entiende en un .ods; en .xlsx va con la referencia estructurada.
func celdasConTablas(sd sheetData, odf bool) map[string]string {
	if len(sd.Tables) == 0 {
		return sd.Cells
	}
	out := maps.Clone(sd.Cells)
	for _, t := range sd.Tables {
		r, ok := leerRango(t.Range)
		if !ok {
			continue
		}
		cols := columnasDeTabla(sd, t, r)
		if t.Headers == nil || *t.Headers {
			for i, n := range cols {
				ponerCabecera(out, A1Ref(r.r1, r.c1+i), n)
			}
		}
		if !t.TotalsRow {
			continue
		}
		for i, n := range cols {
			ref := A1Ref(r.r2, r.c1+i)
			delete(out, ref)
			if f, ok := funcionTotalExcel[t.Totals[strconv.Itoa(i)]]; ok {
				args := strconv.Itoa(f.subtotal) + "," + t.Name + "[" + escaparColumna(n) + "]"
				if odf {
					cuerpo := rango{c1: r.c1 + i, c2: r.c1 + i, r1: r.r1, r2: r.r2 - 1}
					if t.Headers == nil || *t.Headers {
						cuerpo.r1++
					}
					args = strconv.Itoa(f.subtotal) + ";" + cuerpo.a1()
				}
				out[ref] = "=SUBTOTAL(" + args + ")"
			} else if i == 0 {
				out[ref] = "Total"
			}
		}
	}
	return out
}

// ponerCabecera escribe el nombre de columna en su celda; si ya lo dice, conserva el HTML
// (negrita, alineación).
func ponerCabecera(out map[string]string, ref, nombre string) {
	if stripHTML(out[ref]) != nombre {
		out[ref] = xmlTextoCelda(nombre)
	}
}

// xmlTextoCelda es el HTML de una celda con ese texto (el editor guarda HTML).
func xmlTextoCelda(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// escaparColumna es el escape de una columna dentro de `Tabla[…]`: `'` delante de los
// cuatro especiales.
func escaparColumna(n string) string {
	return strings.NewReplacer("'", "''", "[", "'[", "]", "']", "#", "'#").Replace(n)
}

// columnasDeTabla son los nombres únicos de columna, sacados del texto de la cabecera.
func columnasDeTabla(sd sheetData, t sheetTable, r rango) []string {
	crudos := make([]string, r.c2-r.c1+1)
	if t.Headers == nil || *t.Headers {
		for i := range crudos {
			crudos[i] = stripHTML(sd.Cells[A1Ref(r.r1, r.c1+i)])
		}
	}
	return nombresCabecera(crudos)
}

// objetosXlsx arma lo que la hoja n (1-based) necesita para sus objetos.
func objetosXlsx(sd sheetData, hoja string, cuenta *contadorObjetos) *partesObjetos {
	p := &partesObjetos{partes: map[string]string{}}
	rid := func(tipo, destino string) string {
		id := "rIdO" + strconv.Itoa(len(p.rels)+1)
		p.rels = append(p.rels, `<Relationship Id="`+id+`" Type="`+tipo+`" Target="`+destino+`"/>`)
		return id
	}
	if len(sd.Charts) > 0 {
		cuenta.dibujos++
		n := cuenta.dibujos
		dibujo, relsDibujo := dibujoXlsx(sd, hoja, cuenta, p)
		ruta := "xl/drawings/drawing" + strconv.Itoa(n) + ".xml"
		p.partes[ruta] = dibujo
		p.partes["xl/drawings/_rels/drawing"+strconv.Itoa(n)+".xml.rels"] = relsDibujo
		p.overrides = append(p.overrides, `<Override PartName="/`+ruta+`" ContentType="`+ctDibujo+`"/>`)
		p.colaHoja.WriteString(`<drawing r:id="` + rid(relDibujo, "../drawings/drawing"+strconv.Itoa(n)+".xml") + `"/>`)
	}
	var partesTabla []string
	for _, t := range sd.Tables {
		r, ok := leerRango(t.Range)
		if !ok {
			continue
		}
		cuenta.tablas++
		n := cuenta.tablas
		ruta := "xl/tables/table" + strconv.Itoa(n) + ".xml"
		p.partes[ruta] = tablaXlsx(sd, t, r, n)
		p.overrides = append(p.overrides, `<Override PartName="/`+ruta+`" ContentType="`+ctTabla+`"/>`)
		partesTabla = append(partesTabla, `<tablePart r:id="`+rid(relTabla, "../tables/table"+strconv.Itoa(n)+".xml")+`"/>`)
	}
	if len(partesTabla) > 0 {
		p.colaHoja.WriteString(`<tableParts count="` + strconv.Itoa(len(partesTabla)) + `">` + strings.Join(partesTabla, "") + `</tableParts>`)
	}
	var ext strings.Builder
	if len(sd.Sparklines) > 0 {
		ext.WriteString(`<ext uri="` + uriSparklines + `" xmlns:x14="` + nsX14 + `"><x14:sparklineGroups xmlns:xm="` + nsXm + `">`)
		for _, m := range sd.Sparklines {
			ext.WriteString(minigraficoXlsx(m, hoja))
		}
		ext.WriteString(`</x14:sparklineGroups></ext>`)
	}
	if len(sd.Slicers) > 0 {
		b, _ := json.Marshal(sd.Slicers)
		ext.WriteString(`<ext uri="` + uriSytue + `" xmlns:sy="` + nsSytue + `"><sy:slicers>` + xmlEscape(string(b)) + `</sy:slicers></ext>`)
	}
	if ext.Len() > 0 {
		p.colaHoja.WriteString(`<extLst>` + ext.String() + `</extLst>`)
	}
	return p
}

// tablaXlsx es el ListObject: `ref`, cabecera, totales, columnas y estilo.
func tablaXlsx(sd sheetData, t sheetTable, r rango, id int) string {
	cab := t.Headers == nil || *t.Headers
	cols := columnasDeTabla(sd, t, r)
	var b strings.Builder
	b.WriteString(xmlHeader + `<table xmlns="` + nsMainX + `" id="` + strconv.Itoa(id) + `" name="` + xmlEscape(t.Name) +
		`" displayName="` + xmlEscape(t.Name) + `" ref="` + r.a1() + `"`)
	if !cab {
		b.WriteString(` headerRowCount="0"`)
	}
	if t.TotalsRow {
		b.WriteString(` totalsRowCount="1"`)
	} else {
		b.WriteString(` totalsRowShown="0"`)
	}
	b.WriteString(`>`)
	if cab {
		cuerpo := r
		if t.TotalsRow {
			cuerpo.r2--
		}
		b.WriteString(autoFilterTabla(sd, cuerpo))
	}
	b.WriteString(`<tableColumns count="` + strconv.Itoa(len(cols)) + `">`)
	for i, n := range cols {
		b.WriteString(`<tableColumn id="` + strconv.Itoa(i+1) + `" name="` + xmlEscape(n) + `"`)
		if f, ok := funcionTotalExcel[t.Totals[strconv.Itoa(i)]]; t.TotalsRow && ok {
			b.WriteString(` totalsRowFunction="` + f.nombre + `"`)
		} else if t.TotalsRow && i == 0 {
			b.WriteString(` totalsRowLabel="Total"`)
		}
		b.WriteString(`/>`)
	}
	estilo := estiloTablaExcel[t.Style]
	if estilo == "" {
		estilo = estiloTablaExcel["slate"]
	}
	b.WriteString(`</tableColumns><tableStyleInfo name="` + estilo +
		`" showFirstColumn="0" showLastColumn="0" showRowStripes="1" showColumnStripes="0"/></table>`)
	return b.String()
}

// minigraficoXlsx es un grupo de UN minigráfico: su tipo (ganancia/pérdida es
// `stacked`), su color y el rojo de las pérdidas.
func minigraficoXlsx(m sheetSparkline, hoja string) string {
	tipo := map[string]string{"line": "line", "column": "column", "winloss": "stacked"}[m.Type]
	color := m.Color
	if color == "" {
		color = "#2563EB"
	}
	datos, ok := leerRango(m.Range)
	celda, ok2 := leerRango(m.Cell)
	if tipo == "" || !ok || !ok2 {
		return ""
	}
	return `<x14:sparklineGroup type="` + tipo + `" displayEmptyCellsAs="gap"` + `>` +
		`<x14:colorSeries rgb="FF` + strings.ToUpper(strings.TrimPrefix(color, "#")) + `"/>` +
		`<x14:colorNegative rgb="FFDC2626"/>` +
		`<x14:sparklines><x14:sparkline><xm:f>` + xmlEscape(strings.ReplaceAll(datos.absoluto(hoja), "$", "")) + `</xm:f>` +
		`<xm:sqref>` + celda.a1() + `</xm:sqref></x14:sparkline></x14:sparklines></x14:sparklineGroup>`
}

// dibujoXlsx es el `xdr:wsDr` de la hoja con un ancla por gráfico, y escribe cada
// gráfico en su parte.
func dibujoXlsx(sd sheetData, hoja string, cuenta *contadorObjetos, p *partesObjetos) (dibujo, rels string) {
	var d, r strings.Builder
	d.WriteString(xmlHeader + `<xdr:wsDr xmlns:xdr="` + nsXdr + `" xmlns:a="` + nsDrawing + `">`)
	r.WriteString(xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i, g := range sd.Charts {
		cuenta.graficos++
		n := cuenta.graficos
		ruta := "xl/charts/chart" + strconv.Itoa(n) + ".xml"
		p.partes[ruta] = graficoXlsx(sd, g, hoja)
		p.overrides = append(p.overrides, `<Override PartName="/`+ruta+`" ContentType="`+ctGrafico+`"/>`)
		id := "rId" + strconv.Itoa(i+1)
		r.WriteString(`<Relationship Id="` + id + `" Type="` + relGrafico + `" Target="../charts/chart` + strconv.Itoa(n) + `.xml"/>`)
		x, y := g.posicion()
		w, h := g.area()
		col, fila := int(x)/anchoCeldaPx, int(y)/altoCeldaPx
		d.WriteString(`<xdr:oneCellAnchor><xdr:from><xdr:col>` + strconv.Itoa(col) + `</xdr:col><xdr:colOff>` +
			strconv.FormatInt(pxAEmu(x-float64(col*anchoCeldaPx)), 10) + `</xdr:colOff><xdr:row>` + strconv.Itoa(fila) +
			`</xdr:row><xdr:rowOff>` + strconv.FormatInt(pxAEmu(y-float64(fila*altoCeldaPx)), 10) + `</xdr:rowOff></xdr:from>` +
			`<xdr:ext cx="` + strconv.FormatInt(pxAEmu(w), 10) + `" cy="` + strconv.FormatInt(pxAEmu(h), 10) + `"/>` +
			`<xdr:graphicFrame macro=""><xdr:nvGraphicFramePr><xdr:cNvPr id="` + strconv.Itoa(i+2) + `" name="` +
			xmlEscape("Gráfico "+strconv.Itoa(i+1)) + `" descr="` + xmlEscape(g.ID) + `"/><xdr:cNvGraphicFramePr/></xdr:nvGraphicFramePr>` +
			`<xdr:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></xdr:xfrm><a:graphic><a:graphicData uri="` + nsChart + `">` +
			`<c:chart xmlns:c="` + nsChart + `" xmlns:r="` + nsRelX + `" r:id="` + id + `"/></a:graphicData></a:graphic>` +
			`</xdr:graphicFrame><xdr:clientData/></xdr:oneCellAnchor>`)
	}
	d.WriteString(`</xdr:wsDr>`)
	r.WriteString(`</Relationships>`)
	return d.String(), r.String()
}

// Los ejes de un gráfico: la pareja principal y, si hay eje secundario, la segunda.
const (
	ejeCat, ejeVal   = 5001, 5002
	ejeCat2, ejeVal2 = 5003, 5004
)

// graficoXlsx es el `c:chartSpace` de un gráfico.
//
//nolint:gocyclo,gocognit // un gráfico de DrawingML tiene ese número de ramas: una por forma
func graficoXlsx(sd sheetData, g sheetChart, hoja string) string {
	r, ok := leerRango(g.Range)
	if !ok {
		return ""
	}
	esTexto := func(fila, col int) bool {
		raw := stripHTML(sd.Cells[A1Ref(fila, col)])
		if raw == "" || strings.HasPrefix(raw, "=") {
			return false
		}
		_, err := strconv.ParseFloat(raw, 64)
		return err != nil
	}
	cats, series := seriesDeGrafico(r, g.ByRow, esTexto)
	if len(series) > len(coloresSerie) {
		series = series[:len(coloresSerie)]
	}
	var b strings.Builder
	b.WriteString(xmlHeader + `<c:chartSpace xmlns:c="` + nsChart + `" xmlns:a="` + nsDrawing + `" xmlns:r="` + nsRelX + `">` +
		`<c:roundedCorners val="0"/><c:chart>`)
	if g.Title != "" {
		b.WriteString(`<c:title>` + textoRico(g.Title) + `<c:overlay val="0"/></c:title><c:autoTitleDeleted val="0"/>`)
	} else {
		b.WriteString(`<c:autoTitleDeleted val="1"/>`)
	}
	b.WriteString(`<c:plotArea><c:layout/>`)
	sec := map[int]bool{}
	for _, k := range g.SecondaryAxis {
		sec[k] = true
	}
	linea := map[int]bool{}
	for _, k := range g.LineSeries {
		linea[k] = true
	}
	if g.Type == "combo" && len(g.LineSeries) == 0 {
		linea[len(series)-1] = true
	}
	grupo := func(forma string, ks []int, cat, val int) {
		if len(ks) == 0 {
			return
		}
		b.WriteString(abrirGrupo(g, forma))
		for _, k := range ks {
			b.WriteString(serieXlsx(g, forma, k, series[k], cats, hoja))
		}
		b.WriteString(etiquetasXlsx(g, forma) + cerrarGrupo(g, forma, cat, val))
	}
	var todas []int
	for k := range series {
		todas = append(todas, k)
	}
	conSec := false
	switch g.Type {
	case "pie", "donut":
		grupo(g.Type, []int{0}, 0, 0)
	case "scatter", "radar", "hbar":
		grupo(g.Type, todas, ejeCat, ejeVal)
	default:
		// Columnas, líneas, área y combinado: por forma y por eje. El eje secundario
		// solo existe con series a los dos lados y sin apilar, como en el editor.
		enSec := 0
		for k := range series {
			if sec[k] {
				enSec++
			}
		}
		conSec = !g.Stacked && enSec > 0 && enSec < len(series)
		for _, eje := range []bool{false, true} {
			var col, lin []int
			for k := range series {
				if (conSec && sec[k]) != eje {
					continue
				}
				if g.Type == "line" || g.Type == "area" || (g.Type == "combo" && linea[k]) {
					lin = append(lin, k)
				} else {
					col = append(col, k)
				}
			}
			cat, val := ejeCat, ejeVal
			if eje {
				cat, val = ejeCat2, ejeVal2
			}
			formaLin := "line"
			if g.Type == "area" {
				formaLin = "area"
			}
			grupo("bar", col, cat, val)
			grupo(formaLin, lin, cat, val)
		}
	}
	b.WriteString(ejesXlsx(g, conSec))
	b.WriteString(`</c:plotArea>`)
	if g.Legend != "none" {
		pos := map[string]string{"": "b", "bottom": "b", "top": "t", "right": "r"}[g.Legend]
		b.WriteString(`<c:legend><c:legendPos val="` + pos + `"/><c:overlay val="0"/></c:legend>`)
	}
	b.WriteString(`<c:plotVisOnly val="1"/><c:dispBlanksAs val="gap"/></c:chart></c:chartSpace>`)
	return b.String()
}

// textoRico es un `c:tx` con un párrafo.
func textoRico(s string) string {
	return `<c:tx><c:rich><a:bodyPr/><a:p><a:r><a:t>` + xmlEscape(s) + `</a:t></a:r></a:p></c:rich></c:tx>`
}

func abrirGrupo(g sheetChart, forma string) string {
	switch forma {
	case "bar":
		dir, grupo := "col", "clustered"
		if g.Type == "hbar" {
			dir = "bar"
		}
		if g.Stacked {
			grupo = "stacked"
		}
		return `<c:barChart><c:barDir val="` + dir + `"/><c:grouping val="` + grupo + `"/><c:varyColors val="0"/>`
	case "hbar":
		return abrirGrupo(g, "bar")
	case "line":
		return `<c:lineChart><c:grouping val="standard"/><c:varyColors val="0"/>`
	case "area":
		grupo := "standard"
		if g.Stacked {
			grupo = "stacked"
		}
		return `<c:areaChart><c:grouping val="` + grupo + `"/><c:varyColors val="0"/>`
	case "pie":
		return `<c:pieChart><c:varyColors val="1"/>`
	case "donut":
		return `<c:doughnutChart><c:varyColors val="1"/>`
	case "scatter":
		return `<c:scatterChart><c:scatterStyle val="lineMarker"/><c:varyColors val="0"/>`
	default: // radar
		return `<c:radarChart><c:radarStyle val="filled"/><c:varyColors val="0"/>`
	}
}

func cerrarGrupo(g sheetChart, forma string, cat, val int) string {
	ejes := `<c:axId val="` + strconv.Itoa(cat) + `"/><c:axId val="` + strconv.Itoa(val) + `"/>`
	switch forma {
	case "bar", "hbar":
		solape := ""
		if g.Stacked {
			solape = `<c:overlap val="100"/>`
		}
		return `<c:gapWidth val="39"/>` + solape + ejes + `</c:barChart>`
	case "line":
		return `<c:marker val="1"/>` + ejes + `</c:lineChart>`
	case "area":
		return ejes + `</c:areaChart>`
	case "pie":
		return `<c:firstSliceAng val="0"/></c:pieChart>`
	case "donut":
		return `<c:firstSliceAng val="0"/><c:holeSize val="58"/></c:doughnutChart>`
	case "scatter":
		return ejes + `</c:scatterChart>`
	default:
		return ejes + `</c:radarChart>`
	}
}

// rellenoSerie es el `c:spPr` de un color: relleno, o trazo para las líneas.
func rellenoSerie(color string, trazo bool) string {
	hex := strings.ToUpper(strings.TrimPrefix(color, "#"))
	if trazo {
		return `<c:spPr><a:ln w="25400" cap="rnd"><a:solidFill><a:srgbClr val="` + hex + `"/></a:solidFill><a:round/></a:ln></c:spPr>`
	}
	return `<c:spPr><a:solidFill><a:srgbClr val="` + hex + `"/></a:solidFill></c:spPr>`
}

// serieXlsx es un `c:ser` con su nombre, sus categorías (o X) y sus valores.
func serieXlsx(g sheetChart, forma string, k int, s serieRef, cats *rango, hoja string) string {
	var b strings.Builder
	b.WriteString(`<c:ser><c:idx val="` + strconv.Itoa(k) + `"/><c:order val="` + strconv.Itoa(k) + `"/>`)
	if s.nombre != nil {
		b.WriteString(`<c:tx><c:strRef><c:f>` + xmlEscape(s.nombre.absoluto(hoja)) + `</c:f></c:strRef></c:tx>`)
	}
	switch forma {
	case "pie", "donut":
		// Una porción por categoría, cada una con su color (como el editor).
		n := s.valores.r2 - s.valores.r1 + s.valores.c2 - s.valores.c1 + 1
		b.WriteString(rellenoSerie(g.color(0), false))
		for i := range min(n, len(coloresSerie)) {
			b.WriteString(`<c:dPt><c:idx val="` + strconv.Itoa(i) + `"/><c:bubble3D val="0"/>` + rellenoSerie(g.color(i), false) + `</c:dPt>`)
		}
	case "line", "radar":
		b.WriteString(rellenoSerie(g.color(k), true))
		if forma == "line" {
			b.WriteString(`<c:marker><c:symbol val="circle"/><c:size val="5"/>` + rellenoSerie(g.color(k), false) + `</c:marker>`)
		}
	case "scatter":
		b.WriteString(`<c:spPr><a:ln w="19050"><a:noFill/></a:ln></c:spPr><c:marker><c:symbol val="circle"/><c:size val="5"/>` +
			rellenoSerie(g.color(k), false) + `</c:marker>`)
	default:
		b.WriteString(rellenoSerie(g.color(k), false))
	}
	if forma == "scatter" {
		if cats != nil {
			b.WriteString(`<c:xVal><c:numRef><c:f>` + xmlEscape(cats.absoluto(hoja)) + `</c:f></c:numRef></c:xVal>`)
		}
		b.WriteString(`<c:yVal><c:numRef><c:f>` + xmlEscape(s.valores.absoluto(hoja)) + `</c:f></c:numRef></c:yVal><c:smooth val="0"/></c:ser>`)
		return b.String()
	}
	if cats != nil {
		b.WriteString(`<c:cat><c:strRef><c:f>` + xmlEscape(cats.absoluto(hoja)) + `</c:f></c:strRef></c:cat>`)
	}
	b.WriteString(`<c:val><c:numRef><c:f>` + xmlEscape(s.valores.absoluto(hoja)) + `</c:f></c:numRef></c:val>`)
	if forma == "line" {
		b.WriteString(`<c:smooth val="0"/>`)
	}
	b.WriteString(`</c:ser>`)
	return b.String()
}

// etiquetasXlsx son las etiquetas de datos del grupo: el valor, o el porcentaje en el
// circular.
func etiquetasXlsx(g sheetChart, forma string) string {
	if !g.Labels {
		return ""
	}
	val, pct := "1", "0"
	if forma == "pie" || forma == "donut" {
		val, pct = "0", "1"
	}
	return `<c:dLbls><c:showLegendKey val="0"/><c:showVal val="` + val + `"/><c:showCatName val="0"/>` +
		`<c:showSerName val="0"/><c:showPercent val="` + pct + `"/><c:showBubbleSize val="0"/></c:dLbls>`
}

// ejesXlsx son los ejes: el de categorías abajo (a la izquierda en las barras) y el de
// valores con la retícula; en la dispersión los dos son de valores. El título de abajo
// es `xTitle` y el de la izquierda `yTitle`, sea cual sea el eje que caiga ahí.
func ejesXlsx(g sheetChart, conSec bool) string {
	if g.Type == "pie" || g.Type == "donut" {
		return ""
	}
	abajo, izq := "b", "l"
	if g.Type == "hbar" {
		abajo, izq = "l", "b" // el de categorías va a la izquierda y el de valores abajo
	}
	titulo := func(pos string) string {
		t := g.XTitle
		if pos == "l" {
			t = g.YTitle
		}
		if t == "" {
			return ""
		}
		return `<c:title>` + textoRico(t) + `<c:overlay val="0"/></c:title>`
	}
	reticula := ""
	if g.Grid == nil || *g.Grid {
		// Con su trazo: un `majorGridlines` vacío, LibreOffice lo tira al leerlo.
		reticula = `<c:majorGridlines><c:spPr><a:ln w="9525"><a:solidFill><a:srgbClr val="E2E8F0"/></a:solidFill></a:ln></c:spPr></c:majorGridlines>`
	}
	tipoCat := "c:catAx"
	if g.Type == "scatter" {
		tipoCat = "c:valAx"
	}
	eje := func(etiqueta string, id, cruce int, pos string, borrado bool, extra string) string {
		b := `<` + etiqueta + `><c:axId val="` + strconv.Itoa(id) + `"/><c:scaling><c:orientation val="minMax"/></c:scaling>` +
			`<c:delete val="` + map[bool]string{true: "1", false: "0"}[borrado] + `"/><c:axPos val="` + pos + `"/>` + extra
		if !borrado {
			b += titulo(pos)
		}
		return b + `<c:crossAx val="` + strconv.Itoa(cruce) + `"/>` + map[string]string{"c:catAx": `<c:auto val="1"/>`}[etiqueta] + `</` + etiqueta + `>`
	}
	out := eje(tipoCat, ejeCat, ejeVal, abajo, false, "") + eje("c:valAx", ejeVal, ejeCat, izq, false, reticula)
	if conSec {
		// `c:crosses` va DETRÁS de `c:crossAx` (CT_ValAx): el eje secundario corta al
		// final del de categorías, a la derecha.
		out += eje("c:catAx", ejeCat2, ejeVal2, "b", true, "") +
			strings.Replace(eje("c:valAx", ejeVal2, ejeCat2, "r", false, ""), `"/></c:valAx>`, `"/><c:crosses val="max"/></c:valAx>`, 1)
	}
	return out
}
