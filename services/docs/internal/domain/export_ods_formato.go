// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"slices"
	"strconv"
	"strings"
)

// Exportación a ODF del formato de una hoja (ver sheet_formato.go): los estilos de celda
// (automáticos, en content.xml), las validaciones (`table:content-validations`, que van
// antes de las tablas y a las que cada celda apunta por nombre) y el formato condicional
// (`calcext:conditional-formats`, al final de la tabla).

// odsStyles registra los estilos automáticos de celda y los estilos de datos de ODF.
type odsStyles struct {
	seen  map[string]string // clave del estilo -> nombre del estilo de celda
	datos map[string]string // formato de número -> nombre del estilo de datos
	cf    map[string]string // relleno de formato condicional -> nombre del estilo de celda
	defs  strings.Builder
	// nombrados son los estilos con nombre (styles.xml): LibreOffice solo resuelve ahí los de
	// formato condicional.
	nombrados strings.Builder
	n         int
}

func newOdsStyles() *odsStyles {
	return &odsStyles{seen: map[string]string{}, datos: map[string]string{}, cf: map[string]string{}}
}

func (o *odsStyles) estiloDatos(spec numFmtSpec) string {
	if spec.ID == "general" {
		return ""
	}
	clave := spec.ID + ":" + strconv.Itoa(fmtDecimals(spec))
	if n, ok := o.datos[clave]; ok {
		return n
	}
	o.n++
	n := "N" + strconv.Itoa(o.n)
	o.datos[clave] = n
	o.defs.WriteString(odfDataStyle(n, spec))
	return n
}

// ladoODS es un borde de celda de ODF: su grosor y su estilo por el trazo del editor (los de
// Calc para fino, medio y grueso), y su color, negro si no dice otro.
func ladoODS(tag string, on bool, trazo, color string) string {
	if !on {
		return ""
	}
	linea := map[string]string{
		"medium": "1.76pt solid", "thick": "2.49pt solid", "dashed": "0.74pt dashed",
		"dotted": "0.74pt dotted", "double": "2.49pt double",
	}[trazo]
	if linea == "" {
		linea = "0.74pt solid"
	}
	col := strings.ToLower(colorHex(color))
	if col == "" {
		col = "#000000"
	}
	return ` fo:border-` + tag + `="` + linea + ` ` + col + `"`
}

