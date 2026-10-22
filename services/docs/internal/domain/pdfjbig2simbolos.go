// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// JBIG2 en modo símbolo (T.88, 6.4 y 6.5): el diccionario de símbolos guarda
// cada letra una vez y la región de texto dice dónde va cada aparición. Es lo
// que hace un escáner con «compresión alta» (jbig2enc -s, Acrobat): sin esto la
// página entera caía al recuadro gris.
//
// Las dos codificaciones —aritmética y Huffman (pdfjbig2huffman.go)— y los
// símbolos refinados o agregados de otros, en el diccionario y en la región.
package domain

import "math/bits"

// Topes para entradas hostiles: un diccionario puede declarar millones de
// símbolos en cuatro bytes.
const (
	maxSimbolosJBIG2   = 1 << 16
	maxInstanciasTexto = 1 << 20
)

// enteroMQ es un decodificador de enteros con su propio juego de 512 contextos
// (anexo A.2): IADH, IADW, IAEX, IADT, IAFS, IADS, IAIT.
type enteroMQ struct{ cx [512]contextoMQ }

// leer devuelve el entero, o falso si es OOB (signo negativo con valor cero).
func (e *enteroMQ) leer(mq *decodificadorMQ) (int, bool) {
	prev := 1
	bit := func() int {
		b := int(mq.decodificar(&e.cx[prev]))
		if prev < 256 {
			prev = prev<<1 | b
		} else {
			prev = (prev<<1|b)&511 | 256
		}
		return b
	}
	n := func(k int) int {
		v := 0
		for i := 0; i < k; i++ {
			v = v<<1 | bit()
		}
		return v
	}
	s := bit()
	pasos := [...]struct{ k, base int }{
		{2, 0},
		{4, 4},
		{6, 20},
		{8, 84},
		{12, 340},
	}
	var v int
	leido := false
	for _, p := range pasos {
		if bit() == 0 {
			v = n(p.k) + p.base
			leido = true
			break
		}
	}
	if !leido {
		v = n(32) + 4436
	}
	if s == 1 {
		if v == 0 {
			return 0, false
		}
		return -v, true
	}
	return v, true
}

// idMQ es IAID (A.3): el número de símbolo en `largo` bits.
type idMQ struct {
	largo int
	cx    []contextoMQ
}

func nuevoIDMQ(largo int) *idMQ { return &idMQ{largo: largo, cx: make([]contextoMQ, 1<<(largo+1))} }

func (d *idMQ) leer(mq *decodificadorMQ) int {
	prev := 1
	for i := 0; i < d.largo; i++ {
		prev = prev<<1 | int(mq.decodificar(&d.cx[prev]))
	}
	return prev - 1<<d.largo
}

// leerAT lee los píxeles adaptativos de una plantilla genérica. Son enteros CON
// SIGNO de un byte: un −3 viaja como 0xFD, y leerlos sin signo pondría los
// vecinos del contexto 253 píxeles a la derecha. Se devuelven como int porque el
// diccionario de patrones usa −HDPW, que no cabe en un byte (6.7.5).
func leerAT(b []byte, plantilla int) [][2]int {
	n := 1
	if plantilla == 0 {
		n = 4
	}
	at := make([][2]int, 0, n)
	for i := 0; i < n && 2*i+1 < len(b); i++ {
		at = append(at, [2]int{int(int8(b[2*i])), int(int8(b[2*i+1]))}) //nolint:gosec // G115: enteros con signo de un byte (7.4.6.3)
	}
	return at
}

// fuenteJBIG2 da los valores de un diccionario de símbolos o de una región de
// texto: por el codificador aritmético, cada uno con su juego de contextos
// (anexo A), o por Huffman, cada uno con su tabla (anexo B).
type fuenteJBIG2 struct {
	mq   *decodificadorMQ
	bits *lectorBits
	err  error

	iadh, iadw, iaex, iaai, iadt, iafs, iads, iait, iari, iardw, iardh, iardx, iardy enteroMQ
	iaid                                                                             *idMQ

	tdh, tdw, tbmsize, tagg, tfs, tds, tdt, trdw, trdh, trdx, trdy, trsize, tid *tablaHuffman

	cxGB, cxGR []contextoMQ
}

