// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks del decodificador JBIG2.
//
// EL PROBLEMA DE PROBAR ESTO es que no se puede escribir un flujo JBIG2 a mano:
// va comprimido con un codificador aritmético, así que para generar una entrada
// hay que tener el CODIFICADOR. Aquí se resuelve escribiendo el codificador MQ
// —cuarenta líneas, el espejo exacto del decodificador— y comprobando la ida y
// vuelta. Es más honesto que meter un binario en el repo que nadie sabría de
// dónde salió, y además prueba justo lo que importa: que la tabla de estados y
// la máquina de renormalización están bien copiadas.
package domain

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// --- Codificador MQ, solo para los tests -------------------------------------

// codificadorMQ es el espejo del decodificador (T.88, anexo E.3). No forma parte
// del producto: existe para poder generar entradas válidas.
//
// DOS TRAMPAS, las dos pisadas al escribirlo y las dos con el mismo síntoma —la
// ida y vuelta falla en el bit 10 y no en el primero, así que parece un problema
// del decodificador—:
//
//  1. En el camino MPS sin renormalización hay que sumar Qe a C. Es la línea que
//     se olvida porque «no pasa nada» ahí; pero el decodificador SÍ resta Qe en su
//     rama simétrica, y sin la suma los dos dejan de estar de acuerdo.
//  2. BP arranca ANTES del buffer, así que el acarreo del primer byte no tiene
//     dónde caer. Se resuelve con un byte de relleno al principio que se descarta
//     al terminar, que es lo que hacen todas las implementaciones.
type codificadorMQ struct {
	salida []byte
	c      uint32
	a      uint32
	ct     int
	bp     int
}

func nuevoCodificadorMQ() *codificadorMQ {
	// El 0 inicial es el byte de relleno donde puede caer un acarreo; `terminar`
	// lo quita.
	return &codificadorMQ{salida: []byte{0}, bp: 0, a: 0x8000, ct: 12}
}

func (e *codificadorMQ) conRelleno() {
	e.salida = append(e.salida, byte(e.c>>20))
	e.bp = len(e.salida) - 1
	e.c &= 0xFFFFF
	e.ct = 7
}

func (e *codificadorMQ) sinRelleno() {
	e.salida = append(e.salida, byte(e.c>>19))
	e.bp = len(e.salida) - 1
	e.c &= 0x7FFFF
	e.ct = 8
}

// byteOut es BYTEOUT (E.3.7): saca un byte y mete el relleno de bit tras un 0xFF,
// que es lo que impide que el flujo produzca una marca por accidente.
func (e *codificadorMQ) byteOut() {
	if e.salida[e.bp] == 0xFF {
		e.conRelleno()
		return
	}
	if e.c < 0x8000000 {
		e.sinRelleno()
		return
	}
	e.salida[e.bp]++
	if e.salida[e.bp] == 0xFF {
		e.c &= 0x7FFFFFF
		e.conRelleno()
		return
	}
	e.sinRelleno()
}

func (e *codificadorMQ) renorm() {
	for {
		e.a <<= 1
		e.c <<= 1
		e.ct--
		if e.ct == 0 {
			e.byteOut()
		}
		if e.a&0x8000 != 0 {
			return
		}
	}
}

func (e *codificadorMQ) codificar(cx *contextoMQ, bit uint8) {
	qe := tablaQe[cx.i].qe
	if bit == cx.mps {
		// CODEMPS.
		e.a -= qe
		if e.a&0x8000 != 0 {
			// Sin renormalizar, pero C SÍ avanza: es la trampa 1 de arriba.
			e.c += qe
			return
		}
		if e.a < qe {
			e.a = qe
		} else {
			e.c += qe
		}
		cx.i = tablaQe[cx.i].nmps
		e.renorm()
		return
	}
	// CODELPS.
	e.a -= qe
	if e.a < qe {
		e.c += qe
	} else {
		e.a = qe
	}
	if tablaQe[cx.i].cambio == 1 {
		cx.mps = 1 - cx.mps
	}
	cx.i = tablaQe[cx.i].nlps
	e.renorm()
}

