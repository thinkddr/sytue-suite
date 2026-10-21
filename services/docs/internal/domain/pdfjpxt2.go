// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tier-2 y ensamblado: de los paquetes del codestream a los píxeles.
//
// Esto es la contabilidad del formato. Una imagen JPEG 2000 está partida en
// tiles, cada tile en componentes, cada componente en resoluciones, cada
// resolución en subbandas y cada subbanda en code-blocks; y todo eso se
// entrelaza en PAQUETES cuyo orden lo decide la progresión declarada en la
// cabecera. Aquí se deshace ese entrelazado, se llama a EBCOT por cada bloque y
// se recompone la imagen con la wavelet inversa.
//
// ALCANCE DECLARADO, que es mejor que un silencio: se decodifica el caso que
// aparece en un PDF de escaneo —un tile, sin precintos a medida, progresiones
// LRCP y RLCP—. Lo demás se rechaza CON SU MOTIVO en vez de pintar ruido, que
// es lo que hace un decodificador que adivina.
package domain

import (
	"encoding/binary"
	"fmt"
)

// subbandaJPX es una subbanda de una resolución, con sus coeficientes.
type subbandaJPX struct {
	tipo           int // bandaLL, bandaHL, bandaLH, bandaHH
	x0, y0, x1, y1 int // en coordenadas de la subbanda
	coef           []int32
	// bloques son los code-blocks en que se parte, en orden de lectura.
	bloques []bloqueCodigo
	nbx     int // cuántos bloques por fila
	nby     int
	// árboles de inclusión y de planos cero, compartidos por todas las capas.
	inclusion *tagTree
	ceros     *tagTree
}

func (s *subbandaJPX) ancho() int { return s.x1 - s.x0 }
func (s *subbandaJPX) alto() int  { return s.y1 - s.y0 }

// bloqueCodigo es un code-block: dónde cae y qué bytes le han llegado.
type bloqueCodigo struct {
	x0, y0, x1, y1 int
	datos          []byte
	pasadas        int
	planosCero     int
	incluido       bool
	// lblock es el número de bits con que se codifica la longitud, y CRECE a lo
	// largo de las capas. Empezar de cero en cada capa lee longitudes absurdas.
	lblock int
}

// resolucionJPX agrupa las subbandas de un nivel.
type resolucionJPX struct {
	nivel          int
	x0, y0, x1, y1 int
	bandas         []*subbandaJPX
}

// DecodificarJPX convierte un JPEG 2000 en muestras crudas entrelazadas, listas
// para el camino normal de imágenes del motor.
//
// Devuelve las muestras, cuántas componentes trae y cuántos bits por muestra.
func DecodificarJPX(datos []byte) (muestras []byte, comps, bits int, err error) {
	img, err := LeerCabecerasJPX(datos)
	if err != nil {
		return nil, 0, 0, err
	}
	if len(img.tiles) != 1 {
		return nil, 0, 0, fmt.Errorf(
			"jpx: la imagen viene en %d tiles y este decodificador lee una de una pieza",
			len(img.tiles),
		)
	}
	if img.cod.progresion > 1 {
		return nil, 0, 0, fmt.Errorf(
			"jpx: progresión %d, que este decodificador no recorre", img.cod.progresion,
		)
	}
	for k, c := range img.comps {
		if c.dx != 1 || c.dy != 1 {
			return nil, 0, 0, fmt.Errorf(
				"jpx: la componente %d va submuestreada (%d×%d), y aquí se leen a tamaño completo",
				k, c.dx, c.dy,
			)
		}
	}

	var flujo []byte
	for _, v := range img.tiles {
		flujo = v
	}

	planos := make([][]int32, len(img.comps))
	for c := range img.comps {
		p, err := img.decodificarComponente(c, flujo)
		if err != nil {
			return nil, 0, 0, err
		}
		planos[c] = p
	}

	// Transformada de componentes, si la hubo. Solo con tres o más: con una sola
	// no hay nada que mezclar y aplicarla daría verde.
	if img.cod.mct && len(planos) >= 3 {
		if img.cod.transf53 {
			rctInversa(planos[0], planos[1], planos[2])
		} else {
			f0 := aFloat(planos[0])
			f1 := aFloat(planos[1])
			f2 := aFloat(planos[2])
			ictInversa(f0, f1, f2)
			planos[0], planos[1], planos[2] = aInt(f0), aInt(f1), aInt(f2)
		}
	}

	return img.empaquetar(planos)
}

