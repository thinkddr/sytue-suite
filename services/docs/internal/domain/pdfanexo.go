// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Guardar un documento FIRMADO sin romper sus firmas.
//
// Anotar o rellenar reescriben el fichero entero (pdfwrite.go), y eso deja cada
// /ByteRange apuntando a unos bytes que ya no están donde estaban: la firma deja
// de validar aunque nadie haya tocado lo firmado. Medido con
// scripts/certipdf-compat: de 472 salidas cuya entrada traía firmas, 284 salían
// sin ellas, y ninguna operación lo decía.
//
// Lo correcto es lo mismo que hace la firma en cadena (pdfsignincr.go): no tocar
// ni un byte de lo que hay y AÑADIR detrás una actualización incremental
// (§7.5.6) con los objetos nuevos y los cambiados. Si eso no se puede —cifrado,
// tabla de referencias reconstruida, o una certificación (/DocMDP, §12.8.2.2)
// que no admite el cambio—, la operación NO reescribe en silencio: devuelve
// ErrInvalidaFirmas y quien llama pregunta antes de PermitirInvalidarFirmas.
package domain

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
)

// ErrInvalidaFirmas: guardar el cambio invalidaría las firmas del documento.
var ErrInvalidaFirmas = errors.New("pdf: guardar este cambio invalidaría las firmas del documento")

// maxObjetosAnexo acota la tabla del anexo: los objetos nuevos se numeran detrás
// del mayor del documento, y la numeración la pone el fichero.
const maxObjetosAnexo = 8 << 20

// PermitirInvalidarFirmas autoriza a reescribir este documento aunque esté
// firmado. Solo después de que el usuario lo haya confirmado.
func (d *PDFDoc) PermitirInvalidarFirmas() { d.invalidarFirmas = true }

// conservaFirmas dice si un guardado tiene que ir anexado: hay firmas y nadie ha
// autorizado a perderlas.
func (d *PDFDoc) conservaFirmas() bool {
	return !d.invalidarFirmas && len(d.camposDeFirma()) > 0
}

// cambioAnexo es lo que un anexo quiere cambiar, para comprobarlo contra la
// certificación del documento.
type cambioAnexo int

const (
	anexoAnotaciones cambioAnexo = iota
	anexoFormulario
)

// admiteAnexo dice, ANTES de tocar nada, si el cambio puede ir anexado.
func admiteAnexo(doc *PDFDoc, cambio cambioAnexo) error {
	motivo := ""
	switch {
	case doc.cripto != nil:
		motivo = "el documento está cifrado"
	case doc.reparado:
		motivo = "la tabla de referencias estaba rota"
	case doc.maxObjeto() >= maxObjetosAnexo:
		motivo = "demasiados objetos"
	}
	if _, ok := doc.trailer["Root"].(pdfRef); !ok && motivo == "" {
		motivo = "el /Root no es una referencia"
	}
	if _, err := offsetDelUltimoXref(doc.buf); err != nil && motivo == "" {
		motivo = "no se encuentra la última tabla de referencias"
	}
	for _, c := range doc.camposDeFirma() {
		// /P 1: ningún cambio; /P 2: rellenar y firmar, pero no anotar (§12.8.2.2).
		switch p := doc.permisoDocMDP(c); {
		case p == 1:
			motivo = "una firma de certificación no admite ningún cambio"
		case p == 2 && cambio == anexoAnotaciones:
			motivo = "una firma de certificación no admite anotaciones"
		}
	}
	if motivo != "" {
		return fmt.Errorf("%w: %s", ErrInvalidaFirmas, motivo)
	}
	return nil
}

// nuevoAnexo es un ensamblador cuyos objetos NUEVOS se numeran detrás de los del
// documento: lo que se escribe en él se puede anexar tal cual, y las
// referencias a objetos del documento siguen valiendo porque no se renumera nada.
func nuevoAnexo(doc *PDFDoc) *PDFEnsamblador {
	e := NuevoEnsambladorPDF()
	e.cuerpos = make([]objetoSalida, doc.maxObjeto())
	return e
}

// anexar escribe la actualización: los objetos cambiados del documento (con su
// número de siempre) y los nuevos del ensamblador.
func anexar(doc *PDFDoc, e *PDFEnsamblador, cambiados map[int]objetoSalida) ([]byte, error) {
	base := doc.maxObjeto() + 1
	nuevos := make(map[int]objetoSalida, len(cambiados)+len(e.cuerpos)-base+1)
	for num, obj := range cambiados {
		// El anexo escribe la generación 0; reescribir un objeto de otra
		// generación dejaría colgadas las referencias «N g R» que lo nombran.
		if doc.generacion(num) != 0 {
			return nil, fmt.Errorf("%w: el objeto %d es de otra generación", ErrInvalidaFirmas, num)
		}
		nuevos[num] = obj
	}
	for n := base; n <= len(e.cuerpos); n++ {
		nuevos[n] = e.cuerpos[n-1]
	}
	// Un PDF/A firmado al que se anexa algo que no cumple deja de declararlo
	// (decisión del dueño: la marca solo si cumple).
	if num, xmp, quitar := e.marcaPDFAEnAnexo(doc, nuevos); quitar {
		nuevos[num] = xmp
	}
	raiz, _ := doc.trailer["Root"].(pdfRef)
	xrefAnterior, err := offsetDelUltimoXref(doc.buf)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidaFirmas, err)
	}
	return escribirAnexo(doc, nuevos, len(e.cuerpos)+1, raiz, xrefAnterior, nil)
}

// generacion lee la generación con que está escrito el objeto `num`. Los que
// viven en un flujo de objetos son siempre de la 0.
func (d *PDFDoc) generacion(num int) int {
	x, ok := d.xref[num]
	if !ok || x.enFlujo || x.offset < 0 || x.offset >= int64(len(d.buf)) {
		return 0
	}
	cabeza := d.buf[x.offset:min(int(x.offset)+40, len(d.buf))]
	campos := bytes.Fields(cabeza)
	if len(campos) < 2 {
		return 0
	}
	g, err := strconv.Atoi(string(campos[1]))
	if err != nil {
		return 0
	}
	return g
}
