// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"cmp"
	"encoding/xml"
	"math"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/image/colornames"
)

// Colores y rellenos de DrawingML (ECMA-376 §20.1.2.3 y §20.1.8), como los escriben
// PowerPoint, Keynote, Google y LibreOffice: los seis elementos de color, con sus
// transformaciones (lumMod, lumOff, tint, shade, satMod, alpha…) aplicadas en el orden en
// que vienen, y los rellenos liso, degradado, de trama y de imagen. El editor solo pinta un
// color liso por relleno: un degradado entra con su color medio y una trama con la mezcla de
// sus dos colores, que es lo que se ve de lejos.

// rgbDML es un color en sRGB, cada canal en 0..1, con su opacidad.
type rgbDML struct{ r, g, b, a float64 }

func rgbDeHex(hex string) (rgbDML, bool) {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) == 8 { // ARGB
		hex = hex[2:]
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil || len(hex) != 6 {
		return rgbDML{}, false
	}
	return rgbDML{float64(v>>16&255) / 255, float64(v>>8&255) / 255, float64(v&255) / 255, 1}, true
}

func (c rgbDML) hex() string {
	canal := func(x float64) uint64 { return uint64(math.Round(math.Min(1, math.Max(0, x)) * 255)) }
	s := strconv.FormatUint(canal(c.r)<<16|canal(c.g)<<8|canal(c.b), 16)
	return strings.ToUpper(strings.Repeat("0", 6-len(s)) + s)
}

// sobre es el color con su opacidad ya mezclada sobre un fondo opaco.
func (c rgbDML) sobre(fondo rgbDML) rgbDML {
	a := math.Min(1, math.Max(0, c.a))
	return rgbDML{c.r*a + fondo.r*(1-a), c.g*a + fondo.g*(1-a), c.b*a + fondo.b*(1-a), 1}
}

// mezcla es c·k + o·(1−k), canal a canal (también la opacidad).
func (c rgbDML) mezcla(o rgbDML, k float64) rgbDML {
	return rgbDML{c.r*k + o.r*(1-k), c.g*k + o.g*(1-k), c.b*k + o.b*(1-k), c.a*k + o.a*(1-k)}
}

// lineal y srgb pasan un canal entre sRGB y RGB lineal (IEC 61966-2-1).
func lineal(x float64) float64 {
	if x <= 0.04045 {
		return x / 12.92
	}
	return math.Pow((x+0.055)/1.055, 2.4)
}

func srgb(x float64) float64 {
	x = math.Min(1, math.Max(0, x))
	if x <= 0.0031308 {
		return x * 12.92
	}
	return 1.055*math.Pow(x, 1/2.4) - 0.055
}

// hsl y deHSL pasan entre RGB y HSL (h en 0..1).
func (c rgbDML) hsl() (h, s, l float64) {
	mx, mn := math.Max(c.r, math.Max(c.g, c.b)), math.Min(c.r, math.Min(c.g, c.b))
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case c.r:
		h = (c.g - c.b) / d
		if c.g < c.b {
			h += 6
		}
	case c.g:
		h = (c.b-c.r)/d + 2
	default:
		h = (c.r-c.g)/d + 4
	}
	return h / 6, s, l
}

