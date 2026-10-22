// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package powerbi_test

// Tests OFFLINE del driver Power BI (DAX) contra un httptest (sin red ni Power BI
// real). El egress va por el Runtime real de la pasarela (policy.BuildRuntime →
// ssrfhttp + inyección de Bearer). El camino feliz relaja la barrera SSRF para
// loopback (SetIPPolicyForTest) y la restaura; el test de URL privada NO la relaja
// y afirma ErrBlocked. Sin t.Parallel(): la política SSRF es una var de proceso.

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/powerbi"
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

// pbiServer levanta una API Power BI mínima: POST .../datasets/{id}/executeQueries
// devuelve el fixture; GET .../datasets/{id} y GET .../datasets responden 200 (para
// Test). Captura el Authorization y la ruta vistos.
func pbiServer(t *testing.T) (*httptest.Server, *string, *string) {
	t.Helper()
	fixture, err := os.ReadFile("testdata/execute_queries.json")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	var lastAuth, lastPath string

	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/myorg/datasets/", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		lastPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/executeQueries") {
			_, _ = w.Write(fixture)
			return
		}
		_, _ = w.Write([]byte(`{"id":"ds-1","name":"Sales"}`))
	})
	mux.HandleFunc("/v1.0/myorg/datasets", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		lastPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"ds-1"}]}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &lastAuth, &lastPath
}

func instance(apiBase, datasetID string) connector.Instance {
	cfg := map[string]string{powerbi.ConfigAPIBase: apiBase}
	if datasetID != "" {
		cfg[powerbi.ConfigDatasetID] = datasetID
	}
	return connector.Instance{ID: "inst-1", OrgID: "org-1", TemplateKey: powerbi.Kind, Config: cfg}
}

// TestQueryFlattensDAXShape: aplana results[0].tables[0].rows a Table, con las
// columnas EN EL ORDEN de la primera fila (no alfabético) y tipos de los valores
// JSON de DAX. Afirma el dataset en la ruta y el Bearer inyectado.
func TestQueryFlattensDAXShape(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth, lastPath := pbiServer(t)

	spec := []byte(`{"query":"EVALUATE Product","dataset_id":"ds-1"}`)
	tbl, err := powerbi.New().Query(t.Context(), instance(srv.URL, ""), runtime("tok-pbi"), spec)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("filas = %d, quiero 2", len(tbl.Rows))
	}
	// Orden de columnas preservado del EVALUATE (claves de la primera fila).
	want := []string{"Product[Category]", "Product[Units]", "Product[Active]"}
	if len(tbl.Columns) != len(want) {
		t.Fatalf("columnas = %+v, quiero %v", tbl.Columns, want)
	}
	ct := map[string]string{}
	for i, c := range tbl.Columns {
		if c.Name != want[i] {
			t.Fatalf("columna %d = %q, quiero %q (orden del EVALUATE, no alfabético)", i, c.Name, want[i])
		}
		ct[c.Name] = c.Type
	}
	if ct["Product[Category]"] != "text" || ct["Product[Units]"] != "number" || ct["Product[Active]"] != "boolean" {
		t.Fatalf("tipos DAX mal inferidos: %v", ct)
	}
	// Valores JSON tipados de DAX (número como float64, bool como bool).
	if tbl.Rows[0][0] != "Bikes" || tbl.Rows[0][1] != float64(120) || tbl.Rows[0][2] != true {
		t.Fatalf("fila 0 = %#v, quiero [Bikes 120 true]", tbl.Rows[0])
	}
	// El dataset va en la ruta (executeQueries del dataset correcto).
	if !strings.Contains(*lastPath, "/datasets/ds-1/executeQueries") {
		t.Fatalf("ruta = %q, quiero .../datasets/ds-1/executeQueries", *lastPath)
	}
	// Bearer inyectado por el Runtime (el driver nunca lo fija ni lo ve).
	if *lastAuth != "Bearer tok-pbi" {
		t.Fatalf("Authorization = %q, quiero Bearer tok-pbi", *lastAuth)
	}
}

// TestQueryDatasetFromConfig: si la spec no trae dataset, se usa el de la config.
func TestQueryDatasetFromConfig(t *testing.T) {
	allowLoopback(t)
	srv, _, lastPath := pbiServer(t)

	_, err := powerbi.New().Query(t.Context(), instance(srv.URL, "ds-1"), runtime("tok"), []byte(`{"query":"EVALUATE Product"}`))
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !strings.Contains(*lastPath, "/datasets/ds-1/executeQueries") {
		t.Fatalf("ruta = %q, quiero el dataset de la config", *lastPath)
	}
}

// TestQueryAPIError: un 400 con cuerpo de error de Power BI se mapea a un error
// legible (con el message de la API), no a una tabla vacía.
func TestQueryAPIError(t *testing.T) {
	allowLoopback(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"InvalidRequest","message":"consulta DAX inválida"}}`))
	}))
	defer srv.Close()

	_, err := powerbi.New().Query(t.Context(), instance(srv.URL, "ds-1"), runtime("tok"), []byte(`{"query":"EVALUATE X"}`))
	if err == nil {
		t.Fatal("un 400 de la API debería propagarse como error")
	}
	if !strings.Contains(err.Error(), "consulta DAX inválida") {
		t.Fatalf("err = %v, quiero el message de la API", err)
	}
}

// TestTestPingsDataset: Test con dataset hace GET al dataset y exige 2xx.
func TestTestPingsDataset(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth, _ := pbiServer(t)
	if err := powerbi.New().Test(t.Context(), instance(srv.URL, "ds-1"), runtime("tok-test")); err != nil {
		t.Fatalf("Test camino feliz: %v", err)
	}
	if *lastAuth != "Bearer tok-test" {
		t.Fatalf("Authorization = %q, quiero Bearer tok-test", *lastAuth)
	}

	// Un origen que responde 403 ⇒ Test falla.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer bad.Close()
	if err := powerbi.New().Test(t.Context(), instance(bad.URL, "ds-1"), runtime("tok")); err == nil {
		t.Fatal("Test debería fallar con HTTP 403")
	}
}

// TestQueryPrivateURLBlocked: SIN relajar la barrera, api_base apunta a loopback
// (privado) ⇒ el egress se bloquea antes de conectar con ssrfhttp.ErrBlocked.
func TestQueryPrivateURLBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	_, err := powerbi.New().Query(t.Context(), instance(srv.URL, "ds-1"), runtime("tok"), []byte(`{"query":"EVALUATE X"}`))
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("err = %v, quiero ssrfhttp.ErrBlocked (URL privada)", err)
	}
}

// TestQueryBadSpec: sin query DAX o sin dataset ⇒ ErrBadQuery (400), sin red.
func TestQueryBadSpec(t *testing.T) {
	// Falta la query DAX.
	_, err := powerbi.New().Query(t.Context(), instance("https://api.powerbi.com", "ds-1"), runtime("tok"), []byte(`{}`))
	if !errors.Is(err, connector.ErrBadQuery) {
		t.Fatalf("err = %v, quiero connector.ErrBadQuery (falta query)", err)
	}
	// Falta el dataset (ni spec ni config).
	_, err = powerbi.New().Query(t.Context(), instance("https://api.powerbi.com", ""), runtime("tok"), []byte(`{"query":"EVALUATE X"}`))
	if !errors.Is(err, connector.ErrBadQuery) {
		t.Fatalf("err = %v, quiero connector.ErrBadQuery (falta dataset)", err)
	}
}
