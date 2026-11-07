// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBalanceRechazaUnPeriodoMalEscrito: el sumas y saldos por período lee `desde`
// además de `hasta`, y una fecha mal escrita es un 400 que dice cuál, antes de tocar
// la base (por eso basta un Handlers sin store).
//
// VISTO EN ROJO: sin leer `desde`, la petición sigue hasta la base y revienta con el
// store a nil en vez de contestar 400.
func TestBalanceRechazaUnPeriodoMalEscrito(t *testing.T) {
	h := &Handlers{}
	for consulta, campo := range map[string]string{
		"desde=2026-13-01":                  "desde",
		"desde=2026-01-01&hasta=31/12/2026": "hasta",
	} {
		w := httptest.NewRecorder()
		h.Balance(w, httptest.NewRequest(http.MethodGet, "/api/v1/balance?"+consulta, http.NoBody))
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), campo+": YYYY-MM-DD") {
			t.Errorf("%s: %d %s", consulta, w.Code, w.Body.String())
		}
	}
}
