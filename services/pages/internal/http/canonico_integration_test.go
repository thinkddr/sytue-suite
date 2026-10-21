// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package pageshttp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/mapa"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// almacenEnMemoria guarda lo que escribe el mapa para que el lector lo lea.
type almacenEnMemoria struct {
	mu      sync.Mutex
	objetos map[string][]byte
}

func (a *almacenEnMemoria) Put(_ context.Context, _ tenancy.Tenant, clave string, r io.Reader, _ int64, _ objectstore.PutOptions) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.objetos == nil {
		a.objetos = map[string][]byte{}
	}
	a.objetos[clave] = b
	return nil
}

func (a *almacenEnMemoria) Get(_ context.Context, _ tenancy.Tenant, clave string) (io.ReadCloser, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.objetos[clave]
	if !ok {
		return nil, objectstore.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

// WEB-01, de punta a punta. El cuerpo de PUT /sites/{id} ya no puede traer el canónico
// (ni el identificador de analítica): ni se guarda ni se acepta. Y un canónico de texto
// libre que quedara en la base no mete su host en el mapa, así que el `ask` de los
// certificados no lo autoriza.
func TestAjustesNoAdmitenCanonicoDelNavegador(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	super := h.PoolSinRLS(t)
	// Con el rol que elude la RLS, como producción: aquí no puede sostener nada la política.
	st := store.New(super)
	ctx := context.Background()

	victima, err := st.CrearSitio(ctx, "victima", domain.CrearSitioInput{Name: "Víctima", Slug: "victima"}, "sub-v")
	if err != nil {
		t.Fatal(err)
	}
	atacante, err := st.CrearSitio(ctx, "acme", domain.CrearSitioInput{Name: "Atacante", Slug: "atacante"}, "sub")
	if err != nil {
		t.Fatal(err)
	}

	obj := &almacenEnMemoria{}
	lector := mapa.NuevoLector(obj, time.Hour)
	hd := &Handlers{Deps: Deps{Store: st, Mapa: mapa.NuevoEscritor(st, obj, "sites.example.test"), Lector: lector}}
	r := chi.NewRouter()
	r.Put("/sites/{id}", hd.actualizarSitio)
	montarBorde(grupos{borde: r}, hd)

	poner := func(cuerpo string) *httptest.ResponseRecorder {
		t.Helper()
		req := conClaims(httptest.NewRequest(http.MethodPut, "/sites/"+atacante.ID.String(), strings.NewReader(cuerpo)), []string{"owner"}, "sub")
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(tenancy.Into(req.Context(), tenancy.Tenant{Slug: "acme"}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	leer := func() (nombre string, canonico, analitica *string) {
		t.Helper()
		if err := super.QueryRow(ctx, `SELECT name, canonical_host, NULLIF(analytics_site_id, '') FROM sites WHERE id = $1`, atacante.ID).
			Scan(&nombre, &canonico, &analitica); err != nil {
			t.Fatal(err)
		}
		return nombre, canonico, analitica
	}

	for _, cuerpo := range []string{
		`{"name":"Otro","canonical_host":"victima.sites.example.test"}`,
		`{"name":"Otro","canonical_host":null}`,
		`{"name":"Otro","analytics_site_id":"00000000-0000-0000-0000-000000000001"}`,
	} {
		if rec := poner(cuerpo); rec.Code != http.StatusBadRequest {
			t.Fatalf("PUT %s = %d; want 400", cuerpo, rec.Code)
		}
		if nombre, canonico, analitica := leer(); nombre != "Atacante" || canonico != nil || analitica != nil {
			t.Fatalf("PUT %s cambió el sitio: nombre=%q canónico=%v analítica=%v", cuerpo, nombre, canonico, analitica)
		}
	}
	if rec := poner(`{"name":"Otro"}`); rec.Code != http.StatusOK {
		t.Fatalf("un ajuste corriente = %d", rec.Code)
	}

	// Lo que dejaba el PUT antiguo, puesto a mano en la base.
	if _, err := super.Exec(ctx, `UPDATE sites SET canonical_host = 'nadie.example' WHERE id = $1`, atacante.ID); err != nil {
		t.Fatal(err)
	}
	if err := hd.Mapa.Regenerar(ctx); err != nil {
		t.Fatal(err)
	}
	if err := lector.Refrescar(ctx); err != nil {
		t.Fatal(err)
	}
	if id, _, ok := lector.ObtenerPorHost("victima.sites.example.test"); !ok || id != victima.ID.String() {
		t.Fatalf("el host de la víctima apunta a %s", id)
	}
	preguntar := func(host string) int {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_borde/permitido?domain="+host, http.NoBody))
		return rec.Code
	}
	if code := preguntar("nadie.example"); code != http.StatusNotFound {
		t.Fatalf("el ask autoriza un host por el canónico de un sitio: %d", code)
	}
	if code := preguntar("atacante.sites.example.test"); code != http.StatusOK {
		t.Fatalf("control: el ask no reconoce un host del mapa: %d", code)
	}
}
