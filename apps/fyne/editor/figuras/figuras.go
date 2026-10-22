// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package figuras es la geometría de las formas (`slides/shapes.ts`) y la lectura de los
// datos de un gráfico (`extractChartData` y `niceTicks` de `sheets/Chart.tsx`) que la web
// comparte entre las diapositivas y los documentos. Aquí va una sola vez: Diapos y Docus
// la importan, cada uno con su propio SVG alrededor, como en la web.
package figuras

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// N2 es `round2`/`cpx`: centésimas de píxel, como escribe la web sus caminos.
func N2(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

// Inset es INSET de shapes.ts: el margen para que el trazo de 2 no se recorte.
const Inset = 2

// Punto es `ShapePoint`.
type Punto struct{ X, Y float64 }

// Vertices es `polygonPoints`: los vértices de las formas POLIGONALES; vacío para las
// demás, que se pintan con su primitiva.
func Vertices(forma string, w, h float64) []Punto {
	p := float64(Inset)
	cx, cy := w/2, h/2
	rx, ry := max(0.5, cx-p), max(0.5, cy-p)
	x0, x1, y0, y1 := p, w-p, p, h-p
	switch forma {
	case "triangle":
		return []Punto{{cx, y0}, {x1, y1}, {x0, y1}}
	case "diamond":
		return []Punto{{cx, y0}, {x1, cy}, {cx, y1}, {x0, cy}}
	case "pentagon":
		return regular(cx, cy, rx, ry, 5, -math.Pi/2)
	case "hexagon":
		return regular(cx, cy, rx, ry, 6, 0)
	case "star":
		return estrella(cx, cy, rx, ry, 5, -math.Pi/2, 0.42)
	case "callout":
		return bocadillo(w, h, p)
	}
	return nil
}

func regular(cx, cy, rx, ry float64, n int, inicio float64) []Punto {
	out := make([]Punto, n)
	for k := range n {
		a := inicio + float64(k)*2*math.Pi/float64(n)
		out[k] = Punto{cx + rx*math.Cos(a), cy + ry*math.Sin(a)}
	}
	return out
}

func estrella(cx, cy, rx, ry float64, puntas int, inicio, dentro float64) []Punto {
	out := make([]Punto, 2*puntas)
	for k := range out {
		a := inicio + float64(k)*math.Pi/float64(puntas)
		r := 1.0
		if k%2 == 1 {
			r = dentro
		}
		out[k] = Punto{cx + rx*r*math.Cos(a), cy + ry*r*math.Sin(a)}
	}
	return out
}

// Acotar es `clamp`.
func Acotar(n, lo, hi float64) float64 { return min(hi, max(lo, n)) }

// bocadillo es `callout`: el cuerpo con la cola abajo a la izquierda.
func bocadillo(w, h, p float64) []Punto {
	x0, x1, y0, y1 := p, w-p, p, h-p
	cola := Acotar(h*0.22, 4, max(4, (y1-y0)*0.45))
	yb := y1 - cola
	bx := x0 + w*0.16
	bw := min(w*0.16, x1-bx)
	punta := Acotar(x0+w*0.06, x0, x1)
	return []Punto{{x0, y0}, {x1, y0}, {x1, yb}, {bx + bw, yb}, {punta, y1}, {bx, yb}, {x0, yb}}
}

// PuntosSVG es `polygonAttr`.
func PuntosSVG(pts []Punto) string {
	partes := make([]string, len(pts))
	for i, p := range pts {
		partes[i] = N2(p.X) + "," + N2(p.Y)
	}
	return strings.Join(partes, " ")
}

// Cilindro es `CylinderDims`.
type Cilindro struct{ rx, ry, cx, yt, yb float64 }

// MedidasCilindro es `cylinderDims`.
func MedidasCilindro(w, h float64) Cilindro {
	p := float64(Inset)
	rx := max(0.5, w/2-p)
	ry := Acotar(h*0.14, 3, max(3, h/2-p))
	return Cilindro{rx: rx, ry: ry, cx: w / 2, yt: p + ry, yb: h - p - ry}
}

// Cuerpo es `cylinderBodyPath`: cuerpo, tapa de arriba por detrás y la de abajo por
// delante, con los dos arcos de barrido 0.
func (d Cilindro) Cuerpo() string {
	xl, xr := N2(d.cx-d.rx), N2(d.cx+d.rx)
	return "M " + xl + " " + N2(d.yt) + " V " + N2(d.yb) + " A " + N2(d.rx) + " " + N2(d.ry) + " 0 0 0 " + xr + " " + N2(d.yb) +
		" V " + N2(d.yt) + " A " + N2(d.rx) + " " + N2(d.ry) + " 0 0 0 " + xl + " " + N2(d.yt) + " Z"
}

// Costura es `cylinderSeamPath`: el arco frontal de la tapa de arriba.
func (d Cilindro) Costura() string {
	return "M " + N2(d.cx-d.rx) + " " + N2(d.yt) + " A " + N2(d.rx) + " " + N2(d.ry) + " 0 0 0 " + N2(d.cx+d.rx) + " " + N2(d.yt)
}

// Paleta es CONTENT_PALETTE: los colores que la persona aplica a SU documento. No son
// tokens de interfaz y por eso van en hex, y en hex se guardan en el modelo.
var Paleta = []string{
	"#0A0A0A", "#475569", "#94A3B8", "#FFFFFF", "#F97316", "#DC2626",
	"#D97706", "#16A34A", "#2563EB", "#7C3AED", "#DB2777", "#0D9488",
}

// Los colores del gráfico. Series es CHART_SERIES, el subconjunto AA de CONTENT_PALETTE
// en su orden: más series que colores no se pintan, como en la web.
var Series = []string{"#2563EB", "#DC2626", "#16A34A", "#7C3AED", "#D97706", "#DB2777", "#0D9488"}

const (
	Apagado  = "#475569" // CHART_MUTED = CONTENT_PALETTE[1]
	Eje      = "#94A3B8" // CHART_AXIS = CONTENT_PALETTE[2]
	Reticula = "#E2E8F0" // CHART_GRID = HIGHLIGHT_PALETTE[6]
)

var hayLetras = regexp.MustCompile(`[a-zA-Z]`)

// Celda es `parseChartCell`: número (coma decimal de es-ES) o texto.
func Celda(crudo string) (float64, string, bool) {
	t := strings.TrimSpace(crudo)
	if t == "" || hayLetras.MatchString(t) {
		return 0, t, false
	}
	s := t
	if strings.Contains(t, ",") {
		s = strings.Replace(strings.ReplaceAll(t, ".", ""), ",", ".", 1)
	}
	s = strings.ReplaceAll(s, "%", "")
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
		return 0, t, false
	}
	return n, "", true
}

