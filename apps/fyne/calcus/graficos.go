// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/numfmt"
)

// # Los gráficos: la ESCENA de `sheets/Chart.tsx`
//
// La web no dibuja con una librería: calcula una escena —rectángulos, trazos, círculos,
// líneas y textos ya colocados en un viewBox— y la pinta como SVG. Esto es esa misma
// función, línea a línea, y `vectores_test.go` exige que dé la MISMA escena con las
// mismas entradas (los números, los trazos y hasta los rótulos ya formateados). Cómo se
// pinta en Fyne está en `graficos_tarjeta.go`.
//
// # Los `float64(a*b)` no sobran
//
// En arm64 el compilador de Go funde `a*b + c` en una sola instrucción (FMA) que no
// redondea el producto, y JavaScript nunca lo hace: medido, dos coordenadas salían a
// una centésima de la web (155,22 frente a 155,23). La conversión explícita obliga a
// redondear el producto, que es lo que dice la especificación de Go para evitarlo.

// Las medidas de la web.
const (
	anchoBase, altoBase = 320.0, 180.0
	// escalaGrafico es `CHART_K`: px de pantalla por unidad del viewBox.
	escalaGrafico = 278.0 / 320.0
	// anchoAreaDefecto y altoAreaDefecto son `DEFAULT_PLOT_W/H`.
	anchoAreaDefecto = 278.0
	altoAreaDefecto  = 278.0 * altoBase / anchoBase
	// anchoLeyendaLado es `LEGEND_SIDE_W` (w-24).
	anchoLeyendaLado = 84.0

	padArr, padDer, padAbj, padIzq = 10.0, 8.0, 20.0, 42.0
	padDer2                        = 34.0 // con eje secundario
	tituloEje                      = 12.0
)

// La paleta: `SERIES_COLORS` y los tonos de `CONTENT_PALETTE`/`HIGHLIGHT_PALETTE`.
var coloresSerie = []string{"#2563EB", "#DC2626", "#16A34A", "#7C3AED", "#D97706", "#DB2777", "#0D9488"}

const (
	tintaGrafico  = "#0A0A0A"
	apagado       = "#475569"
	colorEje      = "#94A3B8"
	papelGrafico  = "#FFFFFF"
	colorOtros    = "#94A3B8"
	lineaReticula = "#E2E8F0"
)

// marca es `ChartMark`: una pieza de la escena en unidades del viewBox. Qué campos
// valen depende de K.
type marca struct {
	K              string // rect, path, circle, line o text
	X, Y, W, H     float64
	D              string
	CX, CY, R      float64
	X1, Y1, X2, Y2 float64
	Fill, Stroke   string
	Op, SW         float64 // 0 = ausente
	Round          bool
	Title          string
	Text           string
	Size           float64
	Anchor         string // start, middle o end
}

// escena es `ChartScene`.
type escena struct {
	W, H   float64
	Marcas []marca
}

// itemLeyenda es `LegendItem`.
type itemLeyenda struct{ Nombre, Color string }

// vistaGrafico es `ChartView`.
type vistaGrafico struct {
	Escena  escena
	Leyenda []itemLeyenda
	Series  []string
	Vacio   bool
}

// serieGrafico y datosGrafico son `ChartSeries` y `ChartData`.
type serieGrafico struct {
	Nombre  string
	Valores []float64
}

type datosGrafico struct {
	Rotulos []string
	Series  []serieGrafico
}

// datosXY es `ChartXY`.
type datosXY struct {
	X      []float64
	Series []serieGrafico
}

// escalaEje es `Scale`.
type escalaEje struct {
	Min, Max float64
	Marcas   []float64
}

// cent es `px`: al céntimo, con la mitad hacia arriba como `Math.round`.
func cent(n float64) float64 { return redondearJS(float64(n*100)) / 100 }

// numSVG es `String(n)` para un número ya redondeado al céntimo (sin el −0).
func numSVG(n float64) string {
	if n == 0 {
		return "0"
	}
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// numES es `Intl.NumberFormat('es-ES', { maximumFractionDigits: 2 })`.
func numES(n float64) string {
	if math.IsNaN(n) {
		return "NaN"
	}
	if math.IsInf(n, 0) {
		if n > 0 {
			return "∞"
		}
		return "-∞"
	}
	s := numfmt.NumeroES(n, 2, 0, true, 2)
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ",")
	}
	return s
}