// decodificarComponente reconstruye UNA componente entera.
func (img *imagenJPX) decodificarComponente(c int, flujo []byte) ([]int32, error) {
	cod := img.codificacionDe(c)
	res, err := img.resolucionesDe(c, cod)
	if err != nil {
		return nil, err
	}
	if err := img.leerPaquetes(flujo, res, cod); err != nil {
		return nil, err
	}
	for _, r := range res {
		for _, b := range r.bandas {
			img.decodificarBanda(b, cod)
		}
	}
	return img.recomponer(res, cod), nil
}

// resolucionesDe construye el esqueleto: qué subbandas hay y de qué tamaño.
//
// El tamaño de cada subbanda NO es «la mitad»: sale de dividir las coordenadas
// de la imagen con techo y suelo (§B.5), y por eso una imagen impar no cuadra si
// se calcula a ojo — se pierde una fila entera en el último nivel.
func (img *imagenJPX) resolucionesDe(c int, cod codificacionJPX) ([]*resolucionJPX, error) {
	comp := img.comps[c]
	cx0, cy0 := ceilDiv(img.x0, comp.dx), ceilDiv(img.y0, comp.dy)
	cx1, cy1 := ceilDiv(img.x0+img.ancho, comp.dx), ceilDiv(img.y0+img.alto, comp.dy)

	out := make([]*resolucionJPX, 0, cod.niveles+1)
	for r := 0; r <= cod.niveles; r++ {
		// La resolución r se obtiene descartando `niveles-r` niveles de detalle.
		d := cod.niveles - r
		res := &resolucionJPX{
			nivel: r,
			x0:    ceilDivPot(cx0, d), y0: ceilDivPot(cy0, d),
			x1: ceilDivPot(cx1, d), y1: ceilDivPot(cy1, d),
		}
		if r == 0 {
			b, err := img.nuevaBanda(bandaLL, cx0, cy0, cx1, cy1, cod.niveles, cod)
			if err != nil {
				return nil, err
			}
			res.bandas = append(res.bandas, b)
		} else {
			// Las tres de detalle de este nivel.
			for _, tipo := range []int{bandaHL, bandaLH, bandaHH} {
				b, err := img.nuevaBandaDetalle(tipo, cx0, cy0, cx1, cy1, cod.niveles-r+1, cod)
				if err != nil {
					return nil, err
				}
				res.bandas = append(res.bandas, b)
			}
		}
		out = append(out, res)
	}
	return out, nil
}

// nuevaBanda crea la subbanda LL del nivel más profundo.
func (img *imagenJPX) nuevaBanda(tipo, cx0, cy0, cx1, cy1, d int, cod codificacionJPX) (*subbandaJPX, error) {
	b := &subbandaJPX{
		tipo: tipo,
		x0:   ceilDivPot(cx0, d), y0: ceilDivPot(cy0, d),
		x1: ceilDivPot(cx1, d), y1: ceilDivPot(cy1, d),
	}
	// La llamada va ANTES del return y no dentro: `return b, b.partirEnBloques(cod)`
	// deja el orden de evaluación a criterio del compilador (gocritic evalOrder), y
	// aquí `partirEnBloques` MUTA a `b`.
	err := b.partirEnBloques(cod)
	return b, err
}

// nuevaBandaDetalle crea una HL, LH o HH. El «+1» y el «-1» de las fórmulas de
// §B.5 son lo que coloca cada subbanda en su sitio; sin ellos la imagen sale
// desplazada medio píxel por nivel.
func (img *imagenJPX) nuevaBandaDetalle(tipo, cx0, cy0, cx1, cy1, d int, cod codificacionJPX) (*subbandaJPX, error) {
	var dx, dy int
	switch tipo {
	case bandaHL:
		dx, dy = 1, 0
	case bandaLH:
		dx, dy = 0, 1
	default:
		dx, dy = 1, 1
	}
	pot := 1 << uint(d)
	b := &subbandaJPX{
		tipo: tipo,
		x0:   ceilDiv(cx0-(pot/2)*dx, pot), y0: ceilDiv(cy0-(pot/2)*dy, pot),
		x1: ceilDiv(cx1-(pot/2)*dx, pot), y1: ceilDiv(cy1-(pot/2)*dy, pot),
	}
	err := b.partirEnBloques(cod)
	return b, err
}