// terminar es FLUSH: empuja lo que queda en el registro y cierra con 0xFFAC.
func (e *codificadorMQ) terminar() []byte {
	// SETBITS.
	tmp := e.c + e.a
	e.c |= 0xFFFF
	if e.c >= tmp {
		e.c -= 0x8000
	}
	e.c <<= uint(e.ct)
	e.byteOut()
	e.c <<= uint(e.ct)
	e.byteOut()
	if e.salida[e.bp] != 0xFF {
		e.salida = append(e.salida, 0xFF)
	}
	e.salida = append(e.salida, 0xAC)
	return e.salida[1:] // fuera el byte de relleno inicial
}

// --- La ida y vuelta ---------------------------------------------------------

// TestMQIdaYVuelta es el check que sostiene todo lo demás: si la tabla de estados
// o la renormalización estuvieran mal copiadas, los bits saldrían en otro orden y
// una imagen JBIG2 se vería como nieve SIN dar ningún error.
func TestMQIdaYVuelta(t *testing.T) {
	// Una secuencia con rachas y alternancias, que es donde el codificador
	// aritmético cambia de estado y ejercita la tabla.
	var bits []uint8
	for i := 0; i < 400; i++ {
		switch {
		case i < 100:
			bits = append(bits, 0)
		case i < 150:
			bits = append(bits, 1)
		default:
			bits = append(bits, uint8(i%2))
		}
	}

	enc := nuevoCodificadorMQ()
	cxEnc := make([]contextoMQ, 8)
	for i, b := range bits {
		enc.codificar(&cxEnc[i%8], b)
	}
	datos := enc.terminar()

	dec := nuevoMQ(datos)
	cxDec := make([]contextoMQ, 8)
	for i, quiero := range bits {
		if got := dec.decodificar(&cxDec[i%8]); got != quiero {
			t.Fatalf("bit %d = %d, quería %d (la tabla o la renormalización están mal)", i, got, quiero)
		}
	}
}

// --- Región genérica ---------------------------------------------------------

// jbig2DeMapa comprime un mapa de bits como una región genérica inmediata y
// devuelve el flujo JBIG2 embebido completo, con su cabecera de segmento.
func jbig2DeMapa(m *mapaBits, plantilla int) []byte {
	at := [][2]int{{3, -1}, {-3, -1}, {2, -2}, {-2, -2}}
	if plantilla != 0 {
		at = [][2]int{{3, -1}}
	}

	enc := nuevoCodificadorMQ()
	cx := make([]contextoMQ, 1<<16)
	for y := 0; y < m.alto; y++ {
		for x := 0; x < m.ancho; x++ {
			ctx := contextoGenerico(m, x, y, plantilla, at)
			enc.codificar(&cx[ctx], m.px[y*m.ancho+x])
		}
	}
	comprimido := enc.terminar()

	// Cabecera de información de región: 17 bytes.
	be := func(v int) []byte {
		return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
	}
	cuerpo := be(m.ancho)
	cuerpo = append(cuerpo, be(m.alto)...)
	cuerpo = append(cuerpo, be(0)...) // x
	cuerpo = append(cuerpo, be(0)...) // y
	cuerpo = append(cuerpo,
		0, // operador de composición: OR
		byte(plantilla<<1))
	for _, p := range at {
		cuerpo = append(cuerpo, byte(p[0]), byte(p[1]))
	}
	cuerpo = append(cuerpo, comprimido...)

	// Cabecera de segmento: número, banderas (tipo 38), referencias, página,
	// longitud.
	seg := append(be(1),
		38, // tipo: región genérica inmediata
		0,  // sin referencias
		1)  // página 1
	seg = append(seg, be(len(cuerpo))...)
	return append(seg, cuerpo...)
}

