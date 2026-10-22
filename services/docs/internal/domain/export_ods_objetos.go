// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Exportación a ODF (.ods) de los objetos de una hoja, en lo que escribe LibreOffice:
//
//   - un GRÁFICO es un objeto incrustado (`Object N/content.xml`, un `office:chart`)
//     que cuelga de un `draw:frame` en la celda donde cae, con el id en su `svg:desc`;
//   - un MINIGRÁFICO es un `calcext:sparkline-group` (LibreOffice 7.4 o posterior);
//   - una TABLA es un `table:database-range` con los botones de filtro, y lo que ODF no
//     sabe decir de ella —el estilo y las funciones de la fila de totales— va en dos
//     atributos propios (`urn:sytue:calcus`) que LibreOffice ignora;
//   - las SEGMENTACIONES, que ODF no tiene, en un elemento propio dentro de la tabla.

const (
	nsCalcExt = "urn:org:documentfoundation:names:experimental:calc:xmlns:calcext:1.0"
	nsChartO  = "urn:oasis:names:tc:opendocument:xmlns:chart:1.0"
	nsDrawO   = "urn:oasis:names:tc:opendocument:xmlns:drawing:1.0"
	nsSvgO    = "urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0"
	nsXlink   = "http://www.w3.org/1999/xlink"
	tipoChart = "application/vnd.oasis.opendocument.chart"
)

// refODS es una referencia de celda o de rango en la forma de ODF: `'Hoja 1'.A1:'Hoja 1'.C5`.
func (r rango) refODS(hoja string) string {
	h := "'" + strings.ReplaceAll(hoja, "'", "''") + "'."
	out := h + A1Ref(r.r1, r.c1)
	if r.c1 != r.c2 || r.r1 != r.r2 {
		out += ":" + h + A1Ref(r.r2, r.c2)
	}
	return out
}

// pulgadas escribe px de la rejilla (96 por pulgada) como longitud de ODF.
func pulgadas(px float64) string {
	return strconv.FormatFloat(px/96, 'f', 4, 64) + "in"
}

// marcosODS son los `draw:frame` de los gráficos por celda (fila y columna 1-based),
// y los objetos incrustados que hay que añadir al paquete.
func marcosODS(sd sheetData, hoja string, n *int, media map[string][]byte) map[[2]int]string {
	out := map[[2]int]string{}
	for i, g := range sd.Charts {
		*n++
		obj := "Object " + strconv.Itoa(*n)
		media[obj+"/content.xml"] = []byte(graficoODS(sd, g, hoja))
		x, y := g.posicion()
		w, h := g.area()
		col, fila := int(x)/anchoCeldaPx, int(y)/altoCeldaPx
		celda := [2]int{fila + 1, col + 1}
		out[celda] += `<draw:frame draw:z-index="` + strconv.Itoa(i) + `" draw:name="` + xmlEscape("Gráfico "+strconv.Itoa(i+1)) +
			`" svg:width="` + pulgadas(w) + `" svg:height="` + pulgadas(h) + `" svg:x="` + pulgadas(x-float64(col*anchoCeldaPx)) +
			`" svg:y="` + pulgadas(y-float64(fila*altoCeldaPx)) + `"><draw:object xlink:href="./` + obj +
			`" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad"/><svg:desc>` + xmlEscape(g.ID) + `</svg:desc></draw:frame>`
	}
	return out
}

// colaTablaODS es lo que va al final de la `table:table`: los minigráficos y las
// segmentaciones.
func colaTablaODS(sd sheetData, hoja string, styles *odsStyles) string {
	var b strings.Builder
	b.WriteString(formatosCondicionalesODS(sd, hoja, styles))
	if len(sd.Sparklines) > 0 {
		b.WriteString(`<calcext:sparkline-groups xmlns:calcext="` + nsCalcExt + `">`)
		for i, m := range sd.Sparklines {
			datos, ok := leerRango(m.Range)
			celda, ok2 := leerRango(m.Cell)
			tipo := map[string]string{"line": "line", "column": "column", "winloss": "stacked"}[m.Type]
			if !ok || !ok2 || tipo == "" {
				continue
			}
			color := m.Color
			if color == "" {
				color = "#2563EB"
			}
			b.WriteString(`<calcext:sparkline-group calcext:id="{00000000-0000-4000-8000-` + fmtDoce(i) + `}" calcext:type="` + tipo +
				`" calcext:display-empty-cells-as="gap" calcext:color-series="` + strings.ToLower(color) +
				`" calcext:color-negative="#dc2626"><calcext:sparklines><calcext:sparkline calcext:cell-address="` +
				xmlEscape(celda.refODS(hoja)) + `" calcext:data-range="` + xmlEscape(datos.refODS(hoja)) +
				`"/></calcext:sparklines></calcext:sparkline-group>`)
		}
		b.WriteString(`</calcext:sparkline-groups>`)
	}
	if len(sd.Slicers) > 0 {
		j, _ := json.Marshal(sd.Slicers)
		b.WriteString(`<sytue:slicers xmlns:sytue="` + nsSytue + `">` + xmlEscape(string(j)) + `</sytue:slicers>`)
	}
	return b.String()
}

