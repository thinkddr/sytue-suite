// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"math"
	"slices"
	"strconv"
	"strings"
)

// Importación de los objetos de una hoja ODF: el reverso de export_ods_objetos.go, y lo
// que escribe LibreOffice (los gráficos colgados de una celda o de la hoja, los
// minigráficos de calcext y los rangos de base de datos).

// refODSRango lee `'Hoja 1'.A1:'Hoja 1'.C5` (o `$Hoja.$A$1`) como rango sin hoja.
func refODSRango(ref string) (rango, bool) {
	partes := strings.Split(ref, ":")
	if len(partes) > 2 {
		return rango{}, false
	}
	for i, p := range partes {
		if j := strings.LastIndex(p, "."); j >= 0 {
			p = p[j+1:]
		}
		partes[i] = p
	}
	return leerRango(strings.Join(partes, ":"))
}

// partirRefsODS separa una lista de referencias de ODF por los espacios que NO van
// entre comillas (un nombre de hoja puede llevarlos: `'Ventas 2026'.A1`).
func partirRefsODS(s string) []string {
	var out []string
	var b strings.Builder
	comillas := false
	for _, c := range s {
		switch {
		case c == '\'':
			comillas = !comillas
			b.WriteRune(c)
		case c == ' ' && !comillas:
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(c)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// longitudPx lee una longitud de ODF (in, cm, mm, pt, px) en px de la rejilla.
func longitudPx(v string) float64 {
	unidades := map[string]float64{"in": 96, "cm": 96 / 2.54, "mm": 96 / 25.4, "pt": 96.0 / 72, "pc": 16, "px": 1}
	for u, f := range unidades {
		if n, ok := strings.CutSuffix(strings.TrimSpace(v), u); ok {
			x, err := strconv.ParseFloat(n, 64)
			if err != nil {
				return 0
			}
			return math.Round(x*f*100) / 100
		}
	}
	return 0
}

// objetosODS recorre content.xml y devuelve, por hoja y en orden, sus objetos.
//
//nolint:gocyclo,gocognit // un recorrido del XML con una rama por elemento que interesa
func objetosODS(files map[string][]byte) []objetosHoja {
	raw := files["content.xml"]
	if !bytes.Contains(raw, []byte("frame")) &&
		!bytes.Contains(raw, []byte("sparkline-group")) &&
		!bytes.Contains(raw, []byte("slicers")) &&
		!bytes.Contains(raw, []byte("database-range")) {
		return nil
	}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var hojas []objetosHoja
	var nombres []string
	// ordenZ es el `draw:z-index` de cada gráfico: en content.xml salen en el orden de
	// sus celdas, y el del editor es el de apilado.
	ordenZ := map[int][]int{}
	fila, col := 0, 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "table":
			if se.Name.Space != "urn:oasis:names:tc:opendocument:xmlns:table:1.0" {
				continue
			}
			hojas = append(hojas, objetosHoja{})
			nombres = append(nombres, attrLocal(se, "name"))
			fila, col = 0, 0
		case "table-row":
			fila += atoiDefault(attrLocal(se, "number-rows-repeated"), 1)
			col = 0
		case "table-cell", "covered-table-cell":
			col += atoiDefault(attrLocal(se, "number-columns-repeated"), 1)
		case "frame":
			if len(hojas) == 0 {
				continue
			}
			var fr struct {
				Z      int    `xml:"z-index,attr"`
				X      string `xml:"x,attr"`
				Y      string `xml:"y,attr"`
				W      string `xml:"width,attr"`
				H      string `xml:"height,attr"`
				Objeto struct {
					Href string `xml:"href,attr"`
				} `xml:"object"`
				Desc string `xml:"desc"`
			}
			if dec.DecodeElement(&fr, &se) != nil || fr.Objeto.Href == "" {
				continue
			}
			obj := strings.TrimSuffix(strings.TrimPrefix(fr.Objeto.Href, "./"), "/")
			g, ok := graficoDeODS(files[obj+"/content.xml"])
			if !ok {
				continue
			}
			g.ID = fr.Desc
			if !reUUID.MatchString(g.ID) {
				g.ID = idNuevo()
			}
			x, y := longitudPx(fr.X), longitudPx(fr.Y)
			if fila > 0 && col > 0 { // colgado de una celda: el desfase es desde ella
				x += float64((col - 1) * anchoCeldaPx)
				y += float64((fila - 1) * altoCeldaPx)
			}
			ponerMedidas(&g, x, y, longitudPx(fr.W), longitudPx(fr.H))
			i := len(hojas) - 1
			hojas[i].Charts = append(hojas[i].Charts, g)
			ordenZ[i] = append(ordenZ[i], fr.Z)
		case "sparkline-group":
			if len(hojas) == 0 {
				continue
			}
			tipo := map[string]string{"": "line", "line": "line", "column": "column", "stacked": "winloss"}[attrLocal(se, "type")]
			color := strings.ToUpper(attrLocal(se, "color-series"))
			if color == "#2563EB" {
				color = ""
			}
			var gr struct {
				Lineas []struct {
					Celda string `xml:"cell-address,attr"`
					Datos string `xml:"data-range,attr"`
				} `xml:"sparklines>sparkline"`
			}
			if dec.DecodeElement(&gr, &se) != nil || tipo == "" {
				continue
			}
			for _, l := range gr.Lineas {
				celda, ok1 := refODSRango(l.Celda)
				datos, ok2 := refODSRango(l.Datos)
				if ok1 && ok2 {
					i := len(hojas) - 1
					hojas[i].Sparklines = append(hojas[i].Sparklines, sheetSparkline{Cell: celda.a1(), Range: datos.a1(), Type: tipo, Color: color})
				}
			}
		case "slicers":
			if se.Name.Space != nsSytue || len(hojas) == 0 {
				continue
			}
			var s struct {
				JSON string `xml:",chardata"`
			}
			if dec.DecodeElement(&s, &se) == nil {
				var lista []sheetSlicer
				if json.Unmarshal([]byte(s.JSON), &lista) == nil {
					hojas[len(hojas)-1].Slicers = lista
				}
			}
		case "database-range":
			t, hoja, ok := tablaDeODS(se)
			if !ok {
				continue
			}
			if i := slices.Index(nombres, hoja); i >= 0 {
				hojas[i].Tables = append(hojas[i].Tables, t)
			}
		}
	}
	for i, zs := range ordenZ {
		orden := make([]int, len(zs))
		for k := range orden {
			orden[k] = k
		}
		slices.SortStableFunc(orden, func(a, b int) int { return zs[a] - zs[b] })
		graficos := make([]sheetChart, len(orden))
		for k, o := range orden {
			graficos[k] = hojas[i].Charts[o]
		}
		hojas[i].Charts = graficos
	}
	return hojas
}

// tablaDeODS lee un rango de base de datos: la tabla con cabecera si la tiene y lo que
// ODF no dice (estilo y totales) de los atributos propios, si los hay.
func tablaDeODS(se xml.StartElement) (sheetTable, string, bool) {
	nombre := attrLocal(se, "name")
	destino := attrLocal(se, "target-range-address")
	r, ok := refODSRango(destino)
	// Un rango anónimo de LibreOffice (`__Anonymous_Sheet_DB__0`) es un autofiltro, no una tabla.
	if !ok || nombre == "" || strings.HasPrefix(nombre, "__Anonymous") {
		return sheetTable{}, "", false
	}
	hoja := destino
	if i := strings.Index(destino, "."); i >= 0 {
		hoja = strings.ReplaceAll(strings.Trim(destino[:i], "'$"), "''", "'")
	}
	t := sheetTable{Name: nombre, Range: r.a1()}
	if attrLocal(se, "contains-header") == "false" {
		no := false
		t.Headers = &no
	}
	for _, a := range se.Attr {
		if a.Name.Space != nsSytue {
			continue
		}
		switch a.Name.Local {
		case "style":
			if _, ok := estiloTablaExcel[a.Value]; ok {
				t.Style = a.Value
			}
		case "totals":
			t.TotalsRow = true
			var tot map[string]string
			if json.Unmarshal([]byte(a.Value), &tot) == nil && len(tot) > 0 {
				t.Totals = tot
			}
		}
	}
	return t, hoja, true
}

// graficoDeODS lee el `office:chart` de un objeto incrustado.
//
//nolint:gocyclo,gocognit // una rama por cada cosa que el editor sabe de un gráfico
func graficoDeODS(raw []byte) (sheetChart, bool) {
	type serie struct {
		Estilo  string `xml:"style-name,attr"`
		Valores string `xml:"values-cell-range-address,attr"`
		Nombre  string `xml:"label-cell-address,attr"`
		Clase   string `xml:"class,attr"`
		Eje     string `xml:"attached-axis,attr"`
		Puntos  []struct {
			Estilo string `xml:"style-name,attr"`
		} `xml:"data-point"`
	}
	type eje struct {
		Dim    string    `xml:"dimension,attr"`
		Nombre string    `xml:"name,attr"`
		Titulo []string  `xml:"title>p"`
		Grid   *struct{} `xml:"grid"`
		Cat    struct {
			Rango string `xml:"cell-range-address,attr"`
		} `xml:"categories"`
	}
	var doc struct {
		Estilos []struct {
			Nombre string `xml:"name,attr"`
			Props  struct {
				Apilado  string `xml:"stacked,attr"`
				Vertical string `xml:"vertical,attr"`
				Etiqueta string `xml:"data-label-number,attr"`
			} `xml:"chart-properties"`
			Grafico struct {
				Relleno string `xml:"fill-color,attr"`
				Trazo   string `xml:"stroke-color,attr"`
			} `xml:"graphic-properties"`
		} `xml:"automatic-styles>style"`
		Chart struct {
			Clase   string   `xml:"class,attr"`
			Titulo  []string `xml:"title>p"`
			Leyenda *struct {
				Pos string `xml:"legend-position,attr"`
			} `xml:"legend"`
			Area struct {
				Estilo string  `xml:"style-name,attr"`
				Rango  string  `xml:"cell-range-address,attr"`
				Ejes   []eje   `xml:"axis"`
				Series []serie `xml:"series"`
			} `xml:"plot-area"`
		} `xml:"body>chart>chart"`
	}
	if len(raw) == 0 || xml.Unmarshal(raw, &doc) != nil || doc.Chart.Clase == "" {
		return sheetChart{}, false
	}
	estilo := func(n string) (apilado, vertical bool, etiqueta, relleno, trazo string) {
		for _, e := range doc.Estilos {
			if e.Nombre == n {
				return e.Props.Apilado == "true", e.Props.Vertical == "true", e.Props.Etiqueta, e.Grafico.Relleno, e.Grafico.Trazo
			}
		}
		return false, false, "", "", ""
	}
	g := sheetChart{Title: strings.Join(doc.Chart.Titulo, " ")}
	apilado, vertical, etiquetaArea, _, _ := estilo(doc.Chart.Area.Estilo)
	g.Labels = etiquetaArea != "" && etiquetaArea != "none"
	switch strings.TrimPrefix(doc.Chart.Clase, "chart:") {
	case "bar":
		g.Type = "bar"
		if vertical {
			g.Type = "hbar"
		}
	case "line":
		g.Type = "line"
	case "area":
		g.Type = "area"
	case "circle":
		g.Type = "pie"
	case "ring":
		g.Type = "donut"
	case "scatter", "bubble":
		g.Type = "scatter"
	case "radar", "filled-radar", "net", "filled-net":
		g.Type = "radar"
	default:
		return sheetChart{}, false
	}
	if apilado && (g.Type == "bar" || g.Type == "hbar" || g.Type == "area") {
		g.Stacked = true
	}
	hayBarras, hayLineas := false, false
	var caja *rango
	sumar := func(ref string) {
		for _, parte := range partirRefsODS(ref) {
			r, ok := refODSRango(parte)
			if !ok {
				continue
			}
			if caja == nil {
				caja = &r
				continue
			}
			caja.c1, caja.r1, caja.c2, caja.r2 = min(caja.c1, r.c1), min(caja.r1, r.r1), max(caja.c2, r.c2), max(caja.r2, r.r2)
		}
	}
	for k, s := range doc.Chart.Area.Series {
		clase := strings.TrimPrefix(s.Clase, "chart:")
		if clase == "" {
			clase = strings.TrimPrefix(doc.Chart.Clase, "chart:")
		}
		hayBarras = hayBarras || clase == "bar"
		if clase == "line" {
			hayLineas = true
			g.LineSeries = append(g.LineSeries, k)
		}
		if s.Eje == "secondary-y" {
			g.SecondaryAxis = append(g.SecondaryAxis, k)
		}
		sumar(s.Valores)
		sumar(s.Nombre)
		if r, ok := refODSRango(s.Valores); ok && r.r1 == r.r2 && r.c2 > r.c1 {
			g.ByRow = true
		}
		_, _, etiqueta, relleno, trazo := estilo(s.Estilo)
		if etiqueta != "" && etiqueta != "none" {
			g.Labels = true
		}
		color := relleno
		if color == "" {
			color = trazo
		}
		if g.Type == "pie" || g.Type == "donut" {
			for i, p := range s.Puntos {
				_, _, et, rell, _ := estilo(p.Estilo)
				if et != "" && et != "none" {
					g.Labels = true
				}
				g.Colors = ponerColor(g.Colors, i, rell)
			}
			continue
		}
		g.Colors = ponerColor(g.Colors, k, color)
	}
	if !hayBarras || !hayLineas || g.Type != "bar" {
		g.LineSeries = nil
	}
	if hayBarras && hayLineas && g.Type == "bar" {
		g.Type = "combo"
	}
	for _, e := range doc.Chart.Area.Ejes {
		sumar(e.Cat.Rango)
		switch e.Nombre {
		case "primary-x":
			g.XTitle = strings.Join(e.Titulo, " ")
		case "primary-y":
			g.YTitle = strings.Join(e.Titulo, " ")
			if e.Grid == nil && g.Type != "pie" && g.Type != "donut" && g.Type != "radar" {
				no := false
				g.Grid = &no
			}
		}
	}
	if doc.Chart.Area.Rango != "" {
		caja = nil
		sumar(doc.Chart.Area.Rango)
	}
	switch {
	case doc.Chart.Leyenda == nil:
		g.Legend = "none"
	case doc.Chart.Leyenda.Pos == "top":
		g.Legend = "top"
	case doc.Chart.Leyenda.Pos == "end" || doc.Chart.Leyenda.Pos == "start":
		g.Legend = "right"
	}
	if caja == nil {
		return sheetChart{}, false
	}
	g.Range = caja.a1()
	return g, true
}

// ponerColor guarda el color k si no es el de su puesto.
func ponerColor(colores []string, k int, hex string) []string {
	hex = strings.ToUpper(hex)
	if hex == "" || k >= len(coloresSerie) || hex == coloresSerie[k] {
		return colores
	}
	for len(colores) <= k {
		colores = append(colores, "")
	}
	colores[k] = hex
	return colores
}
