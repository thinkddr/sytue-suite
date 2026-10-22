// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sombreados de malla: los tipos 4 a 7 de §8.7.4.5.
//
// LOS OTROS SOMBREADOS SON UNA FÓRMULA; ESTOS SON GEOMETRÍA. Un sombreado axial
// se describe con dos puntos y una función, y eso traduce a `<linearGradient>`
// casi literalmente. Una malla es una lista de TRIÁNGULOS con un color en cada
// vértice, y SVG no tiene nada equivalente: no existe el degradado de malla.
//
// Antes se resolvían con el color del centro del dominio, o sea que un degradado
// de malla —el fondo de una portada, el sombreado de un gráfico de InDesign— se
// veía como un bloque de color plano. No era un error, era otro documento.
//
// LO QUE SE HACE AQUÍ: se decodifica la malla a triángulos y cada uno se
// SUBDIVIDE unas cuantas veces, rellenando cada trocito con el color medio de sus
// esquinas. Es interpolación plana por trozos en vez de Gouraud, y a partir de
// dos niveles de subdivisión la diferencia no se distingue a ojo.
//
// El techo, dicho aquí: no es Gouraud de verdad. Un triángulo enorme con tres
// colores muy separados puede mostrar bandas. La salida sería emitir un
// `<filter>` con tres pasadas aditivas —el truco clásico—, que en la práctica
// pinta distinto en cada navegador y por eso no se ha hecho.
package domain

import (
	"math"
	"strings"
)

// verticeMalla es un punto de la malla con su color ya resuelto a RGB.
type verticeMalla struct {
	x, y float64
	css  string
}

// trianguloMalla es la unidad que se sabe pintar.
type trianguloMalla struct{ a, b, c verticeMalla }

// Topes de la decodificación. Una malla real tiene decenas o cientos de
// triángulos; los topes existen para que un fichero manipulado no describa un
// millón de polígonos en unos kilobytes comprimidos.
const (
	maxTriangulosMalla = 20000
	maxVerticesMalla   = 60000
	// nivelesSubdivision: cada nivel multiplica por CUATRO los triángulos. Dos
	// niveles son 16 trocitos por triángulo de entrada, que es donde la banda
	// deja de verse sin que el SVG se dispare.
	nivelesSubdivision = 2
	// subdivisionMinima es el lado en puntos por debajo del cual no se subdivide
	// más: partir un triángulo de dos puntos no cambia un solo píxel.
	subdivisionMinima = 3.0
)

// lectorDeBits lee campos de anchura arbitraria de un flujo de malla.
//
// Las mallas empaquetan coordenadas de 24 o 32 bits y colores de 8 o 16 SIN
// alinear a byte, así que no hay forma de leerlas con índices normales.
type lectorDeBits struct {
	datos []byte
	pos   int // en BITS
}

// leer saca `n` bits como número sin signo. Devuelve false al agotarse, que es
// la señal de que la malla terminó.
func (r *lectorDeBits) leer(n int) (uint64, bool) {
	if n <= 0 || n > 32 || r.pos+n > len(r.datos)*8 {
		return 0, false
	}
	var v uint64
	for i := 0; i < n; i++ {
		idx := (r.pos + i) / 8
		bit := (r.datos[idx] >> uint(7-(r.pos+i)%8)) & 1
		v = v<<1 | uint64(bit)
	}
	r.pos += n
	return v, true
}

// alinearAByte descarta lo que quede del byte en curso. Los tipos 4, 6 y 7
// alinean cada vértice o parche, y sin esto el siguiente sale desplazado unos
// bits y la malla se convierte en ruido.
func (r *lectorDeBits) alinearAByte() {
	if r.pos%8 != 0 {
		r.pos += 8 - r.pos%8
	}
}

func (r *lectorDeBits) agotado() bool { return r.pos >= len(r.datos)*8 }

