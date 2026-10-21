// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"cmp"
	"encoding/xml"
	"math"
	"slices"
	"strconv"
	"strings"
)

// Importación del formato de una hoja OOXML (ver sheet_formato.go): el reverso de
// export_sheet_formato.go, y lo bastante tolerante para leer lo que escriben Excel (con
// colores de tema y con tinte, o indexados) y Calc.

// colorXlsx es un `<color>`/`<fgColor>`/`<bgColor>` de styles.xml.
type colorXlsx struct {
	RGB     string `xml:"rgb,attr"`
	Theme   string `xml:"theme,attr"`
	Indexed string `xml:"indexed,attr"`
	Tint    string `xml:"tint,attr"`
}

type patronXlsx struct {
	Tipo string    `xml:"patternType,attr"`
	Fg   colorXlsx `xml:"fgColor"`
	Bg   colorXlsx `xml:"bgColor"`
}

// degradadoXlsx es un `gradientFill`: sus paradas (posición 0..1 y color).
type degradadoXlsx struct {
	Paradas []struct {
		Pos   float64   `xml:"position,attr"`
		Color colorXlsx `xml:"color"`
	} `xml:"stop"`
}

// activoXlsx es un `<b/>`, `<i/>`… : presente y sin `val="0"`.
type activoXlsx struct {
	Val string `xml:"val,attr"`
}

func activo(a *activoXlsx) bool {
	return a != nil && a.Val != "0" && !strings.EqualFold(a.Val, "false")
}

type fuenteDeXlsx struct {
	B      *activoXlsx `xml:"b"`
	I      *activoXlsx `xml:"i"`
	Strike *activoXlsx `xml:"strike"`
	U      *struct {
		Val string `xml:"val,attr"`
	} `xml:"u"`
	Sz struct {
		Val string `xml:"val,attr"`
	} `xml:"sz"`
	Color colorXlsx `xml:"color"`
	Name  struct {
		Val string `xml:"val,attr"`
	} `xml:"name"`
}

type ladoXlsx struct {
	Style string     `xml:"style,attr"`
	Color *colorXlsx `xml:"color"`
}

// xlsxXf es un `cellXf` ya traducido al modelo del editor.
type xlsxXf struct {
	num              *numFmtSpec
	estilo           cellStyle
	negrita, cursiva bool
	alinear          string
}

// xlsxEstilos es lo que se lee de `xl/styles.xml`.
type xlsxEstilos struct {
	xfs []xlsxXf
	// dxfFill es, por dxf, el relleno de formato condicional (#RRGGBB) o "", y dxfTinta, su
	// color de letra.
	dxfFill, dxfTinta []string
	tema              []string
	indices           []string
}

// paleta por defecto de los colores indexados (ECMA-376 §18.8.27).
var paletaIndexada = []string{
	"000000", "FFFFFF", "FF0000", "00FF00", "0000FF", "FFFF00", "FF00FF", "00FFFF",
	"000000", "FFFFFF", "FF0000", "00FF00", "0000FF", "FFFF00", "FF00FF", "00FFFF",
	"800000", "008000", "000080", "808000", "800080", "008080", "C0C0C0", "808080",
	"9999FF", "993366", "FFFFCC", "CCFFFF", "660066", "FF8080", "0066CC", "CCCCFF",
	"000080", "FF00FF", "FFFF00", "00FFFF", "800080", "800000", "008080", "0000FF",
	"00CCFF", "CCFFFF", "CCFFCC", "FFFF99", "99CCFF", "FF99CC", "CC99FF", "FFCC99",
	"3366FF", "33CCCC", "99CC00", "FFCC00", "FF9900", "FF6600", "666699", "969696",
	"003366", "339966", "003300", "333300", "993300", "993366", "333399", "333333",
}

// temaPorDefecto es la paleta de Office por si el libro no trae `theme1.xml`: lt1, dk1,
// lt2, dk2 y accent1..6, en el orden en que los numera `theme=` en styles.xml.
var temaPorDefecto = []string{"FFFFFF", "000000", "E7E6E6", "44546A", "4472C4", "ED7D31", "A5A5A5", "FFC000", "5B9BD5", "70AD47"}

