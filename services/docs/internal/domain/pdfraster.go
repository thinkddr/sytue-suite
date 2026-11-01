// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// SALIDA RASTERIZADA de una página, que es UNA MÁS y no sustituye a nada.
//
// El render de la casa dibuja SVG con `<text>` (ADR-0033) y eso NO cambia: es lo
// que hace que en la web el texto se seleccione, se busque y lo lea un lector de
// pantalla, y es la salida por defecto de `op: render`. Este fichero añade la
// segunda salida, `formato=png`, para el cliente que NO tiene un motor de SVG con
// texto detrás: en Fyne todo el SVG pasa por `oksvg`, que no implementa `<text>`,
// así que el escritorio enseñaba el papel y las rayas de una factura SIN UNA SOLA
// CIFRA. Una imagen no se selecciona, pero se ve; y para un visor nativo eso es la
// diferencia entre existir y no existir.
//
// POR QUÉ AHORA SÍ SE PUEDE, si el ADR lo descartó. El ADR descartó escribir un
// motor de gráficos Y uno de fuentes —decenas de miles de líneas— cuando la
// alternativa (el navegador) ya estaba ahí. Lo que hay aquí no es eso: el
// intérprete del flujo de contenido, los espacios de color, las fuentes, las
// imágenes, los patrones y las mallas YA ESTÁN ESCRITOS y se reutilizan tal cual.
// Esto solo cambia el destino de los mismos operadores: donde `pintar` escribía un
// `<path>`, aquí se rellena un mapa de bits con `golang.org/x/image/vector`, que
// ya es dependencia del módulo. Los contornos de los glifos los da
// `x/image/font/sfnt` sobre la MISMA tipografía incrustada que el SVG reemite.
//
// LO QUE ESTA SALIDA NO HACE, y la de SVG sí (dicho aquí para que no se descubra
// tarde): los modos de fusión no separables de CSS. La regla par-impar (`f*`/`W*`),
// los recortes por contorno de texto (`Tr` 4-7), las mallas de sombreado y los
// mosaicos (`PatternType 1`, coloreados y no coloreados) sí se rasterizan aquí.
package domain

import (
	"errors"
	"image"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/draw"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"

	"git.sytue.eu/sytue/one/platform/pkg/pdffuente"
)

// maxPixelesRaster acota el mapa de bits de UNA página. Son 4 bytes por píxel
// mientras se dibuja, así que el tope es también el tope de memoria: 16 Mpx =
// 64 MiB, que es un A4 a ×5,6. Sin él, `escala=16` sobre un A0 pide 2 GiB.
const maxPixelesRaster = 16 << 20

// errPaginaDemasiadoGrande se devuelve ANTES de reservar nada.
var errPaginaDemasiadoGrande = errors.New("la página pedida no cabe en el tope de píxeles")

// --- Recorte -----------------------------------------------------------------

// recorteRaster es el área donde se puede pintar. El caso corriente —un `re W n`
// que acota una caja— se guarda como rectángulo y no cuesta memoria; solo un
// recorte de forma libre paga una máscara.
//
// Es INMUTABLE: vive en el estado gráfico, que se copia con `q`/`Q`, así que el
// anidamiento de recortes sale gratis de la pila de PDF.
type recorteRaster struct {
	rect image.Rectangle
	mask *image.Alpha // nil = el rectángulo es todo el recorte
}

func (rc *recorteRaster) area(pagina image.Rectangle) image.Rectangle {
	if rc == nil {
		return pagina
	}
	return rc.rect.Intersect(pagina)
}

// --- Rasterizador -------------------------------------------------------------

// punto es un punto YA en píxeles del destino.
type punto struct{ x, y float32 }

// rasterizador es el destino de una página. Uno por página: no es concurrente.
type rasterizador struct {
	dst    *image.RGBA
	escala float64
	pagina image.Rectangle

	// tmp es el búfer de la máscara de trabajo, REUTILIZADO: cada relleno escribe
	// su caja entera con `draw.Src`, así que no hace falta limpiarlo, y una página
	// con miles de caminos no reserva miles de máscaras.
	tmp []uint8
	z   vector.Rasterizer
	buf sfnt.Buffer
}

