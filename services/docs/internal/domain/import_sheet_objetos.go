// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"crypto/rand"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Importación de los objetos de una hoja OOXML: el reverso de export_sheet_objetos.go,
// y lo bastante tolerante para leer lo que escriben Excel y Calc (sus tablas, sus
// gráficos anclados a una o a dos celdas, sus minigráficos de x14).

// destinoRel resuelve el destino de una relación respecto de la parte que la declara.
func destinoRel(parte, destino string) string {
	if strings.HasPrefix(destino, "/") {
		return path.Clean(strings.TrimPrefix(destino, "/"))
	}
	return path.Clean(path.Join(path.Dir(parte), destino))
}

// relsDe son las relaciones de una parte (`dir/_rels/nombre.rels`).
func relsDe(files map[string][]byte, parte string) map[string]string {
	return parseRels(files[path.Join(path.Dir(parte), "_rels", path.Base(parte)+".rels")])
}

// objetosDeHojaXlsx lee las tablas, los gráficos, los minigráficos y las segmentaciones
// de la hoja `parte` y quita de sus celdas las de las filas de totales (las fórmulas
// SUBTOTAL que el editor calcula al pintar).
func objetosDeHojaXlsx(files map[string][]byte, parte string, sd *sheetData, ws objetosLeidosXlsx) {
	rels := relsDe(files, parte)
	if ws.AutoFilter != nil { // el de la hoja; el de una tabla va con la tabla (abajo)
		sd.AutoFilter = autoFilterDeXlsx(*ws.AutoFilter, sd.Cells)
	}
	for _, t := range ws.Tablas {
		if tabla, ok := tablaDeXlsx(files[destinoRel(parte, rels[t.ID])]); ok {
			sd.Tables = append(sd.Tables, tabla)
			if af := autoFilterDeTabla(files[destinoRel(parte, rels[t.ID])], sd.Cells); af != nil && sd.AutoFilter == nil {
				sd.AutoFilter = af
			}
			if r, ok := leerRango(tabla.Range); ok && tabla.TotalsRow {
				for c := r.c1; c <= r.c2; c++ {
					delete(sd.Cells, A1Ref(r.r2, c))
				}
			}
		}
	}
	if ws.Drawing.ID != "" {
		dibujo := destinoRel(parte, rels[ws.Drawing.ID])
		sd.Charts = graficosDeDibujo(files, dibujo)
	}
	for _, e := range ws.Ext {
		switch e.URI {
		case uriSparklines:
			sd.Sparklines = minigraficosDeXlsx(e.Inner)
		case uriSytue:
			sd.Slicers = segmentacionesDeXlsx(e.Inner)
		}
	}
}

// estiloDeExcel es la vuelta de estiloTablaExcel; un estilo que no es de los cinco da
// el de por defecto.
func estiloDeExcel(nombre string) string {
	for id, n := range estiloTablaExcel {
		if n == nombre {
			return id
		}
	}
	return ""
}

func tablaDeXlsx(raw []byte) (sheetTable, bool) {
	var t struct {
		Name        string `xml:"name,attr"`
		DisplayName string `xml:"displayName,attr"`
		Ref         string `xml:"ref,attr"`
		Cabecera    string `xml:"headerRowCount,attr"`
		Totales     string `xml:"totalsRowCount,attr"`
		Columnas    []struct {
			Funcion string `xml:"totalsRowFunction,attr"`
		} `xml:"tableColumns>tableColumn"`
		Estilo struct {
			Name string `xml:"name,attr"`
		} `xml:"tableStyleInfo"`
	}
	if len(raw) == 0 || xml.Unmarshal(raw, &t) != nil {
		return sheetTable{}, false
	}
	r, ok := leerRango(t.Ref)
	if !ok {
		return sheetTable{}, false
	}
	nombre := t.DisplayName
	if nombre == "" {
		nombre = t.Name
	}
	out := sheetTable{Name: nombre, Range: r.a1(), Style: estiloDeExcel(t.Estilo.Name)}
	if t.Cabecera == "0" {
		no := false
		out.Headers = &no
	}
	if t.Totales != "" && t.Totales != "0" {
		out.TotalsRow = true
		for i, c := range t.Columnas {
			for nuestra, f := range funcionTotalExcel {
				if f.nombre == c.Funcion {
					if out.Totals == nil {
						out.Totals = map[string]string{}
					}
					out.Totals[strconv.Itoa(i)] = nuestra
				}
			}
		}
	}
	return out, true
}

