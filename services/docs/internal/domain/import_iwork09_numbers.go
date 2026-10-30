// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"html"
	"strconv"
	"strings"
	"time"
)

// # iWork '09: tablas y hojas .numbers
//
// Una tabla (`sf:tabular-model[sf:name]`) es igual en las tres aplicaciones: `sf:grid
// [sf:numrows, sf:numcols]` > `sf:datasource` con las celdas en orden. Cada celda está en
// la posición anterior + 1 o, si trae `sf:ct`, en la anterior + `sf:ct` (la primera parte
// de 0); fila = posición / 256 y columna = posición % 256. Comprobado en las 39 tablas del
// corpus: todas caen dentro de su rejilla y ninguna se repite.
//
// Tipos de celda: `sf:t` texto (`sf:ct sfa:s`), `sf:n` número (`sf:v`), `sf:d` fecha
// (`sf:cell-date`, segundos desde 2001-01-01), `sf:f` fórmula (`sf:fo sf:fs` y el
// resultado en `sf:r` > `sf:rn sf:v`, sin `sf:v` si es 0), `sf:pm` menú desplegable
// (`sf:proxied-cell-ref` → la `sf:t` elegida), `sf:g` vacía, `sf:s` cubierta por una
// combinada y `sf:cb` casilla.

// celda09 es el valor de una celda tal como se ve.
type celda09 struct {
	texto string
	// num: número, y texto es su valor; fecha: texto es AAAA-MM-DD y serial su número de
	// serie de hoja de cálculo.
	num, fecha bool
	serial     float64
	formato    numFmtSpec
	estilo     cellStyle
}

type tabla09 struct {
	nombre      string
	filas, cols int
	celdas      map[[2]int]celda09
	combinadas  []string
	// formulas son las celdas con fórmula: sale su valor calculado (decisión D2).
	formulas int
}

// epoca09 es el origen de las fechas de iWork; su número de serie es 36892.
var epoca09 = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

const serialEpoca09 = 36892

// primerModelo09 es la primera tabla del subárbol, o nil.
func primerModelo09(n *nodo09) *nodo09 {
	var m *nodo09
	n.recorrer(func(x *nodo09) bool {
		if m == nil && x.nombre == "sf:tabular-model" {
			m = x
		}
		return m == nil
	})
	return m
}

// leerTabla09 lee una tabla; nil si no tiene rejilla o celdas que se lean.
func leerTabla09(m *nodo09, est estilos09) *tabla09 {
	grid := m.hijo("sf:grid")
	ds := grid.hijo("sf:datasource")
	if ds == nil {
		return nil
	}
	nr, _ := strconv.Atoi(grid.attr["sf:numrows"])
	nc, _ := strconv.Atoi(grid.attr["sf:numcols"])
	if nr <= 0 || nc <= 0 || nc > 256 || nr > maxFilasHoja {
		return nil
	}
	t := &tabla09{nombre: m.attr["sf:name"], filas: nr, cols: nc, celdas: map[[2]int]celda09{}}
	// Las opciones de los menús, por su id: la celda solo dice cuál está elegida.
	opciones := map[string]string{}
	ds.recorrer(func(n *nodo09) bool {
		if n.nombre == "sf:t" && n.attr["sfa:ID"] != "" {
			opciones[n.attr["sfa:ID"]] = textoCelda09(n)
		}
		return true
	})
	pos := -1
	for _, c := range ds.hijos {
		if c.nombre == "" {
			continue
		}
		if ct, err := strconv.Atoi(c.attr["sf:ct"]); err == nil {
			pos = max(pos, 0) + ct
		} else {
			pos++
		}
		f, col := pos/256, pos%256
		if pos < 0 || f >= nr || col >= nc {
			continue
		}
		if c.nombre == "sf:f" {
			t.formulas++
		}
		if rg := rangoCombinada09(c, f, col, nr, nc); rg != "" {
			t.combinadas = append(t.combinadas, rg)
		}
		if v := valorCelda09(c, opciones); v.texto != "" {
			aplicarEstiloCelda09(&v, c, est)
			t.celdas[[2]int{f, col}] = v
		}
	}
	if len(t.celdas) == 0 {
		return nil
	}
	return t
}

