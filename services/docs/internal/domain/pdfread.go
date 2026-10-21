// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Lector de PDF clean-room, solo con la biblioteca estándar. Es la otra mitad de
// `export_pdf.go`: aquel ESCRIBE un PDF y este LEE uno ajeno, que es la parte
// cara — un fichero de fuera usa todo lo que la especificación permite y no solo
// lo que uno decidió emitir.
//
// Sirve de base a Sytue PDF (roadmap §5). Aquí está la CAPA ESTRUCTURAL: el
// árbol de objetos, las dos formas de tabla de referencias cruzadas, los flujos
// comprimidos y el árbol de páginas. El render fiel se apoya en esto y vive en
// `pdfrender.go`; el descifrado, en `pdfcrypt.go`; la recuperación de un fichero
// con el índice roto, en `pdfrepair.go`.
//
// Sin dependencias nuevas: la biblioteca estándar cubre todos los filtros de
// propósito general (`compress/zlib` para FlateDecode, y LZW, ASCII85, ASCIIHex y
// RunLength escritos aquí). Nada de PDF.js (AGPL) ni binarios externos, por el
// invariante clean-room de la casa.
//
// LO QUE NO HACE, dicho aquí para que nadie lo descubra tarde: los filtros de
// imagen **JBIG2 y JPEG 2000** —CCITT sí, en `pdfccitt.go`—. Y de las
// actualizaciones incrementales solo se lee la xref más reciente, salvo cuando
// hay que reconstruirla: ahí gana la definición que esté más abajo en el fichero.
//
// Esta lista se queda obsoleta en cuanto alguien entrega una de esas piezas, y un
// comentario que MIENTE es peor que ninguno porque se cree. Ya pasó: durante unas
// horas este bloque siguió diciendo que el cifrado y el CCITT no estaban.
package domain

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"sync"
)

// MaxPDFBytes es el tamaño máximo de un PDF que procesa CertiPDF. Se mantiene
// separado de MaxImportBytes: un PDF grande se abre sin descomprimir todo su
// contenido, mientras que un documento ofimático se convierte entero en
// memoria. Cien MiB cubren manuales y expedientes escaneados habituales sin
// aceptar cargas sin cota.
const MaxPDFBytes = 100 << 20

// Errores del lector, para que quien llame distinga «no es un PDF» de «es un PDF
// que aún no sabemos leer», que llevan a mensajes de UI distintos.
var (
	ErrPDFNoEsPDF     = errors.New("pdf: el fichero no empieza por %PDF-")
	ErrPDFCifrado     = errors.New("pdf: el documento está cifrado (/Encrypt) y no se puede leer")
	ErrPDFCorrupto    = errors.New("pdf: estructura ilegible")
	ErrPDFNoSoportado = errors.New("pdf: función de la especificación aún no soportada")
)

// --- Modelo de objetos -------------------------------------------------------

// Los ocho tipos del modelo de datos de PDF (§7.3). Se representan con tipos de
// Go corrientes para que el código que los consuma no necesite envoltorios.
type (
	pdfNull   struct{}
	pdfName   string   // /Nombre
	pdfString []byte   // (literal) o <hex>; ya decodificado
	pdfArray  []pdfObj // [ ... ]
	pdfDict   map[pdfName]pdfObj
	pdfObj    any // null | bool | int64 | float64 | pdfString | pdfName | pdfArray | pdfDict | pdfRef | *pdfStream
)

// pdfRef es una referencia indirecta «n g R». Se resuelve contra la xref.
type pdfRef struct{ Num, Gen int }

// pdfStream es un diccionario con datos crudos detrás. Los datos se descomprimen
// bajo demanda, no al parsear: un PDF de mil páginas no debe descomprimirlas
// todas para responder «¿cuántas páginas tiene?».
type pdfStream struct {
	Dict pdfDict
	Raw  []byte
	// Num y Gen identifican el objeto que contiene el flujo. Hacen falta para
	// descifrarlo: en PDF cada objeto se cifra con una clave derivada de SU
	// número, así que un flujo desligado de su número no se puede abrir.
	Num, Gen int
	// sinCifrar marca los flujos que nunca van cifrados aunque el documento lo
	// esté: la propia tabla de referencias cruzadas es el caso.
	sinCifrar bool
}

// --- Documento ---------------------------------------------------------------

// PDFDoc es un PDF abierto: el índice de objetos ya resuelto y el árbol de
// páginas aplanado.
type PDFDoc struct {
	buf     []byte
	objs    map[int]pdfObj // número de objeto → objeto (perezoso: se llena al resolver)
	xref    map[int]xrefEntrada
	trailer pdfDict
	paginas []PDFPagina
	cripto  *criptoPDF // nil si el documento no está cifrado; ver pdfcrypt.go
	// La huella de los recursos SVG se calcula una vez sobre el fichero original,
	// inmutable: repetirla por página volvería a recorrer hasta 100 MiB cada vez.
	huellaSVG     string
	huellaSVGOnce sync.Once
	// reparado marca que la tabla de referencias estaba rota y hubo que
	// reconstruirla barriendo el fichero. Ver pdfrepair.go.
	reparado bool
	// invalidarFirmas autoriza a reescribir aunque haya firmas. Ver pdfanexo.go.
	invalidarFirmas bool
}

// xrefEntrada localiza un objeto: o está suelto en un offset del fichero, o vive
// dentro de un «object stream» (PDF 1.5+), que es un flujo con varios objetos
// pequeños comprimidos juntos.
type xrefEntrada struct {
	enFlujo  bool
	offset   int64 // si enFlujo == false
	flujoNum int   // si enFlujo == true: objeto del ObjStm que lo contiene
	flujoIdx int   // índice dentro de ese ObjStm
}

// PDFPagina es una página con lo mínimo para pintarla o medirla.
type PDFPagina struct {
	// Ancho y Alto en puntos (1/72 de pulgada), ya normalizados desde MediaBox.
	Ancho, Alto float64
	// Rotacion en grados (0, 90, 180, 270), tal cual la declara /Rotate.
	Rotacion int
	// Caja es el MediaBox ordenado [x0 y0 x1 y1]. El ORIGEN importa: una página
	// con caja [10 10 622 802] mide lo mismo que una [0 0 612 792] pero su
	// contenido está desplazado, y sin restar la esquina el render sale corrido.
	Caja [4]float64
	dict pdfDict
	// num es el objeto que ES esta página en el fichero de origen (0 si no se
	// supo). Lo necesita la actualización incremental para reescribir SOLO ese
	// objeto —añadirle el widget de la firma— sin tocar el resto del fichero.
	num int
}

// NumPaginas devuelve cuántas páginas tiene el documento.
func (d *PDFDoc) NumPaginas() int { return len(d.paginas) }

// Pagina devuelve la página i (0-based).
func (d *PDFDoc) Pagina(i int) (PDFPagina, error) {
	if i < 0 || i >= len(d.paginas) {
		return PDFPagina{}, fmt.Errorf("pdf: página %d fuera de rango (hay %d)", i, len(d.paginas))
	}
	return d.paginas[i], nil
}

// --- Apertura ----------------------------------------------------------------

// LeerPDF abre un PDF desde memoria: valida la cabecera, localiza la tabla de
// referencias cruzadas y aplana el árbol de páginas.
//
// Un documento cifrado se abre con la contraseña VACÍA, que es lo que llevan los
// PDF que solo declaran permisos —la mayoría de los «protegidos» que circulan—.
// Si de verdad hace falta una contraseña, devuelve ErrPDFClaveIncorrecta para
// que la interfaz la pida; ver LeerPDFConClave.
func LeerPDF(datos []byte) (*PDFDoc, error) { return LeerPDFConClave(datos, "") }

