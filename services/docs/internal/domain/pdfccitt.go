// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Decodificador de fax CCITT (ITU-T T.4 y T.6), que en PDF es el filtro
// `CCITTFaxDecode`.
//
// POR QUÉ ESTO IMPORTA MÁS QUE NINGÚN OTRO FILTRO: es el formato de casi todo lo
// ESCANEADO. Un expediente, un contrato firmado a mano, una factura de hace diez
// años — todo eso llega como fax G4 dentro de un PDF, y sin decodificarlo el
// visor enseña un recuadro gris donde está el documento entero. Es la diferencia
// entre «tengo un visor de PDF» y «tengo un visor de MIS PDF».
//
// CÓMO FUNCIONA, en dos frases. Una página de fax es blanco y negro y se
// describe por RACHAS: «120 píxeles blancos, 3 negros, 40 blancos…», y cada
// longitud de racha tiene su código Huffman (tablas de T.4). El modo 2D —el que
// usa G4— aprovecha además que una línea se parece a la anterior: en vez de
// codificar sus rachas, codifica en qué se DIFERENCIA de la línea de referencia.
// Por eso una página de texto escaneada baja de megabytes a decenas de kilobytes.
//
// Se implementan las tres variantes que existen en la práctica: G4 (K < 0), G3
// unidimensional (K = 0) y G3 mixto (K > 0). Sin dependencias.
package domain

import "errors"

// ErrCCITT indica un flujo de fax que no se puede decodificar.
var ErrCCITT = errors.New("pdf: flujo CCITT ilegible")

// parametrosCCITT son las opciones de /DecodeParms del filtro (§7.4.6).
type parametrosCCITT struct {
	K            int
	Columnas     int
	Filas        int
	NegroEs1     bool
	AlinearBytes bool
	MaxFilas     int
}

// maxColumnasCCITT acota el ancho declarado. La especificación no pone techo y
// un valor absurdo reservaría gigabytes por línea.
const maxColumnasCCITT = 1 << 16

// leerParametrosCCITT extrae las opciones con sus valores por defecto, que NO
// son los que uno supondría: /Columns vale 1728 —el ancho de un fax de papel A4—
// y no el ancho de la imagen, así que omitirlo y usar /Width desalinea la página
// entera.
func (l *lienzo) leerParametrosCCITT(st *pdfStream, indiceFiltro int) parametrosCCITT {
	p := parametrosCCITT{Columnas: 1728}
	if _, alto := l.tamanoImagen(st.Dict); alto > 0 {
		p.Filas = alto
		p.MaxFilas = alto
	}
	parms := l.d.parmsDeFiltro(st, indiceFiltro)
	if parms == nil {
		return p
	}
	if v, ok := comoEntero(l.d.resolver(parms["K"])); ok {
		p.K = int(v)
	}
	if v, ok := comoEntero(l.d.resolver(parms["Columns"])); ok && v > 0 && v <= maxColumnasCCITT {
		p.Columnas = int(v)
	}
	if v, ok := comoEntero(l.d.resolver(parms["Rows"])); ok && v > 0 {
		p.Filas = int(v)
	}
	if v, ok := l.d.resolver(parms["BlackIs1"]).(bool); ok {
		p.NegroEs1 = v
	}
	if v, ok := l.d.resolver(parms["EncodedByteAlign"]).(bool); ok {
		p.AlinearBytes = v
	}
	return p
}

// --- Lector de bits ----------------------------------------------------------

type lectorBits struct {
	datos []byte
	pos   int // en bits
}

func (r *lectorBits) fin() bool { return r.pos >= len(r.datos)*8 }

func (r *lectorBits) bit() int {
	if r.fin() {
		return -1
	}
	b := (r.datos[r.pos/8] >> uint(7-r.pos%8)) & 1
	r.pos++
	return int(b)
}

// alinear salta al siguiente byte, que es lo que pide /EncodedByteAlign.
func (r *lectorBits) alinear() {
	if n := r.pos % 8; n != 0 {
		r.pos += 8 - n
	}
}

