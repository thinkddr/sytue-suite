// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Censura de VECTORES: trazos y rellenos que caen en la zona marcada.
//
// Un trazado de PDF se construye con `m l c v y h re` y se PINTA con un operador
// aparte (`S f B n…`). Para quitarlo de verdad no basta con anular el operador de
// pintado: la geometría seguiría en el fichero, a un editor de distancia. Se
// borra el trazado ENTERO —desde su primer operador de construcción hasta el de
// pintado— truncando lo ya escrito en la salida, y se sustituye por lo que
// sobrevive fuera de la zona cuando se puede calcular sin ambigüedad:
//
//   - un RECTÁNGULO relleno se parte en hasta cuatro y se descarta lo de dentro
//     (así un fondo de página sigue siendo un fondo, con un agujero);
//   - un trazo de LÍNEAS RECTAS se recorta por la zona y se conserva el resto
//     (las rayas de una tabla que cruza la zona no desaparecen enteras);
//   - cualquier otro trazado que toque la zona se QUITA ENTERO. Es lo
//     conservador: curvas y polígonos rellenos se aproximan mal por piezas, y la
//     garantía es que la geometría de la zona no queda, no que lo demás se vea
//     idéntico.
//
// Un trazado de RECORTE (`W`) no dibuja nada y quitarlo cambiaría lo que se ve
// después, así que se conserva; solo se anula, si lo hubiera, su pintado.
package domain

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	// maxPilaGrafica acota los `q` sin cerrar que se recuerdan.
	maxPilaGrafica = 256
	// maxPuntosTrazado acota lo que se guarda de un trazado: más allá deja de
	// recortarse y, si toca la zona, se quita entero.
	maxPuntosTrazado = 100_000
)

// estadoGrafico es lo que `q`/`Q` salvan y restauran y a la censura le importa.
type estadoGrafico struct {
	ctm matrizPDF
	lw  float64
}

type ptPDF struct{ x, y float64 }

// caminoCensura es el trazado que se está construyendo, en coordenadas de PÁGINA.
type caminoCensura struct {
	// desdeSalida es dónde empieza el trazado en lo ya escrito en la salida.
	desdeSalida int
	// intermedios es lo que se escribió en mitad del trazado sin ser suyo (un
	// color, un grosor): si el trazado se quita, eso tiene que seguir.
	intermedios []byte

	caja    cajaPDF
	hayCaja bool
	recorte bool

	sub     [][]ptPDF // solo m, l y h; en espacio de página
	cerrado []bool
	// todo es el trazado ENTERO —rectas, rectángulos y curvas aplanadas— en
	// espacio de página: con él se recorta lo que no es un rectángulo suelto ni
	// solo rayas (recortarGeneral), en vez de quitarlo entero.
	todo        [][]ptPDF
	todoCerrado []bool
	nTodo       int
	nRect       int
	rect        [4]float64 // x y w h del único `re`, en espacio de usuario
	hayCurva    bool
	nPuntos     int
	excedido    bool
}

func nuevoCaminoCensura(desde int) *caminoCensura { return &caminoCensura{desdeSalida: desde} }

func (k *caminoCensura) alcanzar(p ptPDF) {
	if !k.hayCaja {
		k.caja, k.hayCaja = cajaPDF{p.x, p.y, p.x, p.y}, true
	} else {
		k.caja = cajaPDF{
			math.Min(k.caja.x1, p.x), math.Min(k.caja.y1, p.y),
			math.Max(k.caja.x2, p.x), math.Max(k.caja.y2, p.y),
		}
	}
	k.nPuntos++
	if k.nPuntos > maxPuntosTrazado {
		k.excedido = true
	}
}

func (k *caminoCensura) soloRect() bool {
	return k.nRect == 1 && len(k.sub) == 0 && !k.hayCurva && !k.excedido
}

func (k *caminoCensura) soloLineas() bool {
	return k.nRect == 0 && len(k.sub) > 0 && !k.hayCurva && !k.excedido
}

func esOperadorDeTrazado(op string) bool {
	switch op {
	case "m", "l", "c", "v", "y", "h", "re":
		return true
	}
	return false
}