// parametrosMalla son los campos comunes a los cuatro tipos.
type parametrosMalla struct {
	bitsCoord   int
	bitsComp    int
	bitsBandera int
	decode      []float64
	ec          *espacioColor
	// fn, si no es nil, convierte UN parámetro en color: con /Function la malla
	// guarda un valor t por vértice en vez de los componentes del color.
	fn *funcionPDF
	// comps es cuántos números de color trae cada vértice.
	comps int
}

func (l *lienzo) parametrosDeMalla(dict pdfDict) (parametrosMalla, bool) {
	var p parametrosMalla
	ent := func(clave pdfName) int {
		v, _ := comoEntero(l.d.resolver(dict[clave]))
		return int(v)
	}
	p.bitsCoord = ent("BitsPerCoordinate")
	p.bitsComp = ent("BitsPerComponent")
	p.bitsBandera = ent("BitsPerFlag")
	p.decode = l.reales(dict["Decode"])
	p.ec = l.espacio(dict["ColorSpace"], 0)
	p.fn = l.funcion(dict["Function"], 0)

	p.comps = p.ec.componentes()
	if p.ec.clase == "Indexed" {
		p.comps = 1
	}
	if p.fn != nil {
		// Con /Function el vértice guarda UN parámetro, no el color entero.
		p.comps = 1
	}
	// La validación importa: con anchuras absurdas el lector devolvería basura
	// consistente, que es la peor clase de fallo — una malla plausible y mal.
	if p.bitsCoord < 1 || p.bitsCoord > 32 || p.bitsComp < 1 || p.bitsComp > 32 {
		return p, false
	}
	if len(p.decode) < 4+2*p.comps {
		return p, false
	}
	return p, true
}

// leerVertice saca un vértice: coordenadas y color, cada campo con su anchura y
// pasado por /Decode.
func (p parametrosMalla) leerVertice(r *lectorDeBits) (verticeMalla, bool) {
	var v verticeMalla
	bx, ok := r.leer(p.bitsCoord)
	if !ok {
		return v, false
	}
	by, ok := r.leer(p.bitsCoord)
	if !ok {
		return v, false
	}
	maxCoord := math.Pow(2, float64(p.bitsCoord)) - 1
	v.x = p.decode[0] + float64(bx)/maxCoord*(p.decode[1]-p.decode[0])
	v.y = p.decode[2] + float64(by)/maxCoord*(p.decode[3]-p.decode[2])

	maxComp := math.Pow(2, float64(p.bitsComp)) - 1
	comps := make([]float64, 0, p.comps)
	for i := 0; i < p.comps; i++ {
		b, ok := r.leer(p.bitsComp)
		if !ok {
			return v, false
		}
		lo, hi := p.decode[4+2*i], p.decode[5+2*i]
		comps = append(comps, lo+float64(b)/maxComp*(hi-lo))
	}
	if p.fn != nil {
		comps = p.fn.evaluar(comps[0])
	}
	v.css = p.ec.css(comps)
	return v, true
}

// trianguloDeMalla decodifica los tipos 4 y 5 a una lista de triángulos.
func (l *lienzo) trianguloDeMalla(st *pdfStream, tipo int) ([]trianguloMalla, bool) {
	datos, err := l.d.datosDeFlujo(st)
	if err != nil {
		return nil, false
	}
	p, ok := l.parametrosDeMalla(st.Dict)
	if !ok {
		return nil, false
	}
	r := &lectorDeBits{datos: datos}

	switch tipo {
	case 4:
		return mallaLibre(r, p)
	case 5:
		filas, _ := comoEntero(l.d.resolver(st.Dict["VerticesPerRow"]))
		return mallaEnRejilla(r, p, int(filas))
	}
	return nil, false
}