// mirar devuelve los siguientes n bits SIN consumirlos, para reconocer marcas
// como el fin de línea sin perder la posición si no estaban.
func (r *lectorBits) mirar(n int) (int, bool) {
	if r.pos+n > len(r.datos)*8 {
		return 0, false
	}
	v := 0
	for i := 0; i < n; i++ {
		p := r.pos + i
		v = v<<1 | int((r.datos[p/8]>>uint(7-p%8))&1)
	}
	return v, true
}

// --- Tablas de códigos -------------------------------------------------------

// clave empaqueta longitud y valor de un código para poder buscarlo en un mapa.
// Los códigos de T.4 son un código PREFIJO, así que leer bit a bit y consultar
// tras cada uno encuentra siempre el correcto y nunca uno más largo por error.
func clave(bits, valor int) int { return bits<<16 | valor }

var (
	codigosBlanco = map[int]int{}
	codigosNegro  = map[int]int{}
)

// tablaBlanco y tablaNegro son las de T.4, escritas como el código en binario y
// la longitud de racha que representa.
var tablaBlanco = [][2]any{
	{"00110101", 0},
	{"000111", 1},
	{"0111", 2},
	{"1000", 3},
	{"1011", 4},
	{"1100", 5},
	{"1110", 6},
	{"1111", 7},
	{"10011", 8},
	{"10100", 9},
	{"00111", 10},
	{"01000", 11},
	{"001000", 12},
	{"000011", 13},
	{"110100", 14},
	{"110101", 15},
	{"101010", 16},
	{"101011", 17},
	{"0100111", 18},
	{"0001100", 19},
	{"0001000", 20},
	{"0010111", 21},
	{"0000011", 22},
	{"0000100", 23},
	{"0101000", 24},
	{"0101011", 25},
	{"0010011", 26},
	{"0100100", 27},
	{"0011000", 28},
	{"00000010", 29},
	{"00000011", 30},
	{"00011010", 31},
	{"00011011", 32},
	{"00010010", 33},
	{"00010011", 34},
	{"00010100", 35},
	{"00010101", 36},
	{"00010110", 37},
	{"00010111", 38},
	{"00101000", 39},
	{"00101001", 40},
	{"00101010", 41},
	{"00101011", 42},
	{"00101100", 43},
	{"00101101", 44},
	{"00000100", 45},
	{"00000101", 46},
	{"00001010", 47},
	{"00001011", 48},
	{"01010010", 49},
	{"01010011", 50},
	{"01010100", 51},
	{"01010101", 52},
	{"00100100", 53},
	{"00100101", 54},
	{"01011000", 55},
	{"01011001", 56},
	{"01011010", 57},
	{"01011011", 58},
	{"01001010", 59},
	{"01001011", 60},
	{"00110010", 61},
	{"00110011", 62},
	{"00110100", 63},
	// Códigos de composición: se suman a uno terminal para pasar de 63.
	{"11011", 64},
	{"10010", 128},
	{"010111", 192},
	{"0110111", 256},
	{"00110110", 320},
	{"00110111", 384},
	{"01100100", 448},
	{"01100101", 512},
	{"01101000", 576},
	{"01100111", 640},
	{"011001100", 704},
	{"011001101", 768},
	{"011010010", 832},
	{"011010011", 896},
	{"011010100", 960},
	{"011010101", 1024},
	{"011010110", 1088},
	{"011010111", 1152},
	{"011011000", 1216},
	{"011011001", 1280},
	{"011011010", 1344},
	{"011011011", 1408},
	{"010011000", 1472},
	{"010011001", 1536},
	{"010011010", 1600},
	{"011000", 1664},
	{"010011011", 1728},
}

