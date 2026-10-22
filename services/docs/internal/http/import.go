// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"io"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/plantillas"
)

// errPayloadTooLarge es el error canónico 413 para ficheros que superan el límite.
var errPayloadTooLarge = httpx.NewError(http.StatusRequestEntityTooLarge, "payload_too_large",
	"The file is larger than the maximum allowed size.")

// importDocument convierte un fichero ofimático subido (multipart/form-data,
// campo "file") al modelo inicial de un documento nuevo: {kind, title, content}.
// Es una conversión PURA: no toca la base de datos ni exige tenant; el cliente
// crea después el documento del kind devuelto y siembra `content` (el valor del
// registro CONTENT_KEY del editor) con una única operación set. Requiere sesión.
func (h *Handler) importDocument(w http.ResponseWriter, r *http.Request) {
	if _, ok := subjectFrom(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	// Cota dura del cuerpo (fichero + framing multipart) para no agotar memoria.
	const framingMargin = 1 << 20
	r.Body = http.MaxBytesReader(w, r.Body, domain.MaxImportBytes+framingMargin)
	//nolint:gosec // G120: el cuerpo ya está acotado por http.MaxBytesReader (línea anterior)
	if err := r.ParseMultipartForm(domain.MaxImportBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Fail(w, r, errPayloadTooLarge)
			return
		}
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(`Send the file as multipart/form-data in the "file" field.`))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(`The file to import is missing ("file" field).`))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, domain.MaxImportBytes+1))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Fail(w, r, errPayloadTooLarge)
			return
		}
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The file could not be read."))
		return
	}
	if len(data) > domain.MaxImportBytes {
		httpx.Fail(w, r, errPayloadTooLarge)
		return
	}

	// Los comentarios del fichero (y su ancla en el HTML) solo se piden si quien llama los va a
	// guardar al crear el documento: un <mark> sin comentario quedaría resaltado para siempre.
	// `locale` es el idioma de quien abre: los avisos que el importador deja dentro del
	// documento (un .pages que no se puede convertir, por ejemplo) salen en el suyo.
	q := r.URL.Query()
	result, err := domain.ImportLocalized(header.Filename, data, q.Get("comments") == "1", plantillas.Idioma(q.Get("locale")))
	if err != nil {
		if errors.Is(err, domain.ErrDemasiadoGrande) {
			httpx.Fail(w, r, errPayloadTooLarge)
			return
		}
		if errors.Is(err, domain.ErrInvalidFormat) {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The file could not be imported: unrecognised or damaged format."))
			return
		}
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, result)
}
