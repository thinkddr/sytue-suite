// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Firma en cadena por ACTUALIZACIÓN INCREMENTAL (§7.5.6).
//
// El camino de pdfsign.go reescribe el fichero entero, y eso invalida cualquier
// firma anterior: su /ByteRange apunta a unos bytes que ya no están donde
// estaban. Para que dos personas firmen el mismo contrato hace falta lo
// contrario — NO TOCAR NI UN BYTE de lo que ya hay y añadir al final:
//
//	[fichero original tal cual] [objetos nuevos] [xref nueva con /Prev] [%%EOF]
//
// La firma anterior sigue resumiendo exactamente el mismo rango, así que sigue
// verificando; la nueva cubre todo el fichero resultante. Un lector aplica las
// actualizaciones en orden y ve el documento con las dos.
package domain

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strconv"
)

// ErrNoIncremental explica por qué un documento concreto no se puede firmar sin
// reescribirlo. Quien llama puede caer a FirmarConCertificadoPDF sabiendo lo que
// eso implica (se pierden las firmas anteriores), pero la decisión es suya.
var ErrNoIncremental = errors.New("pdf: este documento no admite firma incremental")

// ErrCertificarFirmado: certificar (Bloquear) solo vale en la PRIMERA firma. No
// envuelve ErrNoIncremental a propósito: caer a reescribir el fichero borraría las
// firmas que ya tiene, que es justo lo que no se quiere.
var ErrCertificarFirmado = errors.New("pdf: solo la primera firma puede certificar el documento")

// FirmarIncrementalPDF firma anexando, sin tocar los bytes originales.
//
// Es el camino correcto cuando el documento YA lleva una firma. Sobre un
// documento sin firmar hace lo mismo que FirmarConCertificadoPDF, solo que el
// fichero crece en vez de reescribirse.
func FirmarIncrementalPDF(doc *PDFDoc, opts OpcionesFirma) ([]byte, error) {
	if err := admiteIncremental(doc, opts); err != nil {
		return nil, err
	}
	cadena, clave, err := cargarIdentidadDe(opts)
	if err != nil {
		return nil, err
	}
	opts = conValoresPorDefecto(opts, cadena[0])
	h, err := huecoIncremental(doc, opts)
	if err != nil {
		return nil, err
	}
	cms, err := construirCMS(h.resumen(), cadena, clave, opts.Sello)
	if err != nil {
		return nil, err
	}
	return h.incrustar(cms)
}

// admiteIncremental dice, ANTES de tocar nada, si el documento se puede firmar
// anexando. Quien llama cae a reescribirlo sabiendo lo que eso implica.
func admiteIncremental(doc *PDFDoc, opts OpcionesFirma) error {
	if doc == nil || len(doc.buf) == 0 {
		return fmt.Errorf("%w: no hay fichero de partida", ErrNoIncremental)
	}
	// Cifrado: los objetos nuevos tendrían que cifrarse con la clave del
	// documento, y las cadenas de la firma (/Contents entre otras) NO se cifran.
	// Mezclar las dos reglas en un anexo es la clase de detalle que produce un
	// fichero que abre en un lector y no en otro.
	if doc.cripto != nil {
		return fmt.Errorf("%w: está cifrado", ErrNoIncremental)
	}
	// Un fichero cuya xref hubo que reconstruir barriendo (pdfrepair.go) no tiene
	// una xref anterior a la que encadenar: el /Prev apuntaría a basura.
	if doc.reparado {
		return fmt.Errorf("%w: la tabla de referencias estaba rota", ErrNoIncremental)
	}
	if opts.Pagina < 0 || opts.Pagina >= doc.NumPaginas() {
		return fmt.Errorf("pdf: la página %d no existe", opts.Pagina+1)
	}
	if doc.paginas[opts.Pagina].num == 0 {
		return fmt.Errorf("%w: no se sabe qué objeto es la página", ErrNoIncremental)
	}
	raiz, ok := doc.trailer["Root"].(pdfRef)
	if !ok {
		return fmt.Errorf("%w: el /Root no es una referencia", ErrNoIncremental)
	}
	// El anexo reescribe la página y el catálogo con la generación 0. Si alguno
	// es de otra (un fichero ya actualizado que reutilizó números), las
	// referencias «N 1 R» que los nombran quedarían colgando y el resultado no lo
	// abre nadie: medido con el issue-149 de qpdf, que Poppler rechazaba entero.
	if doc.generacion(raiz.Num) != 0 || doc.generacion(doc.paginas[opts.Pagina].num) != 0 {
		return fmt.Errorf("%w: el catálogo o la página son de una generación distinta de 0", ErrNoIncremental)
	}
	if opts.Bloquear && len(doc.camposDeFirma()) > 0 {
		return ErrCertificarFirmado
	}
	return nil
}