// entero lee un valor; falso si es OOB o si el flujo se acabó (f.err lo dice).
func (f *fuenteJBIG2) entero(e *enteroMQ, t *tablaHuffman) (int, bool) {
	if f.err != nil {
		return 0, false
	}
	if f.bits == nil {
		return e.leer(f.mq)
	}
	if t == nil {
		f.err = ErrJBIG2NoSoportado
		return 0, false
	}
	v, oob, err := t.leer(f.bits)
	if err != nil {
		f.err = err
		return 0, false
	}
	return v, !oob
}

// valor es entero para los campos que no admiten OOB: un OOB ahí es un error.
func (f *fuenteJBIG2) valor(e *enteroMQ, t *tablaHuffman) int {
	v, ok := f.entero(e, t)
	if !ok && f.err == nil {
		f.err = ErrJBIG2NoSoportado
	}
	return v
}

func (f *fuenteJBIG2) id() int {
	if f.bits == nil {
		return f.iaid.leer(f.mq)
	}
	return f.valor(nil, f.tid)
}

// refinar decodifica un mapa de ancho×alto refinando ref (6.3). Con Huffman
// el refinamiento va aritmético en un trozo de bytes con su longitud delante
// (6.4.11, pasos 5 y 7); los contextos sí se conservan entre trozos.
func (f *fuenteJBIG2) refinar(ref *mapaBits, ancho, alto, dx, dy, plantilla int, at [][2]int) *mapaBits {
	if f.err != nil {
		return nil
	}
	if ancho < 0 || alto < 0 || ancho*alto > maxPixelesJBIG2 {
		f.err = ErrJBIG2NoSoportado
		return nil
	}
	if f.bits == nil {
		return decodificarRefinamiento(f.mq, f.cxGR, ancho, alto, ref, dx, dy, plantilla, at, false)
	}
	n := f.valor(nil, f.trsize)
	f.bits.alinear()
	ini := f.bits.pos / 8
	if f.err != nil || n < 0 || ini+n > len(f.bits.datos) {
		f.err = ErrJBIG2NoSoportado
		return nil
	}
	m := decodificarRefinamiento(nuevoMQ(f.bits.datos[ini:ini+n]), f.cxGR, ancho, alto, ref, dx, dy, plantilla, at, false)
	f.bits.pos = (ini + n) * 8
	return m
}

// elegirTabla da la estándar de la selección o, con la de «usuario», la
// siguiente de las tablas referidas (7.4.2.1.6 y 7.4.3.1.6).
func elegirTabla(sel, usuario int, estandar []int, propias *[]*tablaHuffman) *tablaHuffman {
	if sel == usuario {
		if len(*propias) == 0 {
			return nil
		}
		t := (*propias)[0]
		*propias = (*propias)[1:]
		return t
	}
	if sel < len(estandar) {
		return tablasEstandar[estandar[sel]]
	}
	return nil
}

// contextosRetenidos son las estadísticas de mapa que un diccionario guarda
// para el siguiente (7.4.2.2, pasos 3 y 7).
type contextosRetenidos struct{ gb, gr []contextoMQ }

type cabeceraDiccionarioSimbolos struct {
	huff, refagg, usado, retenido bool
	plantilla, rplantilla         int
	at, rat                       [][2]int
	exportados, nuevos, q         int
	selDH, selDW, selBM, selAgg   int
}

