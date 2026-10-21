// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package pageshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
	"git.sytue.eu/sytue/one/services/pages/internal/testutil"
)

// WEB-04. Borrar un recurso lo borra del almacén: el maestro y cada ancho, que
// `pages sitios` sirve en `/_astro/<nombre>` sin mirar la tabla. Si el almacén no
// puede borrarlos, la fila se queda para repetir el borrado.
func TestBorrarUnRecursoLoQuitaDelAlmacen(t *testing.T) {
	pg := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	var sitio uuid.UUID
	if err := pg.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO sites (tenant, slug, name, draft, created_by)
			VALUES ('acme', 'recursos', 'Recursos', '{}', 'sub') RETURNING id`).Scan(&sitio)
	}); err != nil {
		t.Fatal(err)
	}
	almacen := testutil.NuevoAlmacen(t)
	h := &Handlers{Deps: Deps{Store: store.New(pg.Pool), Objetos: almacen.Objetos}}
	r := chi.NewRouter()
	r.Post("/sites/{id}/assets", h.subirRecurso)
	r.Delete("/sites/{id}/assets/{aid}", h.borrarRecurso)
	pedir := func(metodo, ruta string, cuerpo []byte) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(metodo, ruta, bytes.NewReader(cuerpo)).WithContext(ctx))
		return rec
	}
	subir := func(ancho int) domain.Recurso {
		t.Helper()
		var b bytes.Buffer
		if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, ancho, 10))); err != nil {
			t.Fatal(err)
		}
		rec := pedir(http.MethodPost, "/sites/"+sitio.String()+"/assets", b.Bytes())
		var out domain.Recurso
		if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
			t.Fatalf("subir: %d %s", rec.Code, rec.Body.String())
		}
		return out
	}
	enAlmacen := func(rec domain.Recurso) []string {
		var quedan []string
		for _, nombre := range rec.Ficheros() {
			if len(almacen.Leer("shared/sitios/"+sitio.String()+"/recursos/"+nombre)) > 0 {
				quedan = append(quedan, nombre)
			}
		}
		return quedan
	}

	foto := subir(1200)
	if len(foto.Variants) != 2 || len(enAlmacen(foto)) != 3 {
		t.Fatalf("subida: anchos %v y en el almacén %v; quiero el maestro y dos anchos", foto.Variants, enAlmacen(foto))
	}
	if rec := pedir(http.MethodDelete, "/sites/"+sitio.String()+"/assets/"+foto.ID.String(), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("borrar: %d %s", rec.Code, rec.Body.String())
	}
	if quedan := enAlmacen(foto); len(quedan) > 0 {
		t.Fatalf("tras borrar el recurso siguen en el almacén, y en su URL pública: %v", quedan)
	}

	// Con el almacén caído no se borra nada, y la fila sigue para repetirlo.
	otra := subir(600)
	almacen.Fallar(true)
	if rec := pedir(http.MethodDelete, "/sites/"+sitio.String()+"/assets/"+otra.ID.String(), nil); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("borrar con el almacén caído: %d %s; quiero 503", rec.Code, rec.Body.String())
	}
	almacen.Fallar(false)
	if _, err := h.Store.RecursoPorID(ctx, sitio, otra.ID); err != nil {
		t.Fatalf("la fila se fue aunque sus ficheros siguen en el almacén: %v", err)
	}
	if rec := pedir(http.MethodDelete, "/sites/"+sitio.String()+"/assets/"+otra.ID.String(), nil); rec.Code != http.StatusNoContent || len(enAlmacen(otra)) > 0 {
		t.Fatalf("repetir el borrado: %d, quedan %v", rec.Code, enAlmacen(otra))
	}
}
