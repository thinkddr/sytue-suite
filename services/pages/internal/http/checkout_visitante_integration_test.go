// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package pageshttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
	"git.sytue.eu/sytue/one/services/pages/internal/tienda"
)

// tiendaPublica siembra una tienda con un dominio activo, una variante con existencias
// y una pasarela cuyas credenciales no se pueden descifrar, y monta /_pages sobre un
// pool que elude la RLS, como producción.
func tiendaPublica(t *testing.T) (http.Handler, *pg.Pool, uuid.UUID) {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	var sitio, producto uuid.UUID
	if err := super.QueryRow(ctx, `INSERT INTO sites (tenant, slug, name, draft, created_by)
		VALUES ('org', 'tienda', 'Tienda', '{}', 'sub') RETURNING id`).Scan(&sitio); err != nil {
		t.Fatal(err)
	}
	if err := super.QueryRow(ctx, `INSERT INTO products (tenant, site_id, slug, name, description, vat_rate, kind, max_per_order, active)
		VALUES ('org', $1, '{"es":"pan"}', '{"es":"Pan"}', '{}', 10, 'general', 10, true) RETURNING id`, sitio).Scan(&producto); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`INSERT INTO domains (tenant, site_id, host, token, status, created_by)
			VALUES ('org', $1, 'tienda.example.test', '0123456789abcdef0123456789abcdef', 'active', 'sub')`,
		`INSERT INTO store_settings (site_id, tenant, enabled, reply_to) VALUES ($1, 'org', true, 'tienda@example.test')`,
		`INSERT INTO gateways (tenant, site_id, provider, mode, credentials_enc, checked_at, created_by)
			VALUES ('org', $1, 'mollie', 'test', 'encv1:no-descifra', now(), 'sub')`,
	} {
		if _, err := super.Exec(ctx, sql, sitio); err != nil {
			t.Fatalf("sembrar: %v\n%s", err, sql)
		}
	}
	if _, err := super.Exec(ctx, `INSERT INTO variants (tenant, site_id, product_id, sku, options, price_cents, stock, reserved)
		VALUES ('org', $1, $2, 'PAN-1', '{}', 250, 10, 0)`, sitio, producto); err != nil {
		t.Fatal(err)
	}
	kc, err := keycrypto.New([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(super)
	r := chi.NewRouter()
	montarPublicoTienda(grupos{publico: r}, &Handlers{Deps: Deps{
		Store:  st,
		Tienda: tienda.Nuevo(st, kc, "https://web.example.test", "sites.example.test", nil, nil),
	}})
	return r, super, sitio
}

func checkout(r http.Handler, xff, cuerpo string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader(cuerpo))
	req.Host = "tienda.example.test"
	req.RemoteAddr = "10.42.0.7:41000" // Traefik, dentro del clúster
	req.Header.Set("X-Forwarded-For", xff)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// WEB-06. Desde una IP de verdad, el sexto checkout seguido de esa IP en ese sitio es un
// 429. Detrás del túnel del piloto, donde toda petición trae la IP de un proxy nuestro, el
// cubo no se aplica: sería un tope para todo el mundo a la vez.
func TestElCheckoutSeLimitaPorVisitanteSoloConSuIPReal(t *testing.T) {
	r, _, _ := tiendaPublica(t)
	for i := 1; i <= 8; i++ {
		if rec := checkout(r, "127.0.0.1", `{}`); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("checkout %d por el túnel: 429; el cubo por IP no puede aplicarse a la IP de un proxy", i)
		}
	}
	for i := 1; i <= 5; i++ {
		if rec := checkout(r, "203.0.113.9", `{}`); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("checkout %d desde una IP pública: 429 antes de tiempo", i)
		}
	}
	if rec := checkout(r, "203.0.113.9", `{}`); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("sexto checkout seguido desde la misma IP: %d %s; quiero 429", rec.Code, rec.Body.String())
	}
	if rec := checkout(r, "198.51.100.4", `{}`); rec.Code == http.StatusTooManyRequests {
		t.Fatal("otra IP pública paga el cubo de la primera")
	}
}

// WEB-06. Si la pasarela no llega a abrir el pago, el pedido recién creado no se queda
// 35 minutos ocupando un hueco de la tienda con sus existencias reservadas.
func TestUnCheckoutQueNoAbrePagoNoReservaNada(t *testing.T) {
	r, super, sitio := tiendaPublica(t)
	cuerpo := `{"checkout_token":"` + uuid.NewString() + `","items":[{"sku":"PAN-1","qty":2}],
		"buyer":{"email":"comprador@example.test","name":"Comprador"},"shipping":{"method_id":"x"},
		"payment":{"gateway":"mollie"},"terms_version":1,"language":"es"}`
	if rec := checkout(r, "127.0.0.1", cuerpo); rec.Code < 400 {
		t.Fatalf("checkout con una pasarela que no abre el pago: %d %s", rec.Code, rec.Body.String())
	}
	ctx := context.Background()
	var estado string
	if err := super.QueryRow(ctx, `SELECT status FROM orders WHERE site_id = $1`, sitio).Scan(&estado); err != nil {
		t.Fatalf("el pedido llegó a crearse antes de la pasarela: %v", err)
	}
	var reservadas int
	if err := super.QueryRow(ctx, `SELECT reserved FROM variants WHERE site_id = $1`, sitio).Scan(&reservadas); err != nil {
		t.Fatal(err)
	}
	if estado != "expired" || reservadas != 0 {
		t.Fatalf("pedido %q con %d unidades reservadas; quiero expired y 0", estado, reservadas)
	}
}
