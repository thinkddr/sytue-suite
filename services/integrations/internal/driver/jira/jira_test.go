// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jira_test

// Tests OFFLINE del driver Jira Cloud contra un httptest (sin red ni sitio real). El
// egress va por el Runtime real de la pasarela (policy.BuildRuntime → ssrfhttp), que
// INYECTA el Bearer del vault; el modo alternativo (auth básica correo + API token) lo
// compone el driver y se comprueba aparte. El camino feliz relaja la barrera SSRF para
// loopback (SetIPPolicyForTest) y la restaura; el de host privado NO la relaja y afirma
// ErrBlocked. Sin t.Parallel(): la política SSRF es una var de proceso.

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
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/jira"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

func allowLoopback(t *testing.T) {
	t.Helper()
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
}

func runtime() connector.Runtime {
	return policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "org-1", InstanceID: "inst-1", Bearer: "jira-token", Log: slog.Default(),
	})
}

// runtimeNoBearer simula una instancia SIN token en el vault (auth básica del driver).
func runtimeNoBearer() connector.Runtime {
	return policy.BuildRuntime(policy.RuntimeParams{OrgID: "org-1", InstanceID: "inst-1", Log: slog.Default()})
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
		ID: "inst-1", OrgID: "org-1", TemplateKey: jira.Kind,
		Config: map[string]string{jira.ConfigSite: base},
	}
}

// jiraServer levanta un sitio mínimo: /rest/api/3/search (2 páginas por startAt) y
// /rest/api/3/myself (Test). Captura Authorization y las query strings vistas.
func jiraServer(t *testing.T) (*httptest.Server, *string, *[]string) {
	t.Helper()
	page1 := mustFixture(t, "search_page1.json")
	page2 := mustFixture(t, "search_page2.json")
	var lastAuth string
	var queries []string

	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/3/search", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("startAt") == "2" {
			_, _ = w.Write(page2)
			return
		}
		_, _ = w.Write(page1)
	})
	mux.HandleFunc("/rest/api/3/myself", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"accountId":"5b1","displayName":"Cuenta de integración"}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &lastAuth, &queries
}

// TestQueryFlattensAndPaginates: JQL → 2 páginas por startAt → Table con los issues
// APLANADOS (status.name, assignee.displayName, labels unidas) y key como 1.ª columna.
func TestQueryFlattensAndPaginates(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth, queries := jiraServer(t)

	spec := []byte(`{"jql":"project = SUITE ORDER BY created DESC","fields":["summary","status","assignee","labels"],"max_results":2}`)
	tbl, err := jira.New().Query(t.Context(), instance(srv.URL), runtime(), spec)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(tbl.Rows) != 3 {
		t.Fatalf("filas = %d, quiero 3 (2 de la página 1 + 1 de la 2)", len(tbl.Rows))
	}
	want := []string{"key", "summary", "status", "assignee", "labels"}
	if len(tbl.Columns) < len(want) {
		t.Fatalf("columnas = %+v, quiero al menos %v", tbl.Columns, want)
	}
	for i, n := range want {
		if tbl.Columns[i].Name != n {
			t.Fatalf("columna %d = %q, quiero %q", i, tbl.Columns[i].Name, n)
		}
	}
	if tbl.Rows[0][0] != "SUITE-101" || tbl.Rows[2][0] != "SUITE-103" {
		t.Fatalf("claves mal paginadas: %#v", tbl.Rows)
	}
	// Aplanado: objetos → escalar legible; lista de etiquetas → texto.
	if tbl.Rows[0][2] != "En curso" || tbl.Rows[0][3] != "Ada Lovelace" {
		t.Fatalf("fila 0 = %#v, quiero status/assignee aplanados", tbl.Rows[0])
	}
	if tbl.Rows[0][4] != "docs, regresión" {
		t.Fatalf("labels = %#v, quiero la lista unida", tbl.Rows[0][4])
	}
	if tbl.Rows[1][3] != nil {
		t.Fatalf("assignee sin asignar = %#v, quiero null", tbl.Rows[1][3])
	}
	// La JQL y la paginación viajan tal cual.
	if !strings.Contains((*queries)[0], "jql=project+%3D+SUITE") || !strings.Contains((*queries)[0], "maxResults=2") {
		t.Fatalf("página 1 = %q, quiero la JQL + maxResults", (*queries)[0])
	}
	if !strings.Contains((*queries)[1], "startAt=2") {
		t.Fatalf("página 2 = %q, quiero startAt=2", (*queries)[1])
	}
	if *lastAuth != "Bearer jira-token" {
		t.Fatalf("Authorization = %q, quiero el Bearer inyectado por el Runtime", *lastAuth)
	}
}

