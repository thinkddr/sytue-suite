// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

var reParametro = regexp.MustCompile(`\{[^}]+\}`)

// Cada ruta registrada tiene que ATENDERSE: pedirla por su propio patrón no puede acabar en
// el 404 o el 405 que da el router cuando no encuentra nada (texto plano; los manejadores
// contestan JSON). Dos áreas montaban `/sites/{id}` cada una por su lado (`handlers.go` y
// `sitios.go`) y chi se quedaba con la primera: GET/PUT/DELETE de un sitio, su lista de
// versiones, su restauración y `check-slug` fallaban en producción, mientras los tests de
// cada área —que montan solo la suya— salían en verde. Este monta el árbol entero, sin la
// verja de la sesión (que no es lo que se prueba) y recorre todo lo registrado.
func TestNingunaRutaQuedaTapada(t *testing.T) {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler { // un manejador sin su Store da panic: no es lo que se prueba
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			defer func() {
				if recover() != nil {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, req)
		})
	})
	var g grupos
	r.Route("/api/v1", func(api chi.Router) { g.api, g.apiLenta, g.apiPersonal = api, api, api })
	r.Route("/_pages", func(pub chi.Router) { g.publico = pub })
	r.Route("/webhooks", func(wh chi.Router) { g.webhooks = wh })
	g.borde = r
	montarRutas(g, &Handlers{Deps: Deps{Store: store.New(nil)}})

	sinRuta := func(metodo, camino string) bool {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(metodo, camino, http.NoBody))
		return w.Code == http.StatusMethodNotAllowed ||
			(w.Code == http.StatusNotFound && strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain"))
	}
	if !sinRuta(http.MethodGet, "/api/v1/esto-no-existe") {
		t.Fatal("el guardián no distingue «sin ruta» de un 404 del manejador: no sirve")
	}

	var visitadas int
	err := chi.Walk(r, func(metodo, ruta string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		ruta = strings.ReplaceAll(ruta, "/*", "")
		// `/_borde/permitido` contesta 404 en texto plano a propósito (el «ask» de Caddy).
		if ruta == "" || ruta == "/" || ruta == "/_borde/permitido" {
			return nil
		}
		visitadas++
		camino := reParametro.ReplaceAllString(strings.TrimSuffix(ruta, "/"), "p1")
		if sinRuta(metodo, camino) {
			t.Errorf("%s %s: el router no la atiende (tapada por otra del mismo prefijo)", metodo, ruta)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if visitadas < 40 {
		t.Fatalf("solo he recorrido %d rutas: el guardián no está mirando todo el árbol", visitadas)
	}
}