func leerCabeceraDiccionarioSimbolos(cuerpo []byte, entrada []*mapaBits) (cabeceraDiccionarioSimbolos, error) {
	var c cabeceraDiccionarioSimbolos
	if len(cuerpo) < 2 {
		return c, ErrJBIG2NoSoportado
	}
	b := int(cuerpo[0])<<8 | int(cuerpo[1])
	c.huff, c.refagg = b&1 != 0, b&2 != 0
	c.selDH, c.selDW, c.selBM, c.selAgg = b>>2&3, b>>4&3, b>>6&1, b>>7&1
	c.usado, c.retenido = b&0x100 != 0, b&0x200 != 0
	c.plantilla, c.rplantilla = b>>10&3, b>>12&1
	q := 2
	if !c.huff {
		n := 2
		if c.plantilla == 0 {
			n = 8
		}
		if q+n > len(cuerpo) {
			return c, ErrJBIG2NoSoportado
		}
		c.at = leerAT(cuerpo[q:q+n], c.plantilla)
		q += n
	}
	if c.refagg && c.rplantilla == 0 {
		if q+4 > len(cuerpo) {
			return c, ErrJBIG2NoSoportado
		}
		c.rat = leerAT(cuerpo[q:q+4], 0)[:2]
		q += 4
	}
	if q+8 > len(cuerpo) {
		return c, ErrJBIG2NoSoportado
	}
	be := func(i int) int {
		return int(uint32(cuerpo[i])<<24 | uint32(cuerpo[i+1])<<16 | uint32(cuerpo[i+2])<<8 | uint32(cuerpo[i+3]))
	}
	c.exportados, c.nuevos = be(q), be(q+4)
	c.q = q + 8
	if c.nuevos < 0 || c.nuevos > maxSimbolosJBIG2 || c.exportados < 0 || c.exportados > len(entrada)+c.nuevos {
		return c, ErrJBIG2NoSoportado
	}
	return c, nil
}

// decodificarDiccionarioSimbolos es el segmento 0 (7.4.2 y 6.5). Devuelve los
// símbolos EXPORTADOS, que es lo que ven las regiones de texto que lo refieren,
// y las estadísticas si el diccionario las retiene.
func decodificarDiccionarioSimbolos(cuerpo []byte, entrada []*mapaBits, propias []*tablaHuffman, previo *contextosRetenidos) ([]*mapaBits, *contextosRetenidos, error) {
	cab, err := leerCabeceraDiccionarioSimbolos(cuerpo, entrada)
	if err != nil {
		return nil, nil, err
	}
	total := len(entrada) + cab.nuevos
	f := nuevaFuenteDiccionario(cab, cuerpo, total, propias, previo)

	nuevos := make([]*mapaBits, 0, cab.nuevos)
	alto := 0
	for len(nuevos) < cab.nuevos && f.err == nil {
		alto += f.valor(&f.iadh, f.tdh)
		ancho, anchoTotal, primero := 0, 0, len(nuevos)
		for f.err == nil {
			dw, ok := f.entero(&f.iadw, f.tdw)
			if !ok {
				break // fin de la clase de altura
			}
			ancho += dw
			anchoTotal += ancho
			if ancho < 0 || alto < 0 || ancho*alto > maxPixelesJBIG2 || anchoTotal*alto > maxPixelesJBIG2 || len(nuevos) >= cab.nuevos {
				return nil, nil, ErrJBIG2NoSoportado
			}
			nuevos = append(nuevos, f.simbolo(cab, ancho, alto, entrada, nuevos))
		}
		if cab.huff && !cab.refagg && f.err == nil {
			f.colectivo(nuevos[primero:], anchoTotal, alto)
		}
	}
	if f.err != nil {
		return nil, nil, f.err
	}
	exp, err := exportarSimbolosJBIG2(f, entrada, nuevos, cab.exportados)
	if err != nil {
		return nil, nil, err
	}
	var ret *contextosRetenidos
	if cab.retenido {
		ret = &contextosRetenidos{gb: f.cxGB, gr: f.cxGR}
	}
	return exp, ret, nil
}

