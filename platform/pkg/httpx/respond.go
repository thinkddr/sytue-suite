// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
)

// maxBodyBytes limita por defecto el tamaño del cuerpo JSON a decodificar (1 MiB).
const maxBodyBytes = 1 << 20

// errorEnvelope es el sobre JSON canónico de error de la API.
type errorEnvelope struct {
	Error     *Error `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

// JSON escribe v como JSON con el código de estado dado.
func JSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	jsonResponse(w, r, status, v, true)
}

// JSONWithLiteralHTML writes a JSON API payload without expanding HTML characters.
// Use it for bounded opaque data responses, never for embedding JSON into HTML.
func JSONWithLiteralHTML(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	jsonResponse(w, r, status, v, false)
}

func jsonResponse(w http.ResponseWriter, r *http.Request, status int, v any, escapeHTML bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil || status == http.StatusNoContent {
		return
	}
	if err := encodeJSON(w, v, escapeHTML); err != nil {
		log.From(r.Context()).Error("httpx: fallo al codificar respuesta JSON", "error", err)
	}
}

func encodeJSON(w io.Writer, v any, escapeHTML bool) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(escapeHTML)
	return enc.Encode(v)
}

// ErrJSONResponseTooLarge means no response bytes should be sent. The caller can
// choose a smaller page before committing status headers.
var ErrJSONResponseTooLarge = errors.New("httpx: JSON response exceeds its byte limit")

// EncodeJSONLimit uses the same JSON encoder as JSONWithLiteralHTML, but prepares
// a bounded response before any headers are committed. Do not embed it in HTML.
func EncodeJSONLimit(v any, maxBytes int) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, ErrJSONResponseTooLarge
	}
	w := limitedJSONBuffer{max: maxBytes}
	if err := encodeJSON(&w, v, false); err != nil {
		return nil, err
	}
	return w.buf.Bytes(), nil
}

type limitedJSONBuffer struct {
	buf bytes.Buffer
	max int
}

func (w *limitedJSONBuffer) Write(p []byte) (int, error) {
	if len(p) > w.max-w.buf.Len() {
		return 0, ErrJSONResponseTooLarge
	}
	return w.buf.Write(p)
}

// NoContent responde 204 sin cuerpo.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// Fail escribe una respuesta de error canónica. Cualquier error se normaliza a
// *Error; los 5xx registran la causa subyacente y los 4xx, su código.
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	apiErr := AsError(err)
	reqID := RequestIDFromContext(r.Context())

	switch {
	case apiErr.Status >= http.StatusInternalServerError:
		log.From(r.Context()).Error("httpx: error de servidor",
			"code", logsafe.Text(apiErr.Code), "error", logsafe.Text(apiErr.Error()), "request_id", reqID)

	case apiErr.Status >= http.StatusBadRequest:
		// UN 4xx NO DEJABA NADA. La línea de `http_request` guarda el status
		// pero no el porqué, así que en los registros un rechazo se veía como
		// «status=400» a secas: mismo aspecto para un JSON mal formado, un tope
		// de gasto superado o una suscripción que falta. Auditando producción el
		// 2026-08-12 aparecieron tres intentos de desplegar rechazados con 400 en
		// trece segundos —alguien pulsando el botón— y no había forma de saber
		// cuál de las seis validaciones había saltado.
		//
		// Va el CÓDIGO, no el mensaje: el código es la etiqueta estable de la
		// causa, mientras que el mensaje se compone con Params que pueden llevar
		// dentro lo que escribió el usuario. Un registro no es sitio para eso.
		log.From(r.Context()).Info("httpx: petición rechazada",
			"status", apiErr.Status, "code", logsafe.Text(apiErr.Code), "request_id", reqID)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// El texto de "message" va SIEMPRE en inglés base: se etiqueta como tal para
	// que un cliente sepa que, si quiere su idioma, debe traducir por "code".
	w.Header().Set("Content-Language", BaseLanguage)
	w.WriteHeader(apiErr.Status)
	// No filtrar causa subyacente al cliente. El mensaje viaja ya compuesto con
	// sus Params (respaldo legible) y los Params también sueltos (traducible).
	safe := &Error{
		Status:  apiErr.Status,
		Code:    apiErr.Code,
		Message: apiErr.RenderMessage(),
		Params:  apiErr.Params,
		Details: apiErr.Details,
	}
	if encErr := json.NewEncoder(w).Encode(errorEnvelope{Error: safe, RequestID: reqID}); encErr != nil {
		log.From(r.Context()).Error("httpx: fallo al codificar error JSON", "error", encErr)
	}
}

// DecodeJSON decodifica el cuerpo de la petición en dst, rechazando campos
// desconocidos y cuerpos sobredimensionados. Devuelve un *Error 400 legible.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	return DecodeJSONLimit(w, r, dst, maxBodyBytes)
}

// DecodeJSONLimit preserves strict JSON validation while allowing a caller to
// declare a bounded payload larger than ordinary API forms, such as attachments.
func DecodeJSONLimit(w http.ResponseWriter, r *http.Request, dst any, limit int64) error {
	if limit <= 0 {
		limit = maxBodyBytes
	}
	if r.Body == nil {
		return errEmptyBody
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.As(err, &maxErr):
			return errBodyTooLarge.WithParams(map[string]any{"limit_bytes": int(limit)}).Wrap(err)
		case errors.Is(err, io.EOF):
			return errEmptyBody.Wrap(err)
		default:
			return errInvalidJSON.Wrap(err)
		}
	}
	// Rechazar payloads con datos adicionales tras el primer objeto.
	if dec.More() {
		return errMultipleJSONObjects
	}
	return nil
}

// Errores de decodificación: cada motivo tiene su propio código estable, porque
// es el código —no el texto— lo que el cliente traduce y sobre lo que decide.
var (
	errEmptyBody           = NewError(http.StatusBadRequest, "empty_body", "The request body is empty.")
	errBodyTooLarge        = NewError(http.StatusBadRequest, "body_too_large", "The request body exceeds the limit of {limit_bytes} bytes.")
	errInvalidJSON         = NewError(http.StatusBadRequest, "invalid_json", "The request body is not valid JSON.")
	errMultipleJSONObjects = NewError(http.StatusBadRequest, "multiple_json_objects", "The request body must contain a single JSON object.")
)
