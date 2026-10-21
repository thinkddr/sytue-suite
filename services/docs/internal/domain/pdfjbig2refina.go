// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Refinamiento genérico de JBIG2 (T.88, 6.3): una imagen se decodifica mirando
// otra ya conocida —una región intermedia, un símbolo del diccionario— y solo
// codifica en qué se diferencian. Lo usan las regiones de refinamiento (40, 42,
// 43), las regiones de texto con SBREFINE y los diccionarios con agregación.
package domain

// contextoRefinamiento arma el contexto de un píxel con vecinos de la imagen
// que se decodifica (m) y de la de referencia (ref, desplazada dx, dy).
//
// Plantilla 0: trece píxeles —cuatro de la actual, uno adaptativo, y el 3×3 de
// la referencia con su esquina superior izquierda adaptativa—. Plantilla 1: diez.
func contextoRefinamiento(m, ref *mapaBits, x, y, dx, dy, plantilla int, at [][2]int) uint16 {
	c := func(ax, ay int) uint16 { return uint16(m.en(x+ax, y+ay)) }
	r := func(ax, ay int) uint16 { return uint16(ref.en(x-dx+ax, y-dy+ay)) }
	if plantilla == 0 {
		return c(-1, 0) | c(1, -1)<<1 | c(0, -1)<<2 | c(at[0][0], at[0][1])<<3 |
			r(1, 1)<<4 | r(0, 1)<<5 | r(-1, 1)<<6 | r(1, 0)<<7 | r(0, 0)<<8 | r(-1, 0)<<9 |
			r(1, -1)<<10 | r(0, -1)<<11 | r(at[1][0], at[1][1])<<12
	}
	return c(-1, 0) | c(1, -1)<<1 | c(0, -1)<<2 | c(-1, -1)<<3 |
		r(1, 1)<<4 | r(0, 1)<<5 | r(1, 0)<<6 | r(0, 0)<<7 | r(-1, 0)<<8 | r(0, -1)<<9
}

// atRefinamientoPorDefecto son GRAT1 y GRAT2 cuando la plantilla 1 no los trae.
var atRefinamientoPorDefecto = [][2]int{{-1, -1}, {-1, -1}}

// contextoSLTPRefinamiento es el contexto del bit de «fila típica» (6.3.5.6,
// figuras 14 y 15 de T.88): todos los píxeles de la plantilla a 0 salvo el
// central de la referencia. Con el orden de contextoRefinamiento, ese píxel es
// el bit 8 en la plantilla 0 y el 7 en la 1.
func contextoSLTPRefinamiento(plantilla int) uint16 {
	if plantilla == 0 {
		return 1 << 8
	}
	return 1 << 7
}

// decodificarRefinamiento es 6.3.5.6. Con TPGRON, cada fila empieza con un bit
// que alterna LTP; en una fila típica, el píxel cuya vecindad 3×3 en la
// referencia es uniforme toma ese valor sin decodificarse (figura 16).
func decodificarRefinamiento(mq *decodificadorMQ, cx []contextoMQ, ancho, alto int, ref *mapaBits,
	dx, dy, plantilla int, at [][2]int, tpgron bool,
) *mapaBits {
	if len(at) < 2 {
		at = atRefinamientoPorDefecto
	}
	m := nuevoMapaBits(ancho, alto)
	ltp := false
	for y := 0; y < alto; y++ {
		if tpgron && mq.decodificar(&cx[contextoSLTPRefinamiento(plantilla)]) == 1 {
			ltp = !ltp
		}
		for x := 0; x < ancho; x++ {
			if ltp {
				if v, ok := vecindadUniforme(ref, x-dx, y-dy); ok {
					m.px[y*ancho+x] = v
					continue
				}
			}
			m.px[y*ancho+x] = mq.decodificar(&cx[contextoRefinamiento(m, ref, x, y, dx, dy, plantilla, at)])
		}
	}
	return m
}

// vecindadUniforme dice si los nueve píxeles de la referencia centrados en
// (x, y) valen lo mismo, y cuál (fuera de los bordes valen 0).
func vecindadUniforme(ref *mapaBits, x, y int) (uint8, bool) {
	v := ref.en(x, y)
	for j := -1; j <= 1; j++ {
		for i := -1; i <= 1; i++ {
			if ref.en(x+i, y+j) != v {
				return 0, false
			}
		}
	}
	return v, true
}

// regionDeRefinamiento decodifica un segmento 40/42/43 (7.4.7). La referencia es
// la región intermedia que nombra, o si no nombra ninguna, lo que ya hay en la
// página bajo la región.
func (est *estadoJBIG2) regionDeRefinamiento(cab cabeceraSegmento, cuerpo []byte) (*regionJBIG2, error) {
	if len(cuerpo) < 18 {
		return nil, ErrJBIG2NoSoportado
	}
	be := func(i int) int {
		return int(uint32(cuerpo[i])<<24 | uint32(cuerpo[i+1])<<16 | uint32(cuerpo[i+2])<<8 | uint32(cuerpo[i+3]))
	}
	ancho, alto, x0, y0 := be(0), be(4), be(8), be(12)
	if ancho <= 0 || alto <= 0 || ancho*alto > maxPixelesJBIG2 {
		return nil, ErrJBIG2NoSoportado
	}
	op := int(cuerpo[16] & 7)
	banderas := cuerpo[17]
	plantilla, tpgron := int(banderas&1), banderas&2 != 0
	q := 18
	at := atRefinamientoPorDefecto
	if plantilla == 0 {
		if q+4 > len(cuerpo) {
			return nil, ErrJBIG2NoSoportado
		}
		at = leerAT(cuerpo[q:q+4], 0)[:2]
		q += 4
	}
	var ref *mapaBits
	rx, ry := x0, y0
	for _, n := range cab.referidas {
		if r, ok := est.regiones[n]; ok {
			ref, rx, ry = r.mapa, r.x0, r.y0
			break
		}
	}
	if ref == nil {
		// Sin región intermedia: se refina lo que ya está en la página.
		ref = nuevoMapaBits(ancho, alto)
		for y := 0; y < alto; y++ {
			for x := 0; x < ancho; x++ {
				ref.px[y*ancho+x] = est.pagina.en(x0+x, y0+y)
			}
		}
		rx, ry = x0, y0
	}
	mq := nuevoMQ(cuerpo[q:])
	cx := make([]contextoMQ, 1<<13)
	m := decodificarRefinamiento(mq, cx, ancho, alto, ref, rx-x0, ry-y0, plantilla, at, tpgron)
	return &regionJBIG2{mapa: m, x0: x0, y0: y0, op: op}, nil
}
