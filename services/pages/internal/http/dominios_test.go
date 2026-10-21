// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/dominios"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

func routerDominiosTest() (*chi.Mux, *Handlers) {
	st := store.New(nil)
	cfg := config.Config{
		PublicURL: "https://web.sytue.localhost",
		Sitios: config.Sitios{
			SitesDomain: "sites.sytue.localhost",
		},
		IPApex: "127.0.0.1",
	}

	domSvc := dominios.Nuevo(st, cfg, nil, nil, nil)
	verifier := &authn.Verifier{}

	h := &Handlers{
		Deps: Deps{
			Store:    st,
			Cfg:      cfg,
			Dominios: domSvc,
			Verifier: verifier,
		},
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			claims := authn.Claims{
				Subject: "sub-123",
				Tenant:  "acme",
				Roles:   []string{"owner"},
				Plan:    "pro",
			}
			ctx := authn.Into(req.Context(), claims)
			ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"})
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})

	g := grupos{
		api: r,
	}
	montarDominios(g, h)
	return r, h
}

func TestDominiosHTTP(t *testing.T) {
	r, _ := routerDominiosTest()
	siteID := uuid.New()

	t.Run("GET /sites/{id}/domains devuelve lista", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/sites/"+siteID.String()+"/domains", http.NoBody)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		// Dado que el store es nil (sin postgres real), devuelve lista vacía o error controlado
		if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
			t.Errorf("esperado 200 o 404, obtenido %d", rec.Code)
		}
	})

	t.Run("POST /sites/{id}/domains con JSON inválido", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/domains", bytes.NewReader([]byte("{invalid-json")))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("esperado 400 por JSON inválido, obtenido %d", rec.Code)
		}
	})

	t.Run("POST /sites/{id}/domains/{did}/check con did inválido", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/domains/not-a-uuid/check", http.NoBody)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("esperado 400 por did inválido, obtenido %d", rec.Code)
		}
	})

	t.Run("DELETE /sites/{id}/domains/{did} con did inválido", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/sites/"+siteID.String()+"/domains/not-a-uuid", http.NoBody)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("esperado 400 por did inválido, obtenido %d", rec.Code)
		}
	})

	t.Run("POST /sites/{id}/domains con dominio reservado de la plataforma", func(t *testing.T) {
		body, _ := json.Marshal(crearDominioPeticion{Host: "sytue.eu"})
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/domains", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("esperado 422 domain_reserved, obtenido %d", rec.Code)
		}
	})
}