// parseTema lee los diez colores de `a:clrScheme` en el orden de `theme=`.
func parseTema(data []byte) []string {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	var hex []string // dk1 lt1 dk2 lt2 accent1..6 hlink folHlink
	dentro := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "clrScheme":
				dentro = true
			case "srgbClr":
				if dentro {
					hex = append(hex, strings.ToUpper(attrLocal(e, "val")))
				}
			case "sysClr":
				if dentro {
					hex = append(hex, strings.ToUpper(attrLocal(e, "lastClr")))
				}
			}
		case xml.EndElement:
			if e.Name.Local == "clrScheme" {
				dentro = false
			}
		}
	}
	if len(hex) < 10 {
		return temaPorDefecto
	}
	// theme= 0 es lt1, 1 dk1, 2 lt2, 3 dk2 (intercambiados dos a dos respecto del esquema).
	return []string{hex[1], hex[0], hex[3], hex[2], hex[4], hex[5], hex[6], hex[7], hex[8], hex[9]}
}

// aplicarTinte aclara (tinte > 0) u oscurece (< 0) un color en HLS, como hace Excel.
func aplicarTinte(hex string, tinte float64) string {
	if tinte == 0 || len(hex) != 6 {
		return hex
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return hex
	}
	r, g, b := float64(v>>16&255)/255, float64(v>>8&255)/255, float64(v&255)/255
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l := (mx + mn) / 2
	var h, s float64
	if mx != mn {
		d := mx - mn
		if l > 0.5 {
			s = d / (2 - mx - mn)
		} else {
			s = d / (mx + mn)
		}
		switch mx {
		case r:
			h = (g - b) / d
			if g < b {
				h += 6
			}
		case g:
			h = (b-r)/d + 2
		default:
			h = (r-g)/d + 4
		}
		h /= 6
	}
	if tinte < 0 {
		l *= 1 + tinte
	} else {
		l = l*(1-tinte) + tinte
	}
	if s == 0 {
		return strings.ToUpper(strconv.FormatUint(uint64(math.Round(l*255))*0x010101, 16))
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	canal := func(t float64) float64 {
		if t < 0 {
			t++
		}
		if t > 1 {
			t--
		}
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		}
		return p
	}
	rr, gg, bb := uint64(math.Round(canal(h+1.0/3)*255)), uint64(math.Round(canal(h)*255)), uint64(math.Round(canal(h-1.0/3)*255))
	s2 := strconv.FormatUint(rr<<16|gg<<8|bb, 16)
	return strings.ToUpper(strings.Repeat("0", 6-len(s2)) + s2)
}

// resolver es el color como «#RRGGBB»; "" si es automático o no se sabe.
func (e *xlsxEstilos) resolver(c colorXlsx) string {
	if e == nil { // un libro sin hoja de estilos: la paleta y el tema de serie
		e = &xlsxEstilos{tema: temaPorDefecto, indices: paletaIndexada}
	}
	switch {
	case c.RGB != "":
		return hexSinAlfa(c.RGB)
	case c.Theme != "":
		i, err := strconv.Atoi(c.Theme)
		if err != nil || i < 0 || i >= len(e.tema) {
			return ""
		}
		t, _ := strconv.ParseFloat(c.Tint, 64)
		return colorHex("#" + aplicarTinte(e.tema[i], t))
	case c.Indexed != "":
		i, err := strconv.Atoi(c.Indexed)
		if err != nil || i < 0 || i >= len(e.indices) {
			return "" // 64 y 65 son los del sistema
		}
		return colorHex("#" + e.indices[i])
	}
	return ""
}

// relleno es el color con que se ve un relleno de celda (ECMA-376 §18.8.20): el liso, su
// `fgColor`; una trama (gray125, darkGrid…), la mezcla de sus dos colores en la parte que cubre
// —sin color, negro sobre blanco, como Excel—; un degradado, su color medio. "" si no rellena.
func (e *xlsxEstilos) relleno(p patronXlsx, g *degradadoXlsx) string {
	if g != nil {
		var paradas []paradaDML
		for _, s := range g.Paradas {
			if c, ok := rgbDeHex(e.resolver(s.Color)); ok {
				paradas = append(paradas, paradaDML{s.Pos, c})
			}
		}
		if m, ok := mediaDeDegradado(paradas); ok {
			return "#" + m.hex()
		}
		return ""
	}
	switch p.Tipo {
	case "", "none":
		return ""
	case "solid":
		return e.resolver(p.Fg)
	}
	var fg, bg *rgbDML
	if c, ok := rgbDeHex(e.resolver(p.Fg)); ok {
		fg = &c
	}
	if c, ok := rgbDeHex(e.resolver(p.Bg)); ok {
		bg = &c
	}
	return "#" + mezclaDeTrama(fg, bg, p.Tipo).hex()
}