// LeerPDFConClave abre un PDF protegido. Sirve tanto la contraseña de usuario
// como la de dueño; con esta última no aplican los permisos del documento.
func LeerPDFConClave(datos []byte, contrasena string) (*PDFDoc, error) {
	sinCabecera := false
	if len(datos) < 8 || !bytes.HasPrefix(datos, []byte("%PDF-")) {
		// Algunos ficheros traen basura delante de la cabecera; la especificación
		// pide que el lector la tolere si aparece dentro del primer kilobyte.
		i := bytes.Index(datos, []byte("%PDF-"))
		switch {
		case i >= 0 && i <= 1024:
			datos = datos[i:]
		case len(datos) >= 8 && bytes.Contains(datos, []byte(" obj")) && bytes.Contains(datos, []byte("startxref")):
			// Y otros no la traen: el resto está y se lee, pero si no sale un
			// documento, lo que se dice es que no es un PDF.
			sinCabecera = true
		default:
			return nil, ErrPDFNoEsPDF
		}
	}
	d := &PDFDoc{buf: datos, objs: map[int]pdfObj{}, xref: map[int]xrefEntrada{}}
	if err := d.cargarXref(); err != nil {
		// El índice está roto. Los objetos siguen en el fichero con su cabecera
		// delante, así que se barre y se reconstruye en vez de rendirse: es la
		// diferencia entre «este PDF no se abre» y abrirlo. Ver pdfrepair.go.
		if !d.reconstruirXref() {
			if sinCabecera {
				return nil, ErrPDFNoEsPDF
			}
			return nil, err
		}
	}
	if err := d.prepararCripto(contrasena); err != nil {
		return nil, err
	}
	// Lo resuelto ANTES de tener la clave se leyó sin descifrar; se olvida para
	// que se vuelva a leer ya en claro. Sin esto, un /Root cacheado en crudo deja
	// el catálogo ilegible y el documento «no tiene páginas».
	if d.cripto != nil {
		d.objs = map[int]pdfObj{}
	}
	if err := d.cargarPaginas(); err != nil {
		// El índice se leyó, pero el árbol de páginas no sale. Pasa cuando los
		// desplazamientos apuntan a sitios plausibles pero equivocados —una
		// descarga cortada y recompuesta—, y ahí el barrido sí acierta porque no
		// se fía de ningún número del fichero.
		if d.reparado || !d.reconstruirXref() {
			return nil, err
		}
		if err := d.cargarPaginas(); err != nil {
			if sinCabecera {
				return nil, ErrPDFNoEsPDF
			}
			return nil, err
		}
	}
	return d, nil
}

// cargarXref localiza `startxref` al final del fichero y sigue la cadena de
// tablas (/Prev) hasta agotarla. La PRIMERA definición de cada objeto gana: se
// recorre de la más reciente a la más antigua, que es como se resuelven las
// actualizaciones incrementales.
func (d *PDFDoc) cargarXref() error {
	cola := d.buf
	if len(cola) > 2048 {
		cola = cola[len(cola)-2048:]
	}
	i := bytes.LastIndex(cola, []byte("startxref"))
	if i < 0 {
		return fmt.Errorf("%w: falta startxref", ErrPDFCorrupto)
	}
	lx := &pdfLexer{buf: cola, pos: i + len("startxref")}
	off, ok := lx.entero()
	if !ok {
		return fmt.Errorf("%w: startxref sin offset", ErrPDFCorrupto)
	}

	vistos := map[int64]bool{}
	for off > 0 && off < int64(len(d.buf)) {
		if vistos[off] {
			break // /Prev circular: un PDF malformado no debe colgar el proceso
		}
		vistos[off] = true
		siguiente, err := d.cargarSeccionXref(off)
		if err != nil {
			return err
		}
		off = siguiente
	}
	if len(d.xref) == 0 {
		return fmt.Errorf("%w: tabla de referencias vacía", ErrPDFCorrupto)
	}
	return nil
}

// cargarSeccionXref lee una sección en `off`, sea tabla clásica («xref») o flujo
// de referencias («/Type /XRef», PDF 1.5+). Devuelve el offset de la anterior
// (/Prev) o 0 si es la última.
func (d *PDFDoc) cargarSeccionXref(off int64) (int64, error) {
	lx := &pdfLexer{buf: d.buf, pos: int(off)}
	lx.saltarEspacios()

	if lx.aceptaPalabra("xref") {
		return d.cargarTablaClasica(lx)
	}
	// Flujo de referencias: «n g obj << ... >> stream».
	obj, err := d.parsearObjetoIndirectoEn(int(off))
	if err != nil {
		return 0, err
	}
	st, ok := obj.(*pdfStream)
	if !ok {
		return 0, fmt.Errorf("%w: en %d no hay ni tabla xref ni flujo xref", ErrPDFCorrupto, off)
	}
	return d.cargarFlujoXref(st)
}

// cargarTablaClasica lee las subsecciones «inicio cantidad» seguidas de entradas
// de 20 bytes, y después el trailer.
func (d *PDFDoc) cargarTablaClasica(lx *pdfLexer) (int64, error) {
	for {
		lx.saltarEspacios()
		if lx.aceptaPalabra("trailer") {
			break
		}
		inicio, ok := lx.entero()
		if !ok {
			break
		}
		cantidad, ok := lx.entero()
		if !ok {
			return 0, fmt.Errorf("%w: subsección xref sin cantidad", ErrPDFCorrupto)
		}
		for k := int64(0); k < cantidad; k++ {
			lx.saltarEspacios()
			offset, ok1 := lx.entero()
			_, ok2 := lx.entero() // generación: no se usa, solo la más reciente
			lx.saltarEspacios()
			tipo := lx.byteActual()
			lx.pos++
			if !ok1 || !ok2 {
				return 0, fmt.Errorf("%w: entrada xref ilegible", ErrPDFCorrupto)
			}
			num := int(inicio + k)
			// 'n' = en uso, 'f' = libre. La primera definición gana (la más reciente).
			if tipo == 'n' {
				if _, ya := d.xref[num]; !ya {
					d.xref[num] = xrefEntrada{offset: offset}
				}
			}
		}
	}
	tr, err := d.parsearObjeto(lx)
	if err != nil {
		return 0, err
	}
	dict, ok := tr.(pdfDict)
	if !ok {
		return 0, fmt.Errorf("%w: trailer no es un diccionario", ErrPDFCorrupto)
	}
	d.fusionarTrailer(dict)

	// Un fichero híbrido apunta con /XRefStm a un flujo con los objetos que la
	// tabla clásica no lista; sin leerlo faltarían objetos en PDF 1.5+ generados
	// para ser compatibles con lectores viejos.
	if x, ok := dict["XRefStm"]; ok {
		if pos, ok := comoEntero(x); ok {
			_, _ = d.cargarSeccionXref(pos)
		}
	}
	if p, ok := dict["Prev"]; ok {
		if prev, ok := comoEntero(p); ok {
			return prev, nil
		}
	}
	return 0, nil
}

// cargarFlujoXref decodifica un /Type /XRef: una matriz binaria de campos cuyos
// anchos declara /W, opcionalmente troceada por /Index.
func (d *PDFDoc) cargarFlujoXref(st *pdfStream) (int64, error) {
	datos, err := d.datosDeFlujo(st)
	if err != nil {
		return 0, err
	}
	anchos, err := anchosDeXref(st.Dict)
	if err != nil {
		return 0, err
	}
	ancho := anchos[0] + anchos[1] + anchos[2]
	if ancho == 0 {
		return 0, fmt.Errorf("%w: /W suma cero", ErrPDFCorrupto)
	}
	rangos := rangosDeXref(st.Dict)

	pos := 0
	for r := 0; r+1 < len(rangos); r += 2 {
		inicio, cantidad := rangos[r], rangos[r+1]
		for k := int64(0); k < cantidad && pos+ancho <= len(datos); k++ {
			tipo := campoDeXref(datos, pos, anchos, 0)
			f2 := campoDeXref(datos, pos, anchos, 1)
			f3 := campoDeXref(datos, pos, anchos, 2)
			pos += ancho

			num := int(inicio + k)
			if _, ya := d.xref[num]; ya {
				continue // la definición más reciente ya ganó
			}
			switch tipo {
			case 1: // suelto en el fichero
				d.xref[num] = xrefEntrada{offset: f2}
			case 2: // dentro de un object stream
				d.xref[num] = xrefEntrada{enFlujo: true, flujoNum: int(f2), flujoIdx: int(f3)}
			}
			// tipo 0 = libre: se ignora.
		}
	}
	d.fusionarTrailer(st.Dict)
	if p, ok := st.Dict["Prev"]; ok {
		if prev, ok := comoEntero(p); ok {
			return prev, nil
		}
	}
	return 0, nil
}

