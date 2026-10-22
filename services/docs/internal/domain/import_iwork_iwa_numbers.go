// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/binary"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"
)

// # iWork 2013 y posteriores: tablas y hojas .numbers
//
// Tipos y campos observados (y comprobados sobre las 90 hojas del corpus: todas las
// cadenas caen dentro de su tabla, todos los números y fechas salen con valores
// razonables y ninguna celda cae fuera de su rejilla):
//
//   - 1, la raíz de un .numbers (objeto 1): 1 → cada hoja.
//   - 2, una hoja: 1 nombre; 2 → cada objeto de la hoja.
//   - 6000, una tabla en la hoja: 1 su dibujo {1 geometría}; 2 → el modelo.
//   - 6001, el modelo: 6 filas, 7 columnas, 8 nombre; 4 los datos: 3 {1 por tesela
//     {1 número de tesela, 2 → tesela}}, 4 → lista de cadenas, 6 → lista de fórmulas,
//     17 → lista de textos enriquecidos.
//   - 6005, una lista: 3 por entrada {1 clave, 3 cadena; 9 → texto enriquecido (6218)}.
//   - 6218, un texto enriquecido: 1 → su almacén de texto (2001).
//   - 6002, una tesela de 256 filas (la n empieza en la fila 256·n): 5 por fila {1 fila
//     en la tesela; 6 celdas y 7 posiciones (formato actual), o 3 y 4 (anterior); 8, las
//     posiciones van en unidades de 4 bytes}. Las posiciones son uint16 por columna;
//     0xFFFF, celda vacía.
//
// Una celda empieza por su versión y su tipo (2 y 10 número, 3 cadena, 5 fecha, 6
// booleano, 7 duración, 8 error, 9 texto enriquecido) y unas banderas que dicen qué
// valores trae, en orden:
//
//   - versión 5: banderas en el byte 8 y los valores desde el 12: 0x1 número en decimal128,
//     0x2 número en float64, 0x4 fecha (float64, segundos desde 2001), 0x8 cadena (uint32),
//     0x10 texto enriquecido (uint32).
//   - versiones anteriores: banderas en el byte 4; los valores empiezan en el 12 (8 en la
//     1) más 4 bytes por cada bandera de 0x0D8E (0x018E en la 1); 0x200 texto enriquecido,
//     luego 4 bytes por cada bandera de 0x3000 (0x1000), 0x10 cadena, 0x20 número y 0x40
//     fecha.

const (
	tipoIWADocumento = 1
	tipoIWAHoja      = 2
	tipoIWATabla     = 6000
	tipoIWAModelo    = 6001
	tipoIWATesela    = 6002
	tipoIWALista     = 6005
	tipoIWARico      = 6218
	filasPorTesela   = 256
	celdaVacia       = 0xFFFF
)

// listaIWA son las entradas de una lista de la tabla por su clave: la cadena, o el texto
// de su texto enriquecido.
func (l *lectorIWA) listaIWA(id uint64) map[uint64]string {
	out := map[uint64]string{}
	l.doc.de(id, tipoIWALista).cada(3, func(e campoPB) {
		if s, ok := e.b.campo(3); ok {
			out[e.b.uint(1)] = string(s.b)
			return
		}
		if r := l.doc.de(e.b.ref(9), tipoIWARico); r != nil {
			var partes []string
			for _, p := range l.parrafos(r.ref(1)) {
				partes = append(partes, p.plain())
			}
			out[e.b.uint(1)] = strings.Join(partes, "\n")
		}
	})
	return out
}

