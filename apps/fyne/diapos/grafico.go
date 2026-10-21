// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"fmt"
	"math"
	"strings"

	"git.sytue.eu/sytue/one/apps/fyne/editor/figuras"
)

// El gráfico de una diapositiva: `SlideChartView` de SlidesCanvas.tsx con la lectura de
// la rejilla de `sheets/Chart.tsx` (`extractChartData`, `niceTicks`).

var (
	seriesGrafico = figuras.Series
	marcasBonitas = figuras.Marcas
	numeroGrafico = figuras.Numero
)

const (
	apagadoGrafico = figuras.Apagado
	ejeGrafico     = figuras.Eje
	reticulaGraf   = figuras.Reticula
)

// datosGrafico es `slideChartData` → `extractChartData` sobre la rejilla entera.
func datosGrafico(g *Grafico) ([]string, []figuras.Serie) { return figuras.Datos(g.Rejilla) }

// Grafico compuesto: el SVG de las marcas y los rótulos que van encima. Vacío = el
// gráfico no tiene datos que pintar y el lienzo enseña `slides.chart.noData`.
type graficoCompuesto struct {
	svg     []byte
	rotulos []figuras.Rotulo
	vacio   bool
	letra   float64
}

// componerGrafico es `SlideChartView` para una caja de vw×vh px del escenario.
func componerGrafico(g *Grafico, vw, vh float64) graficoCompuesto {
	rotulos, series := datosGrafico(g)
	letra := max(7, min(11, vh*0.06))
	var tarta []float64
	if g.Tipo == "pie" && len(series) > 0 {
		tarta = series[0].Valores
	}
	hayPositivo := false
	for _, v := range tarta {
		hayPositivo = hayPositivo || v > 0
	}
	if len(rotulos) == 0 || len(series) == 0 || (g.Tipo == "pie" && !hayPositivo) {
		return graficoCompuesto{vacio: true, letra: letra}
	}
	if len(series) > len(seriesGrafico) {
		series = series[:len(seriesGrafico)]
	}
	pad := struct{ l, r, t, b float64 }{l: min(38, vw*0.16), r: 6, t: 8, b: min(20, vh*0.14)}
	if g.Tipo == "pie" {
		pad.l, pad.b = 6, 6
	}
	plotW, plotH := max(1, vw-pad.l-pad.r), max(1, vh-pad.t-pad.b)

	var b strings.Builder
	cabeceraSVG(&b, vw, vh)
	out := graficoCompuesto{letra: letra}
	if g.Tipo == "pie" {
		componerTarta(&b, rotulos, tarta, pad.l+plotW/2, pad.t+plotH/2, max(1, min(plotW, plotH)/2-2))
	} else {
		out.rotulos = componerBarrasLineas(&b, g.Tipo, rotulos, series, vw, vh, pad.l, pad.r, pad.t, pad.b, letra)
	}
	b.WriteString(`</svg>`)
	out.svg = []byte(b.String())
	return out
}