func nuevoRasterizador(anchoPt, altoPt, escala float64) (*rasterizador, error) {
	w := int(math.Ceil(anchoPt * escala))
	h := int(math.Ceil(altoPt * escala))
	if w <= 0 || h <= 0 {
		return nil, errors.New("la página no tiene tamaño")
	}
	if w*h > maxPixelesRaster {
		return nil, errPaginaDemasiadoGrande
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	// El papel es BLANCO, como el fondo que el SVG declara: un PDF no trae fondo y
	// un PNG transparente se vería negro sobre cualquier visor de tema oscuro.
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	return &rasterizador{
		dst:    dst,
		escala: escala,
		pagina: dst.Bounds(),
	}, nil
}

// punto lleva un punto del espacio de usuario a píxeles.
func (r *rasterizador) punto(m matriz, x, y float64) punto {
	return punto{
		x: float32((m[0]*x + m[2]*y + m[4]) * r.escala),
		y: float32((m[1]*x + m[3]*y + m[5]) * r.escala),
	}
}

// factor es cuánto agranda la matriz una longitud, que es lo que convierte un
// grosor de línea del espacio de usuario en píxeles. Con una matriz que estira
// distinto en X y en Y no hay un número único: la raíz del determinante es la
// media geométrica, que es lo que hace cualquier rasterizador.
func (r *rasterizador) factor(m matriz) float64 {
	det := math.Abs(m[0]*m[3] - m[1]*m[2])
	return math.Sqrt(det) * r.escala
}

// --- Caminos ------------------------------------------------------------------

// segCam es un trozo del camino en construcción, en espacio de USUARIO. Se guarda
// en paralelo al `d` del SVG en vez de volver a parsearlo: es la misma
// información y el parseo de una cadena que acabamos de escribir es trabajo
// tirado.
type segCam struct {
	op byte // 'M', 'L', 'C' o 'Z'
	p  [6]float64
}

// aplanar transforma el camino a píxeles y convierte las curvas en tramos. El
// número de tramos sale del tamaño EN PANTALLA de la curva: una curva de dos
// píxeles no necesita treinta trozos y una de mil sí.
func (r *rasterizador) aplanar(cam []segCam, m matriz) [][]punto {
	var subs [][]punto
	var act []punto
	cerrar := func() {
		if len(act) > 1 {
			subs = append(subs, act)
		}
		act = nil
	}
	for _, s := range cam {
		switch s.op {
		case 'M':
			cerrar()
			act = []punto{r.punto(m, s.p[0], s.p[1])}
		case 'L':
			act = append(act, r.punto(m, s.p[0], s.p[1]))
		case 'C':
			if len(act) == 0 {
				act = []punto{r.punto(m, s.p[0], s.p[1])}
			}
			p1 := r.punto(m, s.p[0], s.p[1])
			p2 := r.punto(m, s.p[2], s.p[3])
			p3 := r.punto(m, s.p[4], s.p[5])
			act = append(act, aplanarCubica(act[len(act)-1], p1, p2, p3)...)
		case 'Z':
			if len(act) > 1 {
				act = append(act, act[0])
				subs = append(subs, act)
				act = []punto{act[0]}
			}
		}
	}
	cerrar()
	return subs
}

// maxTramosCurva acota la subdivisión: más allá no se distingue y una página con
// miles de curvas se vuelve lenta sin que nadie lo vea.
const maxTramosCurva = 48

func aplanarCubica(p0, p1, p2, p3 punto) []punto {
	largo := distancia(p0, p1) + distancia(p1, p2) + distancia(p2, p3)
	n := int(largo / 3)
	if n < 1 {
		n = 1
	}
	if n > maxTramosCurva {
		n = maxTramosCurva
	}
	out := make([]punto, 0, n)
	for i := 1; i <= n; i++ {
		t := float32(i) / float32(n)
		u := 1 - t
		out = append(out, punto{
			x: u*u*u*p0.x + 3*u*u*t*p1.x + 3*u*t*t*p2.x + t*t*t*p3.x,
			y: u*u*u*p0.y + 3*u*u*t*p1.y + 3*u*t*t*p2.y + t*t*t*p3.y,
		})
	}
	return out
}

func distancia(a, b punto) float32 {
	return float32(math.Hypot(float64(b.x-a.x), float64(b.y-a.y)))
}

// caja es el rectángulo entero que cubre los puntos, acotado al recorte y a la
// página. Rellenar solo ahí es lo que hace que una firma de 2 cm en un A4 cueste
// lo que ocupa y no lo que ocupa la página.
func (r *rasterizador) caja(subs [][]punto, rc *recorteRaster) image.Rectangle {
	if len(subs) == 0 {
		return image.Rectangle{}
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, s := range subs {
		for _, p := range s {
			minX = math.Min(minX, float64(p.x))
			minY = math.Min(minY, float64(p.y))
			maxX = math.Max(maxX, float64(p.x))
			maxY = math.Max(maxY, float64(p.y))
		}
	}
	if math.IsInf(minX, 1) {
		return image.Rectangle{}
	}
	caja := image.Rect(
		int(math.Floor(minX))-1, int(math.Floor(minY))-1,
		int(math.Ceil(maxX))+1, int(math.Ceil(maxY))+1,
	)
	return caja.Intersect(rc.area(r.pagina))
}

// mascara rasteriza los subcaminos en la caja dada y devuelve la cobertura.
//
// LA REGLA DE RELLENO ES SIEMPRE NO-CERO: `vector` acumula área con signo y
// satura en 1, que es exactamente no-cero, y no tiene par-impar. Un `f*` con
// contornos internos del mismo sentido saldrá relleno donde el SVG deja hueco.
// Se releva el día que alguien traiga un rasterizador con las dos reglas; hasta
// entonces, el hueco que falta se ve, y el SVG —que es lo que usa la web— está
// bien.
func (r *rasterizador) mascara(subs [][]punto, caja image.Rectangle) *image.Alpha {
	r.z.Reset(caja.Dx(), caja.Dy())
	r.z.DrawOp = draw.Src // Reset lo deja en Over, y aquí se ESCRIBE la cobertura
	ox, oy := float32(caja.Min.X), float32(caja.Min.Y)
	for _, s := range subs {
		if len(s) < 2 {
			continue
		}
		r.z.MoveTo(s[0].x-ox, s[0].y-oy)
		for _, p := range s[1:] {
			r.z.LineTo(p.x-ox, p.y-oy)
		}
		r.z.ClosePath()
	}
	// La máscara es CONTIGUA (stride = ancho de la caja) y no una subimagen de un
	// búfer del tamaño de la página: cuando la caja empieza en (0,0), `vector`
	// toma un camino rápido que copia fila tras fila SIN mirar el stride, y con
	// una subimagen las filas salían cizalladas —medido: una banda lateral de
	// portada se pintaba en su primer quinto y rayada—.
	n := caja.Dx() * caja.Dy()
	if cap(r.tmp) < n {
		r.tmp = make([]uint8, n)
	}
	m := &image.Alpha{Pix: r.tmp[:n], Stride: caja.Dx(), Rect: caja}
	r.z.Draw(m, caja, image.Opaque, image.Point{})
	return m
}

func (r *rasterizador) mascaraRegla(subs [][]punto, caja image.Rectangle, parImpar bool) *image.Alpha {
	if parImpar {
		return r.mascaraParImpar(subs, caja)
	}
	return r.mascara(subs, caja)
}

type aristaParImpar struct {
	y0, y1 float32
	x0     float32
	invDy  float32
}

// mascaraParImpar rasteriza los subcaminos con la regla par-impar (`f*`, `B*`,
// `b*`, `W*`) mediante cobertura analítica horizontal sobre 8 sublíneas por fila.
func (r *rasterizador) mascaraParImpar(subs [][]punto, caja image.Rectangle) *image.Alpha {
	w, h := caja.Dx(), caja.Dy()
	n := w * h
	if cap(r.tmp) < n {
		r.tmp = make([]uint8, n)
	}
	pix := r.tmp[:n]
	clear(pix)
	aristas := recogerAristasParImpar(subs, float32(caja.Min.Y), float32(caja.Max.Y))
	if len(aristas) == 0 {
		return &image.Alpha{Pix: pix, Stride: w, Rect: caja}
	}
	fila := make([]uint16, w)
	xs := make([]float32, 0, 32)
	activas := make([]aristaParImpar, 0, len(aristas))
	ox := float32(caja.Min.X)
	for y := caja.Min.Y; y < caja.Max.Y; y++ {
		yMin, yMax := float32(y), float32(y+1)
		activas = activas[:0]
		for _, a := range aristas {
			if a.y1 > yMin && a.y0 < yMax {
				activas = append(activas, a)
			}
		}
		if len(activas) == 0 {
			continue
		}
		clear(fila)
		for sub := 0; sub < 8; sub++ {
			ys := yMin + (float32(sub)+0.5)*0.125
			xs = interseccionesParImpar(activas, ys, xs[:0])
			acumularTramosParImpar(fila, xs, ox, w)
		}
		off := (y - caja.Min.Y) * w
		for ix, acc := range fila {
			pix[off+ix] = uint8(min((acc+4)>>3, 255)) //nolint:gosec // G115: acotado a 255
		}
	}
	return &image.Alpha{Pix: pix, Stride: w, Rect: caja}
}

func recogerAristasParImpar(subs [][]punto, yCajaMin, yCajaMax float32) []aristaParImpar {
	var out []aristaParImpar
	for _, s := range subs {
		if len(s) < 2 {
			continue
		}
		for i := range s {
			a, b := s[i], s[(i+1)%len(s)]
			if a.y > b.y {
				a, b = b, a
			}
			dy := b.y - a.y
			if dy < 1e-6 || b.y <= yCajaMin || a.y >= yCajaMax {
				continue
			}
			out = append(out, aristaParImpar{
				y0:    a.y,
				y1:    b.y,
				x0:    a.x,
				invDy: (b.x - a.x) / dy,
			})
		}
	}
	return out
}

func interseccionesParImpar(aristas []aristaParImpar, ys float32, xs []float32) []float32 {
	for _, a := range aristas {
		if ys >= a.y0 && ys < a.y1 {
			xs = append(xs, a.x0+(ys-a.y0)*a.invDy)
		}
	}
	slices.Sort(xs)
	return xs
}

func acumularTramosParImpar(fila []uint16, xs []float32, ox float32, w int) {
	wf := float32(w)
	for k := 0; k+1 < len(xs); k += 2 {
		xL := max(xs[k]-ox, 0)
		xR := min(xs[k+1]-ox, wf)
		if xR <= xL {
			continue
		}
		i0, i1 := int(xL), int(xR)
		if i0 == i1 {
			if i0 >= 0 && i0 < w {
				fila[i0] += uint16((xR-xL)*255 + 0.5)
			}
			continue
		}
		if i0 >= 0 && i0 < w {
			fila[i0] += uint16((float32(i0+1)-xL)*255 + 0.5)
		}
		for ix := max(i0+1, 0); ix < min(i1, w); ix++ {
			fila[ix] += 255
		}
		if i1 >= 0 && i1 < w && xR > float32(i1) {
			fila[i1] += uint16((xR-float32(i1))*255 + 0.5)
		}
	}
}

// rellenar pinta los subcaminos con un color liso y regla no-cero.
func (r *rasterizador) rellenar(subs [][]punto, c color.NRGBA, rc *recorteRaster) {
	r.rellenarRegla(subs, c, rc, false)
}

func (r *rasterizador) rellenarRegla(subs [][]punto, c color.NRGBA, rc *recorteRaster, parImpar bool) {
	if c.A == 0 || len(subs) == 0 {
		return
	}
	caja := r.caja(subs, rc)
	if caja.Empty() {
		return
	}
	m := r.mascaraRegla(subs, caja, parImpar)
	if rc != nil && rc.mask != nil {
		multiplicarMascara(m, rc.mask)
	}
	draw.DrawMask(r.dst, caja, image.NewUniform(c), image.Point{}, m, caja.Min, draw.Over)
}

// ensancharFranjaFina ajusta al píxel entero las reglas rectangulares alineadas
// con los ejes cuyo espesor cae por debajo de 1 px (como los fondos de franjas
// `-0.12 re f*` o filetes de tabla), igual que hace el ajuste de trazos finos
// de un visor de PDF. No se aplica a contornos de glifos.
func ensancharFranjaFina(subs [][]punto) [][]punto {
	if len(subs) != 1 {
		return subs
	}
	s := subs[0]
	if len(s) != 4 && len(s) != 5 {
		return subs
	}
	for i := 0; i < 4; i++ {
		a, b := s[i], s[(i+1)%4]
		if math.Abs(float64(a.x-b.x)) > 0.02 && math.Abs(float64(a.y-b.y)) > 0.02 {
			return subs
		}
	}
	minX, maxX := s[0].x, s[0].x
	minY, maxY := s[0].y, s[0].y
	for _, p := range s {
		minX, maxX = min(minX, p.x), max(maxX, p.x)
		minY, maxY = min(minY, p.y), max(maxY, p.y)
	}
	dx, dy := maxX-minX, maxY-minY
	switch {
	case dy > 0 && dy < 1.0 && dx >= 2.0:
		py := float32(math.Floor(float64(minY+maxY) * 0.5))
		return [][]punto{{{minX, py}, {maxX, py}, {maxX, py + 1}, {minX, py + 1}, {minX, py}}}
	case dx > 0 && dx < 1.0 && dy >= 2.0:
		px := float32(math.Floor(float64(minX+maxX) * 0.5))
		return [][]punto{{{px, minY}, {px + 1, minY}, {px + 1, maxY}, {px, maxY}, {px, minY}}}
	default:
		return subs
	}
}

// multiplicarMascara interseca dos coberturas. Es cómo se acumula un recorte
// sobre otro sin recalcular la intersección de las figuras.
func multiplicarMascara(dst, rec *image.Alpha) {
	b := dst.Bounds().Intersect(rec.Bounds())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		i := dst.PixOffset(b.Min.X, y)
		j := rec.PixOffset(b.Min.X, y)
		for x := 0; x < b.Dx(); x++ {
			dst.Pix[i+x] = uint8(uint32(dst.Pix[i+x]) * uint32(rec.Pix[j+x]) / 255) //nolint:gosec // G115: a·b/255 ≤ 255 con a, b ≤ 255
		}
	}
	// Lo que quede del destino fuera del recorte no se toca: la caja del relleno
	// ya venía intersecada con el rectángulo del recorte.
}

// nuevoRecorte interseca el recorte vigente con un camino.
func (r *rasterizador) nuevoRecorte(subs [][]punto, previo *recorteRaster, parImpar bool) *recorteRaster {
	caja := r.caja(subs, previo)
	if caja.Empty() {
		return &recorteRaster{rect: image.Rectangle{}}
	}
	if previo == nil || previo.mask == nil {
		if rect, ok := rectanguloDe(subs); ok {
			// El caso corriente: `re W n`. Un rectángulo alineado no necesita
			// máscara, y una página de texto trae cientos.
			return &recorteRaster{rect: rect.Intersect(caja)}
		}
	}
	m := image.NewAlpha(caja)
	copiarAlfa(m, r.mascaraRegla(subs, caja, parImpar))
	if previo != nil && previo.mask != nil {
		multiplicarMascara(m, previo.mask)
	}
	return &recorteRaster{rect: caja, mask: m}
}

func copiarAlfa(dst, src *image.Alpha) {
	b := dst.Bounds().Intersect(src.Bounds())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		i := dst.PixOffset(b.Min.X, y)
		j := src.PixOffset(b.Min.X, y)
		copy(dst.Pix[i:i+b.Dx()], src.Pix[j:j+b.Dx()])
	}
}