func esPintadoDeTrazado(op string) bool {
	switch op {
	case "S", "s", "f", "F", "f*", "B", "B*", "b", "b*", "n":
		return true
	}
	return false
}

// sumarAlTrazado anota un operador de construcción. Los operandos se leen desde
// el final, como en el resto de la censura.
func (c *censor) sumarAlTrazado(op string, pila []pdfObj, desde int) {
	if c.cam == nil {
		c.cam = nuevoCaminoCensura(desde)
	}
	k := c.cam
	n := len(pila)
	num := func(desdeElFinal int) float64 {
		i := n - desdeElFinal
		if i < 0 || i >= n {
			return 0
		}
		return comoReal(pila[i])
	}
	punto := func(x, y float64) ptPDF {
		px, py := c.ctm.punto(x, y)
		return ptPDF{px, py}
	}
	k.anotarEnTodo(op, func(i int) ptPDF { return punto(num(i), num(i-1)) })
	switch op {
	case "m":
		p := punto(num(2), num(1))
		k.alcanzar(p)
		if !k.excedido {
			k.sub = append(k.sub, []ptPDF{p})
			k.cerrado = append(k.cerrado, false)
		}
	case "l":
		p := punto(num(2), num(1))
		k.alcanzar(p)
		if !k.excedido {
			if len(k.sub) == 0 {
				k.sub = append(k.sub, nil)
				k.cerrado = append(k.cerrado, false)
			}
			i := len(k.sub) - 1
			k.sub[i] = append(k.sub[i], p)
		}
	case "c":
		k.hayCurva = true
		k.alcanzar(punto(num(6), num(5)))
		k.alcanzar(punto(num(4), num(3)))
		k.alcanzar(punto(num(2), num(1)))
	case "v", "y":
		k.hayCurva = true
		k.alcanzar(punto(num(4), num(3)))
		k.alcanzar(punto(num(2), num(1)))
	case "h":
		if i := len(k.cerrado); i > 0 {
			k.cerrado[i-1] = true
		}
	case "re":
		x, y, w, h := num(4), num(3), num(2), num(1)
		k.nRect++
		k.rect = [4]float64{x, y, w, h}
		esquinas := make([]ptPDF, 0, 4)
		for _, p := range [][2]float64{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}} {
			esquinas = append(esquinas, punto(p[0], p[1]))
			k.alcanzar(esquinas[len(esquinas)-1])
		}
		if !k.excedido {
			k.nuevoEnTodo(true, esquinas...)
		}
	}
}

// anotarEnTodo lleva el trazado completo. `pt(i)` es el punto cuyos operandos
// están en las posiciones i e i-1 contando desde el final. Los rectángulos los
// anota sumarAlTrazado, que tiene sus cuatro esquinas.
func (k *caminoCensura) anotarEnTodo(op string, pt func(int) ptPDF) {
	if k.excedido {
		return
	}
	switch op {
	case "m":
		k.nuevoEnTodo(false, pt(2))
	case "l":
		k.anadirATodo(pt(2))
	case "c", "v", "y":
		n := len(k.todo)
		if n == 0 || len(k.todo[n-1]) == 0 {
			return
		}
		p0 := k.todo[n-1][len(k.todo[n-1])-1]
		var p1, p2, p3 ptPDF
		switch op {
		case "c":
			p1, p2, p3 = pt(6), pt(4), pt(2)
		case "v": // el primer punto de control es el punto actual
			p1, p2, p3 = p0, pt(4), pt(2)
		default: // y: el segundo es el final
			p1, p2, p3 = pt(4), pt(2), pt(2)
		}
		k.anadirATodo(aplanarBezier(p0, p1, p2, p3)...)
	case "h":
		if i := len(k.todoCerrado); i > 0 {
			k.todoCerrado[i-1] = true
		}
	}
}

func (k *caminoCensura) nuevoEnTodo(cerrado bool, ps ...ptPDF) {
	k.todo = append(k.todo, append([]ptPDF(nil), ps...))
	k.todoCerrado = append(k.todoCerrado, cerrado)
	k.contarTodo(len(ps))
}

