// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tablas de Huffman de JBIG2 (T.88, anexo B): las quince estándar, las que trae
// un segmento de tablas (53) y la asignación de códigos de B.3, que también usa
// la tabla de identificadores de símbolo de una región de texto (7.4.3.1.7).
package domain

import "errors"

var errJBIG2Huffman = errors.New("jbig2: código de Huffman inválido")

// lineaHuffman es una línea de tabla: longitud del prefijo, bits que lo siguen
// y valor más bajo. La línea inferior resta el desplazamiento en vez de sumarlo
// y la OOB no tiene valor.
type lineaHuffman struct {
	pref, rango, bajo int
	inferior, oob     bool
}

type tablaHuffman struct {
	lineas  []lineaHuffman
	codigos map[[2]int]int // {longitud, código} → línea
}

// nuevaTablaHuffman asigna los códigos con el algoritmo de B.3: por longitud
// creciente y, dentro de una longitud, en el orden de las líneas.
func nuevaTablaHuffman(lineas []lineaHuffman) *tablaHuffman {
	t := &tablaHuffman{lineas: lineas, codigos: map[[2]int]int{}}
	maxLen := 0
	cuenta := map[int]int{}
	for _, l := range lineas {
		cuenta[l.pref]++
		maxLen = max(maxLen, l.pref)
	}
	cuenta[0] = 0
	primero := 0
	for largo := 1; largo <= maxLen && largo <= 32; largo++ {
		primero = (primero + cuenta[largo-1]) << 1
		cod := primero
		for i, l := range lineas {
			if l.pref == largo {
				t.codigos[[2]int{largo, cod}] = i
				cod++
			}
		}
	}
	return t
}

// leerN lee n bits (n ≤ 32) como entero sin signo.
func leerN(r *lectorBits, n int) (int, error) {
	v := 0
	for i := 0; i < n; i++ {
		b := r.bit()
		if b < 0 {
			return 0, errJBIG2Huffman
		}
		v = v<<1 | b
	}
	return v, nil
}

// leer es B.4. Devuelve oob = true para el valor fuera de banda.
func (t *tablaHuffman) leer(r *lectorBits) (v int, oob bool, err error) {
	cod := 0
	for largo := 1; largo <= 32; largo++ {
		b := r.bit()
		if b < 0 {
			return 0, false, errJBIG2Huffman
		}
		cod = cod<<1 | b
		i, ok := t.codigos[[2]int{largo, cod}]
		if !ok {
			continue
		}
		l := t.lineas[i]
		if l.oob {
			return 0, true, nil
		}
		d, err := leerN(r, l.rango)
		if err != nil {
			return 0, false, err
		}
		if l.inferior {
			return l.bajo - d, false, nil
		}
		return l.bajo + d, false, nil
	}
	return 0, false, errJBIG2Huffman
}

func lh(p, r, b int) lineaHuffman { return lineaHuffman{pref: p, rango: r, bajo: b} }
func lhInf(p, b int) lineaHuffman { return lineaHuffman{pref: p, rango: 32, bajo: b, inferior: true} }
func lhOOB(p int) lineaHuffman    { return lineaHuffman{pref: p, oob: true} }

