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
	tipoIWADocumento     = 1
	tipoIWAHoja          = 2
	tipoIWAGrafico       = 5000
	tipoIWAGraficoModelo = 5004
	tipoIWAGrafico2      = 5021
	tipoIWATabla         = 6000
	tipoIWAModelo        = 6001
	tipoIWATesela        = 6002
	tipoIWAEstiloCelda   = 6004
	tipoIWALista         = 6005
	tipoIWACombinadas    = 6144
	tipoIWARico          = 6218
	tipoIWAResumen       = 6316
	filasPorTesela       = 256
	celdaVacia           = 0xFFFF
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

func (l *lectorIWA) listaObjetosIWA(ids ...uint64) map[uint64]uint64 {
	out := map[uint64]uint64{}
	for _, id := range ids {
		l.doc.de(id, tipoIWALista).cada(3, func(e campoPB) {
			if ref := e.b.ref(4); ref != 0 {
				out[e.b.uint(1)] = ref
			}
		})
	}
	return out
}

func (l *lectorIWA) listaFormatosIWA(ids ...uint64) map[uint64]pb {
	out := map[uint64]pb{}
	for _, id := range ids {
		l.doc.de(id, tipoIWALista).cada(3, func(e campoPB) {
			if f := e.b.msg(6); len(f) > 0 {
				out[e.b.uint(1)] = f
			}
		})
	}
	return out
}

type tablasModeloIWA struct {
	cadenas, ricos     map[uint64]string
	estCelda, estTexto map[uint64]uint64
	formatos           map[uint64]pb
}

func (l *lectorIWA) tablasDatosIWA(ds pb) tablasModeloIWA {
	return tablasModeloIWA{
		cadenas:  l.listaIWA(ds.ref(4)),
		ricos:    l.listaIWA(ds.ref(17)),
		estCelda: l.listaObjetosIWA(ds.ref(5), ds.ref(1)),
		estTexto: l.listaObjetosIWA(ds.ref(5), ds.ref(2)),
		formatos: l.listaFormatosIWA(ds.ref(11), ds.ref(22)),
	}
}

// tablaInfoIWA lee un objeto tabla (`6000`): su modelo (`6001` en 2) y, si es una tabla
// dinámica, su modelo de resumen (`6316` en 4).
func (l *lectorIWA) tablaInfoIWA(tb pb) *tabla09 {
	if len(tb) == 0 {
		return nil
	}
	t := l.tablaIWA(tb.ref(2))
	if idRes := tb.ref(4); idRes != 0 {
		l.resumenIWA(&t, idRes)
	}
	return t
}

// tablaIWA lee el modelo de tabla `id`. nil si no hay celdas que se lean.
func (l *lectorIWA) tablaIWA(id uint64) *tabla09 {
	m := l.doc.de(id, tipoIWAModelo)
	nr, nc := acotado(m.uint(6), maxFilasHoja), acotado(m.uint(7), maxColumnasHoja)
	if m == nil || nr <= 0 || nc <= 0 {
		return nil
	}
	ds := m.msg(4)
	t := &tabla09{
		nombre:     string(m.msg(8)),
		filas:      nr,
		cols:       nc,
		celdas:     map[[2]int]celda09{},
		combinadas: l.combinadasIWA(ds.ref(13), nr, nc),
	}
	tb := l.tablasDatosIWA(ds)
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
				l.filaIWA(t, fila, t.cols, false, fi.b, tb)
			}
		})
	})
	if len(t.celdas) == 0 {
		return nil
	}
	return t
}