// fuentesDelEditor son las familias que el editor sabe pintar (`FONT_STACKS`).
var fuentesDelEditor = []string{"Inter", "Arial", "Georgia", "Times New Roman", "Courier New", "JetBrains Mono"}

// parseXlsxStyles lee xl/styles.xml (y el tema, para los colores de tema) y devuelve, por
// índice de cellXf, el formato de número y el estilo visual en el modelo del editor, y por
// dxf el relleno. nil si no hay hoja de estilos.
//
//nolint:gocognit,gocyclo // traductor de un formato a otro: un caso por tipo, y trocearlo reparte la tabla sin simplificarla
func parseXlsxStyles(data, tema []byte) *xlsxEstilos {
	if len(data) == 0 {
		return nil
	}
	var st struct {
		NumFmts []struct {
			ID   string `xml:"numFmtId,attr"`
			Code string `xml:"formatCode,attr"`
		} `xml:"numFmts>numFmt"`
		Fuentes  []fuenteDeXlsx `xml:"fonts>font"`
		Rellenos []struct {
			P patronXlsx     `xml:"patternFill"`
			G *degradadoXlsx `xml:"gradientFill"`
		} `xml:"fills>fill"`
		Bordes []struct {
			L ladoXlsx `xml:"left"`
			R ladoXlsx `xml:"right"`
			T ladoXlsx `xml:"top"`
			B ladoXlsx `xml:"bottom"`
			// start y end son la izquierda y la derecha con los nombres de ISO 29500, que usa
			// algún escritor (PhpSpreadsheet) también en los libros de transición.
			S ladoXlsx `xml:"start"`
			E ladoXlsx `xml:"end"`
		} `xml:"borders>border"`
		Xfs []struct {
			NumFmtID string `xml:"numFmtId,attr"`
			FontID   string `xml:"fontId,attr"`
			FillID   string `xml:"fillId,attr"`
			BorderID string `xml:"borderId,attr"`
			Alinear  struct {
				H    string `xml:"horizontal,attr"`
				V    string `xml:"vertical,attr"`
				Wrap string `xml:"wrapText,attr"`
			} `xml:"alignment"`
			Proteger struct {
				Locked string `xml:"locked,attr"`
				Hidden string `xml:"hidden,attr"`
			} `xml:"protection"`
		} `xml:"cellXfs>xf"`
		Dxfs []struct {
			Fill struct {
				P patronXlsx `xml:"patternFill"`
			} `xml:"fill"`
			Font struct {
				Color *colorXlsx `xml:"color"`
			} `xml:"font"`
		} `xml:"dxfs>dxf"`
		Indexados []string `xml:"colors>indexedColors>rgbColor>rgb"`
	}
	if xml.Unmarshal(data, &st) != nil {
		return nil
	}
	e := &xlsxEstilos{tema: parseTema(tema), indices: paletaIndexada}
	if len(st.Indexados) > 0 {
		e.indices = make([]string, len(st.Indexados))
		for i, v := range st.Indexados {
			e.indices[i] = strings.TrimPrefix(hexSinAlfa(v), "#") // «FF…», «ff…» o «00…»: el alfa no cuenta
		}
	}
	custom := map[string]string{}
	for _, nf := range st.NumFmts {
		custom[nf.ID] = nf.Code
	}
	var base fuenteDeXlsx
	if len(st.Fuentes) > 0 {
		base = st.Fuentes[0]
	}
	tamBase := tamanoFuente(base)
	for i, xf := range st.Xfs {
		if i >= maxEstilosHoja {
			break
		}
		x := xlsxXf{num: numFmtSpecFor(xf.NumFmtID, custom[xf.NumFmtID])}
		if n, err := strconv.Atoi(xf.FontID); err == nil && n > 0 && n < len(st.Fuentes) {
			f := st.Fuentes[n]
			x.negrita, x.cursiva = activo(f.B), activo(f.I)
			x.estilo.S = activo(f.Strike)
			x.estilo.U = f.U != nil && f.U.Val != "none"
			if t := tamanoFuente(f); t != 0 && t != tamBase {
				x.estilo.Size = min(96, max(6, t))
			}
			for _, fam := range fuentesDelEditor {
				if strings.EqualFold(f.Name.Val, fam) && !strings.EqualFold(f.Name.Val, base.Name.Val) {
					x.estilo.Font = fam
				}
			}
			if c := e.resolver(f.Color); c != "" && c != "#000000" {
				x.estilo.Color = c
			}
		}
		if n, err := strconv.Atoi(xf.FillID); err == nil && n >= 0 && n < len(st.Rellenos) {
			x.estilo.Fill = e.relleno(st.Rellenos[n].P, st.Rellenos[n].G)
		}
		if n, err := strconv.Atoi(xf.BorderID); err == nil && n > 0 && n < len(st.Bordes) {
			b := st.Bordes[n]
			if bd := e.borde(b.T, cmp.Or(b.R, b.E), b.B, cmp.Or(b.L, b.S)); bd != (cellBorder{}) {
				x.estilo.Border = &bd
			}
		}
		switch xf.Alinear.H {
		case "center", "centerContinuous":
			x.alinear = "center"
		case "right":
			x.alinear = "right"
		}
		x.estilo.Valign = map[string]string{"top": "top", "center": "middle", "bottom": "bottom"}[xf.Alinear.V]
		x.estilo.Wrap = xf.Alinear.Wrap == "1" || xf.Alinear.Wrap == "true"
		if !verdad(xf.Proteger.Locked, true) { // por omisión una celda está bloqueada
			no := false
			x.estilo.Locked = &no
		}
		x.estilo.Hidden = verdad(xf.Proteger.Hidden, false)
		e.xfs = append(e.xfs, x)
	}
	for _, d := range st.Dxfs {
		// En un dxf el color de un relleno liso va en `bgColor` (y a veces en `fgColor`); el
		// patternType que falta es «solid», y «none» es quitar el relleno.
		c := e.resolver(d.Fill.P.Bg)
		if c == "" {
			c = e.resolver(d.Fill.P.Fg)
		}
		if d.Fill.P.Tipo == "none" {
			c = ""
		}
		e.dxfFill = append(e.dxfFill, c)
		tinta := ""
		if d.Font.Color != nil {
			tinta = e.resolver(*d.Font.Color)
		}
		e.dxfTinta = append(e.dxfTinta, tinta)
	}
	return e
}

