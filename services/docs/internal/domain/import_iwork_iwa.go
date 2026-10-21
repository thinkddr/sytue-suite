// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"unicode/utf16"

	"github.com/klauspost/compress/snappy"
)

// # iWork 2013 y posteriores: el marco IWA
//
// El documento va en `Index/*.iwa` (o dentro de un `Index.zip`). Cada `.iwa` es una serie
// de trozos: un byte de tipo (0), la longitud en 3 bytes little-endian y un bloque Snappy
// sin suma de control (`format_description.txt` de Google). Descomprimido, es una serie de
// objetos: un varint con la longitud de la cabecera, la cabecera (protobuf: 1 id, 2 por
// cada mensaje {1 tipo, 3 longitud}) y los mensajes, uno tras otro.
//
// Los mensajes son protobuf. Por la decisión D1 (`docs/security.md` §4.4.1) no hay
// esquemas en el repositorio: cada campo que se lee está escrito a mano, con su número
// y lo que se observó en los ficheros de prueba.

var errIWA = errors.New("paquete IWA ilegible o fuera de los topes")

// objIWA es un objeto del documento: el tipo de su primer mensaje y el mensaje.
type objIWA struct {
	tipo  uint64
	datos pb
}

// documentoIWA son los objetos del documento por su id.
type documentoIWA map[uint64]objIWA

// de es el mensaje del objeto `id` si es del tipo `tipo`, o nil.
func (d documentoIWA) de(id, tipo uint64) pb {
	if o, ok := d[id]; ok && o.tipo == tipo {
		return o.datos
	}
	return nil
}

// iwa lee todos los `.iwa` del paquete. nil si no se puede o salta un tope.
func (p *paqueteIWork) iwa() documentoIWA {
	fuentes := map[string]*zip.File{}
	for n, f := range p.ficheros {
		if strings.HasPrefix(n, "Index/") && strings.HasSuffix(n, ".iwa") {
			fuentes[n] = f
		}
	}
	if iz := p.leer("Index.zip", maxIndice09); iz != nil {
		zr, err := zip.NewReader(bytes.NewReader(iz), int64(len(iz)))
		if err != nil || len(zr.File) > maxEntradasIWork {
			return nil
		}
		for _, f := range zr.File {
			if strings.HasSuffix(f.Name, ".iwa") {
				fuentes[f.Name] = f
			}
		}
	}
	doc := documentoIWA{}
	total := 0
	for _, f := range fuentes {
		crudo := leerEntrada(f, maxIndice09)
		if crudo == nil {
			return nil
		}
		datos, err := descomprimirIWA(crudo, maxIndice09-total)
		if err != nil {
			return nil
		}
		total += len(datos)
		if err := doc.objetos(datos); err != nil {
			return nil
		}
	}
	if len(doc) == 0 {
		return nil
	}
	return doc
}

// descomprimirIWA deshace el marco y los bloques Snappy, sin pasar de `tope` bytes.
func descomprimirIWA(b []byte, tope int) ([]byte, error) {
	var out []byte
	for len(b) > 0 {
		if len(b) < 4 || b[0] != 0 {
			return nil, errIWA
		}
		n := int(b[1]) | int(b[2])<<8 | int(b[3])<<16
		if 4+n > len(b) {
			return nil, errIWA
		}
		bloque := b[4 : 4+n]
		dl, err := snappy.DecodedLen(bloque)
		if err != nil || len(out)+dl > tope {
			return nil, errIWA
		}
		dec, err := snappy.Decode(nil, bloque)
		if err != nil {
			return nil, errIWA
		}
		out = append(out, dec...)
		b = b[4+n:]
	}
	return out, nil
}