// mallaLibre lee el tipo 4: cada vértice lleva una BANDERA que dice si empieza
// un triángulo nuevo o continúa el anterior.
//
// Bandera 0 = arranca uno nuevo y hay que leer los tres vértices; 1 = tira del
// segundo y tercero del anterior; 2 = del primero y el tercero. Es una tira de
// triángulos, y confundir 1 con 2 la retuerce sin dar ningún error.
func mallaLibre(r *lectorDeBits, p parametrosMalla) ([]trianguloMalla, bool) {
	if p.bitsBandera < 1 || p.bitsBandera > 32 {
		return nil, false
	}
	var out []trianguloMalla
	var va, vb, vc verticeMalla
	tengo := 0

	for len(out) < maxTriangulosMalla && !r.agotado() {
		bandera, ok := r.leer(p.bitsBandera)
		if !ok {
			break
		}
		v, ok := p.leerVertice(r)
		if !ok {
			break
		}
		r.alinearAByte()

		switch {
		case bandera == 0 || tengo < 3:
			// Un triángulo nuevo son TRES vértices con su propia bandera cada uno.
			if tengo < 2 {
				if tengo == 0 {
					va = v
				} else {
					vb = v
				}
				tengo++
				continue
			}
			vc = v
			tengo = 3
		case bandera == 1:
			va, vb, vc = vb, vc, v
		default: // bandera 2
			vb, vc = vc, v
		}
		out = append(out, trianguloMalla{va, vb, vc})
	}
	return out, len(out) > 0
}

// mallaEnRejilla lee el tipo 5: sin banderas, los vértices vienen por filas de
// `porFila` y se triangulan entre filas consecutivas.
func mallaEnRejilla(r *lectorDeBits, p parametrosMalla, porFila int) ([]trianguloMalla, bool) {
	if porFila < 2 || porFila > 8192 {
		return nil, false
	}
	var anterior []verticeMalla
	var out []trianguloMalla

	for len(out) < maxTriangulosMalla {
		fila := make([]verticeMalla, 0, porFila)
		for i := 0; i < porFila; i++ {
			v, ok := p.leerVertice(r)
			if !ok {
				return out, len(out) > 0
			}
			fila = append(fila, v)
		}
		if len(anterior) == porFila {
			// Cada cuadrilátero entre dos filas son dos triángulos.
			for i := 0; i+1 < porFila; i++ {
				out = append(out,
					trianguloMalla{anterior[i], anterior[i+1], fila[i]},
					trianguloMalla{anterior[i+1], fila[i+1], fila[i]})
			}
		}
		anterior = fila
		if len(anterior)*len(out) > maxVerticesMalla*8 {
			break
		}
	}
	return out, len(out) > 0
}

// --- Emisión -----------------------------------------------------------------

// dibujarMalla pinta la malla como triángulos en el lienzo, bajo la
// transformación dada.
//
// Va a `sb` y no a `defs` porque una malla NO es un servidor de pintura: son
// figuras. Cuando hace falta usarla como relleno —un patrón de sombreado—, quien
// llama la envuelve en un `<pattern>`; ver `mallaComoPatron`.
func (l *lienzo) dibujarMalla(tris []trianguloMalla, m matriz, destino *strings.Builder) {
	var b strings.Builder
	b.WriteString(`<g`)
	if !m.esIdentidad() {
		b.WriteString(` transform="` + m.svg() + `"`)
	}
	// `shape-rendering:crispEdges` quita la costura blanca entre triángulos
	// vecinos: con antialiasing, dos bordes que comparten arista dejan ver el
	// fondo por una línea de medio píxel, y una malla entera se ve cuarteada.
	b.WriteString(` shape-rendering="crispEdges">`)
	for _, t := range tris {
		subdividirYEmitir(&b, t, 0)
	}
	b.WriteString("</g>\n")
	destino.WriteString(b.String())
}