func deHSL(h, s, l, a float64) rgbDML {
	h -= math.Floor(h)
	s, l = math.Min(1, math.Max(0, s)), math.Min(1, math.Max(0, l))
	if s == 0 {
		return rgbDML{l, l, l, a}
	}
	q := l + s - l*s
	if l < 0.5 {
		q = l * (1 + s)
	}
	p := 2*l - q
	canal := func(t float64) float64 {
		t -= math.Floor(t)
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
	return rgbDML{canal(h + 1.0/3), canal(h), canal(h - 1.0/3), a}
}

// transformar aplica una transformación de color de DrawingML. `v` va en milésimas de
// porcentaje (100000 = 100 %) salvo los ángulos (sesenta milésimas de grado).
func (c rgbDML) transformar(nombre string, v float64) rgbDML {
	f := v / 100000
	switch nombre {
	case "alpha":
		c.a = f
	case "alphaMod":
		c.a *= f
	case "alphaOff":
		c.a += f
	case "lumMod", "lumOff", "satMod", "satOff", "sat", "lum", "hueMod", "hueOff", "hue":
		h, s, l := c.hsl()
		switch nombre {
		case "lumMod":
			l *= f
		case "lumOff":
			l += f
		case "lum":
			l = f
		case "satMod":
			s *= f
		case "satOff":
			s += f
		case "sat":
			s = f
		case "hueMod":
			h *= f
		case "hueOff":
			h += v / 60000 / 360
		case "hue":
			h = v / 60000 / 360
		}
		return deHSL(h, s, l, c.a)
	case "tint", "shade":
		// [MS-OI29500]: un tinte acerca el color al blanco y una sombra al negro, en RGB
		// lineal; `val` es cuánto queda del color de partida.
		ch := func(x float64) float64 {
			if nombre == "tint" {
				return srgb(lineal(x)*f + (1 - f))
			}
			return srgb(lineal(x) * f)
		}
		return rgbDML{ch(c.r), ch(c.g), ch(c.b), c.a}
	case "inv":
		return rgbDML{1 - c.r, 1 - c.g, 1 - c.b, c.a}
	case "comp":
		h, s, l := c.hsl()
		return deHSL(h+0.5, s, l, c.a)
	case "gray":
		y := 0.2126*c.r + 0.7152*c.g + 0.0722*c.b
		return rgbDML{y, y, y, c.a}
	}
	return c
}

// colorDML lee un elemento de color de DrawingML ya abierto (`se`) hasta su cierre, con sus
// transformaciones. `esquema` resuelve los `schemeClr` (bg1, tx1, accent1…, phClr) a
// «RRGGBB». false si no es un color o no se puede resolver.
func colorDML(dec *xml.Decoder, se xml.StartElement, esquema func(string) string) (rgbDML, bool) {
	var c rgbDML
	ok := true
	pct := func(nombre string) float64 { return float64(atoiDefault(attrLocal(se, nombre), 0)) / 100000 }
	switch se.Name.Local {
	case "srgbClr":
		c, ok = rgbDeHex(attrLocal(se, "val"))
	case "schemeClr":
		c, ok = rgbDeHex(esquema(attrLocal(se, "val")))
	case "sysClr":
		v := attrLocal(se, "lastClr")
		if v == "" {
			v = map[string]string{"windowText": "000000", "window": "FFFFFF", "btnText": "000000", "btnFace": "F0F0F0", "menuText": "000000", "menu": "FFFFFF"}[attrLocal(se, "val")]
		}
		c, ok = rgbDeHex(v)
	case "prstClr":
		c, ok = colorPredefinido(attrLocal(se, "val"))
	case "scrgbClr":
		c = rgbDML{srgb(pct("r")), srgb(pct("g")), srgb(pct("b")), 1}
	case "hslClr":
		c = deHSL(float64(atoiDefault(attrLocal(se, "hue"), 0))/60000/360, pct("sat"), pct("lum"), 1)
	default:
		_ = dec.Skip()
		return rgbDML{}, false
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return c, ok
		}
		switch t := tok.(type) {
		case xml.StartElement:
			v, err := strconv.ParseFloat(attrLocal(t, "val"), 64)
			if err != nil {
				v = 0
			}
			c = c.transformar(t.Name.Local, v)
			_ = dec.Skip()
		case xml.EndElement:
			if t.Name.Local == se.Name.Local {
				return c, ok
			}
		}
	}
}

// colorPredefinido es un `prstClr`: los nombres de color de SVG/CSS, con las abreviaturas
// de DrawingML (dk, lt, med).
func colorPredefinido(nombre string) (rgbDML, bool) {
	n := strings.ToLower(nombre)
	for _, r := range [][2]string{{"dk", "dark"}, {"lt", "light"}, {"med", "medium"}} {
		if strings.HasPrefix(n, r[0]) && !strings.HasPrefix(n, r[1]) {
			n = r[1] + n[len(r[0]):]
		}
	}
	c, ok := colornames.Map[n]
	if !ok {
		return rgbDML{}, false
	}
	return rgbDML{float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255, 1}, true
}