// objetos añade los objetos de un `.iwa` descomprimido. Si un id se repite, gana el
// primero.
// ponytail: un objeto partido en varios trozos («should_merge», campo 3 de la cabecera)
// se queda con el primero; no se ha visto en el corpus.
func (d documentoIWA) objetos(b []byte) error {
	for len(b) > 0 {
		l, k := longitudIWA(b)
		if l < 0 {
			return errIWA
		}
		cab := pb(b[k : k+l])
		b = b[k+l:]
		id := cab.uint(1)
		var o objIWA
		primero, mal := true, false
		cab.cada(2, func(c campoPB) {
			ln := acotado(c.b.uint(3), len(b))
			if mal || ln < 0 {
				mal = true
				return
			}
			if primero {
				o = objIWA{tipo: c.b.uint(1), datos: pb(b[:ln])}
				primero = false
			}
			b = b[ln:]
		})
		if mal || len(d) >= maxNodos09 {
			return errIWA
		}
		if _, ya := d[id]; !ya && !primero {
			d[id] = o
		}
	}
	return nil
}

// ## Protobuf en crudo

// acotado es v como int si no pasa de `tope` (≥ 0), o -1: un tamaño o una posición que se
// lee del fichero y se va a usar para indexar.
func acotado(v uint64, tope int) int {
	if tope < 0 || v > uint64(tope) {
		return -1
	}
	return int(v) //nolint:gosec // G115: v ≤ tope, que es un int
}

// longitudIWA lee el varint con la longitud de lo que le sigue: la longitud, si cabe en
// lo que queda, y los bytes que ocupa el varint; -1 si no se lee o no cabe.
func longitudIWA(b []byte) (l, k int) {
	v, k := binary.Uvarint(b)
	if k <= 0 {
		return -1, 0
	}
	return acotado(v, len(b)-k), k
}

// pb es un mensaje protobuf sin esquema: sus campos se leen por número.
type pb []byte

// campoPB es un campo: varint o fijo en `n`; delimitado, o los bytes del fijo, en `b`.
type campoPB struct {
	num, wire int
	n         uint64
	b         pb
}

// cada llama a `f` con cada campo `num` en orden. Un mensaje mal formado se lee hasta
// donde se entiende.
func (m pb) cada(num int, f func(campoPB)) {
	for len(m) > 0 {
		clave, k := binary.Uvarint(m)
		if k <= 0 {
			return
		}
		m = m[k:]
		c := campoPB{num: int(clave >> 3), wire: int(clave & 7)}
		switch c.wire {
		case 0:
			c.n, k = binary.Uvarint(m)
			if k <= 0 {
				return
			}
			m = m[k:]
		case 1, 5:
			w := 8
			if c.wire == 5 {
				w = 4
			}
			if len(m) < w {
				return
			}
			if w == 8 {
				c.n = binary.LittleEndian.Uint64(m)
			} else {
				c.n = uint64(binary.LittleEndian.Uint32(m))
			}
			c.b = m[:w]
			m = m[w:]
		case 2:
			l, k := longitudIWA(m)
			if l < 0 {
				return
			}
			c.b = m[k : k+l]
			m = m[k+l:]
		default:
			return
		}
		if c.num == num {
			f(c)
		}
	}
}

// campo es el último campo `num` (como en protobuf, el último gana) y si está.
func (m pb) campo(num int) (campoPB, bool) {
	var out campoPB
	ok := false
	m.cada(num, func(c campoPB) { out, ok = c, true })
	return out, ok
}

func (m pb) uint(num int) uint64 {
	c, _ := m.campo(num)
	return c.n
}

func (m pb) msg(num int) pb {
	c, _ := m.campo(num)
	return c.b
}

// f32 es un float32 (campo fijo de 4 bytes), o 0.
func (m pb) f32(num int) float32 {
	c, _ := m.campo(num)
	if c.wire != 5 {
		return 0
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(c.b))
}

// ref es el id de una referencia a otro objeto: un mensaje con el id en el campo 1.
func (m pb) ref(num int) uint64 {
	return m.msg(num).uint(1)
}

