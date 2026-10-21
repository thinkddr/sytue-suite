// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"testing"
)

// escritorBits es el inverso de lectorBits, solo para generar entradas.
type escritorBits struct {
	b []byte
	n int
}

func (w *escritorBits) poner(v, k int) {
	for i := k - 1; i >= 0; i-- {
		if w.n%8 == 0 {
			w.b = append(w.b, 0)
		}
		if v>>i&1 == 1 {
			w.b[len(w.b)-1] |= 1 << (7 - w.n%8)
		}
		w.n++
	}
}

func (w *escritorBits) alinear() { w.n = (w.n + 7) / 8 * 8 }

// codificarHuff escribe v (u OOB) con la tabla: el inverso de tablaHuffman.leer.
func codificarHuff(t *testing.T, w *escritorBits, tabla *tablaHuffman, v int, oob bool) {
	t.Helper()
	for k, i := range tabla.codigos {
		l := tabla.lineas[i]
		switch {
		case oob && l.oob:
			w.poner(k[1], k[0])
			return
		case oob || l.oob:
			continue
		case l.inferior && v <= l.bajo:
			w.poner(k[1], k[0])
			w.poner(l.bajo-v, l.rango)
			return
		case !l.inferior && v >= l.bajo && v-l.bajo < 1<<l.rango:
			w.poner(k[1], k[0])
			w.poner(v-l.bajo, l.rango)
			return
		}
	}
	t.Fatalf("la tabla no codifica %d", v)
}

// Los códigos que la norma escribe en sus tablas salen de B.3 con las líneas
// en su orden: si una línea estuviera mal copiada, cambiarían.
func TestJBIG2TablasEstandarDeLaNorma(t *testing.T) {
	casos := []struct {
		tabla       int
		largo, cod  int
		valor       int
		oob, conVal bool
	}{
		{3, 6, 0b111110, 0, true, false},      // OOB de B.3
		{3, 7, 0b1111110, 75, false, true},    // B.3: 75…∞ con 32 bits a 0
		{8, 2, 0b01, 0, true, false},          // OOB de B.8
		{8, 9, 0b111111101, -3, false, false}, // B.8: −3
		{10, 2, 0b10, 0, true, false},         // OOB de B.10
		{15, 3, 0b101, 1, false, false},       // B.15: 1
		{7, 3, 0b000, -512, false, true},      // B.7: −512 con 8 bits a 0
	}
	for _, c := range casos {
		w := &escritorBits{}
		w.poner(c.cod, c.largo)
		if c.conVal {
			w.poner(0, tablasEstandar[c.tabla].lineas[tablasEstandar[c.tabla].codigos[[2]int{c.largo, c.cod}]].rango)
		}
		v, oob, err := tablasEstandar[c.tabla].leer(&lectorBits{datos: w.b})
		if err != nil || oob != c.oob || (!oob && v != c.valor) {
			t.Errorf("B.%d %0*b = %d (oob %v, %v), quería %d (oob %v)", c.tabla, c.largo, c.cod, v, oob, err, c.valor, c.oob)
		}
	}

	// El ejemplo de B.4: la tabla A transmitida como segmento de tablas.
	tab, err := leerTablaHuffman([]byte{0x42, 0, 0, 0, 0, 0, 1, 1, 0x10, 0x49, 0x23, 0x81, 0x80})
	if err != nil {
		t.Fatal(err)
	}
	quiero := []lineaHuffman{lh(1, 4, 0), lh(2, 8, 16), lh(3, 16, 272), lhInf(0, -1), lh(3, 32, 65808)}
	for i, l := range quiero {
		if i >= len(tab.lineas) || tab.lineas[i] != l {
			t.Fatalf("líneas = %+v, quería %+v", tab.lineas, quiero)
		}
	}
}