// styleFor es el nombre del estilo de celda de ODF de una celda ("" si no lleva ninguno):
// su formato de número, su fuente, su relleno, sus bordes y su alineación.
//
//nolint:gocognit,gocyclo // traductor de un formato a otro: un caso por tipo, y trocearlo reparte la tabla sin simplificarla
func (o *odsStyles) styleFor(c cellVal) string {
	st := c.estilo
	var celdaP, parrafoP, textoP strings.Builder
	if col := colorHex(st.Fill); col != "" {
		celdaP.WriteString(` fo:background-color="` + strings.ToLower(col) + `"`)
	}
	if b := st.Border; b != nil {
		celdaP.WriteString(ladoODS("top", b.T, b.TS, b.TC) + ladoODS("bottom", b.B, b.BS, b.BC) +
			ladoODS("left", b.L, b.LS, b.LC) + ladoODS("right", b.R, b.RS, b.RC))
	}
	if v := map[string]string{"top": "top", "middle": "middle", "bottom": "bottom"}[st.Valign]; v != "" {
		celdaP.WriteString(` style:vertical-align="` + v + `"`)
	}
	if st.Wrap {
		celdaP.WriteString(` fo:wrap-option="wrap"`)
	}
	switch {
	case st.desbloqueada() && st.Hidden:
		celdaP.WriteString(` style:cell-protect="formula-hidden"`)
	case st.desbloqueada():
		celdaP.WriteString(` style:cell-protect="none"`)
	case st.Hidden:
		celdaP.WriteString(` style:cell-protect="hidden-and-protected"`)
	}
	switch c.alinear {
	case "center":
		parrafoP.WriteString(` fo:text-align="center"`)
	case "right":
		parrafoP.WriteString(` fo:text-align="end"`)
	}
	if c.negrita {
		textoP.WriteString(` fo:font-weight="bold"`)
	}
	if c.cursiva {
		textoP.WriteString(` fo:font-style="italic"`)
	}
	if col := colorHex(st.Color); col != "" {
		textoP.WriteString(` fo:color="` + strings.ToLower(col) + `"`)
	}
	if st.Font != "" {
		textoP.WriteString(` fo:font-family="` + xmlEscape(st.Font) + `" style:font-name="` + xmlEscape(st.Font) + `"`)
	}
	if st.Size > 0 {
		textoP.WriteString(` fo:font-size="` + strconv.Itoa(st.Size) + `pt"`)
	}
	if st.U {
		textoP.WriteString(` style:text-underline-style="solid" style:text-underline-width="auto" style:text-underline-color="font-color"`)
	}
	if st.S {
		textoP.WriteString(` style:text-line-through-style="solid"`)
	}
	datos := ""
	if c.fmt != nil {
		datos = o.estiloDatos(*c.fmt)
	}
	cuerpo := ""
	if celdaP.Len() > 0 {
		cuerpo += `<style:table-cell-properties` + celdaP.String() + `/>`
	}
	if parrafoP.Len() > 0 {
		cuerpo += `<style:paragraph-properties` + parrafoP.String() + `/>`
	}
	if textoP.Len() > 0 {
		cuerpo += `<style:text-properties` + textoP.String() + `/>`
	}
	if datos == "" && cuerpo == "" {
		return ""
	}
	clave := datos + "|" + cuerpo
	if n, ok := o.seen[clave]; ok {
		return n
	}
	o.n++
	n := "ce" + strconv.Itoa(o.n)
	o.seen[clave] = n
	o.defs.WriteString(`<style:style style:name="` + n + `" style:family="table-cell"`)
	if datos != "" {
		o.defs.WriteString(` style:data-style-name="` + datos + `"`)
	}
	if cuerpo == "" {
		o.defs.WriteString(`/>`)
	} else {
		o.defs.WriteString(`>` + cuerpo + `</style:style>`)
	}
	return n
}

// estiloCondicional es el estilo de celda (el relleno y el color de letra) al que apunta una
// regla de formato condicional.
func (o *odsStyles) estiloCondicional(fill, tinta string) string {
	col, letra := strings.ToLower(colorHex(fill)), strings.ToLower(colorHex(tinta))
	clave := col + "|" + letra
	if n, ok := o.cf[clave]; ok {
		return n
	}
	o.n++
	n := "cf" + strconv.Itoa(o.n)
	o.cf[clave] = n
	o.nombrados.WriteString(`<style:style style:name="` + n + `" style:family="table-cell" style:parent-style-name="Default">`)
	if col != "" {
		o.nombrados.WriteString(`<style:table-cell-properties fo:background-color="` + col + `"/>`)
	}
	if letra != "" {
		o.nombrados.WriteString(`<style:text-properties fo:color="` + letra + `"/>`)
	}
	o.nombrados.WriteString(`</style:style>`)
	return n
}

// valoresODS es una lista de valores como argumentos de OpenFormula: `"Norte";"Sur"`.
func valoresODS(vs []string) string {
	q := make([]string, len(vs))
	for i, v := range vs {
		q[i] = `"` + strings.ReplaceAll(v, `"`, `""`) + `"`
	}
	return strings.Join(q, ";")
}

var opODS = map[string]string{"gt": ">", "ge": ">=", "lt": "<", "le": "<=", "eq": "=", "ne": "!="}

