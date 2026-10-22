// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Error es un error de API. Lo que el cliente traduce es Code; Message es solo
// el respaldo legible en inglés. Ver el patrón completo en la cabecera del
// paquete (router.go).
type Error struct {
	// Status es el código HTTP a devolver.
	Status int `json:"-"`
	// Code es un identificador estable, en snake_case (p. ej. "not_found").
	// ES CONTRATO DE API: es la clave de traducción del cliente. Cambiarlo
	// rompe a los clientes igual que cambiar una ruta; añadir uno nuevo, no.
	Code string `json:"code"`
	// Message es el mensaje de respaldo EN INGLÉS (idioma base, §13.1), para
	// clientes sin catálogo (curl, un CLI viejo, logs de terceros). Admite
	// marcadores "{nombre}" que se sustituyen con Params al serializar, así
	// que el respaldo sigue siendo legible y completo.
	Message string `json:"message"`
	// Params son los datos que la frase interpola, ESTRUCTURADOS y aparte del
	// texto: sin ellos el cliente tiene el código pero no puede componer la
	// frase traducida ("tamaño de instancia desconocido %q" → params.size).
	// Son valores crudos, no preformateados: el cliente decide cómo se escribe
	// un número o una fecha en su idioma.
	Params map[string]any `json:"params,omitempty"`
	// Details es información estructurada opcional (p. ej. errores por campo).
	Details any `json:"details,omitempty"`
	// wrapped permite errors.Is/As sobre la causa subyacente.
	wrapped error
}

// Error implementa el interfaz error.
func (e *Error) Error() string {
	msg := e.RenderMessage()
	if e.wrapped != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, msg, e.wrapped)
	}
	return fmt.Sprintf("%s: %s", e.Code, msg)
}

// Unwrap permite errors.Is/As.
func (e *Error) Unwrap() error { return e.wrapped }

// Wrap asocia una causa subyacente (no se expone al cliente).
func (e *Error) Wrap(err error) *Error {
	c := *e
	c.wrapped = err
	return &c
}

// WithDetails añade detalles estructurados.
func (e *Error) WithDetails(d any) *Error {
	c := *e
	c.Details = d
	return &c
}

// WithMessage sustituye el mensaje de respaldo (inglés, con marcadores "{x}").
func (e *Error) WithMessage(msg string) *Error {
	c := *e
	c.Message = msg
	return &c
}

// WithCode deriva un error con un código estable propio del servicio,
// manteniendo el status HTTP. Es la forma habitual de especializar un error
// canónico: ErrConflict.WithCode("project_slug_taken").
func (e *Error) WithCode(code string) *Error {
	c := *e
	c.Code = code
	return &c
}

// WithParams adjunta los datos que interpola el mensaje. Sustituye los
// anteriores; se copia el mapa para no compartir estado con el llamante.
func (e *Error) WithParams(params map[string]any) *Error {
	c := *e
	c.Params = nil
	if len(params) > 0 {
		c.Params = make(map[string]any, len(params))
		for k, v := range params {
			c.Params[k] = v
		}
	}
	return &c
}

// RenderMessage devuelve el mensaje de respaldo con los Params sustituidos.
// Un marcador sin parámetro se deja tal cual: mejor una frase con "{size}" a la
// vista que perder el dato en silencio.
func (e *Error) RenderMessage() string {
	if len(e.Params) == 0 || !strings.ContainsRune(e.Message, '{') {
		return e.Message
	}
	var b strings.Builder
	rest := e.Message
	for {
		before, after, found := strings.Cut(rest, "{")
		b.WriteString(before)
		if !found {
			break
		}
		name, tail, closed := strings.Cut(after, "}")
		v, ok := e.Params[name]
		if !closed || !ok {
			b.WriteString("{")
			rest = after
			continue
		}
		fmt.Fprintf(&b, "%v", v)
		rest = tail
	}
	return b.String()
}

// NewError construye un Error de API. message va en inglés (idioma base).
func NewError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// Errores canónicos reutilizables por todos los servicios. Los mensajes son el
// respaldo en inglés; el cliente traduce por Code.
var (
	ErrBadRequest       = NewError(http.StatusBadRequest, "bad_request", "Invalid request.")
	ErrUnauthorized     = NewError(http.StatusUnauthorized, "unauthorized", "Authentication required.")
	ErrForbidden        = NewError(http.StatusForbidden, "forbidden", "You do not have permission to perform this action.")
	ErrNotFound         = NewError(http.StatusNotFound, "not_found", "Resource not found.")
	ErrConflict         = NewError(http.StatusConflict, "conflict", "Conflict with the current state of the resource.")
	ErrUnprocessable    = NewError(http.StatusUnprocessableEntity, "unprocessable", "The request could not be processed.")
	ErrUnsupportedMedia = NewError(http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported content type.")
	ErrTooManyRequests  = NewError(http.StatusTooManyRequests, "too_many_requests", "Too many requests.")
	ErrInternal         = NewError(http.StatusInternalServerError, "internal", "Internal server error.")
	ErrUnavailable      = NewError(http.StatusServiceUnavailable, "unavailable", "Service unavailable.")

	// ErrNoActiveOrg: el token no lleva organización. Código PROPIO y no un
	// `forbidden` genérico porque no es un permiso que falte, es una cuenta que
	// todavía no puede usar la suite, y la salida es completar la suscripción y
	// no pedirle acceso a nadie.
	//
	// Sin distinguirlo, este 403 llegaba a las diez apps como un error de carga
	// cualquiera: la sesión se abre, el usuario entra, y cada superficie enseña
	// «no se pudieron cargar los datos». El motivo real —que su organización no
	// tiene derecho de uso— no aparecía por ningún lado, ni en la interfaz ni en
	// un sitio donde buscarlo. Un cliente puede pasar días así.
	ErrNoActiveOrg = NewError(http.StatusForbidden, "no_active_organization",
		"Your session has no active organization. Complete your organization's subscription to use the suite.")
)

// AsError extrae un *Error de cualquier error; si no lo es, lo envuelve como
// ErrInternal preservando la causa para logging.
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return ErrInternal.Wrap(err)
}