// El ejemplo 1 de 7.4.3.1.7: la tabla de identificadores de 32 símbolos.
func TestJBIG2CodigosDeSimboloDeLaNorma(t *testing.T) {
	datos := []byte{
		0x50, 0x03, 0x35, 0x32, 0x53, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x35, 0x0F, 0x8B, 0x30, 0x9E, 0xB8, 0x5F, 0x1D, 0xD2, 0x83, 0x00,
	}
	quiero := []int{0, 0, 0, 9, 6, 6, 6, 6, 3, 4, 4, 4, 4, 4, 4, 0, 7, 9, 8, 7, 5, 5, 5, 5, 5, 5, 3, 6, 7, 4, 7, 7}
	r := &lectorBits{datos: datos}
	tab, err := leerCodigosID(r, 32)
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range quiero {
		if tab.lineas[i].pref != l {
			t.Fatalf("longitud %d = %d, quería %d", i, tab.lineas[i].pref, l)
		}
	}
	if r.pos != len(datos)*8 {
		t.Errorf("acabó en el bit %d, no al final", r.pos)
	}
}

// Diccionario y región de texto con Huffman: el colectivo sin comprimir
// (BMSIZE = 0) y la tabla de identificadores por rachas. Antes la página caía
// al recuadro gris.
func TestJBIG2SimbolosConHuffman(t *testing.T) {
	be := func(v int) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }
	a, b := nuevoMapaBits(2, 4), nuevoMapaBits(3, 4)
	for i := range a.px {
		a.px[i] = uint8(i % 2) //nolint:gosec // G115: 0 o 1
	}
	for i := range b.px {
		b.px[i] = uint8(i / 3 % 2) //nolint:gosec // G115: 0 o 1
	}

	w := &escritorBits{}
	codificarHuff(t, w, tablasEstandar[4], 4, false) // altura
	codificarHuff(t, w, tablasEstandar[2], 2, false) // anchos 2 y 3
	codificarHuff(t, w, tablasEstandar[2], 1, false)
	codificarHuff(t, w, tablasEstandar[2], 0, true)
	codificarHuff(t, w, tablasEstandar[1], 0, false) // BMSIZE 0: sin comprimir
	w.alinear()
	for y := 0; y < 4; y++ {
		for x := 0; x < 2; x++ {
			w.poner(int(a.px[y*2+x]), 1)
		}
		for x := 0; x < 3; x++ {
			w.poner(int(b.px[y*3+x]), 1)
		}
		w.poner(0, 3)
	}
	codificarHuff(t, w, tablasEstandar[1], 0, false) // exportar: 0 no, 2 sí
	codificarHuff(t, w, tablasEstandar[1], 2, false)
	dic := append(append([]byte{0, 1}, be(2)...), be(2)...)
	dic = append(dic, w.b...)

	w = &escritorBits{}
	for i := 0; i < 35; i++ { // solo RUNCODE1, de longitud 1
		w.poner(map[bool]int{true: 1}[i == 1], 4)
	}
	w.poner(0, 1) // los dos símbolos con código de longitud 1: «0» y «1»
	w.poner(0, 1)
	w.alinear()
	codificarHuff(t, w, tablasEstandar[11], 1, false) // STRIPT = −1
	codificarHuff(t, w, tablasEstandar[11], 1, false) // tira en T = 0
	codificarHuff(t, w, tablasEstandar[6], 0, false)  // S = 0
	w.poner(0, 1)                                     // símbolo a
	codificarHuff(t, w, tablasEstandar[8], 3, false)  // S = 0 + 1 + 3 = 4
	w.poner(1, 1)                                     // símbolo b
	codificarHuff(t, w, tablasEstandar[8], 0, true)
	txt := append(append(append(append(be(8), be(4)...), be(0)...), be(0)...), 0, 0, 0x11, 0, 0)
	txt = append(append(txt, be(2)...), w.b...)

	flujo := append(segmentoJBIG2(1, 0, nil, dic), segmentoJBIG2(2, 6, []byte{1}, txt)...)
	got, err := descomprimirJBIG2(flujo, nil, 8, 4)
	if err != nil {
		t.Fatal(err)
	}
	quiero := nuevoMapaBits(8, 4)
	componerJBIG2(quiero, a, 0, 0, 0)
	componerJBIG2(quiero, b, 4, 0, 0)
	if !bytes.Equal(got, quiero.empaquetar()) {
		t.Fatalf("página = %x, quería %x", got, quiero.empaquetar())
	}
}