func (k *caminoCensura) anadirATodo(ps ...ptPDF) {
	if len(k.todo) == 0 {
		k.nuevoEnTodo(false)
	}
	i := len(k.todo) - 1
	k.todo[i] = append(k.todo[i], ps...)
	k.contarTodo(len(ps))
}

func (k *caminoCensura) contarTodo(n int) {
	k.nTodo += n
	if k.nTodo > maxPuntosTrazado {
		k.excedido = true
	}
}

// aplanarBezier sustituye una curva cúbica por doce rectas. Basta para recortar:
// solo se reescribe un trazado que toca una zona, y a doce tramos la diferencia
// con la curva no se ve en el tamaño de una página.
func aplanarBezier(p0, p1, p2, p3 ptPDF) []ptPDF {
	const tramos = 12
	out := make([]ptPDF, 0, tramos)
	for i := 1; i <= tramos; i++ {
		t := float64(i) / tramos
		u := 1 - t
		a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
		out = append(out, ptPDF{a*p0.x + b*p1.x + c*p2.x + d*p3.x, a*p0.y + b*p1.y + c*p2.y + d*p3.y})
	}
	return out
}

type tipoAccionTrazado int

const (
	trazadoNada tipoAccionTrazado = iota
	// trazadoSustituir quita el trazado entero y escribe `texto` en su lugar.
	trazadoSustituir
	// trazadoSoloPintado conserva la construcción (es un recorte) y anula el pintado.
	trazadoSoloPintado
)

type accionTrazado struct {
	tipo  tipoAccionTrazado
	texto string
}

// resolverTrazado decide qué hacer con el trazado que `op` acaba de pintar.
func (c *censor) resolverTrazado(op string) accionTrazado {
	k := c.cam
	trazo, relleno := false, false
	switch op {
	case "S", "s":
		trazo = true
	case "f", "F", "f*":
		relleno = true
	case "B", "B*", "b", "b*":
		trazo, relleno = true, true
	default:
		return accionTrazado{} // `n`: no pinta
	}
	if !k.hayCaja {
		return accionTrazado{}
	}
	caja := k.caja
	if trazo {
		// El trazo sobresale de la geometría medio grosor, y con uniones en inglete
		// hasta el límite por defecto (10): cinco grosores. Un mínimo de un punto
		// para los de grosor cero, que se dibujan con el ancho de un píxel.
		m := math.Max(c.lw*c.escala()*5, 1)
		caja = cajaPDF{caja.x1 - m, caja.y1 - m, caja.x2 + m, caja.y2 + m}
	}
	if !caja.tocaAlguna(c.zonas) {
		return accionTrazado{}
	}
	c.cambios++
	if k.recorte {
		return accionTrazado{tipo: trazadoSoloPintado}
	}
	if relleno && !trazo && k.soloRect() {
		if t, ok := c.recortarRelleno(op); ok {
			return accionTrazado{tipo: trazadoSustituir, texto: t}
		}
	}
	if trazo && !relleno && k.soloLineas() {
		if t, ok := c.cortarLineas(op == "s"); ok {
			return accionTrazado{tipo: trazadoSustituir, texto: t}
		}
	}
	// Lo demás —polígonos, curvas, varios rectángulos, relleno y trazo a la vez—
	// también se recorta en vez de quitarse entero: un fondo de página dibujado
	// con m/l, el marco de una tabla o un círculo que asoma a la zona dejaban la
	// página en blanco fuera de ella (145 páginas en la medida).
	if t, ok := c.recortarGeneral(op, trazo, relleno); ok {
		return accionTrazado{tipo: trazadoSustituir, texto: t}
	}
	return accionTrazado{tipo: trazadoSustituir}
}

