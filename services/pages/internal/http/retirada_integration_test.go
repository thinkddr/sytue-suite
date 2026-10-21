// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package pageshttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/mapa"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

type almacenRetirada struct {
	fallar     bool
	escrituras int
}

func (a *almacenRetirada) Put(context.Context, tenancy.Tenant, string, io.Reader, int64, objectstore.PutOptions) error {
	a.escrituras++
	if a.fallar {
		return errors.New("almacén no disponible")
	}
	return nil
}

func (*almacenRetirada) Get(context.Context, tenancy.Tenant, string) (io.ReadCloser, error) {
	return nil, objectstore.ErrNotFound
}

// Revocar en PostgreSQL no basta para anunciar retirada: el servidor público lee
// el mapa S3. Un fallo al escribirlo debe permitir reintentar y no responder200.
func TestRetiradaExigeActualizarMapa(t *testing.T) {
	pg := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	var id uuid.UUID
	if err := pg.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO sites (tenant,slug,name,draft,created_by,published_number) VALUES ('acme','retirada','Retirada','{}','sub',1) RETURNING id`).Scan(&id)
	}); err != nil {
		t.Fatal(err)
	}
	st := store.New(pg.Pool)
	obj := &almacenRetirada{fallar: true}
	h := &Handlers{Deps: Deps{Store: st, Mapa: mapa.NuevoEscritor(st, obj, "sites.example.test")}}
	r := chi.NewRouter()
	r.Post("/sites/{id}/unpublish", h.despublicarSitio)
	pedir := func(roles []string) *httptest.ResponseRecorder {
		t.Helper()
		req := conClaims(httptest.NewRequest(http.MethodPost, "/sites/"+id.String()+"/unpublish", http.NoBody), roles, "sub")
		req = req.WithContext(tenancy.Into(req.Context(), tenancy.Tenant{Slug: "acme"}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	if rec := pedir([]string{"member"}); rec.Code != http.StatusForbidden {
		t.Fatalf("member=%d", rec.Code)
	}
	site, err := st.SitioCompleto(ctx, id)
	if err != nil || site.PublishedNumber == nil || obj.escrituras != 0 {
		t.Fatal("member retiró la web")
	}
	if rec := pedir([]string{"admin"}); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("fallo mapa=%d: %s", rec.Code, rec.Body.String())
	}
	site, err = st.SitioCompleto(ctx, id)
	if err != nil || site.PublishedNumber != nil {
		t.Fatal("la revocación DB no se confirmó")
	}
	obj.fallar = false
	if rec := pedir([]string{"admin"}); rec.Code != http.StatusOK {
		t.Fatalf("reintento=%d: %s", rec.Code, rec.Body.String())
	}
	if obj.escrituras != 2 {
		t.Fatalf("escrituras=%d", obj.escrituras)
	}
}