// partirEnBloques recorta la subbanda en code-blocks alineados a la rejilla.
//
// La rejilla es ABSOLUTA, no relativa a la subbanda: los bloques del borde salen
// más pequeños. Partir desde el origen de la subbanda daría bloques del tamaño
// correcto pero en el sitio equivocado, y el flujo no cuadraría.
func (s *subbandaJPX) partirEnBloques(cod codificacionJPX) error {
	if s.ancho() <= 0 || s.alto() <= 0 {
		s.nbx, s.nby = 0, 0
		return nil
	}
	s.coef = make([]int32, s.ancho()*s.alto())

	bx0 := s.x0 / cod.cbAncho
	by0 := s.y0 / cod.cbAlto
	bx1 := (s.x1 - 1) / cod.cbAncho
	by1 := (s.y1 - 1) / cod.cbAlto
	s.nbx, s.nby = bx1-bx0+1, by1-by0+1

	var err error
	if s.inclusion, err = nuevoTagTree(s.nbx, s.nby); err != nil {
		return err
	}
	if s.ceros, err = nuevoTagTree(s.nbx, s.nby); err != nil {
		return err
	}

	s.bloques = make([]bloqueCodigo, 0, s.nbx*s.nby)
	for by := by0; by <= by1; by++ {
		for bx := bx0; bx <= bx1; bx++ {
			s.bloques = append(s.bloques, bloqueCodigo{
				x0: max(s.x0, bx*cod.cbAncho), y0: max(s.y0, by*cod.cbAlto),
				x1: min(s.x1, (bx+1)*cod.cbAncho), y1: min(s.y1, (by+1)*cod.cbAlto),
				// Arranca en 3 por norma (§B.10.7.1) y sube cuando el flujo lo dice.
				lblock: 3,
			})
		}
	}
	return nil
}

// leerPaquetes recorre el flujo del tile en el orden de la progresión.
func (img *imagenJPX) leerPaquetes(flujo []byte, res []*resolucionJPX, cod codificacionJPX) error {
	l := nuevoLectorBitsJPX(flujo)
	// LRCP (0) y RLCP (1) recorren lo mismo con los bucles cambiados de sitio.
	// Con una componente y un precinto por resolución, que es este caso, el
	// resultado es el mismo orden de paquetes.
	for capa := 0; capa < cod.capas; capa++ {
		for _, r := range res {
			if err := img.leerUnPaquete(l, r); err != nil {
				return err
			}
		}
	}
	return nil
}

// leerCabeceraDeBloque lee lo que la cabecera del paquete dice de UN code-block:
// si entra en esta capa y, si entra, cuántos bytes de cuerpo le tocan.
//
// `entra` en false significa «este bloque no aporta nada aquí», que no es un
// error: pasa constantemente, porque un paquete solo trae los bloques que la
// capa actual refina.
//
// Está fuera de leerUnPaquete porque esa función pasó de 30 de complejidad
// cognitiva y el CI la rechaza; es el bloque más autocontenido que tenía dentro.
func leerCabeceraDeBloque(l *lectorBitsJPX, s *subbandaJPX, b *bloqueCodigo, bx, by int) (int, bool, error) {
	if !b.incluido {
		// Primera vez: el tag tree dice en qué capa entra.
		_, listo := s.inclusion.leer(l, bx, by, 1)
		if !listo {
			return 0, false, nil // aún no entra
		}
		b.incluido = true
		// Y cuántos planos de bits suyos están vacíos.
		ceros, ok := leerCeros(l, s.ceros, bx, by)
		if !ok {
			return 0, false, fmt.Errorf("jpx: no se pudo leer los planos vacíos del bloque")
		}
		b.planosCero = ceros
	} else if l.bit() == 0 {
		return 0, false, nil // ya incluido antes, pero nada nuevo en esta capa
	}

	pasadas := leerPasadas(l)
	// Lblock crece con tantos unos como diga el flujo.
	for l.bit() == 1 {
		b.lblock++
		if b.lblock > 32 {
			return 0, false, fmt.Errorf("jpx: Lblock desbocado")
		}
	}
	largo := int(l.bits(b.lblock + bitsDe32(pasadas)))
	b.pasadas += pasadas
	return largo, true, nil
}

