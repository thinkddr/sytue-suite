// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// JBIG2 embebido en PDF (§7.4.7 y la norma T.88): el formato de los escaneos de
// «alta compresión».
//
// POR QUÉ IMPORTA. Cuando un escáner o un «optimizador» de PDF ofrece comprimir
// mucho, lo que hace es pasar la página a JBIG2. Un contrato escaneado de 20 MB
// baja a 400 KB. El precio es que ningún navegador sabe abrirlo, así que sin esto
// esas páginas salían con un recuadro gris — y son documentos completos, no
// adornos.
//
// QUÉ SE IMPLEMENTA: la REGIÓN GENÉRICA con codificador aritmético MQ y
// plantillas 0 a 3, y el MODO SÍMBOLO —diccionario de símbolos y región de
// texto, la parte que reconoce letras repetidas y las guarda una vez— con
// codificación aritmética o Huffman (pdfjbig2simbolos.go, pdfjbig2huffman.go).
//
// También: la información de página (su píxel por defecto), los cinco operadores
// con que cada región se compone (OR, AND, XOR, XNOR, REPLACE), la región de
// longitud desconocida que acaba en un marcador, y la región genérica MMR, que es
// CCITT G4 y va por `pdfccitt.go`.
//
// Y el refinamiento (pdfjbig2refina.go) y los semitonos con su diccionario de
// patrones (pdfjbig2semitonos.go). `JBIG2Globals`, el flujo con los segmentos
// compartidos entre páginas, se recorre antes que la página.
//
// LO QUE NO SE HACE, dicho para que no se busque: la codificación Huffman de
// símbolos y texto.
//
// El decodificador aritmético es el de la norma tal cual —la tabla Qe incluida—
// porque es la única forma de que los bits salgan en el orden correcto: aquí no
// hay margen de interpretación y una tabla mal copiada da una imagen de ruido en
// vez de un error.
package domain

import (
	"errors"
	"fmt"
)

// ErrJBIG2NoSoportado marca los ficheros con segmentos que este decodificador no
// entiende. Se distingue de un error para que quien llame pueda caer al recuadro
// gris en vez de tratarlo como un PDF roto.
var ErrJBIG2NoSoportado = errors.New("jbig2: segmento no soportado")

// --- Decodificador aritmético MQ (T.88, anexo E) -----------------------------

// tablaQe es la tabla de estados del codificador aritmético, copiada de la norma
// (tabla E.1). Cada fila: Qe, NMPS, NLPS, SWITCH.
//
// No se «entiende», se copia. Un valor cambiado no da error: descoloca todas las
// decisiones siguientes y la imagen sale como nieve.
//
// Los tres últimos campos son índices de ESTA tabla (0-46) y una bandera, así que
// se guardan como `uint8` y no como `uint32`: además de ser lo que son, evita una
// conversión en el bucle más caliente del fichero.
var tablaQe = [47]struct {
	qe                 uint32
	nmps, nlps, cambio uint8
}{
	{0x5601, 1, 1, 1},
	{0x3401, 2, 6, 0},
	{0x1801, 3, 9, 0},
	{0x0AC1, 4, 12, 0},
	{0x0521, 5, 29, 0},
	{0x0221, 38, 33, 0},
	{0x5601, 7, 6, 1},
	{0x5401, 8, 14, 0},
	{0x4801, 9, 14, 0},
	{0x3801, 10, 14, 0},
	{0x3001, 11, 17, 0},
	{0x2401, 12, 18, 0},
	{0x1C01, 13, 20, 0},
	{0x1601, 29, 21, 0},
	{0x5601, 15, 14, 1},
	{0x5401, 16, 14, 0},
	{0x5101, 17, 15, 0},
	{0x4801, 18, 16, 0},
	{0x3801, 19, 17, 0},
	{0x3401, 20, 18, 0},
	{0x3001, 21, 19, 0},
	{0x2801, 22, 19, 0},
	{0x2401, 23, 20, 0},
	{0x2201, 24, 21, 0},
	{0x1C01, 25, 22, 0},
	{0x1801, 26, 23, 0},
	{0x1601, 27, 24, 0},
	{0x1401, 28, 25, 0},
	{0x1201, 29, 26, 0},
	{0x1101, 30, 27, 0},
	{0x0AC1, 31, 28, 0},
	{0x09C1, 32, 29, 0},
	{0x08A1, 33, 30, 0},
	{0x0521, 34, 31, 0},
	{0x0441, 35, 32, 0},
	{0x02A1, 36, 33, 0},
	{0x0221, 37, 34, 0},
	{0x0141, 38, 35, 0},
	{0x0111, 39, 36, 0},
	{0x0085, 40, 37, 0},
	{0x0049, 41, 38, 0},
	{0x0025, 42, 39, 0},
	{0x0015, 43, 40, 0},
	{0x0009, 44, 41, 0},
	{0x0005, 45, 42, 0},
	{0x0001, 45, 43, 0},
	{0x5601, 46, 46, 0},
}

