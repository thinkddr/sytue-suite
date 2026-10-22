// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// `/account/` devolvía un JSON `not_found` a un navegador. Lo que se fija: la
// barra final de un GET se quita con redirección (y la query sobrevive), y
// nada más cambia —un POST sin ruta sigue siendo 404, y la raíz no entra en un
// bucle de redirecciones a sí misma—.
func TestBarraFinalSobra(t *testing.T) {
	casos := []struct {
		metodo, ruta string
		estado       int
		destino      string
	}{
		{http.MethodGet, "/account/", 301, "/account"},
		{http.MethodGet, "/account/?tab=security", 301, "/account?tab=security"},
		{http.MethodHead, "/login/", 301, "/login"},
		{http.MethodPost, "/account/", 404, ""},
		{http.MethodGet, "/no-existe", 404, ""},
		{http.MethodGet, "/", 404, ""},
	}
	for _, c := range casos {
		rec := httptest.NewRecorder()
		barraFinalSobra(rec, httptest.NewRequest(c.metodo, c.ruta, http.NoBody))
		if rec.Code != c.estado {
			t.Errorf("%s %s: estado %d, se esperaba %d", c.metodo, c.ruta, rec.Code, c.estado)
		}
		if got := rec.Header().Get("Location"); got != c.destino {
			t.Errorf("%s %s: Location %q, se esperaba %q", c.metodo, c.ruta, got, c.destino)
		}
	}
}
