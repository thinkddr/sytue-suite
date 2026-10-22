// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package servicenow_test

// Tests OFFLINE del driver ServiceNow contra un httptest (sin red ni instancia real).
// El egress va por el Runtime real de la pasarela (policy.BuildRuntime → ssrfhttp), que
// INYECTA el Bearer del vault; el modo alternativo (usuario/contraseña de la cuenta de
// integración) lo compone el driver y se comprueba aparte. El camino feliz relaja la
// barrera SSRF para loopback (SetIPPolicyForTest) y la restaura; el test de host privado
// NO la relaja y afirma ErrBlocked. Sin t.Parallel(): la política SSRF es de proceso.

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
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/servicenow"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

func allowLoopback(t *testing.T) {
	t.Helper()
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
}

func runtime() connector.Runtime {
	return policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "org-1", InstanceID: "inst-1", Bearer: "snow-token", Log: slog.Default(),
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
		ID: "inst-1", OrgID: "org-1", TemplateKey: servicenow.Kind,
		Config: map[string]string{servicenow.ConfigInstance: base},
	}
}

// snServer levanta una instancia mínima: la Table API de incident (2 páginas por
// offset), kb_knowledge y sys_user (Test). Captura Authorization y los sysparm_* vistos.
func snServer(t *testing.T) (*httptest.Server, *string, *[]string) {
	t.Helper()
	page1 := mustFixture(t, "table_page1.json")
	page2 := mustFixture(t, "table_page2.json")
	kb := mustFixture(t, "search_kb.json")
	var lastAuth string
	var queries []string

	mux := http.NewServeMux()
	mux.HandleFunc("/api/now/table/incident", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("sysparm_offset") == "0" || r.URL.Query().Get("sysparm_offset") == "" {
			_, _ = w.Write(page1)
			return
		}
		_, _ = w.Write(page2)
	})
	mux.HandleFunc("/api/now/table/kb_knowledge", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(kb)
	})
	mux.HandleFunc("/api/now/table/sys_user", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"result":[{"sys_id":"u1"}]}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &lastAuth, &queries
}

// TestQueryFlattensAndPaginates: los campos objeto de ServiceNow —{value,display_value}
// y las referencias {link,value}— se aplanan a escalar, y la paginación por offset trae
// las dos páginas.
func TestQueryFlattensAndPaginates(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth, queries := snServer(t)

	spec := []byte(`{"table":"incident","query":"active=true","fields":["number","short_description","state","assigned_to"],"limit":2}`)
	tbl, err := servicenow.New().Query(t.Context(), instance(srv.URL), runtime(), spec)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(tbl.Rows) != 3 {
		t.Fatalf("filas = %d, quiero 3 (2 de la página 1 + 1 de la 2)", len(tbl.Rows))
	}
	want := []string{"number", "short_description", "state", "assigned_to"}
	if len(tbl.Columns) < len(want) {
		t.Fatalf("columnas = %+v, quiero al menos %v", tbl.Columns, want)
	}
	for i, n := range want {
		if tbl.Columns[i].Name != n {
			t.Fatalf("columna %d = %q, quiero %q", i, tbl.Columns[i].Name, n)
		}
	}
	// {value,display_value} ⇒ display_value; referencia {link,value} ⇒ value.
	if tbl.Rows[0][2] != "En curso" {
		t.Fatalf("state = %#v, quiero el display_value 'En curso'", tbl.Rows[0][2])
	}
	if tbl.Rows[0][3] != "u1" {
		t.Fatalf("assigned_to = %#v, quiero el value de la referencia", tbl.Rows[0][3])
	}
	// display_value vacío ⇒ se cae al value (fila 3).
	if tbl.Rows[2][2] != "3" {
		t.Fatalf("state de la fila 3 = %#v, quiero el value '3' (display_value vacío)", tbl.Rows[2][2])
	}
	// La primera página lleva el sysparm_query, el límite y el offset; la segunda avanza.
	if len(*queries) != 2 {
		t.Fatalf("peticiones = %d, quiero 2 páginas", len(*queries))
	}
	if !strings.Contains((*queries)[0], "sysparm_query=active%3Dtrue") || !strings.Contains((*queries)[0], "sysparm_limit=2") {
		t.Fatalf("página 1 = %q, quiero sysparm_query + sysparm_limit", (*queries)[0])
	}
	if !strings.Contains((*queries)[1], "sysparm_offset=2") {
		t.Fatalf("página 2 = %q, quiero sysparm_offset=2", (*queries)[1])
	}
	if *lastAuth != "Bearer snow-token" {
		t.Fatalf("Authorization = %q, quiero el Bearer inyectado por el Runtime", *lastAuth)
	}
}