// contextoMQ es el estado de UN contexto: su índice en la tabla y el bit más
// probable.
type contextoMQ struct {
	i   uint8
	mps uint8
}

// decodificadorMQ es el descompresor aritmético.
type decodificadorMQ struct {
	datos []byte
	bp    int
	c     uint32
	a     uint32
	ct    int
}

func nuevoMQ(datos []byte) *decodificadorMQ {
	d := &decodificadorMQ{datos: datos}
	d.init()
	return d
}

func (d *decodificadorMQ) byteEn(i int) uint32 {
	if i < 0 || i >= len(d.datos) {
		return 0xFF // más allá del final se comporta como relleno
	}
	return uint32(d.datos[i])
}

// init es INITDEC de la norma.
func (d *decodificadorMQ) init() {
	d.bp = 0
	d.c = d.byteEn(0) << 16
	d.byteIn()
	d.c <<= 7
	d.ct -= 7
	d.a = 0x8000
}

// byteIn es BYTEIN: alimenta el registro y aplica la regla del relleno tras un
// 0xFF, que es donde el flujo marca su final.
func (d *decodificadorMQ) byteIn() {
	if d.byteEn(d.bp) == 0xFF {
		if d.byteEn(d.bp+1) > 0x8F {
			d.c += 0xFF00
			d.ct = 8
		} else {
			d.bp++
			d.c += d.byteEn(d.bp) << 9
			d.ct = 7
		}
		return
	}
	d.bp++
	d.c += d.byteEn(d.bp) << 8
	d.ct = 8
}

// decodificar es DECODE: devuelve el siguiente bit para el contexto dado.
func (d *decodificadorMQ) decodificar(cx *contextoMQ) uint8 {
	qe := tablaQe[cx.i].qe
	d.a -= qe

	var bit uint8
	if (d.c>>16)&0xFFFF < qe {
		// LPS o MPS según el intervalo, que es lo que decide la norma en MPSEXCHANGE
		// y LPSEXCHANGE.
		if d.a < qe {
			bit = cx.mps
			cx.i = tablaQe[cx.i].nmps
		} else {
			bit = 1 - cx.mps
			if tablaQe[cx.i].cambio == 1 {
				cx.mps = 1 - cx.mps
			}
			cx.i = tablaQe[cx.i].nlps
		}
		d.a = qe
	} else {
		d.c -= qe << 16
		if d.a&0x8000 != 0 {
			return cx.mps
		}
		if d.a < qe {
			bit = 1 - cx.mps
			if tablaQe[cx.i].cambio == 1 {
				cx.mps = 1 - cx.mps
			}
			cx.i = tablaQe[cx.i].nlps
		} else {
			bit = cx.mps
			cx.i = tablaQe[cx.i].nmps
		}
	}

	// RENORMD: se normaliza hasta que A vuelve a tener el bit alto.
	for {
		if d.ct == 0 {
			d.byteIn()
		}
		d.a <<= 1
		d.c <<= 1
		d.ct--
		if d.a&0x8000 != 0 {
			break
		}
	}
	return bit
}

// --- Mapa de bits ------------------------------------------------------------