// anchosDeXref lee /W: cuántos bytes ocupa cada uno de los tres campos de una
// fila del flujo xref (tipo, y dos valores cuyo significado depende del tipo).
func anchosDeXref(dict pdfDict) ([3]int, error) {
	arr, _ := dict["W"].(pdfArray)
	if len(arr) < 3 {
		return [3]int{}, fmt.Errorf("%w: flujo xref sin /W", ErrPDFCorrupto)
	}
	var anchos [3]int
	for i := range anchos {
		n, _ := comoEntero(arr[i])
		anchos[i] = int(n)
	}
	return anchos, nil
}

// rangosDeXref lee /Index —pares «primer objeto, cuántos»— que dice qué rangos de
// números de objeto describe el flujo. Sin él, el rango es [0, /Size).
func rangosDeXref(dict pdfDict) []int64 {
	idx, ok := dict["Index"].(pdfArray)
	if !ok {
		tam, _ := comoEntero(dict["Size"])
		return []int64{0, tam}
	}
	rangos := make([]int64, 0, len(idx))
	for _, v := range idx {
		n, _ := comoEntero(v)
		rangos = append(rangos, n)
	}
	return rangos
}

// campoDeXref lee el campo `i` (0, 1 o 2) de la fila que empieza en `pos`. Un
// ancho declarado de 0 significa «valor por defecto» (§7.5.8.2): 1 para el tipo,
// que es «objeto en uso», y 0 para los otros dos.
func campoDeXref(datos []byte, pos int, anchos [3]int, i int) int64 {
	if anchos[i] == 0 {
		if i == 0 {
			return 1
		}
		return 0
	}
	desplazamiento := 0
	for j := range i {
		desplazamiento += anchos[j]
	}
	var v int64
	for b := range anchos[i] {
		v = v<<8 | int64(datos[pos+desplazamiento+b])
	}
	return v
}

// fusionarTrailer conserva la primera aparición de cada clave, que viene de la
// sección más reciente.
func (d *PDFDoc) fusionarTrailer(dict pdfDict) {
	if d.trailer == nil {
		d.trailer = pdfDict{}
	}
	for k, v := range dict {
		if _, ya := d.trailer[k]; !ya {
			d.trailer[k] = v
		}
	}
}

// --- Resolución de objetos ---------------------------------------------------

// resolver sigue las referencias indirectas hasta dar con un objeto directo.
func (d *PDFDoc) resolver(o pdfObj) pdfObj {
	for i := 0; i < 32; i++ { // tope: una cadena de referencias circular no cuelga
		ref, ok := o.(pdfRef)
		if !ok {
			return o
		}
		v, err := d.objeto(ref.Num)
		if err != nil {
			return pdfNull{}
		}
		o = v
	}
	return pdfNull{}
}

// objeto devuelve el objeto `num`, parseándolo la primera vez que se pide.
func (d *PDFDoc) objeto(num int) (pdfObj, error) {
	if o, ok := d.objs[num]; ok {
		return o, nil
	}
	e, ok := d.xref[num]
	if !ok {
		return pdfNull{}, nil // referencia a objeto inexistente = null (§7.3.9)
	}
	// Marca de ciclo: un objeto que se resuelve a sí mismo no debe recursar.
	d.objs[num] = pdfNull{}

	var o pdfObj
	var err error
	if e.enFlujo {
		// e.flujoIdx no se pasa a propósito: la búsqueda dentro del ObjStm va por
		// número de objeto, no por índice. Ver objetoDeFlujo.
		o, err = d.objetoDeFlujo(e.flujoNum, num)
	} else {
		o, err = d.parsearObjetoIndirectoEn(int(e.offset))
	}
	if err != nil {
		return pdfNull{}, err
	}
	d.objs[num] = o
	return o, nil
}

// objetoDeFlujo extrae el objeto `num` de un ObjStm. La cabecera del flujo son
// pares «número offset» y /First dice dónde empiezan los datos.
//
// No recibe el índice que anota la xref (/flujoIdx) porque no se usa: se busca
// por número, ver el comentario del bucle.
func (d *PDFDoc) objetoDeFlujo(flujoNum, num int) (pdfObj, error) {
	cont, err := d.objeto(flujoNum)
	if err != nil {
		return nil, err
	}
	st, ok := cont.(*pdfStream)
	if !ok {
		return nil, fmt.Errorf("%w: el objeto %d no es un ObjStm", ErrPDFCorrupto, flujoNum)
	}
	datos, err := d.datosDeFlujo(st)
	if err != nil {
		return nil, err
	}
	n, _ := comoEntero(d.resolver(st.Dict["N"]))
	primero, _ := comoEntero(d.resolver(st.Dict["First"]))

	lx := &pdfLexer{buf: datos}
	desplazamiento := int64(-1)
	for i := int64(0); i < n; i++ {
		objNum, ok1 := lx.entero()
		off, ok2 := lx.entero()
		if !ok1 || !ok2 {
			break
		}
		// Se busca por número y no por índice: /Index y el orden del flujo pueden
		// no coincidir, y fiarse del índice devolvería OTRO objeto sin error.
		if int(objNum) == num {
			desplazamiento = off
			break
		}
	}
	if desplazamiento < 0 {
		return nil, fmt.Errorf("%w: objeto %d no está en el ObjStm %d", ErrPDFCorrupto, num, flujoNum)
	}
	inicio := int(primero + desplazamiento)
	if inicio < 0 || inicio > len(datos) {
		return nil, fmt.Errorf("%w: offset fuera del ObjStm", ErrPDFCorrupto)
	}
	lx2 := &pdfLexer{buf: datos, pos: inicio}
	return d.parsearObjeto(lx2)
}

// parsearObjetoIndirectoEn lee «n g obj … endobj» en un offset del fichero.
func (d *PDFDoc) parsearObjetoIndirectoEn(off int) (pdfObj, error) {
	if off < 0 || off >= len(d.buf) {
		return nil, fmt.Errorf("%w: offset %d fuera del fichero", ErrPDFCorrupto, off)
	}
	lx := &pdfLexer{buf: d.buf, pos: off}
	num, ok := lx.entero() // número de objeto
	if !ok {
		return nil, fmt.Errorf("%w: se esperaba un número de objeto en %d", ErrPDFCorrupto, off)
	}
	gen, ok := lx.entero() // generación
	if !ok {
		return nil, fmt.Errorf("%w: se esperaba la generación en %d", ErrPDFCorrupto, off)
	}
	lx.saltarEspacios()
	if !lx.aceptaPalabra("obj") {
		return nil, fmt.Errorf("%w: falta 'obj' en %d", ErrPDFCorrupto, off)
	}
	o, err := d.parsearObjeto(lx)
	if err != nil {
		return nil, err
	}
	// ¿Va seguido de un stream?
	lx.saltarEspacios()
	if lx.aceptaPalabra("stream") {
		dict, ok := o.(pdfDict)
		if !ok {
			return nil, fmt.Errorf("%w: 'stream' sin diccionario delante", ErrPDFCorrupto)
		}
		// Tras «stream» va CRLF o LF, nunca CR solo (§7.3.8.1).
		if lx.byteActual() == '\r' {
			lx.pos++
		}
		if lx.byteActual() == '\n' {
			lx.pos++
		}
		inicio := lx.pos
		long, tieneLong := comoEntero(d.resolver(dict["Length"]))
		fin := inicio + int(long)
		// /Length puede mentir o ser una referencia rota; en ese caso se busca
		// «endstream», que es lo que hacen los lectores reales.
		if !tieneLong || fin > len(d.buf) || fin < inicio || !d.pareceEndstream(fin) {
			rel := bytes.Index(d.buf[inicio:], []byte("endstream"))
			if rel < 0 {
				return nil, fmt.Errorf("%w: stream sin endstream", ErrPDFCorrupto)
			}
			fin = inicio + rel
			// Quitar el salto de línea que precede a «endstream».
			for fin > inicio && (d.buf[fin-1] == '\n' || d.buf[fin-1] == '\r') {
				fin--
			}
		}
		tipo, _ := d.resolver(dict["Type"]).(pdfName)
		st := &pdfStream{
			Dict: dict, Raw: d.buf[inicio:fin], Num: int(num), Gen: int(gen),
			// La tabla de referencias cruzadas NO va cifrada aunque el documento lo
			// esté: hay que poder leerla para llegar al propio /Encrypt.
			sinCifrar: tipo == "XRef",
		}
		return d.descifrarObjeto(st, int(num), int(gen), 0), nil
	}
	return d.descifrarObjeto(o, int(num), int(gen), 0), nil
}