// recortarRotulo es `clip`: n caracteres como mucho, el último «…».
func recortarRotulo(t string, n int) string {
	r := []rune(t)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return t
}

// orientar es `oriented`: con `porFilas` se traspone el rango.
func orientar(r formula.Rect, get func(c, f int) formula.Value, porFilas bool) (formula.Rect, func(c, f int) formula.Value) {
	if !porFilas {
		return r, get
	}
	return formula.Rect{C1: r.R1, R1: r.C1, C2: r.R2, R2: r.C2}, func(c, f int) formula.Value { return get(f, c) }
}

func esTextoNoVacio(v formula.Value) bool {
	s, ok := v.(string)
	return ok && s != ""
}

func numeroFinito(v formula.Value) (float64, bool) {
	x, ok := v.(float64)
	return x, ok && !math.IsNaN(x) && !math.IsInf(x, 0)
}

// textoCelda es `formatCellValue` sin formato.
func textoCelda(v formula.Value) string { return formula.Display(v) }

// extraerDatos es `extractChartData`.
func extraerDatos(r0 formula.Rect, get0 func(c, f int) formula.Value, porFilas bool) datosGrafico {
	r, get := orientar(r0, get0, porFilas)
	conRotulos := r.C2 > r.C1
	c1 := r.C1
	if conRotulos {
		c1++
	}
	cab := r.R2 > r.R1
	for c := c1; cab && c <= r.C2; c++ {
		if !esTextoNoVacio(get(c, r.R1)) {
			cab = false
		}
	}
	f1 := r.R1
	if cab {
		f1++
	}
	var d datosGrafico
	for f := f1; f <= r.R2; f++ {
		switch {
		case conRotulos:
			d.Rotulos = append(d.Rotulos, textoCelda(get(r.C1, f)))
		case porFilas:
			d.Rotulos = append(d.Rotulos, formula.NumToCol(f))
		default:
			d.Rotulos = append(d.Rotulos, strconv.Itoa(f+1))
		}
	}
	for c := c1; c <= r.C2; c++ {
		s := serieGrafico{}
		for f := f1; f <= r.R2; f++ {
			x, ok := numeroFinito(get(c, f))
			if !ok {
				x = 0
			}
			s.Valores = append(s.Valores, x)
		}
		switch {
		case cab:
			s.Nombre = textoCelda(get(c, r.R1))
		case porFilas:
			s.Nombre = strconv.Itoa(c + 1)
		default:
			s.Nombre = formula.NumToCol(c)
		}
		d.Series = append(d.Series, s)
	}
	return d
}

// extraerXY es `extractXY`.
func extraerXY(r0 formula.Rect, get0 func(c, f int) formula.Value, porFilas bool) datosXY {
	r, get := orientar(r0, get0, porFilas)
	cab := r.R2 > r.R1
	for c := r.C1; cab && c <= r.C2; c++ {
		if !esTextoNoVacio(get(c, r.R1)) {
			cab = false
		}
	}
	f1 := r.R1
	if cab {
		f1++
	}
	num := func(c, f int) float64 {
		x, ok := numeroFinito(get(c, f))
		if !ok {
			return math.NaN()
		}
		return x
	}
	var d datosXY
	for f := f1; f <= r.R2; f++ {
		d.X = append(d.X, num(r.C1, f))
	}
	for c := r.C1 + 1; c <= r.C2; c++ {
		s := serieGrafico{}
		for f := f1; f <= r.R2; f++ {
			s.Valores = append(s.Valores, num(c, f))
		}
		switch {
		case cab:
			s.Nombre = textoCelda(get(c, r.R1))
		case porFilas:
			s.Nombre = strconv.Itoa(c + 1)
		default:
			s.Nombre = formula.NumToCol(c)
		}
		d.Series = append(d.Series, s)
	}
	return d
}

// aPrecision es `Number(x.toPrecision(12))`.
func aPrecision(x float64) float64 {
	v, _ := strconv.ParseFloat(strconv.FormatFloat(x, 'g', 12, 64), 64)
	return v
}

// escalaBonita es `niceTicks`.
func escalaBonita(lo, hi float64) escalaEje {
	if math.IsNaN(lo) || math.IsInf(lo, 0) {
		lo = 0
	}
	if math.IsNaN(hi) || math.IsInf(hi, 0) {
		hi = 0
	}
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
	desde := math.Floor(lo/paso) * paso
	n := int(redondearJS((float64(math.Ceil(hi/paso)*paso) - desde) / paso))
	e := escalaEje{}
	for i := 0; i <= n; i++ {
		e.Marcas = append(e.Marcas, aPrecision(desde+float64(float64(i)*paso)))
	}
	e.Min, e.Max = e.Marcas[0], e.Marcas[n]
	return e
}