// borde es el borde del editor de los cuatro lados de un `border`: cuáles se ven y, de cada
// uno, su trazo y su color. El negro es el de serie: un color «auto» o negro no se escribe
// y el lado se pinta con el del editor, que en modo oscuro no desaparece.
func (e *xlsxEstilos) borde(t, r, b, l ladoXlsx) cellBorder {
	lado := func(x ladoXlsx) (bool, string, string) {
		if x.Style == "" || x.Style == "none" {
			return false, "", ""
		}
		color := ""
		switch {
		case x.Color == nil:
		case x.Color.Indexed == "65":
			color = "#FFFFFF" // el fondo del sistema: un borde blanco, que tapa la cuadrícula
		default:
			if color = e.resolver(*x.Color); color == "#000000" {
				color = ""
			}
		}
		return true, trazoDeXlsx(x.Style), color
	}
	var bd cellBorder
	bd.T, bd.TS, bd.TC = lado(t)
	bd.R, bd.RS, bd.RC = lado(r)
	bd.B, bd.BS, bd.BC = lado(b)
	bd.L, bd.LS, bd.LC = lado(l)
	return bd
}

func tamanoFuente(f fuenteDeXlsx) int {
	v, err := strconv.ParseFloat(f.Sz.Val, 64)
	if err != nil {
		return 0
	}
	return int(math.Round(v))
}

