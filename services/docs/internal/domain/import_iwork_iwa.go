// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
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
	nombres := make([]string, 0, len(fuentes))
	for n := range fuentes {
		nombres = append(nombres, n)
	}
	slices.Sort(nombres)
	doc := documentoIWA{}
	total := 0
	for _, n := range nombres {
		f := fuentes[n]
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
		if !primero {
			if o.tipo == tipoIWAMetadata {
				if _, ya := d[id]; ya {
					id = math.MaxUint64 - uint64(len(d))
				}
				d[id] = o
			} else if prev, ya := d[id]; !ya {
				d[id] = o
			} else if prev.tipo == tipoIWAMetadata {
				d[math.MaxUint64-uint64(len(d))] = prev
				d[id] = o
			}
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

// f64 es un float64 (campo fijo de 8 bytes), o 0.
func (m pb) f64(num int) float64 {
	c, _ := m.campo(num)
	if c.wire != 1 {
		return 0
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(c.b))
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
	tipoIWAPages          = 10000
	tipoIWAMetadata       = 11006
	tipoIWAAutor          = 212
	tipoIWAAlmacen        = 2001
	tipoIWAAdjunto        = 2003
	tipoIWACuadro         = 2011
	tipoIWAHighlight      = 2013
	tipoIWAComentarioInfo = 2014
	tipoIWAEstiloCar      = 2021
	tipoIWAEstiloParr     = 2022
	tipoIWAListaEstilo    = 2023
	tipoIWAImagen         = 3005
	tipoIWAGrupo          = 3008
	tipoIWAComentario     = 3056
	maxHerenciaIWA        = 32
)

// lectorIWA convierte almacenes de texto IWA al modelo rico del editor.
type lectorIWA struct {
	doc            documentoIWA
	paquete        *paqueteIWork
	archivos       map[uint64][]string
	conComentarios bool
	comentarios    []*rawComment
	porIDCom       map[uint64]*rawComment
	// omitido: algo del fichero (una tabla, una imagen sin datos) no se ha convertido.
	omitido bool
}

func nuevoLectorIWA(doc documentoIWA, p *paqueteIWork, conComentarios bool) *lectorIWA {
	archivos := map[uint64][]string{}
	for _, o := range doc {
		if o.tipo != tipoIWAMetadata {
			continue
		}
		o.datos.cada(4, func(c campoPB) {
			id := c.b.uint(1)
			if id == 0 {
				return
			}
			for _, num := range []int{4, 3} {
				if nom := string(c.b.msg(num)); nom != "" {
					archivos[id] = append(archivos[id], nom)
				}
			}
		})
	}
	return &lectorIWA{
		doc:            doc,
		paquete:        p,
		archivos:       archivos,
		conComentarios: conComentarios,
		porIDCom:       map[uint64]*rawComment{},
	}
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

func tablaDeNivelesIWA(m pb) []estiloEn {
	var out []estiloEn
	m.cada(1, func(c campoPB) {
		out = append(out, estiloEn{desde: acotado(c.b.uint(1), math.MaxInt32), id: c.b.uint(2)})
	})
	return out
}

func tablaDeAdjuntosIWA(m pb) map[int]uint64 {
	out := map[int]uint64{}
	m.cada(1, func(c campoPB) {
		if pos := acotado(c.b.uint(1), math.MaxInt32); pos >= 0 {
			out[pos] = c.b.ref(2)
		}
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

func colorMensajeIWA(m pb) string {
	_, okR := m.campo(3)
	_, okG := m.campo(4)
	_, okB := m.campo(5)
	if !okR && !okG && !okB {
		return ""
	}
	var hex strings.Builder
	for _, n := range []int{3, 4, 5} {
		f := float64(m.f32(n))
		if math.IsNaN(f) {
			return ""
		}
		const digitos = "0123456789ABCDEF"
		v := int(min(max(f, 0), 1)*255 + 0.5)
		hex.WriteByte(digitos[v>>4])
		hex.WriteByte(digitos[v&15])
	}
	return hex.String()
}

func (l *lectorIWA) colorIWA(ids ...uint64) string {
	c, ok := l.propIWA(11, 7, ids...)
	if !ok {
		return ""
	}
	hex := colorMensajeIWA(c.b)
	if hex == "000000" {
		return ""
	}
	return hex
}

// tipoListaIWA devuelve "bullet", "number" o "" según el estilo de lista 2023 para `nivel`.
func (l *lectorIWA) tipoListaIWA(id uint64, nivel int) string {
	for i := 0; id != 0 && i < maxHerenciaIWA; i++ {
		o, ok := l.doc[id]
		if !ok || o.tipo != tipoIWAListaEstilo {
			return ""
		}
		var tipos []uint64
		o.datos.cada(11, func(c campoPB) { tipos = append(tipos, c.n) })
		if len(tipos) > 0 {
			switch tipos[min(max(nivel, 0), len(tipos)-1)] {
			case 1, 2:
				return "bullet"
			case 3:
				return "number"
			default:
				return ""
			}
		}
		id = o.datos.msg(1).ref(3)
	}
	return ""
}

func (l *lectorIWA) abrirParrafoIWA(ep, niv, idLista uint64) *xpara {
	p := &xpara{style: tituloDeEstilo09(l.nombreIWA(ep)), level: max(acotado(niv, 8), 0)}
	if a, ok := l.propIWA(12, 1, ep); ok {
		p.align = alineacion09(float64(a.n))
	}
	if idLista == 0 {
		if c, ok := l.propIWA(12, 40, ep); ok {
			idLista = c.b.uint(1)
		}
	}
	p.list = l.tipoListaIWA(idLista, p.level)
	return p
}

func (l *lectorIWA) tramoIWA(tramo []uint16, ec, ep uint64, titulo bool) xrun {
	ids := []uint64{ec, ep}
	if titulo {
		ids = ids[:1] // en un título la negrita del párrafo ya la pone el título
	}
	b := func(num int) bool {
		c, ok := l.propIWA(11, num, ids...)
		return ok && c.n != 0
	}
	return xrun{
		text: string(utf16.Decode(tramo)), b: b(1), i: b(2), u: b(11), strike: b(12),
		color: l.colorIWA(ec, ep),
	}
}

func (l *lectorIWA) tablaAdjuntaIWA(idAdj uint64) *tabla09 {
	idObj := l.doc.de(idAdj, tipoIWAAdjunto).ref(1)
	if idObj == 0 {
		idObj = idAdj
	}
	if tb := l.doc.de(idObj, tipoIWATabla); tb != nil {
		return l.tablaInfoIWA(tb)
	}
	t, _ := l.graficoIWA(idObj)
	return t
}

func geometriaFormaIWA(m pb) (pos, tm pb) {
	for _, g := range []pb{m.msg(1).msg(1).msg(1), m.msg(1).msg(1), m.msg(1)} {
		if p, t := g.msg(1), g.msg(2); len(t) > 0 && (t.f32(1) > 0 || t.f32(2) > 0) {
			return p, t
		}
	}
	return nil, m.msg(4)
}

func (l *lectorIWA) imagenIWA(id uint64) (src string, w, h int, pos, tm pb) {
	im := l.doc.de(id, tipoIWAImagen)
	if im == nil || l.paquete == nil {
		return "", 0, 0, nil, nil
	}
	var candidatos []string
	for _, num := range []int{11, 15, 17, 13} {
		dataID := im.ref(num)
		if dataID == 0 {
			continue
		}
		candidatos = append(candidatos, l.archivos[dataID]...)
		candidatos = append(candidatos, "-"+strconv.FormatUint(dataID, 10)+".")
	}
	src, iw, ih := l.paquete.imagenDataURL(candidatos...)
	if src == "" {
		return "", 0, 0, nil, nil
	}
	pos, tm = geometriaFormaIWA(im)
	gw, gh := int(tm.f32(1)+0.5), int(tm.f32(2)+0.5)
	if gw > 0 && gh > 0 {
		return src, gw, gh, pos, tm
	}
	return src, iw, ih, pos, tm
}

func (l *lectorIWA) registrarComentarioIWA(id uint64, parentKey string) *rawComment {
	if !l.conComentarios || id == 0 {
		return nil
	}
	if h := l.doc.de(id, tipoIWAHighlight); h != nil {
		id = h.ref(1)
	} else if ci := l.doc.de(id, tipoIWAComentarioInfo); ci != nil {
		if r := ci.ref(2); r != 0 {
			id = r
		} else {
			id = ci.ref(1)
		}
	}
	if l.porIDCom == nil {
		l.porIDCom = map[uint64]*rawComment{}
	}
	if c := l.porIDCom[id]; c != nil {
		return c
	}
	com := l.doc.de(id, tipoIWAComentario)
	if com == nil {
		return nil
	}
	body := strings.TrimSpace(string(com.msg(1)))
	if body == "" {
		return nil
	}
	autor := strings.TrimSpace(string(l.doc.de(com.ref(3), tipoIWAAutor).msg(1)))
	fecha := ""
	if seg := com.msg(2).f64(1); seg != 0 && !math.IsNaN(seg) && !math.IsInf(seg, 0) {
		fecha = epoca09.Add(time.Duration(seg * float64(time.Second))).UTC().Format(time.RFC3339)
	}
	key := strconv.FormatUint(id, 10)
	r := &rawComment{
		id:        markID(len(l.comentarios)+1, autor, fecha, body),
		key:       key,
		parentKey: parentKey,
		author:    autor,
		date:      fecha,
		paras:     []string{body},
	}
	l.porIDCom[id] = r
	l.comentarios = append(l.comentarios, r)
	com.cada(4, func(rep campoPB) {
		l.registrarComentarioIWA(rep.b.uint(1), key)
	})
	return r
}

type rangoComIWA struct {
	ini, fin int
	com      *rawComment
}

func (l *lectorIWA) rangosComentariosIWA(st pb, tope int) []rangoComIWA {
	if !l.conComentarios {
		return nil
	}
	var out []rangoComIWA
	var entradas []estiloEn
	st.msg(23).cada(1, func(c campoPB) {
		if pos := acotado(c.b.uint(1), tope); pos >= 0 {
			entradas = append(entradas, estiloEn{desde: pos, id: c.b.ref(2)})
		}
	})
	for idx, e := range entradas {
		if e.id == 0 {
			continue
		}
		fin := tope
		if idx+1 < len(entradas) {
			fin = entradas[idx+1].desde
		}
		if com := l.registrarComentarioIWA(e.id, ""); com != nil && fin > e.desde {
			out = append(out, rangoComIWA{ini: e.desde, fin: fin, com: com})
		}
	}
	st.msg(25).cada(1, func(c campoPB) {
		rng := c.b.msg(1)
		ini := acotado(rng.uint(1), tope)
		ln := acotado(rng.uint(2), tope)
		if ini >= 0 && ln > 0 {
			if com := l.registrarComentarioIWA(c.b.ref(2), ""); com != nil {
				out = append(out, rangoComIWA{ini: ini, fin: min(tope, ini+ln), com: com})
			}
		}
	})
	return out
}

func comentarioEnIWA(rs []rangoComIWA, i int) *rawComment {
	for _, r := range rs {
		if i >= r.ini && i < r.fin {
			return r.com
		}
	}
	return nil
}

func (l *lectorIWA) adjuntoEnLineaIWA(idAdj uint64, cur **xpara, trasTabla *bool, cerrarTramo func(), out []xblock) []xblock {
	idObj := l.doc.de(idAdj, tipoIWAAdjunto).ref(1)
	if idObj == 0 {
		idObj = idAdj
	}
	cerrarBloque := func() {
		cerrarTramo()
		if len((*cur).runs) > 0 {
			p := **cur
			out = append(out, xblock{para: &p})
		}
		*cur, *trasTabla = nil, true
	}
	if t := l.tablaAdjuntaIWA(idAdj); t != nil {
		cerrarBloque()
		return append(out, xblock{table: t.xtable()})
	}
	if src, w, h, _, _ := l.imagenIWA(idObj); src != "" {
		cerrarTramo()
		(*cur).runs = append((*cur).runs, xrun{raw: l.paquete.imgHTML(src, w, h)})
		*trasTabla = false
		return out
	}
	if c := l.doc.de(idObj, tipoIWACuadro); c != nil {
		cerrarBloque()
		return append(out, l.bloques(c.ref(2))...)
	}
	l.omitido = true
	return out
}

func marcarTramoIWA(r xrun, com *rawComment) xrun {
	if com != nil && r.text != "" {
		r.mark = com.id
		com.marked = true
		com.anchored = true
		com.quote.WriteString(r.text)
	}
	return r
}

// bloques son los párrafos, tablas e imágenes ancladas del almacén `id`.
func (l *lectorIWA) bloques(id uint64) []xblock {
	st := l.doc.de(id, tipoIWAAlmacen)
	if st == nil {
		return nil
	}
	var sb strings.Builder
	st.cada(3, func(c campoPB) { sb.Write(c.b) })
	u := utf16.Encode([]rune(sb.String()))
	parr, car := tablaDeEstilosIWA(st.msg(5), true), tablaDeEstilosIWA(st.msg(8), false)
	niv, listas := tablaDeNivelesIWA(st.msg(6)), tablaDeEstilosIWA(st.msg(7), true)
	adj := tablaDeAdjuntosIWA(st.msg(9))
	rangos := l.rangosComentariosIWA(st, len(u))

	var out []xblock
	var cur *xpara
	var ep, ec uint64
	var comAct *rawComment
	var tramo []uint16
	trasTabla := false
	cerrarTramo := func() {
		if len(tramo) > 0 {
			cur.runs = append(cur.runs, marcarTramoIWA(l.tramoIWA(tramo, ec, ep, cur.style != ""), comAct))
			tramo = tramo[:0]
		}
	}
	for i, c := range u {
		if cur == nil {
			ep = en(parr, i)
			cur = l.abrirParrafoIWA(ep, en(niv, i), en(listas, i))
		}
		if e, com := en(car, i), comentarioEnIWA(rangos, i); e != ec || com != comAct {
			cerrarTramo()
			ec, comAct = e, com
		}
		switch {
		case c == '\n' || c == 0x2029:
			cerrarTramo()
			if !trasTabla || len(cur.runs) > 0 {
				p := *cur
				out = append(out, xblock{para: &p})
			}
			cur, trasTabla = nil, false
		case c == 0x2028:
			cerrarTramo()
			cur.runs = append(cur.runs, xrun{brk: true})
			trasTabla = false
		case c == 0xFFFC:
			out = l.adjuntoEnLineaIWA(adj[i], &cur, &trasTabla, cerrarTramo, out)
		case c < 0x20 && c != '\t':
			// saltos de página, de sección o de columna, y marcas de nota: no son texto
		default:
			tramo = append(tramo, c)
			trasTabla = false
		}
	}
	if cur != nil {
		cerrarTramo()
		if len(cur.runs) > 0 {
			p := *cur
			out = append(out, xblock{para: &p})
		}
	}
	return out
}

// parrafos son los párrafos del almacén `id`.
func (l *lectorIWA) parrafos(id uint64) []xpara {
	var out []xpara
	for _, b := range l.bloques(id) {
		if b.para != nil {
			out = append(out, *b.para)
		}
	}
	return out
}

// importPagesIWA convierte un documento .pages de 2013 en adelante: el cuerpo (con sus
// tablas, imágenes y comentarios anclados) y, detrás, el texto, las tablas y las imágenes
// de los objetos flotantes. "" si no hay contenido.
func importPagesIWA(doc documentoIWA, p *paqueteIWork, conComentarios bool) (cuerpo string, coms []ImportedComment, omitido bool) {
	raiz := doc.de(1, tipoIWAPages)
	if raiz == nil {
		return "", nil, false
	}
	l := nuevoLectorIWA(doc, p, conComentarios)
	bs := l.bloques(raiz.ref(4))
	var agregarFlotante func(id uint64)
	agregarFlotante = func(id uint64) {
		if c := doc.de(id, tipoIWACuadro); c != nil {
			bs = append(bs, l.bloques(c.ref(2))...)
		} else if tb := doc.de(id, tipoIWATabla); tb != nil {
			if t := l.tablaInfoIWA(tb); t != nil {
				bs = append(bs, xblock{table: t.xtable()})
			}
		} else if t, _ := l.graficoIWA(id); t != nil {
			bs = append(bs, xblock{table: t.xtable()})
		} else if src, w, h, _, _ := l.imagenIWA(id); src != "" {
			bs = append(bs, xblock{para: &xpara{runs: []xrun{{raw: p.imgHTML(src, w, h)}}}})
		} else if gr := doc.de(id, tipoIWAGrupo); gr != nil {
			gr.cada(2, func(h campoPB) { agregarFlotante(h.b.uint(1)) })
		} else if doc.de(id, tipoIWAComentarioInfo) != nil {
			l.registrarComentarioIWA(id, "")
		} else {
			l.omitido = true
		}
	}
	doc[raiz.ref(3)].datos.cada(1, func(g campoPB) {
		g.b.cada(4, func(r campoPB) {
			agregarFlotante(r.b.ref(1))
		})
	})
	if !conTexto(bs) {
		return "", nil, l.omitido
	}
	if conComentarios {
		coms = importedComments(l.comentarios)
	}
	return blocksToHTML(bs), coms, l.omitido
}