// mapaBits es una imagen de un bit por píxel, un byte por píxel en memoria.
//
// Se guarda desempaquetado a propósito: la región genérica consulta hasta diez
// píxeles vecinos por cada uno que decodifica, y hacerlo con máscaras de bits
// multiplica el coste del bucle más caliente del fichero.
type mapaBits struct {
	ancho, alto int
	px          []uint8
}

func nuevoMapaBits(ancho, alto int) *mapaBits {
	return &mapaBits{ancho: ancho, alto: alto, px: make([]uint8, ancho*alto)}
}

// en devuelve el píxel, y 0 fuera de los bordes. Ese 0 no es una comodidad: la
// norma define que el contexto de los bordes se rellena con ceros, así que
// devolver otra cosa cambiaría la imagen.
func (m *mapaBits) en(x, y int) uint8 {
	if x < 0 || y < 0 || x >= m.ancho || y >= m.alto {
		return 0
	}
	return m.px[y*m.ancho+x]
}

// marcar pone tinta en un píxel, si cae dentro. No hay `poner(v)` porque
// componer una región sobre la página solo AÑADE: el operador por defecto es OR y
// es el único que tiene sentido para tinta.
func (m *mapaBits) marcar(x, y int) {
	if x >= 0 && y >= 0 && x < m.ancho && y < m.alto {
		m.px[y*m.ancho+x] = 1
	}
}

// empaquetar pasa a un bit por píxel, que es lo que espera el resto del render.
//
// EL SENTIDO SE INVIERTE: en JBIG2 un 1 es tinta (negro), y en una imagen de PDF
// en escala de grises de un bit el 0 es negro. Sin invertir aquí, el escaneo sale
// en negativo.
func (m *mapaBits) empaquetar() []byte {
	porFila := (m.ancho + 7) / 8
	out := make([]byte, porFila*m.alto)
	for y := 0; y < m.alto; y++ {
		for x := 0; x < m.ancho; x++ {
			if m.px[y*m.ancho+x] == 0 {
				out[y*porFila+x/8] |= 1 << uint(7-x%8)
			}
		}
	}
	return out
}

// --- Región genérica ---------------------------------------------------------

// maxPixelesJBIG2 acota lo que se decodifica. Un segmento puede declarar una
// región de 60000×60000 en su cabecera y son cuatro bytes.
const maxPixelesJBIG2 = 40 << 20

// decodificarRegionGenerica es la 6.2.5.7 de la norma: por cada píxel se arma un
// contexto con los vecinos ya decodificados y se le pide un bit al MQ.
//
// Las cuatro plantillas se diferencian en QUÉ vecinos entran en el contexto. Se
// implementan las cuatro porque el fichero elige y no avisa: usar la plantilla
// equivocada no da error, da ruido.
//
// skip, si no es nil, marca los píxeles que no se decodifican y quedan a 0
// (USESKIP, 6.2.5.6): los valores de semitono que caen fuera de la región.
func decodificarRegionGenerica(mq *decodificadorMQ, cx []contextoMQ, ancho, alto, plantilla int, at [][2]int, tpgdon bool, skip *mapaBits) *mapaBits {
	m := nuevoMapaBits(ancho, alto)
	// LTP es la «línea típica»: con TPGDON activado, un bit por fila dice si es
	// idéntica a la anterior, que en un escaneo con márgenes blancos ahorra
	// muchísimo. Ignorarlo desplaza toda la imagen.
	ltp := false

	for y := 0; y < alto; y++ {
		if tpgdon {
			ctx := contextoTPGD(plantilla)
			if mq.decodificar(&cx[ctx]) == 1 {
				ltp = !ltp
			}
			if ltp {
				if y > 0 {
					copy(m.px[y*ancho:(y+1)*ancho], m.px[(y-1)*ancho:y*ancho])
				}
				continue
			}
		}
		for x := 0; x < ancho; x++ {
			if skip != nil && skip.px[y*ancho+x] == 1 {
				continue
			}
			ctx := contextoGenerico(m, x, y, plantilla, at)
			m.px[y*ancho+x] = mq.decodificar(&cx[ctx])
		}
	}
	return m
}