// xf es el estilo de una celda por su atributo `s`.
func (e *xlsxEstilos) xf(s string) (xlsxXf, bool) {
	if e == nil || s == "" {
		return xlsxXf{}, false
	}
	i, err := strconv.Atoi(s)
	if err != nil || i < 0 || i >= len(e.xfs) {
		return xlsxXf{}, false
	}
	return e.xfs[i], true
}

// --- Formato condicional y validaciones ---

type reglaXlsxEntrada struct {
	Tipo      string   `xml:"type,attr"`
	Operador  string   `xml:"operator,attr"`
	Dxf       string   `xml:"dxfId,attr"`
	Prioridad int      `xml:"priority,attr"`
	Rank      string   `xml:"rank,attr"`
	Percent   string   `xml:"percent,attr"`
	Bottom    string   `xml:"bottom,attr"`
	Text      string   `xml:"text,attr"`
	Formulas  []string `xml:"formula"`
	DataBar   struct {
		Color colorXlsx `xml:"color"`
	} `xml:"dataBar"`
	ColorScale struct {
		Colores []colorXlsx `xml:"color"`
	} `xml:"colorScale"`
	IconSet struct {
		Nombre string `xml:"iconSet,attr"`
	} `xml:"iconSet"`
}

type formatoCondXlsx struct {
	Sqref  string             `xml:"sqref,attr"`
	Reglas []reglaXlsxEntrada `xml:"cfRule"`
}

type validacionXlsxEntrada struct {
	Tipo       string `xml:"type,attr"`
	Operador   string `xml:"operator,attr"`
	ErrorStyle string `xml:"errorStyle,attr"`
	ShowError  string `xml:"showErrorMessage,attr"`
	Error      string `xml:"error,attr"`
	Sqref      string `xml:"sqref,attr"`
	F1         string `xml:"formula1"`
	F2         string `xml:"formula2"`
}

var (
	opDeCelda = map[string]string{
		"greaterThan": "gt", "greaterThanOrEqual": "ge", "lessThan": "lt", "lessThanOrEqual": "le",
		"equal": "eq", "notEqual": "ne",
	}
	simboloOp = map[string]string{"gt": ">", "ge": ">=", "lt": "<", "le": "<=", "eq": "=", "ne": "<>"}
	// iconosDeXlsx: por la raíz del nombre del juego (3Arrows, 5ArrowsGray…).
	iconosDeXlsx = []struct{ trozo, id string }{{"Arrow", "arrows"}, {"Traffic", "traffic"}, {"Symbols", "symbols"}}
)

// operandoCondicion es el operando de una regla `cellIs` como texto del editor: un número
// con coma decimal, un literal de texto sin comillas; "" y false si es una referencia o
// una fórmula.
func operandoCondicion(f string) (string, bool) {
	f = strings.TrimSpace(f)
	if len(f) >= 2 && f[0] == '"' && f[len(f)-1] == '"' && !strings.Contains(f[1:len(f)-1], `"`) {
		return f[1 : len(f)-1], true
	}
	if n, err := strconv.ParseFloat(f, 64); err == nil && !math.IsInf(n, 0) && !math.IsNaN(n) {
		return textoNumEs(n), true
	}
	return "", false
}

// reglasDeXlsx pasa los `conditionalFormatting` de la hoja al modelo, por orden de
// prioridad; lo que el editor no sabe expresar (relleno ausente, tipos sin equivalente)
// se descarta.
func reglasDeXlsx(cf []formatoCondXlsx, e *xlsxEstilos) []condRule {
	type conPrio struct {
		p int
		r condRule
	}
	var out []conPrio
	for _, bloque := range cf {
		rangos := partesSqref(bloque.Sqref)
		for _, rx := range bloque.Reglas {
			var pinta condRule // el relleno y la letra de su dxf
			if e != nil {
				if n, err := strconv.Atoi(rx.Dxf); err == nil && n >= 0 && n < len(e.dxfFill) {
					pinta.Fill, pinta.TextColor = e.dxfFill[n], e.dxfTinta[n]
				}
			}
			for _, rg := range rangos {
				if len(out) >= maxReglasHoja {
					break
				}
				if r, ok := reglaDeXlsx(rx, rg, pinta, e); ok {
					out = append(out, conPrio{rx.Prioridad, r})
				}
			}
		}
	}
	slices.SortStableFunc(out, func(a, b conPrio) int { return a.p - b.p })
	res := make([]condRule, len(out))
	for i, o := range out {
		res[i] = o.r
	}
	return res
}