// mapaDePrueba dibuja algo reconocible: un marco y una diagonal.
func mapaDePrueba(ancho, alto int) *mapaBits {
	m := nuevoMapaBits(ancho, alto)
	for x := 0; x < ancho; x++ {
		m.marcar(x, 0)
		m.marcar(x, alto-1)
	}
	for y := 0; y < alto; y++ {
		m.marcar(0, y)
		m.marcar(ancho-1, y)
		if y < ancho {
			m.marcar(y, y)
		}
	}
	return m
}

// TestJBIG2RegionGenericaIdaYVuelta comprueba las CUATRO plantillas. El fichero
// elige la que quiere y no avisa: usar la equivocada no da error, da ruido.
func TestJBIG2RegionGenericaIdaYVuelta(t *testing.T) {
	for plantilla := 0; plantilla <= 3; plantilla++ {
		original := mapaDePrueba(32, 24)
		flujo := jbig2DeMapa(original, plantilla)

		bits, err := descomprimirJBIG2(flujo, nil, 32, 24)
		if err != nil {
			t.Errorf("plantilla %d: %v", plantilla, err)
			continue
		}
		// `empaquetar` invierte el sentido: en JBIG2 el 1 es tinta y en una imagen
		// de grises de un bit el 0 es negro.
		porFila := (32 + 7) / 8
		distintos := 0
		for y := 0; y < 24; y++ {
			for x := 0; x < 32; x++ {
				bit := (bits[y*porFila+x/8] >> uint(7-x%8)) & 1
				quiero := 1 - original.px[y*32+x]
				if bit != quiero {
					distintos++
				}
			}
		}
		if distintos != 0 {
			t.Errorf("plantilla %d: %d píxeles distintos de %d", plantilla, distintos, 32*24)
		}
	}
}

// TestJBIG2InvierteElSentido es un check aparte porque el fallo es MUY visible y
// muy fácil de introducir: el escaneo entero sale en negativo.
func TestJBIG2InvierteElSentido(t *testing.T) {
	// Un mapa todo a 1 (todo tinta) tiene que salir todo a 0 (todo negro).
	m := nuevoMapaBits(16, 4)
	for i := range m.px {
		m.px[i] = 1
	}
	bits, err := descomprimirJBIG2(jbig2DeMapa(m, 0), nil, 16, 4)
	if err != nil {
		t.Fatalf("descomprimirJBIG2: %v", err)
	}
	for i, b := range bits {
		if b != 0 {
			t.Fatalf("byte %d = %#02x: todo tinta debe dar todo ceros (negro)", i, b)
		}
	}
}

// TestJBIG2SegmentoNoSoportadoSeDistingue: un fichero con diccionarios de
// símbolos no es un PDF roto, es uno que este decodificador no cubre. Quien llama
// tiene que poder caer al recuadro gris en vez de rechazar el documento.
func TestJBIG2SegmentoNoSoportadoSeDistingue(t *testing.T) {
	// Un segmento de tipo 0 (diccionario de símbolos) y nada más.
	seg := []byte{0, 0, 0, 1, 0, 0, 1, 0, 0, 0, 4, 1, 2, 3, 4}
	_, err := descomprimirJBIG2(seg, nil, 16, 16)
	if !errors.Is(err, ErrJBIG2NoSoportado) {
		t.Errorf("err = %v, quería ErrJBIG2NoSoportado", err)
	}
}

// TestJBIG2NoSeAtascaConBasura: el flujo es binario y viene de fuera.
func TestJBIG2NoSeAtascaConBasura(t *testing.T) {
	hecho := make(chan struct{})
	go func() {
		defer close(hecho)
		for _, basura := range [][]byte{
			{},
			{0},
			{0xFF, 0xFF, 0xFF, 0xFF},
			make([]byte, 200),
			[]byte("no soy un flujo jbig2 en absoluto"),
			// Una cabecera plausible con longitud absurda.
			{0, 0, 0, 1, 38, 0, 1, 0xFF, 0xFF, 0xFF, 0xFF},
		} {
			_, _ = descomprimirJBIG2(basura, nil, 64, 64)
		}
	}()
	select {
	case <-hecho:
	case <-timeoutCorto():
		t.Fatal("la decodificación de JBIG2 se colgó")
	}
}