// pareceEndstream comprueba que en `pos` (saltando espacios) viene «endstream»,
// que es como se valida que el /Length declarado era de fiar.
func (d *PDFDoc) pareceEndstream(pos int) bool {
	for pos < len(d.buf) && esEspacioPDF(d.buf[pos]) {
		pos++
	}
	return bytes.HasPrefix(d.buf[pos:], []byte("endstream"))
}

// datosDeFlujo devuelve el contenido descomprimido de un stream.
func (d *PDFDoc) datosDeFlujo(st *pdfStream) ([]byte, error) {
	datos, terminal, err := d.datosDeFlujoHasta(st)
	if err != nil {
		return nil, err
	}
	if terminal != "" {
		return nil, fmt.Errorf("%w: filtro %s", ErrPDFNoSoportado, terminal)
	}
	return datos, nil
}

// filtrosDeImagen son los que NO se decodifican aquí: producen píxeles, no
// bytes de objeto. El render los pasa tal cual al navegador (DCT es un JPEG que
// entiende de serie) o los decodifica él mismo, así que este lector se para
// antes y dice cuál quedó pendiente en vez de fallar.
func esFiltroDeImagen(f pdfName) bool {
	switch f {
	case "DCTDecode", "DCT", "JPXDecode", "CCITTFaxDecode", "CCF", "JBIG2Decode":
		return true
	}
	return false
}

// datosDeFlujoHasta descomprime aplicando los filtros de propósito general y se
// detiene al llegar a uno de imagen, que devuelve como `terminal`.
func (d *PDFDoc) datosDeFlujoHasta(st *pdfStream) (datos []byte, terminal pdfName, err error) {
	filtros := d.listaDeNombres(st.Dict["Filter"])
	datos = st.Raw
	// El descifrado va ANTES que los filtros: lo que se comprimió es el texto en
	// claro, así que inflar primero solo produce basura.
	if d.cripto != nil && !st.sinCifrar {
		if datos, err = d.cripto.descifrarFlujo(d, st); err != nil {
			return nil, "", err
		}
	}
	for i, f := range filtros {
		if esFiltroDeImagen(f) {
			return datos, f, nil
		}
		switch f {
		case "FlateDecode", "Fl":
			datos, err = inflar(datos)
			if err != nil {
				return nil, "", err
			}
			predictor, columnas, colores, bpc := d.parametrosPredictor(st, i)
			datos = aplicarPredictor(datos, predictor, columnas, colores, bpc)
		case "LZWDecode", "LZW":
			predictor, columnas, colores, bpc := d.parametrosPredictor(st, i)
			// /EarlyChange 0 desplaza el momento de subir de código en uno; es
			// raro pero existe, y equivocarse desalinea TODO el resto del flujo.
			temprano := 1
			if parms := d.parmsDeFiltro(st, i); parms != nil {
				if v, ok := comoEntero(d.resolver(parms["EarlyChange"])); ok {
					temprano = int(v)
				}
			}
			datos = descomprimirLZW(datos, temprano != 0)
			datos = aplicarPredictor(datos, predictor, columnas, colores, bpc)
		case "ASCIIHexDecode", "AHx":
			datos = decodificarHexASCII(datos)
		case "ASCII85Decode", "A85":
			datos = decodificarASCII85(datos)
		case "RunLengthDecode", "RL":
			datos = decodificarRunLength(datos)
		case "Crypt", "":
			// /Crypt con /Identity es un no-op; el descifrado real ya ocurrió en
			// `pdfcrypt.go` antes de llegar aquí.
		default:
			return nil, "", fmt.Errorf("%w: filtro %s", ErrPDFNoSoportado, f)
		}
	}
	return datos, "", nil
}

// listaDeNombres normaliza /Filter, que puede ser un nombre o un array de ellos.
func (d *PDFDoc) listaDeNombres(o pdfObj) []pdfName {
	switch v := d.resolver(o).(type) {
	case pdfName:
		return []pdfName{v}
	case pdfArray:
		var out []pdfName
		for _, e := range v {
			if n, ok := d.resolver(e).(pdfName); ok {
				out = append(out, n)
			}
		}
		return out
	}
	return nil
}

// parmsDeFiltro devuelve el /DecodeParms que corresponde al filtro número `i`.
// Cuando hay varios filtros, /DecodeParms es un array PARALELO a /Filter: coger
// siempre el primero desalinea los parámetros en cuanto el flujo lleva dos.
func (d *PDFDoc) parmsDeFiltro(st *pdfStream, i int) pdfDict {
	switch v := d.resolver(st.Dict["DecodeParms"]).(type) {
	case pdfDict:
		if i == 0 {
			return v
		}
	case pdfArray:
		if i < len(v) {
			p, _ := d.resolver(v[i]).(pdfDict)
			return p
		}
	}
	return nil
}

// parametrosPredictor extrae /DecodeParms para el predictor PNG, que usan casi
// todos los flujos xref comprimidos.
func (d *PDFDoc) parametrosPredictor(st *pdfStream, i int) (predictor, columnas, colores, bpc int) {
	predictor, columnas, colores, bpc = 1, 1, 1, 8
	parms := d.parmsDeFiltro(st, i)
	if parms == nil {
		return predictor, columnas, colores, bpc
	}
	if v, ok := comoEntero(d.resolver(parms["Predictor"])); ok {
		predictor = int(v)
	}
	if v, ok := comoEntero(d.resolver(parms["Columns"])); ok {
		columnas = int(v)
	}
	if v, ok := comoEntero(d.resolver(parms["Colors"])); ok {
		colores = int(v)
	}
	if v, ok := comoEntero(d.resolver(parms["BitsPerComponent"])); ok {
		bpc = int(v)
	}
	return predictor, columnas, colores, bpc
}

const (
	// maxAnidadoObjeto acota los arrays y diccionarios abiertos a la vez. El
	// análisis es recursivo y un fichero de pocos megabytes de «[[[[…» agota la
	// pila de Go, que no es un error recuperable: es `fatal error` y se lleva el
	// proceso entero por delante. Un PDF real no pasa de una docena de niveles.
	maxAnidadoObjeto = 128
	// maxFlujoInflado acota lo que un flujo puede ocupar ya descomprimido. Un
	// «ladrillo» de dos megabytes de Flate se hincha a un gigabyte (medido: 8,6 s
	// y 2,6 GiB asignados), y quien sube el fichero no es de fiar. 128 MiB cubren
	// un escaneo en color a 600 ppp (104 MiB), que es lo más grande que existe.
	maxFlujoInflado = 128 << 20
)

// errAnidadoDemasiado marca un límite de seguridad, no un fichero truncado.
// Se propaga antes de la recuperación de ErrPDFCorrupto: seguir leyendo tras
// este límite reintentaría millones de objetos con la misma profundidad.
var errAnidadoDemasiado = fmt.Errorf("%w: objetos anidados a más de %d niveles",
	ErrPDFCorrupto, maxAnidadoObjeto)