// ## Texto
//
// Tipos de objeto y campos observados en los ficheros de prueba:
//
//   - 10000, la raíz de un .pages (objeto 1): 4 → el almacén del cuerpo; 3 → los dibujos
//     flotantes (1 por grupo, y en cada grupo 4 {1 → cada dibujo}).
//   - 2001, un almacén de texto: 3 el texto (puede venir en trozos); 5 la tabla de estilos
//     de párrafo y 8 la de carácter, entradas {1 posición, 2 → estilo}. Las posiciones
//     cuentan unidades UTF-16. En la de párrafo una entrada sin estilo marca un párrafo
//     que sigue con el anterior; en la de carácter, que el tramo vuelve a no tener estilo.
//   - 2011, un cuadro de texto: 2 → su almacén.
//   - 2022 y 2021, estilos de párrafo y de carácter: 1 {1 nombre, 3 → padre}; 11 las
//     propiedades de carácter (1 negrita, 2 cursiva, 11 subrayado, 12 tachado —en
//     test-formats.numbers de numbers-parser, «the quick brown fox» lleva una cada
//     palabra—, 7 color {3 r, 4 g, 5 b} en 0..1);
//     12 las de párrafo (1 alineación: 0 izquierda, 1 derecha, 2 centro, 3 justificado).
//     Una propiedad que el estilo no trae la decide su padre.
//
// En el texto, «\n» y U+2029 terminan el párrafo, U+2028 es un salto de línea y U+FFFC
// marca un objeto anclado (una tabla, una imagen) que no se convierte.

const (
	tipoIWAPages      = 10000
	tipoIWAAlmacen    = 2001
	tipoIWACuadro     = 2011
	tipoIWAEstiloCar  = 2021
	tipoIWAEstiloParr = 2022
	maxHerenciaIWA    = 32
)

// lectorIWA convierte almacenes de texto IWA al modelo rico del editor.
type lectorIWA struct {
	doc documentoIWA
	// omitido: algo del fichero (una tabla, una imagen) no se ha convertido.
	omitido bool
}

// estiloEn es la tabla de estilos de un almacén: el estilo que rige desde cada posición.
type estiloEn struct {
	desde int
	id    uint64
}

func tablaDeEstilosIWA(m pb, heredaSinEstilo bool) []estiloEn {
	var out []estiloEn
	m.cada(1, func(c campoPB) {
		e := estiloEn{desde: acotado(c.b.uint(1), math.MaxInt32), id: c.b.ref(2)}
		if e.id == 0 && heredaSinEstilo && len(out) > 0 {
			e.id = out[len(out)-1].id
		}
		out = append(out, e)
	})
	return out
}

// en es el estilo que rige en la posición i (las tablas vienen ordenadas).
func en(t []estiloEn, i int) uint64 {
	var id uint64
	for _, e := range t {
		if e.desde > i {
			break
		}
		id = e.id
	}
	return id
}

// propIWA es la propiedad `num` del grupo `grupo` (11 carácter, 12 párrafo) del primer
// estilo de `ids` que la tenga, subiendo por sus padres.
func (l *lectorIWA) propIWA(grupo, num int, ids ...uint64) (campoPB, bool) {
	for _, id := range ids {
		for i := 0; id != 0 && i < maxHerenciaIWA; i++ {
			o, ok := l.doc[id]
			if !ok || (o.tipo != tipoIWAEstiloCar && o.tipo != tipoIWAEstiloParr) {
				break
			}
			if c, ok := o.datos.msg(grupo).campo(num); ok {
				return c, true
			}
			id = o.datos.msg(1).ref(3)
		}
	}
	return campoPB{}, false
}

// nombreIWA es el primer nombre de la cadena de herencia del estilo.
func (l *lectorIWA) nombreIWA(id uint64) string {
	for i := 0; id != 0 && i < maxHerenciaIWA; i++ {
		o, ok := l.doc[id]
		if !ok {
			return ""
		}
		if n := o.datos.msg(1).msg(1); len(n) > 0 {
			return string(n)
		}
		id = o.datos.msg(1).ref(3)
	}
	return ""
}