// subdividirYEmitir parte el triángulo por los puntos medios hasta el nivel
// acordado y rellena cada trocito con el color medio de sus esquinas.
func subdividirYEmitir(b *strings.Builder, t trianguloMalla, nivel int) {
	if nivel < nivelesSubdivision && vale(t) {
		ab := puntoMedio(t.a, t.b)
		bc := puntoMedio(t.b, t.c)
		ca := puntoMedio(t.c, t.a)
		subdividirYEmitir(b, trianguloMalla{t.a, ab, ca}, nivel+1)
		subdividirYEmitir(b, trianguloMalla{ab, t.b, bc}, nivel+1)
		subdividirYEmitir(b, trianguloMalla{ca, bc, t.c}, nivel+1)
		subdividirYEmitir(b, trianguloMalla{ab, bc, ca}, nivel+1)
		return
	}
	b.WriteString(`<path d="M` + num(t.a.x) + " " + num(t.a.y) +
		"L" + num(t.b.x) + " " + num(t.b.y) +
		"L" + num(t.c.x) + " " + num(t.c.y) +
		`Z" fill="` + mediaDeColores(t) + `"/>`)
}

// vale dice si merece la pena seguir partiendo: un triángulo diminuto o con las
// tres esquinas del mismo color no gana nada.
func vale(t trianguloMalla) bool {
	if t.a.css == t.b.css && t.b.css == t.c.css {
		return false
	}
	lado := math.Max(
		math.Hypot(t.b.x-t.a.x, t.b.y-t.a.y),
		math.Max(
			math.Hypot(t.c.x-t.b.x, t.c.y-t.b.y),
			math.Hypot(t.a.x-t.c.x, t.a.y-t.c.y),
		),
	)
	return lado > subdivisionMinima
}

func puntoMedio(p, q verticeMalla) verticeMalla {
	return verticeMalla{
		x:   (p.x + q.x) / 2,
		y:   (p.y + q.y) / 2,
		css: mezclarCSS(p.css, q.css),
	}
}

// mezclarCSS promedia dos colores hexadecimales.
func mezclarCSS(a, b string) string {
	ra, ga, ba := desdeCSS(a)
	rb, gb, bb := desdeCSS(b)
	return cssRGB(
		float64(int(ra)+int(rb))/2/255,
		float64(int(ga)+int(gb))/2/255,
		float64(int(ba)+int(bb))/2/255,
	)
}

func mediaDeColores(t trianguloMalla) string {
	r1, g1, b1 := desdeCSS(t.a.css)
	r2, g2, b2 := desdeCSS(t.b.css)
	r3, g3, b3 := desdeCSS(t.c.css)
	return cssRGB(
		float64(int(r1)+int(r2)+int(r3))/3/255,
		float64(int(g1)+int(g2)+int(g3))/3/255,
		float64(int(b1)+int(b2)+int(b3))/3/255,
	)
}

// mallaComoPatron envuelve la malla en un `<pattern>` para poder usarla como
// RELLENO, que es lo que pide un patrón de sombreado (PatternType 2 con una
// malla dentro).
//
// El mosaico se declara enorme a propósito: una malla no se repite, se pinta una
// vez donde está. Con un paso del tamaño del lienzo no llega a repetirse nunca y
// se consigue con la única primitiva de SVG que acepta figuras como pintura.
func (l *lienzo) mallaComoPatron(tris []trianguloMalla, m matriz) (string, bool) {
	if len(tris) == 0 {
		return "", false
	}
	id := l.id("malla")
	var b strings.Builder
	b.WriteString(`<pattern id="` + id + `" patternUnits="userSpaceOnUse" ` +
		`x="-100000" y="-100000" width="200000" height="200000">`)
	var dentro strings.Builder
	l.dibujarMalla(tris, matriz{1, 0, 0, 1, 100000, 100000}.por(m), &dentro)
	b.WriteString(dentro.String())
	b.WriteString("</pattern>")
	l.defsEscribir(b.String())
	return id, true
}