func (l *lectorIWA) resumenIWA(tp **tabla09, id uint64) {
	sm := l.doc.de(id, tipoIWAResumen)
	if sm == nil {
		return
	}
	ds := sm.msg(2)
	t := *tp
	if t == nil {
		t = &tabla09{celdas: map[[2]int]celda09{}}
	}
	tb := l.tablasDatosIWA(ds)
	l.doc.de(ds.ref(6), tipoIWALista).cada(3, func(campoPB) { t.formulas++ })
	ds.msg(3).cada(1, func(te campoPB) {
		base := acotado(te.b.uint(1), maxFilasHoja/filasPorTesela) * filasPorTesela
		if base < 0 {
			return
		}
		l.doc.de(te.b.ref(2), tipoIWATesela).cada(5, func(fi campoPB) {
			fila := base + acotado(fi.b.uint(1), filasPorTesela-1)
			if fila < base || fila >= maxFilasHoja {
				return
			}
			l.filaIWA(t, fila, maxColumnasHoja, true, fi.b, tb)
		})
	})
	if len(t.celdas) > 0 {
		*tp = t
	}
}

func (l *lectorIWA) combinadasIWA(id uint64, nr, nc int) []string {
	var out []string
	l.doc.de(id, tipoIWACombinadas).cada(1, func(c campoPB) {
		orig := c.b.msg(1).uint(1)
		tam := c.b.msg(2).uint(1)
		f, col := int(orig&0xFFFF), int((orig>>16)&0xFFFF)
		rs, cs := int(tam&0xFFFF), int((tam>>16)&0xFFFF)
		if f >= nr || col >= nc || (rs <= 1 && cs <= 1) {
			return
		}
		r2 := min(f+max(rs, 1)-1, nr-1)
		c2 := min(col+max(cs, 1)-1, nc-1)
		if r2 > f || c2 > col {
			out = append(out, A1Ref(f+1, col+1)+":"+A1Ref(r2+1, c2+1))
		}
	})
	return out
}

func (l *lectorIWA) graficoIWA(id uint64) (t *tabla09, forma pb) {
	o, ok := l.doc[id]
	if !ok || (o.tipo != tipoIWAGrafico && o.tipo != tipoIWAGrafico2) {
		return nil, nil
	}
	datos := o.datos
	if o.tipo == tipoIWAGrafico2 {
		if sub := l.doc.de(datos.ref(1), tipoIWAGrafico); len(sub) > 0 {
			datos = sub
		} else if sub := datos.msg(1); len(sub) > 0 && len(datos.msg(4)) == 0 {
			datos = sub
		}
	}
	forma = datos
	if len(datos.msg(1).msg(1).msg(1)) > 0 {
		forma = datos.msg(1)
	}
	titulo := strings.TrimSpace(string(datos.msg(11)))
	candidatos := []pb{datos.msg(4), datos.msg(2).msg(1), datos.msg(2)}
	for _, num := range []int{4, 2} {
		if ref := datos.ref(num); ref != 0 {
			m := l.doc[ref].datos
			candidatos = append(candidatos, m, m.msg(4), m.msg(1))
		}
	}
	var rejilla pb
	for _, cand := range candidatos {
		if len(cand.msg(2)) > 0 || len(cand.msg(3)) > 0 || len(cand.msg(4)) > 0 {
			rejilla = cand
			break
		}
	}
	if len(rejilla) == 0 {
		return nil, forma
	}
	var filas, cols []string
	rejilla.cada(2, func(c campoPB) {
		if len(c.b) > 0 {
			filas = append(filas, string(c.b))
		}
	})
	rejilla.cada(3, func(c campoPB) {
		if len(c.b) > 0 {
			cols = append(cols, string(c.b))
		}
	})
	var matriz [][]celda09
	rejilla.cada(4, func(f campoPB) {
		var fila []celda09
		f.b.cada(1, func(cel campoPB) {
			fila = append(fila, celdaGraficoIWA(cel.b))
		})
		if len(fila) > 0 {
			matriz = append(matriz, fila)
		}
	})
	return armarTablaGrafico(titulo, filas, cols, matriz), forma
}

