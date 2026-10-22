// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"testing"
)

// codificarEntero es el inverso de enteroMQ.leer (anexo A.2).
func codificarEntero(enc *codificadorMQ, cx *[512]contextoMQ, v int, oob bool) {
	prev := 1
	bit := func(b int) {
		enc.codificar(&cx[prev], uint8(b)) //nolint:gosec // G115: 0 o 1
		if prev < 256 {
			prev = prev<<1 | b
		} else {
			prev = (prev<<1|b)&511 | 256
		}
	}
	n := func(k, val int) {
		for i := k - 1; i >= 0; i-- {
			bit(val >> i & 1)
		}
	}
	if oob {
		bit(1)
		bit(0)
		n(2, 0)
		return
	}
	s := 0
	if v < 0 {
		s, v = 1, -v
	}
	bit(s)
	prefijos := []struct{ hasta, unos, bits, base int }{{4, 0, 2, 0}, {20, 1, 4, 4}, {84, 2, 6, 20}, {340, 3, 8, 84}, {4436, 4, 12, 340}}
	for _, p := range prefijos {
		if v < p.hasta {
			for i := 0; i < p.unos; i++ {
				bit(1)
			}
			bit(0)
			n(p.bits, v-p.base)
			return
		}
	}
	for i := 0; i < 5; i++ {
		bit(1)
	}
	n(32, v-4436)
}

func segmentoJBIG2(numero, tipo byte, referidas, cuerpo []byte) []byte {
	be := func(v int) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }
	seg := append(be(int(numero)), tipo, byte(len(referidas)<<5))
	seg = append(append(seg, referidas...), 1)
	return append(append(seg, be(len(cuerpo))...), cuerpo...)
}

// Un diccionario con dos símbolos y una región de texto que los coloca: es el
// modo símbolo de jbig2enc y Acrobat. Antes la página caía al recuadro gris.
func TestJBIG2DiccionarioYRegionDeTexto(t *testing.T) {
	a, b := nuevoMapaBits(3, 4), nuevoMapaBits(2, 4)
	for i := range a.px {
		a.px[i] = uint8(i % 2) //nolint:gosec // G115: 0 o 1
	}
	for i := range b.px {
		b.px[i] = 1
	}
	at := [][2]int{{3, -1}, {-3, -1}, {2, -2}, {-2, -2}}
	be := func(v int) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }

	// Diccionario: altura 4, anchos 3 y 2 (deltas 3 y -1), OOB y exportar los dos.
	enc := nuevoCodificadorMQ()
	var iadh, iadw, iaex [512]contextoMQ
	cx := make([]contextoMQ, 1<<16)
	codificarEntero(enc, &iadh, 4, false)
	for i, s := range []*mapaBits{a, b} {
		codificarEntero(enc, &iadw, []int{3, -1}[i], false)
		for y := 0; y < s.alto; y++ {
			for x := 0; x < s.ancho; x++ {
				enc.codificar(&cx[contextoGenerico(s, x, y, 0, at)], s.px[y*s.ancho+x])
			}
		}
	}
	codificarEntero(enc, &iadw, 0, true)
	codificarEntero(enc, &iaex, 0, false) // ninguno sin exportar…
	codificarEntero(enc, &iaex, 2, false) // …y los dos exportados
	dic := []byte{0, 0, 3, 0xFF, 0xFD, 0xFF, 2, 0xFE, 0xFE, 0xFE}
	dic = append(append(append(dic, be(2)...), be(2)...), enc.terminar()...)

	// Región de texto 10×6, esquina arriba-izquierda: A en (1,1) y B en (6,1).
	enc = nuevoCodificadorMQ()
	var iadt, iafs, iads [512]contextoMQ
	cxID := make([]contextoMQ, 4)
	id := func(v int) {
		enc.codificar(&cxID[1], uint8(v)) //nolint:gosec // G115: 0 o 1
	}
	codificarEntero(enc, &iadt, 0, false) // STRIPT inicial
	codificarEntero(enc, &iadt, 1, false) // T = 1
	codificarEntero(enc, &iafs, 1, false) // S = 1
	id(0)
	codificarEntero(enc, &iads, 3, false) // S = 1 + 2 + 3 = 6
	id(1)
	txt := append(append(append(append(be(10), be(6)...), be(0)...), be(0)...), 0, 0, 0x10)
	txt = append(append(txt, be(2)...), enc.terminar()...)

	flujo := append(segmentoJBIG2(1, 0, nil, dic), segmentoJBIG2(2, 6, []byte{1}, txt)...)
	got, err := descomprimirJBIG2(flujo, nil, 10, 6)
	if err != nil {
		t.Fatal(err)
	}
	quiero := nuevoMapaBits(10, 6)
	componerJBIG2(quiero, a, 1, 1, 0)
	componerJBIG2(quiero, b, 6, 1, 0)
	if !bytes.Equal(got, quiero.empaquetar()) {
		t.Fatalf("página = %x, quería %x", got, quiero.empaquetar())
	}
}

