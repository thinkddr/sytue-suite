// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// EBCOT Tier-1 (ISO 15444-1 §D): decodificar UN code-block.
//
// Aquí es donde JPEG 2000 se gana la fama de complicado, y también donde este
// decodificador sale casi gratis: el descompresor aritmético es el MQ que ya
// estaba escrito para JBIG2 (`pdfjbig2.go`). Lo que falta encima es contabilidad.
//
// Un code-block se decodifica plano de bits a plano de bits, del más
// significativo al menos, y cada plano en TRES pasadas:
//
//  1. propagación de significancia: los coeficientes aún en cero que tienen
//     algún vecino ya significativo — son los que probablemente despierten ahora;
//  2. refinamiento: un bit más de precisión para los que ya eran significativos;
//  3. limpieza: todo lo demás, con una codificación de rachas para las columnas
//     de cuatro que siguen enteras a cero.
//
// El orden importa hasta el último detalle: cada pasada mira el estado que
// dejaron las anteriores, así que confundirlas no da un error sino una imagen
// con la basura bien repartida.
package domain

// Contextos del decodificador (§D.3): 0-8 significancia, 9-13 signo, 14-16
// refinamiento, 17 racha y 18 uniforme.
const (
	ctxRacha       = 17
	ctxUniforme    = 18
	numContextosT1 = 19
)

// Banderas de estado de cada coeficiente.
const (
	bitSignificativo = 1 << 0
	bitVisitado      = 1 << 1 // ya tratado en la pasada de significancia de ESTE plano
	bitRefinado      = 1 << 2 // ya ha pasado por refinamiento alguna vez
	bitSigno         = 1 << 3 // negativo
)

// bloqueT1 es el estado de un code-block mientras se decodifica.
type bloqueT1 struct {
	ancho, alto int
	// magnitud y banderas van en rejillas con UN borde de una celda alrededor,
	// para que mirar a los ocho vecinos no tenga que comprobar los límites en
	// cada acceso —que es el bucle más caliente de todo el códec—.
	mag  []int32
	flag []uint8
	paso int // anchura de la rejilla con borde
}

func nuevoBloqueT1(ancho, alto int) *bloqueT1 {
	paso := ancho + 2
	return &bloqueT1{
		ancho: ancho, alto: alto, paso: paso,
		mag:  make([]int32, paso*(alto+2)),
		flag: make([]uint8, paso*(alto+2)),
	}
}

// idx da la posición de (x, y) dentro de la rejilla con borde.
func (b *bloqueT1) idx(x, y int) int { return (y+1)*b.paso + (x + 1) }

// Orientaciones de subbanda, que cambian la tabla de contextos: una subbanda HL
// tiene los bordes verticales y la LH los horizontales, así que lo que predice
// que un vecino despierte no es lo mismo.
const (
	bandaLL = 0
	bandaHL = 1
	bandaLH = 2
	bandaHH = 3
)

// contextoSignificancia devuelve el contexto (0-8) según cuántos vecinos
// significativos hay y en qué dirección, para la orientación de la subbanda.
//
// Es la tabla D.1 de la norma escrita como código. Se cuenta aparte horizontal,
// vertical y diagonal porque de eso va: un coeficiente rodeado en horizontal es
// mucho más probable que despierte que uno con dos vecinos en diagonal.
// contextoHH es la mitad de la tabla §D.1 que corresponde a la subbanda HH,
// donde manda la DIAGONAL: ahí el detalle es diagonal por construcción, así que
// los vecinos en cruz solo desempatan. Está aparte porque la tabla completa pasa
// de 20 de complejidad ciclomática y el CI la rechaza; son dos mitades sin nada
// en común salvo el nombre.
func contextoHH(hv, d int) int {
	switch {
	case d >= 3:
		return 8
	case d == 2 && hv >= 1:
		return 7
	case d == 2:
		return 6
	case d == 1 && hv >= 2:
		return 5
	case d == 1 && hv == 1:
		return 4
	case d == 1:
		return 3
	case hv >= 2:
		return 2
	case hv == 1:
		return 1
	}
	return 0
}

func (b *bloqueT1) contextoSignificancia(x, y, banda int) int {
	sig := func(dx, dy int) int {
		if b.flag[b.idx(x+dx, y+dy)]&bitSignificativo != 0 {
			return 1
		}
		return 0
	}
	h := sig(-1, 0) + sig(1, 0)
	v := sig(0, -1) + sig(0, 1)
	d := sig(-1, -1) + sig(1, -1) + sig(-1, 1) + sig(1, 1)

	// En HL se intercambian los papeles de horizontal y vertical: la subbanda
	// guarda detalle vertical, así que la correlación va por el otro eje.
	if banda == bandaHL {
		h, v = v, h
	}
	if banda == bandaHH {
		return contextoHH(h+v, d)
	}
	switch {
	case h == 2:
		return 8
	case h == 1 && v >= 1:
		return 7
	case h == 1 && d >= 1:
		return 6
	case h == 1:
		return 5
	case v == 2:
		return 4
	case v == 1:
		return 3
	case d >= 2:
		return 2
	case d == 1:
		return 1
	}
	return 0
}

