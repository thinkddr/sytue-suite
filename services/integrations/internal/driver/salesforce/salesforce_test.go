// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package salesforce_test

// Tests OFFLINE del driver Salesforce contra un httptest (sin red ni org real). El
// egress va por el Runtime real de la pasarela (policy.BuildRuntime → ssrfhttp), que
// INYECTA el Bearer del vault: el driver no fija Authorization. El camino feliz relaja
// la barrera SSRF para loopback (SetIPPolicyForTest) y la restaura; el test de host
// privado NO la relaja y afirma ErrBlocked. Sin t.Parallel(): la política SSRF es una
// var de proceso.

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
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/salesforce"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

func allowLoopback(t *testing.T) {
	t.Helper()
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
}

// runtime fabrica el Runtime de la pasarela con el token OAuth de la org en el vault
// (el driver no lo ve: lo inyecta el RoundTripper).
func runtime() connector.Runtime {
	return policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "org-1", InstanceID: "inst-1", Bearer: "sf-oauth-token", Log: slog.Default(),
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

func instance(base string) connector.Instance {
	return connector.Instance{
		ID: "inst-1", OrgID: "org-1", TemplateKey: salesforce.Kind,
		Config: map[string]string{salesforce.ConfigInstanceURL: base},
	}
}

// sfServer levanta una org mínima: /services/data (Test), /query (página 1),
// /query/{locator} (página 2) y /search (SOSL). Captura el Authorization y el último
// parámetro q visto.
func sfServer(t *testing.T) (*httptest.Server, *string, *string) {
	t.Helper()
	page1 := mustFixture(t, "query_page1.json")
	page2 := mustFixture(t, "query_page2.json")
	search := mustFixture(t, "search.json")
	var lastAuth, lastQ string

	mux := http.NewServeMux()
	mux.HandleFunc("/services/data", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`[{"version":"61.0","url":"/services/data/v61.0"}]`))
	})
	mux.HandleFunc("/services/data/v61.0/query", func(w http.ResponseWriter, r *http.Request) {
		lastAuth, lastQ = r.Header.Get("Authorization"), r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(page1)
	})
	mux.HandleFunc("/services/data/v61.0/query/", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(page2)
	})
	mux.HandleFunc("/services/data/v61.0/search", func(w http.ResponseWriter, r *http.Request) {
		lastAuth, lastQ = r.Header.Get("Authorization"), r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(search)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &lastAuth, &lastQ
}

// TestQueryPaginatesAndNormalizes: SOQL → 2 páginas por nextRecordsUrl → Table con las
// columnas en el orden del SELECT, sin los "attributes" de control, y con el Bearer
// inyectado por el Runtime.
func TestQueryPaginatesAndNormalizes(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth, lastQ := sfServer(t)

	spec := []byte(`{"soql":"SELECT Id, Name, AnnualRevenue FROM Account"}`)
	tbl, err := salesforce.New().Query(t.Context(), instance(srv.URL), runtime(), spec)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(tbl.Rows) != 3 {
		t.Fatalf("filas = %d, quiero 3 (paginación por nextRecordsUrl)", len(tbl.Rows))
	}
	want := []string{"Id", "Name", "AnnualRevenue"}
	if len(tbl.Columns) != len(want) {
		t.Fatalf("columnas = %+v, quiero %v (sin 'attributes')", tbl.Columns, want)
	}
	for i, c := range tbl.Columns {
		if c.Name != want[i] {
			t.Fatalf("columna %d = %q, quiero %q", i, c.Name, want[i])
		}
	}
	if tbl.Columns[2].Type != "number" {
		t.Fatalf("tipo de AnnualRevenue = %q, quiero number", tbl.Columns[2].Type)
	}
	if tbl.Rows[0][1] != "Acme Corporation" || tbl.Rows[2][1] != "Initech" {
		t.Fatalf("filas mal ordenadas/paginadas: %#v", tbl.Rows)
	}
	if *lastQ != "SELECT Id, Name, AnnualRevenue FROM Account" {
		t.Fatalf("q = %q, quiero la SOQL tal cual", *lastQ)
	}
	if *lastAuth != "Bearer sf-oauth-token" {
		t.Fatalf("Authorization = %q, quiero el Bearer inyectado por el Runtime", *lastAuth)
	}
}

// TestQueryMaxRows: el tope de filas corta la acumulación (no se materializa el dataset).
func TestQueryMaxRows(t *testing.T) {
	allowLoopback(t)
	srv, _, _ := sfServer(t)

	tbl, err := salesforce.New().Query(t.Context(), instance(srv.URL), runtime(),
		[]byte(`{"soql":"SELECT Id, Name FROM Account","max_rows":2}`))
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("filas = %d, quiero 2 (max_rows)", len(tbl.Rows))
	}
}