// huecoIncremental anexa la firma con su hueco sin tocar un byte de lo que había.
func huecoIncremental(doc *PDFDoc, opts OpcionesFirma) (conHueco, error) {
	if err := admiteIncremental(doc, opts); err != nil {
		return conHueco{}, err
	}
	pag := doc.paginas[opts.Pagina]
	raizRef, _ := doc.trailer["Root"].(pdfRef)
	raiz, ok := doc.resolver(raizRef).(pdfDict)
	if !ok {
		return conHueco{}, fmt.Errorf("%w: el catálogo no se pudo leer", ErrNoIncremental)
	}
	xrefAnterior, err := offsetDelUltimoXref(doc.buf)
	if err != nil {
		return conHueco{}, err
	}

	base := len(doc.buf)
	sinFirmar, err := anexarFirma(doc, opts, pag, raizRef, raiz, xrefAnterior)
	if err != nil {
		return conHueco{}, err
	}
	// Se busca SOLO dentro del anexo. Delante hay firmas anteriores con su propio
	// /ByteRange y su propio /Contents, y localizar «el primero del fichero»
	// sobrescribiría la firma del primer firmante con los datos de la segunda:
	// exactamente el estropicio que esta función existe para evitar.
	inicio, fin, err := localizarHueco(sinFirmar[base:])
	if err != nil {
		return conHueco{}, err
	}
	inicio, fin = inicio+base, fin+base
	if err := escribirByteRange(sinFirmar[base:], [4]int{0, inicio, fin, len(sinFirmar) - fin}); err != nil {
		return conHueco{}, err
	}
	return conHueco{datos: sinFirmar, inicio: inicio, fin: fin}, nil
}

// anexarFirma escribe el bloque incremental completo detrás del original.
func anexarFirma(doc *PDFDoc, opts OpcionesFirma, pag PDFPagina,
	raizRef pdfRef, raiz pdfDict, xrefAnterior int,
) ([]byte, error) {
	siguiente := doc.maxObjeto() + 1
	nuevos := map[int]objetoSalida{}
	reservar := func() int { n := siguiente; siguiente++; return n }

	numSig := reservar()
	nuevos[numSig] = objetoSalida{valor: dictDeFirma(opts)}

	numCampo := reservar()
	campo := pdfDict{
		"Type":    pdfName("Annot"),
		"Subtype": pdfName("Widget"),
		"FT":      pdfName("Sig"),
		"T":       cadenaUTF16(nombreDeCampoLibre(doc)),
		"V":       pdfRef{Num: numSig},
		"F":       int64(4),
		"P":       pdfRef{Num: pag.num},
		"Rect":    rectPDF(opts.Rect),
	}
	if opts.Rect[2]-opts.Rect[0] > 1 && opts.Rect[3]-opts.Rect[1] > 1 {
		numAp := reservar()
		nuevos[numAp] = objetoAparienciaFirma(opts)
		campo["AP"] = pdfDict{"N": pdfRef{Num: numAp}}
	} else {
		campo["Rect"] = pdfArray{0.0, 0.0, 0.0, 0.0}
		numAp := reservar()
		nuevos[numAp] = aparienciaVacia()
		campo["AP"] = pdfDict{"N": pdfRef{Num: numAp}}
	}
	nuevos[numCampo] = objetoSalida{valor: campo}

	// La página se reescribe ENTERA con su /Annots ampliado. Es el objeto tal
	// cual está en el fichero, no el efectivo con los atributos heredados: copiar
	// esos aquí los congelaría, y si un día cambia el nodo /Pages la página
	// dejaría de seguirle.
	nodo, ok := doc.resolver(pdfRef{Num: pag.num}).(pdfDict)
	if !ok {
		return nil, fmt.Errorf("%w: la página %d no es un diccionario", ErrNoIncremental, opts.Pagina+1)
	}
	pagina := copiaLlana(nodo)
	pagina["Annots"] = append(arrayResuelto(doc, nodo["Annots"]), pdfRef{Num: numCampo})
	nuevos[pag.num] = objetoSalida{valor: pagina}

	// Y el catálogo, con el campo colgando de su /AcroForm.
	catalogo := copiaLlana(raiz)
	catalogo["AcroForm"] = acroFormAmpliado(doc, raiz, numCampo)
	if opts.Bloquear {
		// Sin /Perms en el catálogo el /DocMDP de la firma no lo mira ningún
		// lector: el documento salía «bloqueado» solo en el nombre de la casilla.
		catalogo["Perms"] = pdfDict{"DocMDP": pdfRef{Num: numSig}}
	}
	nuevos[raizRef.Num] = objetoSalida{valor: catalogo}

	return escribirAnexo(doc, nuevos, siguiente, raizRef, xrefAnterior, nil)
}