var tablaNegro = [][2]any{
	{"0000110111", 0},
	{"010", 1},
	{"11", 2},
	{"10", 3},
	{"011", 4},
	{"0011", 5},
	{"0010", 6},
	{"00011", 7},
	{"000101", 8},
	{"000100", 9},
	{"0000100", 10},
	{"0000101", 11},
	{"0000111", 12},
	{"00000100", 13},
	{"00000111", 14},
	{"000011000", 15},
	{"0000010111", 16},
	{"0000011000", 17},
	{"0000001000", 18},
	{"00001100111", 19},
	{"00001101000", 20},
	{"00001101100", 21},
	{"00000110111", 22},
	{"00000101000", 23},
	{"00000010111", 24},
	{"00000011000", 25},
	{"000011001010", 26},
	{"000011001011", 27},
	{"000011001100", 28},
	{"000011001101", 29},
	{"000001101000", 30},
	{"000001101001", 31},
	{"000001101010", 32},
	{"000001101011", 33},
	{"000011010010", 34},
	{"000011010011", 35},
	{"000011010100", 36},
	{"000011010101", 37},
	{"000011010110", 38},
	{"000011010111", 39},
	{"000001101100", 40},
	{"000001101101", 41},
	{"000011011010", 42},
	{"000011011011", 43},
	{"000001010100", 44},
	{"000001010101", 45},
	{"000001010110", 46},
	{"000001010111", 47},
	{"000001100100", 48},
	{"000001100101", 49},
	{"000001010010", 50},
	{"000001010011", 51},
	{"000000100100", 52},
	{"000000110111", 53},
	{"000000111000", 54},
	{"000000100111", 55},
	{"000000101000", 56},
	{"000001011000", 57},
	{"000001011001", 58},
	{"000000101011", 59},
	{"000000101100", 60},
	{"000001011010", 61},
	{"000001100110", 62},
	{"000001100111", 63},
	{"0000001111", 64},
	{"000011001000", 128},
	{"000011001001", 192},
	{"000001011011", 256},
	{"000000110011", 320},
	{"000000110100", 384},
	{"000000110101", 448},
	{"0000001101100", 512},
	{"0000001101101", 576},
	{"0000001001010", 640},
	{"0000001001011", 704},
	{"0000001001100", 768},
	{"0000001001101", 832},
	{"0000001110010", 896},
	{"0000001110011", 960},
	{"0000001110100", 1024},
	{"0000001110101", 1088},
	{"0000001110110", 1152},
	{"0000001110111", 1216},
	{"0000001010010", 1280},
	{"0000001010011", 1344},
	{"0000001010100", 1408},
	{"0000001010101", 1472},
	{"0000001011010", 1536},
	{"0000001011011", 1600},
	{"0000001100100", 1664},
	{"0000001100101", 1728},
}

// tablaComun son los códigos de composición extendidos, iguales para blanco y
// negro (T.4 §4.1.4).
var tablaComun = [][2]any{
	{"00000001000", 1792},
	{"00000001100", 1856},
	{"00000001101", 1920},
	{"000000010010", 1984},
	{"000000010011", 2048},
	{"000000010100", 2112},
	{"000000010101", 2176},
	{"000000010110", 2240},
	{"000000010111", 2304},
	{"000000011100", 2368},
	{"000000011101", 2432},
	{"000000011110", 2496},
	{"000000011111", 2560},
}

func init() {
	// Las aserciones NO se comprueban a propósito: las filas son literales de ESTE
	// fichero, escritos a mano desde la norma T.4. Un tipo equivocado ahí es un
	// error de programación que debe salir en el primer arranque, no un dato de
	// fuera que haya que validar.
	cargar := func(dst map[int]int, filas [][2]any) {
		for _, f := range filas {
			bits, _ := f[0].(string)
			valor, _ := f[1].(int)
			v := 0
			for _, c := range bits {
				v <<= 1
				if c == '1' {
					v |= 1
				}
			}
			dst[clave(len(bits), v)] = valor
		}
	}
	cargar(codigosBlanco, tablaBlanco)
	cargar(codigosBlanco, tablaComun)
	cargar(codigosNegro, tablaNegro)
	cargar(codigosNegro, tablaComun)
}

// maxBitsCodigo es el código más largo de las tablas. Pasado ese punto, seguir
// leyendo solo puede ser basura.
const maxBitsCodigo = 14

