// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"math"
	"slices"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// # Los minigráficos: `sparklineGeom` y `sparklineSvg`
//
// Se pintan con el MISMO SVG que la web, dentro de la celda anfitriona: el `<svg>` es
// un bloque de `100%` de ancho y 18 de alto arriba del todo de la caja de contenido
// (`px-1`), con `preserveAspectRatio="none"`. Aquí el lienzo se declara ya al ancho
// real (en vez de 84 estirado), que es la misma transformación salvo el grosor del
// trazo, que el navegador ensancha un 5 % y aquí no.

// Las medidas de `format.ts`: el lienzo de 84×18 con 2 de aire arriba y abajo.
const (
	anchoMini float64 = 84
	altoMini  float64 = 18
	aireMini  float64 = 2
)

// Los colores: `SPARK_BLUE` por defecto y `SPARK_RED` para las pérdidas.
const (
	azulMini = "#2563EB"
	rojoMini = "#DC2626"
)

// barraMini es una barra de columnas o de ganancia/pérdida.
type barraMini struct {
	X, Y, W, H float64
	Neg        bool
}

// geometriaMini es `SparkGeom`: los puntos de la línea o las barras, en el lienzo 84×18.
type geometriaMini struct {
	Puntos [][2]float64
	Barras []barraMini
}

// redondear2 es `round2`: al céntimo de píxel.
func redondear2(n float64) float64 { return redondearJS(float64(n*100)) / 100 }

// geometriaMinigrafico es `sparklineGeom`: lo no finito cuenta 0.
func geometriaMinigrafico(valores []float64, tipo string) geometriaMini {
	vals := make([]float64, len(valores))
	for i, v := range valores {
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			vals[i] = v
		}
	}
	n := len(vals)
	dentro := altoMini - aireMini*2
	var out geometriaMini
	if n == 0 {
		return out
	}
	if tipo == "line" {
		lo, hi := slices.Min(vals), slices.Max(vals)
		tramo := hi - lo
		if tramo == 0 {
			tramo = 1
		}
		paso := 0.0
		if n > 1 {
			paso = anchoMini / float64(n-1)
		}
		for i, v := range vals {
			x := anchoMini / 2
			if n > 1 {
				x = float64(i) * paso
			}
			out.Puntos = append(out.Puntos, [2]float64{redondear2(x), redondear2(aireMini + float64((1-(v-lo)/tramo)*dentro))})
		}
		return out
	}
	hueco := anchoMini / float64(n)
	bw := max(1, hueco*0.7)
	if tipo == "winloss" {
		medio, bh := altoMini/2, dentro/2
		for i, v := range vals {
			b := barraMini{X: redondear2(float64(float64(i)*hueco) + (hueco-bw)/2), Y: redondear2(medio - bh), W: redondear2(bw), H: redondear2(bh), Neg: v < 0}
			if v < 0 {
				b.Y = redondear2(medio)
			}
			if v == 0 {
				b.H = 0
			}
			out.Barras = append(out.Barras, b)
		}
		return out
	}
	lo, hi := min(0, slices.Min(vals)), max(0, slices.Max(vals))
	tramo := hi - lo
	if tramo == 0 {
		tramo = 1
	}
	cero := aireMini + float64((hi/tramo)*dentro)
	for i, v := range vals {
		vy := aireMini + float64(((hi-v)/tramo)*dentro)
		out.Barras = append(out.Barras, barraMini{
			X: redondear2(float64(float64(i)*hueco) + (hueco-bw)/2), Y: redondear2(min(vy, cero)),
			W: redondear2(bw), H: redondear2(math.Abs(vy - cero)), Neg: v < 0,
		})
	}
	return out
}