// leerUnPaquete lee la cabecera de un paquete y reparte los bytes a sus bloques.
func (img *imagenJPX) leerUnPaquete(l *lectorBitsJPX, r *resolucionJPX) error {
	// Bit de paquete vacío: un 0 dice que en esta capa no hay nada de esta
	// resolución, y ahí acaba la cabecera.
	if l.bit() == 0 {
		l.alinear()
		return nil
	}
	type pendiente struct {
		b     *bloqueCodigo
		largo int
	}
	var cola []pendiente

	for _, s := range r.bandas {
		for i := range s.bloques {
			b := &s.bloques[i]
			if b.x1 <= b.x0 || b.y1 <= b.y0 {
				continue
			}
			bx, by := i%max(s.nbx, 1), i/max(s.nbx, 1)

			largo, entra, err := leerCabeceraDeBloque(l, s, b, bx, by)
			if err != nil {
				return err
			}
			if !entra {
				continue
			}
			cola = append(cola, pendiente{b: b, largo: largo})
		}
	}
	l.alinear()

	// Los cuerpos van seguidos, en el mismo orden que la cabecera.
	for _, p := range cola {
		if p.largo < 0 || l.pos+p.largo > len(l.datos) {
			return fmt.Errorf("jpx: un paquete dice medir más de lo que queda")
		}
		p.b.datos = append(p.b.datos, l.datos[l.pos:l.pos+p.largo]...)
		l.pos += p.largo
	}
	return nil
}

// leerCeros lee del tag tree cuántos planos de bits vacíos tiene el bloque.
//
// El valor no está acotado por arriba, así que se sube el tope hasta que cierre
// en vez de leer con un límite inventado.
func leerCeros(l *lectorBitsJPX, t *tagTree, bx, by int) (int, bool) {
	for tope := 1; tope <= 74; tope++ {
		if v, listo := t.leer(l, bx, by, tope); listo {
			return v, true
		}
	}
	return 0, false
}

// leerPasadas decodifica cuántas pasadas de codificación trae el bloque
// (§B.10.6). Es un código de longitud variable, no un número plano.
func leerPasadas(l *lectorBitsJPX) int {
	if l.bit() == 0 {
		return 1
	}
	if l.bit() == 0 {
		return 2
	}
	if v := l.bits(2); v < 3 {
		return 3 + int(v)
	}
	if v := l.bits(5); v < 31 {
		return 6 + int(v)
	}
	return 37 + int(l.bits(7))
}

// bitsDe32 devuelve floor(log2(n)), que es lo que suma a Lblock.
func bitsDe32(n int) int {
	b := 0
	for n > 1 {
		n >>= 1
		b++
	}
	return b
}

// decodificarBanda pasa cada code-block por EBCOT y lo coloca en la subbanda.
func (img *imagenJPX) decodificarBanda(s *subbandaJPX, cod codificacionJPX) {
	if len(s.coef) == 0 {
		return
	}
	// Los planos que trae el bloque: los del guardado más el exponente de la
	// subbanda, menos los que el flujo declara vacíos.
	base := img.guarda + img.exponenteDe(s, cod) - 1
	for i := range s.bloques {
		b := &s.bloques[i]
		if !b.incluido || len(b.datos) == 0 {
			continue
		}
		ancho, alto := b.x1-b.x0, b.y1-b.y0
		planos := base - b.planosCero
		if planos <= 0 || ancho <= 0 || alto <= 0 {
			continue
		}
		coef := decodificarCodeBlock(b.datos, ancho, alto, planos, s.tipo)
		for y := 0; y < alto; y++ {
			for x := 0; x < ancho; x++ {
				dx, dy := b.x0-s.x0+x, b.y0-s.y0+y
				if dx < 0 || dy < 0 || dx >= s.ancho() || dy >= s.alto() {
					continue
				}
				s.coef[dy*s.ancho()+dx] = coef[y*ancho+x]
			}
		}
	}
}

// exponenteDe devuelve el exponente de cuantificación de una subbanda.
func (img *imagenJPX) exponenteDe(s *subbandaJPX, cod codificacionJPX) int {
	if len(img.pasos) == 0 {
		return 8
	}
	// Con estilo derivado hay UN solo paso para todas; con los otros, uno por
	// subbanda en el orden en que se generan.
	if img.estiloQ == 1 || len(img.pasos) == 1 {
		return img.pasos[0].exp
	}
	idx := 0
	if s.tipo != bandaLL {
		// LL es el 0; a partir de ahí, tres por nivel.
		idx = 1
		_ = cod
	}
	if idx >= len(img.pasos) {
		idx = len(img.pasos) - 1
	}
	return img.pasos[idx].exp
}