// TestQueryMaxRows: el tope de filas corta la acumulación entre páginas.
func TestQueryMaxRows(t *testing.T) {
	allowLoopback(t)
	srv, _, _ := jiraServer(t)

	tbl, err := jira.New().Query(t.Context(), instance(srv.URL), runtime(), []byte(`{"max_results":2,"max_rows":2}`))
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("filas = %d, quiero 2 (max_rows)", len(tbl.Rows))
	}
}

// TestQueryProjectShorthand: sin JQL cruda, project/status componen la consulta.
func TestQueryProjectShorthand(t *testing.T) {
	allowLoopback(t)
	srv, _, queries := jiraServer(t)

	if _, err := jira.New().Query(t.Context(), instance(srv.URL), runtime(),
		[]byte(`{"project":"SUITE","status":"En curso","max_results":2,"max_rows":2}`)); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !strings.Contains((*queries)[0], `jql=project+%3D+%22SUITE%22+AND+status+%3D+%22En+curso%22`) {
		t.Fatalf("jql = %q, quiero la consulta compuesta", (*queries)[0])
	}
}

// TestQueryBasicAuth: con correo + API token el driver compone la cabecera Basic (el
// Runtime no inyecta bearer en ese caso).
func TestQueryBasicAuth(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth, _ := jiraServer(t)

	inst := instance(srv.URL)
	inst.Config[jira.ConfigEmail] = "bot@acme.com"
	inst.Secrets = map[string]string{"api_token": "atl-token"}
	if _, err := jira.New().Query(t.Context(), inst, runtimeNoBearer(), []byte(`{"max_rows":2,"max_results":2}`)); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if *lastAuth != "Basic Ym90QGFjbWUuY29tOmF0bC10b2tlbg==" { // bot@acme.com:atl-token
		t.Fatalf("Authorization = %q, quiero la auth básica correo+token", *lastAuth)
	}
}

// TestQueryBearerBeatsBasicAuth: si la instancia tiene consentimiento (bearer en el
// vault), el driver NO manda la credencial vieja aunque siga guardada. Sin esto, el admin
// autoriza, el asistente dice «conectado» y las lecturas siguen usando el token pegado.
func TestQueryBearerBeatsBasicAuth(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth, _ := jiraServer(t)

	inst := instance(srv.URL)
	inst.Config[jira.ConfigEmail] = "bot@acme.com"
	inst.Secrets = map[string]string{"api_token": "atl-token"}
	inst.HasBearer = true // lo marca la pasarela al cargar la instancia (loadInstance).
	if _, err := jira.New().Query(t.Context(), inst, runtime(), []byte(`{"max_rows":2,"max_results":2}`)); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if *lastAuth != "Bearer jira-token" {
		t.Fatalf("Authorization = %q, quiero el Bearer del consentimiento", *lastAuth)
	}
}

// TestQueryAPIError: un 400 de Jira (errorMessages) se mapea a un error legible.
func TestQueryAPIError(t *testing.T) {
	allowLoopback(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errorMessages":["El campo 'foo' no existe"],"errors":{}}`))
	}))
	defer srv.Close()

	_, err := jira.New().Query(t.Context(), instance(srv.URL), runtime(), []byte(`{"jql":"foo = bar"}`))
	if err == nil || !strings.Contains(err.Error(), "no existe") {
		t.Fatalf("err = %v, quiero el errorMessage de Jira", err)
	}
}

// TestQueryPrivateURLBlocked: sin relajar la barrera ⇒ ssrfhttp.ErrBlocked.
func TestQueryPrivateURLBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	_, err := jira.New().Query(t.Context(), instance(srv.URL), runtime(), []byte(`{}`))
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("err = %v, quiero ssrfhttp.ErrBlocked (host privado)", err)
	}
}

