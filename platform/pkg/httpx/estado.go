// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import "net/http"

// EsEscritura dice si el método cambia algo: POST, PUT, PATCH o DELETE. Los verbos
// de WebDAV que escriben (MKCOL, MOVE, COPY, PROPPATCH) también cuentan, y el
// MKCALENDAR de CalDAV (RFC 4791 §5.3.1), que es como Apple Calendar o Thunderbird
// crean un calendario.
func EsEscritura(metodo string) bool {
	switch metodo {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
		"MKCOL", "MOVE", "COPY", "PROPPATCH", "MKCALENDAR":
		return true
	}
	return false
}

// ServirConEstado sirve `next` y devuelve el código con que contestó (200 si el
// handler escribió sin cabecera). Es el mismo registro que usan el log y las métricas,
// así que conserva el streaming, el Hijack y el ResponseController.
func ServirConEstado(next http.Handler, w http.ResponseWriter, r *http.Request) int {
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	next.ServeHTTP(rec, r)
	return rec.status
}