// TestJBIG2EnUnaPagina comprueba el cableado completo: un XObject con
// /Filter /JBIG2Decode tiene que acabar en un <image> y no en el recuadro gris.
func TestJBIG2EnUnaPagina(t *testing.T) {
	flujo := jbig2DeMapa(mapaDePrueba(24, 16), 0)
	svg := renderizar(t, pdfConContenido(t,
		"q 80 0 0 60 10 20 cm /Im0 Do Q",
		`/Resources << /XObject << /Im0 5 0 R >> >>`,
		fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 24 /Height 16 "+
			"/ColorSpace /DeviceGray /BitsPerComponent 1 /Filter /JBIG2Decode "+
			"/Length %d >>\nstream\n%s\nendstream", len(flujo), string(flujo))))

	if !strings.Contains(svg, "<image ") {
		t.Fatalf("el JBIG2 no se pintó: %s", primerasLineas(svg))
	}
	if strings.Contains(svg, "#e5e5e5") {
		t.Errorf("cayó al recuadro gris de «no sé decodificar esto»")
	}
}

// Cada región se compone con SU operador (XOR aquí): la misma región dos veces
// con XOR deja la página en blanco. Antes todo era OR y quedaba la región.
func TestJBIG2ComponeConElOperadorDeLaRegion(t *testing.T) {
	m := mapaDePrueba(16, 8)
	seg := jbig2DeMapa(m, 0)
	xor := append([]byte(nil), seg...)
	xor[11+16] = 2 // operador de composición de la segunda: XOR
	bits, err := descomprimirJBIG2(append(append([]byte(nil), seg...), xor...), nil, 16, 8)
	if err != nil {
		t.Fatal(err)
	}
	for i, b := range bits {
		if b != 0xFF {
			t.Fatalf("byte %d = %08b, quería la página en blanco", i, b)
		}
	}
}

// Una región de longitud desconocida (0xFFFFFFFF) acaba en 0xFF 0xAC y el
// número de filas, y DETRÁS puede haber más segmentos. Antes se dejaba de leer
// ahí: aquí la segunda región (la misma con XOR) no se aplicaba.
func TestJBIG2RegionDeLongitudDesconocida(t *testing.T) {
	m := mapaDePrueba(16, 8)
	seg := jbig2DeMapa(m, 0)
	xor := append([]byte(nil), seg...)
	xor[11+16] = 2
	copy(seg[7:11], []byte{0xFF, 0xFF, 0xFF, 0xFF})
	// El terminador del MQ ya es 0xFF 0xAC: detrás va el número de filas.
	if !bytes.HasSuffix(seg, []byte{0xFF, 0xAC}) {
		seg = append(seg, 0xFF, 0xAC)
	}
	seg = append(seg, 0, 0, 0, 8)
	bits, err := descomprimirJBIG2(append(seg, xor...), nil, 16, 8)
	if err != nil {
		t.Fatal(err)
	}
	for i, b := range bits {
		if b != 0xFF {
			t.Fatalf("byte %d = %08b, quería la página en blanco", i, b)
		}
	}
}

// Una región genérica MMR es CCITT G4 dentro de JBIG2. Antes se rechazaba y la
// página caía al recuadro gris.
func TestJBIG2RegionGenericaMMR(t *testing.T) {
	// Una fila de 8: cuatro blancos y cuatro negros (001 horizontal, 1011, 011).
	g4 := bitsAByte(t, "001"+"1011"+"011")
	be := func(v int) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }
	cuerpo := append(append(append(append(be(8), be(1)...), be(0)...), be(0)...), 0, 1) // op OR, MMR
	cuerpo = append(cuerpo, g4...)
	seg := append(append(be(1), 38, 0, 1), be(len(cuerpo))...)
	bits, err := descomprimirJBIG2(append(seg, cuerpo...), nil, 8, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(bits) != 1 || bits[0] != 0xF0 {
		t.Fatalf("fila = %08b, quería 11110000", bits)
	}
}