func (l *lectorIWA) colorIWA(ids ...uint64) string {
	c, ok := l.propIWA(11, 7, ids...)
	if !ok {
		return ""
	}
	var hex strings.Builder
	for _, n := range []int{3, 4, 5} {
		f := float64(c.b.f32(n))
		if math.IsNaN(f) {
			return ""
		}
		const digitos = "0123456789ABCDEF"
		v := int(min(max(f, 0), 1)*255 + 0.5)
		hex.WriteByte(digitos[v>>4])
		hex.WriteByte(digitos[v&15])
	}
	if hex.String() == "000000" {
		return ""
	}
	return hex.String()
}

// parrafos son los párrafos del almacén `id`.
func (l *lectorIWA) parrafos(id uint64) []xpara {
	st := l.doc.de(id, tipoIWAAlmacen)
	if st == nil {
		return nil
	}
	var sb strings.Builder
	st.cada(3, func(c campoPB) { sb.Write(c.b) })
	u := utf16.Encode([]rune(sb.String()))
	parr, car := tablaDeEstilosIWA(st.msg(5), true), tablaDeEstilosIWA(st.msg(8), false)

	var out []xpara
	var cur *xpara
	var ep uint64
	var tramo []uint16
	var ec uint64
	cerrarTramo := func() {
		if len(tramo) == 0 {
			return
		}
		b := func(num int) bool {
			ids := []uint64{ec, ep}
			if cur.style != "" {
				ids = ids[:1] // en un título la negrita del párrafo ya la pone el título
			}
			c, ok := l.propIWA(11, num, ids...)
			return ok && c.n != 0
		}
		cur.runs = append(cur.runs, xrun{
			text: string(utf16.Decode(tramo)), b: b(1), i: b(2), u: b(11), strike: b(12),
			color: l.colorIWA(ec, ep),
		})
		tramo = tramo[:0]
	}
	for i, c := range u {
		if cur == nil {
			ep = en(parr, i)
			cur = &xpara{style: tituloDeEstilo09(l.nombreIWA(ep))}
			if a, ok := l.propIWA(12, 1, ep); ok {
				cur.align = alineacion09(float64(a.n))
			}
		}
		if e := en(car, i); e != ec {
			cerrarTramo()
			ec = e
		}
		switch {
		case c == '\n' || c == 0x2029:
			cerrarTramo()
			out = append(out, *cur)
			cur = nil
		case c == 0x2028:
			cerrarTramo()
			cur.runs = append(cur.runs, xrun{brk: true})
		case c == 0xFFFC:
			l.omitido = true
		case c < 0x20 && c != '\t':
			// saltos de página, de sección o de columna, y marcas de nota: no son texto
		default:
			tramo = append(tramo, c)
		}
	}
	if cur != nil {
		cerrarTramo()
		if len(cur.runs) > 0 {
			out = append(out, *cur)
		}
	}
	return out
}

// importPagesIWA convierte un documento .pages de 2013 en adelante: el cuerpo y, detrás,
// el texto de los cuadros flotantes. "" si no hay texto.
func importPagesIWA(doc documentoIWA) (cuerpo string, omitido bool) {
	raiz := doc.de(1, tipoIWAPages)
	if raiz == nil {
		return "", false
	}
	l := &lectorIWA{doc: doc}
	paras := l.parrafos(raiz.ref(4))
	doc[raiz.ref(3)].datos.cada(1, func(g campoPB) {
		g.b.cada(4, func(r campoPB) {
			if c := doc.de(r.b.ref(1), tipoIWACuadro); c != nil {
				paras = append(paras, l.parrafos(c.ref(2))...)
			} else {
				l.omitido = true
			}
		})
	})
	return htmlDeParrafos(paras), l.omitido
}

// htmlDeParrafos es el HTML de unos párrafos, o "" si no tienen texto.
func htmlDeParrafos(paras []xpara) string {
	bs := bloquesDeParrafos(paras)
	if !conTexto(bs) {
		return ""
	}
	return blocksToHTML(bs)
}