// escribirAnexo serializa los objetos nuevos y la xref que los indexa.
func escribirAnexo(doc *PDFDoc, nuevos map[int]objetoSalida, siguiente int,
	raizRef pdfRef, xrefAnterior int, extrasTrailer pdfDict,
) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write(doc.buf)
	// Un %%EOF sin salto detrás deja el primer «N 0 obj» pegado a él.
	if n := buf.Len(); n > 0 && buf.Bytes()[n-1] != '\n' {
		buf.WriteByte('\n')
	}

	enTabla := xrefEsTablaClasica(doc.buf, xrefAnterior)
	numXref := 0
	if !enTabla {
		// El propio flujo de xref es un objeto, y se indexa a sí mismo.
		numXref = siguiente
		siguiente++
	}

	nums := make([]int, 0, len(nuevos)+1)
	for n := range nuevos {
		nums = append(nums, n)
	}
	if numXref != 0 {
		nums = append(nums, numXref)
	}
	sort.Ints(nums)

	offsets := make(map[int]int, len(nums))
	for _, n := range nums {
		if n == numXref {
			continue // se escribe al final, cuando ya sabe dónde cae
		}
		offsets[n] = buf.Len()
		buf.WriteString(strconv.Itoa(n) + " 0 obj\n")
		buf.Write(serializarSinCifrar(nuevos[n]))
		buf.WriteString("\nendobj\n")
	}

	inicioXref := buf.Len()
	trailer := pdfDict{
		"Size": int64(siguiente),
		"Root": raizRef,
		"Prev": int64(xrefAnterior),
	}
	if id, ok := doc.trailer["ID"]; ok {
		trailer["ID"] = id
	}
	// El /Info vive en el TRAILER y no en el catálogo, así que un anexo que
	// cambie los metadatos tiene que reescribirlo aquí. Se arrastra el que
	// hubiera para no perderlo cuando el anexo va de otra cosa.
	if info, ok := doc.trailer["Info"]; ok {
		trailer["Info"] = info
	}
	for k, v := range extrasTrailer {
		trailer[k] = v
	}
	if enTabla {
		escribirXrefClasica(&buf, nums, offsets, trailer)
	} else {
		offsets[numXref] = inicioXref
		escribirXrefEnFlujo(&buf, numXref, nums, offsets, trailer)
	}
	buf.WriteString("startxref\n" + strconv.Itoa(inicioXref) + "\n%%EOF\n")
	return buf.Bytes(), nil
}

// escribirXrefClasica emite la tabla y el trailer, agrupando los números
// consecutivos en subsecciones como manda §7.5.4.
func escribirXrefClasica(buf *bytes.Buffer, nums []int, offsets map[int]int, trailer pdfDict) {
	buf.WriteString("xref\n")
	for i := 0; i < len(nums); {
		j := i + 1
		for j < len(nums) && nums[j] == nums[j-1]+1 {
			j++
		}
		buf.WriteString(strconv.Itoa(nums[i]) + " " + strconv.Itoa(j-i) + "\n")
		for _, n := range nums[i:j] {
			buf.WriteString(pad10(offsets[n]) + " 00000 n \n")
		}
		i = j
	}
	buf.WriteString("trailer\n" + string(serializarObjetoPDF(trailer)) + "\n")
}

// escribirXrefEnFlujo emite la misma información como /Type /XRef (§7.5.8), que
// es lo que toca cuando el fichero original ya usa flujos de referencias: un
// lector que solo entiende ese formato no sabría leer una tabla clásica.
//
// Sin comprimir a propósito: /Filter es opcional y son unas decenas de bytes.
func escribirXrefEnFlujo(buf *bytes.Buffer, numXref int, nums []int, offsets map[int]int, trailer pdfDict) {
	var indice pdfArray
	var datos bytes.Buffer
	for i := 0; i < len(nums); {
		j := i + 1
		for j < len(nums) && nums[j] == nums[j-1]+1 {
			j++
		}
		indice = append(indice, int64(nums[i]), int64(j-i))
		for _, n := range nums[i:j] {
			off := offsets[n]
			datos.WriteByte(1) // tipo 1: objeto suelto en un offset
			// Máscara explícita: el offset cabe de sobra en 4 bytes (el fichero no
			// llega a 4 GiB), pero sin ella el análisis lee un int truncándose.
			datos.Write([]byte{
				byte((off >> 24) & 0xFF), byte((off >> 16) & 0xFF),
				byte((off >> 8) & 0xFF), byte(off & 0xFF),
			})
			datos.Write([]byte{0, 0}) // generación
		}
		i = j
	}
	dict := pdfDict{
		"Type":  pdfName("XRef"),
		"W":     pdfArray{int64(1), int64(4), int64(2)},
		"Index": indice,
	}
	for k, v := range trailer {
		dict[k] = v
	}
	buf.WriteString(strconv.Itoa(numXref) + " 0 obj\n")
	buf.Write(serializarSinCifrar(objetoSalida{valor: dict, flujo: datos.Bytes(), esFlujo: true}))
	buf.WriteString("\nendobj\n")
}