// tablaIWA lee el modelo de tabla `id`. nil si no hay celdas que se lean.
func (l *lectorIWA) tablaIWA(id uint64) *tabla09 {
	m := l.doc.de(id, tipoIWAModelo)
	nr, nc := acotado(m.uint(6), maxFilasHoja), acotado(m.uint(7), maxColumnasHoja)
	if m == nil || nr <= 0 || nc <= 0 {
		return nil
	}
	ds := m.msg(4)
	t := &tabla09{nombre: string(m.msg(8)), filas: nr, cols: nc, celdas: map[[2]int]celda09{}}
	cadenas, ricos := l.listaIWA(ds.ref(4)), l.listaIWA(ds.ref(17))
	l.doc.de(ds.ref(6), tipoIWALista).cada(3, func(campoPB) { t.formulas++ })
	ds.msg(3).cada(1, func(te campoPB) {
		base := acotado(te.b.uint(1), maxFilasHoja/filasPorTesela) * filasPorTesela
		if base < 0 {
			return
		}
		l.doc.de(te.b.ref(2), tipoIWATesela).cada(5, func(fi campoPB) {
			fila := base + acotado(fi.b.uint(1), filasPorTesela-1)
			if fila < base {
				return
			}
			if fila < nr {
				t.filaIWA(fila, fi.b, cadenas, ricos)
			}
		})
	})
	if len(t.celdas) == 0 {
		return nil
	}
	return t
}

// valoresIWA son los valores que trae una celda, cada uno con si está.
type valoresIWA struct {
	num, seg       float64
	cad, rico      uint64
	hayNum, hayFec bool
	hayCad, hayRic bool
}

// lectorCelda lee valores de una celda sin salirse de ella.
type lectorCelda []byte

func (c lectorCelda) u32(off int) (uint64, bool) {
	if off < 0 || off+4 > len(c) {
		return 0, false
	}
	return uint64(binary.LittleEndian.Uint32(c[off:])), true
}

func (c lectorCelda) f64(off int) (float64, bool) {
	if off < 0 || off+8 > len(c) {
		return 0, false
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(c[off:])), true
}

// valoresV5 lee los valores del formato actual.
func valoresV5(c lectorCelda) (v valoresIWA) {
	flags := binary.LittleEndian.Uint32(c[8:])
	off := 12
	if flags&0x1 != 0 {
		if off+16 <= len(c) {
			v.num, v.hayNum = decimal128(c[off:off+16]), true
		}
		off += 16
	}
	if flags&0x2 != 0 {
		if n, ok := c.f64(off); ok && !v.hayNum {
			v.num, v.hayNum = n, true
		}
		off += 8
	}
	if flags&0x4 != 0 {
		v.seg, v.hayFec = c.f64(off)
		off += 8
	}
	if flags&0x8 != 0 {
		v.cad, v.hayCad = c.u32(off)
		off += 4
	}
	if flags&0x10 != 0 {
		v.rico, v.hayRic = c.u32(off)
	}
	return v
}

// valoresViejos lee los valores de los formatos anteriores al 5.
func valoresViejos(c lectorCelda) (v valoresIWA) {
	flags := binary.LittleEndian.Uint32(c[4:])
	off, previas, intermedias := 12, uint32(0x0D8E), uint32(0x3000)
	if c[0] <= 1 {
		off, previas, intermedias = 8, 0x018E, 0x1000
	}
	off += 4 * bitsEnUno(flags&previas)
	if flags&0x200 != 0 {
		v.rico, v.hayRic = c.u32(off)
		off += 4
	}
	off += 4 * bitsEnUno(flags&intermedias)
	if flags&0x10 != 0 {
		v.cad, v.hayCad = c.u32(off)
		off += 4
	}
	if flags&0x20 != 0 {
		v.num, v.hayNum = c.f64(off)
		off += 8
	}
	if flags&0x40 != 0 {
		v.seg, v.hayFec = c.f64(off)
	}
	return v
}

// filaIWA lee las celdas de una fila de tesela (`fi`).
func (t *tabla09) filaIWA(fila int, fi pb, cadenas, ricos map[uint64]string) {
	celdas, posiciones := fi.msg(6), fi.msg(7)
	if len(posiciones) == 0 {
		celdas, posiciones = fi.msg(3), fi.msg(4)
	}
	ancho := fi.uint(8) != 0
	for col := 0; col*2+1 < len(posiciones) && col < t.cols; col++ {
		o := int(binary.LittleEndian.Uint16(posiciones[col*2:]))
		if o == celdaVacia {
			continue
		}
		if ancho {
			o *= 4
		}
		if o >= len(celdas) {
			continue
		}
		if v := celdaIWA(celdas[o:], cadenas, ricos); v.texto != "" {
			t.celdas[[2]int{fila, col}] = v
		}
	}
}

