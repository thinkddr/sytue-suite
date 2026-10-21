// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package pageshttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// /_pages resuelve también la dirección propia de cada web, <slug>.sites.<dominio>, que
// el IngressRoute ya le manda: antes solo resolvía dominios propios, y una web sin
// dominio no podía usar ni la tienda, ni los formularios, ni las denuncias (404
// site_not_found). Solo el slug exacto de un sitio vivo, y solo bajo el dominio de webs.
func TestPagesResuelveLaDireccionPorSlug(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	if _, err := super.Exec(ctx, `INSERT INTO sites (tenant, slug, name, draft, created_by) VALUES
		('org', 'tienda', 'Web', '{}', 'sub'), ('org', 'parada', 'Web', '{}', 'sub')`); err != nil {
		t.Fatal(err)
	}
	if _, err := super.Exec(ctx, `UPDATE sites SET status = 'suspended' WHERE slug = 'parada'`); err != nil {
		t.Fatal(err)
	}
	st := store.New(super)
	r := chi.NewRouter()
	hd := &Handlers{Deps: Deps{Store: st, Cfg: config.Config{SitesDomain: "sites.example.test"}}}
	montarPublicoTienda(grupos{publico: r}, hd)
	// Una ruta que solo contesta si el middleware de /_pages encontró el sitio.
	r.Get("/eco", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	for host, quiero := range map[string]int{
		"tienda.sites.example.test":      http.StatusOK,
		"parada.sites.example.test":      http.StatusNotFound, // suspendida
		"x.tienda.sites.example.test":    http.StatusNotFound,
		"tienda.sites.otro.example.test": http.StatusNotFound,
		"sites.example.test":             http.StatusNotFound,
	} {
		req := httptest.NewRequest(http.MethodGet, "/eco", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != quiero {
			t.Errorf("%s: %d %s; quiero %d", host, rec.Code, rec.Body.String(), quiero)
		}
	}
}