// componerBarrasLineas es `bodyBarLine`: retícula y eje Y bonito, barras o líneas, la
// línea base y los rótulos de categoría.
func componerBarrasLineas(b *strings.Builder, tipo string, rotulos []string, series []figuras.Serie,
	vw, vh, padL, padR, padT, padB, letra float64,
) []figuras.Rotulo {
	plotW, plotH := max(1, vw-padL-padR), max(1, vh-padT-padB)
	var todos []float64
	for _, s := range series {
		todos = append(todos, s.Valores...)
	}
	var esc figuras.Escala
	if tipo == "bar" {
		lo, hi := 0.0, 0.0
		for _, v := range todos {
			lo, hi = min(lo, v), max(hi, v)
		}
		esc = marcasBonitas(lo, hi)
	} else {
		lo, hi := 0.0, 1.0
		if len(todos) > 0 {
			lo, hi = todos[0], todos[0]
			for _, v := range todos {
				lo, hi = min(lo, v), max(hi, v)
			}
		}
		esc = marcasBonitas(lo, hi)
	}
	rango := esc.Max - esc.Min
	if rango == 0 {
		rango = 1
	}
	yDe := func(v float64) float64 { return padT + (esc.Max-v)/rango*plotH }
	n := max(1, len(rotulos))
	hueco := plotW / float64(n)
	paso := int(math.Ceil(float64(n) / max(1, math.Floor(plotW/(letra*4)))))

	var fuera []figuras.Rotulo
	for _, m := range esc.Marcas {
		y := n2(yDe(m))
		fmt.Fprintf(b, `<line x1="%s" x2="%s" y1="%s" y2="%s" stroke="%s" stroke-width="1"/>`,
			n2(padL), n2(vw-padR), y, y, reticulaGraf)
		fuera = append(fuera, figuras.Rotulo{
			Texto: numeroGrafico(m), X: padL - 3, Y: math.Round(yDe(m)*100)/100 + 0.32*letra,
			Tam: letra, Ancla: figuras.AnclaFin, Color: apagadoGrafico,
		})
	}
	if tipo == "bar" {
		grupo := hueco * 0.72
		bw := grupo / float64(max(1, len(series)))
		y0 := yDe(0)
		for k, s := range series {
			for i, v := range s.Valores {
				yv := yDe(v)
				fmt.Fprintf(b, `<rect x="%s" y="%s" width="%s" height="%s" fill="%s"/>`,
					n2(padL+float64(i)*hueco+(hueco-grupo)/2+float64(k)*bw+1), n2(min(yv, y0)),
					n2(max(1, bw-2)), n2(math.Abs(yv-y0)), seriesGrafico[k])
			}
		}
	} else {
		for k, s := range series {
			var d strings.Builder
			for i, v := range s.Valores {
				orden := "L"
				if i == 0 {
					orden = "M"
				}
				fmt.Fprintf(&d, "%s%s %s", orden, n2(padL+hueco*(float64(i)+0.5)), n2(yDe(v)))
			}
			fmt.Fprintf(b, `<path d="%s" fill="none" stroke="%s" stroke-width="2" stroke-linecap="round"`+
				` stroke-linejoin="round"/>`, d.String(), seriesGrafico[k])
		}
	}
	base := padT + plotH
	if tipo == "bar" {
		base = yDe(0)
	}
	fmt.Fprintf(b, `<line x1="%s" x2="%s" y1="%s" y2="%s" stroke="%s" stroke-width="1"/>`,
		n2(padL), n2(vw-padR), n2(base), n2(base), ejeGrafico)
	for i, r := range rotulos {
		if i%paso != 0 || r == "" {
			continue
		}
		if len([]rune(r)) > 8 {
			r = string([]rune(r)[:7]) + "…"
		}
		fuera = append(fuera, figuras.Rotulo{
			Texto: r, X: math.Round((padL+hueco*(float64(i)+0.5))*100) / 100,
			Y: vh - padB/2 + letra*0.3, Tam: letra, Ancla: figuras.AnclaMedio, Color: apagadoGrafico,
		})
	}
	return fuera
}

// componerTarta es `bodyPie`: sectores de la primera serie, los valores > 0.
func componerTarta(b *strings.Builder, rotulos []string, valores []float64, cx, cy, r float64) {
	type parte struct {
		valor float64
		color string
	}
	var partes []parte
	total := 0.0
	for i := range rotulos {
		v := 0.0
		if i < len(valores) {
			v = max(0, valores[i])
		}
		if v > 0 {
			partes = append(partes, parte{valor: v, color: seriesGrafico[i%len(seriesGrafico)]})
			total += v
		}
	}
	if total == 0 {
		total = 1
	}
	a := -math.Pi / 2
	for _, p := range partes {
		frac := p.valor / total
		a1 := a + frac*2*math.Pi
		if frac > 0.9995 {
			fmt.Fprintf(b, `<circle cx="%s" cy="%s" r="%s" fill="%s" stroke="#FFFFFF" stroke-width="1.5"/>`,
				n2(cx), n2(cy), n2(r), p.color)
		} else {
			grande := 0
			if frac > 0.5 {
				grande = 1
			}
			fmt.Fprintf(b, `<path d="M%s %sL%s %sA%s %s 0 %d 1 %s %sZ" fill="%s" stroke="#FFFFFF"`+
				` stroke-width="1.5" stroke-linejoin="round"/>`,
				n2(cx), n2(cy), n2(cx+r*math.Cos(a)), n2(cy+r*math.Sin(a)), n2(r), n2(r), grande,
				n2(cx+r*math.Cos(a1)), n2(cy+r*math.Sin(a1)), p.color)
		}
		a = a1
	}
}