// contextoTPGD son los valores de contexto que la norma reserva para el bit de
// línea típica, uno por plantilla (6.2.5.7).
func contextoTPGD(plantilla int) uint16 {
	switch plantilla {
	case 0:
		return 0x9B25
	case 1:
		return 0x0795
	case 2:
		return 0x00E5
	default:
		return 0x0195
	}
}

// contextoGenerico arma el número de contexto de un píxel.
//
// El orden de los bits NO es libre: es el de las figuras 8 a 11 de la norma, y
// cambiarlo produce una imagen distinta sin dar ningún error. Por eso está
// escrito literal, plantilla a plantilla, en vez de con un bucle «elegante» sobre
// una tabla de desplazamientos.
func contextoGenerico(m *mapaBits, x, y, plantilla int, at [][2]int) uint16 {
	a := func(i int) uint8 {
		if i < len(at) {
			return m.en(x+at[i][0], y+at[i][1])
		}
		return 0
	}
	switch plantilla {
	case 0:
		return uint16(m.en(x-1, y))<<0 | uint16(m.en(x-2, y))<<1 | uint16(m.en(x-3, y))<<2 |
			uint16(m.en(x-4, y))<<3 | uint16(a(0))<<4 |
			uint16(m.en(x+2, y-1))<<5 | uint16(m.en(x+1, y-1))<<6 | uint16(m.en(x, y-1))<<7 |
			uint16(m.en(x-1, y-1))<<8 | uint16(m.en(x-2, y-1))<<9 | uint16(a(1))<<10 |
			uint16(a(2))<<11 | uint16(m.en(x+1, y-2))<<12 | uint16(m.en(x, y-2))<<13 |
			uint16(m.en(x-1, y-2))<<14 | uint16(a(3))<<15
	case 1:
		return uint16(m.en(x-1, y))<<0 | uint16(m.en(x-2, y))<<1 | uint16(m.en(x-3, y))<<2 |
			uint16(a(0))<<3 | uint16(m.en(x+2, y-1))<<4 | uint16(m.en(x+1, y-1))<<5 |
			uint16(m.en(x, y-1))<<6 | uint16(m.en(x-1, y-1))<<7 | uint16(m.en(x-2, y-1))<<8 |
			uint16(m.en(x+2, y-2))<<9 | uint16(m.en(x+1, y-2))<<10 | uint16(m.en(x, y-2))<<11 |
			uint16(m.en(x-1, y-2))<<12
	case 2:
		return uint16(m.en(x-1, y))<<0 | uint16(m.en(x-2, y))<<1 | uint16(a(0))<<2 |
			uint16(m.en(x+1, y-1))<<3 | uint16(m.en(x, y-1))<<4 | uint16(m.en(x-1, y-1))<<5 |
			uint16(m.en(x-2, y-1))<<6 | uint16(m.en(x+1, y-2))<<7 | uint16(m.en(x, y-2))<<8 |
			uint16(m.en(x-1, y-2))<<9
	default:
		return uint16(m.en(x-1, y))<<0 | uint16(m.en(x-2, y))<<1 | uint16(m.en(x-3, y))<<2 |
			uint16(m.en(x-4, y))<<3 | uint16(a(0))<<4 | uint16(m.en(x+1, y-1))<<5 |
			uint16(m.en(x, y-1))<<6 | uint16(m.en(x-1, y-1))<<7 | uint16(m.en(x-2, y-1))<<8 |
			uint16(m.en(x-3, y-1))<<9
	}
}

// --- Segmentos ---------------------------------------------------------------

