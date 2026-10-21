// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"math"
	"strconv"
	"strings"
)

// El formato del editor hacia WordprocessingML: lo inverso de import_docx_formato.go. El
// documento ES HTML y su formato vive en el `style` de bloques, celdas y tramos; aquí se lee ese
// CSS y se escribe como rPr, pPr, tcPr y tblPr (ECMA-376 Parte 1 §17.3 y §17.4), de modo que
// Docus -> .docx -> Docus vuelva con lo mismo.
//
// Lo que hereda el texto (tipografía, tamaño, color, negrita…) viaja en el xrun desde el bloque
// hasta cada tramo, como en CSS; lo que es del párrafo (espaciado, sangrías, bordes, fondo,
// paradas) se lee del `style` del bloque, que el recorrido guarda en xpara.css.
//
// Clean-room sobre la especificación; nada se ha portado de otra implementación.

// declaracionesCSS parte un `style` en sus declaraciones, con la propiedad en minúsculas. ponytail:
// parte por `;` a secas; un `;` dentro de comillas daría una declaración rara que no se entiende.
func declaracionesCSS(css string) [][2]string {
	var out [][2]string
	for _, d := range strings.Split(css, ";") {
		k, v, ok := strings.Cut(d, ":")
		if !ok {
			continue
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		if k != "" && v != "" {
			out = append(out, [2]string{k, v})
		}
	}
	return out
}

// familiaDeCSS es la primera familia de un font-family y su genérica, si la lista acaba en una.
func familiaDeCSS(v string) (nombre, generica string) {
	partes := strings.Split(v, ",")
	limpia := func(s string) string { return strings.Trim(strings.TrimSpace(s), `"'`) }
	nombre = limpia(partes[0])
	if g := strings.ToLower(limpia(partes[len(partes)-1])); docxGenericas[g] != "" && len(partes) > 1 {
		generica = g
	}
	if docxGenericas[strings.ToLower(nombre)] != "" {
		return "", "" // solo una genérica: no es una fuente que se pueda nombrar en Word
	}
	return nombre, generica
}

// docxGenericas son las familias de fontTable (w:family) de cada genérica de CSS.
var docxGenericas = map[string]string{"serif": "roman", "sans-serif": "swiss", "monospace": "modern", "cursive": "script", "fantasy": "decorative"}

// docxTallas son los tamaños por nombre que escriben la web y rico (los siete peldaños de
// execCommand('fontSize')), en los puntos que la cinta enseña al elegirlos (Inicio.tsx).
var docxTallas = map[string]float64{
	"xx-small": 7, "x-small": 8, "small": 10, "medium": 12, "large": 14, "x-large": 18, "xx-large": 24, "xxx-large": 36,
}

// ptDeCSS es una longitud de CSS en puntos: pt, px o, relativa al tamaño de la letra, em y %; y
// los tamaños por nombre.
func ptDeCSS(v string, letra float64) (float64, bool) {
	if letra <= 0 {
		letra = 12
	}
	v = strings.ToLower(strings.TrimSpace(v))
	if pt, ok := docxTallas[v]; ok {
		return pt, true
	}
	for _, u := range []struct {
		sufijo string
		factor float64
	}{{"pt", 1}, {"px", 0.75}, {"em", letra}, {"%", letra / 100}} {
		if num, ok := strings.CutSuffix(v, u.sufijo); ok {
			n, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
			if err != nil || math.IsNaN(n) || math.Abs(n) > 2000 {
				return 0, false
			}
			return n * u.factor, true
		}
	}
	if v == "0" {
		return 0, true
	}
	return 0, false
}

// pxDeCSS es una longitud de CSS en px.
func pxDeCSS(v string, letra float64) (float64, bool) {
	pt, ok := ptDeCSS(v, letra)
	return pt / 0.75, ok
}

// conCSS aplica a un tramo las declaraciones de un `style`. En un bloque solo valen las que el
// texto hereda en CSS; el fondo, el borde, la decoración y el desplazamiento son del elemento.
//
//nolint:gocyclo,gocognit // una rama por propiedad de CSS: la ramificación ES la tabla
func (r xrun) conCSS(css string, bloque bool) xrun {
	for _, d := range declaracionesCSS(css) {
		k, v := d[0], strings.ToLower(d[1])
		switch k {
		case "font-family":
			r.fuente, r.generica = familiaDeCSS(d[1])
		case "font-size":
			if pt, ok := ptDeCSS(v, r.tamPt); ok && pt > 0 {
				r.tamPt = pt
			}
		case "color":
			if c := normalizeColor(v); c != "" {
				r.color = c
			}
		case "font-weight":
			n, err := strconv.Atoi(v)
			switch {
			case v == "bold" || v == "bolder" || err == nil && n >= 600:
				r.b, r.sinB = true, false
			case v == "normal" || v == "lighter" || err == nil:
				r.b, r.sinB = false, true
			}
		case "font-style":
			r.i, r.sinI = v == "italic" || v == "oblique", v == "normal"
		case "text-transform":
			r.mayus = v == "uppercase"
		case "font-variant", "font-variant-caps":
			r.versalitas = strings.Contains(v, "small-caps")
		case "letter-spacing":
			if pt, ok := ptDeCSS(v, r.tamPt); ok {
				r.espaciado = pt
			}
		case "white-space":
			r.pre = strings.HasPrefix(v, "pre") || v == "break-spaces"
		}
		if bloque {
			continue
		}
		switch k {
		case "background-color":
			r.fondo = normalizeColor(v)
		case "text-decoration", "text-decoration-line":
			r.u = r.u || strings.Contains(v, "underline")
			r.strike = r.strike || strings.Contains(v, "line-through")
		case "vertical-align":
			switch v {
			case "super":
				r.vert = "sup"
			case "sub":
				r.vert = "sub"
			default:
				if pt, ok := ptDeCSS(v, r.tamPt); ok {
					r.posicion = pt
				}
			}
		case "border":
			r.borde = d[1]
		case "display":
			r.oculto = v == "none"
		}
	}
	return r
}

// vinetaDeCSS es la viñeta de un list-style-type con una cadena («'- '», la que escribe el
// importador): su primer carácter. ok es false si el estilo no lo dice; con un valor que no es
// una cadena (disc, none…) vuelve la del editor.
func vinetaDeCSS(css string) (string, bool) {
	for _, d := range declaracionesCSS(css) {
		if d[0] != "list-style-type" {
			continue
		}
		v := strings.TrimSpace(d[1])
		if len(v) < 2 || (v[0] != '\'' && v[0] != '"') || v[len(v)-1] != v[0] {
			return "", true
		}
		r := []rune(strings.TrimSpace(v[1 : len(v)-1]))
		if len(r) == 0 {
			return "", true
		}
		return string(r[0]), true
	}
	return "", false
}

// twips escribe una medida en vigésimos de punto.
func twips(pt float64) string { return strconv.Itoa(int(math.Round(pt * 20))) }

// rPrDocx es el rPr de un tramo, en el orden de CT_RPr (§17.3.2.28): Word rechaza un fichero con
// las propiedades desordenadas.
//
//nolint:gocyclo // una rama por propiedad de rPr: la ramificación ES la tabla
func rPrDocx(r xrun, enlace bool) string {
	var b strings.Builder
	if r.fuente != "" {
		f := xmlEscape(r.fuente)
		b.WriteString(`<w:rFonts w:ascii="` + f + `" w:hAnsi="` + f + `" w:eastAsia="` + f + `" w:cs="` + f + `"/>`)
	}
	switch {
	case r.b:
		b.WriteString(`<w:b/><w:bCs/>`)
	case r.sinB:
		b.WriteString(`<w:b w:val="0"/><w:bCs w:val="0"/>`)
	}
	switch {
	case r.i:
		b.WriteString(`<w:i/><w:iCs/>`)
	case r.sinI:
		b.WriteString(`<w:i w:val="0"/><w:iCs w:val="0"/>`)
	}
	if r.mayus {
		b.WriteString(`<w:caps/>`)
	} else if r.versalitas {
		b.WriteString(`<w:smallCaps/>`)
	}
	if r.strike {
		b.WriteString(`<w:strike/>`)
	}
	if r.oculto {
		b.WriteString(`<w:vanish/>`)
	}
	switch {
	case r.color != "":
		b.WriteString(`<w:color w:val="` + r.color + `"/>`)
	case enlace:
		b.WriteString(`<w:color w:val="0563C1"/>`) // el azul de los enlaces de Word, si el texto no trae el suyo
	}
	if r.espaciado != 0 {
		b.WriteString(`<w:spacing w:val="` + twips(r.espaciado) + `"/>`)
	}
	if r.posicion != 0 {
		b.WriteString(`<w:position w:val="` + strconv.Itoa(int(math.Round(r.posicion*2))) + `"/>`)
	}
	if r.tamPt > 0 {
		hp := strconv.Itoa(int(math.Round(r.tamPt * 2)))
		b.WriteString(`<w:sz w:val="` + hp + `"/><w:szCs w:val="` + hp + `"/>`)
	}
	if r.u || enlace {
		b.WriteString(`<w:u w:val="single"/>`)
	}
	if bdr := bordeDocx("bdr", r.borde); bdr != "" {
		b.WriteString(bdr)
	}
	if r.fondo != "" {
		b.WriteString(`<w:shd w:val="clear" w:color="auto" w:fill="` + r.fondo + `"/>`)
	}
	switch r.vert {
	case "sup":
		b.WriteString(`<w:vertAlign w:val="superscript"/>`)
	case "sub":
		b.WriteString(`<w:vertAlign w:val="subscript"/>`)
	}
	return b.String()
}

// bordeDocx es un borde de CSS («1px solid #RRGGBB») como el elemento de OOXML `nombre`; "" si
// no se entiende. «none» es un borde que se quita (w:val="nil"). El grosor va en octavos de
// punto: lo inverso de lo que hace el importador (§17.3.4).
func bordeDocx(nombre, css string) string {
	css = strings.TrimSpace(strings.ToLower(css))
	if css == "" {
		return ""
	}
	if css == "none" || css == "0" {
		return `<w:` + nombre + ` w:val="nil"/>`
	}
	val, ancho, color := "", 1.0, "000000"
	for _, parte := range strings.Fields(css) {
		switch {
		case parte == "solid":
			val = "single"
		case parte == "double", parte == "dotted", parte == "dashed":
			val = parte
		case parte == "none" || parte == "hidden":
			return `<w:` + nombre + ` w:val="nil"/>`
		case strings.HasPrefix(parte, "#") || strings.HasPrefix(parte, "rgb"):
			if c := normalizeColor(parte); c != "" {
				color = c
			}
		default:
			if px, ok := pxDeCSS(parte, 12); ok {
				ancho = px
			}
		}
	}
	if val == "" {
		return ""
	}
	sz := strconv.Itoa(int(math.Max(2, math.Min(96, math.Round(ancho*6)))))
	return `<w:` + nombre + ` w:val="` + val + `" w:sz="` + sz + `" w:space="0" w:color="` + color + `"/>`
}

// docxRellenosInv son los rellenos de las paradas (ST_TabTlc) desde el nombre del editor.
var docxRellenosInv = map[string]string{"dot": "dot", "hyphen": "hyphen", "underscore": "underscore", "heavy": "heavy", "middledot": "middleDot"}

// pPrDocx es el pPr de un párrafo a partir del `style` de su bloque, en el orden de CT_PPrBase
// (§17.3.1.26). El estilo, la lista y la alineación los pone el que llama, en su sitio: van
// antes (pStyle, numPr) y después (jc) de lo de aquí, así que se devuelven por partes.
//
//nolint:gocyclo,gocognit // una rama por propiedad de CSS: la ramificación ES la tabla
func pPrDocx(css string, lista bool) (medio, sangria string) {
	var bordes [4]string
	var fondo, paradas string
	var antes, despues, linea, regla, izq, der, primera string
	izqPx, primeraPx := 0.0, 0.0
	letra := 12.0
	for _, d := range declaracionesCSS(css) {
		if d[0] == "font-size" {
			if pt, ok := ptDeCSS(d[1], 12); ok && pt > 0 {
				letra = pt
			}
		}
	}
	for _, d := range declaracionesCSS(css) {
		k, v := d[0], d[1]
		switch k {
		case "margin-top", "margin-bottom":
			if pt, ok := ptDeCSS(v, letra); ok && pt >= 0 {
				if k == "margin-top" {
					antes = twips(pt)
				} else {
					despues = twips(pt)
				}
			}
		case "line-height":
			if pt, ok := ptDeCSS(v, letra); ok && !strings.HasSuffix(v, "em") && !strings.HasSuffix(v, "%") {
				linea, regla = twips(pt), "exact"
			} else if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 && n < 10 {
				// Lo inverso del importador: «auto» son 240 avos de 1,2 veces la letra.
				linea, regla = strconv.Itoa(int(math.Round(n/1.2*240))), "auto"
			}
		case "margin-left", "margin-right", "text-indent":
			// En una lista las sangrías son de la lista (numbering.xml), como en el importador.
			px, ok := pxDeCSS(v, letra)
			if !ok || lista {
				continue
			}
			switch k {
			case "margin-left":
				izq, izqPx = twips(px*0.75), px
			case "margin-right":
				der = twips(px * 0.75)
			default:
				primera, primeraPx = twips(px*0.75), px
			}
		case "background-color":
			if c := normalizeColor(v); c != "" {
				fondo = `<w:shd w:val="clear" w:color="auto" w:fill="` + c + `"/>`
			}
		case "border-top", "border-right", "border-bottom", "border-left":
			i := map[string]int{"border-top": 0, "border-right": 1, "border-bottom": 2, "border-left": 3}[k]
			bordes[i] = v
		case "border":
			bordes = [4]string{v, v, v, v}
		case "--sytue-tabs":
			paradas = v
		}
	}
	tabs := ""
	if paradas != "" {
		tabs = tabsDocx(paradas, izqPx, primeraPx < 0)
	}
	var b strings.Builder
	if bordes != [4]string{} {
		var pb strings.Builder
		for i, lado := range []string{"top", "left", "bottom", "right"} {
			// El orden de CT_PBdr es top, left, bottom, right.
			pb.WriteString(bordeDocx(lado, bordes[[]int{0, 3, 2, 1}[i]]))
		}
		if pb.Len() > 0 {
			b.WriteString(`<w:pBdr>` + pb.String() + `</w:pBdr>`)
		}
	}
	b.WriteString(fondo)
	b.WriteString(tabs)
	if antes != "" || despues != "" || linea != "" {
		b.WriteString(`<w:spacing`)
		if antes != "" {
			b.WriteString(` w:before="` + antes + `"`)
		}
		if despues != "" {
			b.WriteString(` w:after="` + despues + `"`)
		}
		if linea != "" {
			b.WriteString(` w:line="` + linea + `" w:lineRule="` + regla + `"`)
		}
		b.WriteString(`/>`)
	}
	medio = b.String()
	if izq != "" || der != "" || primera != "" {
		sangria = `<w:ind`
		if izq != "" {
			sangria += ` w:left="` + izq + `"`
		}
		if der != "" {
			sangria += ` w:right="` + der + `"`
		}
		switch {
		case primeraPx < 0:
			sangria += ` w:hanging="` + strings.TrimPrefix(primera, "-") + `"`
		case primera != "":
			sangria += ` w:firstLine="` + primera + `"`
		}
		sangria += `/>`
	}
	return medio, sangria
}

// tabsDocx son las paradas de --sytue-tabs como w:tabs. El editor las cuenta desde el borde del
// texto y Word desde el margen: se les suma la sangría izquierda. La parada «0px left» de una
// sangría francesa no se escribe: es la implícita de Word, que el importador vuelve a poner.
func tabsDocx(v string, izqPx float64, francesa bool) string {
	var b strings.Builder
	for _, parada := range strings.Split(v, ",") {
		c := strings.Fields(parada)
		if len(c) < 2 {
			continue
		}
		px, ok := pxDeCSS(c[0], 12)
		if !ok {
			continue
		}
		if francesa && px == 0 && c[1] == "left" && len(c) == 2 {
			continue
		}
		val := c[1]
		if val != "left" && val != "center" && val != "right" && val != "decimal" {
			continue
		}
		b.WriteString(`<w:tab w:val="` + val + `"`)
		if len(c) > 2 && docxRellenosInv[c[2]] != "" {
			b.WriteString(` w:leader="` + docxRellenosInv[c[2]] + `"`)
		}
		b.WriteString(` w:pos="` + twips((px+izqPx)*0.75) + `"/>`)
	}
	if b.Len() == 0 {
		return ""
	}
	return `<w:tabs>` + b.String() + `</w:tabs>`
}

// tabuladorDocx es el w:defaultTabStop del documento: el tab-size del primer bloque que lo dice
// (el importador lo pone igual en todos), en twips; "" si ninguno lo dice.
func tabuladorDocx(bloques []xblock) string {
	for _, blk := range bloques {
		var cssDe []string
		switch {
		case blk.para != nil:
			cssDe = append(cssDe, blk.para.css)
		case blk.table != nil:
			for _, fila := range blk.table.rows {
				for _, c := range fila {
					for _, p := range c.paras {
						cssDe = append(cssDe, p.css)
					}
				}
			}
		}
		for _, css := range cssDe {
			for _, d := range declaracionesCSS(css) {
				if d[0] == "tab-size" {
					if px, ok := pxDeCSS(d[1], 12); ok && px > 0 {
						return twips(px * 0.75)
					}
				}
			}
		}
	}
	return ""
}

// fuentesDocx son las fuentes con familia genérica que nombran los tramos, para fontTable.xml:
// así vuelve la genérica que el importador escribe detrás del nombre.
func fuentesDocx(bloques []xblock, notas []*xnote) map[string]string {
	out := map[string]string{}
	anota := func(runs []xrun) {
		for _, r := range runs {
			if r.fuente != "" && r.generica != "" {
				out[r.fuente] = docxGenericas[r.generica]
			}
		}
	}
	// La de la marca de un párrafo sin texto sale de su bloque (paraProps).
	parrafo := func(p xpara) { anota(append(p.runs, xrun{}.conCSS(p.css, true))) }
	for _, blk := range bloques {
		if blk.para != nil {
			parrafo(*blk.para)
		}
		if blk.table != nil {
			for _, fila := range blk.table.rows {
				for _, c := range fila {
					for _, p := range c.paras {
						parrafo(p)
					}
				}
			}
		}
	}
	for _, n := range notas {
		anota(n.runs)
	}
	return out
}

// tcPrDocx es lo que el `style` de una celda da a su tcPr: ancho, bordes, sombreado, márgenes y
// alineación vertical, en el orden de CT_TcPr (§17.4.70). El ancho va antes del gridSpan y del
// vMerge, que pone el que llama: se devuelve aparte.
func tcPrDocx(css string) (ancho, resto string) {
	var lados [4]string
	var fondo, relleno, valign string
	for _, d := range declaracionesCSS(css) {
		k, v := d[0], d[1]
		switch k {
		case "width":
			if px, ok := pxDeCSS(v, 12); ok && px > 0 {
				ancho = `<w:tcW w:w="` + twips(px*0.75) + `" w:type="dxa"/>`
			}
		case "border":
			lados = [4]string{v, v, v, v}
		case "border-top", "border-right", "border-bottom", "border-left":
			lados[map[string]int{"border-top": 0, "border-right": 1, "border-bottom": 2, "border-left": 3}[k]] = v
		case "background-color":
			if c := normalizeColor(v); c != "" {
				fondo = `<w:shd w:val="clear" w:color="auto" w:fill="` + c + `"/>`
			}
		case "padding":
			relleno = margenesCeldaDocx(v)
		case "vertical-align":
			switch v {
			case "middle":
				valign = `<w:vAlign w:val="center"/>`
			case "bottom":
				valign = `<w:vAlign w:val="bottom"/>`
			}
		}
	}
	var b strings.Builder
	if lados != [4]string{} {
		var tb strings.Builder
		for i, lado := range []string{"top", "left", "bottom", "right"} {
			tb.WriteString(bordeDocx(lado, lados[[]int{0, 3, 2, 1}[i]]))
		}
		if tb.Len() > 0 {
			b.WriteString(`<w:tcBorders>` + tb.String() + `</w:tcBorders>`)
		}
	}
	b.WriteString(fondo)
	b.WriteString(relleno)
	b.WriteString(valign)
	return ancho, b.String()
}

// margenesCeldaDocx es un padding de CSS (uno a cuatro valores) como tcMar.
func margenesCeldaDocx(v string) string {
	var px []float64
	for _, p := range strings.Fields(v) {
		n, ok := pxDeCSS(p, 12)
		if !ok {
			return ""
		}
		px = append(px, n)
	}
	var t, r, b, l float64
	switch len(px) {
	case 1:
		t, r, b, l = px[0], px[0], px[0], px[0]
	case 2:
		t, r, b, l = px[0], px[1], px[0], px[1]
	case 3:
		t, r, b, l = px[0], px[1], px[2], px[1]
	case 4:
		t, r, b, l = px[0], px[1], px[2], px[3]
	default:
		return ""
	}
	m := func(nombre string, px float64) string {
		return `<w:` + nombre + ` w:w="` + twips(px*0.75) + `" w:type="dxa"/>`
	}
	return `<w:tcMar>` + m("top", t) + m("left", l) + m("bottom", b) + m("right", r) + `</w:tcMar>`
}

// tblPrDocx es lo que el `style` de una tabla da a su tblPr: ancho, alineación y sangría.
func tblPrDocx(css string) (ancho, jc, sangria string) {
	izqAuto, derAuto := false, false
	for _, d := range declaracionesCSS(css) {
		k, v := d[0], d[1]
		switch k {
		case "width":
			if px, ok := pxDeCSS(v, 12); ok && px > 0 {
				ancho = `<w:tblW w:w="` + twips(px*0.75) + `" w:type="dxa"/>`
			}
		case "margin-left":
			if v == "auto" {
				izqAuto = true
			} else if px, ok := pxDeCSS(v, 12); ok && px > 0 {
				sangria = `<w:tblInd w:w="` + twips(px*0.75) + `" w:type="dxa"/>`
			}
		case "margin-right":
			derAuto = v == "auto"
		}
	}
	switch {
	case izqAuto && derAuto:
		jc = `<w:jc w:val="center"/>`
	case izqAuto:
		jc = `<w:jc w:val="right"/>`
	}
	return ancho, jc, sangria
}

// framePrDocx es el framePr de un párrafo que va en un marco de texto (div[data-marco]): lo inverso
// de docxMarco.css, de modo que la vuelta escriba el mismo CSS. Flotando, el texto lo rodea
// (wrap around), a la izquierda en su posición desde el margen o a la derecha; si no flota, va en
// su renglón (notBeside). La posición vertical es la del texto, donde está.
func framePrDocx(css string) string {
	if css == "" {
		return ""
	}
	var flota string
	var ancho, izq, der, bajo float64
	centrado := false
	for _, d := range declaracionesCSS(css) {
		px, _ := pxDeCSS(d[1], 12)
		switch d[0] {
		case "float":
			flota = strings.ToLower(d[1])
		case "width":
			ancho = px
		case "margin-left":
			izq, centrado = px, d[1] == "auto"
		case "margin-right":
			der = px
		case "margin-bottom":
			bajo = px
		}
	}
	tw := func(px float64) string { return twips(px * 0.75) }
	out := `<w:framePr`
	if ancho > 0 {
		out += ` w:w="` + tw(ancho) + `"`
	}
	espacio := der
	switch {
	case flota == "right":
		out += ` w:wrap="around" w:hAnchor="margin" w:xAlign="right"`
		espacio = izq
	case centrado:
		out += ` w:wrap="notBeside" w:hAnchor="margin" w:xAlign="center"`
		espacio = 0
	case flota == "left":
		out += ` w:wrap="around" w:hAnchor="margin" w:x="` + tw(izq) + `"`
	default:
		out += ` w:wrap="notBeside" w:hAnchor="margin" w:x="` + tw(izq) + `"`
		espacio = 0
	}
	if espacio > 0 {
		out += ` w:hSpace="` + tw(espacio) + `"`
	}
	if bajo > 0 {
		out += ` w:vSpace="` + tw(bajo) + `"`
	}
	return out + ` w:vAnchor="text" w:y="0"/>`
}