// rectanguloDe reconoce un camino que es UN rectángulo alineado con los ejes,
// que es lo que produce `re` sin rotación. Devuelve su caja en píxeles enteros.
func rectanguloDe(subs [][]punto) (image.Rectangle, bool) {
	if len(subs) != 1 {
		return image.Rectangle{}, false
	}
	s := subs[0]
	// Cuatro esquinas, con o sin el punto de cierre repetido.
	if len(s) != 4 && len(s) != 5 {
		return image.Rectangle{}, false
	}
	for i := 0; i < 4; i++ {
		a, b := s[i], s[(i+1)%4]
		if math.Abs(float64(a.x-b.x)) > 0.01 && math.Abs(float64(a.y-b.y)) > 0.01 {
			return image.Rectangle{}, false // una diagonal: no es un rectángulo
		}
	}
	minX, minY := float64(s[0].x), float64(s[0].y)
	maxX, maxY := minX, minY
	for _, p := range s {
		minX, maxX = math.Min(minX, float64(p.x)), math.Max(maxX, float64(p.x))
		minY, maxY = math.Min(minY, float64(p.y)), math.Max(maxY, float64(p.y))
	}
	return image.Rect(
		int(math.Round(minX)), int(math.Round(minY)),
		int(math.Round(maxX)), int(math.Round(maxY)),
	), true
}

// recorteDeCaja interseca el recorte con una /BBox ya transformada. Es lo que
// impide que un formulario —una apariencia de anotación, un sello— pinte fuera
// de su caja.
func (r *rasterizador) recorteDeCaja(v [4]float64, m matriz, previo *recorteRaster) *recorteRaster {
	x0, y0 := math.Min(v[0], v[2]), math.Min(v[1], v[3])
	x1, y1 := math.Max(v[0], v[2]), math.Max(v[1], v[3])
	subs := [][]punto{{
		r.punto(m, x0, y0), r.punto(m, x1, y0), r.punto(m, x1, y1), r.punto(m, x0, y1),
	}}
	return r.nuevoRecorte(subs, previo, false)
}

// --- Trazo --------------------------------------------------------------------

// trazar dibuja el contorno. No hay «rasterizador de líneas»: cada tramo se
// convierte en un polígono y se rellenan todos JUNTOS y con el MISMO sentido de
// giro, que es lo que hace que los solapes de dos tramos consecutivos no se
// resten entre sí con la regla no-cero.
func (r *rasterizador) trazar(subs [][]punto, e *ejecucion) {
	factor := r.factor(e.estado.ctm)
	w := float32(e.estado.grosor * factor)
	if e.estado.grosor <= 0 || w < 1 {
		// Grosor 0 en PDF es «lo más fino que el dispositivo sepa pintar» (§8.4.3.2),
		// y por debajo de un píxel una tabla se queda sin bordes.
		w = 1
	}
	if g := guionesEnPixeles(e.estado.guiones, factor); len(g) > 0 {
		subs = trocearConGuiones(subs, g, float32(e.estado.faseGuion*factor))
	}
	polis := make([][]punto, 0, len(subs))
	for _, s := range subs {
		polis = append(polis, poligonosDeTramo(s, w, e.estado.tapa, e.estado.union, e.estado.limite)...)
	}
	r.pintarConPatron(e, polis, false, e.estado.patTrazo, e.estado.trazo, e.estado.alfaTrazo)
}

// poligonosDeTramo convierte UNA polilínea en los polígonos que la dibujan: un
// rectángulo por tramo, una unión por vértice interior y un remate por punta.
func poligonosDeTramo(s []punto, w float32, tapa, union int, limite float64) [][]punto {
	hw := w / 2
	if len(s) == 0 {
		return nil
	}
	if tramoDegenerado(s) {
		return rematePuntoDegenerado(s[0], hw, tapa)
	}
	var out [][]punto
	cerrado := len(s) > 2 && s[0] == s[len(s)-1]
	for i := 0; i+1 < len(s); i++ {
		a, b := s[i], s[i+1]
		if a == b {
			continue
		}
		dx, dy := unitario(b.x-a.x, b.y-a.y)
		if tapa == 2 && !cerrado && (i == 0 || i+2 == len(s)) {
			// Remate cuadrado: el tramo se alarga medio grosor por su punta.
			if i == 0 {
				a = punto{a.x - dx*hw, a.y - dy*hw}
			}
			if i+2 == len(s) {
				b = punto{b.x + dx*hw, b.y + dy*hw}
			}
		}
		nx, ny := -dy*hw, dx*hw
		out = append(out, orientar([]punto{
			{a.x + nx, a.y + ny},
			{b.x + nx, b.y + ny},
			{b.x - nx, b.y - ny},
			{a.x - nx, a.y - ny},
		}))
	}
	out = append(out, uniones(s, hw, union, limite, cerrado)...)
	if tapa == 1 && !cerrado {
		out = append(out, circulo(s[0], hw), circulo(s[len(s)-1], hw))
	}
	return out
}

// uniones cierra los vértices. Sin esto, un marco grueso sale con una muesca en
// cada esquina: los dos rectángulos se tocan en la línea central y por fuera
// dejan una cuña vacía.
func uniones(s []punto, hw float32, union int, limite float64, cerrado bool) [][]punto {
	var out [][]punto
	for i := 1; i+1 < len(s); i++ {
		out = append(out, unionEn(s[i-1], s[i], s[i+1], hw, union, limite)...)
	}
	if cerrado && len(s) > 3 {
		out = append(out, unionEn(s[len(s)-2], s[0], s[1], hw, union, limite)...)
	}
	return out
}

func unionEn(p, v, q punto, hw float32, union int, limite float64) [][]punto {
	if union == 1 {
		return [][]punto{circulo(v, hw)}
	}
	d1x, d1y := unitario(v.x-p.x, v.y-p.y)
	d2x, d2y := unitario(q.x-v.x, q.y-v.y)
	cruz := d1x*d2y - d1y*d2x
	if math.Abs(float64(cruz)) < 1e-6 {
		return nil // recto: no hay esquina que rellenar
	}
	lado := float32(1)
	if cruz > 0 {
		lado = -1
	}
	n1x, n1y := -d1y*hw*lado, d1x*hw*lado
	n2x, n2y := -d2y*hw*lado, d2x*hw*lado
	a := punto{v.x + n1x, v.y + n1y}
	b := punto{v.x + n2x, v.y + n2y}
	if union == 0 {
		sx, sy := n1x+n2x, n1y+n2y
		if l2 := sx*sx + sy*sy; l2 > 1e-9 {
			// Punta en inglete: la longitud sale de 2/|n1+n2| veces el medio grosor.
			razon := float64(2 * hw / float32(math.Sqrt(float64(l2))))
			if limite <= 0 || razon <= limite {
				m := punto{v.x + sx*2*hw*hw/l2, v.y + sy*2*hw*hw/l2}
				return [][]punto{orientar([]punto{v, a, m, b})}
			}
		}
	}
	return [][]punto{orientar([]punto{v, a, b})} // bisel
}

func unitario(x, y float32) (float32, float32) {
	l := float32(math.Hypot(float64(x), float64(y)))
	if l == 0 {
		return 0, 0
	}
	return x / l, y / l
}

// circulo aproxima un disco. Los lados salen del radio: a grosor fino ocho lados
// no se distinguen de una circunferencia y cuestan la mitad.
func circulo(c punto, r float32) []punto {
	n := int(r * 2)
	if n < 8 {
		n = 8
	}
	if n > 32 {
		n = 32
	}
	out := make([]punto, 0, n+1)
	for i := 0; i <= n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		out = append(out, punto{c.x + r*float32(math.Cos(a)), c.y + r*float32(math.Sin(a))})
	}
	return orientar(out)
}