func fmtDoce(i int) string {
	s := strconv.Itoa(i)
	return strings.Repeat("0", max(12-len(s), 0)) + s
}

// rangosODS son los `table:database-range` de las tablas de todas las hojas.
func rangosODS(hojas []sheetData) string {
	var b strings.Builder
	for i, sd := range hojas {
		hoja := sd.Name
		if hoja == "" {
			hoja = "Hoja" + strconv.Itoa(i+1)
		}
		b.WriteString(rangoFiltroODS(sd, hoja, i))
		for _, t := range sd.Tables {
			r, ok := leerRango(t.Range)
			if !ok {
				continue
			}
			cab := t.Headers == nil || *t.Headers
			b.WriteString(`<table:database-range table:name="` + xmlEscape(t.Name) + `" table:target-range-address="` +
				xmlEscape(r.refODS(hoja)) + `" table:contains-header="` + strconv.FormatBool(cab) + `" table:display-filter-buttons="` +
				strconv.FormatBool(cab) + `" xmlns:sytue="` + nsSytue + `"`)
			if t.Style != "" {
				b.WriteString(` sytue:style="` + xmlEscape(t.Style) + `"`)
			}
			if t.TotalsRow {
				tot, _ := json.Marshal(t.Totals)
				b.WriteString(` sytue:totals="` + xmlEscape(string(tot)) + `"`)
			}
			b.WriteString(`/>`)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return `<table:database-ranges>` + b.String() + `</table:database-ranges>`
}

// graficoODS es el documento de un gráfico incrustado.
//
//nolint:gocyclo,gocognit // una rama por forma de gráfico, como en DrawingML
func graficoODS(sd sheetData, g sheetChart, hoja string) string {
	r, _ := leerRango(g.Range)
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
	clase := map[string]string{
		"bar": "chart:bar", "hbar": "chart:bar", "line": "chart:line", "area": "chart:area", "combo": "chart:bar",
		"pie": "chart:circle", "donut": "chart:ring", "scatter": "chart:scatter", "radar": "chart:filled-radar",
	}[g.Type]
	w, h := g.area()
	var est, cuerpo strings.Builder
	// El estilo del área de trazado: apilado, barras en horizontal y las etiquetas.
	est.WriteString(`<style:style style:name="pa" style:family="chart"><style:chart-properties chart:stacked="` +
		strconv.FormatBool(g.Stacked) + `" chart:vertical="` + strconv.FormatBool(g.Type == "hbar") + `"`)
	if g.Type == "pie" || g.Type == "donut" {
		est.WriteString(` chart:angle-offset="90"`)
	}
	est.WriteString(`/></style:style>`)
	etiqueta := ""
	if g.Labels {
		etiqueta = ` chart:data-label-number="value"`
		if g.Type == "pie" || g.Type == "donut" {
			etiqueta = ` chart:data-label-number="percentage"`
		}
	}
	sec := map[int]bool{}
	enSec := 0
	for _, k := range g.SecondaryAxis {
		if k < len(series) {
			sec[k] = true
			enSec++
		}
	}
	conSec := !g.Stacked && enSec > 0 && enSec < len(series) && (g.Type == "bar" || g.Type == "line" || g.Type == "area" || g.Type == "combo")
	linea := map[int]bool{}
	for _, k := range g.LineSeries {
		linea[k] = true
	}
	if g.Type == "combo" && len(g.LineSeries) == 0 {
		linea[len(series)-1] = true
	}
	cuerpo.WriteString(`<chart:chart svg:width="` + pulgadas(w) + `" svg:height="` + pulgadas(h) + `" chart:class="` + clase + `">`)
	if g.Title != "" {
		cuerpo.WriteString(`<chart:title><text:p>` + xmlEscape(g.Title) + `</text:p></chart:title>`)
	}
	if g.Legend != "none" {
		pos := map[string]string{"": "bottom", "bottom": "bottom", "top": "top", "right": "end"}[g.Legend]
		cuerpo.WriteString(`<chart:legend chart:legend-position="` + pos + `"/>`)
	}
	etiquetas := "none"
	switch {
	case cats != nil && len(series) > 0 && series[0].nombre != nil:
		etiquetas = "both"
	case cats != nil:
		etiquetas = "column"
	case len(series) > 0 && series[0].nombre != nil:
		etiquetas = "row"
	}
	if g.ByRow {
		etiquetas = map[string]string{"both": "both", "column": "row", "row": "column", "none": "none"}[etiquetas]
	}
	cuerpo.WriteString(`<chart:plot-area chart:style-name="pa" table:cell-range-address="` + xmlEscape(r.refODS(hoja)) +
		`" chart:data-source-has-labels="` + etiquetas + `">`)
	titulo := func(t string) string {
		if t == "" {
			return ""
		}
		return `<chart:title><text:p>` + xmlEscape(t) + `</text:p></chart:title>`
	}
	if g.Type != "pie" && g.Type != "donut" {
		cuerpo.WriteString(`<chart:axis chart:dimension="x" chart:name="primary-x">` + titulo(g.XTitle))
		if cats != nil && g.Type != "scatter" {
			cuerpo.WriteString(`<chart:categories table:cell-range-address="` + xmlEscape(cats.refODS(hoja)) + `"/>`)
		}
		cuerpo.WriteString(`</chart:axis><chart:axis chart:dimension="y" chart:name="primary-y">` + titulo(g.YTitle))
		if g.Grid == nil || *g.Grid {
			cuerpo.WriteString(`<chart:grid chart:class="major"/>`)
		}
		cuerpo.WriteString(`</chart:axis>`)
		if conSec {
			cuerpo.WriteString(`<chart:axis chart:dimension="y" chart:name="secondary-y"/>`)
		}
	} else if cats != nil {
		cuerpo.WriteString(`<chart:axis chart:dimension="x" chart:name="primary-x"><chart:categories table:cell-range-address="` +
			xmlEscape(cats.refODS(hoja)) + `"/></chart:axis>`)
	}
	for k, s := range series {
		if (g.Type == "pie" || g.Type == "donut") && k > 0 {
			break
		}
		nombre := "s" + strconv.Itoa(k)
		color := strings.ToLower(g.color(k))
		trazo := `draw:stroke="none" draw:fill-color="` + color + `"`
		claseSerie := clase
		if g.Type == "donut" {
			claseSerie = "chart:circle" // el anillo de Calc es una serie circular en un gráfico `ring`
		}
		if g.Type == "line" || g.Type == "radar" || (g.Type == "combo" && linea[k]) {
			trazo = `draw:stroke="solid" svg:stroke-width="0.07cm" svg:stroke-color="` + color + `" draw:fill-color="` + color + `"`
			if g.Type == "combo" {
				claseSerie = "chart:line"
			}
		}
		if g.Type == "scatter" {
			trazo = `draw:stroke="none" draw:fill-color="` + color + `" svg:stroke-color="` + color + `"`
		}
		est.WriteString(`<style:style style:name="` + nombre + `" style:family="chart"><style:chart-properties` + etiqueta)
		if g.Type == "scatter" {
			est.WriteString(` chart:symbol-type="automatic"`)
		}
		est.WriteString(`/><style:graphic-properties ` + trazo + `/></style:style>`)
		cuerpo.WriteString(`<chart:series chart:style-name="` + nombre + `" chart:values-cell-range-address="` +
			xmlEscape(s.valores.refODS(hoja)) + `" chart:class="` + claseSerie + `"`)
		if s.nombre != nil {
			cuerpo.WriteString(` chart:label-cell-address="` + xmlEscape(s.nombre.refODS(hoja)) + `"`)
		}
		if conSec && sec[k] {
			cuerpo.WriteString(` chart:attached-axis="secondary-y"`)
		}
		cuerpo.WriteString(`>`)
		if g.Type == "scatter" && cats != nil {
			cuerpo.WriteString(`<chart:domain table:cell-range-address="` + xmlEscape(cats.refODS(hoja)) + `"/>`)
		}
		if g.Type == "pie" || g.Type == "donut" {
			n := s.valores.r2 - s.valores.r1 + s.valores.c2 - s.valores.c1 + 1
			for i := range n {
				p := "p" + strconv.Itoa(i)
				est.WriteString(`<style:style style:name="` + p + `" style:family="chart"><style:chart-properties` + etiqueta +
					`/><style:graphic-properties draw:stroke="solid" svg:stroke-color="#ffffff" draw:fill-color="` +
					strings.ToLower(g.color(min(i, len(coloresSerie)))) + `"/></style:style>`)
				cuerpo.WriteString(`<chart:data-point chart:style-name="` + p + `"/>`)
			}
		}
		cuerpo.WriteString(`</chart:series>`)
	}
	cuerpo.WriteString(`<chart:wall/><chart:floor/></chart:plot-area></chart:chart>`)
	return xmlHeader + `<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
		`xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" ` +
		`xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" xmlns:draw="` + nsDrawO + `" ` +
		`xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" xmlns:xlink="` + nsXlink + `" ` +
		`xmlns:svg="` + nsSvgO + `" xmlns:chart="` + nsChartO + `" office:version="1.3">` +
		`<office:automatic-styles>` + est.String() + `</office:automatic-styles><office:body><office:chart>` +
		cuerpo.String() + `</office:chart></office:body></office:document-content>`
}