func rangoCombinada09(c *nodo09, f, col, nr, nc int) string {
	cs, _ := strconv.Atoi(c.attr["sf:col-span"])
	rs, _ := strconv.Atoi(c.attr["sf:row-span"])
	if cs <= 1 && rs <= 1 {
		return ""
	}
	r2 := min(f+max(rs, 1)-1, nr-1)
	c2 := min(col+max(cs, 1)-1, nc-1)
	if r2 <= f && c2 <= col {
		return ""
	}
	return A1Ref(f+1, col+1) + ":" + A1Ref(r2+1, c2+1)
}

func textosStrings09(n *nodo09) []string {
	var out []string
	for _, h := range n.hijosDe() {
		if h.nombre == "sf:string" || h.nombre == "sfa:string" {
			out = append(out, h.attr["sfa:string"])
		}
	}
	return out
}

func seriesGrafico09(datosNodo *nodo09) [][]celda09 {
	var series [][]celda09
	for _, arr := range datosNodo.hijosDe() {
		if arr.nombre != "sf:mutable-array" && arr.nombre != "sfa:mutable-array" {
			continue
		}
		var s []celda09
		for _, v := range arr.hijos {
			switch v.nombre {
			case "sf:number", "sfa:number":
				s = append(s, numero09(v.attr["sfa:number"]))
			case "sf:string", "sfa:string":
				s = append(s, celda09{texto: v.attr["sfa:string"]})
			case "sf:null", "sfa:null":
				s = append(s, celda09{})
			}
		}
		series = append(series, s)
	}
	return series
}

func transponerSeries(series [][]celda09, minFilas int) [][]celda09 {
	nr := minFilas
	for _, s := range series {
		nr = max(nr, len(s))
	}
	datos := make([][]celda09, nr)
	for r := range datos {
		datos[r] = make([]celda09, len(series))
		for c, s := range series {
			if r < len(s) {
				datos[r][c] = s[r]
			}
		}
	}
	return datos
}

// leerGrafico09 convierte un gráfico (`sf:chart-info`) en una tabla con su título,
// etiquetas de fila y columna y datos en caché (`sf:cached-data`). nil si no trae datos.
func leerGrafico09(n *nodo09) *tabla09 {
	if n == nil {
		return nil
	}
	var modelo *nodo09
	n.recorrer(func(x *nodo09) bool {
		if modelo == nil && (x.nombre == "sf:formula-chart-model" || x.nombre == "sf:chart-model") {
			modelo = x
		}
		return modelo == nil
	})
	if modelo == nil {
		return nil
	}
	titulo := strings.TrimSpace(modelo.hijo("sf:chart-name").attr["sfa:string"])
	cols := textosStrings09(modelo.hijo("sf:chart-column_names"))
	filas := textosStrings09(modelo.hijo("sf:chart-row_names"))
	datosNodo := modelo.hijo("sf:cached-data")
	if datosNodo == nil {
		datosNodo = modelo.hijo("sf:chart-data")
	}
	series := seriesGrafico09(datosNodo)
	datos := series
	porCol := modelo.attr["sf:chart-direction"] == "1" || (len(series) > 0 && len(series) == len(cols) && len(series) != len(filas))
	if porCol && len(series) > 0 {
		datos = transponerSeries(series, len(filas))
	}
	return armarTablaGrafico(titulo, filas, cols, datos)
}