// proyectar es `project`.
func proyectar(v, lo, hi, a, b float64) float64 {
	if hi == lo {
		return (a + b) / 2
	}
	return a + float64((v-lo)/(hi-lo)*(b-a))
}

// puntoPolar es `polarPoint`.
func puntoPolar(cx, cy, r float64, i, n int, frac float64) (x, y float64) {
	a := -math.Pi/2 + float64(float64(i)/float64(max(1, n))*2*math.Pi)
	return cx + float64(r*frac*math.Cos(a)), cy + float64(r*frac*math.Sin(a))
}

// escalaDe es `scaleOf`.
func escalaDe(vals []float64, cero bool) escalaEje {
	lo, hi := math.Inf(1), math.Inf(-1)
	if cero {
		lo, hi = 0, 0
	}
	for _, v := range vals {
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			lo, hi = min(lo, v), max(hi, v)
		}
	}
	return escalaBonita(lo, hi)
}

// colorSerie es `seriesColor`.
func colorSerie(g Grafico, k int) string {
	if k < len(g.Colors) && g.Colors[k] != "" {
		return g.Colors[k]
	}
	if k < len(coloresSerie) {
		return coloresSerie[k]
	}
	return colorOtros
}

func esCartesiano(t string) bool { return t == "bar" || t == "line" || t == "area" || t == "combo" }

// Apilable y ConEjes son `STACKABLE_TYPES` y `AXIS_TYPES`.
func Apilable(t string) bool { return t == "bar" || t == "hbar" || t == "area" }
func ConEjes(t string) bool  { return esCartesiano(t) || t == "hbar" || t == "scatter" }

// leyendaVisible es `legendVisible`.
func leyendaVisible(g Grafico, items int) bool { return g.Legend != "none" && items >= 2 }

// tamanoEscena es `sceneSize`.
func tamanoEscena(g Grafico, conLeyenda bool) (w, h float64) {
	ancho, alto := anchoAreaDefecto, altoAreaDefecto
	if g.W != nil {
		ancho = *g.W
	}
	if g.H != nil {
		alto = *g.H
	}
	if conLeyenda && g.Legend == "right" {
		ancho -= anchoLeyendaLado + 1
	}
	return cent(ancho / escalaGrafico), cent(alto / escalaGrafico)
}

// marco es `Frame`.
type marcoGrafico struct{ W, H, l, r, t, b float64 }

func (f marcoGrafico) ancho() float64 { return f.W - f.l - f.r }
func (f marcoGrafico) alto() float64  { return f.H - f.t - f.b }

func marcoPara(g Grafico, w, h, padR float64) marcoGrafico {
	f := marcoGrafico{W: w, H: h, l: padIzq, r: padR, t: padArr, b: padAbj}
	if g.YTitle != "" {
		f.t += tituloEje
	}
	if g.XTitle != "" {
		f.b += tituloEje
	}
	return f
}

func (f marcoGrafico) y(e escalaEje, v float64) float64 {
	return f.t + float64((e.Max-v)/(e.Max-e.Min)*f.alto())
}

func texto(x, y float64, t string, tam float64, fill, ancla string) marca {
	return marca{K: "text", X: x, Y: y, Text: t, Size: tam, Fill: fill, Anchor: ancla}
}

func linea(x1, y1, x2, y2 float64, stroke string) marca {
	return marca{K: "line", X1: x1, Y1: y1, X2: x2, Y2: y2, Stroke: stroke}
}

// titulosEje es `axisTitles`.
func titulosEje(g Grafico, f marcoGrafico, out []marca) []marca {
	if g.YTitle != "" {
		out = append(out, texto(4, 12, recortarRotulo(g.YTitle, 40), 9, apagado, "start"))
	}
	if g.XTitle != "" {
		out = append(out, texto(cent(f.l+f.ancho()/2), cent(f.H-4), recortarRotulo(g.XTitle, 40), 9, apagado, "middle"))
	}
	return out
}