// serializarSinCifrar escribe un objeto sin pasar por el cifrador: el anexo solo
// se construye sobre documentos sin cifrar (lo comprueba FirmarIncrementalPDF).
func serializarSinCifrar(obj objetoSalida) []byte {
	var e PDFEnsamblador
	return e.serializar(0, obj)
}

// maxObjeto devuelve el número de objeto más alto que usa el fichero. Se mira
// también la xref y no solo /Size porque un /Size corto (lo escriben algunos
// generadores) haría que el anexo pisara objetos existentes.
func (d *PDFDoc) maxObjeto() int {
	mayor := 0
	if n, ok := comoEntero(d.resolver(d.trailer["Size"])); ok && int(n) > 0 {
		mayor = int(n) - 1
	}
	for n := range d.xref {
		if n > mayor {
			mayor = n
		}
	}
	for n := range d.objs {
		if n > mayor {
			mayor = n
		}
	}
	return mayor
}

// offsetDelUltimoXref lee el `startxref` final, que es de donde cuelga toda la
// cadena de actualizaciones anteriores.
func offsetDelUltimoXref(buf []byte) (int, error) {
	cola := buf
	if len(cola) > 2048 {
		cola = cola[len(cola)-2048:]
	}
	i := bytes.LastIndex(cola, []byte("startxref"))
	if i < 0 {
		return 0, fmt.Errorf("%w: no hay startxref al final", ErrNoIncremental)
	}
	campo := bytes.TrimLeft(cola[i+len("startxref"):], " \r\n\t")
	fin := 0
	for fin < len(campo) && campo[fin] >= '0' && campo[fin] <= '9' {
		fin++
	}
	off, err := strconv.Atoi(string(campo[:fin]))
	if err != nil || off <= 0 || off >= len(buf) {
		return 0, fmt.Errorf("%w: el startxref final no apunta a ningún sitio", ErrNoIncremental)
	}
	return off, nil
}

// xrefEsTablaClasica mira si en ese offset empieza la palabra `xref` (tabla) o
// un objeto (flujo de referencias).
func xrefEsTablaClasica(buf []byte, off int) bool {
	resto := bytes.TrimLeft(buf[off:], " \r\n\t")
	return bytes.HasPrefix(resto, []byte("xref"))
}

// copiaLlana duplica el primer nivel del diccionario. Basta: lo que hay debajo
// no se toca, y las referencias siguen apuntando a los objetos originales.
func copiaLlana(d pdfDict) pdfDict {
	out := make(pdfDict, len(d)+1)
	for k, v := range d {
		out[k] = v
	}
	return out
}

// arrayResuelto devuelve el array al que apunta el valor, ya sea directo o por
// referencia. Vacío si no hay ninguno.
func arrayResuelto(doc *PDFDoc, v pdfObj) pdfArray {
	arr, _ := doc.resolver(v).(pdfArray)
	return append(pdfArray{}, arr...)
}

// acroFormAmpliado devuelve el /AcroForm del documento con el campo nuevo
// añadido a /Fields, conservando lo que ya hubiera: un formulario existente con
// sus campos NO se puede sustituir por uno con solo la firma.
func acroFormAmpliado(doc *PDFDoc, raiz pdfDict, numCampo int) pdfDict {
	var out pdfDict
	if ya, ok := doc.resolver(raiz["AcroForm"]).(pdfDict); ok {
		out = copiaLlana(ya)
	} else {
		out = pdfDict{}
	}
	out["Fields"] = append(arrayResuelto(doc, out["Fields"]), pdfRef{Num: numCampo})
	// /SigFlags 3 = hay firmas y el fichero no se puede guardar de forma que las
	// invalide. Se fuerza a 3 aunque ya trajera otro valor: a partir de aquí el
	// documento SÍ tiene firma.
	out["SigFlags"] = int64(3)
	return out
}

// nombreDeCampoLibre evita chocar con el nombre de una firma anterior: dos
// campos con el mismo /T son el MISMO campo para un lector, así que la segunda
// firma se leería como una versión de la primera.
func nombreDeCampoLibre(doc *PDFDoc) string {
	usados := map[string]bool{}
	for _, f := range doc.FirmasDelDocumento() {
		usados[f.Campo] = true
	}
	for i := 1; ; i++ {
		n := "Sytue.Firma" + strconv.Itoa(i)
		if !usados[n] {
			return n
		}
	}
}
