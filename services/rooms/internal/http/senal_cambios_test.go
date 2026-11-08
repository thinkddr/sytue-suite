// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Lo que va a ritmo de tecla (el borrador) o solo lee no manda la señal del widget;
// el resto de escrituras, sí. Visto en rojo quitando "/me/drafts" de sinSenalDeCambios.
func TestLasEscriturasSinSenalDeCambios(t *testing.T) {
	for ruta, sin := range map[string]bool{
		"/api/v1/me/drafts":        true,
		"/api/v1/gifs/search":      true,
		"/api/v1/people:resolve":   true,
		"/api/v1/channels":         false,
		"/api/v1/channels/c1/read": false,
		"/api/v1/channels/c1/join": false,
	} {
		if got := sinSenalDeCambios(httptest.NewRequest(http.MethodPut, ruta, http.NoBody)); got != sin {
			t.Errorf("%s: sin señal = %v, quiero %v", ruta, got, sin)
		}
	}
}