func celdaGraficoIWA(m pb) celda09 {
	if c1, ok := m.campo(1); ok {
		switch c1.wire {
		case 1:
			v := math.Float64frombits(c1.n)
			if !math.IsNaN(v) && !math.IsInf(v, 0) {
				return celda09{texto: strconv.FormatFloat(v, 'f', -1, 64), num: true}
			}
		case 5:
			v := float64(math.Float32frombits(uint32(c1.n))) //nolint:gosec // G115: wire 5 es uint32
			if !math.IsNaN(v) && !math.IsInf(v, 0) {
				return celda09{texto: strconv.FormatFloat(v, 'f', -1, 64), num: true}
			}
		case 0:
			return celda09{texto: strconv.FormatUint(c1.n, 10), num: true}
		}
	}
	if c2, ok := m.campo(2); ok && c2.wire == 1 {
		v := math.Float64frombits(c2.n)
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			return celda09{texto: strconv.FormatFloat(v, 'f', -1, 64), num: true}
		}
	}
	if s := string(m.msg(3)); s != "" {
		return celda09{texto: s}
	}
	return celda09{}
}

// valoresIWA son los valores que trae una celda, cada uno con si está.
type valoresIWA struct {
	num, seg               float64
	cad, rico, fmt         uint64
	estCelda, estTexto     uint64
	hayNum, hayFec         bool
	hayCad, hayRic, hayFmt bool
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
		off += 4
	}
	if flags&0x20 != 0 {
		v.estCelda, _ = c.u32(off)
		off += 4
	}
	if flags&0x40 != 0 {
		v.estTexto, _ = c.u32(off)
		off += 4
	}
	off += 4 * bitsEnUno(flags&0x1F80)
	v.fmt, v.hayFmt = formatoV5(c, off, flags, c[1])
	return v
}

func formatoV5(c lectorCelda, off int, flags uint32, tipo byte) (fmt uint64, hay bool) {
	for bit := uint32(0x2000); bit <= 0x40000; bit <<= 1 {
		if flags&bit == 0 {
			continue
		}
		fk, ok := c.u32(off)
		off += 4
		if !ok {
			break
		}
		if !hay || (tipo == 7 && bit == 0x10000) || ((tipo == 2 || tipo == 10) && (bit == 0x2000 || bit == 0x4000)) {
			fmt, hay = fk, true
		}
	}
	return fmt, hay
}

// valoresViejos lee los valores de los formatos anteriores al 5.
func valoresViejos(c lectorCelda) (v valoresIWA) {
	flags := binary.LittleEndian.Uint32(c[4:])
	off, intermedias := 12, uint32(0x3000)
	extraPrevias := uint32(0x0D00)
	if c[0] <= 1 {
		off, intermedias, extraPrevias = 8, 0x1000, 0x0100
	}
	if flags&0x2 != 0 {
		v.fmt, v.hayFmt = c.u32(off)
		off += 4
	}
	if flags&0x4 != 0 {
		v.estCelda, _ = c.u32(off)
		off += 4
	}
	if flags&0x8 != 0 {
		off += 4
	}
	if flags&0x80 != 0 {
		v.estTexto, _ = c.u32(off)
		off += 4
	}
	off += 4 * bitsEnUno(flags&extraPrevias)
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
		off += 8
	}
	if c[0] > 1 {
		flags2 := binary.LittleEndian.Uint32(c[8:]) >> 16
		if flags2&0xFF != 0 {
			if fk, ok := c.u32(off); ok {
				v.fmt, v.hayFmt = fk, true
			}
		}
	}
	return v
}

// filaIWA lee las celdas de una fila de tesela (`fi`).
func (l *lectorIWA) filaIWA(t *tabla09, fila, maxCol int, expandir bool, fi pb, tb tablasModeloIWA) {
	celdas, posiciones := fi.msg(6), fi.msg(7)
	if len(posiciones) == 0 {
		celdas, posiciones = fi.msg(3), fi.msg(4)
	}
	ancho := fi.uint(8) != 0
	for col := 0; col*2+1 < len(posiciones) && col < maxCol; col++ {
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
		if v := l.celdaIWA(celdas[o:], tb); v.texto != "" {
			t.celdas[[2]int{fila, col}] = v
			if expandir {
				t.filas = max(t.filas, fila+1)
				t.cols = max(t.cols, col+1)
			}
		}
	}
}

func tipoCeldaIWA(c []byte) byte {
	if c[0] < 5 && c[1] == 0 {
		return c[2]
	}
	return c[1]
}