// codificarID es el inverso de idMQ.leer.
func codificarID(enc *codificadorMQ, cx []contextoMQ, largo, v int) {
	prev := 1
	for i := largo - 1; i >= 0; i-- {
		b := v >> i & 1
		enc.codificar(&cx[prev], uint8(b)) //nolint:gosec // G115: 0 o 1
		prev = prev<<1 | b
	}
}

// Un diccionario con agregación (SDREFAGG): un símbolo hecho de dos copias de
// otro (una región de texto dentro del diccionario) y otro refinado de uno
// solo. El segundo solo sale bien si la región de texto consume el OOB que
// cierra su última tira. Antes el diccionario entero se descartaba.
func TestJBIG2SimbolosAgregados(t *testing.T) {
	be := func(v int) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }
	a := nuevoMapaBits(3, 4)
	for i := range a.px {
		a.px[i] = uint8(i % 2) //nolint:gosec // G115: 0 o 1
	}
	at := [][2]int{{3, -1}, {-3, -1}, {2, -2}, {-2, -2}}
	atBytes := []byte{3, 0xFF, 0xFD, 0xFF, 2, 0xFE, 0xFE, 0xFE}

	// Diccionario 1: A, directo.
	enc := nuevoCodificadorMQ()
	var iadh, iadw, iaex [512]contextoMQ
	cx := make([]contextoMQ, 1<<16)
	codificarEntero(enc, &iadh, 4, false)
	codificarEntero(enc, &iadw, 3, false)
	for y := 0; y < 4; y++ {
		for x := 0; x < 3; x++ {
			enc.codificar(&cx[contextoGenerico(a, x, y, 0, at)], a.px[y*3+x])
		}
	}
	codificarEntero(enc, &iadw, 0, true)
	codificarEntero(enc, &iaex, 0, false)
	codificarEntero(enc, &iaex, 1, false)
	dic1 := append(append(append([]byte{0, 0}, atBytes...), be(1)...), be(1)...)
	dic1 = append(dic1, enc.terminar()...)

	// Diccionario 2 (refagg, plantilla de refinamiento 1): S1 = A A, S2 = A.
	enc = nuevoCodificadorMQ()
	var dh, dw, aai, dt, fs, ds, ri, rdx, rdy, ex [512]contextoMQ
	cxID := make([]contextoMQ, 8)
	cxGR := make([]contextoMQ, 1<<13)
	codificarEntero(enc, &dh, 4, false)
	codificarEntero(enc, &dw, 6, false)
	codificarEntero(enc, &aai, 2, false)
	codificarEntero(enc, &dt, 0, false) // STRIPT
	codificarEntero(enc, &dt, 0, false)
	codificarEntero(enc, &fs, 0, false)
	codificarID(enc, cxID, 2, 0)
	codificarEntero(enc, &ri, 0, false)
	codificarEntero(enc, &ds, 1, false) // S = 0 + 2 + 1 = 3
	codificarID(enc, cxID, 2, 0)
	codificarEntero(enc, &ri, 0, false)
	codificarEntero(enc, &ds, 0, true) // fin de la tira
	codificarEntero(enc, &dw, -3, false)
	codificarEntero(enc, &aai, 1, false)
	codificarID(enc, cxID, 2, 0)
	codificarEntero(enc, &rdx, 0, false)
	codificarEntero(enc, &rdy, 0, false)
	m := nuevoMapaBits(3, 4)
	for y := 0; y < 4; y++ {
		for x := 0; x < 3; x++ {
			enc.codificar(&cxGR[contextoRefinamiento(m, a, x, y, 0, 0, 1, nil)], a.px[y*3+x])
			m.px[y*3+x] = a.px[y*3+x]
		}
	}
	codificarEntero(enc, &dw, 0, true)
	codificarEntero(enc, &ex, 1, false)
	codificarEntero(enc, &ex, 2, false)
	dic2 := append(append(append([]byte{0x10, 0x02}, atBytes...), be(2)...), be(2)...)
	dic2 = append(dic2, enc.terminar()...)

	// Región 10×4: S1 en x = 0 y S2 en x = 7.
	enc = nuevoCodificadorMQ()
	var tdt, tfs, tds [512]contextoMQ
	cxID = make([]contextoMQ, 4)
	codificarEntero(enc, &tdt, 0, false)
	codificarEntero(enc, &tdt, 0, false)
	codificarEntero(enc, &tfs, 0, false)
	codificarID(enc, cxID, 1, 0)
	codificarEntero(enc, &tds, 2, false) // S = 0 + 5 + 2 = 7
	codificarID(enc, cxID, 1, 1)
	codificarEntero(enc, &tds, 0, true)
	txt := append(append(append(append(be(10), be(4)...), be(0)...), be(0)...), 0, 0, 0x10)
	txt = append(append(txt, be(2)...), enc.terminar()...)

	flujo := append(segmentoJBIG2(1, 0, nil, dic1), segmentoJBIG2(2, 0, []byte{1}, dic2)...)
	flujo = append(flujo, segmentoJBIG2(3, 6, []byte{2}, txt)...)
	got, err := descomprimirJBIG2(flujo, nil, 10, 4)
	if err != nil {
		t.Fatal(err)
	}
	quiero := nuevoMapaBits(10, 4)
	for _, x := range []int{0, 3, 7} {
		componerJBIG2(quiero, a, x, 0, 0)
	}
	if !bytes.Equal(got, quiero.empaquetar()) {
		t.Fatalf("página = %x, quería %x", got, quiero.empaquetar())
	}
}