// condicionValidacionODS es el `table:condition` de una validación; "" si no se puede.
func condicionValidacionODS(v dataValidation) string {
	switch v.Type {
	case "", "list":
		if len(v.Values) == 0 {
			return ""
		}
		return "of:cell-content-is-in-list(" + valoresODS(v.Values) + ")"
	case "formula":
		f := strings.TrimPrefix(strings.TrimSpace(v.Formula), "=")
		if f == "" {
			return ""
		}
		return "of:is-true-formula(" + a1AODF(f, false) + ")"
	case "number", "date", "textLength":
		a, ok := operandoValidacion(v.Type, v.A)
		if !ok {
			return ""
		}
		tipo := map[string]string{"number": "cell-content-is-decimal-number()", "date": "cell-content-is-date()", "textLength": ""}[v.Type]
		if v.Type == "number" && v.Integer {
			tipo = "cell-content-is-whole-number()"
		}
		var cmp string
		switch v.Op {
		case "between", "notBetween":
			b, ok := operandoValidacion(v.Type, v.B)
			if !ok {
				return ""
			}
			nombre := map[string]string{"between": "cell-content-is-between", "notBetween": "cell-content-is-not-between"}[v.Op]
			if v.Type == "textLength" {
				nombre = map[string]string{"between": "cell-content-text-length-is-between", "notBetween": "cell-content-text-length-is-not-between"}[v.Op]
			}
			cmp = nombre + "(" + a + "," + b + ")" // LibreOffice escribe y lee la coma (un decimal lleva punto)
		default:
			op := opODS[v.Op]
			if op == "" {
				return ""
			}
			cmp = "cell-content()" + op + a
			if v.Type == "textLength" {
				cmp = "cell-content-text-length()" + op + a
			}
		}
		if tipo == "" {
			return "of:" + cmp
		}
		return "of:" + tipo + " and " + cmp
	}
	return ""
}

// validacionesODS son las `table:content-validation` de todas las hojas.
type validacionesODS struct {
	xml strings.Builder
	n   int
}

// registrar escribe las validaciones de la hoja y devuelve, por celda, el nombre de la que
// la cubre (la última que la cubre gana, como en el editor). ODF pide el nombre en cada
// celda: un rango enorme se acota a maxCeldasVacias celdas.
func (v *validacionesODS) registrar(sd sheetData, hoja string) map[string]string {
	var por map[string]string
	presupuesto := maxCeldasVacias
	for _, dv := range sd.Validations {
		rg, ok := leerRango(dv.Range)
		cond := condicionValidacionODS(dv)
		if !ok || !rg.valido() || cond == "" {
			continue
		}
		v.n++
		nombre := "val" + strconv.Itoa(v.n)
		aviso := "stop"
		if !dv.Strict {
			aviso = "warning"
		}
		v.xml.WriteString(`<table:content-validation table:name="` + nombre + `" table:condition="` + xmlEscape(cond) +
			`" table:allow-empty-cell="true" table:display-list="unsorted" table:base-cell-address="` +
			xmlEscape(rango{c1: rg.c1, r1: rg.r1, c2: rg.c1, r2: rg.r1}.refODS(hoja)) + `">`)
		if e := strings.TrimSpace(dv.Error); e != "" {
			v.xml.WriteString(`<table:error-message table:message-type="` + aviso + `" table:display="true"><text:p>` + xmlEscape(e) + `</text:p></table:error-message>`)
		} else {
			v.xml.WriteString(`<table:error-message table:message-type="` + aviso + `" table:display="true"/>`)
		}
		v.xml.WriteString(`</table:content-validation>`)
		if por == nil {
			por = map[string]string{}
		}
		for r := rg.r1; r <= rg.r2 && presupuesto > 0; r++ {
			for c := rg.c1; c <= rg.c2 && presupuesto > 0; c++ {
				presupuesto--
				por[A1Ref(r, c)] = nombre
			}
		}
	}
	return por
}

func (v *validacionesODS) cabecera() string {
	if v.n == 0 {
		return ""
	}
	return `<table:content-validations>` + v.xml.String() + `</table:content-validations>`
}

// literalODS es un valor del editor como operando de una condición de ODF: un número con
// punto decimal, o el texto entre comillas.
func literalODS(valor string) string { return literalFormula(valor) }

func entradaCalcext(tag, tipo, valor, color string) string {
	x := `<calcext:` + tag + ` calcext:value="` + valor + `" calcext:type="` + tipo + `"`
	if color != "" {
		x += ` calcext:color="` + color + `"`
	}
	return x + `/>`
}

