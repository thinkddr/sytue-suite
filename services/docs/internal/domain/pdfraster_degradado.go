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
// del sombreado, de ahí al parámetro t del eje (tipos 2 y 3) o al dominio 2D
// (tipo 1), y de ahí a su color.
type degradadoRaster struct {
	inv       matriz // espacio de usuario de la página → espacio del sombreado
	invMatriz matriz // espacio del sombreado → dominio de /Function (tipo 1)
	escala    float64
	alfa      float64
	tipo      int
	c         []float64
	dom       [4]float64
	bbox      [4]float64
	tieneBBox bool
	fondo     color.NRGBA
	ec        *espacioColor
	fn        *funcionPDF
	ext       [2]bool
	tabla     [256]color.NRGBA
}

// degradadoDe prepara el sombreado `sh` con la matriz que lo lleva a la página.
// `conFondo` activa `/Background`, que según §8.7.4.3 solo se aplica en un
// patrón (`PatternType 2`) y no con el operador directo `sh`.
func (l *lienzo) degradadoDe(sh pdfObj, m matriz, conFondo bool) *degradadoRaster {
	dict := dictDeSombreado(sh)
	if dict == nil {
		return nil
	}
	tipo, ok := comoInt(l.d.resolver(dict["ShadingType"]))
	if !ok {
		return nil
	}
	coords := l.reales(dict["Coords"])
	if !sombreadoValido(tipo, coords) {
		return nil
	}
	inv, ok := m.inversa()
	if !ok {
		return nil
	}
	ec := l.espacio(dict["ColorSpace"], 0)
	fn := l.funcion(dict["Function"], 0)
	if fn == nil || ((tipo == 2 || tipo == 3) && len(fn.dominio) > 2) {
		return nil
	}
	escala := 1.0
	if l.r != nil {
		escala = l.r.escala
	}
	g := &degradadoRaster{
		inv: inv, invMatriz: identidad, escala: escala, alfa: 1,
		tipo: tipo, c: coords, ec: ec, fn: fn,
	}
	if bbox, ok := l.rectangulo(dict["BBox"]); ok {
		g.bbox, g.tieneBBox = bbox, true
	}
	if conFondo {
		if bg := l.reales(dict["Background"]); len(bg) > 0 {
			g.fondo = colorRaster(ec.css(bg), 1)
		}
	}
	dom := l.reales(dict["Domain"])
	if tipo == 1 {
		l.configurarDegradadoTipo1(g, dict, dom)
		return g
	}
	if len(dom) < 2 {
		dom = []float64{0, 1}
	}
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

func dictDeSombreado(sh pdfObj) pdfDict {
	switch v := sh.(type) {
	case pdfDict:
		return v
	case *pdfStream:
		return v.Dict
	default:
		return nil
	}
}

func sombreadoValido(tipo int, coords []float64) bool {
	return tipo == 1 || (tipo == 2 && len(coords) >= 4) || (tipo == 3 && len(coords) >= 6)
}

func (l *lienzo) configurarDegradadoTipo1(g *degradadoRaster, dict pdfDict, dom []float64) {
	g.dom = [4]float64{0, 1, 0, 1}
	if len(dom) >= 4 {
		copy(g.dom[:], dom[:4])
	}
	arr, ok := l.d.resolver(dict["Matrix"]).(pdfArray)
	if !ok || len(arr) < 6 {
		return
	}
	var ms matriz
	for i := 0; i < 6; i++ {
		ms[i] = comoReal(l.d.resolver(arr[i]))
	}
	if invMs, ok := ms.inversa(); ok {
		g.invMatriz = invMs
	}
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
	return g.colorEn((float64(x)+0.5)/g.escala, (float64(y)+0.5)/g.escala)
}

func (g *degradadoRaster) colorEn(px, py float64) color.NRGBA {
	u := g.inv[0]*px + g.inv[2]*py + g.inv[4]
	v := g.inv[1]*px + g.inv[3]*py + g.inv[5]
	if g.tieneBBox && (u < g.bbox[0] || u > g.bbox[2] || v < g.bbox[1] || v > g.bbox[3]) {
		return color.NRGBA{}
	}
	if g.tipo == 1 {
		ux := g.invMatriz[0]*u + g.invMatriz[2]*v + g.invMatriz[4]
		vy := g.invMatriz[1]*u + g.invMatriz[3]*v + g.invMatriz[5]
		xMin, xMax := math.Min(g.dom[0], g.dom[1]), math.Max(g.dom[0], g.dom[1])
		yMin, yMax := math.Min(g.dom[2], g.dom[3]), math.Max(g.dom[2], g.dom[3])
		if ux < xMin || ux > xMax || vy < yMin || vy > yMax {
			return g.fondo
		}
		return colorRaster(g.ec.css(g.fn.evaluarVarias([]float64{ux, vy})), g.alfa)
	}
	s, ok := g.parametro(u, v)
	if !ok {
		return g.fondo
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
	k := math.Max(alfa, 0)
	h.alfa = g.alfa * k
	h.fondo.A = uint8(float64(h.fondo.A) * k) //nolint:gosec // G115: A·alfa ≤ 255
	for i, c := range &h.tabla {
		h.tabla[i].A = uint8(float64(c.A) * k) //nolint:gosec // G115: A·alfa ≤ 255
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

// dibujarMalla rasteriza los triángulos de una malla (tipos 4 a 7) interpolando
// con pesos baricéntricos el color de sus tres vértices (Gouraud).
func (r *rasterizador) dibujarMalla(tris []trianguloMalla, m matriz, alfa float64, rc *recorteRaster) {
	area := rc.area(r.pagina)
	if area.Empty() || alfa <= 0 {
		return
	}
	for _, t := range tris {
		r.dibujarTrianguloMalla(t, m, alfa, area, rc)
	}
}

func (r *rasterizador) dibujarTrianguloMalla(
	t trianguloMalla, m matriz, alfa float64, area image.Rectangle, rc *recorteRaster,
) {
	pa := r.punto(m, t.a.x, t.a.y)
	pb := r.punto(m, t.b.x, t.b.y)
	pc := r.punto(m, t.c.x, t.c.y)
	det := (pb.y-pc.y)*(pa.x-pc.x) + (pc.x-pb.x)*(pa.y-pc.y)
	if math.Abs(float64(det)) < 1e-6 {
		return
	}
	invDet := 1 / det
	minX := max(int(math.Floor(float64(min(pa.x, pb.x, pc.x)))), area.Min.X)
	maxX := min(int(math.Ceil(float64(max(pa.x, pb.x, pc.x)))), area.Max.X)
	minY := max(int(math.Floor(float64(min(pa.y, pb.y, pc.y)))), area.Min.Y)
	maxY := min(int(math.Ceil(float64(max(pa.y, pb.y, pc.y)))), area.Max.Y)
	if minX >= maxX || minY >= maxY {
		return
	}
	ca := colorRaster(t.a.css, alfa)
	cb := colorRaster(t.b.css, alfa)
	cc := colorRaster(t.c.css, alfa)
	for y := minY; y < maxY; y++ {
		py := float32(y) + 0.5
		for x := minX; x < maxX; x++ {
			px := float32(x) + 0.5
			w0 := ((pb.y-pc.y)*(px-pc.x) + (pc.x-pb.x)*(py-pc.y)) * invDet
			w1 := ((pc.y-pa.y)*(px-pc.x) + (pa.x-pc.x)*(py-pc.y)) * invDet
			w2 := 1 - w0 - w1
			if w0 < -1e-4 || w1 < -1e-4 || w2 < -1e-4 {
				continue
			}
			cob := r.coberturaRecorte(rc, x, y)
			if cob == 0 {
				continue
			}
			r.mezclarPixelMalla(x, y, w0, w1, w2, ca, cb, cc, cob)
		}
	}
}

func (r *rasterizador) coberturaRecorte(rc *recorteRaster, x, y int) uint32 {
	if rc == nil || rc.mask == nil {
		return 255
	}
	return uint32(rc.mask.AlphaAt(x, y).A)
}

func (r *rasterizador) mezclarPixelMalla(
	x, y int, w0, w1, w2 float32, ca, cb, cc color.NRGBA, cob uint32,
) {
	sa := uint32(canalBaricentrico(w0, w1, w2, ca.A, cb.A, cc.A)) * cob / 255
	if sa == 0 {
		return
	}
	sr := uint32(canalBaricentrico(w0, w1, w2, ca.R, cb.R, cc.R)) * sa / 255
	sg := uint32(canalBaricentrico(w0, w1, w2, ca.G, cb.G, cc.G)) * sa / 255
	sb := uint32(canalBaricentrico(w0, w1, w2, ca.B, cb.B, cc.B)) * sa / 255
	off := r.dst.PixOffset(x, y)
	invA := 255 - sa
	r.dst.Pix[off] = uint8(min(sr+uint32(r.dst.Pix[off])*invA/255, 255))     //nolint:gosec // G115: acotado a 255
	r.dst.Pix[off+1] = uint8(min(sg+uint32(r.dst.Pix[off+1])*invA/255, 255)) //nolint:gosec // G115: acotado a 255
	r.dst.Pix[off+2] = uint8(min(sb+uint32(r.dst.Pix[off+2])*invA/255, 255)) //nolint:gosec // G115: acotado a 255
	r.dst.Pix[off+3] = uint8(min(sa+uint32(r.dst.Pix[off+3])*invA/255, 255)) //nolint:gosec // G115: acotado a 255
}

func canalBaricentrico(w0, w1, w2 float32, a, b, c uint8) uint8 {
	v := float64(w0*float32(a) + w1*float32(b) + w2*float32(c) + 0.5)
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}