func TestQueryBadSpec(t *testing.T) {
	drv, rt := jira.New(), runtime()
	inst := instance("https://acme.atlassian.net")
	for name, spec := range map[string]string{
		"campo inválido":    `{"fields":["summary; drop"]}`,
		"start_at negativo": `{"start_at":-1}`,
		"no es JSON":        `no-json`,
	} {
		if _, err := drv.Query(t.Context(), inst, rt, []byte(spec)); !errors.Is(err, connector.ErrBadQuery) {
			t.Errorf("%s: err = %v, quiero connector.ErrBadQuery", name, err)
		}
	}
	// Sin sitio en la config tampoco se toca la red.
	if _, err := drv.Query(t.Context(), connector.Instance{}, rt, []byte(`{}`)); !errors.Is(err, connector.ErrBadQuery) {
		t.Errorf("err = %v, quiero connector.ErrBadQuery (falta site)", err)
	}
}

// TestSearchMapsIssues: texto → issues con deep-link /browse/KEY.
func TestSearchMapsIssues(t *testing.T) {
	allowLoopback(t)
	srv, _, queries := jiraServer(t)

	hits, err := jira.New().Search(t.Context(), instance(srv.URL), runtime(), "foco", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, quiero 2 (los issues de la primera página)", len(hits))
	}
	h := hits[0]
	if h.Title != "SUITE-101 · El editor de Docs pierde el foco al pegar" || h.Kind != "issue" || h.Source != "Jira" {
		t.Fatalf("hit 0 inesperado: %+v", h)
	}
	if h.URL != srv.URL+"/browse/SUITE-101" {
		t.Fatalf("URL = %q, quiero el deep-link /browse/KEY", h.URL)
	}
	if h.Snippet != "En curso" {
		t.Fatalf("snippet = %q, quiero el estado", h.Snippet)
	}
	if !strings.Contains((*queries)[0], `jql=text+~+%22foco%22`) {
		t.Fatalf("jql = %q, quiero la búsqueda por texto", (*queries)[0])
	}
}

// TestSearchEscapesJQL: una comilla en el término no puede cerrar el literal JQL.
func TestSearchEscapesJQL(t *testing.T) {
	allowLoopback(t)
	srv, _, queries := jiraServer(t)

	if _, err := jira.New().Search(t.Context(), instance(srv.URL), runtime(), `foo" OR key = "SUITE-1`, 5); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains((*queries)[0], `%5C%22`) { // \" escapada
		t.Fatalf("jql = %q, quiero la comilla escapada", (*queries)[0])
	}
}

// TestSearchEmptyQueryNoEgress: una q vacía no toca la red.
func TestSearchEmptyQueryNoEgress(t *testing.T) {
	hits, err := jira.New().Search(t.Context(), instance("https://acme.atlassian.net"), runtime(), " ", 10)
	if err != nil || hits != nil {
		t.Fatalf("Search(q vacía) = (%v, %v), quiero (nil, nil) sin egress", hits, err)
	}
}

// TestTestMyself: Test sonda /rest/api/3/myself con la auth de la instancia.
func TestTestMyself(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth, _ := jiraServer(t)

	if err := jira.New().Test(t.Context(), instance(srv.URL), runtime()); err != nil {
		t.Fatalf("Test camino feliz: %v", err)
	}
	if *lastAuth != "Bearer jira-token" {
		t.Fatalf("Authorization = %q, quiero el Bearer inyectado", *lastAuth)
	}
}

// TestTestUnauthorized: un 401 hace fallar el Test (credencial inválida).
func TestTestUnauthorized(t *testing.T) {
	allowLoopback(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessages":["Client must be authenticated"],"errors":{}}`))
	}))
	defer srv.Close()

	err := jira.New().Test(t.Context(), instance(srv.URL), runtime())
	if err == nil || !strings.Contains(err.Error(), "must be authenticated") {
		t.Fatalf("err = %v, quiero el fallo de credencial", err)
	}
}

// TestTestPrivateBlocked: sin relajar la barrera ⇒ ssrfhttp.ErrBlocked.
func TestTestPrivateBlocked(t *testing.T) {
	priv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer priv.Close()
	if err := jira.New().Test(t.Context(), instance(priv.URL), runtime()); !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("Test host privado: err = %v, quiero ssrfhttp.ErrBlocked", err)
	}
}