// recortarGeneral escribe, en espacio de usuario, lo que del trazado queda fuera
// de las zonas: el relleno recortado por las cuatro franjas que rodean cada zona
// (Sutherland–Hodgman contra un rectángulo, que es convexo y conserva la regla de
// relleno dentro de cada franja), y el trazo cortado tramo a tramo.
//
// La garantía es la misma que la de siempre: ningún punto de dentro de la zona
// sobrevive. Lo recortado solo tiene vértices fuera o en su borde.
func (c *censor) recortarGeneral(op string, trazo, relleno bool) (string, bool) {
	k := c.cam
	if k.excedido || len(k.todo) == 0 {
		return "", false
	}
	inv, ok := c.ctm.inversa()
	if !ok {
		return "", false
	}
	var b strings.Builder
	if relleno && !c.rellenoRecortado(&b, inv, op) {
		return "", false
	}
	if trazo && !c.trazoRecortado(&b, inv, op == "s" || op == "b" || op == "b*") {
		return "", false
	}
	return b.String(), true
}

// escribirPunto escribe un punto de página en espacio de usuario con su operador.
func escribirPunto(b *strings.Builder, inv matrizPDF, p ptPDF, op string) {
	x, y := inv.punto(p.x, p.y)
	fmt.Fprintf(b, "%s %s %s\n", numFino(x), numFino(y), op)
}

// rellenoRecortado escribe el relleno recortado por las franjas de cada zona.
func (c *censor) rellenoRecortado(b *strings.Builder, inv matrizPDF, op string) bool {
	piezas := [][][]ptPDF{c.cam.todo}
	for _, z := range c.zonas {
		var sig [][][]ptPDF
		for _, p := range piezas {
			for _, f := range franjasFuera(z, cajaDeSubcaminos(p)) {
				if q := recortarARectangulo(p, f); len(q) > 0 {
					sig = append(sig, q)
				}
			}
		}
		piezas = sig
		if len(piezas) > 256 {
			return false
		}
	}
	hay := false
	for _, p := range piezas {
		for _, sp := range p {
			escribirPunto(b, inv, sp[0], "m")
			for _, pt := range sp[1:] {
				escribirPunto(b, inv, pt, "l")
			}
			b.WriteString("h\n")
			hay = true
		}
	}
	if hay {
		if strings.HasSuffix(op, "*") {
			b.WriteString("f*\n")
		} else {
			b.WriteString("f\n")
		}
	}
	return true
}

// trazoRecortado escribe los tramos del trazo que quedan fuera de las zonas.
func (c *censor) trazoRecortado(b *strings.Builder, inv matrizPDF, cierraElUltimo bool) bool {
	k := c.cam
	var tramos [][2]ptPDF
	for i, sp := range k.todo {
		for j := 0; j+1 < len(sp); j++ {
			tramos = append(tramos, [2]ptPDF{sp[j], sp[j+1]})
		}
		if len(sp) > 2 && (k.todoCerrado[i] || (cierraElUltimo && i == len(k.todo)-1)) {
			tramos = append(tramos, [2]ptPDF{sp[len(sp)-1], sp[0]})
		}
	}
	for _, z := range c.zonas {
		sig := make([][2]ptPDF, 0, len(tramos))
		for _, t := range tramos {
			sig = append(sig, tramosFueraDeZona(t[0], t[1], z)...)
		}
		tramos = sig
		if len(tramos) > 2*maxPuntosTrazado {
			return false
		}
	}
	for _, t := range tramos {
		escribirPunto(b, inv, t[0], "m")
		escribirPunto(b, inv, t[1], "l")
	}
	if len(tramos) > 0 {
		b.WriteString("S\n")
	}
	return true
}