// descomprimirJBIG2 decodifica un flujo JBIG2 embebido en PDF a un bitmap de un
// bit por píxel, con el sentido de una imagen en escala de grises.
//
// El formato EMBEBIDO no trae cabecera de fichero: es una secuencia de segmentos
// pelada, y el tamaño de la página lo dice el diccionario del PDF (/Width,
// /Height). Es la diferencia con un `.jb2` suelto, y confundirlas hace que el
// primer segmento se lea desplazado ocho bytes.
func descomprimirJBIG2(datos, globales []byte, ancho, alto int) ([]byte, error) {
	if ancho <= 0 || alto <= 0 || ancho*alto > maxPixelesJBIG2 {
		return nil, fmt.Errorf("jbig2: tamaño inválido %dx%d", ancho, alto)
	}
	pagina := nuevoMapaBits(ancho, alto)
	pintadoAlgo := false
	// Los símbolos exportados de cada diccionario, por número de segmento: una
	// región de texto los pide por referencia, y suelen venir en los globales.
	est := &estadoJBIG2{
		pagina: pagina, simbolos: map[uint32][]*mapaBits{}, regiones: map[uint32]*regionJBIG2{}, patrones: map[uint32][]*mapaBits{},
		tablas: map[uint32]*tablaHuffman{}, contextos: map[uint32]*contextosRetenidos{},
	}

	// Los globales se recorren igual: pueden traer una región que sí se entienda.
	for _, flujo := range [][]byte{globales, datos} {
		if len(flujo) == 0 {
			continue
		}
		recorrerSegmentosJBIG2(sinCabeceraDeFichero(flujo), est, &pintadoAlgo)
	}
	if !pintadoAlgo {
		return nil, ErrJBIG2NoSoportado
	}
	return pagina.empaquetar(), nil
}

// sinCabeceraDeFichero quita la cabecera de un .jb2 suelto (D.4) si alguien la
// metió en el PDF, donde no va: identificador, banderas y, si se sabe, el número
// de páginas. Solo la organización secuencial, que es la que se puede recorrer
// segmento a segmento.
func sinCabeceraDeFichero(datos []byte) []byte {
	const id = "\x97JB2\r\n\x1a\n"
	if len(datos) < 9 || string(datos[:8]) != id || datos[8]&1 == 0 {
		return datos
	}
	if datos[8]&2 != 0 {
		return datos[9:]
	}
	if len(datos) < 13 {
		return nil
	}
	return datos[13:]
}

// recorrerSegmentosJBIG2 lee las cabeceras de segmento y decodifica las regiones
// genéricas inmediatas, que son las que llevan la imagen de la página.
// No devuelve error a propósito: un segmento que no se entiende se SALTA, y que
// no se haya pintado nada lo dice `pintado`. Distinguir «no lo sé decodificar» de
// «está roto» es justo lo que permite caer al recuadro gris en vez de rechazar el
// documento entero.
// estadoJBIG2 es lo que los segmentos se pasan entre sí.
type estadoJBIG2 struct {
	pagina *mapaBits
	// simbolos son los exportados de cada diccionario y regiones las regiones
	// intermedias (36, 40), que no van a la página: las refina otro segmento.
	simbolos map[uint32][]*mapaBits
	regiones map[uint32]*regionJBIG2
	// patrones son los de cada diccionario de patrones (16), para los semitonos.
	patrones map[uint32][]*mapaBits
	// tablas son las de Huffman de los segmentos 53, y contextos las
	// estadísticas que un diccionario retiene para el siguiente.
	tablas    map[uint32]*tablaHuffman
	contextos map[uint32]*contextosRetenidos
}

func recorrerSegmentosJBIG2(datos []byte, est *estadoJBIG2, pintado *bool) {
	p := 0
	for p+11 <= len(datos) {
		cab, siguiente, ok := leerCabeceraSegmento(datos, p)
		if !ok {
			break
		}
		cuerpo := datos[siguiente:]
		if cab.longitud == 0xFFFFFFFF {
			// Longitud desconocida (7.2.7): solo es legal en una región genérica
			// inmediata, que acaba en un marcador y el número de filas que trae de
			// verdad. Sin marcador no se adivina dónde acaba.
			n, ok := longitudGenericaDesconocida(cuerpo, cab.tipo)
			if !ok {
				return
			}
			cab.longitud = uint32(n) //nolint:gosec // G115: n ≤ len(datos)
			// La altura de verdad (las 4 últimas) sustituye a la de la cabecera, en
			// una copia: el flujo puede ser el del propio documento.
			cuerpo = append([]byte(nil), cuerpo[:n]...)
			copy(cuerpo[4:8], cuerpo[n-4:n])
		}
		if int(cab.longitud) <= len(cuerpo) {
			cuerpo = cuerpo[:cab.longitud]
		}

		if est.aplicarSegmento(cab, cuerpo) {
			*pintado = true
		}
		p = siguiente + int(cab.longitud)
	}
}

