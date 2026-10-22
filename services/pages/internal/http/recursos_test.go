// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestRecursosHTTP_Validacion(t *testing.T) {
	h := &Handlers{}
	r := chi.NewRouter()
	siteID := uuid.New()
	r.Route("/sites/{id}", func(sr chi.Router) {
		sr.Get("/assets", h.listarRecursos)
		sr.Post("/assets", h.subirRecurso)
		sr.Delete("/assets/{aid}", h.borrarRecurso)
	})

	t.Run("subida_vacia_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/assets", bytes.NewReader(nil))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("code = %d; want 400", w.Code)
		}
	})

	t.Run("tipo_no_soportado_422", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/assets", bytes.NewReader([]byte("not an image or pdf")))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("code = %d; want 422", w.Code)
		}
	})

	t.Run("imagen_corrupta_422", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/assets", bytes.NewReader([]byte("\x89PNG\r\n\x1a\ncorruptdata")))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("code = %d; want 422", w.Code)
		}
	})

	t.Run("id_sitio_invalido_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/sites/no-uuid/assets", http.NoBody)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("code = %d; want 400", w.Code)
		}
	})

	t.Run("id_asset_invalido_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/sites/"+siteID.String()+"/assets/no-uuid", http.NoBody)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("code = %d; want 400", w.Code)
		}
	})

	t.Run("reconoce_png_valido", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 10, 10))
		img.Set(0, 0, color.RGBA{R: 255, A: 255})
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		if !esPNG(buf.Bytes()) {
			t.Fatalf("debe reconocer magic bytes de PNG")
		}
	})
}