// esColorDML dice si el elemento es uno de los seis de color.
func esColorDML(local string) bool {
	switch local {
	case "srgbClr", "schemeClr", "sysClr", "prstClr", "scrgbClr", "hslClr":
		return true
	}
	return false
}

// pinturaDML es un relleno ya leído.
type pinturaDML struct {
	color  rgbDML
	hay    bool   // hay color (liso, medio de un degradado o mezcla de una trama)
	nada   bool   // a:noFill
	imagen string // r:embed de un a:blipFill
	grupo  bool   // a:grpFill: el del grupo que lo contiene
}

// esRellenoDML dice si el elemento es un relleno.
func esRellenoDML(local string) bool {
	switch local {
	case "solidFill", "gradFill", "pattFill", "blipFill", "noFill", "grpFill":
		return true
	}
	return false
}

// rellenoDML lee un relleno ya abierto (solidFill, gradFill, pattFill, blipFill, noFill,
// grpFill) hasta su cierre.
func rellenoDML(dec *xml.Decoder, se xml.StartElement, esquema func(string) string) pinturaDML {
	switch se.Name.Local {
	case "noFill", "grpFill":
		_ = dec.Skip()
		return pinturaDML{nada: se.Name.Local == "noFill", grupo: se.Name.Local == "grpFill"}
	}
	var p pinturaDML
	var paradas []paradaDML
	var fg, bg *rgbDML
	pos := 0.0
	for {
		tok, err := dec.Token()
		if err != nil {
			return p
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Local == "gs":
				pos = float64(atoiDefault(attrLocal(t, "pos"), 0)) / 100000
			case t.Name.Local == "blip":
				p.imagen = attrLocal(t, "embed")
				_ = dec.Skip()
			case esColorDML(t.Name.Local):
				if c, ok := colorDML(dec, t, esquema); ok && se.Name.Local == "solidFill" {
					p.color, p.hay = c, true
				} else if ok {
					paradas = append(paradas, paradaDML{pos, c})
				}
			case t.Name.Local == "fgClr" || t.Name.Local == "bgClr":
				if c, ok := colorHijo(dec, t, esquema); ok && t.Name.Local == "fgClr" {
					fg = &c
				} else if ok {
					bg = &c
				}
			}
		case xml.EndElement:
			if t.Name.Local != se.Name.Local {
				continue
			}
			switch se.Name.Local {
			case "gradFill":
				p.color, p.hay = mediaDeDegradado(paradas)
			case "pattFill":
				p.color, p.hay = mezclaDeTrama(fg, bg, attrLocal(se, "prst")), true
			}
			return p
		}
	}
}

// paradaDML es una parada de un degradado: su posición (0..1) y su color.
type paradaDML struct {
	pos float64
	c   rgbDML
}

// mediaDeDegradado es el color medio a lo largo de un degradado: cada tramo entre dos paradas
// pesa lo que mide, y lo de antes de la primera y después de la última es de su color.
func mediaDeDegradado(paradas []paradaDML) (rgbDML, bool) {
	if len(paradas) == 0 {
		return rgbDML{}, false
	}
	slices.SortStableFunc(paradas, func(a, b paradaDML) int { return cmp.Compare(a.pos, b.pos) })
	var m rgbDML
	sumar := func(c rgbDML, w float64) {
		m.r, m.g, m.b, m.a = m.r+c.r*w, m.g+c.g*w, m.b+c.b*w, m.a+c.a*w
	}
	sumar(paradas[0].c, math.Max(0, paradas[0].pos))
	for i := 1; i < len(paradas); i++ {
		sumar(paradas[i].c.mezcla(paradas[i-1].c, 0.5), paradas[i].pos-paradas[i-1].pos)
	}
	sumar(paradas[len(paradas)-1].c, math.Max(0, 1-paradas[len(paradas)-1].pos))
	return m, true
}

// mezclaDeTrama es lo que se ve de lejos de una trama: su color de primer plano sobre el de
// fondo en la parte que cubre. Sin colores, negro sobre blanco.
func mezclaDeTrama(fg, bg *rgbDML, prst string) rgbDML {
	negro, blanco := rgbDML{0, 0, 0, 1}, rgbDML{1, 1, 1, 1}
	if fg == nil {
		fg = &negro
	}
	if bg == nil {
		bg = &blanco
	}
	return fg.mezcla(*bg, coberturaTrama(prst))
}