// cajaDeSubcaminos es la caja que encierra todos los puntos.
func cajaDeSubcaminos(p [][]ptPDF) cajaPDF {
	c := cajaPDF{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, sp := range p {
		for _, q := range sp {
			c = cajaPDF{math.Min(c.x1, q.x), math.Min(c.y1, q.y), math.Max(c.x2, q.x), math.Max(c.y2, q.y)}
		}
	}
	return c
}

// franjasFuera parte lo que rodea a la zona dentro de la caja en cuatro franjas
// que no se solapan: izquierda y derecha de alto entero, y abajo y arriba del
// ancho de la zona.
func franjasFuera(z AreaRedaccion, c cajaPDF) []cajaPDF {
	out := make([]cajaPDF, 0, 4)
	anadir := func(f cajaPDF) {
		if f.x2 > f.x1 && f.y2 > f.y1 {
			out = append(out, f)
		}
	}
	anadir(cajaPDF{c.x1, c.y1, math.Min(z.X1, c.x2), c.y2})
	anadir(cajaPDF{math.Max(z.X2, c.x1), c.y1, c.x2, c.y2})
	x1, x2 := math.Max(z.X1, c.x1), math.Min(z.X2, c.x2)
	anadir(cajaPDF{x1, c.y1, x2, math.Min(z.Y1, c.y2)})
	anadir(cajaPDF{x1, math.Max(z.Y2, c.y1), x2, c.y2})
	return out
}

// recortarARectangulo recorta cada subcamino, como polígono cerrado, contra el
// rectángulo (Sutherland–Hodgman: un borde cada vez).
func recortarARectangulo(p [][]ptPDF, r cajaPDF) [][]ptPDF {
	var out [][]ptPDF
	for _, sp := range p {
		poli := sp
		for borde := 0; borde < 4 && len(poli) > 0; borde++ {
			poli = recortarPorBorde(poli, r, borde)
		}
		if len(poli) >= 3 {
			out = append(out, poli)
		}
	}
	return out
}

func recortarPorBorde(poli []ptPDF, r cajaPDF, borde int) []ptPDF {
	dentro := func(p ptPDF) bool {
		switch borde {
		case 0:
			return p.x >= r.x1
		case 1:
			return p.x <= r.x2
		case 2:
			return p.y >= r.y1
		default:
			return p.y <= r.y2
		}
	}
	corte := func(a, b ptPDF) ptPDF {
		var t float64
		switch borde {
		case 0:
			t = (r.x1 - a.x) / (b.x - a.x)
		case 1:
			t = (r.x2 - a.x) / (b.x - a.x)
		case 2:
			t = (r.y1 - a.y) / (b.y - a.y)
		default:
			t = (r.y2 - a.y) / (b.y - a.y)
		}
		return ptPDF{a.x + t*(b.x-a.x), a.y + t*(b.y-a.y)}
	}
	out := make([]ptPDF, 0, len(poli)+2)
	for i, b := range poli {
		a := poli[(i+len(poli)-1)%len(poli)]
		switch {
		case dentro(b) && dentro(a):
			out = append(out, b)
		case dentro(b):
			out = append(out, corte(a, b), b)
		case dentro(a):
			out = append(out, corte(a, b))
		}
	}
	return out
}

// escala es cuánto agranda la CTM vigente: cota para convertir grosores de
// espacio de usuario a puntos de página.
func (c *censor) escala() float64 {
	return math.Max(math.Hypot(c.ctm[0], c.ctm[1]), math.Hypot(c.ctm[2], c.ctm[3]))
}

// finita dice si todos los números de la matriz son finitos.
func (m matrizPDF) finita() bool {
	for _, v := range m {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// inversa devuelve la matriz inversa, o false si la transformación es singular.
func (m matrizPDF) inversa() (matrizPDF, bool) {
	det := m[0]*m[3] - m[1]*m[2]
	if math.Abs(det) < 1e-12 || math.IsNaN(det) || math.IsInf(det, 0) {
		return matrizPDF{}, false
	}
	return matrizPDF{
		m[3] / det, -m[1] / det,
		-m[2] / det, m[0] / det,
		(m[2]*m[5] - m[3]*m[4]) / det,
		(m[1]*m[4] - m[0]*m[5]) / det,
	}, true
}

// zonaEnUsuario lleva una zona de página al espacio de usuario vigente. Solo hay
// resultado si la CTM conserva los ejes (escala, traslación, giro de 90°): con un
// giro cualquiera la zona dejaría de ser un rectángulo y no se puede restar.
func (c *censor) zonaEnUsuario(z AreaRedaccion) (cajaPDF, bool) {
	inv, ok := c.ctm.inversa()
	if !ok {
		return cajaPDF{}, false
	}
	m := c.ctm
	tol := 1e-9 * (math.Abs(m[0]) + math.Abs(m[1]) + math.Abs(m[2]) + math.Abs(m[3]))
	conservaEjes := (math.Abs(m[1]) <= tol && math.Abs(m[2]) <= tol) ||
		(math.Abs(m[0]) <= tol && math.Abs(m[3]) <= tol)
	if !conservaEjes {
		return cajaPDF{}, false
	}
	xa, ya := inv.punto(z.X1, z.Y1)
	xb, yb := inv.punto(z.X2, z.Y2)
	return cajaPDF{math.Min(xa, xb), math.Min(ya, yb), math.Max(xa, xb), math.Max(ya, yb)}, true
}

// restarCaja devuelve lo que queda de r al quitarle z, en piezas que no se solapan.
func restarCaja(r, z cajaPDF) []cajaPDF {
	if z.x2 <= r.x1 || z.x1 >= r.x2 || z.y2 <= r.y1 || z.y1 >= r.y2 {
		return []cajaPDF{r}
	}
	var out []cajaPDF
	if z.x1 > r.x1 {
		out = append(out, cajaPDF{r.x1, r.y1, z.x1, r.y2})
	}
	if z.x2 < r.x2 {
		out = append(out, cajaPDF{z.x2, r.y1, r.x2, r.y2})
	}
	ix1, ix2 := math.Max(r.x1, z.x1), math.Min(r.x2, z.x2)
	if z.y1 > r.y1 {
		out = append(out, cajaPDF{ix1, r.y1, ix2, z.y1})
	}
	if z.y2 < r.y2 {
		out = append(out, cajaPDF{ix1, z.y2, ix2, r.y2})
	}
	return out
}

// recortarRelleno sustituye un único `re` relleno por lo que queda fuera de las zonas.
func (c *censor) recortarRelleno(op string) (string, bool) {
	r := c.cam.rect
	piezas := []cajaPDF{{
		math.Min(r[0], r[0]+r[2]), math.Min(r[1], r[1]+r[3]),
		math.Max(r[0], r[0]+r[2]), math.Max(r[1], r[1]+r[3]),
	}}
	for _, z := range c.zonas {
		zu, ok := c.zonaEnUsuario(z)
		if !ok {
			return "", false
		}
		var sig []cajaPDF
		for _, p := range piezas {
			sig = append(sig, restarCaja(p, zu)...)
		}
		piezas = sig
		if len(piezas) > 64 {
			return "", false // muchas zonas sobre un mismo rectángulo: se quita entero
		}
	}
	var b strings.Builder
	for _, p := range piezas {
		fmt.Fprintf(&b, "%s %s %s %s re\n", numFino(p.x1), numFino(p.y1), numFino(p.x2-p.x1), numFino(p.y2-p.y1))
	}
	if len(piezas) > 0 {
		b.WriteString(op + "\n")
	}
	return b.String(), true
}

// cortarLineas recorta por las zonas un trazo de segmentos rectos.
func (c *censor) cortarLineas(cierraElUltimo bool) (string, bool) {
	inv, ok := c.ctm.inversa()
	if !ok {
		return "", false
	}
	k := c.cam
	var tramos [][2]ptPDF
	for i, sp := range k.sub {
		for j := 0; j+1 < len(sp); j++ {
			tramos = append(tramos, [2]ptPDF{sp[j], sp[j+1]})
		}
		if len(sp) > 2 && (k.cerrado[i] || (cierraElUltimo && i == len(k.sub)-1)) {
			tramos = append(tramos, [2]ptPDF{sp[len(sp)-1], sp[0]})
		}
	}
	for _, z := range c.zonas {
		sig := make([][2]ptPDF, 0, len(tramos))
		for _, t := range tramos {
			sig = append(sig, tramosFueraDeZona(t[0], t[1], z)...)
		}
		tramos = sig
		if len(tramos) > 2*maxPuntosTrazado {
			return "", false
		}
	}
	var b strings.Builder
	for _, t := range tramos {
		ax, ay := inv.punto(t[0].x, t[0].y)
		bx, by := inv.punto(t[1].x, t[1].y)
		fmt.Fprintf(&b, "%s %s m %s %s l\n", numFino(ax), numFino(ay), numFino(bx), numFino(by))
	}
	if len(tramos) > 0 {
		b.WriteString("S\n")
	}
	return b.String(), true
}

// tramosFueraDeZona devuelve las partes del segmento a-b que quedan fuera de la
// zona (Liang–Barsky): ninguna si cae entero dentro, una o dos si la cruza.
func tramosFueraDeZona(a, b ptPDF, z AreaRedaccion) [][2]ptPDF {
	dx, dy := b.x-a.x, b.y-a.y
	t0, t1 := 0.0, 1.0
	recorta := func(p, q float64) bool {
		if p == 0 {
			return q >= 0
		}
		t := q / p
		if p < 0 {
			if t > t1 {
				return false
			}
			t0 = math.Max(t0, t)
		} else {
			if t < t0 {
				return false
			}
			t1 = math.Min(t1, t)
		}
		return true
	}
	dentro := recorta(-dx, a.x-z.X1) && recorta(dx, z.X2-a.x) &&
		recorta(-dy, a.y-z.Y1) && recorta(dy, z.Y2-a.y)
	if !dentro || t0 >= t1 {
		return [][2]ptPDF{{a, b}}
	}
	var out [][2]ptPDF
	if t0 > 0 {
		out = append(out, [2]ptPDF{a, {a.x + t0*dx, a.y + t0*dy}})
	}
	if t1 < 1 {
		out = append(out, [2]ptPDF{{a.x + t1*dx, a.y + t1*dy}, b})
	}
	return out
}

// numFino formatea un número de espacio de usuario con precisión de sobra: el
// espacio de usuario puede estar escalado (0,001 por unidad) y tres decimales
// descuadrarían lo que se conserva.
func numFino(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "0"
	}
	s := strconv.FormatFloat(v, 'f', 6, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "" || s == "-" || s == "-0" {
		return "0"
	}
	return s
}

// grosorDeEstado recoge /LW de un estado gráfico con nombre (`gs`).
func (c *censor) grosorDeEstado(pila []pdfObj) {
	if len(pila) == 0 {
		return
	}
	nombre, ok := pila[len(pila)-1].(pdfName)
	if !ok {
		return
	}
	estados, _ := c.doc.resolver(c.recursos["ExtGState"]).(pdfDict)
	gs, _ := c.doc.resolver(estados[nombre]).(pdfDict)
	switch lw := c.doc.resolver(gs["LW"]).(type) {
	case float64:
		c.lw = math.Abs(lw)
	case int64:
		c.lw = math.Abs(float64(lw))
	}
}

// comprobarDegradado se niega a seguir ante un degradado que pueda llevar datos.
//
// Los axiales y radiales (tipos 2 y 3) son una función de color sobre una recta o
// un círculo: no pueden esconder una imagen ni un texto. Las mallas (4 a 7) y los
// basados en función (1) sí: son datos de vértice o una función arbitraria, y
// `sh` pinta toda la región de recorte vigente, de extensión desconocida.
func (c *censor) comprobarDegradado(nombre pdfName) error {
	catalogo, _ := c.doc.resolver(c.recursos["Shading"]).(pdfDict)
	var d pdfDict
	switch v := c.doc.resolver(catalogo[nombre]).(type) {
	case pdfDict:
		d = v
	case *pdfStream:
		d = v.Dict
	}
	if d == nil {
		return nil
	}
	tipo, _ := comoEntero(c.doc.resolver(d["ShadingType"]))
	if tipo == 2 || tipo == 3 {
		return nil
	}
	if bb, ok := c.doc.resolver(d["BBox"]).(pdfArray); ok && len(bb) >= 4 {
		var v [4]float64
		for i := 0; i < 4; i++ {
			v[i] = comoReal(c.doc.resolver(bb[i]))
		}
		if !cajaDeRectangulo(c.ctm, v[0], v[1], v[2], v[3]).tocaAlguna(c.zonas) {
			return nil
		}
	}
	return fmt.Errorf("%w: la página dibuja un degradado de malla o de función (/%s) cuya "+
		"extensión no se puede acotar a la zona", ErrRedaccionNoGarantizada, nombre)
}
