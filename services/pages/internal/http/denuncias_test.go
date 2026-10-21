// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/mapa"
)

func routerDenunciasTest() *chi.Mux {
	lector := mapa.NuevoLector(nil, 10*time.Second)
	_ = lector.Refrescar(context.Background())

	h := &Handlers{
		Deps: Deps{
			Lector: lector,
		},
	}

	r := chi.NewRouter()
	g := grupos{
		publico: r,
	}
	montarDenuncias(g, h)
	return r
}

func TestDenunciasAbuso(t *testing.T) {
	r := routerDenunciasTest()

	t.Run("Sin Origin o mismatch devuelve 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/reports", bytes.NewReader([]byte("{}")))
		req.Host = "panaderia-aurora.sites.sytue.eu"
		// Sin Origin
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("esperado 403, obtenido %d", rec.Code)
		}

		// Con Origin diferente
		req = httptest.NewRequest(http.MethodPost, "/reports", bytes.NewReader([]byte("{}")))
		req.Host = "panaderia-aurora.sites.sytue.eu"
		req.Header.Set("Origin", "https://otro-sitio.com")
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("esperado 403, obtenido %d", rec.Code)
		}
	})

	t.Run("Sitio no disponible en mapa devuelve 503", func(t *testing.T) {
		body, _ := json.Marshal(denunciaPeticion{
			Reason:  string(domain.AbuseReasonPhishing),
			Details: "Intento de suplantación",
		})
		req := httptest.NewRequest(http.MethodPost, "/reports", bytes.NewReader(body))
		req.Host = "sitio-fantasma.sites.sytue.eu"
		req.Header.Set("Origin", "https://sitio-fantasma.sites.sytue.eu")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("esperado 503, obtenido %d", rec.Code)
		}
	})
}