// celdaIWA lee una celda; vacía si no se entiende.
func (l *lectorIWA) celdaIWA(c []byte, tb tablasModeloIWA) celda09 {
	if len(c) < 12 {
		return celda09{}
	}
	tipo := tipoCeldaIWA(c)
	var v valoresIWA
	if c[0] >= 5 {
		v = valoresV5(c)
	} else {
		v = valoresViejos(c)
	}
	var out celda09
	switch {
	case (tipo == 2 || tipo == 10) && v.hayNum:
		out = celda09{texto: strconv.FormatFloat(v.num, 'f', -1, 64), num: true}
		if v.hayFmt {
			out.formato = formatoNumeroIWA(tb.formatos[v.fmt])
		}
	case tipo == 3 && v.hayCad:
		out = celda09{texto: tb.cadenas[v.cad]}
	case tipo == 5 && v.hayFec:
		out = fechaIWA(v.seg)
	case tipo == 6 && v.hayNum:
		out = celda09{texto: strings.ToUpper(strconv.FormatBool(v.num > 0))}
	case tipo == 7 && v.hayNum:
		f, ok := tb.formatos[v.fmt]
		out = celda09{texto: duracionIWA(v.num, f, v.hayFmt && ok)}
	case tipo == 9 && v.hayRic:
		out = celda09{texto: tb.ricos[v.rico]}
	}
	if out.texto != "" {
		out.estilo = l.estiloCeldaIWA(tb.estCelda[v.estCelda], tb.estTexto[v.estTexto])
	}
	return out
}

func (l *lectorIWA) estiloCeldaIWA(idCelda, idTexto uint64) cellStyle {
	var st cellStyle
	for i := 0; idCelda != 0 && i < maxHerenciaIWA; i++ {
		o, ok := l.doc[idCelda]
		if !ok || o.tipo != tipoIWAEstiloCelda {
			break
		}
		if c, ok := o.datos.msg(11).campo(1); ok {
			if hex := colorMensajeIWA(c.b.msg(1)); hex != "" && hex != "FFFFFF" {
				st.Fill = "#" + hex
			}
			break
		}
		idCelda = o.datos.msg(1).ref(3)
	}
	if idTexto != 0 {
		if col := l.colorIWA(idTexto); col != "" {
			st.Color = "#" + col
		}
		if c, ok := l.propIWA(11, 11, idTexto); ok && c.n != 0 {
			st.U = true
		}
		if c, ok := l.propIWA(11, 12, idTexto); ok && c.n != 0 {
			st.S = true
		}
	}
	return st
}

func formatoNumeroIWA(f pb) numFmtSpec {
	ft := acotado(f.uint(1), 258) - 256
	if len(f) == 0 || ft < 0 {
		return numFmtSpec{}
	}
	cDec, hayDec := f.campo(2)
	dec := acotado(cDec.n, 8)
	miles := f.uint(4) != 0 || f.uint(5) != 0
	return especFormatoNumero(ft, dec, hayDec && dec >= 0, miles)
}

type unidadDuracionIWA struct {
	cod             uint64
	ms              int64
	corto, sing, pl string
}

var unidadesDuracionIWA = []unidadDuracionIWA{
	{1, 604800000, "w", "week", "weeks"},
	{2, 86400000, "d", "day", "days"},
	{4, 3600000, "h", "hour", "hours"},
	{8, 60000, "m", "minute", "minutes"},
	{16, 1000, "s", "second", "seconds"},
	{32, 1, "ms", "millisecond", "milliseconds"},
}

func rangoUnidadesDuracionIWA(total int64, f pb, hayFmt bool) (estilo uint64, activas []unidadDuracionIWA) {
	estilo, umax, umin, auto := uint64(1), uint64(0), uint64(0), true
	if hayFmt && f.uint(1) == 268 {
		if c, ok := f.campo(7); ok {
			estilo = c.n
		}
		umax, umin = f.uint(15), f.uint(16)
		auto = f.uint(40) != 0
	}
	if auto && total == 0 {
		umax, umin = 2, 2
	} else if umax == 0 || umin == 0 {
		umax, umin = extremosDuracionIWA(total)
	}
	dentro := false
	for _, u := range unidadesDuracionIWA {
		if u.cod == umax {
			dentro = true
		}
		if dentro {
			activas = append(activas, u)
		}
		if u.cod == umin {
			break
		}
	}
	if len(activas) == 0 {
		activas = unidadesDuracionIWA[1:2]
	}
	return estilo, activas
}