// nuevaFuenteDiccionario prepara la fuente de un diccionario: estadísticas
// heredadas si las usa, tablas de Huffman o el decodificador aritmético, y los
// códigos de símbolo de la agregación.
func nuevaFuenteDiccionario(cab cabeceraDiccionarioSimbolos, cuerpo []byte, total int, propias []*tablaHuffman, previo *contextosRetenidos) *fuenteJBIG2 {
	f := &fuenteJBIG2{cxGB: make([]contextoMQ, 1<<16), cxGR: make([]contextoMQ, 1<<13)}
	if cab.usado && previo != nil {
		copy(f.cxGB, previo.gb)
		copy(f.cxGR, previo.gr)
	}
	if cab.huff {
		f.bits = &lectorBits{datos: cuerpo[cab.q:]}
		f.tdh = elegirTabla(cab.selDH, 3, []int{4, 5}, &propias)
		f.tdw = elegirTabla(cab.selDW, 3, []int{2, 3}, &propias)
		f.tbmsize = elegirTabla(cab.selBM, 1, []int{1}, &propias)
		f.tagg = elegirTabla(cab.selAgg, 1, []int{1}, &propias)
		// La agregación usa siempre estas (tabla 17) y códigos de símbolo de
		// longitud fija (6.5.8.2.3).
		f.tfs, f.tds, f.tdt = tablasEstandar[6], tablasEstandar[8], tablasEstandar[11]
		f.trdw, f.trdh, f.trdx, f.trdy, f.trsize = tablasEstandar[15], tablasEstandar[15], tablasEstandar[15], tablasEstandar[15], tablasEstandar[1]
		largo := max(bits.Len(uint(max(total-1, 0))), 1)
		lineas := make([]lineaHuffman, total)
		for i := range lineas {
			lineas[i] = lh(largo, 0, i)
		}
		f.tid = nuevaTablaHuffman(lineas)
	} else {
		f.mq = nuevoMQ(cuerpo[cab.q:])
		f.iaid = nuevoIDMQ(bits.Len(uint(max(total-1, 0))))
	}

	return f
}

// simbolo es 6.5.8: directo, agregado o, con Huffman sin agregación, vacío
// hasta que llegue el colectivo de su clase de altura.
func (f *fuenteJBIG2) simbolo(cab cabeceraDiccionarioSimbolos, ancho, alto int, entrada, nuevos []*mapaBits) *mapaBits {
	switch {
	case cab.refagg:
		return f.simboloAgregado(cab, ancho, alto, append(append([]*mapaBits(nil), entrada...), nuevos...))
	case cab.huff || ancho == 0 || alto == 0:
		return nuevoMapaBits(ancho, alto)
	default:
		return decodificarRegionGenerica(f.mq, f.cxGB, ancho, alto, cab.plantilla, cab.at, false, nil)
	}
}

// colectivo es 6.5.9: los símbolos de una clase de altura van juntos en un
// mapa MMR, o sin comprimir si BMSIZE es 0, y se recortan por su ancho.
func (f *fuenteJBIG2) colectivo(simbolos []*mapaBits, anchoTotal, alto int) {
	n := f.valor(nil, f.tbmsize)
	f.bits.alinear()
	ini := f.bits.pos / 8
	if f.err != nil {
		return
	}
	todo := nuevoMapaBits(anchoTotal, alto)
	if n == 0 {
		porFila := (anchoTotal + 7) / 8
		n = porFila * alto
		if ini+n > len(f.bits.datos) {
			f.err = ErrJBIG2NoSoportado
			return
		}
		for y := 0; y < alto; y++ {
			for x := 0; x < anchoTotal; x++ {
				todo.px[y*anchoTotal+x] = f.bits.datos[ini+y*porFila+x/8] >> uint(7-x%8) & 1
			}
		}
	} else {
		if n < 0 || ini+n > len(f.bits.datos) {
			f.err = ErrJBIG2NoSoportado
			return
		}
		if alto > 0 && anchoTotal > 0 {
			m, _, err := mapaMMR(f.bits.datos[ini:ini+n], anchoTotal, alto)
			if err != nil {
				f.err = err
				return
			}
			todo = m
		}
	}
	f.bits.pos = (ini + n) * 8
	x0 := 0
	for _, s := range simbolos {
		for y := 0; y < alto; y++ {
			copy(s.px[y*s.ancho:(y+1)*s.ancho], todo.px[y*anchoTotal+x0:])
		}
		x0 += s.ancho
	}
}