// ejeValores es `valueAxis`.
func ejeValores(g Grafico, f marcoGrafico, e escalaEje, ejeY float64, out []marca) []marca {
	for _, t := range e.Marcas {
		y := cent(f.y(e, t))
		if g.Grid == nil || *g.Grid {
			out = append(out, linea(f.l, y, f.W-f.r, y, lineaReticula))
		}
		out = append(out, texto(f.l-5, y+3, numES(t), 9, apagado, "end"))
	}
	y := cent(ejeY)
	return append(out, linea(f.l, y, f.W-f.r, y, colorEje))
}

// rotulosCategoria es `categoryLabels`.
func rotulosCategoria(rotulos []string, f marcoGrafico, out []marca) []marca {
	n := max(1, len(rotulos))
	hueco := f.ancho() / float64(n)
	paso := int(math.Ceil(float64(n) / 6))
	for i, t := range rotulos {
		if i%paso != 0 || t == "" {
			continue
		}
		out = append(out, texto(cent(f.l+float64(hueco*(float64(i)+0.5))), cent(f.t+f.alto()+14), recortarRotulo(t, 9), 9, apagado, "middle"))
	}
	return out
}

// apilar da, por serie y categoría, la base y la cima de cada tramo (positivos y
// negativos por separado; con `todoArriba`, como el área, todo suma en un sentido).
func apilar(series []serieGrafico, n int, todoArriba bool) (base, cima [][]float64) {
	pos, neg := make([]float64, n), make([]float64, n)
	for _, s := range series {
		b, c := make([]float64, n), make([]float64, n)
		for i, v := range s.Valores {
			acc := neg
			if todoArriba || v >= 0 {
				acc = pos
			}
			b[i] = acc[i]
			acc[i] += v
			c[i] = acc[i]
		}
		base, cima = append(base, b), append(cima, c)
	}
	return base, cima
}

func aplanar(m [][]float64) []float64 {
	var out []float64
	for _, x := range m {
		out = append(out, x...)
	}
	return out
}

func trazo(pts [][2]float64, empieza string) string {
	var b strings.Builder
	for i, p := range pts {
		if i == 0 {
			b.WriteString(empieza)
		} else {
			b.WriteString("L")
		}
		b.WriteString(numSVG(p[0]) + " " + numSVG(p[1]))
	}
	return b.String()
}