func extremosDuracionIWA(total int64) (umax, umin uint64) {
	rem := total
	for _, u := range unidadesDuracionIWA {
		v := rem / u.ms
		rem %= u.ms
		if v > 0 {
			if umax == 0 {
				umax = u.cod
			}
			umin = u.cod
		}
	}
	if umax == 0 {
		return 2, 2
	}
	return umax, umin
}

func desglosarDuracionIWA(total int64, activas []unidadDuracionIWA) []int64 {
	vals := make([]int64, len(activas))
	rem := total
	for i, u := range activas {
		if i == len(activas)-1 {
			vals[i] = int64(math.Round(float64(rem) / float64(u.ms)))
		} else {
			vals[i] = rem / u.ms
			rem %= u.ms
		}
	}
	for i := len(activas) - 1; i > 0; i-- {
		tope := activas[i-1].ms / activas[i].ms
		if vals[i] >= tope {
			vals[i-1] += vals[i] / tope
			vals[i] %= tope
		}
	}
	return vals
}

func duracionIWA(seg float64, f pb, hayFmt bool) string {
	if math.IsNaN(seg) || math.IsInf(seg, 0) || math.Abs(seg) > 1e13 {
		return ""
	}
	neg := seg < 0
	total := int64(math.Round(math.Abs(seg) * 1000))
	estilo, activas := rangoUnidadesDuracionIWA(total, f, hayFmt)
	vals := desglosarDuracionIWA(total, activas)
	var partes []string
	for i, u := range activas {
		partes = agregarUnidadDuracionIWA(partes, estilo, u, vals[i], neg, i, len(activas))
	}
	if estilo == 0 {
		return strings.Join(partes, ":")
	}
	return strings.Join(partes, " ")
}

func agregarUnidadDuracionIWA(partes []string, estilo uint64, u unidadDuracionIWA, val int64, neg bool, i, n int) []string {
	v := val
	if neg && v > 0 {
		v = -v
	}
	switch estilo {
	case 0:
		if u.cod == 32 && i > 0 && len(partes) > 0 {
			partes[len(partes)-1] += "." + fmtZerosIWA(val, 3)
			return partes
		}
		if u.cod == 32 {
			return append(partes, fmtConSignoIWA(v, 3))
		}
		if (u.cod == 8 || u.cod == 16) && (i > 0 || n > 1) {
			return append(partes, fmtConSignoIWA(v, 2))
		}
		return append(partes, strconv.FormatInt(v, 10))
	case 1:
		return append(partes, strconv.FormatInt(v, 10)+u.corto)
	default:
		et := u.pl
		if val == 1 {
			et = u.sing
		}
		return append(partes, strconv.FormatInt(v, 10)+" "+et)
	}
}

func fmtZerosIWA(v int64, ancho int) string {
	s := strconv.FormatInt(v, 10)
	for len(s) < ancho {
		s = "0" + s
	}
	return s
}

func fmtConSignoIWA(v int64, ancho int) string {
	if v < 0 {
		return "-" + fmtZerosIWA(-v, ancho)
	}
	return fmtZerosIWA(v, ancho)
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
			idObj := d.b.uint(1)
			o := doc[idObj]
			switch o.tipo {
			case tipoIWATabla:
				if t := l.tablaInfoIWA(o.datos); t != nil {
					tablas = append(tablas, t)
				}
			case tipoIWAGrafico, tipoIWAGrafico2:
				if t, _ := l.graficoIWA(idObj); t != nil {
					tablas = append(tablas, t)
				} else {
					l.omitido = true
				}
			default:
				l.omitido = true
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