// reglaODS es la `calcext:condition`, la barra de datos, la escala de color o el juego de
// iconos de una regla del editor; "" si no se puede escribir.
//
//nolint:gocognit,gocyclo // traductor de un formato a otro: un caso por tipo, y trocearlo reparte la tabla sin simplificarla
func reglaODS(r condRule, rg rango, hoja string, styles *odsStyles) string {
	base := xmlEscape(rango{c1: rg.c1, r1: rg.r1, c2: rg.c1, r2: rg.r1}.refODS(hoja))
	cond := func(valor string) string {
		col, tinta := colorHex(r.Fill), colorHex(r.TextColor)
		if col == "" && tinta == "" {
			return ""
		}
		return `<calcext:condition calcext:apply-style-name="` + styles.estiloCondicional(col, tinta) + `" calcext:value="` + xmlEscape(valor) +
			`" calcext:base-cell-address="` + base + `"/>`
	}
	switch r.Kind {
	case "", "cell":
		if r.Value == "" {
			return ""
		}
		if r.Op == "contains" {
			return cond(`contains-text("` + strings.ReplaceAll(strings.TrimSpace(r.Value), `"`, `""`) + `")`)
		}
		op := opODS[r.Op]
		if op == "" {
			return ""
		}
		return cond(op + literalODS(r.Value))
	case "formula":
		f := strings.TrimPrefix(strings.TrimSpace(r.Formula), "=")
		if f == "" {
			return ""
		}
		return cond("formula-is(" + a1AODF(f, false) + ")")
	case "top", "bottom":
		if r.Rank <= 0 {
			return ""
		}
		unidad := "elements"
		if r.Percent {
			unidad = "percent"
		}
		return cond(r.Kind + "-" + unidad + "(" + strconv.Itoa(r.Rank) + ")")
	case "dup":
		return cond("duplicate")
	case "unique":
		return cond("unique")
	case "dataBar":
		col := strings.ToLower(colorHex(r.Color))
		if col == "" {
			return ""
		}
		return `<calcext:data-bar calcext:min-length="10" calcext:max-length="90" calcext:negative-color="#ff0000" calcext:axis-position="none" calcext:positive-color="` +
			col + `" calcext:axis-color="#000000">` + entradaCalcext("formatting-entry", "minimum", "0", "") + entradaCalcext("formatting-entry", "maximum", "0", "") + `</calcext:data-bar>`
	case "colorScale":
		if len(r.Scale) != 2 && len(r.Scale) != 3 {
			return ""
		}
		var cs []string
		for _, c := range r.Scale {
			h := strings.ToLower(colorHex(c))
			if h == "" {
				return ""
			}
			cs = append(cs, h)
		}
		x := entradaCalcext("color-scale-entry", "minimum", "0", cs[0])
		if len(cs) == 3 {
			x += entradaCalcext("color-scale-entry", "percentile", "50", cs[1])
		}
		return `<calcext:color-scale>` + x + entradaCalcext("color-scale-entry", "maximum", "0", cs[len(cs)-1]) + `</calcext:color-scale>`
	case "iconSet":
		set := iconosXlsx[r.Icons]
		if set == "" {
			set = iconosXlsx["arrows"]
		}
		return `<calcext:icon-set calcext:icon-set-type="` + set + `">` + entradaCalcext("formatting-entry", "percent", "0", "") +
			entradaCalcext("formatting-entry", "percent", "33", "") + entradaCalcext("formatting-entry", "percent", "67", "") + `</calcext:icon-set>`
	}
	return ""
}