// mallaDeSombreado devuelve los triángulos de un sombreado si es de malla.
//
// Es la ÚNICA puerta: quien pinta un sombreado pregunta aquí primero y, si dice
// que no, sigue por el camino de los degradados. Así el conocimiento de qué tipos
// son malla vive en un solo sitio.
func (l *lienzo) mallaDeSombreado(sh pdfObj) ([]trianguloMalla, bool) {
	// Los datos de una malla van en el CUERPO del flujo, así que un sombreado
	// declarado como diccionario suelto no puede ser de malla.
	st, ok := sh.(*pdfStream)
	if !ok {
		return nil, false
	}
	tipo, _ := comoEntero(l.d.resolver(st.Dict["ShadingType"]))
	switch tipo {
	case 4, 5:
		return l.trianguloDeMalla(st, int(tipo))
	case 6, 7:
		return l.parchesDeMalla(st, int(tipo))
	}
	return nil, false
}

// --- Parches Coons y tensor (tipos 6 y 7) ------------------------------------

// pasosDelParche es en cuántos trozos se divide cada lado del parche al
// triangularlo. Ocho por lado son 128 triángulos por parche: suficiente para que
// una curva de Bézier no se vea poligonal y poco para que una portada con veinte
// parches siga siendo un SVG manejable.
const pasosDelParche = 8

// maxParches acota cuántos se leen de un flujo.
const maxParches = 2000

// parchesDeMalla decodifica los tipos 6 (Coons) y 7 (tensor).
//
// LA DIFERENCIA ENTRE LOS DOS ES CASI NADA para lo que aquí se hace: el tensor
// trae cuatro puntos de control INTERIORES además de los doce del borde. Esos
// cuatro solo curvan el interior de la superficie, así que se leen y se
// descartan; el borde —que es lo que se ve— es idéntico.
//
// Un parche son cuatro curvas de Bézier cúbicas encadenadas y un color en cada
// esquina. Se evalúa la superficie de Coons en una rejilla y se triangula: con
// eso el degradado sigue las curvas del borde en vez de recortarlas en recto.
func (l *lienzo) parchesDeMalla(st *pdfStream, tipo int) ([]trianguloMalla, bool) {
	datos, err := l.d.datosDeFlujo(st)
	if err != nil {
		return nil, false
	}
	p, ok := l.parametrosDeMalla(st.Dict)
	if !ok || p.bitsBandera < 1 || p.bitsBandera > 32 {
		return nil, false
	}
	puntosPorParche := 12
	if tipo == 7 {
		puntosPorParche = 16
	}

	r := &lectorDeBits{datos: datos}
	var out []trianguloMalla
	// El parche anterior hace falta porque una bandera distinta de 0 REUTILIZA uno
	// de sus bordes y dos de sus colores: así se encadenan sin repetir datos.
	var previo *parcheCoons

	for len(out) < maxTriangulosMalla && len(out)/(pasosDelParche*pasosDelParche*2) < maxParches {
		if r.agotado() {
			break
		}
		bandera, ok := r.leer(p.bitsBandera)
		if !ok {
			break
		}
		// Con bandera 0 vienen los 12 (o 16) puntos y los 4 colores; con 1, 2 o 3
		// se heredan un borde y dos colores del parche anterior, así que solo
		// llegan 8 (o 12) puntos y 2 colores.
		nuevos, colores := puntosPorParche, 4
		if bandera != 0 && previo != nil {
			nuevos, colores = puntosPorParche-4, 2
		}

		pts := make([][2]float64, 0, nuevos)
		fallo := false
		for i := 0; i < nuevos; i++ {
			x, ok1 := r.leer(p.bitsCoord)
			y, ok2 := r.leer(p.bitsCoord)
			if !ok1 || !ok2 {
				fallo = true
				break
			}
			maxCoord := math.Pow(2, float64(p.bitsCoord)) - 1
			pts = append(pts, [2]float64{
				p.decode[0] + float64(x)/maxCoord*(p.decode[1]-p.decode[0]),
				p.decode[2] + float64(y)/maxCoord*(p.decode[3]-p.decode[2]),
			})
		}
		if fallo {
			break
		}
		cols := make([]string, 0, colores)
		for i := 0; i < colores; i++ {
			css, ok := p.leerColor(r)
			if !ok {
				fallo = true
				break
			}
			cols = append(cols, css)
		}
		if fallo {
			break
		}
		r.alinearAByte()

		parche, ok := ensamblarParche(bandera, pts, cols, previo)
		if !ok {
			break
		}
		out = append(out, parche.triangular()...)
		copia := *parche
		previo = &copia
	}
	return out, len(out) > 0
}