// leerRacha lee una longitud de racha completa: los códigos de composición se
// van sumando hasta que llega uno terminal (< 64).
func leerRacha(r *lectorBits, blanco bool) (int, bool) {
	tabla := codigosNegro
	if blanco {
		tabla = codigosBlanco
	}
	total := 0
	for vueltas := 0; vueltas < 64; vueltas++ {
		bits, valor := 0, 0
		encontrado := false
		for bits < maxBitsCodigo {
			b := r.bit()
			if b < 0 {
				return total, false
			}
			valor = valor<<1 | b
			bits++
			if n, ok := tabla[clave(bits, valor)]; ok {
				total += n
				encontrado = true
				// Terminal: la racha termina aquí. De composición: sigue.
				if n < 64 {
					return total, true
				}
				break
			}
		}
		if !encontrado {
			return total, false
		}
	}
	return total, false
}

// --- Decodificación ----------------------------------------------------------

// descomprimirCCITT devuelve la imagen en 1 bit por píxel, empaquetada por filas
// como espera el resto del lector: **0 = negro**, que es el valor por defecto de
// /BlackIs1 en PDF.
func descomprimirCCITT(datos []byte, p parametrosCCITT) ([]byte, error) {
	out, _, err := descomprimirCCITTHasta(datos, p)
	return out, err
}

// descomprimirCCITTHasta es descomprimirCCITT y además dice cuántos bytes
// consumió, con la marca de fin de bloque (EOFB) si la hay. Lo necesitan los
// planos de bits MMR de JBIG2, que van seguidos sin longitud (T.88, 6.2.6).
func descomprimirCCITTHasta(datos []byte, p parametrosCCITT) ([]byte, int, error) {
	if p.Columnas <= 0 || p.Columnas > maxColumnasCCITT {
		return nil, 0, ErrCCITT
	}
	filasMax := p.Filas
	if filasMax <= 0 {
		filasMax = 1 << 16 // sin /Rows se lee hasta que se acabe el flujo
	}
	if p.MaxFilas > 0 && filasMax > p.MaxFilas {
		filasMax = p.MaxFilas
	}

	r := &lectorBits{datos: datos}
	bytesPorFila := (p.Columnas + 7) / 8
	out := make([]byte, 0, bytesPorFila*minEntero(filasMax, 4096))

	// referencia son las posiciones de cambio de color de la línea anterior. La
	// primera línea imaginaria es toda blanca, o sea sin ningún cambio.
	referencia := []int{p.Columnas, p.Columnas}

	for fila := 0; fila < filasMax; fila++ {
		if p.AlinearBytes {
			r.alinear()
		}
		saltarFinDeLinea(r)
		if r.fin() {
			break
		}

		dosDimensiones := p.K < 0
		if p.K > 0 {
			// G3 mixto: un bit tras el fin de línea dice si ESTA línea es 1D o 2D.
			b := r.bit()
			if b < 0 {
				break
			}
			dosDimensiones = b == 0
		}

		var (
			cambios []int
			err     error
		)
		if dosDimensiones {
			cambios, err = decodificarFila2D(r, referencia, p.Columnas)
		} else {
			cambios, err = decodificarFila1D(r, p.Columnas)
		}
		if err != nil {
			if fila == 0 {
				return nil, 0, err
			}
			break // una línea rota al final no invalida lo ya decodificado
		}

		out = append(out, pintarFila(cambios, p.Columnas, p.NegroEs1)...)
		// Se COPIA en vez de extender `cambios`: reutilizar su array haría que la
		// fila siguiente leyera posiciones ya pisadas, y el fax saldría rayado.
		referencia = append(append([]int{}, cambios...), p.Columnas, p.Columnas)
	}

	if len(out) == 0 {
		return nil, 0, ErrCCITT
	}
	saltarFinDeLinea(r) // EOFB son dos EOL
	consumidos := minEntero((r.pos+7)/8, len(datos))
	// Se completa hasta el alto declarado con blanco: una página cortada se
	// enseña entera con lo que falta en blanco, que es como la ve un fax.
	if p.MaxFilas > 0 {
		relleno := byte(0xFF) // 1 = blanco cuando NegroEs1 es falso
		if p.NegroEs1 {
			relleno = 0x00
		}
		for len(out) < bytesPorFila*p.MaxFilas {
			out = append(out, relleno)
		}
	}
	return out, consumidos, nil
}