// formatosCondicionalesODS son los `calcext:conditional-formats` de la hoja.
func formatosCondicionalesODS(sd sheetData, hoja string, styles *odsStyles) string {
	var b strings.Builder
	for _, r := range sd.CondFormats {
		rg, ok := leerRango(r.Range)
		if !ok || !rg.valido() {
			continue
		}
		if x := reglaODS(r, rg, hoja, styles); x != "" {
			b.WriteString(`<calcext:conditional-format calcext:target-range-address="` + xmlEscape(rg.refODS(hoja)) + `">` + x + `</calcext:conditional-format>`)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return `<calcext:conditional-formats xmlns:calcext="` + nsCalcExt + `">` + b.String() + `</calcext:conditional-formats>`
}

// documentoEstilosODS es styles.xml de una hoja: el estilo «Default» y los que lleva el
// formato condicional; "" si no hace falta.
func (o *odsStyles) documentoEstilos() string {
	if o.nombrados.Len() == 0 {
		return ""
	}
	return xmlHeader + `<office:document-styles xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
		`xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" ` +
		`office:version="1.3"><office:styles><style:style style:name="Default" style:family="table-cell"/>` +
		o.nombrados.String() + `</office:styles></office:document-styles>`
}

// atributoPestana es el estilo de tabla de una hoja con el color de su pestaña
// (`tableooo:tab-color`, el que escribe y lee LibreOffice), como atributo de `table:table`;
// "" si la hoja no lo lleva.
func (o *odsStyles) atributoPestana(pestana string) string {
	c := strings.ToLower(colorHex(pestana))
	if c == "" {
		return ""
	}
	clave := "ta:" + c
	n, ok := o.seen[clave]
	if !ok {
		o.n++
		n = "ta" + strconv.Itoa(o.n)
		o.seen[clave] = n
		o.defs.WriteString(`<style:style style:name="` + n + `" style:family="table"><style:table-properties table:display="true" tableooo:tab-color="` + c + `"/></style:style>`)
	}
	return ` table:style-name="` + n + `"`
}

// estiloColumna y estiloFila son los estilos de columna y de fila de un tamaño en px.
func (o *odsStyles) estiloColumna(px float64) string {
	clave := "co:" + strconv.FormatFloat(px, 'f', 2, 64)
	if n, ok := o.seen[clave]; ok {
		return n
	}
	o.n++
	n := "co" + strconv.Itoa(o.n)
	o.seen[clave] = n
	o.defs.WriteString(`<style:style style:name="` + n + `" style:family="table-column"><style:table-column-properties fo:break-before="auto" style:column-width="` + pulgadas(px) + `"/></style:style>`)
	return n
}

func (o *odsStyles) estiloFila(px float64) string {
	clave := "ro:" + strconv.FormatFloat(px, 'f', 2, 64)
	if n, ok := o.seen[clave]; ok {
		return n
	}
	o.n++
	n := "ro" + strconv.Itoa(o.n)
	o.seen[clave] = n
	o.defs.WriteString(`<style:style style:name="` + n + `" style:family="table-row"><style:table-row-properties fo:break-before="auto" style:row-height="` + pulgadas(px) +
		`" style:use-optimal-row-height="false"/></style:style>`)
	return n
}

// columnasODS son los `table:table-column` de la hoja, agrupando las contiguas del mismo
// ancho y partiéndolas donde empieza o acaba un grupo de columnas; las que no dicen el suyo
// llevan la medida de serie del editor.
func columnasODS(sd sheetData, maxCol int, styles *odsStyles) string {
	var b strings.Builder
	ancho := func(c int) float64 {
		if px, ok := medidaValida(sd.ColWidths[strconv.Itoa(c-1)], maxAnchoPx); ok {
			return px
		}
		return anchoSeriePx
	}
	grupos := gruposAnidados(sd.ColGroups, maxColumnasHoja)
	pila := nuevaPilaODS(grupos, "table-column-group")
	cs := cortes(grupos)
	for c := 1; c <= maxCol; {
		w, j := ancho(c), c
		for j < maxCol && ancho(j+1) == w && !slices.Contains(cs, j+1) {
			j++
		}
		pila.avanzar(c, &b)
		b.WriteString(`<table:table-column table:style-name="` + styles.estiloColumna(w) + `"` + colapsoODS(sd.ColGroups, c-1) + ` table:number-columns-repeated="` + strconv.Itoa(j-c+1) + `"/>`)
		c = j + 1
	}
	pila.cerrar(&b)
	return b.String()
}