// contextoDelSigno devuelve el contexto (10-13) y si el bit leído hay que
// invertirlo (§D.3.2).
//
// El signo se predice por los vecinos de arriba/abajo e izquierda/derecha: dos
// coeficientes contiguos de un borde suelen tener el mismo signo. Cuando la
// predicción dice «negativo», se codifica el bit AL REVÉS, y por eso hace falta
// devolver también el XOR.
func (b *bloqueT1) contextoDelSigno(x, y int) (int, uint8) {
	contrib := func(dx, dy int) int {
		f := b.flag[b.idx(x+dx, y+dy)]
		if f&bitSignificativo == 0 {
			return 0
		}
		if f&bitSigno != 0 {
			return -1
		}
		return 1
	}
	h := recorta1(contrib(-1, 0) + contrib(1, 0))
	v := recorta1(contrib(0, -1) + contrib(0, 1))
	return tablaSigno[(h+1)*3+(v+1)].ctx, tablaSigno[(h+1)*3+(v+1)].xor
}

// recorta1 lleva la suma de dos vecinos al rango {-1, 0, 1} de la tabla D.3.
func recorta1(n int) int {
	switch {
	case n > 1:
		return 1
	case n < -1:
		return -1
	}
	return n
}

// tablaSigno es la tabla D.3 —(h, v) → contexto y XOR— indexada por
// `(h+1)*3 + (v+1)`, con h y v ya recortados a {-1, 0, 1}.
//
// Se escribe como TABLA porque en la norma es una tabla: el `switch` equivalente
// eran nueve ramas encadenadas, pasaba de 20 de complejidad ciclomática —el CI lo
// rechaza— y además obligaba a leer nueve condiciones para comprobar una casilla.
var tablaSigno = [9]struct {
	ctx int
	xor uint8
}{
	{13, 1}, {12, 1}, {11, 1}, // h = -1, con v = -1, 0, +1
	{10, 1}, {9, 0}, {10, 0}, // h =  0
	{11, 0}, {12, 0}, {13, 0}, // h = +1
}

// contextoRefinamiento devuelve 14, 15 o 16 (§D.3.3).
//
// El primer refinamiento de un coeficiente se codifica distinto del resto, y
// además mira si tiene vecinos: un bit de precisión sobre algo aislado no se
// parece al de algo dentro de un borde.
func (b *bloqueT1) contextoRefinamiento(x, y int) int {
	f := b.flag[b.idx(x, y)]
	if f&bitRefinado != 0 {
		return 16
	}
	// ¿Algún vecino significativo?
	for _, d := range [8][2]int{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}} {
		if b.flag[b.idx(x+d[0], y+d[1])]&bitSignificativo != 0 {
			return 15
		}
	}
	return 14
}

// marcarSignificativo enciende el bit y apunta el signo.
func (b *bloqueT1) marcarSignificativo(x, y int, negativo bool) {
	f := bitSignificativo
	if negativo {
		f |= bitSigno
	}
	b.flag[b.idx(x, y)] |= uint8(f)
}

// decodificarCodeBlock decodifica un code-block entero y devuelve sus
// coeficientes con signo.
//
// `planos` es cuántos planos de bits trae; `banda` la orientación de la subbanda.
// El recorrido es por BANDAS DE CUATRO FILAS —no fila a fila—, que es como está
// escrito el flujo: recorrerlo en el orden natural leería los bits de otro sitio.
func decodificarCodeBlock(datos []byte, ancho, alto, planos, banda int) []int32 {
	b := nuevoBloqueT1(ancho, alto)
	if ancho <= 0 || alto <= 0 || planos <= 0 {
		return nil
	}
	mq := nuevoMQ(datos)
	cx := make([]contextoMQ, numContextosT1)
	// Estados iniciales de la norma (tabla D.7): la mayoría a cero, pero el de
	// racha, el uniforme y el contexto 0 arrancan sesgados. Sin esto los primeros
	// símbolos salen mal y el bloque entero se desvía.
	cx[0] = contextoMQ{i: 4}
	cx[ctxRacha] = contextoMQ{i: 3}
	cx[ctxUniforme] = contextoMQ{i: 46}

	// El primer plano solo lleva pasada de limpieza: no hay nada significativo
	// todavía, así que las otras dos no tendrían a quién mirar.
	for p := 0; p < planos; p++ {
		if p > 0 {
			b.pasadaSignificancia(mq, cx, banda)
			b.pasadaRefinamiento(mq, cx)
		}
		b.pasadaLimpieza(mq, cx, banda)
		b.desplazar()
	}

	out := make([]int32, ancho*alto)
	for y := 0; y < alto; y++ {
		for x := 0; x < ancho; x++ {
			v := b.mag[b.idx(x, y)]
			if b.flag[b.idx(x, y)]&bitSigno != 0 {
				v = -v
			}
			out[y*ancho+x] = v
		}
	}
	return out
}