// TestQueryMaxRows: el tope de filas corta la acumulación entre páginas.
func TestQueryMaxRows(t *testing.T) {
	allowLoopback(t)
	srv, _, _ := snServer(t)

	tbl, err := servicenow.New().Query(t.Context(), instance(srv.URL), runtime(),
		[]byte(`{"table":"incident","limit":2,"max_rows":1}`))
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(tbl.Rows) != 1 {
		t.Fatalf("filas = %d, quiero 1 (max_rows)", len(tbl.Rows))
	}
}

// TestQueryBasicAuth: con usuario/contraseña en el vault el driver compone la cabecera
// Basic (el Runtime no inyecta bearer en ese caso).
func TestQueryBasicAuth(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth, _ := snServer(t)

	inst := instance(srv.URL)
	inst.Secrets = map[string]string{"username": "svc_sytue", "password": "s3cr3t"}
	if _, err := servicenow.New().Query(t.Context(), inst, runtimeNoBearer(), []byte(`{"table":"incident"}`)); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if *lastAuth != "Basic c3ZjX3N5dHVlOnMzY3IzdA==" { // svc_sytue:s3cr3t
		t.Fatalf("Authorization = %q, quiero la auth básica compuesta por el driver", *lastAuth)
	}
}

// TestQueryBearerBeatsBasicAuth: si la instancia tiene consentimiento (bearer en el
// vault), el driver NO manda la credencial vieja aunque siga guardada. Sin esto, el admin
// autoriza, el asistente dice «conectado» y las lecturas siguen usando la contraseña de
// servicio de antes.
func TestQueryBearerBeatsBasicAuth(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth, _ := snServer(t)

	inst := instance(srv.URL)
	inst.Secrets = map[string]string{"username": "svc_sytue", "password": "s3cr3t"}
	inst.HasBearer = true // lo marca la pasarela al cargar la instancia (loadInstance).
	if _, err := servicenow.New().Query(t.Context(), inst, runtime(), []byte(`{"table":"incident"}`)); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if *lastAuth != "Bearer snow-token" {
		t.Fatalf("Authorization = %q, quiero el Bearer del consentimiento", *lastAuth)
	}
}

