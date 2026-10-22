// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package pageshttp

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/ia"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
	"git.sytue.eu/sytue/one/services/pages/internal/tienda"
)

// WEB-14, por la API. Toda ruta de `/sites/{id}/store/*` y de `/sites/{id}/ia/*` con el
// UUID del sitio de OTRA organización contesta 404 y no toca nada, también cuando la base
// no evalúa ninguna política (el pool es de un rol que elude la RLS, como producción).
func TestLaTiendaDeOtraOrganizacionNoExistePorLaAPI(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)

	var sitio, producto uuid.UUID
	if err := super.QueryRow(ctx, `INSERT INTO sites (tenant, slug, name, draft, created_by)
		VALUES ('victima', 'tienda-victima', 'Tienda', '{}', 'sub-v') RETURNING id`).Scan(&sitio); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`INSERT INTO store_settings (site_id, tenant, enabled, reply_to) VALUES ($1, 'victima', true, 'tienda@victima.example')`,
		`INSERT INTO gateways (tenant, site_id, provider, mode, credentials_enc, checked_at, created_by)
			VALUES ('victima', $1, 'mollie', 'test', 'encv1:de-la-victima', now(), 'sub-v')`,
		`INSERT INTO orders (tenant, site_id, number, test, checkout_token, access_token_hash, status,
				subtotal_cents, shipping_cents, total_cents, vat_cents, vat_breakdown, buyer_email, buyer_name,
				shipping_method, language, terms_version, terms_accepted_at, reserved_until)
			VALUES ('victima', $1, 'P-000001', true, gen_random_uuid(), decode('00','hex'), 'paid',
				1000, 0, 1000, 0, '{}', 'comprador@example.test', 'Comprador', '{}', 'es', 1, now(), now())`,
	} {
		if _, err := super.Exec(ctx, sql, sitio); err != nil {
			t.Fatalf("sembrar: %v", err)
		}
	}
	if err := super.QueryRow(ctx, `INSERT INTO products (tenant, site_id, slug, name, description, vat_rate, kind, max_per_order, active)
		VALUES ('victima', $1, '{}', '{"es":"Pan"}', '{}', 10, 'general', 10, true) RETURNING id`, sitio).Scan(&producto); err != nil {
		t.Fatal(err)
	}

	kc, err := keycrypto.New([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(super)
	hd := &Handlers{Deps: Deps{
		Store:  st,
		Tienda: tienda.Nuevo(st, kc, "https://web.example.test", "sites.example.test", nil, nil),
		IA:     ia.Nuevo(config.Config{}, st, nil, kc, nil, nil, nil, slog.Default()),
	}}
	r := chi.NewRouter()
	g := grupos{api: r}
	montarTienda(g, hd)
	montarIA(g, hd)

	base := "/sites/" + sitio.String()
	pasarela := `{"provider":"mollie","mode":"test","credentials":{"api_key":"test_del_atacante"}}`
	for _, p := range []struct{ metodo, ruta, cuerpo string }{
		{http.MethodGet, "/store/settings", ""},
		{http.MethodPut, "/store/settings", `{"reply_to":"yo@atacante.example","countries":["ES"]}`},
		{http.MethodPost, "/store/enable", ""},
		{http.MethodPost, "/store/disable", ""},
		{http.MethodGet, "/store/products", ""},
		{http.MethodPost, "/store/products", `{"name":{"es":"Colado"},"vat_rate":21,"variants":[{"sku":"X","price_cents":1}]}`},
		{http.MethodGet, "/store/products/" + producto.String(), ""},
		{http.MethodPut, "/store/products/" + producto.String(), `{"name":{"es":"Cambiado"},"vat_rate":21}`},
		{http.MethodDelete, "/store/products/" + producto.String(), ""},
		{http.MethodGet, "/store/gateways", ""},
		{http.MethodPost, "/store/gateways", pasarela},
		{http.MethodDelete, "/store/gateways/mollie", ""},
		{http.MethodGet, "/store/orders", ""},
		{http.MethodGet, "/store/orders.csv", ""},
		{http.MethodPost, "/ia/design", `{"prompt":"una portada"}`},
		{http.MethodPost, "/ia/translate", `{"from":"es","to":["en"]}`},
		{http.MethodPost, "/ia/export", `{"destination":"woocommerce"}`},
	} {
		// `conAutenticacion` es la organización «acme», propietaria en la SUYA.
		req := conAutenticacion(httptest.NewRequest(p.metodo, base+p.ruta, strings.NewReader(p.cuerpo)), []string{"owner"}, "pro")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d; want 404 (cuerpo: %.200s)", p.metodo, p.ruta, rec.Code, rec.Body.String())
		}
	}

	var huella string
	if err := super.QueryRow(ctx, `
		SELECT concat_ws('|',
			(SELECT enabled::text || reply_to FROM store_settings WHERE site_id = $1),
			(SELECT string_agg(name::text, ',') FROM products WHERE site_id = $1),
			(SELECT string_agg(credentials_enc || ':' || active, ',') FROM gateways WHERE site_id = $1),
			(SELECT count(*) FROM ai_jobs WHERE site_id = $1))`, sitio).Scan(&huella); err != nil {
		t.Fatal(err)
	}
	if want := `truetienda@victima.example|{"es": "Pan"}|encv1:de-la-victima:true|0`; huella != want {
		t.Fatalf("otra organización cambió la tienda:\n got  %s\n want %s", huella, want)
	}
}