// aplicarSegmento decodifica un segmento y dice si pintó algo en la página.
//
// 48 es la información de página; 0, el diccionario de símbolos; 16, el de
// patrones; 53, una tabla de Huffman; y las regiones: 4/6/7 de texto, 20/22/23 de semitonos, 36/38/39
// genéricas y 40/42/43 de refinamiento. Lo demás se salta: la página queda
// incompleta, no rota.
func (est *estadoJBIG2) aplicarSegmento(cab cabeceraSegmento, cuerpo []byte) bool {
	var (
		r   *regionJBIG2
		err error
	)
	switch cab.tipo {
	case 48:
		// Píxel por defecto (bit 2 de las banderas, 7.4.8.5): una página que
		// arranca en negro y se compone con XOR no se parece en nada a la misma
		// sobre blanco.
		if len(cuerpo) > 16 && cuerpo[16]&4 != 0 {
			for i := range est.pagina.px {
				est.pagina.px[i] = 1
			}
		}
		return false
	case 0:
		est.diccionarioSimbolos(cab, cuerpo)
		return false
	case 53:
		if t, err := leerTablaHuffman(cuerpo); err == nil {
			est.tablas[cab.numero] = t
		}
		return false
	case 16:
		if p, err := decodificarDiccionarioPatrones(cuerpo); err == nil {
			est.patrones[cab.numero] = p
		}
		return false
	case 4, 6, 7:
		r, err = decodificarRegionTexto(cuerpo, simbolosReferidos(cab, est.simbolos), est.tablasReferidas(cab))
	case 20, 22, 23:
		r, err = regionSemitonos(cuerpo, est.patronesReferidos(cab))
	case 36, 38, 39:
		r, err = decodificarRegionGenericaSegmento(cuerpo)
	case 40, 42, 43:
		r, err = est.regionDeRefinamiento(cab, cuerpo)
	default:
		return false
	}
	if err != nil {
		return false
	}
	// Las regiones intermedias (4, 20, 36, 40) no van a la página: las refina
	// otro segmento que las nombra.
	if cab.tipo%4 == 0 {
		est.regiones[cab.numero] = r
		return false
	}
	componerJBIG2(est.pagina, r.mapa, r.x0, r.y0, r.op)
	return true
}

// diccionarioSimbolos decodifica el segmento 0 con lo que nombra: símbolos,
// tablas de Huffman y, si las pide, las estadísticas del ÚLTIMO diccionario
// referido (7.4.2.2).
func (est *estadoJBIG2) diccionarioSimbolos(cab cabeceraSegmento, cuerpo []byte) {
	var previo *contextosRetenidos
	for _, n := range cab.referidas {
		if _, ok := est.simbolos[n]; ok {
			previo = est.contextos[n]
		}
	}
	exp, ret, err := decodificarDiccionarioSimbolos(cuerpo, simbolosReferidos(cab, est.simbolos), est.tablasReferidas(cab), previo)
	if err != nil {
		return
	}
	est.simbolos[cab.numero] = exp
	if ret != nil {
		est.contextos[cab.numero] = ret
	}
}

// tablasReferidas son las tablas de Huffman propias que el segmento nombra, en
// ese orden: se reparten entre los campos que piden una (7.4.2.1.6).
func (est *estadoJBIG2) tablasReferidas(cab cabeceraSegmento) []*tablaHuffman {
	var out []*tablaHuffman
	for _, r := range cab.referidas {
		if t, ok := est.tablas[r]; ok {
			out = append(out, t)
		}
	}
	return out
}

// patronesReferidos son los del primer diccionario de patrones que el segmento
// nombra (7.4.5.2).
func (est *estadoJBIG2) patronesReferidos(cab cabeceraSegmento) []*mapaBits {
	for _, n := range cab.referidas {
		if p, ok := est.patrones[n]; ok {
			return p
		}
	}
	return nil
}