// tablasEstandar son B.1 a B.15 (índice 1 a 15), copiadas de la norma con las
// líneas en su orden: el orden decide qué código recibe cada una.
var tablasEstandar = [16]*tablaHuffman{
	1: nuevaTablaHuffman([]lineaHuffman{lh(1, 4, 0), lh(2, 8, 16), lh(3, 16, 272), lh(3, 32, 65808)}),
	2: nuevaTablaHuffman([]lineaHuffman{lh(1, 0, 0), lh(2, 0, 1), lh(3, 0, 2), lh(4, 3, 3), lh(5, 6, 11), lh(6, 32, 75), lhOOB(6)}),
	3: nuevaTablaHuffman([]lineaHuffman{
		lh(8, 8, -256), lh(1, 0, 0), lh(2, 0, 1), lh(3, 0, 2), lh(4, 3, 3), lh(5, 6, 11),
		lhInf(8, -257), lh(7, 32, 75), lhOOB(6),
	}),
	4: nuevaTablaHuffman([]lineaHuffman{lh(1, 0, 1), lh(2, 0, 2), lh(3, 0, 3), lh(4, 3, 4), lh(5, 6, 12), lh(5, 32, 76)}),
	5: nuevaTablaHuffman([]lineaHuffman{
		lh(7, 8, -255), lh(1, 0, 1), lh(2, 0, 2), lh(3, 0, 3), lh(4, 3, 4), lh(5, 6, 12),
		lhInf(7, -256), lh(6, 32, 76),
	}),
	6: nuevaTablaHuffman([]lineaHuffman{
		lh(5, 10, -2048), lh(4, 9, -1024), lh(4, 8, -512), lh(4, 7, -256), lh(5, 6, -128), lh(5, 5, -64),
		lh(4, 5, -32), lh(2, 7, 0), lh(3, 7, 128), lh(3, 8, 256), lh(4, 9, 512), lh(4, 10, 1024),
		lhInf(6, -2049), lh(6, 32, 2048),
	}),
	7: nuevaTablaHuffman([]lineaHuffman{
		lh(4, 9, -1024), lh(3, 8, -512), lh(4, 7, -256), lh(5, 6, -128), lh(5, 5, -64), lh(4, 5, -32),
		lh(4, 5, 0), lh(5, 5, 32), lh(5, 6, 64), lh(4, 7, 128), lh(3, 8, 256), lh(3, 9, 512), lh(3, 10, 1024),
		lhInf(5, -1025), lh(5, 32, 2048),
	}),
	8: nuevaTablaHuffman([]lineaHuffman{
		lh(8, 3, -15), lh(9, 1, -7), lh(8, 1, -5), lh(9, 0, -3), lh(7, 0, -2), lh(4, 0, -1), lh(2, 1, 0),
		lh(5, 0, 2), lh(6, 0, 3), lh(3, 4, 4), lh(6, 1, 20), lh(4, 4, 22), lh(4, 5, 38), lh(5, 6, 70),
		lh(5, 7, 134), lh(6, 7, 262), lh(7, 8, 390), lh(6, 10, 646),
		lhInf(9, -16), lh(9, 32, 1670), lhOOB(2),
	}),
	9: nuevaTablaHuffman([]lineaHuffman{
		lh(8, 4, -31), lh(9, 2, -15), lh(8, 2, -11), lh(9, 1, -7), lh(7, 1, -5), lh(4, 1, -3), lh(3, 1, -1),
		lh(3, 1, 1), lh(5, 1, 3), lh(6, 1, 5), lh(3, 5, 7), lh(6, 2, 39), lh(4, 5, 43), lh(4, 6, 75),
		lh(5, 7, 139), lh(5, 8, 267), lh(6, 8, 523), lh(7, 9, 779), lh(6, 11, 1291),
		lhInf(9, -32), lh(9, 32, 3339), lhOOB(2),
	}),
	10: nuevaTablaHuffman([]lineaHuffman{
		lh(7, 4, -21), lh(8, 0, -5), lh(7, 0, -4), lh(5, 0, -3), lh(2, 2, -2), lh(5, 0, 2), lh(6, 0, 3),
		lh(7, 0, 4), lh(8, 0, 5), lh(2, 6, 6), lh(5, 5, 70), lh(6, 5, 102), lh(6, 6, 134), lh(6, 7, 198),
		lh(6, 8, 326), lh(6, 9, 582), lh(6, 10, 1094), lh(7, 11, 2118),
		lhInf(8, -22), lh(8, 32, 4166), lhOOB(2),
	}),
	11: nuevaTablaHuffman([]lineaHuffman{
		lh(1, 0, 1), lh(2, 1, 2), lh(4, 0, 4), lh(4, 1, 5), lh(5, 1, 7), lh(5, 2, 9), lh(6, 2, 13),
		lh(7, 2, 17), lh(7, 3, 21), lh(7, 4, 29), lh(7, 5, 45), lh(7, 6, 77), lh(7, 32, 141),
	}),
	12: nuevaTablaHuffman([]lineaHuffman{
		lh(1, 0, 1), lh(2, 0, 2), lh(3, 1, 3), lh(5, 0, 5), lh(5, 1, 6), lh(6, 1, 8), lh(7, 0, 10),
		lh(7, 1, 11), lh(7, 2, 13), lh(7, 3, 17), lh(7, 4, 25), lh(8, 5, 41), lh(8, 32, 73),
	}),
	13: nuevaTablaHuffman([]lineaHuffman{
		lh(1, 0, 1), lh(3, 0, 2), lh(4, 0, 3), lh(5, 0, 4), lh(4, 1, 5), lh(3, 3, 7), lh(6, 1, 15),
		lh(6, 2, 17), lh(6, 3, 21), lh(6, 4, 29), lh(6, 5, 45), lh(7, 6, 77), lh(7, 32, 141),
	}),
	14: nuevaTablaHuffman([]lineaHuffman{lh(3, 0, -2), lh(3, 0, -1), lh(1, 0, 0), lh(3, 0, 1), lh(3, 0, 2)}),
	15: nuevaTablaHuffman([]lineaHuffman{
		lh(7, 4, -24), lh(6, 2, -8), lh(5, 1, -4), lh(4, 0, -2), lh(3, 0, -1), lh(1, 0, 0), lh(3, 0, 1),
		lh(4, 0, 2), lh(5, 1, 3), lh(6, 2, 5), lh(7, 4, 9), lhInf(7, -25), lh(7, 32, 25),
	}),
}