// sinHoja quita el prefijo de hoja y los `$` de una referencia.
func sinHoja(ref string) string {
	if i := strings.LastIndex(ref, "!"); i >= 0 {
		ref = ref[i+1:]
	}
	return strings.ReplaceAll(ref, "$", "")
}

// hexDeARGB pasa «FF2563EB» a «#2563EB»; "" si no lo es.
func hexDeARGB(v string) string {
	if len(v) == 8 {
		v = v[2:]
	}
	if len(v) != 6 {
		return ""
	}
	if _, err := strconv.ParseUint(v, 16, 32); err != nil {
		return ""
	}
	return "#" + strings.ToUpper(v)
}

func minigraficosDeXlsx(inner []byte) []sheetSparkline {
	var g struct {
		Grupos []struct {
			Tipo  string `xml:"type,attr"`
			Color struct {
				RGB string `xml:"rgb,attr"`
			} `xml:"colorSeries"`
			Lineas []struct {
				F     string `xml:"f"`
				Sqref string `xml:"sqref"`
			} `xml:"sparklines>sparkline"`
		} `xml:"sparklineGroups>sparklineGroup"`
	}
	if xml.Unmarshal([]byte("<x>"+string(inner)+"</x>"), &g) != nil {
		return nil
	}
	var out []sheetSparkline
	for _, gr := range g.Grupos {
		tipo := map[string]string{"": "line", "line": "line", "column": "column", "stacked": "winloss"}[gr.Tipo]
		color := hexDeARGB(gr.Color.RGB)
		if color == "#2563EB" {
			color = "" // el azul por defecto no se guarda
		}
		for _, l := range gr.Lineas {
			datos, ok := leerRango(sinHoja(l.F))
			celda, ok2 := leerRango(strings.SplitN(strings.TrimSpace(l.Sqref), " ", 2)[0]) // un minigráfico sin celda: no ok2
			if tipo == "" || !ok || !ok2 {
				continue
			}
			out = append(out, sheetSparkline{Cell: celda.a1(), Range: datos.a1(), Type: tipo, Color: color})
		}
	}
	return out
}

func segmentacionesDeXlsx(inner []byte) []sheetSlicer {
	var s struct {
		JSON string `xml:"slicers"`
	}
	if xml.Unmarshal([]byte("<x>"+string(inner)+"</x>"), &s) != nil {
		return nil
	}
	var out []sheetSlicer
	if json.Unmarshal([]byte(s.JSON), &out) != nil {
		return nil
	}
	return out
}

// ---- gráficos ----

// anclaXlsx es un ancla de DrawingML: una celda con su desfase (y otra, o un tamaño).
type anclaXlsx struct {
	Col    int   `xml:"col"`
	ColOff int64 `xml:"colOff"`
	Row    int   `xml:"row"`
	RowOff int64 `xml:"rowOff"`
}

func (a anclaXlsx) px() (x, y float64) {
	return float64(a.Col*anchoCeldaPx) + emuAPx(a.ColOff), float64(a.Row*altoCeldaPx) + emuAPx(a.RowOff)
}