// svgMinigrafico es `sparklineSvg` con el lienzo declarado a `ancho` px de CSS: las x se
// estiran como las estira `preserveAspectRatio="none"`.
func svgMinigrafico(valores []float64, tipo, color string, ancho float64) string {
	g := geometriaMinigrafico(valores, tipo)
	principal := azulMini
	if reHex.MatchString(color) {
		principal = color
	}
	ex := ancho / anchoMini
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` + numSVG(ancho) + ` ` + numSVG(altoMini) +
		`" width="` + numSVG(ancho) + `" height="` + numSVG(altoMini) + `">`)
	if tipo == "line" {
		if len(g.Puntos) > 0 {
			b.WriteString(`<path d="`)
			for i, p := range g.Puntos {
				if i == 0 {
					b.WriteString("M")
				} else {
					b.WriteString("L")
				}
				b.WriteString(numSVG(p[0]*ex) + " " + numSVG(p[1]))
			}
			b.WriteString(`" fill="none" stroke="` + principal + `" stroke-width="1.25" stroke-linecap="round" stroke-linejoin="round"/>`)
		}
	} else {
		for _, br := range g.Barras {
			if br.H <= 0 {
				continue
			}
			col := principal
			if tipo == "winloss" && br.Neg {
				col = rojoMini
			}
			b.WriteString(`<rect x="` + numSVG(br.X*ex) + `" y="` + numSVG(br.Y) + `" width="` + numSVG(br.W*ex) +
				`" height="` + numSVG(br.H) + `" fill="` + col + `"/>`)
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// valoresMinigrafico es `sparkValues`: los números del rango en orden de lectura, con lo
// que no es número como 0, dentro de la rejilla.
func (l *Libro) valoresMinigrafico(rango string) []float64 {
	r, ok := formula.ParseRangeRef(rango)
	if !ok {
		return nil
	}
	var out []float64
	for f := r.R1; f <= min(r.R2, Filas-1); f++ {
		for c := r.C1; c <= min(r.C2, Columnas-1); c++ {
			x, _ := l.Valor(c, f).(float64)
			if math.IsNaN(x) || math.IsInf(x, 0) {
				x = 0
			}
			out = append(out, x)
		}
	}
	return out
}

// Minigraficos son los de la hoja activa.
func (l *Libro) Minigraficos() []Minigrafico { return l.h().minis }

// minigraficosPorCelda es `sparkByCell`: el de cada celda anfitriona (el último gana).
func (l *Libro) minigraficosPorCelda() map[Celda]Minigrafico {
	if len(l.h().minis) == 0 {
		return nil
	}
	out := map[Celda]Minigrafico{}
	for _, m := range l.h().minis {
		if k, ok := celdaDe(m.Cell); ok {
			out[k] = m
		}
	}
	return out
}

// AnadirMinigrafico es `sparkadd`: uno por celda, así que sustituye al que hubiera.
func (l *Libro) AnadirMinigrafico(crudo minigraficoCrudo) bool {
	m, ok := crudo.sanear()
	if !ok {
		return false
	}
	l.anotar()
	h := l.h()
	h.minis = append(slices.DeleteFunc(slices.Clone(h.minis), func(x Minigrafico) bool { return x.Cell == m.Cell }), m)
	l.version++
	return true
}

// QuitarMinigrafico es `sparkdel`: el de esa celda anfitriona.
func (l *Libro) QuitarMinigrafico(celda string) bool {
	h := l.h()
	lista := slices.DeleteFunc(slices.Clone(h.minis), func(x Minigrafico) bool { return x.Cell == celda })
	if len(lista) == len(h.minis) {
		return false
	}
	l.anotar()
	if len(lista) == 0 {
		lista = nil
	}
	l.h().minis = lista
	l.version++
	return true
}

// pintarMinigrafico coloca el de la celda: `width="100%" height="18"` en la caja de
// contenido, arriba del todo.
func (rr *rejillaRenderer) pintarMinigrafico(r *rasterSVG, m Minigrafico, x, y, anchoRelleno, z float32) {
	margen := MargenCelda * z
	ancho := (anchoRelleno - 2*margen) / z
	if ancho <= 0 {
		r.raster.Hide()
		return
	}
	r.poner(svgMinigrafico(rr.r.L.valoresMinigrafico(m.Range), m.Type, m.Color, float64(ancho)))
	r.raster.Move(fyne.NewPos(x+margen, y))
	r.raster.Resize(fyne.NewSize(ancho*z, float32(altoMini)*z))
	r.raster.Show()
}