// cartesiano es `cartesian`.
//
//nolint:gocyclo,gocognit // es el puerto línea a línea de la web; partirlo lo haría divergir
func cartesiano(g Grafico, d datosGrafico, w, h float64) []marca {
	modo := g.Type
	var out, rotulos []marca
	series := d.Series[:min(len(d.Series), len(coloresSerie))]
	n := max(1, len(d.Rotulos))
	apilado := g.Stacked && (modo == "bar" || modo == "area")
	sec := map[int]bool{}
	if !apilado {
		for _, k := range g.SecondaryAxis {
			if k < len(series) {
				sec[k] = true
			}
		}
	}
	conSec := len(sec) > 0 && len(sec) < len(series)
	enSec := func(k int) bool { return conSec && sec[k] }
	padR := padDer
	if conSec {
		padR = padDer2
	}
	f := marcoPara(g, w, h, padR)
	enLinea := map[int]bool{}
	if modo == "combo" {
		for _, k := range g.LineSeries {
			if k < len(series) {
				enLinea[k] = true
			}
		}
		if len(enLinea) == 0 {
			enLinea[len(series)-1] = true
		}
	}
	esLinea := func(k int) bool { return modo == "line" || modo == "area" || (modo == "combo" && enLinea[k]) }
	base, cima := apilar(series, len(d.Rotulos), modo == "area")
	if !apilado {
		cima = nil
		for _, s := range series {
			cima = append(cima, slices.Clone(s.Valores))
		}
	}
	baseCero := modo != "line"
	var vals []float64
	if apilado {
		vals = append(aplanar(cima), aplanar(base)...)
	} else {
		for k, s := range series {
			if !enSec(k) {
				vals = append(vals, s.Valores...)
			}
		}
	}
	pri := escalaDe(vals, baseCero)
	var secE *escalaEje
	if conSec {
		var v2 []float64
		for k, s := range series {
			if enSec(k) {
				v2 = append(v2, s.Valores...)
			}
		}
		e := escalaDe(v2, false)
		secE = &e
	}
	escK := func(k int) escalaEje {
		if enSec(k) && secE != nil {
			return *secE
		}
		return pri
	}
	ejeY := f.y(pri, pri.Min)
	if baseCero {
		ejeY = f.y(pri, 0)
	}
	out = titulosEje(g, f, out)
	out = ejeValores(g, f, pri, ejeY, out)

	hueco := f.ancho() / float64(n)
	xc := func(i int) float64 { return f.l + float64(hueco*(float64(i)+0.5)) }
	var barras []int
	for k := range series {
		if !esLinea(k) {
			barras = append(barras, k)
		}
	}
	grupo := hueco * 0.72
	bw := grupo
	if !apilado {
		bw = grupo / float64(max(1, len(barras)))
	}
	puntos := len(d.Rotulos) <= 24
	pista := func(i int, s serieGrafico, v float64) string {
		r := ""
		if i < len(d.Rotulos) {
			r = d.Rotulos[i]
		}
		return r + " — " + s.Nombre + ": " + numES(v)
	}

	if modo == "area" {
		for k, s := range series {
			e := escK(k)
			suelo := f.y(e, e.Min)
			if baseCero {
				suelo = f.y(e, 0)
			}
			var arriba [][2]float64
			for i, v := range cima[k] {
				arriba = append(arriba, [2]float64{cent(xc(i)), cent(f.y(e, v))})
			}
			bajada := "L" + numSVG(cent(xc(len(s.Valores)-1))) + " " + numSVG(cent(suelo)) + "L" + numSVG(cent(xc(0))) + " " + numSVG(cent(suelo))
			if apilado {
				var b strings.Builder
				for i := len(base[k]) - 1; i >= 0; i-- {
					b.WriteString("L" + numSVG(cent(xc(i))) + " " + numSVG(cent(f.y(e, base[k][i]))))
				}
				bajada = b.String()
			}
			out = append(out, marca{K: "path", D: trazo(arriba, "M") + bajada + "Z", Fill: colorSerie(g, k), Op: 0.18})
		}
	}
	for j, k := range barras {
		e := escK(k)
		s := series[k]
		for i, v := range s.Valores {
			y0v, yvv := 0.0, v
			if apilado {
				y0v, yvv = base[k][i], cima[k][i]
			}
			y0, yv := f.y(e, y0v), f.y(e, yvv)
			x := f.l + float64(float64(i)*hueco) + (hueco-grupo)/2 + 1
			if !apilado {
				x += float64(float64(j) * bw)
			}
			alto := math.Abs(yv - y0)
			out = append(out, marca{K: "rect", X: cent(x), Y: cent(min(yv, y0)), W: cent(max(1, bw-2)), H: cent(alto), Fill: colorSerie(g, k), Title: pista(i, s, v)})
			if !g.Labels || v == 0 {
				continue
			}
			cx := cent(x + max(1, bw-2)/2)
			if apilado {
				if alto >= 11 {
					rotulos = append(rotulos, texto(cx, cent((yv+y0)/2+3), numES(v), 8, papelGrafico, "middle"))
				}
			} else {
				y := max(yv, y0) + 8
				if v >= 0 {
					y = min(yv, y0) - 2
				}
				rotulos = append(rotulos, texto(cx, cent(y), numES(v), 8, apagado, "middle"))
			}
		}
	}
	for k, s := range series {
		if !esLinea(k) {
			continue
		}
		e := escK(k)
		col := colorSerie(g, k)
		var pts [][2]float64
		for i, v := range cima[k] {
			pts = append(pts, [2]float64{cent(xc(i)), cent(f.y(e, v))})
		}
		out = append(out, marca{K: "path", D: trazo(pts, "M"), Fill: "none", Stroke: col, SW: 2, Round: true})
		for i, p := range pts {
			v := 0.0
			if i < len(s.Valores) {
				v = s.Valores[i]
			}
			if puntos {
				out = append(out, marca{K: "circle", CX: p[0], CY: p[1], R: 2.5, Fill: col, Stroke: papelGrafico, SW: 1, Title: pista(i, s, v)})
			}
			if g.Labels {
				rotulos = append(rotulos, texto(p[0], cent(p[1]-5), numES(v), 8, apagado, "middle"))
			}
		}
	}
	if secE != nil {
		x := cent(f.W - padDer2)
		out = append(out, linea(x, f.t, x, cent(f.t+f.alto()), colorEje))
		for _, t := range secE.Marcas {
			out = append(out, texto(cent(x+4), cent(f.y(*secE, t)+3), numES(t), 9, apagado, "start"))
		}
	}
	out = rotulosCategoria(d.Rotulos, f, out)
	return append(out, rotulos...)
}