// simboloAgregado es 6.5.8.2: el símbolo es un refinamiento de otro (una sola
// instancia) o una región de texto con varios.
func (f *fuenteJBIG2) simboloAgregado(cab cabeceraDiccionarioSimbolos, ancho, alto int, sbsyms []*mapaBits) *mapaBits {
	n := f.valor(&f.iaai, f.tagg)
	if f.err != nil {
		return nil
	}
	if n == 1 {
		id := f.id()
		rdx := f.valor(&f.iardx, f.trdx)
		rdy := f.valor(&f.iardy, f.trdy)
		if f.err != nil || id < 0 || id >= len(sbsyms) {
			f.err = ErrJBIG2NoSoportado
			return nil
		}
		return f.refinar(sbsyms[id], ancho, alto, rdx, rdy, cab.rplantilla, cab.rat)
	}
	if n < 1 || n > maxInstanciasTexto {
		f.err = ErrJBIG2NoSoportado
		return nil
	}
	inf := infoRegionTexto{
		ancho: ancho, alto: alto, instancias: n, esquina: 1,
		refinar: true, rplantilla: cab.rplantilla, rat: cab.rat,
	}
	return f.texto(inf, sbsyms)
}

func exportarSimbolosJBIG2(f *fuenteJBIG2, entrada, nuevos []*mapaBits, exportados int) ([]*mapaBits, error) {
	// Qué se exporta: rachas alternas de «no» y «sí» sobre entrada + nuevos.
	todos := append(append([]*mapaBits(nil), entrada...), nuevos...)
	out := make([]*mapaBits, 0, exportados)
	marca := false
	for i := 0; i < len(todos); {
		n := f.valor(&f.iaex, tablasEstandar[1])
		if f.err != nil || n < 0 || i+n > len(todos) {
			return nil, ErrJBIG2NoSoportado
		}
		if marca {
			out = append(out, todos[i:i+n]...)
		}
		i += n
		marca = !marca
	}
	return out, nil
}

// regionJBIG2 es una región ya decodificada: su mapa, dónde va y con qué
// operador se compone.
type regionJBIG2 struct {
	mapa   *mapaBits
	x0, y0 int
	op     int
}

type infoRegionTexto struct {
	x0, y0, op, ancho, alto                        int
	logTiras, esquina, opSimbolo, defecto, desfase int
	traspuesto, huff, refinar                      bool
	rplantilla                                     int
	rat                                            [][2]int
	selHuff                                        int
	instancias, q                                  int
}

