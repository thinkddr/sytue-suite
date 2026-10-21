// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package databricks_test

// Tests OFFLINE del driver Databricks SQL contra un httptest (sin red ni workspace
// real). El egress va por el Runtime real de la pasarela (policy.BuildRuntime →
// ssrfhttp + inyección de Bearer). El camino feliz relaja la barrera SSRF para
// loopback (SetIPPolicyForTest) y la restaura; el test de URL privada NO la relaja
// y afirma ErrBlocked. Sin t.Parallel(): la política SSRF es una var de proceso.

import (
	"encoding/json"
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
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/databricks"
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

// dbxServer levanta una Statement Execution API mínima: POST /statements devuelve
// postFixture; GET /statements/{id} devuelve el estado SUCCEEDED con 2 chunks; GET
// .../result/chunks/{n} devuelve el chunk 1; GET /warehouses/{id} responde 200.
// Captura el Authorization visto (para afirmar la inyección del Bearer).
func dbxServer(t *testing.T, postFixture string) (*httptest.Server, *string) {
	t.Helper()
	post := mustFixture(t, postFixture)
	chunked := mustFixture(t, "statement_chunked.json")
	chunk1 := mustFixture(t, "chunk1.json")
	var lastAuth string

	mux := http.NewServeMux()
	mux.HandleFunc("/api/2.0/sql/statements", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(post)
	})
	mux.HandleFunc("/api/2.0/sql/statements/", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/result/chunks/") {
			_, _ = w.Write(chunk1)
			return
		}
		_, _ = w.Write(chunked)
	})
	mux.HandleFunc("/api/2.0/sql/warehouses/", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"wh-1","state":"RUNNING"}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &lastAuth
}

func instance(workspaceURL string) connector.Instance {
	return connector.Instance{
		ID: "inst-1", OrgID: "org-1", TemplateKey: databricks.Kind,
		Config: map[string]string{
			databricks.ConfigWorkspaceURL: workspaceURL,
			databricks.ConfigWarehouseID:  "wh-1",
		},
	}
}

func colTypes(t *testing.T, tbl connector.Table, order []string) map[string]string {
	t.Helper()
	if len(tbl.Columns) != len(order) {
		t.Fatalf("columnas = %+v, quiero %v", tbl.Columns, order)
	}
	ct := map[string]string{}
	for i, c := range tbl.Columns {
		if c.Name != order[i] {
			t.Fatalf("columna %d = %q, quiero %q", i, c.Name, order[i])
		}
		ct[c.Name] = c.Type
	}
	return ct
}

func num(t *testing.T, v any) string {
	t.Helper()
	n, ok := v.(json.Number)
	if !ok {
		t.Fatalf("valor = %#v, quiero json.Number", v)
	}
	return n.String()
}

// TestQueryPollsAndAssemblesChunks: POST→RUNNING obliga a sondear; GET status llega
// SUCCEEDED con 2 chunks; el segundo se pide por índice. 3 filas ensambladas, tipos
// del schema (no inferidos sobre texto), valores coaccionados, Bearer inyectado.
func TestQueryPollsAndAssemblesChunks(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth := dbxServer(t, "statement_running.json")

	spec := []byte(`{"statement":"SELECT * FROM t"}`)
	tbl, err := databricks.New().Query(t.Context(), instance(srv.URL), runtime("tok-dbx"), spec)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(tbl.Rows) != 3 {
		t.Fatalf("filas = %d, quiero 3 (chunk 0 con 2 + chunk 1 con 1)", len(tbl.Rows))
	}
	ct := colTypes(t, tbl, []string{"id", "name", "active", "amount"})
	if ct["id"] != "number" || ct["name"] != "text" || ct["active"] != "boolean" || ct["amount"] != "number" {
		t.Fatalf("tipos del schema mal mapeados: %v", ct)
	}
	// Valores coaccionados desde el texto de JSON_ARRAY: número→json.Number, bool→bool.
	if got := num(t, tbl.Rows[0][0]); got != "1" {
		t.Fatalf("id[0] = %q, quiero 1", got)
	}
	if tbl.Rows[0][1] != "Alice" {
		t.Fatalf("name[0] = %v, quiero Alice", tbl.Rows[0][1])
	}
	if tbl.Rows[0][2] != true {
		t.Fatalf("active[0] = %v, quiero true", tbl.Rows[0][2])
	}
	if got := num(t, tbl.Rows[0][3]); got != "9.99" {
		t.Fatalf("amount[0] = %q, quiero 9.99", got)
	}
	// null preservado (amount de Bob es null en el fixture).
	if tbl.Rows[1][3] != nil {
		t.Fatalf("amount[1] = %#v, quiero nil (null)", tbl.Rows[1][3])
	}
	// Fila del segundo chunk.
	if got := num(t, tbl.Rows[2][0]); got != "3" || tbl.Rows[2][1] != "Carol" {
		t.Fatalf("fila del chunk 1 mal ensamblada: %#v", tbl.Rows[2])
	}
	// Bearer inyectado por el Runtime (el driver nunca lo fija ni lo ve).
	if *lastAuth != "Bearer tok-dbx" {
		t.Fatalf("Authorization = %q, quiero Bearer tok-dbx", *lastAuth)
	}
}