// errFlujoDemasiadoGrande es el aviso de una bomba de descompresión.
var errFlujoDemasiadoGrande = fmt.Errorf("%w: un flujo se descomprime a más de %d MiB",
	ErrPDFNoSoportado, maxFlujoInflado>>20)

// leerAcotado lee todo, pero no más de maxFlujoInflado.
func leerAcotado(r io.Reader) ([]byte, error) {
	// ReadAll crece geométricamente sin acotar la capacidad: para rechazar 128 MiB
	// llegó a asignar 537 MiB. Duplicar hasta el tope consume menos del doble del
	// límite en asignaciones; el byte de exceso se comprueba sin crecer otra vez.
	out := make([]byte, 0, 32<<10)
	for {
		if len(out) == cap(out) {
			if len(out) == maxFlujoInflado {
				var extra [1]byte
				n, err := r.Read(extra[:])
				if n > 0 {
					return nil, errFlujoDemasiadoGrande
				}
				if errors.Is(err, io.EOF) {
					return out, nil
				}
				if err != nil {
					return out, err
				}
				continue
			}
			nuevo := make([]byte, len(out), min(2*cap(out), maxFlujoInflado))
			copy(nuevo, out)
			out = nuevo
		}
		n, err := r.Read(out[len(out):cap(out)])
		out = out[:len(out)+n]
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
	}
}

// inflar descomprime FlateDecode. Se intenta primero con cabecera zlib, que es lo
// normal, y si no cuadra como deflate crudo: hay generadores que omiten la
// cabecera y un lector estricto los rechazaría sin motivo.
func inflar(datos []byte) ([]byte, error) {
	if len(datos) == 0 {
		return nil, nil
	}
	if r, err := zlib.NewReader(bytes.NewReader(datos)); err == nil {
		out, err := leerAcotado(r)
		_ = r.Close()
		if errors.Is(err, errFlujoDemasiadoGrande) {
			return nil, err // no se reintenta: sería gastar lo mismo otra vez
		}
		// Un stream truncado devuelve ErrUnexpectedEOF con datos válidos delante;
		// se aprovecha lo leído en vez de perder la página entera. Igual con la
		// suma Adler-32 equivocada: los datos ya salieron enteros (medido: una
		// /ToUnicode así dejaba sin texto una factura china del corpus de pdf.js).
		if err == nil || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, zlib.ErrChecksum) {
			return out, nil
		}
	}
	out, err := inflarCrudo(datos)
	if err != nil {
		return nil, fmt.Errorf("pdf: FlateDecode: %w", err)
	}
	return out, nil
}

// inflarCrudo descomprime deflate SIN cabecera zlib. Es el plan B de `inflar`.
func inflarCrudo(datos []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(datos))
	defer func() { _ = r.Close() }()
	out, err := leerAcotado(r)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	return out, nil
}

// aplicarPredictor deshace el filtrado PNG por filas (predictor ≥ 10), que es el
// que usan los flujos xref. Sin esto los bytes salen desplazados y la tabla de
// referencias apunta a cualquier parte.
func aplicarPredictor(datos []byte, predictor, columnas, colores, bpc int) []byte {
	if predictor == 2 && len(datos) > 0 {
		return predictorTIFF(datos, columnas, colores, bpc)
	}
	if predictor < 10 || len(datos) == 0 {
		return datos
	}
	muestra := (colores*bpc + 7) / 8 // bytes por píxel, mínimo 1
	if muestra < 1 {
		muestra = 1
	}
	fila := columnas * colores * bpc / 8
	if fila < 1 {
		return datos
	}
	var out []byte
	anterior := make([]byte, fila)
	for pos := 0; pos+1 <= len(datos); pos += fila + 1 {
		tipo := datos[pos]
		fin := pos + 1 + fila
		if fin > len(datos) {
			fin = len(datos)
		}
		actual := make([]byte, fin-pos-1)
		copy(actual, datos[pos+1:fin])
		for i := range actual {
			var izq, arr, arrIzq byte
			if i >= muestra {
				izq = actual[i-muestra]
				arrIzq = anterior[i-muestra]
			}
			if i < len(anterior) {
				arr = anterior[i]
			}
			switch tipo {
			case 1: // Sub
				actual[i] += izq
			case 2: // Up
				actual[i] += arr
			case 3: // Average
				actual[i] += byte((int(izq) + int(arr)) / 2)
			case 4: // Paeth
				actual[i] += paeth(izq, arr, arrIzq)
			}
		}
		out = append(out, actual...)
		anterior = actual
	}
	return out
}

// paeth es el predictor homónimo de PNG (RFC 2083 §6.6).
func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	if pa <= pb && pa <= pc {
		return a
	}
	if pb <= pc {
		return b
	}
	return c
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// decodificarHexASCII deshace ASCIIHexDecode: pares hexadecimales hasta '>'.
func decodificarHexASCII(datos []byte) []byte {
	var out []byte
	var alto byte
	tieneAlto := false
	for _, c := range datos {
		if c == '>' {
			break
		}
		v, ok := valorHex(c)
		if !ok {
			continue
		}
		if tieneAlto {
			out = append(out, alto<<4|v)
			tieneAlto = false
		} else {
			alto, tieneAlto = v, true
		}
	}
	if tieneAlto {
		out = append(out, alto<<4) // dígito impar final: se completa con 0 (§7.4.2)
	}
	return out
}