// desplazar deja hueco para el siguiente plano y limpia las marcas de «visitado»,
// que valen solo dentro de un plano.
func (b *bloqueT1) desplazar() {
	for i := range b.mag {
		b.mag[i] <<= 1
	}
	for i := range b.flag {
		b.flag[i] &^= bitVisitado
	}
}

// pasadaSignificancia (§D.3.1): los que aún son cero pero tienen vecinos.
func (b *bloqueT1) pasadaSignificancia(mq *decodificadorMQ, cx []contextoMQ, banda int) {
	b.porBandas(func(x, y int) {
		i := b.idx(x, y)
		if b.flag[i]&bitSignificativo != 0 {
			return
		}
		c := b.contextoSignificancia(x, y, banda)
		if c == 0 {
			// Sin vecinos significativos no entra en esta pasada: le toca a la de
			// limpieza. Marcarlo aquí lo sacaría de las dos.
			return
		}
		if mq.decodificar(&cx[c]) == 1 {
			b.leerSigno(mq, cx, x, y)
		}
		// Visitado tanto si despertó como si no: la pasada de limpieza no debe
		// volver a mirarlo en este plano.
		b.flag[i] |= bitVisitado
	})
}

// pasadaRefinamiento (§D.3.3): un bit más para los que ya eran significativos.
func (b *bloqueT1) pasadaRefinamiento(mq *decodificadorMQ, cx []contextoMQ) {
	b.porBandas(func(x, y int) {
		i := b.idx(x, y)
		if b.flag[i]&bitSignificativo == 0 || b.flag[i]&bitVisitado != 0 {
			return
		}
		c := b.contextoRefinamiento(x, y)
		if mq.decodificar(&cx[c]) == 1 {
			b.mag[i] |= 1
		}
		b.flag[i] |= bitRefinado
	})
}

// pasadaLimpieza (§D.3.4): el resto, con codificación de rachas.
//
// La racha es lo que hace que una zona vacía casi no ocupe: si una columna de
// cuatro está entera a cero, sin vecinos y alineada, se dice con UN símbolo en
// vez de con cuatro.
func (b *bloqueT1) pasadaLimpieza(mq *decodificadorMQ, cx []contextoMQ, banda int) {
	for y0 := 0; y0 < b.alto; y0 += 4 {
		for x := 0; x < b.ancho; x++ {
			y := y0
			// ¿Cabe la columna entera y está toda en cero, sin vecinos?
			if y0+3 < b.alto && b.columnaLimpia(x, y0, banda) {
				if mq.decodificar(&cx[ctxRacha]) == 0 {
					continue // las cuatro siguen a cero
				}
				// Dos bits en el contexto uniforme dicen CUÁL de las cuatro
				// despierta; las de encima siguen a cero.
				y = y0 + int(mq.decodificar(&cx[ctxUniforme])<<1|mq.decodificar(&cx[ctxUniforme]))
				b.leerSigno(mq, cx, x, y)
				y++
			}
			for ; y < y0+4 && y < b.alto; y++ {
				i := b.idx(x, y)
				if b.flag[i]&(bitSignificativo|bitVisitado) != 0 {
					continue
				}
				c := b.contextoSignificancia(x, y, banda)
				if mq.decodificar(&cx[c]) == 1 {
					b.leerSigno(mq, cx, x, y)
				}
			}
		}
	}
}

// columnaLimpia dice si las cuatro celdas están a cero, sin visitar y sin ningún
// vecino significativo, que es la condición para usar la racha.
func (b *bloqueT1) columnaLimpia(x, y0, banda int) bool {
	for k := 0; k < 4; k++ {
		i := b.idx(x, y0+k)
		if b.flag[i]&(bitSignificativo|bitVisitado) != 0 {
			return false
		}
		if b.contextoSignificancia(x, y0+k, banda) != 0 {
			return false
		}
	}
	return true
}

// leerSigno marca el coeficiente como significativo y lee su signo.
func (b *bloqueT1) leerSigno(mq *decodificadorMQ, cx []contextoMQ, x, y int) {
	c, xorBit := b.contextoDelSigno(x, y)
	bit := mq.decodificar(&cx[c]) ^ xorBit
	b.mag[b.idx(x, y)] |= 1
	b.marcarSignificativo(x, y, bit == 1)
}

// porBandas recorre el bloque en bandas de cuatro filas, columna a columna, que
// es el orden en que está escrito el flujo.
func (b *bloqueT1) porBandas(f func(x, y int)) {
	for y0 := 0; y0 < b.alto; y0 += 4 {
		for x := 0; x < b.ancho; x++ {
			for y := y0; y < y0+4 && y < b.alto; y++ {
				f(x, y)
			}
		}
	}
}