// leerColor saca solo el color de un vértice, sin coordenadas. Los parches los
// traen aparte de los puntos, al contrario que las mallas de triángulos.
func (p parametrosMalla) leerColor(r *lectorDeBits) (string, bool) {
	maxComp := math.Pow(2, float64(p.bitsComp)) - 1
	comps := make([]float64, 0, p.comps)
	for i := 0; i < p.comps; i++ {
		b, ok := r.leer(p.bitsComp)
		if !ok {
			return "", false
		}
		lo, hi := p.decode[4+2*i], p.decode[5+2*i]
		comps = append(comps, lo+float64(b)/maxComp*(hi-lo))
	}
	if p.fn != nil {
		comps = p.fn.evaluar(comps[0])
	}
	return p.ec.css(comps), true
}

// parcheCoons es un parche listo para triangular: los doce puntos del borde en
// el orden de la especificación y el color de las cuatro esquinas.
type parcheCoons struct {
	// borde son los 12 puntos de control, empezando en la esquina 1 y dando la
	// vuelta (§8.7.4.5.7, figura 46).
	borde [12][2]float64
	// colores son las esquinas 1 a 4, en el mismo sentido que el borde.
	colores [4]string
}

// ensamblarParche coloca los puntos y colores nuevos en su sitio, heredando del
// anterior lo que la bandera diga.
//
// LAS BANDERAS SON EL SITIO DONDE ESTO SE ROMPE EN SILENCIO. Cada una dice qué
// BORDE del parche previo se reutiliza como primer borde del nuevo, y por tanto
// qué dos colores se heredan. Equivocarse no da error: encaja los parches por el
// lado equivocado y el degradado sale retorcido.
func ensamblarParche(bandera uint64, pts [][2]float64, cols []string, previo *parcheCoons) (*parcheCoons, bool) {
	p := &parcheCoons{}
	if bandera == 0 || previo == nil {
		if len(pts) < 12 || len(cols) < 4 {
			return nil, false
		}
		copy(p.borde[:], pts[:12])
		copy(p.colores[:], cols[:4])
		return p, true
	}
	if len(pts) < 8 || len(cols) < 2 {
		return nil, false
	}

	// El borde heredado según la bandera, con sus dos colores. Los índices salen
	// de la tabla 85 de la especificación.
	var heredado [4][2]float64
	var c1, c2 string
	switch bandera {
	case 1:
		heredado = [4][2]float64{previo.borde[3], previo.borde[4], previo.borde[5], previo.borde[6]}
		c1, c2 = previo.colores[1], previo.colores[2]
	case 2:
		heredado = [4][2]float64{previo.borde[6], previo.borde[7], previo.borde[8], previo.borde[9]}
		c1, c2 = previo.colores[2], previo.colores[3]
	default: // 3
		heredado = [4][2]float64{previo.borde[9], previo.borde[10], previo.borde[11], previo.borde[0]}
		c1, c2 = previo.colores[3], previo.colores[0]
	}
	p.borde[0], p.borde[1], p.borde[2], p.borde[3] = heredado[0], heredado[1], heredado[2], heredado[3]
	copy(p.borde[4:], pts[:8])
	p.colores[0], p.colores[1] = c1, c2
	p.colores[2], p.colores[3] = cols[0], cols[1]
	return p, true
}

