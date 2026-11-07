// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"net/http"
	"testing"
)

// Cuentan como escritura los verbos que cambian algo, también los de WebDAV y el
// MKCALENDAR de CalDAV, por donde Apple Calendar crea un calendario. VISTO EN ROJO sin
// MKCALENDAR en la lista: el calendario nuevo no mandaba la señal de cambios.
func TestEsEscrituraCuentaLosVerbosQueCambianAlgo(t *testing.T) {
	escrituras := []string{
		http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
		"MKCOL", "MOVE", "COPY", "PROPPATCH", "MKCALENDAR",
	}
	for _, m := range escrituras {
		if !EsEscritura(m) {
			t.Errorf("%s no cuenta como escritura", m)
		}
	}
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions, "PROPFIND", "REPORT"} {
		if EsEscritura(m) {
			t.Errorf("%s cuenta como escritura", m)
		}
	}
}
