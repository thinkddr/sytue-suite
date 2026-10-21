// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// LA BÚSQUEDA TIENE QUE ACEPTAR QUERY, y se comprueba en el ENRUTADO —sin base de
// datos y sin ejecutar el handler—, que es donde estaba el fallo.
//
// El cliente pide `/notes/search` con `api.Consulta`, que manda el término en el
// cuerpo para que no se quede en el log de acceso de cada proxy. Registrada solo
// con `Get` NO se rompe nada visible: el router contesta 405 y el cliente reintenta
// por GET. Lo que se pierde es lo único que QUERY venía a dar —el término acaba en
// la URL igualmente— y, además, el cliente apaga QUERY para el resto de consultas
// de este servicio en cuanto ve ese 405.
//
// Un fallo que no se ve en ninguna respuesta solo lo puede sujetar una prueba.
func TestLaBusquedaAceptaQuery(t *testing.T) {
	// Un Store vacío, no nulo: montar el router le pide el `*sql.DB` para el cubo
	// de peticiones compartido. No llega a usarse porque aquí no se sirve nada.
	enrutador, vale := NewRouter(Deps{Store: &store.Store{}}).(chi.Router)
	if !vale {
		t.Fatal("NewRouter ya no devuelve un router de chi: esta prueba mira el enrutado, hay que reescribirla")
	}

	const ruta = "/api/v1/notes/search"
	// GET primero: si esto fallara, el fallo no es el método sino la ruta, y
	// conviene saberlo antes de acusar a QUERY.
	if !enrutador.Match(chi.NewRouteContext(), http.MethodGet, ruta) {
		t.Fatalf("%s no está enrutada ni para GET", ruta)
	}
	if !enrutador.Match(chi.NewRouteContext(), httpx.MethodQuery, ruta) {
		t.Errorf("%s no acepta %s: el cliente reintentaría por GET —con el término en la URL, "+
			"que es lo que se quería evitar— y apagaría QUERY para las demás consultas de este servicio",
			ruta, httpx.MethodQuery)
	}
}

// Las rutas de tareas existen, y lo que llega mal se rechaza ANTES de tocar la base
// de datos: con un Handlers vacío, llegar al store sería un pánico.
func TestRutasDeTareas(t *testing.T) {
	enrutador, _ := NewRouter(Deps{Store: &store.Store{}}).(chi.Router)
	if !enrutador.Match(chi.NewRouteContext(), http.MethodGet, "/api/v1/tasks") {
		t.Error("GET /tasks no está enrutada")
	}
	if !enrutador.Match(chi.NewRouteContext(), http.MethodPost, "/api/v1/notes/0b9f8c1e-2f9a-4c55-9a0e-3f2d9b6c1a10/postpone") {
		t.Error("POST /notes/{id}/postpone no está enrutada")
	}
	for _, q := range []string{"state=todas", "state=open&from=ayer", "to=2026-10-01"} {
		rec := httptest.NewRecorder()
		(&Handlers{}).ListTasks(rec, httptest.NewRequest(http.MethodGet, "/tasks?"+q, http.NoBody))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("/tasks?%s: %d, quería 400", q, rec.Code)
		}
	}
	for _, cuerpo := range []string{`{"date":"2026-02-30","title":"x"}`, `{"date":"mañana","title":"x"}`, `{"date":"2026-10-02","title":""}`} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/notes/x/postpone", strings.NewReader(cuerpo))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "0b9f8c1e-2f9a-4c55-9a0e-3f2d9b6c1a10")
		(&Handlers{}).PostponeTask(rec, req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("posponer con %s: %d, quería 400", cuerpo, rec.Code)
		}
	}
}
