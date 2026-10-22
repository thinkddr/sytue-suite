// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/plantillas"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

func routerSitiosTest() (*chi.Mux, *Handlers) {
	st := store.New(nil)
	h := &Handlers{
		Deps: Deps{
			Store: st,
		},
	}

	r := chi.NewRouter()
	r.Route("/sites", func(sr chi.Router) {
		sr.Get("/", h.listarSitios)
		sr.Post("/", h.crearSitio)
		sr.Get("/check-slug", h.comprobarSlug)

		sr.Route("/{id}", func(ssr chi.Router) {
			ssr.Get("/", h.obtenerSitio)
			ssr.Put("/", h.actualizarSitio)
			ssr.Delete("/", h.borrarSitio)
			ssr.Post("/restore", h.restaurarSitio)
			ssr.Get("/versions", h.listarVersiones)
			ssr.Post("/versions/{vid}/restore", h.restaurarVersion)
		})
	})

	r.Route("/templates", func(tr chi.Router) {
		tr.Get("/", h.listarPlantillas)
		tr.Get("/sections", h.listarSeccionesPlantilla)
		tr.Get("/{id}", h.obtenerPlantilla)
	})

	return r, h
}

func TestPlantillasHTTP(t *testing.T) {
	r, _ := routerSitiosTest()

	t.Run("GET /templates devuelve lista", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/templates", http.NoBody)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperado 200, obtenido %d", rec.Code)
		}
		var lista []plantillas.PlantillaInfo
		if err := json.Unmarshal(rec.Body.Bytes(), &lista); err != nil {
			t.Fatalf("json unmarshal: %v", err)
		}
		if len(lista) == 0 {
			t.Fatal("esperaba al menos 1 plantilla")
		}
	})

	t.Run("GET /templates/{id} devuelve plantilla o 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/templates/producto", http.NoBody)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperado 200, obtenido %d", rec.Code)
		}

		req404 := httptest.NewRequest(http.MethodGet, "/templates/inexistente", http.NoBody)
		rec404 := httptest.NewRecorder()
		r.ServeHTTP(rec404, req404)

		if rec404.Code != http.StatusNotFound {
			t.Fatalf("esperado 404, obtenido %d", rec404.Code)
		}
	})

	t.Run("GET /templates/sections devuelve secciones", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/templates/sections", http.NoBody)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperado 200, obtenido %d", rec.Code)
		}
	})
}

func TestSitiosHTTP_Validaciones(t *testing.T) {
	r, _ := routerSitiosTest()

	t.Run("GET /sites/check-slug vacío devuelve no disponible", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/sites/check-slug?slug=", http.NoBody)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("esperado 200, obtenido %d", rec.Code)
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp["available"] != false {
			t.Fatalf("esperaba available=false para slug vacío")
		}
	})

	t.Run("UUID inválido en /sites/{id}", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/sites/no-uuid", http.NoBody)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("esperado 400, obtenido %d", rec.Code)
		}
	})

	t.Run("UUID inválido en /sites/{id}/versions/{vid}/restore", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/sites/00000000-0000-0000-0000-000000000001/versions/bad-uuid/restore", http.NoBody)
		req = req.WithContext(tenancy.Into(req.Context(), tenancy.Tenant{Slug: "acme"}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("esperado 400, obtenido %d", rec.Code)
		}
	})
}

// Sin organización en el contexto no hay búsqueda: el 401 sale antes de tocar la base.
func TestBuscarSitiosSinOrganizacion(t *testing.T) {
	_, h := routerSitiosTest()
	rec := httptest.NewRecorder()
	h.buscarSitios(rec, httptest.NewRequest(http.MethodGet, "/search?q=aurora", http.NoBody))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obtenido %d", rec.Code)
	}
}