func armarTablaGrafico(titulo string, filas, cols []string, datos [][]celda09) *tabla09 {
	nr := max(len(filas), len(datos))
	nc := len(cols)
	for _, f := range datos {
		nc = max(nc, len(f))
	}
	if nr == 0 && nc == 0 {
		return nil
	}
	offF, offC := 0, 0
	if len(cols) > 0 || (titulo != "" && len(filas) == 0) {
		offF = 1
	}
	if len(filas) > 0 {
		offC = 1
	}
	nombre := strings.TrimSpace(titulo)
	if nombre == "" {
		nombre = "Gráfico"
	}
	t := &tabla09{
		nombre: nombre,
		filas:  min(nr+offF, maxFilasHoja),
		cols:   min(max(nc+offC, 1), maxColumnasHoja),
		celdas: map[[2]int]celda09{},
	}
	if titulo != "" && (offF > 0 || offC > 0) {
		t.celdas[[2]int{0, 0}] = celda09{texto: titulo}
	}
	volcarDatosGrafico(t, offF, offC, filas, cols, datos)
	if len(t.celdas) == 0 {
		return nil
	}
	return t
}

func volcarDatosGrafico(t *tabla09, offF, offC int, filas, cols []string, datos [][]celda09) {
	for c, s := range cols {
		if offC+c < t.cols && strings.TrimSpace(s) != "" {
			t.celdas[[2]int{0, offC + c}] = celda09{texto: s}
		}
	}
	for r, s := range filas {
		if offF+r < t.filas && strings.TrimSpace(s) != "" {
			t.celdas[[2]int{offF + r, 0}] = celda09{texto: s}
		}
	}
	for r, fila := range datos {
		if offF+r >= t.filas {
			break
		}
		for c, v := range fila {
			if offC+c >= t.cols {
				break
			}
			if v.texto != "" {
				t.celdas[[2]int{offF + r, offC + c}] = v
			}
		}
	}
}

// valorCelda09 es el valor de una celda tal como se ve; vacío si no tiene.
func valorCelda09(c *nodo09, opciones map[string]string) celda09 {
	switch c.nombre {
	case "sf:t":
		return celda09{texto: textoCelda09(c)}
	case "sf:n":
		return numero09(c.attr["sf:v"])
	case "sf:d":
		return fecha09(c.attr["sf:cell-date"])
	case "sf:du", "sf:span":
		s := c.attr["sf:v"]
		if s == "" {
			s = c.attr["sf:du"]
		}
		if seg, err := strconv.ParseFloat(s, 64); err == nil {
			return celda09{texto: duracionIWA(seg, nil, false)}
		}
	case "sf:f":
		r := c.hijo("sf:r")
		if rn := r.hijo("sf:rn"); rn != nil {
			return numero09(rn.attr["sf:v"])
		}
		if rt := r.hijo("sf:rt"); rt != nil {
			return celda09{texto: textoCelda09(rt)}
		}
		if rd := r.hijo("sf:rd"); rd != nil {
			return fecha09(rd.attr["sf:cell-date"])
		}
		if rb := r.hijo("sf:rb"); rb != nil {
			v := rb.attr["sf:v"]
			return celda09{texto: strings.ToUpper(strconv.FormatBool(v == "1" || strings.EqualFold(v, "true")))}
		}
	case "sf:pm":
		return celda09{texto: opciones[c.hijo("sf:proxied-cell-ref").attr["sfa:IDREF"]]}
	case "sf:cb":
		v := c.attr["sf:v"]
		if v == "" {
			if n := c.hijo("sf:n"); n != nil {
				v = n.attr["sf:v"]
			}
		}
		return celda09{texto: strings.ToUpper(strconv.FormatBool(v == "1" || strings.EqualFold(v, "true")))}
	}
	return celda09{}
}

func aplicarEstiloCelda09(v *celda09, c *nodo09, est estilos09) {
	idEstilo := c.attr["sf:s"]
	if fill := colorNodo09(est.prop("fill", idEstilo)); fill != "" && fill != "FFFFFF" {
		v.estilo.Fill = "#" + fill
	}
	if col := est.color(idEstilo); col != "" {
		v.estilo.Color = "#" + col
	}
	v.estilo.U = est.num("underline", idEstilo) != 0
	v.estilo.S = est.num("strikethru", idEstilo) != 0
	if !v.num {
		return
	}
	nf := c.hijo("sf:cf").hijo("sf:number-format")
	if nf == nil {
		nf = c.hijo("sf:r").hijo("sf:rn").hijo("sf:cf").hijo("sf:number-format")
	}
	ft := int(est.num("SFTCellStylePropertyFormatType", idEstilo))
	if nf == nil && (ft == 0 || ft == 256 || ft == 257 || ft == 258) {
		nf = est.prop("SFTCellStylePropertyNumberFormat", idEstilo)
	}
	v.formato = formatoNumero09(nf, ft)
}