// cabeceraSegmento es lo que hace falta de la cabecera (7.2).
type cabeceraSegmento struct {
	numero   uint32
	tipo     int
	longitud uint32
	// referidas son los segmentos de los que depende (una región de texto, sus
	// diccionarios de símbolos), en el orden en que la cabecera los nombra.
	referidas []uint32
}

// leerCabeceraSegmento devuelve la cabecera y dónde empieza su cuerpo.
//
// Es la parte tediosa del formato: el número de referencias y su anchura son
// variables, y el número de página ocupa uno o cuatro bytes según una bandera. Un
// byte mal contado aquí desplaza TODO lo que sigue.
func leerCabeceraSegmento(datos []byte, p int) (cabeceraSegmento, int, bool) {
	var c cabeceraSegmento
	if p+11 > len(datos) {
		return c, 0, false
	}
	q := p + 4 // número de segmento
	banderas := datos[q]
	q++
	c.tipo = int(banderas & 0x3F)
	paginaGrande := banderas&0x40 != 0

	if q >= len(datos) {
		return c, 0, false
	}
	rt := datos[q]
	cuantas := int(rt >> 5)
	if cuantas == 7 {
		// Formato largo: 29 bits con el número de referencias y después un mapa de
		// bits de retención.
		if q+4 > len(datos) {
			return c, 0, false
		}
		cuantas = int(uint32(datos[q]&0x1F)<<24 | uint32(datos[q+1])<<16 |
			uint32(datos[q+2])<<8 | uint32(datos[q+3]))
		q += 4
		q += (cuantas + 8) / 8
	} else {
		q++
	}
	if cuantas < 0 || cuantas > 1<<20 {
		return c, 0, false
	}

	// El tamaño de cada referencia depende del número de ESTE segmento.
	numero := uint32(datos[p])<<24 | uint32(datos[p+1])<<16 | uint32(datos[p+2])<<8 | uint32(datos[p+3])
	c.numero = numero
	tamRef := 1
	switch {
	case numero > 65536:
		tamRef = 4
	case numero > 256:
		tamRef = 2
	}
	if q+cuantas*tamRef > len(datos) {
		return c, 0, false
	}
	for i := 0; i < cuantas && i < 64; i++ {
		var r uint32
		for k := 0; k < tamRef; k++ {
			r = r<<8 | uint32(datos[q+i*tamRef+k])
		}
		c.referidas = append(c.referidas, r)
	}
	q += cuantas * tamRef

	if paginaGrande {
		q += 4
	} else {
		q++
	}
	if q+4 > len(datos) {
		return c, 0, false
	}
	c.longitud = uint32(datos[q])<<24 | uint32(datos[q+1])<<16 | uint32(datos[q+2])<<8 | uint32(datos[q+3])
	q += 4
	return c, q, true
}

// decodificarRegionGenericaSegmento decodifica un segmento de región genérica
// (la página la compone quien llama; las intermedias se guardan).
func decodificarRegionGenericaSegmento(cuerpo []byte) (*regionJBIG2, error) {
	// Cabecera de información de región (7.4.1): 17 bytes.
	if len(cuerpo) < 18 {
		return nil, ErrJBIG2NoSoportado
	}
	be := func(i int) int {
		return int(uint32(cuerpo[i])<<24 | uint32(cuerpo[i+1])<<16 |
			uint32(cuerpo[i+2])<<8 | uint32(cuerpo[i+3]))
	}
	ancho, alto := be(0), be(4)
	x0, y0 := be(8), be(12)
	q := 17

	banderas := cuerpo[q]
	q++
	mmr := banderas&1 != 0
	plantilla := int(banderas>>1) & 3
	tpgdon := banderas&8 != 0
	if ancho <= 0 || alto <= 0 || ancho*alto > maxPixelesJBIG2 {
		return nil, ErrJBIG2NoSoportado
	}
	op := int(cuerpo[16] & 7)
	if mmr {
		region, _, err := mapaMMR(cuerpo[q:], ancho, alto)
		if err != nil {
			return nil, err
		}
		return &regionJBIG2{mapa: region, x0: x0, y0: y0, op: op}, nil
	}

	// Los píxeles AT: cuatro pares con la plantilla 0, uno con las demás.
	cuantosAT := 1
	if plantilla == 0 {
		cuantosAT = 4
	}
	if q+2*cuantosAT > len(cuerpo) {
		return nil, ErrJBIG2NoSoportado
	}
	at := leerAT(cuerpo[q:q+2*cuantosAT], plantilla)
	q += 2 * cuantosAT

	mq := nuevoMQ(cuerpo[q:])
	cx := make([]contextoMQ, 1<<16)
	region := decodificarRegionGenerica(mq, cx, ancho, alto, plantilla, at, tpgdon, nil)

	return &regionJBIG2{mapa: region, x0: x0, y0: y0, op: op}, nil
}