// TestQueryDirectSuccess: el POST ya vuelve SUCCEEDED (un solo chunk) ⇒ sin sondeo
// ni fetch de chunks. Valida la normalización básica y valores con signo.
func TestQueryDirectSuccess(t *testing.T) {
	allowLoopback(t)
	srv, _ := dbxServer(t, "statement_succeeded.json")

	tbl, err := databricks.New().Query(t.Context(), instance(srv.URL), runtime("tok"), []byte(`{"query":"SELECT 1"}`))
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("filas = %d, quiero 2", len(tbl.Rows))
	}
	if got := num(t, tbl.Rows[0][0]); got != "10" {
		t.Fatalf("id[0] = %q, quiero 10", got)
	}
	if got := num(t, tbl.Rows[0][3]); got != "-5.25" {
		t.Fatalf("amount[0] = %q, quiero -5.25", got)
	}
	if tbl.Rows[1][2] != true {
		t.Fatalf("active[1] = %v, quiero true", tbl.Rows[1][2])
	}
}

// TestQueryFailedState: un estado terminal FAILED se propaga como error (no como
// tabla vacía), llevando el mensaje del estado.
func TestQueryFailedState(t *testing.T) {
	allowLoopback(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"statement_id":"s","status":{"state":"FAILED","error":{"error_code":"BAD_REQUEST","message":"tabla inexistente"}}}`))
	}))
	defer srv.Close()

	_, err := databricks.New().Query(t.Context(), instance(srv.URL), runtime("tok"), []byte(`{"statement":"SELECT 1"}`))
	if err == nil {
		t.Fatal("un estado FAILED debería propagarse como error")
	}
	if !strings.Contains(err.Error(), "FAILED") {
		t.Fatalf("err = %v, quiero mención de FAILED", err)
	}
}

// TestTestPingsWarehouse: Test hace GET al warehouse y exige 2xx.
func TestTestPingsWarehouse(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth := dbxServer(t, "statement_succeeded.json")
	if err := databricks.New().Test(t.Context(), instance(srv.URL), runtime("tok-test")); err != nil {
		t.Fatalf("Test camino feliz: %v", err)
	}
	if *lastAuth != "Bearer tok-test" {
		t.Fatalf("Authorization = %q, quiero Bearer tok-test", *lastAuth)
	}

	// Un warehouse que responde 500 ⇒ Test falla.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	if err := databricks.New().Test(t.Context(), instance(bad.URL), runtime("tok")); err == nil {
		t.Fatal("Test debería fallar con HTTP 500")
	}
}

// TestQueryPrivateURLBlocked: SIN relajar la barrera, httptest escucha en loopback
// (privado) ⇒ el egress se bloquea antes de conectar con ssrfhttp.ErrBlocked.
func TestQueryPrivateURLBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":{"state":"SUCCEEDED"}}`))
	}))
	defer srv.Close()

	_, err := databricks.New().Query(t.Context(), instance(srv.URL), runtime("tok"), []byte(`{"statement":"SELECT 1"}`))
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("err = %v, quiero ssrfhttp.ErrBlocked (URL privada)", err)
	}
}

// TestQueryBadSpec: sin sentencia ⇒ ErrBadQuery (400 en la capa HTTP), sin red.
func TestQueryBadSpec(t *testing.T) {
	_, err := databricks.New().Query(t.Context(), instance("https://dbc-x.cloud.databricks.com"), runtime("tok"), []byte(`{}`))
	if !errors.Is(err, connector.ErrBadQuery) {
		t.Fatalf("err = %v, quiero connector.ErrBadQuery (falta statement)", err)
	}
}