// orientar deja el polígono girando SIEMPRE en el mismo sentido. Es el detalle
// que hace correcta la unión de figuras solapadas con la regla no-cero: dos
// polígonos de sentidos opuestos se anularían justo donde se pisan, y una línea
// quebrada aparecería agujereada en cada vértice.
func orientar(p []punto) []punto {
	var area float64
	for i := range p {
		j := (i + 1) % len(p)
		area += float64(p[i].x*p[j].y - p[j].x*p[i].y)
	}
	if area < 0 {
		for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
			p[i], p[j] = p[j], p[i]
		}
	}
	return p
}

// guionesEnPixeles pasa el `stroke-dasharray` ya calculado para el SVG a píxeles.
func guionesEnPixeles(csv string, factor float64) []float32 {
	if csv == "" {
		return nil
	}
	var out []float32
	total := float32(0)
	for _, parte := range strings.Split(csv, ",") {
		v, err := strconv.ParseFloat(strings.TrimSpace(parte), 64)
		if err != nil {
			return nil
		}
		out = append(out, float32(v*factor))
		total += float32(v * factor)
	}
	if total <= 0.01 {
		return nil
	}
	if len(out)%2 == 1 {
		out = append(out, out...) // un patrón impar se repite para alternar bien
	}
	return out
}

func tramoDegenerado(s []punto) bool {
	p0 := s[0]
	for _, p := range s[1:] {
		if distancia(p0, p) > 1e-4 {
			return false
		}
	}
	return true
}

func rematePuntoDegenerado(p punto, hw float32, tapa int) [][]punto {
	switch tapa {
	case 1:
		return [][]punto{circulo(p, hw)}
	case 2:
		return [][]punto{orientar([]punto{
			{p.x - hw, p.y - hw},
			{p.x + hw, p.y - hw},
			{p.x + hw, p.y + hw},
			{p.x - hw, p.y + hw},
		})}
	default:
		return nil
	}
}

// trocearConGuiones parte las polilíneas en los trozos que sí se pintan,
// respetando la fase inicial y los guiones de longitud 0 (`[0 gap] d` + `1 J`).
func trocearConGuiones(subs [][]punto, patron []float32, fase float32) [][]punto {
	idxIni, restoIni, pintaIni := estadoInicialGuion(patron, fase)
	var out [][]punto
	for _, s := range subs {
		if len(s) > 0 {
			out = append(out, trocearPolilineaConGuiones(s, patron, idxIni, restoIni, pintaIni)...)
		}
	}
	return out
}

func trocearPolilineaConGuiones(
	s []punto, patron []float32, idx int, resto float32, pinta bool,
) [][]punto {
	var out [][]punto
	var act []punto
	if pinta {
		act = []punto{s[0]}
	}
	for i := 0; i+1 < len(s); i++ {
		a, b := s[i], s[i+1]
		largo := distancia(a, b)
		pasosCero := 0
		for largo > resto || (resto <= 0 && pasosCero < len(patron)) {
			corte, nuevoA, nuevoLargo := avanzarCorteGuion(a, b, largo, resto)
			if resto <= 0 {
				pasosCero++
			} else {
				a, largo, pasosCero = nuevoA, nuevoLargo, 0
			}
			if pinta {
				out = append(out, append(act, corte))
				act = nil
			} else {
				act = []punto{corte}
			}
			pinta = !pinta
			idx = (idx + 1) % len(patron)
			resto = max(patron[idx], 0)
		}
		resto -= largo
		if pinta {
			act = append(act, b)
		}
	}
	if len(act) > 1 {
		out = append(out, act)
	} else if len(act) == 1 && resto <= 0 {
		out = append(out, []punto{act[0], act[0]})
	}
	return out
}

func avanzarCorteGuion(a, b punto, largo, resto float32) (punto, punto, float32) {
	if resto <= 0 {
		return a, a, largo
	}
	t := resto / largo
	corte := punto{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t}
	return corte, corte, largo - resto
}

func estadoInicialGuion(patron []float32, fase float32) (int, float32, bool) {
	var periodo float32
	for _, x := range patron {
		if x > 0 {
			periodo += x
		}
	}
	if periodo <= 0 {
		return 0, patron[0], true
	}
	f := float32(math.Mod(float64(fase), float64(periodo)))
	if f < 0 {
		f += periodo
	}
	idx, pinta := 0, true
	for pasos := 0; f > 0 && pasos < len(patron)*2; pasos++ {
		d := max(patron[idx], 0)
		if f < d {
			return idx, d - f, pinta
		}
		f -= d
		pinta = !pinta
		idx = (idx + 1) % len(patron)
	}
	return idx, max(patron[idx], 0), pinta
}

// --- Pintado de caminos ---------------------------------------------------------

// pintarRaster es lo que `pintar` hace en el mapa de bits: el MISMO camino, el
// MISMO estado gráfico y el mismo orden, con otro destino.
func (e *ejecucion) pintarRaster(hab, relleno, trazo, parImpar bool, pend int) {
	if !hab {
		return
	}
	r := e.l.r
	subs := r.aplanar(e.cam, e.estado.ctm)
	if len(subs) == 0 {
		return
	}
	if relleno {
		subsRelleno := subs
		if !trazo {
			subsRelleno = ensancharFranjaFina(subs)
		}
		r.pintarConPatron(e, subsRelleno, parImpar, e.estado.patRelleno, e.estado.relleno, e.estado.alfaRelleno)
	}
	if trazo {
		r.trazar(subs, e)
	}
	if pend != 0 {
		// El recorte entra en el estado gráfico vigente y lo deshace `Q`, igual que
		// en PDF. En el SVG eso mismo es un `<g clip-path>` que se cierra al salir
		// del nivel.
		e.estado.recorte = r.nuevoRecorte(subs, e.estado.recorte, pend == 2)
	}
}

func (r *rasterizador) pintarConPatron(
	e *ejecucion, subs [][]punto, parImpar bool, pat *patronPDF, css string, alfa float64,
) {
	switch {
	case pat != nil && pat.tipo == 1:
		r.rellenarConMosaico(e, subs, parImpar, pat, alfa)
	case pat != nil && len(pat.malla) > 0:
		r.dibujarMalla(pat.malla, pat.mp, alfa, r.nuevoRecorte(subs, e.estado.recorte, parImpar))
	case pat != nil && pat.deg != nil:
		r.rellenarConDegradadoRegla(subs, pat.deg.conAlfa(alfa), e.estado.recorte, parImpar)
	default:
		if c, ok := colorDePintura(css, pat, alfa); ok {
			r.rellenarRegla(subs, c, e.estado.recorte, parImpar)
		}
	}
}

func colorDePintura(css string, pat *patronPDF, alfa float64) (color.NRGBA, bool) {
	if pat != nil {
		if pat.aprox == "" {
			return color.NRGBA{}, false
		}
		return colorRaster(pat.aprox, alfa), true
	}
	c := colorRaster(css, alfa)
	return c, c.A != 0
}

// rellenarConMosaico pinta un patrón de tipo 1 en PNG: directamente celda por
// celda cuando caben pocas repeticiones (preserva degradados y fuentes anidadas
// con nitidez vectorial) y muestreando una celda periódica transparente cuando
// el rayado es denso.
func (r *rasterizador) rellenarConMosaico(
	e *ejecucion, subs [][]punto, parImpar bool, pat *patronPDF, alfa float64,
) {
	if pat == nil || pat.st == nil || alfa <= 0 || len(subs) == 0 || e.l.prof > maxProfundidadXObject {
		return
	}
	caja := r.caja(subs, e.estado.recorte)
	if caja.Empty() {
		return
	}
	inv, ok := pat.mp.inversa()
	if !ok {
		return
	}
	contenido, err := e.l.d.datosDeFlujo(pat.st)
	if err != nil {
		return
	}
	mCopia := image.NewAlpha(caja)
	copiarAlfa(mCopia, r.mascaraRegla(subs, caja, parImpar))
	if e.estado.recorte != nil && e.estado.recorte.mask != nil {
		multiplicarMascara(mCopia, e.estado.recorte.mask)
	}
	if alfa < 1 {
		escalarMascara(mCopia, uint8(acotar01(alfa)*255))
	}

	kxMin, kxMax, kyMin, kyMax := rangoCeldasMosaico(caja, r.escala, inv, pat.bbox, pat.ancho, pat.alto)
	nx, ny := kxMax-kxMin+1, kyMax-kyMin+1
	if nx <= 0 || ny <= 0 {
		return
	}
	recursos, _ := e.l.d.resolver(pat.st.Dict["Resources"]).(pdfDict)
	if nx <= 16 && ny <= 16 && nx*ny <= 256 {
		r.reproducirCeldasMosaico(e, contenido, recursos, pat, &recorteRaster{rect: caja, mask: mCopia}, kxMin, kxMax, kyMin, kyMax)
		return
	}
	r.muestrearCeldaMosaico(e, contenido, recursos, pat, caja, mCopia, inv)
}

func escalarMascara(m *image.Alpha, factor uint8) {
	f := uint32(factor)
	for i, a := range m.Pix {
		m.Pix[i] = uint8((uint32(a)*f + 127) / 255) //nolint:gosec // G115: a·f/255 ≤ 255
	}
}

