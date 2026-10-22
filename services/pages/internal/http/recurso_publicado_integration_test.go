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
	"strings"
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

// WEB-04, con la decisión del dueño («borrar de verdad, avisando»): un recurso que usa la
// versión publicada no se borra sin confirmarlo —409 `asset_published`, y sigue en su
// sitio—, porque se dejará de ver en la web. Con ?confirm=published se borra al momento,
// el maestro y sus anchos.
func TestBorrarUnRecursoPublicadoPideConfirmacion(t *testing.T) {
	pg := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	var sitio uuid.UUID
	if err := pg.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO sites (tenant, slug, name, draft, created_by)
			VALUES ('acme', 'publicada', 'Publicada', '{}', 'sub') RETURNING id`).Scan(&sitio)
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
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1200, 10))); err != nil {
		t.Fatal(err)
	}
	subida := pedir(http.MethodPost, "/sites/"+sitio.String()+"/assets", b.Bytes())
	var foto domain.Recurso
	if subida.Code != http.StatusCreated || json.Unmarshal(subida.Body.Bytes(), &foto) != nil {
		t.Fatalf("subir: %d %s", subida.Code, subida.Body.String())
	}
	// La versión en vivo la usa; el borrador ya no.
	if err := pg.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var version uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO site_versions (tenant, site_id, kind, ir, draft_rev, created_by)
			VALUES ('acme', $1, 'publish', jsonb_build_object('imagen', $2::text), 1, 'sub') RETURNING id`,
			sitio, foto.File+"."+foto.Ext).Scan(&version); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO publications (tenant, site_id, kind, number, version_id, files, bytes, renderer_version, created_by)
			VALUES ('acme', $1, 'live', 1, $2, 1, 1, 'test', 'sub')`, sitio, version); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE sites SET published_number = 1 WHERE id = $1`, sitio)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	quedan := func() int {
		n := 0
		for _, nombre := range foto.Ficheros() {
			if len(almacen.Leer("shared/sitios/"+sitio.String()+"/recursos/"+nombre)) > 0 {
				n++
			}
		}
		return n
	}

	ruta := "/sites/" + sitio.String() + "/assets/" + foto.ID.String()
	sin := pedir(http.MethodDelete, ruta, nil)
	if sin.Code != http.StatusConflict || !strings.Contains(sin.Body.String(), "asset_published") || quedan() != len(foto.Ficheros()) {
		t.Fatalf("borrar sin confirmar lo que usa la web publicada: %d %s, quedan %d; quiero 409 asset_published y todo en su sitio",
			sin.Code, sin.Body.String(), quedan())
	}
	if con := pedir(http.MethodDelete, ruta+"?confirm=published", nil); con.Code != http.StatusNoContent || quedan() != 0 {
		t.Fatalf("borrar confirmado: %d %s, quedan %d; quiero 204 y nada en el almacén", con.Code, con.Body.String(), quedan())
	}
}