type marcoGrafico struct {
	From anclaXlsx  `xml:"from"`
	To   *anclaXlsx `xml:"to"`
	Pos  *struct {
		X int64 `xml:"x,attr"`
		Y int64 `xml:"y,attr"`
	} `xml:"pos"`
	Ext *struct {
		Cx int64 `xml:"cx,attr"`
		Cy int64 `xml:"cy,attr"`
	} `xml:"ext"`
	Marco struct {
		Nv struct {
			Desc string `xml:"descr,attr"`
		} `xml:"nvGraphicFramePr>cNvPr"`
		Chart struct {
			ID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"graphic>graphicData>chart"`
		Xfrm struct {
			Cx int64 `xml:"cx,attr"`
			Cy int64 `xml:"cy,attr"`
		} `xml:"xfrm>ext"`
	} `xml:"graphicFrame"`
}

// Las medidas por defecto de una celda de Excel y de Calc: 64 × 20 px.
const (
	anchoCeldaExcelPx = 64
	altoCeldaExcelPx  = 20
)

// ponerMedidas pone la posición y el tamaño como los guarda el editor: enteros, el
// tamaño entre sus topes, y sin tamaño si es el de por defecto (a un píxel).
func ponerMedidas(g *sheetChart, x, y, w, h float64) {
	x, y = math.Round(x), math.Round(y)
	g.X, g.Y = &x, &y
	if w <= 0 || h <= 0 || (math.Abs(w-anchoGraficoPx) <= 1 && math.Abs(h-altoGraficoPx) <= 1) {
		return
	}
	w, h = math.Min(math.Max(math.Round(w), 120), 1400), math.Min(math.Max(math.Round(h), 120), 1400)
	g.W, g.H = &w, &h
}

var reUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func idNuevo() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func graficosDeDibujo(files map[string][]byte, dibujo string) []sheetChart {
	var d struct {
		Uno  []marcoGrafico `xml:"oneCellAnchor"`
		Dos  []marcoGrafico `xml:"twoCellAnchor"`
		Abso []marcoGrafico `xml:"absoluteAnchor"`
	}
	if xml.Unmarshal(files[dibujo], &d) != nil {
		return nil
	}
	rels := relsDe(files, dibujo)
	var out []sheetChart
	for tipo, lista := range [][]marcoGrafico{d.Uno, d.Dos, d.Abso} {
		for _, m := range lista {
			if m.Marco.Chart.ID == "" {
				continue // una imagen o una forma, no un gráfico
			}
			g, ok := graficoDeXlsx(files[destinoRel(dibujo, rels[m.Marco.Chart.ID])])
			if !ok {
				continue
			}
			g.ID = m.Marco.Nv.Desc
			if !reUUID.MatchString(g.ID) {
				g.ID = idNuevo()
			}
			var x, y, w, h float64
			switch {
			case tipo == 2 && m.Pos != nil:
				x, y = emuAPx(m.Pos.X), emuAPx(m.Pos.Y)
			default:
				x, y = m.From.px()
			}
			switch {
			case m.Ext != nil:
				w, h = emuAPx(m.Ext.Cx), emuAPx(m.Ext.Cy)
			case m.Marco.Xfrm.Cx > 0 && m.Marco.Xfrm.Cy > 0:
				// Calc anota el tamaño en el `xfrm` del marco.
				w, h = emuAPx(m.Marco.Xfrm.Cx), emuAPx(m.Marco.Xfrm.Cy)
			case m.To != nil:
				// Excel deja el `xfrm` a cero: el tamaño sale de las dos celdas, con SUS
				// medidas por defecto (64 × 20 px), no con las de esta rejilla.
				w = float64((m.To.Col-m.From.Col)*anchoCeldaExcelPx) + emuAPx(m.To.ColOff-m.From.ColOff)
				h = float64((m.To.Row-m.From.Row)*altoCeldaExcelPx) + emuAPx(m.To.RowOff-m.From.RowOff)
			}
			ponerMedidas(&g, x, y, w, h)
			out = append(out, g)
		}
	}
	return out
}

// Lo que se lee de un chartSpace: una estructura laxa que admite las dos formas de
// escribir los textos y los colores.
type textoXlsx struct {
	Rich []string `xml:"tx>rich>p>r>t"`
}

type rellenoXlsx struct {
	Fill  valXlsx `xml:"solidFill>srgbClr"`
	Linea valXlsx `xml:"ln>solidFill>srgbClr"`
}

// valXlsx es un elemento con su `val` (Go no deja leer un atributo por una ruta).
type valXlsx struct {
	Val string `xml:"val,attr"`
}

func (v valXlsx) entero() int { n, _ := strconv.Atoi(v.Val); return n }

type serieLeida struct {
	Idx   valXlsx     `xml:"idx"`
	Tx    string      `xml:"tx>strRef>f"`
	SpPr  rellenoXlsx `xml:"spPr"`
	Punto []struct {
		Idx  valXlsx     `xml:"idx"`
		SpPr rellenoXlsx `xml:"spPr"`
	} `xml:"dPt"`
	Etiq *struct {
		Val valXlsx `xml:"showVal"`
		Pct valXlsx `xml:"showPercent"`
	} `xml:"dLbls"`
	Cat  string `xml:"cat>strRef>f"`
	CatN string `xml:"cat>numRef>f"`
	Val  string `xml:"val>numRef>f"`
	XVal string `xml:"xVal>numRef>f"`
	XStr string `xml:"xVal>strRef>f"`
	YVal string `xml:"yVal>numRef>f"`
}

type grupoXlsx struct {
	XMLName  xml.Name
	BarDir   valXlsx      `xml:"barDir"`
	Grouping valXlsx      `xml:"grouping"`
	Series   []serieLeida `xml:"ser"`
	Etiq     *struct {
		Val valXlsx `xml:"showVal"`
		Pct valXlsx `xml:"showPercent"`
	} `xml:"dLbls"`
	Ejes []valXlsx `xml:"axId"`
}

type ejeXlsx struct {
	XMLName  xml.Name
	ID       valXlsx   `xml:"axId"`
	Pos      valXlsx   `xml:"axPos"`
	Borrado  valXlsx   `xml:"delete"`
	Reticula *struct{} `xml:"majorGridlines"`
	Titulo   textoXlsx `xml:"title"`
}

func (e ejeXlsx) visible() bool { return e.Borrado.Val != "1" && e.Borrado.Val != "true" }

//nolint:gocognit,gocyclo // una rama por pieza de DrawingML que se lee
func graficoDeXlsx(raw []byte) (sheetChart, bool) {
	var cs struct {
		Chart struct {
			Titulo *textoXlsx `xml:"title"`
			Area   struct {
				Todo []byte `xml:",innerxml"`
			} `xml:"plotArea"`
			Leyenda *struct {
				Pos valXlsx `xml:"legendPos"`
			} `xml:"legend"`
		} `xml:"chart"`
	}
	if len(raw) == 0 || xml.Unmarshal(raw, &cs) != nil {
		return sheetChart{}, false
	}
	var area struct {
		Grupos []grupoXlsx `xml:",any"`
	}
	_ = xml.Unmarshal([]byte("<x>"+string(cs.Chart.Area.Todo)+"</x>"), &area)
	var grupos []grupoXlsx
	var ejes []ejeXlsx
	for _, gr := range area.Grupos {
		switch gr.XMLName.Local {
		case "barChart", "bar3DChart", "lineChart", "line3DChart", "areaChart", "area3DChart", "pieChart", "pie3DChart",
			"doughnutChart", "scatterChart", "radarChart":
			grupos = append(grupos, gr)
		}
	}
	var areaEjes struct {
		Ejes []ejeXlsx `xml:",any"`
	}
	_ = xml.Unmarshal([]byte("<x>"+string(cs.Chart.Area.Todo)+"</x>"), &areaEjes)
	ejes = areaEjes.Ejes
	if len(grupos) == 0 {
		return sheetChart{}, false
	}
	g := sheetChart{}
	if cs.Chart.Titulo != nil {
		g.Title = strings.Join(cs.Chart.Titulo.Rich, "")
	}
	g.Type, g.Stacked, g.LineSeries, g.SecondaryAxis = tipoDeGrupos(grupos)
	var series []serieLeida
	for _, gr := range grupos {
		series = append(series, gr.Series...)
		if gr.Etiq != nil && (gr.Etiq.Val.Val == "1" || gr.Etiq.Pct.Val == "1") {
			g.Labels = true
		}
	}
	slices.SortStableFunc(series, func(a, b serieLeida) int { return a.Idx.entero() - b.Idx.entero() })
	for _, s := range series { // Calc las escribe por serie
		if s.Etiq != nil && (s.Etiq.Val.Val == "1" || s.Etiq.Pct.Val == "1") {
			g.Labels = true
		}
	}
	rangoYSentido(&g, series)
	g.Colors = coloresDeSeries(g.Type, series)
	switch {
	case cs.Chart.Leyenda == nil:
		g.Legend = "none"
	case cs.Chart.Leyenda.Pos.Val == "t":
		g.Legend = "top"
	case cs.Chart.Leyenda.Pos.Val == "r" || cs.Chart.Leyenda.Pos.Val == "l" || cs.Chart.Leyenda.Pos.Val == "tr":
		g.Legend = "right"
	}
	reticula := false
	for _, e := range ejes {
		switch e.XMLName.Local {
		case "catAx", "valAx", "dateAx":
		default:
			continue
		}
		if !e.visible() {
			continue
		}
		t := strings.Join(e.Titulo.Rich, "")
		switch e.Pos.Val {
		case "b":
			if g.XTitle == "" {
				g.XTitle = t
			}
		case "l":
			if g.YTitle == "" {
				g.YTitle = t
			}
		}
		if e.XMLName.Local == "valAx" && e.Reticula != nil {
			reticula = true
		}
	}
	if !reticula && g.Type != "pie" && g.Type != "donut" && g.Type != "radar" {
		no := false
		g.Grid = &no
	}
	return g, g.Range != ""
}

// tipoDeGrupos decide el tipo del editor por los grupos del plotArea: columnas y líneas
// juntas son el combinado; y qué series van en línea o en el eje secundario (las de un
// grupo con otra pareja de ejes que la del primero).
//
//nolint:gocyclo // una rama por combinación de grupos de DrawingML (barras, líneas, apilado, eje secundario)
func tipoDeGrupos(grupos []grupoXlsx) (tipo string, apilado bool, lineas, secundario []int) {
	nombre := func(g grupoXlsx) string {
		switch strings.TrimSuffix(strings.TrimSuffix(g.XMLName.Local, "Chart"), "3D") {
		case "bar":
			if g.BarDir.Val == "bar" {
				return "hbar"
			}
			return "bar"
		case "line":
			return "line"
		case "area":
			return "area"
		case "pie":
			return "pie"
		case "doughnut":
			return "donut"
		case "scatter":
			return "scatter"
		}
		return "radar"
	}
	tipo = nombre(grupos[0])
	principal := 0
	if len(grupos[0].Ejes) > 1 {
		principal = grupos[0].Ejes[1].entero()
	}
	hayBarras, hayLineas := false, false
	for _, gr := range grupos {
		n := nombre(gr)
		hayBarras = hayBarras || n == "bar"
		hayLineas = hayLineas || n == "line"
		if gr.Grouping.Val == "stacked" || gr.Grouping.Val == "percentStacked" {
			apilado = true
		}
		for _, s := range gr.Series {
			if n == "line" {
				lineas = append(lineas, s.Idx.entero())
			}
			if len(gr.Ejes) > 1 && gr.Ejes[1].entero() != principal {
				secundario = append(secundario, s.Idx.entero())
			}
		}
	}
	if hayBarras && hayLineas {
		tipo = "combo"
	} else {
		lineas = nil
	}
	if tipo != "bar" && tipo != "hbar" && tipo != "area" {
		apilado = false
	}
	slices.Sort(lineas)
	slices.Sort(secundario)
	return tipo, apilado, lineas, secundario
}

// rangoYSentido reconstruye el rango de datos (la caja de todas las referencias de la
// misma hoja) y si las series van por filas (sus valores son horizontales).
func rangoYSentido(g *sheetChart, series []serieLeida) {
	var caja *rango
	sumar := func(ref string) {
		if ref == "" {
			return
		}
		r, ok := leerRango(sinHoja(ref))
		if !ok {
			return
		}
		if caja == nil {
			caja = &r
			return
		}
		caja.c1, caja.r1 = min(caja.c1, r.c1), min(caja.r1, r.r1)
		caja.c2, caja.r2 = max(caja.c2, r.c2), max(caja.r2, r.r2)
	}
	for _, s := range series {
		for _, ref := range []string{s.Tx, s.Cat, s.CatN, s.Val, s.XVal, s.XStr, s.YVal} {
			sumar(ref)
		}
		vals := s.Val
		if vals == "" {
			vals = s.YVal
		}
		if r, ok := leerRango(sinHoja(vals)); ok && r.r1 == r.r2 && r.c2 > r.c1 {
			g.ByRow = true
		}
	}
	if caja != nil {
		g.Range = caja.a1()
	}
}

// coloresDeSeries son los colores que no son el de su puesto (los que el editor guarda).
func coloresDeSeries(tipo string, series []serieLeida) []string {
	var out []string
	poner := func(k int, hex string) {
		if hex == "" || k >= len(coloresSerie) || strings.EqualFold("#"+hex, coloresSerie[k]) {
			return
		}
		for len(out) <= k {
			out = append(out, "")
		}
		out[k] = "#" + strings.ToUpper(hex)
	}
	if (tipo == "pie" || tipo == "donut") && len(series) > 0 {
		for _, p := range series[0].Punto {
			poner(p.Idx.entero(), p.SpPr.Fill.Val)
		}
		return out
	}
	for k, s := range series {
		c := s.SpPr.Fill.Val
		if c == "" {
			c = s.SpPr.Linea.Val
		}
		poner(k, c)
	}
	return out
}
