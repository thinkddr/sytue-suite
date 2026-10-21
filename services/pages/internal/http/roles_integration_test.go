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
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
	"git.sytue.eu/sytue/one/services/pages/internal/tienda"
)

// WEB-02. Un miembro sin rol de administración edita el sitio, pero no decide lo que la
// organización firma o cobra: no cambia su dirección ni sus datos legales, no lo borra ni lo
// devuelve, no abre ni cierra la tienda ni cambia sus condiciones, y no toca la pasarela de
// cobro. Antes podía todo eso.
func TestUnMiembroNoDecideLaDireccionLosDatosLegalesNiElCobro(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	st := store.New(h.Pool)
	sitio, err := st.CrearSitio(ctx, "acme", domain.CrearSitioInput{Name: "Aurora", Slug: "aurora"}, "sub-123")
	if err != nil {
		t.Fatal(err)
	}
	kc, err := keycrypto.New([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	hd := &Handlers{Deps: Deps{Store: st, Tienda: tienda.Nuevo(st, kc, "https://web.example.test", "sites.example.test", nil, nil)}}
	r := chi.NewRouter()
	g := grupos{api: r}
	montarSitios(g, hd)
	montarTienda(g, hd)

	pedir := func(rol, metodo, ruta, cuerpo string) int {
		t.Helper()
		req := conAutenticacion(httptest.NewRequest(metodo, "/sites/"+sitio.ID.String()+ruta, strings.NewReader(cuerpo)), []string{rol}, "pro")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}
	estado := func() (nombre, slug, titular string, borrado bool, pasarelas int) {
		t.Helper()
		if err := h.Pool.InTenantTx(tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"}), func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT name, slug, coalesce(legal->>'holder_name', ''), deleted_at IS NOT NULL,
				(SELECT count(*) FROM gateways g WHERE g.site_id = sites.id AND g.tenant = sites.tenant AND g.active)
				FROM sites WHERE id = $1 AND tenant = 'acme'`, sitio.ID).Scan(&nombre, &slug, &titular, &borrado, &pasarelas)
		}); err != nil {
			t.Fatal(err)
		}
		return
	}

	legal := `{"legal":{"holder_name":"Otra S.L.","tax_id":"B00000000","country":"ES"}}`
	pasarela := `{"provider":"mollie","mode":"test","credentials":{"api_key":"test_abcdefghijklmnopqrstuvwxyz1234"}}`

	// Lo que un miembro SÍ hace: el nombre, y guardar Ajustes con el mismo slug.
	if c := pedir("member", http.MethodPut, "", `{"name":"Aurora 2","slug":"aurora","analytics_enabled":false}`); c != http.StatusOK {
		t.Fatalf("un miembro no puede guardar el nombre: %d", c)
	}
	// Lo que no.
	for _, p := range []struct{ metodo, ruta, cuerpo string }{
		{http.MethodPut, "", `{"slug":"otra-direccion"}`},
		{http.MethodPut, "", legal},
		{http.MethodDelete, "", ""},
		{http.MethodPost, "/restore", ""},
		{http.MethodPost, "/store/gateways", pasarela},
		{http.MethodDelete, "/store/gateways/mollie", ""},
		{http.MethodPut, "/store/settings", `{"reply_to":"otro@example.test"}`},
		{http.MethodPost, "/store/enable", ""},
		{http.MethodPost, "/store/disable", ""},
	} {
		if c := pedir("member", p.metodo, p.ruta, p.cuerpo); c != http.StatusForbidden {
			t.Errorf("miembro: %s %s %s = %d; want 403", p.metodo, p.ruta, p.cuerpo, c)
		}
	}
	if nombre, slug, titular, borrado, pasarelas := estado(); nombre != "Aurora 2" || slug != "aurora" || titular != "" || borrado || pasarelas != 0 {
		t.Fatalf("un miembro cambió lo que no debía: nombre=%q slug=%q titular=%q borrado=%v pasarelas=%d", nombre, slug, titular, borrado, pasarelas)
	}

	// Quien administra, sí.
	for _, p := range []struct{ metodo, ruta, cuerpo string }{
		{http.MethodPut, "", `{"slug":"otra-direccion"}`},
		{http.MethodPut, "", legal},
		{http.MethodPost, "/store/gateways", pasarela},
	} {
		if c := pedir("admin", p.metodo, p.ruta, p.cuerpo); c != http.StatusOK {
			t.Errorf("admin: %s %s = %d; want 200", p.metodo, p.ruta, c)
		}
	}
	if _, slug, titular, _, pasarelas := estado(); slug != "otra-direccion" || titular != "Otra S.L." || pasarelas != 1 {
		t.Fatalf("quien administra no pudo: slug=%q titular=%q pasarelas=%d", slug, titular, pasarelas)
	}
	if c := pedir("owner", http.MethodDelete, "", ""); c != http.StatusOK {
		t.Fatalf("el propietario no puede borrar el sitio: %d", c)
	}
	if c := pedir("owner", http.MethodPost, "/restore", ""); c != http.StatusOK {
		t.Fatalf("el propietario no puede restaurar el sitio: %d", c)
	}
}