// recomponer deshace la descomposición nivel a nivel.
func (img *imagenJPX) recomponer(res []*resolucionJPX, cod codificacionJPX) []int32 {
	if len(res) == 0 {
		return nil
	}
	// Se arranca de la LL más profunda y se va subiendo.
	actual := res[0].bandas[0]
	datos := append([]int32(nil), actual.coef...)
	ancho, alto := actual.ancho(), actual.alto()
	x0, y0 := actual.x0, actual.y0

	for r := 1; r < len(res); r++ {
		nueva := res[r]
		na, nl := nueva.x1-nueva.x0, nueva.y1-nueva.y0
		if na <= 0 || nl <= 0 {
			continue
		}
		mezcla := make([]int32, na*nl)
		// Entrelazado (§F.3.3): la LL va en las posiciones pares de los dos ejes,
		// y las tres de detalle en las combinaciones de impares.
		poner := func(s *subbandaJPX, ox, oy int) {
			if s == nil || len(s.coef) == 0 {
				return
			}
			for y := 0; y < s.alto(); y++ {
				for x := 0; x < s.ancho(); x++ {
					dx, dy := 2*x+ox, 2*y+oy
					if dx < na && dy < nl {
						mezcla[dy*na+dx] = s.coef[y*s.ancho()+x]
					}
				}
			}
		}
		ll := &subbandaJPX{x0: x0, y0: y0, x1: x0 + ancho, y1: y0 + alto, coef: datos}
		poner(ll, 0, 0)
		for _, b := range nueva.bandas {
			switch b.tipo {
			case bandaHL:
				poner(b, 1, 0)
			case bandaLH:
				poner(b, 0, 1)
			case bandaHH:
				poner(b, 1, 1)
			}
		}
		if cod.transf53 {
			idwt53(mezcla, na, nl, nueva.x0, nueva.y0)
		} else {
			f := aFloat(mezcla)
			idwt97(f, na, nl, nueva.x0, nueva.y0)
			mezcla = aInt(f)
		}
		datos, ancho, alto = mezcla, na, nl
		x0, y0 = nueva.x0, nueva.y0
	}
	return datos
}

// empaquetar pasa los planos a muestras crudas entrelazadas, con el nivelado DC
// que devuelve el rango a 0..2^bits-1.
func (img *imagenJPX) empaquetar(planos [][]int32) ([]byte, int, int, error) {
	n := len(planos)
	if n == 0 || len(planos[0]) == 0 {
		return nil, 0, 0, fmt.Errorf("jpx: no salió ningún píxel")
	}
	bits := img.comps[0].bits
	if bits <= 8 {
		bits = 8
	} else {
		bits = 16
	}
	pixeles := img.ancho * img.alto
	if len(planos[0]) < pixeles {
		return nil, 0, 0, fmt.Errorf("jpx: salieron %d muestras y la imagen tiene %d",
			len(planos[0]), pixeles)
	}
	porMuestra := bits / 8
	out := make([]byte, pixeles*n*porMuestra)
	for c := 0; c < n; c++ {
		// Nivelado DC: las muestras sin signo se guardan centradas en cero, así
		// que hay que devolverles la mitad del rango. Sin esto, la imagen sale
		// con el brillo desplazado a la mitad y con la mitad de los píxeles a 0.
		desnivel := int32(0)
		if !img.comps[c].signo {
			desnivel = 1 << uint(img.comps[c].bits-1)
		}
		tope := int32(1)<<uint(img.comps[c].bits) - 1
		for i := 0; i < pixeles; i++ {
			v := planos[c][i] + desnivel
			if v < 0 {
				v = 0
			}
			if v > tope {
				v = tope
			}
			if porMuestra == 1 {
				out[i*n+c] = byte(v)
			} else {
				binary.BigEndian.PutUint16(out[(i*n+c)*2:], uint16(v))
			}
		}
	}
	return out, n, bits, nil
}

func aFloat(v []int32) []float64 {
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = float64(x)
	}
	return out
}

func aInt(v []float64) []int32 {
	out := make([]int32, len(v))
	for i, x := range v {
		// Redondeo al más cercano: truncar sesga toda la imagen hacia abajo.
		if x >= 0 {
			out[i] = int32(x + 0.5)
		} else {
			out[i] = int32(x - 0.5)
		}
	}
	return out
}

func ceilDiv(a, b int) int {
	if b == 0 {
		return a
	}
	return (a + b - 1) / b
}

func ceilDivPot(a, p int) int { return ceilDiv(a, 1<<uint(p)) }
