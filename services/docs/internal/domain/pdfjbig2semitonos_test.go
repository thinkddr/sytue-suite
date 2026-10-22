// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"testing"
)

// Un diccionario de cuatro patrones 2×2 y una región de semitonos 3×2 celdas
// con los cuatro niveles: los planos van en código Gray (anexo C.5) y la rejilla
// es la simple (HRX = 2·256). Antes la página caía al recuadro gris.
func TestJBIG2Semitonos(t *testing.T) {
	be := func(v int) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }
	codificar := func(m *mapaBits, at [][2]int, enc *codificadorMQ, cx []contextoMQ) {
		for y := 0; y < m.alto; y++ {
			for x := 0; x < m.ancho; x++ {
				enc.codificar(&cx[contextoGenerico(m, x, y, 0, at)], m.px[y*m.ancho+x])
			}
		}
	}

	// Patrón g: g píxeles de tinta en orden de lectura.
	patrones := nuevoMapaBits(8, 2)
	for g := 0; g < 4; g++ {
		for i := 0; i < g; i++ {
			patrones.px[(i/2)*8+g*2+i%2] = 1
		}
	}
	enc := nuevoCodificadorMQ()
	codificar(patrones, [][2]int{{-2, 0}, {-3, -1}, {2, -2}, {-2, -2}}, enc, make([]contextoMQ, 1<<16))
	dic := append([]byte{0, 2, 2}, be(3)...)
	dic = append(dic, enc.terminar()...)

	gris := []int{0, 1, 2, 3, 2, 1}
	enc = nuevoCodificadorMQ()
	cx := make([]contextoMQ, 1<<16)
	at := [][2]int{{3, -1}, {-3, -1}, {2, -2}, {-2, -2}}
	for j := 1; j >= 0; j-- {
		plano := nuevoMapaBits(3, 2)
		for i, g := range gris {
			plano.px[i] = uint8((g>>j ^ g>>(j+1)) & 1) //nolint:gosec // G115: 0 o 1
		}
		codificar(plano, at, enc, cx)
	}
	ht := append(append(append(append(be(6), be(4)...), be(0)...), be(0)...), 0, 0)
	ht = append(append(append(append(append(ht, be(3)...), be(2)...), be(0)...), be(0)...), 2, 0, 0, 0)
	ht = append(ht, enc.terminar()...)

	flujo := append(segmentoJBIG2(1, 16, nil, dic), segmentoJBIG2(2, 22, []byte{1}, ht)...)
	got, err := descomprimirJBIG2(flujo, nil, 6, 4)
	if err != nil {
		t.Fatal(err)
	}
	quiero := nuevoMapaBits(6, 4)
	for i, g := range gris {
		p := nuevoMapaBits(2, 2)
		for y := 0; y < 2; y++ {
			copy(p.px[y*2:], patrones.px[y*8+g*2:y*8+g*2+2])
		}
		componerJBIG2(quiero, p, i%3*2, i/3*2, 0)
	}
	if !bytes.Equal(got, quiero.empaquetar()) {
		t.Fatalf("página = %x, quería %x", got, quiero.empaquetar())
	}
}
