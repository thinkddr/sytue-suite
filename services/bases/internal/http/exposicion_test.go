// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package baseshttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Desde el 2026-10-05 el Ingress publica `/api/v1` y solo eso. Lo que sirve el mismo
// puerto SIN sesión (formularios públicos y conector) no debe alcanzarse subiendo desde
// `/api/v1` con `..`: el router no normaliza la ruta, así que esas peticiones se quedan
// en el árbol con sesión y mueren en el 401 de RequireAuth.
//
// Verlo en rojo: `r.Use(middleware.CleanPath)` en NewRouter.
func TestDesdeApiV1NoSeSubeAlArbolSinSesion(t *testing.T) {
	srv := NewRouter(Deps{})
	for _, ruta := range []string{
		"/api/v1/../../_bases/v1/connector/read",
		"/api/v1/%2e%2e/%2e%2e/_bases/v1/connector/read",
		"/api/v1/bases/../../../_bases/v1/forms/abc/",
		"/api/v1/..%2f..%2f_bases/v1/forms/abc/altcha",
		"/api/v1//../_bases/v1/connector/read",
	} {
		r := httptest.NewRequest(http.MethodGet, "http://bases.test"+ruta, http.NoBody)
		r.Header.Set("X-Sytue-Consumer", "calcus")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		// 401 del árbol con sesión («Falta el token Bearer»), no el del conector ni un
		// 404/200 de los formularios.
		if w.Code != http.StatusUnauthorized || strings.Contains(w.Header().Get("WWW-Authenticate"), "connector") || strings.Contains(w.Body.String(), "connection_invalid") {
			t.Errorf("%s: %d %s (WWW-Authenticate %q)", ruta, w.Code, w.Body, w.Header().Get("WWW-Authenticate"))
		}
	}
}

// La API con sesión no abre orígenes: una app nativa no necesita CORS, y la web habla
// con su BFF, no con este host.
func TestLaAPINoAbreCORS(t *testing.T) {
	srv := NewRouter(Deps{})
	for _, metodo := range []string{http.MethodOptions, http.MethodGet} {
		r := httptest.NewRequest(metodo, "http://bases.test/api/v1/bases", http.NoBody)
		r.Header.Set("Origin", "https://otra.example")
		r.Header.Set("Access-Control-Request-Method", "GET")
		r.Header.Set("Access-Control-Request-Headers", "authorization")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		for cab := range w.Header() {
			if strings.HasPrefix(cab, "Access-Control-") {
				t.Errorf("%s /api/v1/bases responde %s: %q", metodo, cab, w.Header().Get(cab))
			}
		}
		if w.Code < 400 {
			t.Errorf("%s sin token: %d", metodo, w.Code)
		}
	}
}