func rangoCeldasMosaico(
	caja image.Rectangle, escala float64, inv matriz, bbox [4]float64, dx, dy float64,
) (int, int, int, int) {
	x0, y0 := float64(caja.Min.X)/escala, float64(caja.Min.Y)/escala
	x1, y1 := float64(caja.Max.X)/escala, float64(caja.Max.Y)/escala
	esquinas := [4][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
	uMin, vMin := math.Inf(1), math.Inf(1)
	uMax, vMax := math.Inf(-1), math.Inf(-1)
	for _, pt := range esquinas {
		u := inv[0]*pt[0] + inv[2]*pt[1] + inv[4]
		v := inv[1]*pt[0] + inv[3]*pt[1] + inv[5]
		uMin, uMax = math.Min(uMin, u), math.Max(uMax, u)
		vMin, vMax = math.Min(vMin, v), math.Max(vMax, v)
	}
	kxMin := int(math.Floor((uMin - bbox[2]) / dx))
	kxMax := int(math.Ceil((uMax - bbox[0]) / dx))
	kyMin := int(math.Floor((vMin - bbox[3]) / dy))
	kyMax := int(math.Ceil((vMax - bbox[1]) / dy))
	return kxMin, kxMax, kyMin, kyMax
}

func (r *rasterizador) reproducirCeldasMosaico(
	e *ejecucion, contenido []byte, recursos pdfDict, pat *patronPDF,
	recForma *recorteRaster, kxMin, kxMax, kyMin, kyMax int,
) {
	prevSolo := e.l.soloRaster
	e.l.soloRaster = true
	defer func() { e.l.soloRaster = prevSolo }()

	ignorar := pat.paintType == 2
	for ky := kyMin; ky <= kyMax; ky++ {
		for kx := kxMin; kx <= kxMax; kx++ {
			ctmCelda := matriz{1, 0, 0, 1, float64(kx) * pat.ancho, float64(ky) * pat.alto}.por(pat.mp)
			recCelda := r.recorteDeCaja(pat.bbox, ctmCelda, recForma)
			if recCelda.area(r.pagina).Empty() {
				continue
			}
			est := estadoInicial(ctmCelda)
			est.recorte = recCelda
			if ignorar {
				est.relleno, est.trazo = pat.colorBase, pat.colorBase
			}
			e.l.prof++
			e.l.ejecutarModo(contenido, recursos, est, ignorar)
			e.l.prof--
		}
	}
}

func (r *rasterizador) muestrearCeldaMosaico(
	e *ejecucion, contenido []byte, recursos pdfDict, pat *patronPDF,
	caja image.Rectangle, mCopia *image.Alpha, inv matriz,
) {
	sx := math.Hypot(pat.mp[0], pat.mp[1]) * r.escala
	sy := math.Hypot(pat.mp[2], pat.mp[3]) * r.escala
	wC := min(max(int(math.Ceil(pat.ancho*max(sx*2, 4))), 4), 512)
	hC := min(max(int(math.Ceil(pat.alto*max(sy*2, 4))), 4), 512)

	dstCelda := image.NewRGBA(image.Rect(0, 0, wC, hC))
	rCelda := &rasterizador{dst: dstCelda, escala: 1, pagina: dstCelda.Bounds()}
	sub := &lienzo{
		d:           e.l.d,
		opts:        e.l.opts,
		fuentes:     e.l.fuentes,
		caraCSS:     e.l.caraCSS,
		ocgApagados: e.l.ocgApagados,
		prof:        e.l.prof + 1,
		r:           rCelda,
		soloRaster:  true,
	}
	escX, escY := float64(wC)/pat.ancho, float64(hC)/pat.alto
	mCelda := matriz{escX, 0, 0, escY, -pat.bbox[0] * escX, -pat.bbox[1] * escY}
	ox := min(max(int(math.Ceil((pat.bbox[2]-pat.bbox[0])/pat.ancho))-1, 0), 4)
	oy := min(max(int(math.Ceil((pat.bbox[3]-pat.bbox[1])/pat.alto))-1, 0), 4)
	ignorar := pat.paintType == 2
	for jy := -oy; jy <= 0; jy++ {
		for jx := -ox; jx <= 0; jx++ {
			ctmJ := matriz{1, 0, 0, 1, float64(jx) * pat.ancho, float64(jy) * pat.alto}.por(mCelda)
			est := estadoInicial(ctmJ)
			est.recorte = rCelda.recorteDeCaja(pat.bbox, ctmJ, nil)
			if ignorar {
				est.relleno, est.trazo = pat.colorBase, pat.colorBase
			}
			sub.ejecutarModo(contenido, recursos, est, ignorar)
		}
	}
	r.aplicarMuestrasCelda(dstCelda, wC, hC, pat, caja, mCopia, inv)
}

func (r *rasterizador) aplicarMuestrasCelda(
	celda *image.RGBA, wC, hC int, pat *patronPDF,
	caja image.Rectangle, mCopia *image.Alpha, inv matriz,
) {
	bx0, by0, dx, dy := pat.bbox[0], pat.bbox[1], pat.ancho, pat.alto
	for y := caja.Min.Y; y < caja.Max.Y; y++ {
		py := (float64(y) + 0.5) / r.escala
		mRow := mCopia.PixOffset(caja.Min.X, y)
		dRow := r.dst.PixOffset(caja.Min.X, y)
		for ix := 0; ix < caja.Dx(); ix++ {
			cob := uint32(mCopia.Pix[mRow+ix])
			if cob == 0 {
				continue
			}
			px := (float64(caja.Min.X+ix) + 0.5) / r.escala
			uu := math.Mod(inv[0]*px+inv[2]*py+inv[4]-bx0, dx)
			if uu < 0 {
				uu += dx
			}
			vv := math.Mod(inv[1]*px+inv[3]*py+inv[5]-by0, dy)
			if vv < 0 {
				vv += dy
			}
			cx := min(int(uu/dx*float64(wC)), wC-1)
			cy := min(int(vv/dy*float64(hC)), hC-1)
			cOff := celda.PixOffset(cx, cy)
			sa := (uint32(celda.Pix[cOff+3])*cob + 127) / 255
			if sa == 0 {
				continue
			}
			sr := (uint32(celda.Pix[cOff])*cob + 127) / 255
			sg := (uint32(celda.Pix[cOff+1])*cob + 127) / 255
			sb := (uint32(celda.Pix[cOff+2])*cob + 127) / 255
			invA := 255 - sa
			o := dRow + ix*4
			r.dst.Pix[o] = uint8(min(sr+(uint32(r.dst.Pix[o])*invA+127)/255, 255))     //nolint:gosec // G115: acotado a 255
			r.dst.Pix[o+1] = uint8(min(sg+(uint32(r.dst.Pix[o+1])*invA+127)/255, 255)) //nolint:gosec // G115: acotado a 255
			r.dst.Pix[o+2] = uint8(min(sb+(uint32(r.dst.Pix[o+2])*invA+127)/255, 255)) //nolint:gosec // G115: acotado a 255
			r.dst.Pix[o+3] = uint8(min(sa+(uint32(r.dst.Pix[o+3])*invA+127)/255, 255)) //nolint:gosec // G115: acotado a 255
		}
	}
}

// rellenarArea pinta todo el recorte vigente, que es lo que pide el operador
// `sh`: un sombreado se extiende por el área de recorte entera.
func (r *rasterizador) rellenarArea(rc *recorteRaster, c color.NRGBA) {
	area := rc.area(r.pagina)
	if area.Empty() || c.A == 0 {
		return
	}
	if rc != nil && rc.mask != nil {
		draw.DrawMask(r.dst, area, image.NewUniform(c), image.Point{}, rc.mask, area.Min, draw.Over)
		return
	}
	draw.Draw(r.dst, area, image.NewUniform(c), image.Point{}, draw.Over)
}

// rellenarAreaCon es rellenarArea con un degradado en vez de un color.
func (r *rasterizador) rellenarAreaCon(rc *recorteRaster, g *degradadoRaster) {
	area := rc.area(r.pagina)
	if rc != nil && rc.mask != nil {
		r.pintarDegradado(g, area, rc.mask)
		return
	}
	r.pintarDegradado(g, area, nil)
}

// rellenarConDegradadoRegla es rellenar con un degradado en vez de un color.
func (r *rasterizador) rellenarConDegradadoRegla(subs [][]punto, g *degradadoRaster, rc *recorteRaster, parImpar bool) {
	caja := r.caja(subs, rc)
	if caja.Empty() {
		return
	}
	m := r.mascaraRegla(subs, caja, parImpar)
	if rc != nil && rc.mask != nil {
		multiplicarMascara(m, rc.mask)
	}
	r.pintarDegradado(g, caja, m)
}

// --- Texto --------------------------------------------------------------------

// dibujarTexto pinta los contornos de los glifos. Es el motivo entero de este
// fichero: lo que `oksvg` no sabe hacer y por lo que una factura salía sin cifras.
func (e *ejecucion) dibujarTexto(f *fuentePDF, glifos []glifo) {
	r := e.l.r
	if r == nil || e.estado.modoRender == 3 || e.estado.modoRender == 7 || e.estado.tamano == 0 {
		return
	}
	cara, upem := f.caraSfnt()
	if cara == nil || upem <= 0 {
		return
	}
	trm := matriz{e.estado.escalaH, 0, 0, 1, 0, e.estado.subida}.por(e.tm).por(e.estado.ctm)
	relleno := colorRaster(e.estado.relleno, e.estado.alfaRelleno)
	desplazamiento := 0.0
	for _, g := range glifos {
		dx, dy := desplazamiento, 0.0
		if f.vertical {
			dx = -g.vx / 1000 * e.estado.tamano
			dy = desplazamiento - g.vy/1000*e.estado.tamano
		}
		if f.reserva && esZapfDingbatsNombre(f.nombreBase) {
			e.pintarGlifoReservaExtendida(f, g, trm, dx, dy, relleno)
		} else if gid, ok := f.indiceDeGlifo(cara, &r.buf, g); ok {
			if subs := r.contorno(cara, gid, upem, trm, dx, dy, e.estado.tamano); len(subs) > 0 {
				e.pintarGlifo(subs, relleno)
			}
		} else if f.reserva {
			e.pintarGlifoReservaExtendida(f, g, trm, dx, dy, relleno)
		}
		paso := g.ancho
		if f.vertical {
			paso = g.alto
		}
		a := paso/1000*e.estado.tamano + e.estado.espCar
		if g.esEspacioSimple {
			a += e.estado.espPal
		}
		desplazamiento += a
	}
}

// pintarGlifo aplica el modo de render del texto (§9.3.6) al contorno ya
// transformado. El recorte de los modos 4-7 se acumula en `acumularRecorteTexto`
// y se aplica al cerrar `ET`.
func (e *ejecucion) pintarGlifo(subs [][]punto, relleno color.NRGBA) {
	r := e.l.r
	rellenar := func() {
		if e.estado.patRelleno != nil {
			r.pintarConPatron(e, subs, false, e.estado.patRelleno, e.estado.relleno, e.estado.alfaRelleno)
			return
		}
		r.rellenar(subs, relleno, e.estado.recorte)
	}
	switch e.estado.modoRender {
	case 1, 5:
		r.trazar(subs, e)
	case 2, 6:
		rellenar()
		r.trazar(subs, e)
	default:
		rellenar()
	}
}

func (e *ejecucion) reiniciarRecorteTexto() {
	e.hayClipTexto = false
	e.clipTextoSVG.Reset()
	e.clipTextoPts = nil
}

// acumularRecorteTexto añade los contornos de los glifos mostrados con `Tr` 4-7
// para intersectarlos con el recorte vigente al ejecutar `ET` (§9.3.6).
func (e *ejecucion) acumularRecorteTexto(f *fuentePDF, glifos []glifo) {
	cara, upem := f.caraSfnt()
	if cara == nil || upem <= 0 {
		return
	}
	e.hayClipTexto = true
	trm := matriz{e.estado.escalaH, 0, 0, 1, 0, e.estado.subida}.por(e.tm).por(e.estado.ctm)
	var bufLocal sfnt.Buffer
	buf := &bufLocal
	if e.l.r != nil {
		buf = &e.l.r.buf
	}
	desplazamiento := 0.0
	for _, g := range glifos {
		dx, dy := desplazamiento, 0.0
		if f.vertical {
			dx = -g.vx / 1000 * e.estado.tamano
			dy = desplazamiento - g.vy/1000*e.estado.tamano
		}
		caraGlifo, upemGlifo, gid, ok := resolverContornoRecorte(f, cara, upem, buf, g)
		if ok {
			if e.l.r != nil {
				e.clipTextoPts = append(e.clipTextoPts, e.l.r.contorno(caraGlifo, gid, upemGlifo, trm, dx, dy, e.estado.tamano)...)
			}
			if !e.l.soloRaster {
				escribirContornoSVG(&e.clipTextoSVG, caraGlifo, buf, gid, upemGlifo, trm, dx, dy, e.estado.tamano)
			}
		}
		paso := g.ancho
		if f.vertical {
			paso = g.alto
		}
		a := paso/1000*e.estado.tamano + e.estado.espCar
		if g.esEspacioSimple {
			a += e.estado.espPal
		}
		desplazamiento += a
	}
}

func resolverContornoRecorte(
	f *fuentePDF, cara *sfnt.Font, upem float64, buf *sfnt.Buffer, g glifo,
) (*sfnt.Font, float64, sfnt.GlyphIndex, bool) {
	if gid, ok := f.indiceDeGlifo(cara, buf, g); ok {
		return cara, upem, gid, true
	}
	if !f.reserva {
		return nil, 0, 0, false
	}
	uni := primeraRuna(g.texto)
	if uni <= ' ' {
		return nil, 0, 0, false
	}
	pila := pdffuente.Pila(f.negrita, f.italica)
	for i := len(pila) - 1; i >= 0; i-- {
		caraExt := pila[i].SFNT()
		if caraExt == nil {
			continue
		}
		if gid, err := caraExt.GlyphIndex(buf, uni); err == nil && gid != 0 {
			return caraExt, float64(caraExt.UnitsPerEm()), gid, true
		}
	}
	return nil, 0, 0, false
}

func escribirContornoSVG(
	b *strings.Builder, cara *sfnt.Font, buf *sfnt.Buffer, gid sfnt.GlyphIndex,
	upem float64, trm matriz, dx, dy, tamano float64,
) {
	segs, err := cara.LoadGlyph(buf, gid, fixed.Int26_6(int32(upem)<<6), nil)
	if err != nil || len(segs) == 0 {
		return
	}
	k := tamano / upem / 64
	pt := func(fx, fy fixed.Int26_6) (float64, float64) {
		lx, ly := dx+float64(fx)*k, dy-float64(fy)*k
		return trm[0]*lx + trm[2]*ly + trm[4], trm[1]*lx + trm[3]*ly + trm[5]
	}
	abierto := false
	for _, s := range segs {
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			if abierto {
				b.WriteByte('Z')
			}
			x0, y0 := pt(s.Args[0].X, s.Args[0].Y)
			b.WriteString("M" + num(x0) + " " + num(y0))
			abierto = true
		case sfnt.SegmentOpLineTo:
			x0, y0 := pt(s.Args[0].X, s.Args[0].Y)
			b.WriteString("L" + num(x0) + " " + num(y0))
		case sfnt.SegmentOpQuadTo:
			x0, y0 := pt(s.Args[0].X, s.Args[0].Y)
			x1, y1 := pt(s.Args[1].X, s.Args[1].Y)
			b.WriteString("Q" + num(x0) + " " + num(y0) + " " + num(x1) + " " + num(y1))
		case sfnt.SegmentOpCubeTo:
			x0, y0 := pt(s.Args[0].X, s.Args[0].Y)
			x1, y1 := pt(s.Args[1].X, s.Args[1].Y)
			x2, y2 := pt(s.Args[2].X, s.Args[2].Y)
			b.WriteString("C" + num(x0) + " " + num(y0) + " " + num(x1) + " " + num(y1) + " " + num(x2) + " " + num(y2))
		}
	}
	if abierto {
		b.WriteByte('Z')
	}
}

