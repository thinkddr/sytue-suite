// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package odata_test

// Tests OFFLINE del driver OData v4 contra un httptest (sin red ni SaaS real). El
// egress va por el Runtime real de la pasarela (policy.BuildRuntime → ssrfhttp +
// inyección de Bearer). httptest escucha en loopback, que la barrera SSRF bloquea
// por diseño; los tests de camino feliz la relajan con SetIPPolicyForTest (y la
// restauran). El test de URL privada NO la relaja y afirma ErrBlocked. No usan
// t.Parallel(): la política SSRF es una var de proceso.

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/odata"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

func allowLoopback(t *testing.T) {
	t.Helper()
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
}

func runtime(bearer string) connector.Runtime {
	return policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "org-1", InstanceID: "inst-1", Bearer: bearer, Log: slog.Default(),
	})
}

func mustFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return b
}

// odataServer levanta un servicio OData v4 mínimo: raíz + $metadata + entity set
// Products con dos páginas (nextLink relativo). Captura el Authorization visto.
func odataServer(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	page1 := mustFixture(t, "collection.json")
	page2 := mustFixture(t, "collection_page2.json")
	meta := mustFixture(t, "metadata.xml")
	var lastAuth string

	mux := http.NewServeMux()
	mux.HandleFunc("/svc/Products", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("$skiptoken") == "PAGE2" {
			_, _ = w.Write(page2)
			return
		}
		_, _ = w.Write(page1)
	})
	mux.HandleFunc("/svc/$metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(meta)
	})
	mux.HandleFunc("/svc/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"@odata.context":"$metadata","value":[{"name":"Products","url":"Products"}]}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &lastAuth
}

func instance(serviceURL string) connector.Instance {
	return connector.Instance{
		ID: "inst-1", OrgID: "org-1", TemplateKey: odata.Kind,
		Config: map[string]string{odata.ConfigServiceURL: serviceURL},
	}
}

func TestQueryPaginatesAndNormalizes(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth := odataServer(t)
	inst := instance(srv.URL + "/svc/")

	d := odata.New()
	tbl, err := d.Query(t.Context(), inst, runtime("tok-odata"), []byte(`{"dataset":"Products"}`))
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	// Paginación: 2 filas de la página 1 + 1 de la página 2 (nextLink seguido).
	if len(tbl.Rows) != 3 {
		t.Fatalf("filas = %d, quiero 3 (paginación por nextLink)", len(tbl.Rows))
	}
	// Columnas = unión alfabética, SIN las anotaciones @odata.* de control.
	wantCols := []string{"Active", "ID", "Name", "Price"}
	if len(tbl.Columns) != len(wantCols) {
		t.Fatalf("columnas = %+v, quiero %v", tbl.Columns, wantCols)
	}
	types := map[string]string{}
	for i, c := range tbl.Columns {
		if c.Name != wantCols[i] {
			t.Fatalf("columna %d = %q, quiero %q", i, c.Name, wantCols[i])
		}
		types[c.Name] = c.Type
	}
	// Tipos inferidos del JSON.
	if types["ID"] != "number" || types["Name"] != "text" || types["Active"] != "boolean" || types["Price"] != "number" {
		t.Fatalf("tipos inferidos mal: %v", types)
	}
	// Bearer inyectado por el Runtime (el driver nunca lo fija ni lo ve).
	if *lastAuth != "Bearer tok-odata" {
		t.Fatalf("Authorization = %q, quiero Bearer tok-odata", *lastAuth)
	}
}

func TestSchemaParsesMetadata(t *testing.T) {
	allowLoopback(t)
	srv, _ := odataServer(t)
	inst := instance(srv.URL + "/svc/")

	meta, err := odata.New().Schema(t.Context(), inst, runtime("tok"))
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if len(meta.EntitySets) != 2 {
		t.Fatalf("entity sets = %d, quiero 2 (Products, Categories)", len(meta.EntitySets))
	}
	var products *odata.EntitySet
	for i := range meta.EntitySets {
		if meta.EntitySets[i].Name == "Products" {
			products = &meta.EntitySets[i]
		}
	}
	if products == nil {
		t.Fatal("no se encontró el entity set Products")
	}
	if len(products.Fields) != 5 {
		t.Fatalf("campos de Products = %d, quiero 5", len(products.Fields))
	}
	ft := map[string]string{}
	for _, f := range products.Fields {
		ft[f.Name] = f.Type
	}
	if ft["ID"] != "number" || ft["Name"] != "text" || ft["Active"] != "boolean" || ft["Released"] != "datetime" || ft["Price"] != "number" {
		t.Fatalf("tipos Edm mapeados mal: %v", ft)
	}
}

func TestTestPingsServiceRoot(t *testing.T) {
	allowLoopback(t)
	srv, _ := odataServer(t)
	if err := odata.New().Test(t.Context(), instance(srv.URL+"/svc/"), runtime("tok")); err != nil {
		t.Fatalf("Test camino feliz: %v", err)
	}

	// Una raíz que responde 500 ⇒ Test falla.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	if err := odata.New().Test(t.Context(), instance(bad.URL+"/"), runtime("tok")); err == nil {
		t.Fatal("Test debería fallar con HTTP 500")
	}
}

func TestQueryPrivateURLBlocked(t *testing.T) {
	// SIN relajar la barrera: httptest escucha en loopback (privado) ⇒ ErrBlocked.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	defer srv.Close()

	_, err := odata.New().Query(t.Context(), instance(srv.URL+"/svc/"), runtime("tok"), []byte(`{"dataset":"Products"}`))
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("err = %v, quiero ssrfhttp.ErrBlocked (URL privada)", err)
	}
}

func TestQueryBadSpec(t *testing.T) {
	// dataset ausente ⇒ ErrBadQuery (400 en la capa HTTP), sin tocar la red.
	_, err := odata.New().Query(t.Context(), instance("https://sap.example.com/svc/"), runtime("tok"), []byte(`{}`))
	if !errors.Is(err, connector.ErrBadQuery) {
		t.Fatalf("err = %v, quiero connector.ErrBadQuery", err)
	}
}