func valorHex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// decodificarASCII85 deshace ASCII85Decode (§7.4.3): grupos de cinco caracteres
// base-85 que valen cuatro bytes, con 'z' como atajo de cuatro ceros y '~>' de
// terminador. Un grupo final incompleto codifica menos de cuatro bytes.
func decodificarASCII85(datos []byte) []byte {
	out := make([]byte, 0, len(datos)*4/5)
	var grupo [5]byte
	n := 0
	// Algunos generadores emiten el prefijo «<~» que la especificación no pide.
	if i := bytes.Index(datos, []byte("<~")); i >= 0 && i < 8 {
		datos = datos[i+2:]
	}
	emitir := func(cuantos int) {
		v := uint32(0)
		for i := 0; i < 5; i++ {
			c := grupo[i]
			if i >= cuantos {
				c = 84 // relleno con 'u': el máximo, para que el redondeo no reste
			}
			v = v*85 + uint32(c)
		}
		b := [4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
		out = append(out, b[:cuantos-1]...)
	}
	for i := 0; i < len(datos); i++ {
		c := datos[i]
		switch {
		case esEspacioPDF(c):
			continue
		case c == '~':
			if n > 1 {
				emitir(n)
			}
			return out
		case c == 'z' && n == 0:
			out = append(out, 0, 0, 0, 0)
			continue
		case c < '!' || c > 'u':
			continue // basura: se ignora en vez de corromper el resto
		}
		grupo[n] = c - '!'
		n++
		if n == 5 {
			emitir(5)
			n = 0
		}
	}
	if n > 1 {
		emitir(n)
	}
	return out
}

// decodificarRunLength deshace RunLengthDecode (§7.4.5): un byte de longitud
// manda. 0-127 significa «copia los n+1 siguientes»; 129-255 «repite el
// siguiente 257-n veces»; 128 es el fin de datos.
func decodificarRunLength(datos []byte) []byte {
	var out []byte
	for i := 0; i < len(datos); {
		if len(out) > maxFlujoInflado {
			return out[:maxFlujoInflado] // ponytail: se trunca; es una bomba, no un documento
		}
		n := int(datos[i])
		i++
		switch {
		case n == 128:
			return out
		case n < 128:
			fin := i + n + 1
			if fin > len(datos) {
				fin = len(datos)
			}
			out = append(out, datos[i:fin]...)
			i = fin
		default:
			resto := datos[i:]
			if len(resto) == 0 {
				return out
			}
			repetido := resto[0]
			for j := 0; j < 257-n; j++ {
				out = append(out, repetido)
			}
			i++
		}
	}
	return out
}

// descomprimirLZW deshace LZWDecode (§7.4.4). Es el LZW de TIFF: códigos de
// ancho variable de 9 a 12 bits, 256 = reiniciar el diccionario y 257 = fin.
//
// No se usa `compress/lzw` de la stdlib porque su lector MSB reinicia el
// diccionario de otra manera y `EarlyChange`, que aquí es obligatorio y allí no
// existe, adelanta el salto de ancho en un código: con la variante de la stdlib
// la salida se desalinea a mitad de flujo y sale basura sin error.
func descomprimirLZW(datos []byte, temprano bool) []byte {
	const (
		reiniciar = 256
		fin       = 257
		primero   = 258
		maxCodigo = 4096
	)
	tabla := make([][]byte, primero, maxCodigo)
	reset := func() {
		tabla = tabla[:primero]
		for i := 0; i < 256; i++ {
			tabla[i] = []byte{byte(i)}
		}
	}
	reset()

	var out []byte
	ancho := 9
	var anterior []byte
	acumulado, bits := uint32(0), 0
	ajuste := 0
	if temprano {
		ajuste = 1
	}
	for _, b := range datos {
		acumulado = acumulado<<8 | uint32(b)
		bits += 8
		for bits >= ancho {
			codigo := int(acumulado>>uint(bits-ancho)) & ((1 << uint(ancho)) - 1)
			bits -= ancho
			switch codigo {
			case fin:
				return out
			case reiniciar:
				reset()
				ancho, anterior = 9, nil
				continue
			}
			var entrada []byte
			switch {
			case codigo < len(tabla):
				entrada = tabla[codigo]
			case anterior != nil:
				// Caso KwKwK: el código aún no está en la tabla y se deduce.
				entrada = append(append([]byte{}, anterior...), anterior[0])
			default:
				return out // flujo roto: no hay de dónde deducir
			}
			out = append(out, entrada...)
			if len(out) > maxFlujoInflado {
				return out[:maxFlujoInflado] // ponytail: se trunca; es una bomba, no un documento
			}
			if anterior != nil && len(tabla) < maxCodigo {
				tabla = append(tabla, append(append([]byte{}, anterior...), entrada[0]))
			}
			anterior = entrada
			if n := len(tabla) + ajuste; n >= 1<<uint(ancho) && ancho < 12 {
				ancho++
			}
		}
	}
	return out
}

// predictorTIFF deshace el predictor 2 (TIFF, §7.4.4.4): cada muestra se guarda
// como diferencia con la de su izquierda en la misma fila y componente. Sin
// byte de tipo por fila, a diferencia de los de PNG.
func predictorTIFF(datos []byte, columnas, colores, bpc int) []byte {
	fila := (columnas*colores*bpc + 7) / 8
	if fila < 1 || colores < 1 || (bpc != 1 && bpc != 2 && bpc != 4 && bpc != 8 && bpc != 16) {
		return datos
	}
	out := append([]byte(nil), datos...)
	for ini := 0; ini < len(out); ini += fila {
		f := out[ini:min(ini+fila, len(out))]
		switch bpc {
		case 8:
			for i := colores; i < len(f); i++ {
				f[i] += f[i-colores]
			}
		case 16:
			for i := 2 * colores; i+1 < len(f); i += 2 {
				v := uint16(f[i])<<8 | uint16(f[i+1])
				v += uint16(f[i-2*colores])<<8 | uint16(f[i-2*colores+1])
				f[i], f[i+1] = byte(v>>8), byte(v) //nolint:gosec // G115: los dos bytes de una muestra de 16 bits
			}
		default: // 1, 2 o 4 bits: se suma muestra a muestra, módulo 2^bpc
			mascara := byte(1<<bpc - 1)
			muestra := func(k int) byte { return f[k*bpc/8] >> (8 - bpc - k*bpc%8) & mascara }
			for k := colores; k < columnas*colores && (k*bpc)/8 < len(f); k++ {
				v := (muestra(k) + muestra(k-colores)) & mascara
				desp := 8 - bpc - k*bpc%8
				f[k*bpc/8] = f[k*bpc/8]&^(mascara<<desp) | v<<desp
			}
		}
	}
	return out
}

// --- Árbol de páginas --------------------------------------------------------

// cargarPaginas recorre /Root → /Pages y aplana los nodos hoja en orden.
func (d *PDFDoc) cargarPaginas() error {
	raiz, ok := d.resolver(d.trailer["Root"]).(pdfDict)
	if !ok {
		return fmt.Errorf("%w: el trailer no tiene /Root", ErrPDFCorrupto)
	}
	nodo, ok := d.resolver(raiz["Pages"]).(pdfDict)
	if ok {
		numRaiz := 0
		if r, ok := raiz["Pages"].(pdfRef); ok {
			numRaiz = r.Num
		}
		d.recorrerPaginas(nodo, pdfDict{}, 0, numRaiz, map[int]bool{})
	}
	if len(d.paginas) == 0 {
		// El árbol falta o no lleva a ninguna página, pero las páginas pueden
		// estar en el fichero: se buscan por su /Type, como haría quien lo abre a
		// mano, antes de decir que el documento no tiene ninguna.
		d.paginasPorBarrido()
	}
	if len(d.paginas) == 0 {
		if !ok {
			return fmt.Errorf("%w: el catálogo no tiene /Pages", ErrPDFCorrupto)
		}
		return fmt.Errorf("%w: el documento no declara ninguna página", ErrPDFCorrupto)
	}
	return nil
}

// paginasPorBarrido junta, por orden de número de objeto, los diccionarios con
// /Type /Page, con lo que hereden de sus /Parent.
func (d *PDFDoc) paginasPorBarrido() {
	nums := make([]int, 0, len(d.xref))
	for n := range d.xref {
		nums = append(nums, n)
	}
	slices.Sort(nums)
	for _, n := range nums {
		if len(d.paginas) >= maxPaginasArbol {
			return
		}
		nodo, ok := d.resolver(pdfRef{Num: n}).(pdfDict)
		if !ok {
			continue
		}
		if t, _ := d.resolver(nodo["Type"]).(pdfName); t != "Page" {
			continue
		}
		heredado := pdfDict{}
		padre := nodo
		for i := 0; i < maxProfArbolPaginas; i++ {
			padre, ok = d.resolver(padre["Parent"]).(pdfDict)
			if !ok {
				break
			}
			for _, k := range atributosHeredables {
				if _, ya := heredado[k]; !ya && padre[k] != nil {
					heredado[k] = padre[k]
				}
			}
		}
		p := d.construirPagina(nodo, heredado)
		p.num = n
		d.paginas = append(d.paginas, p)
	}
}

const (
	maxPaginasArbol     = 20000
	maxProfArbolPaginas = 1024
)

// atributosHeredables son los que un nodo /Pages pasa a sus hijos (§7.7.3.4). Un
// PDF real declara el MediaBox una vez en la raíz y las páginas no lo repiten.
var atributosHeredables = []pdfName{"MediaBox", "CropBox", "Resources", "Rotate"}

func (d *PDFDoc) recorrerPaginas(nodo, heredado pdfDict, prof, num int, vistos map[int]bool) {
	// Un árbol circular se corta por los nodos ya vistos; uno absurdamente hondo,
	// por la profundidad. 64 se quedaba corto: hay generadores que encadenan un
	// /Pages por página (deep-pages de qpdf, más de cien niveles).
	if prof > maxProfArbolPaginas || len(d.paginas) > maxPaginasArbol {
		return
	}
	efectivo := pdfDict{}
	for k, v := range heredado {
		efectivo[k] = v
	}
	for _, k := range atributosHeredables {
		if v, ok := nodo[k]; ok {
			efectivo[k] = v
		}
	}

	tipo, _ := d.resolver(nodo["Type"]).(pdfName)
	hijos, tieneHijos := d.resolver(nodo["Kids"]).(pdfArray)
	if tipo == "Page" || (!tieneHijos && tipo != "Pages") {
		p := d.construirPagina(nodo, efectivo)
		p.num = num
		d.paginas = append(d.paginas, p)
		return
	}
	// Solo los nodos intermedios: una misma página repetida en /Kids se enseña
	// dos veces, como hacen los lectores.
	if num > 0 {
		if vistos[num] {
			return
		}
		vistos[num] = true
	}
	for _, h := range hijos {
		if hd, ok := d.resolver(h).(pdfDict); ok {
			hijo := 0
			if r, ok := h.(pdfRef); ok {
				hijo = r.Num
			}
			d.recorrerPaginas(hd, efectivo, prof+1, hijo, vistos)
		}
	}
}

// construirPagina calcula el tamaño desde MediaBox, aplicando la rotación: una
// página apaisada por /Rotate 90 mide al revés de lo que dice su caja.
func (d *PDFDoc) construirPagina(nodo, heredado pdfDict) PDFPagina {
	completo := pdfDict{}
	for k, v := range heredado {
		completo[k] = v
	}
	for k, v := range nodo {
		completo[k] = v
	}

	p := PDFPagina{Ancho: 612, Alto: 792, Caja: [4]float64{0, 0, 612, 792}, dict: completo} // Letter, por defecto
	if caja, ok := d.resolver(completo["MediaBox"]).(pdfArray); ok && len(caja) >= 4 {
		var v [4]float64
		for i := 0; i < 4; i++ {
			v[i] = comoReal(d.resolver(caja[i]))
		}
		// Las esquinas pueden venir en cualquier orden; el tamaño es la distancia.
		ancho, alto := v[2]-v[0], v[3]-v[1]
		if ancho < 0 {
			ancho = -ancho
		}
		if alto < 0 {
			alto = -alto
		}
		if ancho > 0 && alto > 0 {
			p.Ancho, p.Alto = ancho, alto
			p.Caja = [4]float64{min(v[0], v[2]), min(v[1], v[3]), max(v[0], v[2]), max(v[1], v[3])}
		}
	}
	if r, ok := comoEntero(d.resolver(completo["Rotate"])); ok {
		p.Rotacion = ((int(r) % 360) + 360) % 360
		if p.Rotacion == 90 || p.Rotacion == 270 {
			p.Ancho, p.Alto = p.Alto, p.Ancho
		}
	}
	return p
}

// contenidoDePagina concatena los flujos de contenido de una página. /Contents
// puede ser un flujo o un array de flujos que se leen como uno solo, y un
// operador puede quedar partido entre dos, así que se separan con un espacio.
func (d *PDFDoc) contenidoDePagina(p PDFPagina) ([]byte, error) {
	var partes [][]byte
	switch v := d.resolver(p.dict["Contents"]).(type) {
	case *pdfStream:
		b, err := d.datosDeFlujo(v)
		if err != nil {
			return nil, err
		}
		partes = append(partes, b)
	case pdfArray:
		for _, e := range v {
			st, ok := d.resolver(e).(*pdfStream)
			if !ok {
				continue
			}
			b, err := d.datosDeFlujo(st)
			if err != nil {
				return nil, err
			}
			partes = append(partes, b)
		}
	}
	return bytes.Join(partes, []byte("\n")), nil
}

// --- Conversiones ------------------------------------------------------------

func comoEntero(o pdfObj) (int64, bool) {
	switch v := o.(type) {
	case int64:
		return v, true
	case float64:
		return int64(v), true
	}
	return 0, false
}

// comoReal convierte un número del PDF a float64. Lo que no es un número vale 0:
// no devuelve un «se pudo» porque ningún sitio lo miraba —en un flujo de
// contenido un operando que no es número es un fichero roto y el 0 es la
// degradación que ya se quería—, y un resultado que nadie usa solo obliga a
// escribir `, _` en cada llamada.
func comoReal(o pdfObj) float64 {
	switch v := o.(type) {
	case int64:
		return float64(v)
	case float64:
		return v
	}
	return 0
}

// --- Léxico ------------------------------------------------------------------

// pdfLexer recorre bytes de PDF. Trabaja sobre el buffer sin copiarlo.
type pdfLexer struct {
	buf []byte
	pos int
	// prof es cuántos arrays y diccionarios hay abiertos ahora mismo.
	prof int
}

func esEspacioPDF(c byte) bool {
	return c == 0 || c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == ' '
}

// esDelimitadorPDF: los caracteres que cortan un token (§7.2.2).
func esDelimitadorPDF(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func (lx *pdfLexer) byteActual() byte {
	if lx.pos < len(lx.buf) {
		return lx.buf[lx.pos]
	}
	return 0
}

// saltarEspacios avanza sobre blancos y comentarios («% hasta fin de línea»).
func (lx *pdfLexer) saltarEspacios() {
	for lx.pos < len(lx.buf) {
		c := lx.buf[lx.pos]
		if esEspacioPDF(c) {
			lx.pos++
			continue
		}
		if c == '%' {
			for lx.pos < len(lx.buf) && lx.buf[lx.pos] != '\n' && lx.buf[lx.pos] != '\r' {
				lx.pos++
			}
			continue
		}
		return
	}
}

// aceptaPalabra consume la palabra clave si viene ahora mismo.
func (lx *pdfLexer) aceptaPalabra(p string) bool {
	lx.saltarEspacios()
	if !bytes.HasPrefix(lx.buf[lx.pos:], []byte(p)) {
		return false
	}
	fin := lx.pos + len(p)
	// Debe terminar en delimitador o espacio: «endstream» no es «end».
	if fin < len(lx.buf) && !esEspacioPDF(lx.buf[fin]) && !esDelimitadorPDF(lx.buf[fin]) {
		return false
	}
	lx.pos = fin
	return true
}

// entero lee el siguiente número entero, saltando lo que haya antes.
func (lx *pdfLexer) entero() (int64, bool) {
	lx.saltarEspacios()
	inicio := lx.pos
	if lx.byteActual() == '+' || lx.byteActual() == '-' {
		lx.pos++
	}
	digitos := lx.pos
	for lx.pos < len(lx.buf) && lx.buf[lx.pos] >= '0' && lx.buf[lx.pos] <= '9' {
		lx.pos++
	}
	if lx.pos == digitos {
		lx.pos = inicio
		return 0, false
	}
	v, err := strconv.ParseInt(string(lx.buf[inicio:lx.pos]), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// --- Análisis sintáctico -----------------------------------------------------

// parsearObjeto lee un objeto PDF completo en la posición actual.
func (d *PDFDoc) parsearObjeto(lx *pdfLexer) (pdfObj, error) {
	lx.saltarEspacios()
	if lx.pos >= len(lx.buf) {
		return nil, fmt.Errorf("%w: fin de fichero inesperado", ErrPDFCorrupto)
	}
	switch c := lx.buf[lx.pos]; {
	case c == '<' && lx.pos+1 < len(lx.buf) && lx.buf[lx.pos+1] == '<':
		return d.parsearDiccionario(lx)
	case c == '<':
		return lx.cadenaHex(), nil
	case c == '(':
		return lx.cadenaLiteral(), nil
	case c == '/':
		return lx.nombre(), nil
	case c == '[':
		return d.parsearArray(lx)
	case c == ']' || c == '>' || c == '}':
		lx.pos++ // delimitador suelto: se consume para no atascar el bucle
		return pdfNull{}, nil
	case lx.aceptaPalabra("true"):
		return true, nil
	case lx.aceptaPalabra("false"):
		return false, nil
	case lx.aceptaPalabra("null"):
		return pdfNull{}, nil
	default:
		return d.parsearNumeroOReferencia(lx)
	}
}

// parsearNumeroOReferencia distingue «12» de «12 0 R» mirando hacia delante: sin
// ese salto atrás, una referencia se leería como dos enteros sueltos.
func (d *PDFDoc) parsearNumeroOReferencia(lx *pdfLexer) (pdfObj, error) {
	inicio := lx.pos
	// `esReal` y no `real`: llamarlo `real` tapa el builtin homónimo.
	num, esReal, ok := lx.numero()
	if !ok {
		lx.pos = inicio + 1 // byte no reconocido: avanzar para no colgarse
		return pdfNull{}, nil
	}
	// Solo un ENTERO puede ser el principio de «12 0 R». La aserción se comprueba
	// en vez de darse por hecha: hoy `numero` devuelve int64 siempre que esReal sea
	// falso, pero eso es un acuerdo entre dos funciones, no algo que el tipo diga,
	// y una aserción a ciegas se convierte en pánico el día que deje de cumplirse.
	entero, esEntero := num.(int64)
	if esReal || !esEntero {
		return num, nil
	}
	guardado := lx.pos
	if gen, ok := lx.entero(); ok {
		if lx.aceptaPalabra("R") {
			return pdfRef{Num: int(entero), Gen: int(gen)}, nil
		}
	}
	lx.pos = guardado
	return num, nil
}

// numero lee un entero o un real. Devuelve además si tenía parte decimal.
func (lx *pdfLexer) numero() (pdfObj, bool, bool) {
	lx.saltarEspacios()
	inicio := lx.pos
	if lx.byteActual() == '+' || lx.byteActual() == '-' {
		lx.pos++
	}
	hayDigitos, hayPunto := false, false
	for lx.pos < len(lx.buf) {
		c := lx.buf[lx.pos]
		if c >= '0' && c <= '9' {
			hayDigitos = true
			lx.pos++
			continue
		}
		if c == '.' && !hayPunto {
			hayPunto = true
			lx.pos++
			continue
		}
		break
	}
	if !hayDigitos && !hayPunto {
		lx.pos = inicio
		return nil, false, false
	}
	txt := string(lx.buf[inicio:lx.pos])
	if hayPunto {
		v, err := strconv.ParseFloat(txt, 64)
		if err != nil {
			return nil, false, false
		}
		return v, true, true
	}
	v, err := strconv.ParseInt(txt, 10, 64)
	if err != nil {
		return nil, false, false
	}
	return v, false, true
}

// nombre lee /Nombre, deshaciendo los escapes «#xx».
func (lx *pdfLexer) nombre() pdfName {
	lx.pos++ // '/'
	var out []byte
	for lx.pos < len(lx.buf) {
		c := lx.buf[lx.pos]
		if esEspacioPDF(c) || esDelimitadorPDF(c) {
			break
		}
		if c == '#' && lx.pos+2 < len(lx.buf) {
			alto, ok1 := valorHex(lx.buf[lx.pos+1])
			bajo, ok2 := valorHex(lx.buf[lx.pos+2])
			if ok1 && ok2 {
				out = append(out, alto<<4|bajo)
				lx.pos += 3
				continue
			}
		}
		out = append(out, c)
		lx.pos++
	}
	return pdfName(out)
}

// cadenaLiteral lee «(texto)» con paréntesis anidados y escapes de barra.
func (lx *pdfLexer) cadenaLiteral() pdfString {
	lx.pos++ // '('
	var out []byte
	nivel := 1
	for lx.pos < len(lx.buf) {
		c := lx.buf[lx.pos]
		lx.pos++
		switch c {
		case '\\':
			if b, hay := lx.escapeLiteral(); hay {
				out = append(out, b)
			}
		case '(':
			nivel++
			out = append(out, c)
		case ')':
			nivel--
			if nivel == 0 {
				return out
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}

// escapeLiteral resuelve lo que viene detrás de una barra invertida dentro de
// «(...)» y devuelve el byte que aporta a la cadena. El segundo resultado es
// falso cuando NO aporta ninguno: la continuación de línea («\» seguido de salto)
// y la barra final de un fichero truncado.
func (lx *pdfLexer) escapeLiteral() (byte, bool) {
	if lx.pos >= len(lx.buf) {
		return 0, false
	}
	e := lx.buf[lx.pos]
	lx.pos++
	switch e {
	case 'n':
		return '\n', true
	case 'r':
		return '\r', true
	case 't':
		return '\t', true
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	case '\n': // continuación de línea: no aporta carácter
		return 0, false
	case '\r':
		if lx.byteActual() == '\n' {
			lx.pos++ // CRLF cuenta como UN salto
		}
		return 0, false
	}
	if e < '0' || e > '7' {
		return e, true // \( \) \\ y cualquier otro: el carácter, literal
	}
	// Octal de hasta tres dígitos. Tres dígitos llegan hasta \777 = 511, que no
	// cabe en un byte: la especificación (§7.3.4.2) dice que el desbordamiento por
	// arriba se IGNORA, así que se enmascaran los 8 bits bajos. La máscara está
	// escrita a propósito en vez de dejarla implícita en la conversión: así se ve
	// que el recorte es la regla de la norma y no un descuido.
	v := int(e - '0')
	for k := 0; k < 2 && lx.pos < len(lx.buf); k++ {
		o := lx.buf[lx.pos]
		if o < '0' || o > '7' {
			break
		}
		v = v*8 + int(o-'0')
		lx.pos++
	}
	return byte(v & 0xFF), true
}

// cadenaHex lee «<48656C6C6F>».
func (lx *pdfLexer) cadenaHex() pdfString {
	lx.pos++ // '<'
	inicio := lx.pos
	for lx.pos < len(lx.buf) && lx.buf[lx.pos] != '>' {
		lx.pos++
	}
	datos := lx.buf[inicio:lx.pos]
	if lx.pos < len(lx.buf) {
		lx.pos++ // '>'
	}
	return decodificarHexASCII(datos)
}

func (d *PDFDoc) parsearArray(lx *pdfLexer) (pdfObj, error) {
	lx.pos++ // '['
	if lx.prof >= maxAnidadoObjeto {
		return nil, errAnidadoDemasiado
	}
	lx.prof++
	defer func() { lx.prof-- }()
	out := pdfArray{}
	for {
		lx.saltarEspacios()
		if lx.pos >= len(lx.buf) {
			return out, nil // array sin cerrar: se devuelve lo leído
		}
		if lx.buf[lx.pos] == ']' {
			lx.pos++
			return out, nil
		}
		antes := lx.pos
		o, err := d.parsearObjeto(lx)
		if errors.Is(err, errAnidadoDemasiado) {
			return nil, err
		}
		if errors.Is(err, ErrPDFCorrupto) {
			// Fichero truncado dentro del array: se entrega lo leído, igual que en
			// el corte de arriba. La condición mira el error CONCRETO en vez de
			// «err != nil» para que cualquier fallo nuevo que aparezca aquí salga
			// hacia arriba en lugar de perderse en silencio.
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if lx.pos == antes { // sin avance: cortar para no girar en vacío
			lx.pos++
			continue
		}
		out = append(out, o)
	}
}

func (d *PDFDoc) parsearDiccionario(lx *pdfLexer) (pdfObj, error) {
	lx.pos += 2 // '<<'
	if lx.prof >= maxAnidadoObjeto {
		return nil, errAnidadoDemasiado
	}
	lx.prof++
	defer func() { lx.prof-- }()
	out := pdfDict{}
	for {
		lx.saltarEspacios()
		if lx.pos >= len(lx.buf) {
			return out, nil
		}
		if lx.buf[lx.pos] == '>' {
			lx.pos++
			if lx.byteActual() == '>' {
				lx.pos++
			}
			return out, nil
		}
		if lx.buf[lx.pos] != '/' {
			// Clave que no es un nombre: el diccionario está mal formado. Se salta
			// el valor y se sigue, que es preferible a perder el objeto entero.
			antes := lx.pos
			_, err := d.parsearObjeto(lx)
			if errors.Is(err, errAnidadoDemasiado) {
				return nil, err
			}
			if err != nil || lx.pos == antes {
				lx.pos++
			}
			continue
		}
		clave := lx.nombre()
		valor, err := d.parsearObjeto(lx)
		if errors.Is(err, errAnidadoDemasiado) {
			return nil, err
		}
		if errors.Is(err, ErrPDFCorrupto) {
			// Mismo criterio que en parsearArray: el fichero se acabó a mitad del
			// diccionario y se devuelven las claves que sí se leyeron. Cualquier
			// otro error se propaga en vez de tragarse.
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out[clave] = valor
	}
}