// Serie es `ChartSeries`.
type Serie struct {
	Nombre  string
	Valores []float64
}

// Datos es `extractChartData` sobre la rejilla entera: la primera columna son los
// rótulos si hay más de una, y la primera fila la cabecera si es toda texto.
func Datos(rejilla [][]string) ([]string, []Serie) {
	filas := len(rejilla)
	cols := 0
	for _, f := range rejilla {
		cols = max(cols, len(f))
	}
	if filas == 0 || cols == 0 {
		return nil, nil
	}
	celda := func(c, r int) (float64, string, bool) {
		if c < len(rejilla[r]) {
			return Celda(rejilla[r][c])
		}
		return 0, "", false
	}
	texto := func(c, r int) string {
		n, s, num := celda(c, r)
		if num {
			return formula.FormatNumber(n)
		}
		return s
	}
	conRotulos := cols > 1
	c1 := 0
	if conRotulos {
		c1 = 1
	}
	cabecera := filas > 1
	for c := c1; cabecera && c < cols; c++ {
		if _, s, num := celda(c, 0); num || s == "" {
			cabecera = false
		}
	}
	r1 := 0
	if cabecera {
		r1 = 1
	}
	rotulos := make([]string, 0, filas-r1)
	for r := r1; r < filas; r++ {
		if conRotulos {
			rotulos = append(rotulos, texto(0, r))
		} else {
			rotulos = append(rotulos, strconv.Itoa(r+1))
		}
	}
	series := make([]Serie, 0, cols-c1)
	for c := c1; c < cols; c++ {
		s := Serie{Nombre: formula.NumToCol(c)}
		if cabecera {
			s.Nombre = texto(c, 0)
		}
		for r := r1; r < filas; r++ {
			n, _, num := celda(c, r)
			if !num {
				n = 0
			}
			s.Valores = append(s.Valores, n)
		}
		series = append(series, s)
	}
	return rotulos, series
}

// Escala es `Scale`.
type Escala struct {
	Min, Max float64
	Marcas   []float64
}

// Marcas es `niceTicks`: paso 1·2·5×10^k con unas cuatro divisiones.
func Marcas(lo, hi float64) Escala {
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo == hi {
		switch {
		case hi > 0:
			lo = 0
		case hi < 0:
			hi = 0
		default:
			hi = 1
		}
	}
	bruto := (hi - lo) / 4
	mag := math.Pow(10, math.Floor(math.Log10(bruto)))
	norm := bruto / mag
	paso := mag
	switch {
	case norm > 5:
		paso = 10 * mag
	case norm > 2:
		paso = 5 * mag
	case norm > 1:
		paso = 2 * mag
	}
	base := math.Floor(lo/paso) * paso
	n := int(math.Round((math.Ceil(hi/paso)*paso - base) / paso))
	marcas := make([]float64, 0, n+1)
	for i := 0; i <= n; i++ {
		v, _ := strconv.ParseFloat(strconv.FormatFloat(base+float64(i)*paso, 'g', 12, 64), 64)
		marcas = append(marcas, v)
	}
	return Escala{Min: marcas[0], Max: marcas[n], Marcas: marcas}
}

// Numero es CHART_NUM: `Intl.NumberFormat('es-ES', { maximumFractionDigits: 2 })`.
// Es-ES FIJO, como en la web: el gráfico no cambia con el idioma de quien lo mira.
func Numero(v float64) string {
	return i18n.FormatNumber("es", math.Round(v*100)/100)
}

// Ancla es el `text-anchor` de un rótulo del SVG.
type Ancla uint8

// Los tres anclajes que usan el gráfico y el diagrama.
const (
	AnclaInicio Ancla = iota
	AnclaMedio
	AnclaFin
)

// Rotulo es un `<text>` del SVG, que `oksvg` no pinta: lo coloca quien pinta en su
// línea base. X e Y en px de la caja; Y es la LÍNEA BASE, como en SVG.
type Rotulo struct {
	Texto string
	X, Y  float64
	Tam   float64
	Ancla Ancla
	Color string
}