// Una región intermedia (36) no va a la página: la corrige una región de
// refinamiento (42) que la nombra, y lo refinado es lo que se ve. Antes la
// intermedia se pintaba tal cual y el refinamiento se ignoraba.
func TestJBIG2RegionDeRefinamiento(t *testing.T) {
	ref := mapaDePrueba(16, 8)
	quiero := nuevoMapaBits(16, 8)
	copy(quiero.px, ref.px)
	quiero.px[3*16+5] ^= 1 // un píxel distinto
	quiero.px[6*16+9] ^= 1

	enc := nuevoCodificadorMQ()
	cx := make([]contextoMQ, 1<<13)
	m := nuevoMapaBits(16, 8)
	for y := 0; y < 8; y++ {
		for x := 0; x < 16; x++ {
			v := quiero.px[y*16+x]
			enc.codificar(&cx[contextoRefinamiento(m, ref, x, y, 0, 0, 1, nil)], v)
			m.px[y*16+x] = v
		}
	}
	be := func(v int) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }
	refina := append(append(append(append(be(16), be(8)...), be(0)...), be(0)...), 0, 1) // OR, plantilla 1
	refina = append(refina, enc.terminar()...)

	intermedia := jbig2DeMapa(ref, 0)
	intermedia[4] = 36 // región genérica intermedia, segmento 1
	intermedia = append(intermedia, segmentoJBIG2(2, 42, []byte{1}, refina)...)
	got, err := descomprimirJBIG2(intermedia, nil, 16, 8)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, quiero.empaquetar()) {
		t.Fatalf("página = %x, quería %x", got, quiero.empaquetar())
	}
}

// Con TPGRON (6.3.5.6) cada fila lleva un bit de «fila típica» con el contexto de
// las figuras 14 y 15, y en una fila típica los píxeles de vecindad uniforme no
// se codifican. Antes se pintaba la referencia sin refinar.
func TestJBIG2RefinamientoConTPGRON(t *testing.T) {
	for plantilla := 0; plantilla <= 1; plantilla++ {
		ref := mapaDePrueba(16, 8)
		quiero := nuevoMapaBits(16, 8)
		copy(quiero.px, ref.px)
		for _, y := range []int{3, 6} {
			for x := 0; x < 16; x++ {
				if _, ok := vecindadUniforme(ref, x, y); !ok {
					quiero.px[y*16+x] ^= 1
					break
				}
			}
		}
		at := [][2]int{{-1, -1}, {-1, -1}}
		enc := nuevoCodificadorMQ()
		cx := make([]contextoMQ, 1<<13)
		m := nuevoMapaBits(16, 8)
		for y := 0; y < 8; y++ {
			// Fila 0: LTP pasa a 1 y se queda así.
			enc.codificar(&cx[contextoSLTPRefinamiento(plantilla)], map[bool]uint8{true: 1, false: 0}[y == 0])
			for x := 0; x < 16; x++ {
				v := quiero.px[y*16+x]
				if _, ok := vecindadUniforme(ref, x, y); !ok {
					enc.codificar(&cx[contextoRefinamiento(m, ref, x, y, 0, 0, plantilla, at)], v)
				}
				m.px[y*16+x] = v
			}
		}
		be := func(v int) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }
		refina := append(append(append(append(be(16), be(8)...), be(0)...), be(0)...), 0, byte(plantilla)|2)
		if plantilla == 0 {
			refina = append(refina, 0xFF, 0xFF, 0xFF, 0xFF)
		}
		refina = append(refina, enc.terminar()...)

		intermedia := jbig2DeMapa(ref, 0)
		intermedia[4] = 36
		intermedia = append(intermedia, segmentoJBIG2(2, 42, []byte{1}, refina)...)
		got, err := descomprimirJBIG2(intermedia, nil, 16, 8)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, quiero.empaquetar()) {
			t.Fatalf("plantilla %d: página = %x, quería %x", plantilla, got, quiero.empaquetar())
		}
	}
}