// TestQueryObjectShorthand: object+fields compone la SOQL (sin SOQL cruda).
func TestQueryObjectShorthand(t *testing.T) {
	allowLoopback(t)
	srv, _, lastQ := sfServer(t)

	if _, err := salesforce.New().Query(t.Context(), instance(srv.URL), runtime(),
		[]byte(`{"object":"Account","fields":["Id","Name"],"where":"IsDeleted = false","limit":10}`)); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if *lastQ != "SELECT Id, Name FROM Account WHERE IsDeleted = false LIMIT 10" {
		t.Fatalf("q = %q, quiero la SOQL compuesta", *lastQ)
	}
}

// TestQueryNextRecordsCrossHostBlocked: un nextRecordsUrl que salta a otro host
// filtraría el token inyectado ⇒ se bloquea.
func TestQueryNextRecordsCrossHostBlocked(t *testing.T) {
	allowLoopback(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"totalSize":2,"done":false,` +
			`"nextRecordsUrl":"https://evil.example.com/services/data/v61.0/query/01g-2000",` +
			`"records":[{"Id":"001","Name":"Acme"}]}`))
	}))
	defer srv.Close()

	_, err := salesforce.New().Query(t.Context(), instance(srv.URL), runtime(),
		[]byte(`{"soql":"SELECT Id, Name FROM Account"}`))
	if err == nil || !strings.Contains(err.Error(), "cambia de host") {
		t.Fatalf("err = %v, quiero el bloqueo de nextRecordsUrl cross-host", err)
	}
}

// TestQueryAPIError: un 400 de Salesforce (array [{errorCode,message}]) se mapea a un
// error legible.
func TestQueryAPIError(t *testing.T) {
	allowLoopback(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`[{"message":"unexpected token: FROM","errorCode":"MALFORMED_QUERY"}]`))
	}))
	defer srv.Close()

	_, err := salesforce.New().Query(t.Context(), instance(srv.URL), runtime(),
		[]byte(`{"soql":"SELECT FROM Account"}`))
	if err == nil || !strings.Contains(err.Error(), "unexpected token") {
		t.Fatalf("err = %v, quiero el message de Salesforce", err)
	}
}

// TestQueryPrivateURLBlocked: SIN relajar la barrera, la org apunta a loopback ⇒
// ssrfhttp.ErrBlocked.
func TestQueryPrivateURLBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	_, err := salesforce.New().Query(t.Context(), instance(srv.URL), runtime(),
		[]byte(`{"soql":"SELECT Id FROM Account"}`))
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("err = %v, quiero ssrfhttp.ErrBlocked (host privado)", err)
	}
}

func TestQueryBadSpec(t *testing.T) {
	drv, rt := salesforce.New(), runtime()
	inst := instance("https://acme.my.salesforce.com")
	for name, spec := range map[string]string{
		"sin soql ni object": `{}`,
		"no es SELECT":       `{"soql":"DELETE FROM Account"}`,
		"objeto inválido":    `{"object":"Account; DROP"}`,
		"max_rows negativo":  `{"soql":"SELECT Id FROM Account","max_rows":-1}`,
	} {
		if _, err := drv.Query(t.Context(), inst, rt, []byte(spec)); !errors.Is(err, connector.ErrBadQuery) {
			t.Errorf("%s: err = %v, quiero connector.ErrBadQuery", name, err)
		}
	}
	// Sin instance_url ni subdomain tampoco se toca la red.
	if _, err := drv.Query(t.Context(), connector.Instance{}, rt, []byte(`{"soql":"SELECT Id FROM Account"}`)); !errors.Is(err, connector.ErrBadQuery) {
		t.Errorf("err = %v, quiero connector.ErrBadQuery (falta instance_url)", err)
	}
}

// TestSearchMapsRecords: SOSL → hits con deep-link Lightning, tipo y fuente.
func TestSearchMapsRecords(t *testing.T) {
	allowLoopback(t)
	srv, _, lastQ := sfServer(t)

	hits, err := salesforce.New().Search(t.Context(), instance(srv.URL), runtime(), "acme", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("hits = %d, quiero 3 (Account+Contact+Case)", len(hits))
	}
	if !strings.HasPrefix(*lastQ, "FIND {acme}") || !strings.Contains(*lastQ, "RETURNING Account(") {
		t.Fatalf("q = %q, quiero una SOSL FIND {…} RETURNING …", *lastQ)
	}
	h := hits[0]
	if h.Title != "Acme Corporation" || h.Kind != "account" || h.Source != "Salesforce" {
		t.Fatalf("hit 0 inesperado: %+v", h)
	}
	if h.URL != srv.URL+"/lightning/r/Account/0015e00000AAA111/view" {
		t.Fatalf("URL = %q, quiero el deep-link Lightning", h.URL)
	}
	if hits[1].Snippet != "ada@acme.example.com" {
		t.Fatalf("snippet del contacto = %q, quiero el email", hits[1].Snippet)
	}
	if hits[2].Title != "Acme: fallo de facturación" {
		t.Fatalf("hit 2 = %+v, quiero el asunto del caso", hits[2])
	}
}

// TestSearchEscapesSOSL: los metacaracteres reservados del término se escapan (no
// pueden alterar la cláusula FIND).
func TestSearchEscapesSOSL(t *testing.T) {
	allowLoopback(t)
	srv, _, lastQ := sfServer(t)

	if _, err := salesforce.New().Search(t.Context(), instance(srv.URL), runtime(), `ac{me} OR *`, 5); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.HasPrefix(*lastQ, `FIND {ac\{me\} OR \*}`) {
		t.Fatalf("q = %q, quiero los metacaracteres escapados", *lastQ)
	}
}

// TestSearchEmptyQueryNoEgress: una q vacía no toca la red.
func TestSearchEmptyQueryNoEgress(t *testing.T) {
	hits, err := salesforce.New().Search(t.Context(), instance("https://acme.my.salesforce.com"), runtime(), "   ", 10)
	if err != nil || hits != nil {
		t.Fatalf("Search(q vacía) = (%v, %v), quiero (nil, nil) sin egress", hits, err)
	}
}

// TestSearchPrivateURLBlocked: sin relajar la barrera ⇒ ssrfhttp.ErrBlocked.
func TestSearchPrivateURLBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	_, err := salesforce.New().Search(t.Context(), instance(srv.URL), runtime(), "acme", 10)
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("err = %v, quiero ssrfhttp.ErrBlocked (host privado)", err)
	}
}

// TestTestReachability: Test sondea /services/data con el Bearer inyectado.
func TestTestReachability(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth, _ := sfServer(t)

	// El subdominio del alta 1-click también resuelve la raíz (aquí se usa la URL directa).
	if err := salesforce.New().Test(t.Context(), instance(srv.URL), runtime()); err != nil {
		t.Fatalf("Test camino feliz: %v", err)
	}
	if *lastAuth != "Bearer sf-oauth-token" {
		t.Fatalf("Authorization = %q, quiero el Bearer inyectado", *lastAuth)
	}
}

// TestTestUnauthorized: un 401 de la org hace fallar el Test (credencial inválida).
func TestTestUnauthorized(t *testing.T) {
	allowLoopback(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`[{"message":"Session expired or invalid","errorCode":"INVALID_SESSION_ID"}]`))
	}))
	defer srv.Close()

	err := salesforce.New().Test(t.Context(), instance(srv.URL), runtime())
	if err == nil || !strings.Contains(err.Error(), "Session expired") {
		t.Fatalf("err = %v, quiero el fallo de credencial", err)
	}
}

// TestTestPrivateBlocked: sin relajar la barrera, Test a loopback ⇒ ssrfhttp.ErrBlocked.
func TestTestPrivateBlocked(t *testing.T) {
	priv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer priv.Close()
	if err := salesforce.New().Test(t.Context(), instance(priv.URL), runtime()); !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("Test host privado: err = %v, quiero ssrfhttp.ErrBlocked", err)
	}
}