func formatoNumero09(nf *nodo09, ft int) numFmtSpec {
	if nf == nil || nf.nombre != "sf:number-format" {
		return numFmtSpec{}
	}
	tipo, err := strconv.Atoi(nf.attr["sf:format-type"])
	if err != nil {
		tipo = max(ft-256, 0)
	}
	dec, errDec := strconv.Atoi(nf.attr["sf:format-decimal-places"])
	miles := nf.attr["sf:format-show-thousands-separator"] == "true"
	return especFormatoNumero(tipo, dec, errDec == nil, miles)
}

func especFormatoNumero(tipo, dec int, hayDec, miles bool) numFmtSpec {
	okDec := hayDec && dec >= 0 && dec <= 8
	switch tipo {
	case 1:
		if !okDec {
			dec = 2
		}
		return numFmtSpec{ID: "currency", Decimals: &dec}
	case 2:
		if !okDec {
			dec = 0
		}
		return numFmtSpec{ID: "percent", Decimals: &dec}
	case 0:
		if miles {
			if !okDec {
				dec = 2
			}
			return numFmtSpec{ID: "thousands", Decimals: &dec}
		}
		if okDec {
			return numFmtSpec{ID: "number", Decimals: &dec}
		}
	}
	return numFmtSpec{}
}

// textoCelda09 es el texto de una celda de texto: sus `sf:ct sfa:s`.
func textoCelda09(c *nodo09) string {
	var b strings.Builder
	for _, h := range c.hijos {
		if h.nombre == "sf:ct" {
			b.WriteString(h.attr["sfa:s"])
		}
	}
	return b.String()
}

// numero09 es un número escrito como se ve: «-97.400000000000006» es -97.4. Sin valor, 0.
func numero09(s string) celda09 {
	if s == "" {
		return celda09{texto: "0", num: true}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return celda09{texto: s}
	}
	return celda09{texto: strconv.FormatFloat(f, 'f', -1, 64), num: true}
}

func fecha09(s string) celda09 {
	seg, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return celda09{}
	}
	t := epoca09.Add(time.Duration(seg * float64(time.Second)))
	texto := t.Format("2006-01-02")
	if t.Hour() != 0 || t.Minute() != 0 {
		texto = t.Format("2006-01-02 15:04")
	}
	return celda09{texto: texto, fecha: true, serial: seg/86400 + serialEpoca09}
}

// xtable es la tabla como tabla de un documento: el texto de cada celda tal como se ve.
func (t *tabla09) xtable() *xtable {
	out := &xtable{cols: t.cols, rows: make([][]xcell, t.filas)}
	for f := range out.rows {
		out.rows[f] = make([]xcell, t.cols)
		for c := range out.rows[f] {
			out.rows[f][c].paras = []xpara{{runs: []xrun{{text: t.celdas[[2]int{f, c}].texto}}}}
		}
	}
	return out
}