// Un diccionario que retiene sus estadísticas y otro que las usa (7.4.2.2):
// el segundo se codificó siguiendo el estado del primero y sin ellas sale
// ruido. Y el flujo trae la cabecera de un .jb2 suelto, que se salta.
func TestJBIG2ContextosRetenidosYCabeceraDeFichero(t *testing.T) {
	be := func(v int) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }
	a := mapaDePrueba(8, 6)
	at := [][2]int{{3, -1}, {-3, -1}, {2, -2}, {-2, -2}}
	atBytes := []byte{3, 0xFF, 0xFD, 0xFF, 2, 0xFE, 0xFE, 0xFE}
	cx := make([]contextoMQ, 1<<16)
	dic := func(flags byte, exporta []int) []byte {
		enc := nuevoCodificadorMQ()
		var iadh, iadw, iaex [512]contextoMQ
		codificarEntero(enc, &iadh, 6, false)
		codificarEntero(enc, &iadw, 8, false)
		for y := 0; y < 6; y++ {
			for x := 0; x < 8; x++ {
				enc.codificar(&cx[contextoGenerico(a, x, y, 0, at)], a.px[y*8+x])
			}
		}
		codificarEntero(enc, &iadw, 0, true)
		for _, n := range exporta {
			codificarEntero(enc, &iaex, n, false)
		}
		d := append(append(append([]byte{flags, 0}, atBytes...), be(1)...), be(1)...)
		return append(d, enc.terminar()...)
	}
	dic1 := dic(2, []int{0, 1}) // retenido
	dic2 := dic(1, []int{1, 1}) // usado: sigue con los contextos de dic1; exporta solo el nuevo

	enc := nuevoCodificadorMQ()
	var tdt, tfs, tds [512]contextoMQ
	codificarEntero(enc, &tdt, 0, false)
	codificarEntero(enc, &tdt, 0, false)
	codificarEntero(enc, &tfs, 0, false)
	codificarEntero(enc, &tds, 0, true)
	txt := append(append(append(append(be(8), be(6)...), be(0)...), be(0)...), 0, 0, 0x10)
	txt = append(append(txt, be(1)...), enc.terminar()...)

	flujo := []byte("\x97JB2\r\n\x1a\n\x01\x00\x00\x00\x01")
	flujo = append(flujo, segmentoJBIG2(1, 0, nil, dic1)...)
	flujo = append(flujo, segmentoJBIG2(2, 0, []byte{1}, dic2)...)
	flujo = append(flujo, segmentoJBIG2(3, 6, []byte{2}, txt)...)
	got, err := descomprimirJBIG2(flujo, nil, 8, 6)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, a.empaquetar()) {
		t.Fatalf("página = %x, quería %x", got, a.empaquetar())
	}
}