func (e *ejecucion) aplicarRecorteTexto() {
	if !e.hayClipTexto {
		return
	}
	dClip := e.clipTextoSVG.String()
	pts := e.clipTextoPts
	e.reiniciarRecorteTexto()

	if e.l.r != nil {
		e.estado.recorte = e.l.r.nuevoRecorte(pts, e.estado.recorte, false)
	}
	if !e.l.soloRaster {
		id := e.l.id("rec")
		if dClip == "" {
			e.l.defsEscribir(`<clipPath id="` + id + `"/>`)
		} else {
			e.l.defsEscribir(`<clipPath id="` + id + `"><path d="` + dClip + `"/></clipPath>`)
		}
		e.abrirGrupo(`<g clip-path="url(#` + id + `)">`)
	}
}

// contorno saca los segmentos del glifo y los lleva a píxeles. `sfnt` los
// devuelve con la Y hacia ABAJO y el espacio de glifo de PDF la tiene hacia
// arriba: sin ese volteo, el texto sale reflejado.
func (r *rasterizador) contorno(
	cara *sfnt.Font, gid sfnt.GlyphIndex, upem float64, trm matriz, dx, dy, tamano float64,
) [][]punto {
	segs, err := cara.LoadGlyph(&r.buf, gid, fixed.Int26_6(int32(upem)<<6), nil)
	if err != nil || len(segs) == 0 {
		return nil
	}
	k := tamano / upem / 64
	local := func(x, y fixed.Int26_6) (float64, float64) {
		return dx + float64(x)*k, dy - float64(y)*k
	}
	var subs [][]punto
	var act []punto
	for _, s := range segs {
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			if len(act) > 1 {
				subs = append(subs, act)
			}
			x, y := local(s.Args[0].X, s.Args[0].Y)
			act = []punto{r.punto(trm, x, y)}
		case sfnt.SegmentOpLineTo:
			x, y := local(s.Args[0].X, s.Args[0].Y)
			act = append(act, r.punto(trm, x, y))
		case sfnt.SegmentOpQuadTo:
			act = r.cuadratica(act, trm, local, s)
		case sfnt.SegmentOpCubeTo:
			if len(act) == 0 {
				continue
			}
			x1, y1 := local(s.Args[0].X, s.Args[0].Y)
			x2, y2 := local(s.Args[1].X, s.Args[1].Y)
			x3, y3 := local(s.Args[2].X, s.Args[2].Y)
			act = append(act, aplanarCubica(act[len(act)-1],
				r.punto(trm, x1, y1), r.punto(trm, x2, y2), r.punto(trm, x3, y3))...)
		}
	}
	if len(act) > 1 {
		subs = append(subs, act)
	}
	return subs
}