// importNumbers09 convierte una hoja .numbers '09: una hoja del editor por tabla, con el
// nombre de su hoja (y el de la tabla si la hoja tiene varias). Las fórmulas salen con
// su valor y lo que no se convierte (gráficos, imágenes) se cuenta para el aviso.
//
// `ls:document` > `ls:workspace-array` > `ls:workspace[ls:workspace-name]` > … >
// `sf:tabular-model`. Las de `ls:tabular-prototypes` son las tablas de muestra de la
// plantilla.
func importNumbers09(raiz *nodo09) (m *sheetModel, formulas int, omitido bool) {
	est := leerEstilos09(raiz)
	m = &sheetModel{}
	nombres := map[string]bool{}
	for _, ws := range raiz.hijo("ls:workspace-array").hijosDe() {
		if ws.nombre != "ls:workspace" {
			continue
		}
		var tablas []*tabla09
		ws.recorrer(func(n *nodo09) bool {
			switch n.nombre {
			case "sf:tabular-model":
				if t := leerTabla09(n, est); t != nil {
					tablas = append(tablas, t)
				}
				return false
			case "sf:chart-info":
				if t := leerGrafico09(n); t != nil {
					tablas = append(tablas, t)
				} else {
					omitido = true
				}
				return false
			case "sf:image", "sf:media":
				omitido = true
				return false
			}
			return true
		})
		for _, t := range tablas {
			nombre := ws.attr["ls:workspace-name"]
			if len(tablas) > 1 && t.nombre != "" {
				nombre += " · " + t.nombre
			}
			formulas += t.formulas
			m.Sheets = append(m.Sheets, t.hoja(nombreUnico(nombre, nombres)))
		}
	}
	return m, formulas, omitido
}

// hoja es la tabla como hoja del editor.
func (t *tabla09) hoja(nombre string) sheetData {
	sd := sheetData{Name: nombre, Cells: map[string]string{}}
	sd.Merges = combinadasDeRangos(sanearCombinadas(t.combinadas))
	for pos, v := range t.celdas {
		ref := A1Ref(pos[0]+1, pos[1]+1)
		switch {
		case v.fecha:
			sd.Cells[ref] = textoSerial(v.serial)
			if sd.Formats == nil {
				sd.Formats = map[string]numFmtSpec{}
			}
			sd.Formats[ref] = numFmtSpec{ID: "date"}
		default:
			sd.Cells[ref] = html.EscapeString(v.texto)
			if v.formato.ID != "" {
				if sd.Formats == nil {
					sd.Formats = map[string]numFmtSpec{}
				}
				sd.Formats[ref] = v.formato
			}
		}
		if v.estilo.Color != "" || v.estilo.Fill != "" || v.estilo.U || v.estilo.S {
			if sd.Styles == nil {
				sd.Styles = map[string]cellStyle{}
			}
			sd.Styles[ref] = v.estilo
		}
	}
	return sd
}

// nombreUnico es `nombre`, o «nombre 2», «nombre 3»… si ya está usado; «Hoja» si viene vacío.
func nombreUnico(nombre string, usados map[string]bool) string {
	if strings.TrimSpace(nombre) == "" {
		nombre = "Hoja"
	}
	n, base := nombre, nombre
	for i := 2; usados[n]; i++ {
		n = base + " " + strconv.Itoa(i)
	}
	usados[n] = true
	return n
}

// hojaDeAviso es la hoja que va delante cuando una hoja de cálculo no se convierte entera:
// las fórmulas salen con su valor (decisión D2) y lo que no se convierte no está.
func hojaDeAviso(f srcFormat, idioma string, formulas int, omitido bool, usados map[string]bool) sheetData {
	var lineas []string
	if formulas > 0 {
		lineas = append(lineas, avisoImportar(idioma, "texto.formulas"))
	}
	if omitido {
		lineas = append(lineas, avisoImportar(idioma, "texto.parcial_hoja"))
	}
	return hojaConAviso(idioma, f, lineas, usados)
}

// hojaConAviso es la hoja «Aviso» que va delante de una hoja de cálculo que no se abre
// entera: el título de la conversión parcial en A1 y una línea por fila debajo.
func hojaConAviso(idioma string, f srcFormat, lineas []string, usados map[string]bool) sheetData {
	t := avisoImportar(idioma, "titulo.parcial")
	if tipo, ok := tiposIWork[f]; ok {
		t += " · " + avisoImportar(idioma, tipo)
	}
	sd := sheetData{Name: nombreUnico(avisoImportar(idioma, "hoja.aviso"), usados), Cells: map[string]string{
		"A1": "<b>" + html.EscapeString(t) + "</b>",
	}}
	for i, l := range lineas {
		sd.Cells[A1Ref(i+2, 1)] = html.EscapeString(l)
	}
	return sd
}
