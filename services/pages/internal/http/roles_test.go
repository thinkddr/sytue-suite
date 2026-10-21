// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// soloAdministracion es la matriz, ESCRITA ENTERA, de lo que en la API de Sytue Web hace
// solo quien administra la organización (`owner` o `admin`): un miembro recibe 403 antes de
// que el manejador toque nada. No se deriva del router a propósito: una ruta que pase a
// pedir administración, o que deje de pedirla, tiene que cambiar esta lista (WEB-02).
//
// Fuera de esta lista quedan las que lo deciden por lo que traen —publicar en vivo, cambiar
// la dirección o los datos legales de un sitio, traducir la tienda—, que tienen su prueba.
var soloAdministracion = []string{
	// El sitio en internet: quitarlo y devolverlo.
	"DELETE /api/v1/sites/{id}/",
	"POST /api/v1/sites/{id}/restore",
	"POST /api/v1/sites/{id}/publications/{number}/rollback",
	"POST /api/v1/sites/{id}/unpublish",
	// Sus dominios.
	"GET /api/v1/sites/{id}/domains/",
	"POST /api/v1/sites/{id}/domains/",
	"POST /api/v1/sites/{id}/domains/{did}/check",
	"POST /api/v1/sites/{id}/domains/{did}/primary",
	"DELETE /api/v1/sites/{id}/domains/{did}",
	// La venta: abrirla o cerrarla y sus condiciones.
	"PUT /api/v1/sites/{id}/store/settings",
	"POST /api/v1/sites/{id}/store/enable",
	"POST /api/v1/sites/{id}/store/disable",
	// El dinero: a qué cuenta va y quién ha comprado.
	"POST /api/v1/sites/{id}/store/gateways",
	"DELETE /api/v1/sites/{id}/store/gateways/{provider}",
	"GET /api/v1/sites/{id}/store/orders",
	"GET /api/v1/sites/{id}/store/orders.csv",
	// Las respuestas de los formularios y a quién avisan (datos personales, WEB-02).
	"GET /api/v1/sites/{id}/forms/",
	"PUT /api/v1/sites/{id}/forms/{formId}/notify",
	"GET /api/v1/sites/{id}/forms/{formId}/submissions",
	"GET /api/v1/sites/{id}/forms/{formId}/submissions.csv",
	// Datos de fuera que acaban publicados y código que sale de la organización.
	"POST /api/v1/sites/{id}/bases/table",
	"POST /api/v1/sites/{id}/ia/export",
	"GET /api/v1/ia/jobs/{jid}/zip",
	// La clave de IA de la organización.
	"GET /api/v1/ia/key",
	"PUT /api/v1/ia/key",
	"DELETE /api/v1/ia/key",
	"POST /api/v1/ia/key:check",
}

func TestMatrizDeRolesDeLaAPI(t *testing.T) {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler { // sin Store un manejador da panic: aquí solo se mira la verja
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
	// Con el secreto del publicador puesto: esa verja es otra, la del BFF, no la del rol.
	montarRutas(g, &Handlers{Deps: Deps{Store: store.New(nil), Cfg: config.Config{PublicadorSecret: "secreto-publicador"}}})

	codigo := func(metodo, ruta string, roles []string) int {
		camino := reParametro.ReplaceAllString(ruta, "00000000-0000-0000-0000-000000000001")
		req := conAutenticacion(httptest.NewRequest(metodo, camino, strings.NewReader(`{}`)), roles, "pro")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Sytue-Publicador", "secreto-publicador")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	var negadas []string
	err := chi.Walk(r, func(metodo, ruta string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(ruta, "/api/v1/") {
			return nil
		}
		clave := metodo + " " + ruta
		if codigo(metodo, ruta, []string{"member"}) == http.StatusForbidden {
			negadas = append(negadas, clave)
			// Y a quien administra no se le niega por rol: sin esto la verja podría negar a todos.
			for _, rol := range []string{"admin", "owner"} {
				if codigo(metodo, ruta, []string{rol}) == http.StatusForbidden {
					t.Errorf("%s: 403 también para %s", clave, rol)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	esperadas := append([]string(nil), soloAdministracion...)
	sort.Strings(esperadas)
	sort.Strings(negadas)
	if strings.Join(negadas, "\n") != strings.Join(esperadas, "\n") {
		t.Fatalf("lo que la API niega a un miembro no es lo escrito en la matriz.\n niega:\n  %s\n matriz:\n  %s",
			strings.Join(negadas, "\n  "), strings.Join(esperadas, "\n  "))
	}
}