// cuadratica eleva la curva de segundo grado a una de tercero, que es exacto, en
// vez de escribir un segundo aplanador.
func (r *rasterizador) cuadratica(
	act []punto, trm matriz, local func(x, y fixed.Int26_6) (float64, float64), s sfnt.Segment,
) []punto {
	if len(act) == 0 {
		return act
	}
	cx, cy := local(s.Args[0].X, s.Args[0].Y)
	fx, fy := local(s.Args[1].X, s.Args[1].Y)
	p0 := act[len(act)-1]
	c := r.punto(trm, cx, cy)
	p3 := r.punto(trm, fx, fy)
	p1 := punto{p0.x + 2.0/3.0*(c.x-p0.x), p0.y + 2.0/3.0*(c.y-p0.y)}
	p2 := punto{p3.x + 2.0/3.0*(c.x-p3.x), p3.y + 2.0/3.0*(c.y-p3.y)}
	return append(act, aplanarCubica(p0, p1, p2, p3)...)
}

// --- Tipografías ---------------------------------------------------------------

// caraSfnt devuelve la tipografía con la que dibujar, parseada una sola vez por
// documento. Es LA MISMA que el SVG incrusta en su `@font-face`, cmap
// reconstruida incluida: un Identity-H de InDesign se dibuja con su tipo, no con
// un sustituto.
func (f *fuentePDF) caraSfnt() (*sfnt.Font, float64) {
	if !f.caraLista {
		f.caraLista = true
		f.cargarCaraSfnt()
	}
	if f.cara == nil {
		return nil, 0
	}
	return f.cara, float64(f.cara.UnitsPerEm())
}

func (f *fuentePDF) cargarCaraSfnt() {
	if len(f.programa) > 0 {
		if f.crudo == nil && !f.yaEnvueltaCFF && !f.dosBytes {
			if limpia, ok := normalizarSfnt(f.programa, f.nombreBase); ok {
				f.crudo = leerTablasCrudasSfnt(limpia)
			}
		}
		cara, err := sfnt.Parse(f.programa)
		if err != nil {
			saneada := f.repararProgramaParaSfnt(f.programa)
			cara, err = sfnt.Parse(saneada)
			if err == nil {
				f.programa = saneada
			}
		}
		if err == nil {
			f.cara = cara
			f.cmapUnicode = tieneCmapUnicode(f.programa)
			f.cmapMacRoman = tieneCmapMacRoman(f.programa)
			return
		}
	}
	// Sin tipografía utilizable —no incrustada, Type1 antiguo, CFF suelto—
	// se dibuja con la de reserva y CADA GLIFO EN SU SITIO: el avance lo
	// manda el PDF, así que la línea no se descuadra aunque la letra sea
	// otra. Es la misma degradación que el SVG hace con `familiaSustituta`.
	f.cara, f.reserva = caraDeReserva(f.negrita, f.italica, f.familia), true
}

func (f *fuentePDF) repararProgramaParaSfnt(programa []byte) []byte {
	saneada := f.completarMetadatosTTF(programa)
	if !f.dosBytes {
		if p, ok := f.cmapSimpleParaTexto(saneada); ok {
			return p
		}
		return saneada
	}
	if p, inv, ok := f.reescribirCmapCompuesta(saneada, 0); ok {
		f.inversoCompuesto = inv
		return p
	}
	return saneada
}

// indiceDeGlifo resuelve qué contorno dibujar para un código del PDF.
func (f *fuentePDF) indiceDeGlifo(cara *sfnt.Font, buf *sfnt.Buffer, g glifo) (sfnt.GlyphIndex, bool) {
	if !f.reserva {
		if f.dosBytes {
			return f.indiceGlifoCompuesto(cara, g.codigo)
		}
		if gid, ok := f.indiceGlifoSimpleDirecto(cara, g); ok {
			return gid, true
		}
	}
	// Fuente simple (§9.6.6.4) o de reserva.
	uni, codigo := primeraRuna(g.texto), rune(g.codigo&0xFF)
	orden := [3]rune{uni, 0xF000 + codigo, codigo}
	if !f.cmapUnicode && !f.cmapMacRoman {
		orden = [3]rune{0xF000 + codigo, codigo, uni}
	}
	if f.simbolica {
		orden = [3]rune{0xF000 + codigo, codigo}
	}
	if f.reserva {
		orden = [3]rune{uni}
	}
	for _, r := range orden {
		if r == 0 {
			continue
		}
		if x, err := cara.GlyphIndex(buf, r); err == nil && x != 0 {
			return x, true
		}
	}
	return 0, false
}

func (f *fuentePDF) indiceGlifoCompuesto(cara *sfnt.Font, codigo uint32) (sfnt.GlyphIndex, bool) {
	// Identity-H: el código ES el índice de glifo, salvo que /CIDToGIDMap traiga
	// una tabla que diga otra cosa.
	gid, ok := f.gidDeCID(codigo)
	if !ok || int(gid) >= cara.NumGlyphs() {
		return 0, false
	}
	return sfnt.GlyphIndex(gid), true
}

func (f *fuentePDF) indiceGlifoSimpleDirecto(cara *sfnt.Font, g glifo) (sfnt.GlyphIndex, bool) {
	c := byte(g.codigo & 0xFF)
	if gid := f.simpleGid[c]; gid != 0 && int(gid) < cara.NumGlyphs() {
		return sfnt.GlyphIndex(gid), true
	}
	if f.crudo != nil {
		if gid, ok := f.resolverGlifoSimple(f.crudo, g); ok && gid != 0 && int(gid) < cara.NumGlyphs() {
			return sfnt.GlyphIndex(gid), true
		}
	}
	return 0, false
}

// tieneCmapUnicode dice si la tipografía trae una subtabla `cmap` indexada por
// Unicode —(0,*), (3,1) o (3,10)— o solo las que se indexan por código.
func tieneCmapUnicode(b []byte) bool {
	return hayCmap(b, func(pid, psid int) bool {
		return pid == 0 || (pid == 3 && (psid == 1 || psid == 10))
	})
}

// tieneCmapMacRoman reconoce la CMap Macintosh Roman (1,0). Aunque no sea
// Unicode, sfnt la consulta por carácter y no por el código interno del PDF.
// Un subconjunto de Word puede asignar el código 0x50 a «e»: buscar primero
// 0x50 como 'P' dibujaría una letra válida, pero equivocada.
func tieneCmapMacRoman(b []byte) bool {
	return hayCmap(b, func(pid, psid int) bool { return pid == 1 && psid == 0 })
}

func hayCmap(b []byte, coincide func(pid, psid int) bool) bool {
	be := func(i int) int {
		if i+2 > len(b) {
			return 0
		}
		return int(b[i])<<8 | int(b[i+1])
	}
	for i := 0; i < be(4) && 12+16*i+12 <= len(b); i++ {
		e := 12 + 16*i
		if string(b[e:e+4]) != "cmap" {
			continue
		}
		off := be(e+8)<<16 | be(e+10)
		for j := 0; j < be(off+2); j++ {
			pid, psid := be(off+4+8*j), be(off+6+8*j)
			if coincide(pid, psid) {
				return true
			}
		}
	}
	return false
}

