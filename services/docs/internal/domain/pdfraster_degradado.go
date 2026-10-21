// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Degradados axiales y radiales en la salida PNG (sombreados de tipo 2 y 3,
// §8.7.4.5.3 y §8.7.4.5.4). Antes se pintaba el color medio del sombreado: se
// veía el bloque del tono correcto, pero no el degradado.
package domain

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// degradadoRaster es un sombreado como imagen: cada píxel se lleva al espacio
// del sombreado, de ahí al parámetro t del eje y de t a su color, que sale de
// una tabla de 256 tonos muestreada una vez.
type degradadoRaster struct {
	inv    matriz // espacio de usuario de la página → espacio del sombreado
	escala float64
	tipo   int
	c      []float64
	ext    [2]bool
	tabla  [256]color.NRGBA
}

// degradadoDe prepara el sombreado `sh` con la matriz que lo lleva a la página.
// Solo los tipos 2 y 3; los demás siguen con su color medio.
func (l *lienzo) degradadoDe(sh pdfObj, m matriz) *degradadoRaster {
	var dict pdfDict
	switch v := sh.(type) {
	case pdfDict:
		dict = v
	case *pdfStream:
		dict = v.Dict
	default:
		return nil
	}
	tipo, _ := comoEntero(l.d.resolver(dict["ShadingType"]))
	coords := l.reales(dict["Coords"])
	if (tipo != 2 || len(coords) < 4) && (tipo != 3 || len(coords) < 6) {
		return nil
	}
	inv, ok := m.inversa()
	if !ok {
		return nil
	}
	ec := l.espacio(dict["ColorSpace"], 0)
	fn := l.funcion(dict["Function"], 0)
	dom := l.reales(dict["Domain"])
	if len(dom) < 2 {
		dom = []float64{0, 1}
	}
	g := &degradadoRaster{inv: inv, escala: l.r.escala, tipo: int(tipo), c: coords}
	if ext, ok := l.d.resolver(dict["Extend"]).(pdfArray); ok && len(ext) == 2 {
		g.ext[0], _ = l.d.resolver(ext[0]).(bool)
		g.ext[1], _ = l.d.resolver(ext[1]).(bool)
	}
	for i := range g.tabla {
		t := dom[0] + (dom[1]-dom[0])*float64(i)/255
		g.tabla[i] = colorRaster(ec.css(fn.evaluar(t)), 1)
	}
	return g
}

// inversa de la matriz afín; falso si no tiene (escala cero).
func (m matriz) inversa() (matriz, bool) {
	det := m[0]*m[3] - m[1]*m[2]
	if math.Abs(det) < 1e-12 {
		return matriz{}, false
	}
	return matriz{
		m[3] / det, -m[1] / det, -m[2] / det, m[0] / det,
		(m[2]*m[5] - m[3]*m[4]) / det, (m[1]*m[4] - m[0]*m[5]) / det,
	}, true
}

func (g *degradadoRaster) ColorModel() color.Model { return color.NRGBAModel }

func (g *degradadoRaster) Bounds() image.Rectangle {
	return image.Rect(-1<<20, -1<<20, 1<<20, 1<<20)
}

func (g *degradadoRaster) At(x, y int) color.Color {
	px, py := (float64(x)+0.5)/g.escala, (float64(y)+0.5)/g.escala
	u := g.inv[0]*px + g.inv[2]*py + g.inv[4]
	v := g.inv[1]*px + g.inv[3]*py + g.inv[5]
	s, ok := g.parametro(u, v)
	if !ok {
		return color.NRGBA{}
	}
	return g.tabla[int(s*255+0.5)]
}

// parametro es la posición s ∈ [0, 1] del punto en el eje, con /Extend; falso
// si cae fuera y ese extremo no se extiende.
func (g *degradadoRaster) parametro(x, y float64) (float64, bool) {
	c := g.c
	if g.tipo == 2 {
		dx, dy := c[2]-c[0], c[3]-c[1]
		n := dx*dx + dy*dy
		if n == 0 {
			return 0, false
		}
		return g.extender(((x-c[0])*dx + (y-c[1])*dy) / n)
	}
	// Radial: el s MAYOR con el punto sobre el círculo de centro c0 + s(c1 − c0)
	// y radio r0 + s(r1 − r0) ≥ 0, que es lo que pinta encima.
	cdx, cdy, dr := c[3]-c[0], c[4]-c[1], c[5]-c[2]
	pdx, pdy := x-c[0], y-c[1]
	a := cdx*cdx + cdy*cdy - dr*dr
	b := pdx*cdx + pdy*cdy + c[2]*dr
	cc := pdx*pdx + pdy*pdy - c[2]*c[2]
	var cand []float64
	if math.Abs(a) < 1e-12 {
		if b != 0 {
			cand = []float64{cc / (2 * b)}
		}
	} else if disc := b*b - a*cc; disc >= 0 {
		r := math.Sqrt(disc)
		s1, s2 := (b+r)/a, (b-r)/a
		cand = []float64{max(s1, s2), min(s1, s2)}
	}
	for _, s := range cand {
		if c[2]+s*dr < 0 {
			continue
		}
		if t, ok := g.extender(s); ok {
			return t, true
		}
	}
	return 0, false
}

func (g *degradadoRaster) extender(s float64) (float64, bool) {
	switch {
	case s < 0:
		return 0, g.ext[0]
	case s > 1:
		return 1, g.ext[1]
	}
	return s, true
}

// conAlfa es el mismo degradado con la opacidad del estado gráfico.
func (g *degradadoRaster) conAlfa(alfa float64) *degradadoRaster {
	if alfa >= 1 {
		return g
	}
	h := *g
	for i, c := range &h.tabla {
		h.tabla[i].A = uint8(float64(c.A) * math.Max(alfa, 0)) //nolint:gosec // G115: A·alfa ≤ 255
	}
	return &h
}

// pintarDegradado compone el degradado en la caja con esa máscara (nil = la
// caja entera).
func (r *rasterizador) pintarDegradado(g *degradadoRaster, caja image.Rectangle, m image.Image) {
	if caja.Empty() {
		return
	}
	draw.DrawMask(r.dst, caja, g, caja.Min, m, caja.Min, draw.Over)
}