func leerCabeceraRegionTexto(cuerpo []byte) (infoRegionTexto, error) {
	if len(cuerpo) < 17+2+4 {
		return infoRegionTexto{}, ErrJBIG2NoSoportado
	}
	be := func(i int) int {
		return int(uint32(cuerpo[i])<<24 | uint32(cuerpo[i+1])<<16 | uint32(cuerpo[i+2])<<8 | uint32(cuerpo[i+3]))
	}
	ancho, alto := be(0), be(4)
	if ancho <= 0 || alto <= 0 || ancho*alto > maxPixelesJBIG2 {
		return infoRegionTexto{}, ErrJBIG2NoSoportado
	}
	b := int(cuerpo[17])<<8 | int(cuerpo[18])
	desfase := b >> 10 & 0x1F
	if desfase >= 16 {
		desfase -= 32 // SBDSOFFSET es de cinco bits con signo
	}
	inf := infoRegionTexto{
		ancho:      ancho,
		alto:       alto,
		x0:         be(8),
		y0:         be(12),
		op:         int(cuerpo[16] & 7),
		huff:       b&1 != 0,
		refinar:    b&2 != 0,
		logTiras:   b >> 2 & 3,
		esquina:    b >> 4 & 3,
		traspuesto: b&0x40 != 0,
		opSimbolo:  b >> 7 & 3,
		defecto:    b >> 9 & 1,
		desfase:    desfase,
		rplantilla: b >> 15 & 1,
	}
	q := 19
	if inf.huff {
		inf.selHuff = int(cuerpo[q])<<8 | int(cuerpo[q+1])
		q += 2
	}
	if inf.refinar && inf.rplantilla == 0 {
		if q+4 > len(cuerpo) {
			return inf, ErrJBIG2NoSoportado
		}
		inf.rat = leerAT(cuerpo[q:q+4], 0)[:2]
		q += 4
	}
	if q+4 > len(cuerpo) {
		return inf, ErrJBIG2NoSoportado
	}
	inf.instancias = be(q)
	inf.q = q + 4
	if inf.instancias < 0 || inf.instancias > maxInstanciasTexto {
		return inf, ErrJBIG2NoSoportado
	}
	return inf, nil
}

func componerInstanciaTexto(m, ib *mapaBits, curS, t, opSimbolo, esquina int, traspuesto bool) int {
	wi, hi := ib.ancho, ib.alto
	if !traspuesto && (esquina == 2 || esquina == 3) {
		curS += wi - 1
	} else if traspuesto && (esquina == 0 || esquina == 2) {
		curS += hi - 1
	}
	// Esquina de referencia en (S, T), o en (T, S) si va traspuesto.
	x, y := curS, t
	if traspuesto {
		x, y = t, curS
	}
	if esquina == 2 || esquina == 3 { // derecha
		x -= wi - 1
	}
	if esquina == 0 || esquina == 2 { // abajo
		y -= hi - 1
	}
	componerJBIG2(m, ib, x, y, opSimbolo)
	if !traspuesto && (esquina == 0 || esquina == 1) {
		curS += wi - 1
	} else if traspuesto && (esquina == 1 || esquina == 3) {
		curS += hi - 1
	}
	return curS
}

// texto es 6.4.5: coloca cada instancia de símbolo donde dice la región,
// refinada si RI lo pide.
func (f *fuenteJBIG2) texto(inf infoRegionTexto, simbolos []*mapaBits) *mapaBits {
	m := nuevoMapaBits(inf.ancho, inf.alto)
	if inf.defecto == 1 {
		for i := range m.px {
			m.px[i] = 1
		}
	}
	tiras := 1 << inf.logTiras
	tira := -f.valor(&f.iadt, f.tdt) * tiras
	primeraS := 0
	for n := 0; n < inf.instancias && f.err == nil; {
		tira += f.valor(&f.iadt, f.tdt) * tiras
		curS := 0
		// La tira acaba en OOB también tras la última instancia (6.4.5, paso 3 c):
		// dentro de un diccionario el flujo sigue y hay que consumirlo.
		for primera := true; f.err == nil; primera = false {
			if primera {
				primeraS += f.valor(&f.iafs, f.tfs)
				curS = primeraS
			} else {
				ids, ok := f.entero(&f.iads, f.tds)
				if !ok || n >= inf.instancias {
					break // fin de la tira
				}
				curS += ids + inf.desfase
			}
			curT, ib := f.instancia(inf, simbolos)
			if ib == nil {
				return nil
			}
			curS = componerInstanciaTexto(m, ib, curS, tira+curT, inf.opSimbolo, inf.esquina, inf.traspuesto)
			n++
		}
	}
	return m
}