//nolint:gocognit,gocyclo // traductor de un formato a otro: un caso por tipo, y trocearlo reparte la tabla sin simplificarla
func reglaDeXlsx(rx reglaXlsxEntrada, rg rango, pinta condRule, e *xlsxEstilos) (condRule, bool) {
	r := condRule{Range: rg.a1()}
	f := func(i int) string {
		if i < len(rx.Formulas) {
			return strings.TrimSpace(rx.Formulas[i])
		}
		return ""
	}
	celda := A1Ref(rg.r1, rg.c1)
	// Una regla que pinta la celda: con relleno, con color de letra o con los dos.
	conRelleno := func() bool {
		r.Fill, r.TextColor = pinta.Fill, pinta.TextColor
		return r.Fill != "" || r.TextColor != ""
	}
	switch rx.Tipo {
	case "cellIs":
		if !conRelleno() {
			return r, false
		}
		if op := opDeCelda[rx.Operador]; op != "" {
			if v, ok := operandoCondicion(f(0)); ok {
				r.Op, r.Value = op, v
				return r, true
			}
			// Un operando que es una referencia o una fórmula: regla de fórmula.
			r.Kind, r.Formula = "formula", "="+celda+simboloOp[op]+f(0)
			return r, true
		}
		switch rx.Operador {
		case "between":
			r.Kind, r.Formula = "formula", "=AND("+celda+">="+f(0)+","+celda+"<="+f(1)+")"
		case "notBetween":
			r.Kind, r.Formula = "formula", "=OR("+celda+"<"+f(0)+","+celda+">"+f(1)+")"
		default:
			return r, false
		}
		return r, true
	case "containsText":
		if !conRelleno() || strings.TrimSpace(rx.Text) == "" {
			return r, false
		}
		r.Op, r.Value = "contains", rx.Text
		return r, true
	case "expression", "beginsWith", "endsWith", "notContainsText", "containsBlanks", "notContainsBlanks", "containsErrors", "notContainsErrors", "timePeriod":
		if !conRelleno() || f(0) == "" {
			return r, false
		}
		r.Kind, r.Formula = "formula", "="+f(0)
		return r, true
	case "top10":
		n, err := strconv.Atoi(rx.Rank)
		if err != nil || n <= 0 || !conRelleno() {
			return r, false
		}
		r.Kind = "top"
		if rx.Bottom == "1" || rx.Bottom == "true" {
			r.Kind = "bottom"
		}
		r.Percent = rx.Percent == "1" || rx.Percent == "true"
		r.Rank = min(n, map[bool]int{true: 100, false: 1000}[r.Percent])
		return r, true
	case "duplicateValues", "uniqueValues":
		if !conRelleno() {
			return r, false
		}
		r.Kind = map[string]string{"duplicateValues": "dup", "uniqueValues": "unique"}[rx.Tipo]
		return r, true
	case "dataBar":
		if e == nil {
			return r, false
		}
		r.Color = e.resolver(rx.DataBar.Color)
		if r.Color == "" {
			return r, false
		}
		r.Kind = "dataBar"
		return r, true
	case "colorScale":
		if e == nil {
			return r, false
		}
		for _, c := range rx.ColorScale.Colores {
			if h := e.resolver(c); h != "" {
				r.Scale = append(r.Scale, h)
			}
		}
		if len(r.Scale) != 2 && len(r.Scale) != 3 {
			return r, false
		}
		r.Kind = "colorScale"
		return r, true
	case "iconSet":
		for _, i := range iconosDeXlsx {
			if strings.Contains(rx.IconSet.Nombre, i.trozo) {
				r.Kind, r.Icons = "iconSet", i.id
				return r, true
			}
		}
	}
	return r, false
}