// barrasHorizontales es `horizontalBars`.
//
//nolint:gocognit // es el puerto línea a línea de `horizontalBars`; partirlo lo haría divergir
func barrasHorizontales(g Grafico, d datosGrafico, w, h float64) []marca {
	var out, rotulos []marca
	series := d.Series[:min(len(d.Series), len(coloresSerie))]
	n := max(1, len(d.Rotulos))
	f := marcoPara(g, w, h, padDer)
	base, cima := apilar(series, len(d.Rotulos), false)
	var vals []float64
	if g.Stacked {
		vals = append(aplanar(cima), aplanar(base)...)
	} else {
		for _, s := range series {
			vals = append(vals, s.Valores...)
		}
	}
	e := escalaDe(vals, true)
	xDe := func(v float64) float64 { return proyectar(v, e.Min, e.Max, f.l, f.W-f.r) }
	out = titulosEje(g, f, out)
	for _, t := range e.Marcas {
		x := cent(xDe(t))
		if g.Grid == nil || *g.Grid {
			out = append(out, linea(x, f.t, x, cent(f.t+f.alto()), lineaReticula))
		}
		out = append(out, texto(x, cent(f.t+f.alto()+14), numES(t), 9, apagado, "middle"))
	}
	x0 := cent(xDe(0))
	out = append(out, linea(x0, f.t, x0, cent(f.t+f.alto()), colorEje))
	hueco := f.alto() / float64(n)
	grupo := hueco * 0.72
	bh := grupo
	if !g.Stacked {
		bh = grupo / float64(max(1, len(series)))
	}
	paso := int(math.Ceil(float64(n) / 8))
	for i, t := range d.Rotulos {
		if i%paso != 0 || t == "" {
			continue
		}
		out = append(out, texto(f.l-5, cent(f.t+float64(float64(n-1-i)*hueco)+hueco/2+3), recortarRotulo(t, 9), 9, apagado, "end"))
	}
	for k, s := range series {
		for i, v := range s.Valores {
			av, bv := 0.0, v
			if g.Stacked {
				av, bv = base[k][i], cima[k][i]
			}
			a, b := xDe(av), xDe(bv)
			y := f.t + float64(float64(n-1-i)*hueco) + (hueco-grupo)/2 + 1
			if !g.Stacked {
				y += float64(float64(k) * bh)
			}
			hh := max(1, bh-2)
			r := ""
			if i < len(d.Rotulos) {
				r = d.Rotulos[i]
			}
			out = append(out, marca{
				K: "rect", X: cent(min(a, b)), Y: cent(y), W: cent(math.Abs(b - a)), H: cent(hh), Fill: colorSerie(g, k),
				Title: r + " — " + s.Nombre + ": " + numES(v),
			})
			if !g.Labels || v == 0 {
				continue
			}
			if g.Stacked {
				if math.Abs(b-a) >= 18 {
					rotulos = append(rotulos, texto(cent((a+b)/2), cent(y+hh/2+3), numES(v), 8, papelGrafico, "middle"))
				}
				continue
			}
			x, ancla := max(a, b)+3, "start"
			if v < 0 {
				x, ancla = min(a, b)-3, "end"
			}
			rotulos = append(rotulos, texto(cent(x), cent(y+hh/2+3), numES(v), 8, apagado, ancla))
		}
	}
	return append(out, rotulos...)
}

