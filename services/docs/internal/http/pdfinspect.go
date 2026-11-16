// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// Índice de un PDF: qué páginas tiene, cómo son y qué dice cada una.
//
// Existe para resolver hoy el problema que el visor propio resolverá del todo:
// elegir páginas sin escribir «1,3,5-8» de memoria. Rasterizar un PDF a imagen
// sin dependencias externas es el trabajo más caro de todo Sytue PDF, pero para
// RECONOCER una página no hace falta verla — basta su número, su tamaño y las
// primeras palabras. Con eso el usuario sabe cuál es la portada, cuál el anexo y
// cuál la que quiere quitar.
//
// Cuando exista el render fiel, esta respuesta se le añade la miniatura y la
// interfaz no cambia de forma.

// paginaIndice describe una página para poder reconocerla en una lista.
type paginaIndice = domain.PageIndex

// pdfInspect atiende POST /api/v1/pdf/inspect: recibe un PDF y devuelve su
// índice. Conversión pura como el resto del grupo — nada se guarda.
func (h *Handler) pdfInspect(w http.ResponseWriter, r *http.Request) {
	if _, ok := subjectFrom(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	const framingMargin = 1 << 20
	r.Body = http.MaxBytesReader(w, r.Body, domain.MaxPDFBytes+framingMargin)
	//nolint:gosec // G120: el cuerpo ya está acotado por http.MaxBytesReader
	if err := r.ParseMultipartForm(domain.MaxPDFBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Fail(w, r, errPayloadTooLarge)
			return
		}
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			`Send the PDF as multipart/form-data in the "file" field.`,
		))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	docs, err := leerPDFsDelFormulario(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(docs) != 1 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Send a single PDF."))
		return
	}

	httpx.JSON(w, r, http.StatusOK, map[string]any{"paginas": indiceDe(docs[0], maxPaginasIndexadas, 0)})
}

// maxPaginasIndexadas acota los EXTRACTOS. Un PDF de diez mil páginas existe —un
// expediente escaneado— y no tiene sentido mandar diez mil extractos al
// navegador. `/pdf/inspect` (el orden de páginas de Drive) se queda además en
// las primeras, y la interfaz ofrece escribir el rango a mano.
const maxPaginasIndexadas = domain.MaxIndexedPages

// presupuestoExtractos es lo que `/pdf/content` se permite gastar en extractos.
//
// Sacar el texto de una página es leerla entera, y el contenido los sacaba de
// TODAS antes de contestar: medido con el motor real (arnés del frontal,
// 2026-10-16), el 100 % del tiempo de abrir un documento —un BOE de 43 páginas,
// 45 s con la máquina cargada; un formulario de 9, 9,4 s— para escribir 120
// caracteres bajo cada miniatura. Y además las cortaba en 500: en un expediente
// de 1192 páginas, de la 501 en adelante no existían en el visor.
//
// Ahora la lista lleva todas las páginas —su tamaño cuesta nada— y el extracto
// de las que dé tiempo a leer. La miniatura sigue enseñando la página dibujada.
const presupuestoExtractos = 300 * time.Millisecond

// indiceDe construye el índice de las `hasta` primeras páginas, con el extracto de
// las que quepan en `presupuesto` (0: de todas las que lo admitan).
func indiceDe(doc *domain.PDFDoc, hasta int, presupuesto time.Duration) []paginaIndice {
	return domain.IndexPages(doc, hasta, presupuesto)
}

func extractoDe(texto string, maxRunas int) string {
	return domain.Excerpt(texto, maxRunas)
}