func primeraRuna(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

// Las cuatro caras de reserva se parsean una vez por proceso: son las Go Fonts,
// que ya vienen con `x/image` y no añaden dependencia ninguna.
var (
	unaVezReserva sync.Once
	reservaNormal *sfnt.Font
	reservaBold   *sfnt.Font
	reservaItal   *sfnt.Font
	reservaBoldIt *sfnt.Font
	reservaMono   *sfnt.Font
)

func caraDeReserva(negrita, italica bool, familia string) *sfnt.Font {
	unaVezReserva.Do(func() {
		reservaNormal, _ = sfnt.Parse(goregular.TTF)
		reservaBold, _ = sfnt.Parse(gobold.TTF)
		reservaItal, _ = sfnt.Parse(goitalic.TTF)
		reservaBoldIt, _ = sfnt.Parse(gobolditalic.TTF)
		reservaMono, _ = sfnt.Parse(gomono.TTF)
	})
	switch {
	case strings.Contains(familia, "mono"):
		return reservaMono
	case negrita && italica:
		return reservaBoldIt
	case negrita:
		return reservaBold
	case italica:
		return reservaItal
	}
	return reservaNormal
}

// pintarGlifoReservaExtendida cubre en la salida PNG los caracteres que Go Font
// no trae (griego de Symbol, cirílico, operadores matemáticos y ZapfDingbats),
// apoyándose primero en Liberation Sans (`pdffuente.Pila`) y después en trazos
// geométricos para los símbolos de Dingbats.
func (e *ejecucion) pintarGlifoReservaExtendida(
	f *fuentePDF, g glifo, trm matriz, dx, dy float64, relleno color.NRGBA,
) {
	uni := primeraRuna(g.texto)
	if uni <= ' ' {
		return
	}
	r := e.l.r
	if esZapfDingbatsNombre(f.nombreBase) {
		if subs := r.contornoDingbatReserva(uni, trm, dx, dy, e.estado.tamano); len(subs) > 0 {
			e.pintarGlifo(subs, relleno)
			return
		}
	}
	pila := pdffuente.Pila(f.negrita, f.italica)
	for i := len(pila) - 1; i >= 0; i-- {
		caraExt := pila[i].SFNT()
		if caraExt == nil {
			continue
		}
		if gid, err := caraExt.GlyphIndex(&r.buf, uni); err == nil && gid != 0 {
			if subs := r.contorno(caraExt, gid, float64(caraExt.UnitsPerEm()), trm, dx, dy, e.estado.tamano); len(subs) > 0 {
				e.pintarGlifo(subs, relleno)
				return
			}
		}
	}
	if subs := r.contornoDingbatReserva(uni, trm, dx, dy, e.estado.tamano); len(subs) > 0 {
		e.pintarGlifo(subs, relleno)
	}
}

func esZapfDingbatsNombre(nombre string) bool {
	n := strings.ToLower(nombre)
	return strings.Contains(n, "zapf") || strings.Contains(n, "dingbat")
}

func (r *rasterizador) contornoDingbatReserva(uni rune, trm matriz, dx, dy, tamano float64) [][]punto {
	pt := func(u, v float64) punto {
		return r.punto(trm, dx+u*tamano, dy+v*tamano)
	}
	if forma, ok := dingbatFijo(uni, pt); ok {
		return forma
	}
	if esDiscoDingbat(uni) {
		return [][]punto{discoDingbat(pt)}
	}
	if esEstrellaDingbat(uni) {
		return [][]punto{estrellaDingbat(pt)}
	}
	if uni >= 0x2701 && uni <= 0x27BF {
		return [][]punto{{pt(0.14, 0.16), pt(0.66, 0.16), pt(0.66, 0.62), pt(0.14, 0.62)}}
	}
	return nil
}

func dingbatFijo(uni rune, pt func(u, v float64) punto) ([][]punto, bool) {
	switch uni {
	case '✓', '✔':
		return [][]punto{{pt(0.08, 0.35), pt(0.28, 0.12), pt(0.76, 0.68), pt(0.68, 0.75), pt(0.28, 0.28), pt(0.15, 0.43)}}, true
	case '✗', '✘', 0x2715, 0x2716:
		return [][]punto{
			{pt(0.12, 0.16), pt(0.20, 0.10), pt(0.72, 0.66), pt(0.64, 0.72)},
			{pt(0.64, 0.10), pt(0.72, 0.16), pt(0.20, 0.72), pt(0.12, 0.66)},
		}, true
	case '■', 0x274F, 0x2750, 0x2751, 0x2752:
		return [][]punto{{pt(0.10, 0.08), pt(0.70, 0.08), pt(0.70, 0.68), pt(0.10, 0.68)}}, true
	case '▲':
		return [][]punto{{pt(0.08, 0.08), pt(0.72, 0.08), pt(0.40, 0.70)}}, true
	case '▼':
		return [][]punto{{pt(0.08, 0.70), pt(0.72, 0.70), pt(0.40, 0.08)}}, true
	case '◆', '❖', '♦':
		return [][]punto{{pt(0.40, 0.06), pt(0.72, 0.38), pt(0.40, 0.70), pt(0.08, 0.38)}}, true
	case '♠':
		return [][]punto{
			{pt(0.40, 0.74), pt(0.10, 0.38), pt(0.16, 0.22), pt(0.34, 0.24), pt(0.40, 0.34), pt(0.46, 0.24), pt(0.64, 0.22), pt(0.70, 0.38)},
			{pt(0.35, 0.28), pt(0.45, 0.28), pt(0.52, 0.06), pt(0.28, 0.06)},
		}, true
	case '♥', 0x2764, 0x2765:
		return [][]punto{{
			pt(0.40, 0.06), pt(0.10, 0.42), pt(0.10, 0.58), pt(0.24, 0.70),
			pt(0.40, 0.56), pt(0.56, 0.70), pt(0.70, 0.58), pt(0.70, 0.42),
		}}, true
	case '♣':
		return [][]punto{
			discoCentroDingbat(pt, 0.40, 0.54, 0.16),
			discoCentroDingbat(pt, 0.24, 0.34, 0.16),
			discoCentroDingbat(pt, 0.56, 0.34, 0.16),
			{pt(0.35, 0.30), pt(0.45, 0.30), pt(0.52, 0.06), pt(0.28, 0.06)},
		}, true
	case '→', '➔', 0x2799, 0x279C, 0x279D, 0x279E, 0x279F, 0x27A0, 0x27A1:
		return [][]punto{{
			pt(0.06, 0.32), pt(0.48, 0.32), pt(0.48, 0.16), pt(0.76, 0.38),
			pt(0.48, 0.60), pt(0.48, 0.44), pt(0.06, 0.44),
		}}, true
	case '↔':
		return [][]punto{{
			pt(0.06, 0.38), pt(0.26, 0.20), pt(0.26, 0.32), pt(0.54, 0.32),
			pt(0.54, 0.20), pt(0.74, 0.38), pt(0.54, 0.56), pt(0.54, 0.44),
			pt(0.26, 0.44), pt(0.26, 0.56),
		}}, true
	case '↕':
		return [][]punto{{
			pt(0.40, 0.74), pt(0.22, 0.54), pt(0.34, 0.54), pt(0.34, 0.24),
			pt(0.22, 0.24), pt(0.40, 0.04), pt(0.58, 0.24), pt(0.46, 0.24),
			pt(0.46, 0.54), pt(0.58, 0.54),
		}}, true
	case '☎', '☛', '☞':
		return [][]punto{{pt(0.10, 0.18), pt(0.70, 0.18), pt(0.70, 0.58), pt(0.10, 0.58)}}, true
	case '◗':
		return [][]punto{{pt(0.40, 0.08), pt(0.68, 0.20), pt(0.68, 0.56), pt(0.40, 0.68)}}, true
	default:
		return nil, false
	}
}

func discoCentroDingbat(pt func(u, v float64) punto, cx, cy, rad float64) []punto {
	poly := make([]punto, 0, 12)
	for i := 0; i < 12; i++ {
		a := float64(i) * math.Pi / 6
		poly = append(poly, pt(cx+rad*math.Cos(a), cy+rad*math.Sin(a)))
	}
	return poly
}

func esDiscoDingbat(uni rune) bool {
	return uni == '●' || uni == '❍' || (uni >= 0x2460 && uni <= 0x2469) || (uni >= 0x2776 && uni <= 0x2793)
}

func esEstrellaDingbat(uni rune) bool {
	return uni == '★' || (uni >= 0x2729 && uni <= 0x274B)
}

func discoDingbat(pt func(u, v float64) punto) []punto {
	poly := make([]punto, 0, 16)
	for i := 0; i < 16; i++ {
		a := float64(i) * math.Pi / 8
		poly = append(poly, pt(0.40+0.30*math.Cos(a), 0.38+0.30*math.Sin(a)))
	}
	return poly
}

func estrellaDingbat(pt func(u, v float64) punto) []punto {
	poly := make([]punto, 0, 10)
	for i := 0; i < 10; i++ {
		rad := 0.34
		if i%2 == 1 {
			rad = 0.14
		}
		a := math.Pi/2 + float64(i)*math.Pi/5
		poly = append(poly, pt(0.40+rad*math.Cos(a), 0.38+rad*math.Sin(a)))
	}
	return poly
}

// --- Imágenes -------------------------------------------------------------------

// imagen coloca un mapa de bits en el cuadrado unidad transformado por `m`, que
// es donde la especificación pone las imágenes de PDF.
func (r *rasterizador) imagen(img image.Image, m matriz, rc *recorteRaster, alfa float64) {
	b := img.Bounds()
	if b.Empty() {
		return
	}
	esc := r.escala
	a0 := m[0] / float64(b.Dx()) * esc
	a1 := m[2] / float64(b.Dy()) * esc
	a3 := m[1] / float64(b.Dx()) * esc
	a4 := m[3] / float64(b.Dy()) * esc
	aff := f64.Aff3{
		a0, a1, m[4]*esc - a0*float64(b.Min.X) - a1*float64(b.Min.Y),
		a3, a4, m[5]*esc - a3*float64(b.Min.X) - a4*float64(b.Min.Y),
	}
	destino := draw.Image(r.dst)
	if rc != nil {
		area := rc.area(r.pagina)
		if area.Empty() {
			return
		}
		sub, ok := r.dst.SubImage(area).(*image.RGBA)
		if !ok {
			return
		}
		destino = sub
	}
	opciones := &draw.Options{}
	if rc != nil && rc.mask != nil {
		opciones.DstMask = rc.mask
	}
	if alfa < 1 {
		opciones.SrcMask = image.NewUniform(color.Alpha{A: uint8(acotar01(alfa) * 255)})
	}
	// CatmullRom y no BiLinear: un escaneo de 300 ppp reducido a pantalla con
	// interpolación lineal muestrea de menos y saca muaré en las tramas del papel,
	// que es exactamente lo que un visor de documentos no puede hacer.
	draw.CatmullRom.Transform(destino, aff, img, b, draw.Over, opciones)
}

// --- Color ----------------------------------------------------------------------

// colorRaster pasa el color ya calculado para el SVG («#rrggbb») al del mapa de
// bits. Se reutiliza la conversión de espacios de color entera: aquí solo cambia
// el envase.
func colorRaster(css string, alfa float64) color.NRGBA {
	if css == "" || css == "none" {
		return color.NRGBA{}
	}
	r, g, b := desdeCSS(css)
	return color.NRGBA{R: r, G: g, B: b, A: uint8(acotar01(alfa) * 255)}
}