// validacionesDeXlsx pasa los `dataValidation` de la hoja al modelo.
//
//nolint:gocognit,gocyclo // traductor de un formato a otro: un caso por tipo, y trocearlo reparte la tabla sin simplificarla
func validacionesDeXlsx(dv []validacionXlsxEntrada) []dataValidation {
	var out []dataValidation
	for _, vx := range dv {
		for _, rg := range partesSqref(vx.Sqref) {
			if len(out) >= maxValidacionesHoja {
				return out
			}
			v := dataValidation{Range: rg.a1(), Error: strings.TrimSpace(vx.Error)}
			v.Strict = (vx.ErrorStyle == "" || vx.ErrorStyle == "stop") && vx.ShowError != "0" && vx.ShowError != "false"
			f1 := strings.TrimSpace(vx.F1)
			op := vx.Operador
			if op == "" {
				op = "between"
			}
			switch vx.Tipo {
			case "list":
				if len(f1) >= 2 && f1[0] == '"' && f1[len(f1)-1] == '"' {
					v.Values = valoresLista(strings.ReplaceAll(f1[1:len(f1)-1], `""`, `"`))
				} else {
					v.listaDe = f1
				}
				if len(v.Values) == 0 && v.listaDe == "" {
					continue
				}
			case "custom":
				if f1 == "" {
					continue
				}
				v.Type, v.Formula = "formula", "="+f1
			case "whole", "decimal", "date", "textLength":
				v.Type = map[string]string{"whole": "number", "decimal": "number", "date": "date", "textLength": "textLength"}[vx.Tipo]
				v.Integer = vx.Tipo == "whole"
				var ok bool
				switch op {
				case "between", "notBetween":
					v.Op = op
				default:
					v.Op = opDeCelda[op]
				}
				v.A, ok = operandoDeValidacion(v.Type, f1)
				if ok && (v.Op == "between" || v.Op == "notBetween") {
					v.B, ok = operandoDeValidacion(v.Type, strings.TrimSpace(vx.F2))
				}
				if !ok || v.Op == "" {
					continue
				}
			default:
				continue
			}
			out = append(out, v)
		}
	}
	return out
}

// valoresLista trocea una lista de Excel («Norte,Sur,Este») en valores únicos y sin
// vacíos, como `parseValidationValues`.
func valoresLista(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// operandoDeValidacion es un operando de OOXML (un número) como texto del editor: un
// serial de fecha se escribe «dd/mm/aaaa».
func operandoDeValidacion(tipo, f string) (string, bool) {
	n, err := strconv.ParseFloat(strings.TrimSpace(f), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return "", false
	}
	if tipo == "date" {
		return textoFechaEs(n), true
	}
	return textoNumEs(n), true
}

// resolverListas da valores a las validaciones de lista que apuntan a un rango («$A$1:$A$9»,
// «Hoja2!$B$2:$B$5»): los de las celdas del libro, que es lo que Excel enseña.
func resolverListas(m *sheetModel) {
	for i := range m.Sheets {
		sd := &m.Sheets[i]
		var ok []dataValidation
		for _, v := range sd.Validations {
			if v.listaDe != "" {
				v.Values = valoresDeRango(m, sd, v.listaDe)
				v.listaDe = ""
				if len(v.Values) == 0 {
					continue
				}
			}
			ok = append(ok, v)
		}
		sd.Validations = ok
	}
}

// valoresDeRango son los textos de las celdas de un rango A1 (con hoja opcional), de uno
// en uno y sin repetir, hasta 500.
func valoresDeRango(m *sheetModel, aqui *sheetData, ref string) []string {
	hoja := aqui
	if i := strings.LastIndexByte(ref, '!'); i >= 0 {
		nombre := strings.Trim(ref[:i], "'")
		hoja = nil
		for j := range m.Sheets {
			if strings.EqualFold(m.Sheets[j].Name, nombre) {
				hoja = &m.Sheets[j]
			}
		}
		ref = ref[i+1:]
	}
	if hoja == nil {
		return nil
	}
	rg, ok := leerRango(ref)
	if !ok || !rg.valido() || (rg.r2-rg.r1+1)*(rg.c2-rg.c1+1) > 5000 {
		return nil
	}
	var out []string
	for r := rg.r1; r <= rg.r2 && len(out) < 500; r++ {
		for c := rg.c1; c <= rg.c2 && len(out) < 500; c++ {
			if t := strings.ReplaceAll(strings.TrimSpace(cellRaw(hoja.Cells[A1Ref(r, c)])), ",", ";"); t != "" && !slices.Contains(out, t) {
				out = append(out, t)
			}
		}
	}
	return out
}
