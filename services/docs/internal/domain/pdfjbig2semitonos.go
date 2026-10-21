// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Semitonos de JBIG2 (T.88, 6.6, 6.7 y anexo C): un diccionario de patrones
// guarda una baldosa por nivel de gris, y la región de semitonos dice qué nivel
// va en cada celda de una rejilla. Es como un escáner comprime una foto de una
// página impresa en trama sin pasarla a escala de grises.
package domain

import "math/bits"

// decodificarDiccionarioPatrones es el segmento 16 (7.4.4 y 6.7): las GRAYMAX+1
// baldosas van una al lado de otra en un solo mapa genérico y se recortan.
func decodificarDiccionarioPatrones(cuerpo []byte) ([]*mapaBits, error) {
	if len(cuerpo) < 7 {
		return nil, ErrJBIG2NoSoportado
	}
	mmr, plantilla := cuerpo[0]&1 != 0, int(cuerpo[0]>>1&3)
	pw, ph := int(cuerpo[1]), int(cuerpo[2])
	n := int(uint32(cuerpo[3])<<24|uint32(cuerpo[4])<<16|uint32(cuerpo[5])<<8|uint32(cuerpo[6])) + 1
	if pw == 0 || ph == 0 || n <= 0 || n > maxSimbolosJBIG2 || n*pw*ph > maxPixelesJBIG2 {
		return nil, ErrJBIG2NoSoportado
	}
	var todo *mapaBits
	if mmr {
		m, _, err := mapaMMR(cuerpo[7:], n*pw, ph)
		if err != nil {
			return nil, err
		}
		todo = m
	} else {
		// Los AT de la tabla 27: el primero, una baldosa a la izquierda.
		at := [][2]int{{-pw, 0}, {-3, -1}, {2, -2}, {-2, -2}}
		todo = decodificarRegionGenerica(nuevoMQ(cuerpo[7:]), make([]contextoMQ, 1<<16), n*pw, ph, plantilla, at, false, nil)
	}
	patrones := make([]*mapaBits, n)
	for g := range patrones {
		p := nuevoMapaBits(pw, ph)
		for y := 0; y < ph; y++ {
			copy(p.px[y*pw:(y+1)*pw], todo.px[y*todo.ancho+g*pw:])
		}
		patrones[g] = p
	}
	return patrones, nil
}

// regionSemitonos es el segmento 20/22/23 (7.4.5 y 6.6) con los patrones del
// diccionario que nombra.
func regionSemitonos(cuerpo []byte, patrones []*mapaBits) (*regionJBIG2, error) {
	if len(cuerpo) < 38 || len(patrones) == 0 {
		return nil, ErrJBIG2NoSoportado
	}
	be := func(i int) int {
		return int(int32(uint32(cuerpo[i])<<24 | uint32(cuerpo[i+1])<<16 | uint32(cuerpo[i+2])<<8 | uint32(cuerpo[i+3]))) //nolint:gosec // G115: HGX y HGY son con signo
	}
	ancho, alto := be(0), be(4)
	if ancho <= 0 || alto <= 0 || ancho*alto > maxPixelesJBIG2 {
		return nil, ErrJBIG2NoSoportado
	}
	f := cuerpo[17]
	mmr, plantilla, saltar := f&1 != 0, int(f>>1&3), f&8 != 0
	op, defecto := int(f>>4&7), f>>7
	gw, gh, gx, gy := be(18), be(22), be(26), be(30)
	rx, ry := int(cuerpo[34])<<8|int(cuerpo[35]), int(cuerpo[36])<<8|int(cuerpo[37])
	if gw <= 0 || gh <= 0 || gw*gh > maxPixelesJBIG2 {
		return nil, ErrJBIG2NoSoportado
	}
	pw, ph := patrones[0].ancho, patrones[0].alto
	celda := func(mg, ng int) (int, int) {
		return (gx + mg*ry + ng*rx) >> 8, (gy + mg*rx - ng*ry) >> 8
	}

	m := nuevoMapaBits(ancho, alto)
	if defecto == 1 {
		for i := range m.px {
			m.px[i] = 1
		}
	}
	var skip *mapaBits
	if saltar && !mmr {
		skip = mapaSkip(gw, gh, func(mg, ng int) bool {
			x, y := celda(mg, ng)
			return x+pw <= 0 || x >= ancho || y+ph <= 0 || y >= alto
		})
	}
	gris, err := imagenDeGrises(cuerpo[38:], mmr, plantilla, gw, gh, bits.Len(uint(len(patrones)-1)), skip)
	if err != nil {
		return nil, err
	}
	for mg := 0; mg < gh; mg++ {
		for ng := 0; ng < gw; ng++ {
			if g := gris[mg*gw+ng]; g < len(patrones) {
				x, y := celda(mg, ng)
				componerJBIG2(m, patrones[g], x, y, op)
			}
		}
	}
	return &regionJBIG2{mapa: m, x0: be(8), y0: be(12), op: int(cuerpo[16] & 7)}, nil
}

// mapaSkip es HSKIP (6.6.5.1): 1 donde el patrón caería entero fuera.
func mapaSkip(gw, gh int, fuera func(mg, ng int) bool) *mapaBits {
	skip := nuevoMapaBits(gw, gh)
	for mg := 0; mg < gh; mg++ {
		for ng := 0; ng < gw; ng++ {
			if fuera(mg, ng) {
				skip.px[mg*gw+ng] = 1
			}
		}
	}
	return skip
}

// imagenDeGrises es el anexo C.5: bpp planos de bits del más significativo al
// menos, en código Gray (cada plano se XOR con el de encima). Los planos
// aritméticos comparten el decodificador y los contextos; los MMR van seguidos,
// cada uno con su fin de bloque.
func imagenDeGrises(datos []byte, mmr bool, plantilla, ancho, alto, bpp int, skip *mapaBits) ([]int, error) {
	gris := make([]int, ancho*alto)
	if bpp == 0 {
		return gris, nil
	}
	if bpp > 30 {
		return nil, ErrJBIG2NoSoportado
	}
	axt := 3
	if plantilla >= 2 {
		axt = 2
	}
	at := [][2]int{{axt, -1}, {-3, -1}, {2, -2}, {-2, -2}}
	var mq *decodificadorMQ
	var cx []contextoMQ
	if !mmr {
		mq, cx = nuevoMQ(datos), make([]contextoMQ, 1<<16)
	}
	var anterior *mapaBits
	for j := bpp - 1; j >= 0; j-- {
		var plano *mapaBits
		if mmr {
			p, n, err := mapaMMR(datos, ancho, alto)
			if err != nil {
				return nil, err
			}
			plano, datos = p, datos[n:]
		} else {
			plano = decodificarRegionGenerica(mq, cx, ancho, alto, plantilla, at, false, skip)
		}
		if anterior != nil {
			for i := range plano.px {
				plano.px[i] ^= anterior.px[i]
			}
		}
		for i, b := range plano.px {
			gris[i] |= int(b) << j
		}
		anterior = plano
	}
	return gris, nil
}