// dispersion es `scatter`.
//
//nolint:gocyclo // es el puerto línea a línea de `scatter`; partirlo lo haría divergir
func dispersion(g Grafico, xy datosXY, rotulos0 []string, w, h float64) []marca {
	var out, rotulos []marca
	f := marcoPara(g, w, h, padDer)
	ys, xs := xy.Series, xy.X
	if len(ys) == 0 {
		nombre := ""
		if len(rotulos0) > 0 {
			nombre = rotulos0[0]
		}
		ys = []serieGrafico{{Nombre: nombre, Valores: xy.X}}
		xs = make([]float64, len(xy.X))
		for i := range xs {
			xs[i] = float64(i)
		}
	}
	var xf, yf []float64
	for _, v := range xs {
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			xf = append(xf, v)
		}
	}
	for _, s := range ys {
		for _, v := range s.Valores {
			if !math.IsNaN(v) && !math.IsInf(v, 0) {
				yf = append(yf, v)
			}
		}
	}
	ex := escalaBonita(0, 1)
	if len(xf) > 0 {
		ex = escalaBonita(slices.Min(xf), slices.Max(xf))
	}
	ey := escalaBonita(0, 1)
	if len(yf) > 0 {
		ey = escalaBonita(min(0, slices.Min(yf)), max(0, slices.Max(yf)))
	}
	xDe := func(v float64) float64 { return proyectar(v, ex.Min, ex.Max, f.l, f.W-f.r) }
	out = titulosEje(g, f, out)
	out = ejeValores(g, f, ey, f.y(ey, ey.Min), out)
	for _, t := range ex.Marcas {
		out = append(out, texto(cent(xDe(t)), cent(f.t+f.alto()+14), numES(t), 9, apagado, "middle"))
	}
	for k, s := range ys[:min(len(ys), len(coloresSerie))] {
		col := colorSerie(g, k)
		for i, v := range s.Valores {
			if i >= len(xs) || math.IsNaN(xs[i]) || math.IsInf(xs[i], 0) || math.IsNaN(v) || math.IsInf(v, 0) {
				continue
			}
			cx, cy := cent(xDe(xs[i])), cent(f.y(ey, v))
			out = append(out, marca{
				K: "circle", CX: cx, CY: cy, R: 2.6, Fill: col, Op: 0.85,
				Title: s.Nombre + ": (" + numES(xs[i]) + ", " + numES(v) + ")",
			})
			if g.Labels {
				rotulos = append(rotulos, texto(cx, cent(cy-5), numES(v), 8, apagado, "middle"))
			}
		}
	}
	return append(out, rotulos...)
}

// radar es `radar`.
func radar(g Grafico, d datosGrafico, w, h float64) []marca {
	var out []marca
	series := d.Series[:min(len(d.Series), len(coloresSerie))]
	n := len(d.Rotulos)
	tope := 1.0
	for _, s := range series {
		for _, v := range s.Valores {
			if !math.IsNaN(v) && !math.IsInf(v, 0) {
				tope = max(tope, v)
			}
		}
	}
	cx, cy := w/2, h/2
	r := min(h-padArr-padAbj, h)/2 - 8
	poli := func(frac func(i int) float64) string {
		var b strings.Builder
		for i := range d.Rotulos {
			x, y := puntoPolar(cx, cy, r, i, n, frac(i))
			if i == 0 {
				b.WriteString("M")
			} else {
				b.WriteString("L")
			}
			b.WriteString(numSVG(cent(x)) + " " + numSVG(cent(y)))
		}
		return b.String() + "Z"
	}
	for _, fr := range []float64{0.5, 1} {
		out = append(out, marca{K: "path", D: poli(func(int) float64 { return fr }), Fill: "none", Stroke: lineaReticula, SW: 1})
	}
	for i := range d.Rotulos {
		x, y := puntoPolar(cx, cy, r, i, n, 1)
		out = append(out, linea(cent(cx), cent(cy), cent(x), cent(y), lineaReticula))
	}
	for k, s := range series {
		col := colorSerie(g, k)
		out = append(out, marca{K: "path", D: poli(func(i int) float64 {
			v := 0.0
			if i < len(s.Valores) {
				v = s.Valores[i]
			}
			return max(0, v) / tope
		}), Fill: col, Op: 0.14, Stroke: col, SW: 1.5, Round: true, Title: s.Nombre})
	}
	for i, t := range d.Rotulos {
		x, y := puntoPolar(cx, cy, r, i, n, 1.12)
		out = append(out, texto(cent(x), cent(y+3), recortarRotulo(t, 7), 8, apagado, "middle"))
	}
	return out
}

// porcion es `PiePart`.
type porcion struct {
	Nombre string
	Valor  float64
	Color  string
}

// porciones es `pieParts`.
func porciones(g Grafico, d datosGrafico, otros string) []porcion {
	if len(d.Series) == 0 {
		return nil
	}
	primera := d.Series[0]
	var pos []porcion
	for i, nombre := range d.Rotulos {
		v := 0.0
		if i < len(primera.Valores) {
			v = max(0, primera.Valores[i])
		}
		if v > 0 {
			pos = append(pos, porcion{Nombre: nombre, Valor: v})
		}
	}
	if len(pos) <= len(coloresSerie) {
		for i := range pos {
			pos[i].Color = colorSerie(g, i)
		}
		return pos
	}
	cabeza := pos[:len(coloresSerie)-1]
	resto := 0.0
	for _, p := range pos[len(coloresSerie)-1:] {
		resto += p.Valor
	}
	out := make([]porcion, 0, len(coloresSerie))
	for i, p := range cabeza {
		p.Color = colorSerie(g, i)
		out = append(out, p)
	}
	return append(out, porcion{Nombre: otros, Valor: resto, Color: colorOtros})
}

