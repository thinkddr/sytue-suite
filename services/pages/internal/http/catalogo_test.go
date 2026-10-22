// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/services/pages/internal/catalogo"
	"git.sytue.eu/sytue/one/services/pages/internal/plantillas"
)

func routerCatalogoTest(t *testing.T) (*chi.Mux, *Handlers) {
	cat, err := catalogo.CargarPorDefecto()
	if err != nil {
		t.Fatalf("cargar catalogo: %v", err)
	}
	h := &Handlers{
		Deps: Deps{
			Catalogo: cat,
		},
	}
	r := chi.NewRouter()
	r.Route("/catalog", func(cr chi.Router) {
		cr.Get("/fonts", h.catalogoFuentes)
		cr.Get("/library", h.catalogoBiblioteca)
		cr.Get("/themes", h.catalogoTemas)
		cr.Get("/files/{nombre}", h.catalogoFichero)
	})
	return r, h
}

func TestCatalogoHTTP(t *testing.T) {
	r, _ := routerCatalogoTest(t)

	t.Run("fonts", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/catalog/fonts", http.NoBody)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("code = %d; want 200", w.Code)
		}
		var familias []catalogo.Familia
		if err := json.Unmarshal(w.Body.Bytes(), &familias); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(familias) != 12 {
			t.Fatalf("familias = %d; want 12", len(familias))
		}
	})

	t.Run("library", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/catalog/library", http.NoBody)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("code = %d; want 200", w.Code)
		}
		var fotos []catalogo.Foto
		if err := json.Unmarshal(w.Body.Bytes(), &fotos); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(fotos) == 0 {
			t.Fatalf("fotos vacías; want > 0")
		}
	})

	t.Run("themes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/catalog/themes", http.NoBody)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("code = %d; want 200", w.Code)
		}
		var tc plantillas.TemasCatalogo
		if err := json.Unmarshal(w.Body.Bytes(), &tc); err != nil {
			t.Fatalf("unmarshal themes: %v", err)
		}
		if len(tc.Palettes) < 4 {
			t.Fatalf("paletas = %d; want >= 4", len(tc.Palettes))
		}
		if len(tc.FontPairs) < 4 {
			t.Fatalf("parejas = %d; want >= 4", len(tc.FontPairs))
		}
	})

	t.Run("file_traversal_rechazado", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/catalog/files/..%2F..%2Fetc%2Fpasswd", http.NoBody)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
			t.Fatalf("code = %d; want 400 or 404", w.Code)
		}
	})
}
