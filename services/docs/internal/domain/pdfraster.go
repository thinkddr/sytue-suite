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
// tarde): regla par-impar (`f*` se rellena como no-cero), modos de fusión, mallas
// de sombreado y mosaicos. Cada uno degrada a algo visible —el color medio del
// sombreado, el relleno normal— nunca a un hueco mudo. Quien necesite fidelidad
// completa pide SVG, que sigue siendo lo que la web usa.
package domain

import (
	"errors"
	"image"
	"image/color"
	"math"
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

// rellenar pinta los subcaminos con un color liso.
func (r *rasterizador) rellenar(subs [][]punto, c color.NRGBA, rc *recorteRaster) {
	if c.A == 0 || len(subs) == 0 {
		return
	}
	caja := r.caja(subs, rc)
	if caja.Empty() {
		return
	}
	m := r.mascara(subs, caja)
	if rc != nil && rc.mask != nil {
		multiplicarMascara(m, rc.mask)
	}
	draw.DrawMask(r.dst, caja, image.NewUniform(c), image.Point{}, m, caja.Min, draw.Over)
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
func (r *rasterizador) nuevoRecorte(subs [][]punto, previo *recorteRaster) *recorteRaster {
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
	copiarAlfa(m, r.mascara(subs, caja))
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
	return r.nuevoRecorte(subs, previo)
}

// --- Trazo --------------------------------------------------------------------

// trazar dibuja el contorno. No hay «rasterizador de líneas»: cada tramo se
// convierte en un polígono y se rellenan todos JUNTOS y con el MISMO sentido de
// giro, que es lo que hace que los solapes de dos tramos consecutivos no se
// resten entre sí con la regla no-cero.
func (r *rasterizador) trazar(subs [][]punto, e *ejecucion, c color.NRGBA) {
	w := float32(e.estado.grosor * r.factor(e.estado.ctm))
	if e.estado.grosor <= 0 || w < 1 {
		// Grosor 0 en PDF es «lo más fino que el dispositivo sepa pintar» (§8.4.3.2),
		// y por debajo de un píxel una tabla se queda sin bordes.
		w = 1
	}
	if g := guionesEnPixeles(e.estado.guiones, r.factor(e.estado.ctm)); len(g) > 0 {
		subs = trocearConGuiones(subs, g)
	}
	polis := make([][]punto, 0, len(subs))
	for _, s := range subs {
		polis = append(polis, poligonosDeTramo(s, w, e.estado.tapa, e.estado.union, e.estado.limite)...)
	}
	r.rellenar(polis, c, e.estado.recorte)
}

// poligonosDeTramo convierte UNA polilínea en los polígonos que la dibujan: un
// rectángulo por tramo, una unión por vértice interior y un remate por punta.
func poligonosDeTramo(s []punto, w float32, tapa, union int, limite float64) [][]punto {
	if len(s) < 2 {
		if len(s) == 1 && tapa == 1 {
			return [][]punto{circulo(s[0], w/2)}
		}
		return nil
	}
	hw := w / 2
	var out [][]punto
	cerrado := s[0] == s[len(s)-1]
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

// trocearConGuiones parte las polilíneas en los trozos que sí se pintan.
func trocearConGuiones(subs [][]punto, patron []float32) [][]punto {
	var out [][]punto
	for _, s := range subs {
		idx, resto, pinta := 0, patron[0], true
		var act []punto
		if pinta {
			act = []punto{s[0]}
		}
		for i := 0; i+1 < len(s); i++ {
			a, b := s[i], s[i+1]
			largo := distancia(a, b)
			for largo > resto {
				t := resto / largo
				corte := punto{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t}
				if pinta {
					act = append(act, corte)
					out = append(out, act)
					act = nil
				} else {
					act = []punto{corte}
				}
				pinta = !pinta
				a = corte
				largo -= resto
				idx = (idx + 1) % len(patron)
				resto = patron[idx]
				if resto <= 0 {
					resto = 0.01
				}
			}
			resto -= largo
			if pinta {
				act = append(act, b)
			}
		}
		if len(act) > 1 {
			out = append(out, act)
		}
	}
	return out
}

// --- Pintado de caminos ---------------------------------------------------------

// pintarRaster es lo que `pintar` hace en el mapa de bits: el MISMO camino, el
// MISMO estado gráfico y el mismo orden, con otro destino.
func (e *ejecucion) pintarRaster(hab, relleno, trazo bool, pend int) {
	if !hab {
		return
	}
	r := e.l.r
	subs := r.aplanar(e.cam, e.estado.ctm)
	if len(subs) == 0 {
		return
	}
	switch {
	case relleno && e.estado.patronRelleno != "" && len(e.estado.mallaRelleno) > 0:
		r.dibujarMalla(e.estado.mallaRelleno, e.estado.matrizMalla, e.estado.alfaRelleno, r.nuevoRecorte(subs, e.estado.recorte))
	case relleno && e.estado.patronRelleno != "" && e.estado.degRelleno != nil:
		r.rellenarConDegradado(subs, e.estado.degRelleno.conAlfa(e.estado.alfaRelleno), e.estado.recorte)
	case relleno:
		if c, ok := colorDePintura(e.estado.relleno, e.estado.patronRelleno,
			e.estado.aproxRelleno, e.estado.alfaRelleno); ok {
			r.rellenar(subs, c, e.estado.recorte)
		}
	}
	if trazo {
		if c, ok := colorDePintura(e.estado.trazo, e.estado.patronTrazo,
			e.estado.aproxTrazo, e.estado.alfaTrazo); ok {
			r.trazar(subs, e, c)
		}
	}
	if pend != 0 {
		// El recorte entra en el estado gráfico vigente y lo deshace `Q`, igual que
		// en PDF. En el SVG eso mismo es un `<g clip-path>` que se cierra al salir
		// del nivel.
		e.estado.recorte = r.nuevoRecorte(subs, e.estado.recorte)
	}
}

// colorDePintura decide con qué color liso se pinta. Un patrón no es un color:
// de un sombreado se toma su color medio —que es lo que ya hace el SVG con las
// mallas— y de un mosaico no se toma nada, porque inventar uno pintaría bloques
// del color equivocado encima del documento.
func colorDePintura(css, patron, aprox string, alfa float64) (color.NRGBA, bool) {
	if patron != "" {
		if aprox == "" {
			return color.NRGBA{}, false
		}
		return colorRaster(aprox, alfa), true
	}
	c := colorRaster(css, alfa)
	return c, c.A != 0
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

// rellenarConDegradado es rellenar con un degradado en vez de un color.
func (r *rasterizador) rellenarConDegradado(subs [][]punto, g *degradadoRaster, rc *recorteRaster) {
	caja := r.caja(subs, rc)
	if caja.Empty() {
		return
	}
	m := r.mascara(subs, caja)
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
		if gid, ok := f.indiceDeGlifo(cara, &r.buf, g); ok {
			if subs := r.contorno(cara, gid, upem, trm, dx, dy, e.estado.tamano); len(subs) > 0 {
				e.pintarGlifo(subs, relleno)
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

// pintarGlifo aplica el modo de render del texto (§9.3.6) al contorno ya
// transformado. Los modos 4-6 añaden recorte además de pintar; el recorte no se
// aplica, pero lo que dibujan sí, que es lo que se ve.
func (e *ejecucion) pintarGlifo(subs [][]punto, relleno color.NRGBA) {
	r := e.l.r
	rellenar := func() {
		if e.estado.patronRelleno != "" && e.estado.degRelleno != nil {
			r.rellenarConDegradado(subs, e.estado.degRelleno.conAlfa(e.estado.alfaRelleno), e.estado.recorte)
			return
		}
		r.rellenar(subs, relleno, e.estado.recorte)
	}
	switch e.estado.modoRender {
	case 1, 5:
		r.trazar(subs, e, colorRaster(e.estado.trazo, e.estado.alfaTrazo))
	case 2, 6:
		rellenar()
		r.trazar(subs, e, colorRaster(e.estado.trazo, e.estado.alfaTrazo))
	default:
		rellenar()
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
		if len(f.programa) > 0 {
			if cara, err := sfnt.Parse(f.programa); err == nil {
				f.cara = cara
				f.cmapUnicode = tieneCmapUnicode(f.programa)
				f.cmapMacRoman = tieneCmapMacRoman(f.programa)
			}
		}
		if f.cara == nil {
			// Sin tipografía utilizable —no incrustada, Type1 antiguo, CFF suelto—
			// se dibuja con la de reserva y CADA GLIFO EN SU SITIO: el avance lo
			// manda el PDF, así que la línea no se descuadra aunque la letra sea
			// otra. Es la misma degradación que el SVG hace con `familiaSustituta`.
			f.cara, f.reserva = caraDeReserva(f.negrita, f.italica, f.familia), true
		}
	}
	if f.cara == nil {
		return nil, 0
	}
	return f.cara, float64(f.cara.UnitsPerEm())
}

// indiceDeGlifo resuelve qué contorno dibujar para un código del PDF.
func (f *fuentePDF) indiceDeGlifo(cara *sfnt.Font, buf *sfnt.Buffer, g glifo) (sfnt.GlyphIndex, bool) {
	if f.dosBytes && !f.reserva {
		// Identity-H: el código ES el índice de glifo, salvo que /CIDToGIDMap traiga
		// una tabla que diga otra cosa.
		gid := g.codigo
		if gid > maxCID {
			return 0, false
		}
		if len(f.cidAGid) > 0 {
			if int(2*gid+1) >= len(f.cidAGid) {
				return 0, false
			}
			gid = uint32(f.cidAGid[2*gid])<<8 | uint32(f.cidAGid[2*gid+1])
		}
		if int(gid) >= cara.NumGlyphs() {
			return 0, false
		}
		return sfnt.GlyphIndex(gid), true //nolint:gosec // G115: acotado por NumGlyphs, que maxp guarda en 16 bits
	}
	// Fuente simple (§9.6.6.4). Con una tabla Unicode se busca por carácter; con
	// una (3,0) o (1,0) —lo corriente en un SUBCONJUNTO, que renumera los
	// códigos desde 1— la clave es el CÓDIGO, y buscar por carácter no encuentra
	// nada: medido en un formulario de CUPS, no salía ni una letra. Las de
	// símbolos colocan sus códigos en el área privada F000.
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