// triangular evalúa la superficie de Coons en una rejilla y devuelve triángulos.
func (p *parcheCoons) triangular() []trianguloMalla {
	n := pasosDelParche
	rejilla := make([][]verticeMalla, n+1)
	for i := 0; i <= n; i++ {
		rejilla[i] = make([]verticeMalla, n+1)
		u := float64(i) / float64(n)
		for j := 0; j <= n; j++ {
			v := float64(j) / float64(n)
			x, y := p.superficie(u, v)
			rejilla[i][j] = verticeMalla{x: x, y: y, css: p.colorEn(u, v)}
		}
	}

	out := make([]trianguloMalla, 0, n*n*2)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			out = append(out,
				trianguloMalla{rejilla[i][j], rejilla[i+1][j], rejilla[i][j+1]},
				trianguloMalla{rejilla[i+1][j], rejilla[i+1][j+1], rejilla[i][j+1]})
		}
	}
	return out
}

// superficie evalúa el parche de Coons en (u,v).
//
// La fórmula (§8.7.4.5.7) es la suma de las dos interpolaciones lineales entre
// bordes opuestos MENOS la interpolación bilineal de las cuatro esquinas. Ese
// último término es el que se resta para no contar dos veces las esquinas; sin
// él la superficie se abomba.
func (p *parcheCoons) superficie(u, v float64) (float64, float64) {
	// Los cuatro bordes como curvas de Bézier cúbicas, en el orden del borde.
	c1 := bezier(p.borde[0], p.borde[1], p.borde[2], p.borde[3], v)   // esquina 1 → 2
	c2 := bezier(p.borde[3], p.borde[4], p.borde[5], p.borde[6], u)   // 2 → 3
	c3 := bezier(p.borde[6], p.borde[7], p.borde[8], p.borde[9], 1-v) // 3 → 4
	c4 := bezier(p.borde[9], p.borde[10], p.borde[11], p.borde[0], 1-u)

	e1, e2, e3, e4 := p.borde[0], p.borde[3], p.borde[6], p.borde[9]
	coord := func(k int) float64 {
		lineal1 := (1-u)*c1[k] + u*c3[k]
		lineal2 := (1-v)*c4[k] + v*c2[k]
		bilineal := (1-u)*(1-v)*e1[k] + (1-u)*v*e2[k] + u*v*e3[k] + u*(1-v)*e4[k]
		return lineal1 + lineal2 - bilineal
	}
	return coord(0), coord(1)
}

// colorEn interpola bilinealmente los cuatro colores de las esquinas.
func (p *parcheCoons) colorEn(u, v float64) string {
	arriba := mezclarPeso(p.colores[0], p.colores[1], v)
	abajo := mezclarPeso(p.colores[3], p.colores[2], v)
	return mezclarPeso(arriba, abajo, u)
}

// mezclarPeso interpola dos colores con peso `t` en el segundo.
func mezclarPeso(a, b string, t float64) string {
	ra, ga, ba := desdeCSS(a)
	rb, gb, bb := desdeCSS(b)
	lerp := func(x, y byte) float64 {
		return (float64(x)*(1-t) + float64(y)*t) / 255
	}
	return cssRGB(lerp(ra, rb), lerp(ga, gb), lerp(ba, bb))
}

// bezier evalúa una cúbica en `t`.
func bezier(p0, p1, p2, p3 [2]float64, t float64) [2]float64 {
	s := 1 - t
	a, b, c, d := s*s*s, 3*s*s*t, 3*s*t*t, t*t*t
	return [2]float64{
		a*p0[0] + b*p1[0] + c*p2[0] + d*p3[0],
		a*p0[1] + b*p1[1] + c*p2[1] + d*p3[1],
	}
}