// celdaIWA lee una celda; vacía si no se entiende.
func celdaIWA(c []byte, cadenas, ricos map[uint64]string) celda09 {
	if len(c) < 12 {
		return celda09{}
	}
	var v valoresIWA
	if c[0] >= 5 {
		v = valoresV5(c)
	} else {
		v = valoresViejos(c)
	}
	switch {
	case (c[1] == 2 || c[1] == 10) && v.hayNum:
		return celda09{texto: strconv.FormatFloat(v.num, 'f', -1, 64), num: true}
	case c[1] == 3 && v.hayCad:
		return celda09{texto: cadenas[v.cad]}
	case c[1] == 5 && v.hayFec:
		return fechaIWA(v.seg)
	case c[1] == 6 && v.hayNum:
		return celda09{texto: strings.ToUpper(strconv.FormatBool(v.num > 0))}
	case c[1] == 7 && v.hayNum:
		// ponytail: una duración sale en días, sin su formato («1 semana» es 7).
		return celda09{texto: strconv.FormatFloat(v.num/86400, 'f', -1, 64), num: true}
	case c[1] == 9 && v.hayRic:
		return celda09{texto: ricos[v.rico]}
	}
	return celda09{}
}

func bitsEnUno(x uint32) int {
	n := 0
	for ; x != 0; x &= x - 1 {
		n++
	}
	return n
}

// fechaIWA es una fecha en segundos desde 2001-01-01.
func fechaIWA(seg float64) celda09 {
	if math.IsNaN(seg) || math.IsInf(seg, 0) || math.Abs(seg) > 1e12 {
		return celda09{}
	}
	t := epoca09.Add(time.Duration(seg * float64(time.Second)))
	texto := t.Format("2006-01-02")
	if t.Hour() != 0 || t.Minute() != 0 {
		texto = t.Format("2006-01-02 15:04")
	}
	return celda09{texto: texto, fecha: true, serial: seg/86400 + serialEpoca09}
}

// decimal128 es un decimal128 little-endian (IEEE 754-2008, codificación binaria) como
// float64: 113 bits de mantisa, 14 de exponente con sesgo 6176 y el signo.
func decimal128(b []byte) float64 {
	exp := (int(b[15]&0x7F)<<7 | int(b[14])>>1) - 6176
	mant := make([]byte, 15)
	for i := range 14 {
		mant[14-i] = b[i]
	}
	mant[0] = b[14] & 1
	f, err := strconv.ParseFloat(new(big.Int).SetBytes(mant).String()+"e"+strconv.Itoa(exp), 64)
	if err != nil {
		return 0
	}
	if b[15]&0x80 != 0 {
		return -f
	}
	return f
}

// importNumbersIWA convierte una hoja .numbers de 2013 en adelante: una hoja del editor
// por tabla, como en '09.
func importNumbersIWA(doc documentoIWA) (m *sheetModel, formulas int, omitido bool) {
	raiz := doc.de(1, tipoIWADocumento)
	l := &lectorIWA{doc: doc}
	m = &sheetModel{}
	nombres := map[string]bool{}
	raiz.cada(1, func(h campoPB) {
		hoja := doc.de(h.b.uint(1), tipoIWAHoja)
		var tablas []*tabla09
		hoja.cada(2, func(d campoPB) {
			o := doc[d.b.uint(1)]
			if o.tipo != tipoIWATabla {
				l.omitido = true
				return
			}
			if t := l.tablaIWA(o.datos.ref(2)); t != nil {
				tablas = append(tablas, t)
			}
		})
		for _, t := range tablas {
			nombre := string(hoja.msg(1))
			if len(tablas) > 1 && t.nombre != "" {
				nombre += " · " + t.nombre
			}
			formulas += t.formulas
			m.Sheets = append(m.Sheets, t.hoja(nombreUnico(nombre, nombres)))
		}
	})
	return m, formulas, l.omitido
}