// leerTablaHuffman es el segmento de tablas (53, anexo B.2).
func leerTablaHuffman(cuerpo []byte) (*tablaHuffman, error) {
	if len(cuerpo) < 9 {
		return nil, ErrJBIG2NoSoportado
	}
	f := cuerpo[0]
	conOOB, ps, rs := f&1 != 0, int(f>>1&7)+1, int(f>>4&7)+1
	be := func(i int) int {
		return int(int32(uint32(cuerpo[i])<<24 | uint32(cuerpo[i+1])<<16 | uint32(cuerpo[i+2])<<8 | uint32(cuerpo[i+3]))) //nolint:gosec // G115: con signo (B.2.2)
	}
	bajo, alto := be(1), be(5)
	r := &lectorBits{datos: cuerpo[9:]}
	var lineas []lineaHuffman
	for cur := bajo; cur < alto; {
		p, err := leerN(r, ps)
		if err != nil {
			return nil, err
		}
		rango, err := leerN(r, rs)
		if err != nil || rango > 32 || len(lineas) > 1<<12 {
			return nil, ErrJBIG2NoSoportado
		}
		lineas = append(lineas, lh(p, rango, cur))
		cur += 1 << rango
	}
	pInf, err1 := leerN(r, ps)
	pSup, err2 := leerN(r, ps)
	if err1 != nil || err2 != nil {
		return nil, ErrJBIG2NoSoportado
	}
	lineas = append(lineas, lhInf(pInf, bajo-1), lh(pSup, 32, alto))
	if conOOB {
		p, err := leerN(r, ps)
		if err != nil {
			return nil, err
		}
		lineas = append(lineas, lhOOB(p))
	}
	return nuevaTablaHuffman(lineas), nil
}

// tablaDeLongitudes asigna códigos de B.3 a valores 0…n-1 con esas longitudes
// (0 = sin código): los identificadores de símbolo de una región de texto.
func tablaDeLongitudes(largos []int) *tablaHuffman {
	lineas := make([]lineaHuffman, len(largos))
	for i, l := range largos {
		lineas[i] = lh(l, 0, i)
	}
	return nuevaTablaHuffman(lineas)
}

// leerCodigosID es 7.4.3.1.7: las longitudes de los códigos de símbolo van en
// rachas con su propio código de Huffman, parecido a zlib.
func leerCodigosID(r *lectorBits, n int) (*tablaHuffman, error) {
	runas := make([]int, 35)
	for i := range runas {
		v, err := leerN(r, 4)
		if err != nil {
			return nil, err
		}
		runas[i] = v
	}
	tRunas := tablaDeLongitudes(runas)
	largos := make([]int, 0, n)
	for len(largos) < n {
		c, _, err := tRunas.leer(r)
		if err != nil {
			return nil, err
		}
		veces, valor := 1, c
		switch c {
		case 32:
			if len(largos) == 0 {
				return nil, errJBIG2Huffman
			}
			k, err := leerN(r, 2)
			if err != nil {
				return nil, err
			}
			veces, valor = k+3, largos[len(largos)-1]
		case 33:
			k, err := leerN(r, 3)
			if err != nil {
				return nil, err
			}
			veces, valor = k+3, 0
		case 34:
			k, err := leerN(r, 7)
			if err != nil {
				return nil, err
			}
			veces, valor = k+11, 0
		}
		for i := 0; i < veces && len(largos) < n; i++ {
			largos = append(largos, valor)
		}
	}
	r.alinear()
	return tablaDeLongitudes(largos), nil
}