// TestQueryAPIError: un 403 con cuerpo de error de ServiceNow se mapea a algo legible.
func TestQueryAPIError(t *testing.T) {
	allowLoopback(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"Insufficient rights","detail":"ACL"},"status":"failure"}`))
	}))
	defer srv.Close()

	_, err := servicenow.New().Query(t.Context(), instance(srv.URL), runtime(), []byte(`{"table":"incident"}`))
	if err == nil || !strings.Contains(err.Error(), "Insufficient rights") {
		t.Fatalf("err = %v, quiero el mensaje de ServiceNow", err)
	}
}

// TestQueryPrivateURLBlocked: sin relajar la barrera ⇒ ssrfhttp.ErrBlocked.
func TestQueryPrivateURLBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	_, err := servicenow.New().Query(t.Context(), instance(srv.URL), runtime(), []byte(`{"table":"incident"}`))
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("err = %v, quiero ssrfhttp.ErrBlocked (host privado)", err)
	}
}

func TestQueryBadSpec(t *testing.T) {
	drv, rt := servicenow.New(), runtime()
	inst := instance("acme")
	for name, spec := range map[string]string{
		"sin tabla":            `{}`,
		"tabla inválida":       `{"table":"incident;drop"}`,
		"offset negativo":      `{"table":"incident","offset":-1}`,
		"display_value ilegal": `{"table":"incident","display_value":"maybe"}`,
		"no es JSON":           `no-json`,
	} {
		if _, err := drv.Query(t.Context(), inst, rt, []byte(spec)); !errors.Is(err, connector.ErrBadQuery) {
			t.Errorf("%s: err = %v, quiero connector.ErrBadQuery", name, err)
		}
	}
	// Sin instancia en la config tampoco se toca la red.
	if _, err := drv.Query(t.Context(), connector.Instance{}, rt, []byte(`{"table":"incident"}`)); !errors.Is(err, connector.ErrBadQuery) {
		t.Errorf("err = %v, quiero connector.ErrBadQuery (falta instance)", err)
	}
}

// TestSearchIncidentsAndArticles: incidencias + artículos de conocimiento → hits con
// deep-link a la instancia.
func TestSearchIncidentsAndArticles(t *testing.T) {
	allowLoopback(t)
	srv, _, queries := snServer(t)

	hits, err := servicenow.New().Search(t.Context(), instance(srv.URL), runtime(), "VPN", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("hits = %d, quiero 3 (2 incidencias + 1 artículo)", len(hits))
	}
	h := hits[0]
	if h.Title != "INC0010001 · El portal de clientes no carga" || h.Kind != "incident" || h.Source != "ServiceNow" {
		t.Fatalf("hit 0 inesperado: %+v", h)
	}
	if h.URL != srv.URL+"/incident.do?sys_id=aa11" {
		t.Fatalf("URL = %q, quiero el deep-link al registro", h.URL)
	}
	if h.Snippet != "En curso" {
		t.Fatalf("snippet = %q, quiero el estado legible", h.Snippet)
	}
	if hits[2].Kind != "article" || hits[2].URL != srv.URL+"/kb_knowledge.do?sys_id=kb99" {
		t.Fatalf("hit del artículo inesperado: %+v", hits[2])
	}
	if !strings.Contains((*queries)[0], "short_descriptionLIKEVPN") {
		t.Fatalf("consulta de búsqueda = %q, quiero el LIKE por texto", (*queries)[0])
	}
}

// TestSearchDegradesWhenOneTableFails: si kb_knowledge no está disponible (ACL/plugin),
// la búsqueda sigue devolviendo las incidencias.
func TestSearchDegradesWhenOneTableFails(t *testing.T) {
	allowLoopback(t)
	page1 := mustFixture(t, "table_page1.json")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/now/table/incident", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(page1)
	})
	mux.HandleFunc("/api/now/table/kb_knowledge", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"no ACL"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	hits, err := servicenow.New().Search(t.Context(), instance(srv.URL), runtime(), "VPN", 10)
	if err != nil {
		t.Fatalf("Search debería degradarse, no fallar: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, quiero las 2 incidencias", len(hits))
	}
}

// TestSearchSanitizesTerm: los delimitadores de sysparm_query (^ y ,) se descartan.
func TestSearchSanitizesTerm(t *testing.T) {
	allowLoopback(t)
	srv, _, queries := snServer(t)

	if _, err := servicenow.New().Search(t.Context(), instance(srv.URL), runtime(), "vpn^ORactive=true", 5); err != nil {
		t.Fatalf("Search: %v", err)
	}
	// El único ^ admisible es el que compone el driver (^OR entre sus dos cláusulas):
	// el del término se descarta, así que aparece exactamente una vez por cláusula.
	if strings.Count((*queries)[0], "%5E") != 1 {
		t.Fatalf("consulta = %q, el ^ del término no debería sobrevivir", (*queries)[0])
	}
}

// TestSearchEmptyQueryNoEgress: una q vacía no toca la red.
func TestSearchEmptyQueryNoEgress(t *testing.T) {
	hits, err := servicenow.New().Search(t.Context(), instance("acme"), runtime(), "  ", 10)
	if err != nil || hits != nil {
		t.Fatalf("Search(q vacía) = (%v, %v), quiero (nil, nil) sin egress", hits, err)
	}
}

// TestTestReachability: Test sondea sys_user con la auth de la instancia.
func TestTestReachability(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth, _ := snServer(t)

	if err := servicenow.New().Test(t.Context(), instance(srv.URL), runtime()); err != nil {
		t.Fatalf("Test camino feliz: %v", err)
	}
	if *lastAuth != "Bearer snow-token" {
		t.Fatalf("Authorization = %q, quiero el Bearer inyectado", *lastAuth)
	}
}

// TestTestPrivateBlocked: sin relajar la barrera ⇒ ssrfhttp.ErrBlocked.
func TestTestPrivateBlocked(t *testing.T) {
	priv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer priv.Close()
	if err := servicenow.New().Test(t.Context(), instance(priv.URL), runtime()); !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("Test host privado: err = %v, quiero ssrfhttp.ErrBlocked", err)
	}
}