// instancia lee la T relativa, el símbolo y, si RI lo pide, su refinamiento
// (6.4.9 a 6.4.11). Un nil es un error y lo dice f.err.
func (f *fuenteJBIG2) instancia(inf infoRegionTexto, simbolos []*mapaBits) (int, *mapaBits) {
	curT := 0
	if inf.logTiras > 0 {
		if f.bits != nil {
			v, err := leerN(f.bits, inf.logTiras)
			if err != nil {
				f.err = err
			}
			curT = v
		} else {
			curT = f.valor(&f.iait, nil)
		}
	}
	id := f.id()
	if f.err != nil || id < 0 || id >= len(simbolos) {
		f.err = ErrJBIG2NoSoportado
		return 0, nil
	}
	ib := simbolos[id]
	if inf.refinar && f.ri() == 1 {
		ib = f.instanciaRefinada(ib, inf)
	}
	return curT, ib
}

func (f *fuenteJBIG2) ri() int {
	if f.bits == nil {
		v, _ := f.iari.leer(f.mq)
		return v
	}
	b := f.bits.bit()
	if b < 0 {
		f.err = errJBIG2Huffman
		return 0
	}
	return b
}

// instanciaRefinada es 6.4.11 con RI = 1 (tabla 12): el desplazamiento de la
// referencia es la mitad del cambio de tamaño, redondeada hacia abajo, más el
// suyo propio.
func (f *fuenteJBIG2) instanciaRefinada(ref *mapaBits, inf infoRegionTexto) *mapaBits {
	rdw := f.valor(&f.iardw, f.trdw)
	rdh := f.valor(&f.iardh, f.trdh)
	rdx := f.valor(&f.iardx, f.trdx)
	rdy := f.valor(&f.iardy, f.trdy)
	if f.err != nil {
		return nil
	}
	return f.refinar(ref, ref.ancho+rdw, ref.alto+rdh, rdw>>1+rdx, rdh>>1+rdy, inf.rplantilla, inf.rat)
}

// decodificarRegionTexto es el segmento 4/6/7 (7.4.3), aritmético o Huffman.
func decodificarRegionTexto(cuerpo []byte, simbolos []*mapaBits, propias []*tablaHuffman) (*regionJBIG2, error) {
	inf, err := leerCabeceraRegionTexto(cuerpo)
	if err != nil {
		return nil, err
	}
	f := &fuenteJBIG2{cxGR: make([]contextoMQ, 1<<13)}
	if inf.huff {
		s := inf.selHuff
		f.tfs = elegirTabla(s&3, 3, []int{6, 7}, &propias)
		f.tds = elegirTabla(s>>2&3, 3, []int{8, 9, 10}, &propias)
		f.tdt = elegirTabla(s>>4&3, 3, []int{11, 12, 13}, &propias)
		f.trdw = elegirTabla(s>>6&3, 3, []int{14, 15}, &propias)
		f.trdh = elegirTabla(s>>8&3, 3, []int{14, 15}, &propias)
		f.trdx = elegirTabla(s>>10&3, 3, []int{14, 15}, &propias)
		f.trdy = elegirTabla(s>>12&3, 3, []int{14, 15}, &propias)
		f.trsize = elegirTabla(s>>14&1, 1, []int{1}, &propias)
		f.bits = &lectorBits{datos: cuerpo[inf.q:]}
		if f.tid, err = leerCodigosID(f.bits, len(simbolos)); err != nil {
			return nil, err
		}
	} else {
		f.mq = nuevoMQ(cuerpo[inf.q:])
		f.iaid = nuevoIDMQ(bits.Len(uint(max(len(simbolos)-1, 0))))
	}
	m := f.texto(inf, simbolos)
	if f.err != nil {
		return nil, f.err
	}
	return &regionJBIG2{mapa: m, x0: inf.x0, y0: inf.y0, op: inf.op}, nil
}