// mapaMMR decodifica un mapa MMR, que es CCITT G4 dentro de JBIG2 (6.2.6) con
// el 1 como negro, y dice cuántos bytes consumió.
func mapaMMR(datos []byte, ancho, alto int) (*mapaBits, int, error) {
	bits, n, err := descomprimirCCITTHasta(datos, parametrosCCITT{K: -1, Columnas: ancho, Filas: alto, NegroEs1: true})
	if err != nil {
		return nil, 0, ErrJBIG2NoSoportado
	}
	m := nuevoMapaBits(ancho, alto)
	porFila := (ancho + 7) / 8
	for y := 0; y < alto && (y+1)*porFila <= len(bits); y++ {
		for x := 0; x < ancho; x++ {
			m.px[y*ancho+x] = bits[y*porFila+x/8] >> uint(7-x%8) & 1
		}
	}
	return m, n, nil
}

// simbolosReferidos junta los símbolos de los diccionarios que el segmento
// nombra, en ese orden (6.4.4 y 6.5.4): sus números son los SBSYMS.
func simbolosReferidos(cab cabeceraSegmento, simbolos map[uint32][]*mapaBits) []*mapaBits {
	var out []*mapaBits
	for _, r := range cab.referidas {
		out = append(out, simbolos[r]...)
	}
	return out
}

// componerJBIG2 pone la región sobre la página con su operador (7.4.1.5 y 6.2):
// 0 OR, 1 AND, 2 XOR, 3 XNOR, 4 REPLACE. Hasta 2026-10 todo era OR, y una
// página que se compone con XOR (las pruebas de pdf.js lo hacen con TPGDON)
// salía con un 16 % de píxeles cambiados.
func componerJBIG2(pagina, region *mapaBits, x0, y0, op int) {
	for y := 0; y < region.alto; y++ {
		py := y0 + y
		if py < 0 || py >= pagina.alto {
			continue
		}
		for x := 0; x < region.ancho; x++ {
			px := x0 + x
			if px < 0 || px >= pagina.ancho {
				continue
			}
			i, r := py*pagina.ancho+px, region.px[y*region.ancho+x]
			switch op {
			case 1:
				pagina.px[i] &= r
			case 2:
				pagina.px[i] ^= r
			case 3:
				pagina.px[i] = (pagina.px[i] ^ r) ^ 1
			case 4:
				pagina.px[i] = r
			default:
				pagina.px[i] |= r
			}
		}
	}
}

// longitudGenericaDesconocida busca dónde acaba una región genérica inmediata de
// longitud 0xFFFFFFFF (7.2.7): tras los datos va 0xFF 0xAC (codificación
// aritmética) o 0x00 0x00 (MMR), y detrás el número de filas de verdad, que
// sustituye a la altura de la cabecera. Devuelve la longitud total del segmento.
func longitudGenericaDesconocida(cuerpo []byte, tipo int) (int, bool) {
	if (tipo != 38 && tipo != 39) || len(cuerpo) < 18 {
		return 0, false
	}
	mmr := cuerpo[17]&1 != 0
	marca := [2]byte{0xFF, 0xAC}
	if mmr {
		marca = [2]byte{0, 0}
	}
	for i := 18; i+6 <= len(cuerpo); i++ {
		if cuerpo[i] == marca[0] && cuerpo[i+1] == marca[1] {
			return i + 6, true
		}
	}
	return 0, false
}