// colorHijo lee el primer color de un elemento que lo envuelve (fgClr, bgClr…) hasta su cierre.
func colorHijo(dec *xml.Decoder, se xml.StartElement, esquema func(string) string) (rgbDML, bool) {
	var c rgbDML
	ok := false
	for {
		tok, err := dec.Token()
		if err != nil {
			return c, ok
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if esColorDML(t.Name.Local) && !ok {
				c, ok = colorDML(dec, t, esquema)
			} else {
				_ = dec.Skip()
			}
		case xml.EndElement:
			if t.Name.Local == se.Name.Local {
				return c, ok
			}
		}
	}
}

// coberturaTrama es la parte de la celda que cubre el color de primer plano de una trama:
// la de DrawingML (`pct5`…`pct90`, `dkHorz`…) y la de SpreadsheetML (`gray125`, `darkGrid`…).
// Las de líneas y rejillas, por su grosor a la vista.
func coberturaTrama(prst string) float64 {
	if n, ok := strings.CutPrefix(prst, "pct"); ok {
		if v, err := strconv.Atoi(n); err == nil {
			return float64(v) / 100
		}
	}
	switch prst {
	case "solid":
		return 1
	case "gray0625":
		return 0.0625
	case "gray125":
		return 0.125
	case "lightGray":
		return 0.25
	case "mediumGray":
		return 0.5
	case "darkGray":
		return 0.75
	}
	l := strings.ToLower(prst)
	switch {
	case strings.HasPrefix(l, "dk") || strings.HasPrefix(l, "dark"):
		return 0.5
	case strings.HasPrefix(l, "lt") || strings.HasPrefix(l, "light"):
		return 0.25
	case strings.HasPrefix(l, "wd") || strings.HasPrefix(l, "wide"):
		return 0.35
	case strings.HasPrefix(l, "narrow") || strings.HasPrefix(l, "dash") || strings.HasPrefix(l, "sm") || strings.HasPrefix(l, "lg"):
		return 0.3
	}
	return 0.5
}

// listasDeEstilos son las listas de estilos de un tema (`a:fmtScheme`): el XML de cada relleno
// (`fillStyleLst`), cada fondo (`bgFillStyleLst`) y cada trazo (`lnStyleLst`), en orden.
func listasDeEstilos(raw []byte) (rellenos, fondos, lineas [][]byte) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var lista *[][]byte
	for {
		ini := dec.InputOffset()
		tok, err := dec.Token()
		if err != nil {
			return rellenos, fondos, lineas
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "fillStyleLst":
				lista = &rellenos
			case "bgFillStyleLst":
				lista = &fondos
			case "lnStyleLst":
				lista = &lineas
			default:
				if lista != nil && len(*lista) < 16 {
					_ = dec.Skip()
					*lista = append(*lista, raw[ini:dec.InputOffset()])
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "fillStyleLst", "bgFillStyleLst", "lnStyleLst":
				lista = nil
			}
		}
	}
}

// rellenoDeEstilo es el relleno `idx` (desde 1) de una lista de estilos del tema, con `ph` en
// el lugar de phClr. Un trazo (`a:ln`) da el relleno de su línea.
func rellenoDeEstilo(lista [][]byte, idx int, ph rgbDML, esquema func(string) string) (pinturaDML, bool) {
	if idx < 1 || idx > len(lista) {
		return pinturaDML{}, false
	}
	hexPh := ph.hex()
	esq := func(n string) string {
		if n == "phClr" {
			return hexPh
		}
		return esquema(n)
	}
	dec := xml.NewDecoder(bytes.NewReader(lista[idx-1]))
	for {
		tok, err := dec.Token()
		if err != nil {
			return pinturaDML{}, false
		}
		if se, ok := tok.(xml.StartElement); ok && esRellenoDML(se.Name.Local) {
			r := rellenoDML(dec, se, esq)
			if r.hay {
				r.color.a *= ph.a // la transparencia de la referencia también cuenta
			}
			return r, true
		}
	}
}