func minEntero(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// saltarFinDeLinea consume la marca EOL (000000000001) y sus bits de relleno si
// están. Es opcional en G4 y obligatoria en G3, así que se mira sin exigirla.
func saltarFinDeLinea(r *lectorBits) {
	for i := 0; i < 64; i++ {
		v, ok := r.mirar(12)
		if !ok {
			return
		}
		if v == 1 {
			r.pos += 12
			continue
		}
		// Relleno: ceros hasta la EOL. Solo se saltan si detrás hay una de verdad.
		if v == 0 {
			salvado := r.pos
			ceros := 0
			for ceros < 64 {
				if b := r.bit(); b != 0 {
					break
				}
				ceros++
				if w, ok := r.mirar(12); ok && w == 1 {
					r.pos += 12
					break
				}
			}
			if r.pos == salvado+ceros {
				r.pos = salvado // no había EOL detrás: los ceros son datos
				return
			}
			continue
		}
		return
	}
}

// decodificarFila1D lee una línea como rachas alternas empezando por blanco.
func decodificarFila1D(r *lectorBits, columnas int) ([]int, error) {
	var cambios []int
	pos, blanco := 0, true
	for pos < columnas {
		n, ok := leerRacha(r, blanco)
		if !ok {
			if len(cambios) == 0 {
				return nil, ErrCCITT
			}
			break
		}
		pos += n
		if pos > columnas {
			pos = columnas
		}
		cambios = append(cambios, pos)
		blanco = !blanco
	}
	return cambios, nil
}

// decodificarFila2D lee una línea por diferencias con la de referencia, que es
// de donde sale casi toda la compresión de G4.
//
// El vocabulario de T.6, porque sin él el código no se entiende: `a0` es dónde
// va el lápiz; `b1` es el primer cambio de la línea de REFERENCIA a la derecha
// de a0 y de color contrario al actual; `b2` el siguiente. Los modos dicen si la
// línea nueva cambia justo donde la de arriba (vertical), si se salta un cambio
// entero (paso) o si hay que escribir dos rachas a mano (horizontal).
func decodificarFila2D(r *lectorBits, referencia []int, columnas int) ([]int, error) {
	var cambios []int
	a0, blanco := -1, true

	for a0 < columnas {
		b1 := buscarB1(referencia, a0, blanco)
		b2 := columnas
		if i := indiceDe(referencia, b1); i+1 < len(referencia) {
			b2 = referencia[i+1]
		}
		if b2 > columnas {
			b2 = columnas
		}

		modo, delta, ok := leerModo2D(r)
		if !ok {
			if len(cambios) == 0 {
				return nil, ErrCCITT
			}
			break
		}

		switch modo {
		case modoPaso:
			a0 = b2

		case modoHorizontal:
			inicio := a0
			if inicio < 0 {
				inicio = 0
			}
			r1, ok1 := leerRacha(r, blanco)
			r2, ok2 := leerRacha(r, !blanco)
			if !ok1 || !ok2 {
				return cambios, nil
			}
			a1 := acotarPos(inicio+r1, columnas)
			a2 := acotarPos(a1+r2, columnas)
			cambios = append(cambios, a1, a2)
			a0 = a2

		case modoVertical:
			a1 := acotarPos(b1+delta, columnas)
			cambios = append(cambios, a1)
			a0 = a1
			blanco = !blanco

		default: // extensión o fin de bloque: aquí se para
			return cambios, nil
		}
		if len(cambios) > 2*columnas+4 {
			return nil, ErrCCITT // línea que no avanza: flujo corrupto
		}
	}
	return cambios, nil
}

const (
	modoPaso = iota
	modoHorizontal
	modoVertical
	modoDesconocido
)

// leerModo2D reconoce el código de modo. Son códigos prefijo cortos, así que se
// leen bit a bit igual que las rachas.
func leerModo2D(r *lectorBits) (modo, delta int, ok bool) {
	// V0
	b := r.bit()
	if b < 0 {
		return 0, 0, false
	}
	if b == 1 {
		return modoVertical, 0, true
	}
	b = r.bit()
	if b < 0 {
		return 0, 0, false
	}
	if b == 1 { // 01x
		b = r.bit()
		if b < 0 {
			return 0, 0, false
		}
		if b == 1 {
			return modoVertical, 1, true // 011 = VR1
		}
		return modoVertical, -1, true // 010 = VL1
	}
	b = r.bit()
	if b < 0 {
		return 0, 0, false
	}
	if b == 1 {
		return modoHorizontal, 0, true // 001
	}
	b = r.bit()
	if b < 0 {
		return 0, 0, false
	}
	if b == 1 {
		return modoPaso, 0, true // 0001
	}
	b = r.bit()
	if b < 0 {
		return 0, 0, false
	}
	if b == 1 { // 00001x
		b = r.bit()
		if b < 0 {
			return 0, 0, false
		}
		if b == 1 {
			return modoVertical, 2, true // 000011 = VR2
		}
		return modoVertical, -2, true // 000010 = VL2
	}
	b = r.bit()
	if b < 0 {
		return 0, 0, false
	}
	if b == 1 { // 000001x
		b = r.bit()
		if b < 0 {
			return 0, 0, false
		}
		if b == 1 {
			return modoVertical, 3, true // 0000011 = VR3
		}
		return modoVertical, -3, true // 0000010 = VL3
	}
	return modoDesconocido, 0, true
}

// buscarB1 localiza el primer cambio de la línea de referencia a la derecha de
// a0 con color CONTRARIO al actual. Los cambios de una línea alternan colores
// empezando por blanco→negro, así que el color de referencia[i] lo decide la
// paridad de i: par = cambia a negro, impar = cambia a blanco.
func buscarB1(referencia []int, a0 int, blanco bool) int {
	for i, c := range referencia {
		if c <= a0 {
			continue
		}
		// Se busca un cambio «a no-blanco» cuando el color actual es blanco.
		if (i%2 == 0) == blanco {
			return c
		}
	}
	if len(referencia) > 0 {
		return referencia[len(referencia)-1]
	}
	return a0 + 1
}

func indiceDe(referencia []int, valor int) int {
	for i, c := range referencia {
		if c == valor {
			return i
		}
	}
	return len(referencia)
}

func acotarPos(v, columnas int) int {
	if v < 0 {
		return 0
	}
	if v > columnas {
		return columnas
	}
	return v
}

// pintarFila convierte las posiciones de cambio en bits. Las rachas alternan
// empezando por BLANCO.
func pintarFila(cambios []int, columnas int, negroEs1 bool) []byte {
	bytesPorFila := (columnas + 7) / 8
	fila := make([]byte, bytesPorFila)
	// Se parte de todo blanco y se pintan las rachas negras.
	if !negroEs1 {
		for i := range fila {
			fila[i] = 0xFF // 1 = blanco
		}
	}
	pos, blanco := 0, true
	for _, c := range cambios {
		if c > columnas {
			c = columnas
		}
		if !blanco {
			for x := pos; x < c; x++ {
				if negroEs1 {
					fila[x/8] |= 1 << uint(7-x%8)
				} else {
					fila[x/8] &^= 1 << uint(7-x%8)
				}
			}
		}
		pos = c
		blanco = !blanco
		if pos >= columnas {
			break
		}
	}
	// El resto de la línea conserva el color de la última racha.
	if !blanco && pos < columnas {
		for x := pos; x < columnas; x++ {
			if negroEs1 {
				fila[x/8] |= 1 << uint(7-x%8)
			} else {
				fila[x/8] &^= 1 << uint(7-x%8)
			}
		}
	}
	return fila
}