// circular es `pie` (y el anillo).
func circular(g Grafico, partes []porcion, w, h float64) []marca {
	var out, rotulos []marca
	total := 0.0
	for _, p := range partes {
		total += p.Valor
	}
	cx, cy := w/2, h/2
	r := h/2 - 12
	anillo := g.Type == "donut"
	a := -math.Pi / 2
	for _, p := range partes {
		frac := p.Valor / total
		a1 := a + float64(frac*2*math.Pi)
		titulo := p.Nombre + ": " + numES(p.Valor) + " (" + numES(frac*100) + " %)"
		if frac > 0.9995 {
			out = append(out, marca{K: "circle", CX: cent(cx), CY: cent(cy), R: cent(r), Fill: p.Color, Stroke: papelGrafico, SW: 2, Title: titulo})
		} else {
			grande := "0"
			if frac > 0.5 {
				grande = "1"
			}
			out = append(out, marca{
				K: "path", Fill: p.Color, Stroke: papelGrafico, SW: 2, Title: titulo,
				D: "M" + numSVG(cent(cx)) + " " + numSVG(cent(cy)) + "L" + numSVG(cent(cx+float64(r*math.Cos(a)))) + " " + numSVG(cent(cy+float64(r*math.Sin(a)))) +
					"A" + numSVG(cent(r)) + " " + numSVG(cent(r)) + " 0 " + grande + " 1 " + numSVG(cent(cx+float64(r*math.Cos(a1)))) + " " + numSVG(cent(cy+float64(r*math.Sin(a1)))) + "Z",
			})
		}
		if g.Labels && frac >= 0.05 {
			medio := (a + a1) / 2
			rr := r * 0.62
			if anillo {
				rr = r * 0.79
			}
			rotulos = append(rotulos, texto(cent(cx+float64(rr*math.Cos(medio))), cent(cy+float64(rr*math.Sin(medio))+3), numES(redondearJS(frac*100))+" %", 8, papelGrafico, "middle"))
		}
		a = a1
	}
	if anillo {
		out = append(out, marca{K: "circle", CX: cent(cx), CY: cent(cy), R: cent(r * 0.58), Fill: papelGrafico})
	}
	return append(out, rotulos...)
}

// verGrafico es `chartView`: la escena, la leyenda y los nombres de las series.
func verGrafico(g Grafico, get func(c, f int) formula.Value, otros string) vistaGrafico {
	r, ok := formula.ParseRangeRef(g.Range)
	var v vistaGrafico
	var d datosGrafico
	if ok {
		r = formula.Rect{C1: min(r.C1, Columnas-1), R1: min(r.R1, Filas-1), C2: min(r.C2, Columnas-1), R2: min(r.R2, Filas-1)}
		d = extraerDatos(r, get, g.ByRow)
	}
	circ := g.Type == "pie" || g.Type == "donut"
	var partes []porcion
	if circ && ok {
		partes = porciones(g, d, otros)
		for _, p := range partes {
			v.Leyenda = append(v.Leyenda, itemLeyenda{Nombre: p.Nombre, Color: p.Color})
		}
	} else {
		for k, s := range d.Series[:min(len(d.Series), len(coloresSerie))] {
			v.Leyenda = append(v.Leyenda, itemLeyenda{Nombre: s.Nombre, Color: colorSerie(g, k)})
		}
	}
	for _, s := range d.Series[:min(len(d.Series), len(coloresSerie))] {
		v.Series = append(v.Series, s.Nombre)
	}
	v.Vacio = !ok || len(d.Rotulos) == 0 || (circ && len(partes) == 0)
	w, h := tamanoEscena(g, leyendaVisible(g, len(v.Leyenda)))
	v.Escena = escena{W: w, H: h}
	if v.Vacio {
		return v
	}
	switch {
	case esCartesiano(g.Type):
		v.Escena.Marcas = cartesiano(g, d, w, h)
	case g.Type == "hbar":
		v.Escena.Marcas = barrasHorizontales(g, d, w, h)
	case g.Type == "scatter":
		v.Escena.Marcas = dispersion(g, extraerXY(r, get, g.ByRow), d.Rotulos, w, h)
	case g.Type == "radar":
		v.Escena.Marcas = radar(g, d, w, h)
	case circ:
		v.Escena.Marcas = circular(g, partes, w, h)
	}
	return v
}
