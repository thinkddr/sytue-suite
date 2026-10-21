// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package graph_test

// Tests OFFLINE del driver Microsoft Graph contra un httptest (sin red ni M365 real).
// El egress va por el Runtime real de la pasarela (policy.BuildRuntime → ssrfhttp). El
// driver mintea su propio token en el token endpoint del httptest (client-credentials)
// y lo pone como Bearer en las llamadas a Graph: el Runtime NO inyecta bearer (estos
// conectores no tienen uno estático en el vault). El camino feliz relaja la barrera
// SSRF para loopback (SetIPPolicyForTest) y la restaura; el test de host privado NO la
// relaja y afirma ErrBlocked. Sin t.Parallel(): la política SSRF es una var de proceso.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/graph"
	sytueoauth2 "git.sytue.eu/sytue/one/services/integrations/internal/oauth2"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

func allowLoopback(t *testing.T) {
	t.Helper()
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
}

func runtime() connector.Runtime {
	// Graph mintea su propio token (client-credentials): sin inyección de bearer.
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

// graphServer levanta un Microsoft Graph mínimo: token endpoint de Entra + la colección
// users en dos páginas (nextLink relativo) + $metadata (para Test). Captura el
// Authorization visto por Graph y el formulario recibido por el token endpoint.
func graphServer(t *testing.T) (*httptest.Server, *string, *url.Values) {
	t.Helper()
	page1 := mustFixture(t, "users_page1.json")
	page2 := mustFixture(t, "users_page2.json")
	token := mustFixture(t, "token.json")
	var lastAuth string
	var tokenForm url.Values

	mux := http.NewServeMux()
	mux.HandleFunc("/tenant-1/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		tokenForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(token)
	})
	mux.HandleFunc("/v1.0/users", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("$skiptoken") == "PAGE2" {
			_, _ = w.Write(page2)
			return
		}
		_, _ = w.Write(page1)
	})
	mux.HandleFunc("/v1.0/$metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<edmx:Edmx/>`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &lastAuth, &tokenForm
}

func instance(base, secret string) connector.Instance {
	return connector.Instance{
		ID: "inst-1", OrgID: "org-1", TemplateKey: graph.Kind,
		Config: map[string]string{
			graph.ConfigTenant:        "tenant-1",
			graph.ConfigClientID:      "app-1",
			graph.ConfigGraphBase:     base,
			graph.ConfigAuthorityBase: base,
		},
		Secrets: map[string]string{"client_secret": secret},
	}
}

func TestQueryMintsTokenPaginatesNormalizes(t *testing.T) {
	allowLoopback(t)
	srv, lastAuth, tokenForm := graphServer(t)

	spec := []byte(`{"resource":"users","select":["id","displayName","userPrincipalName","accountEnabled"]}`)
	tbl, err := graph.New(sytueoauth2.NewCache()).Query(t.Context(), instance(srv.URL, "shh-secret"), runtime(), spec)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	// Paginación por @odata.nextLink: 2 filas de la página 1 + 1 de la página 2.
	if len(tbl.Rows) != 3 {
		t.Fatalf("filas = %d, quiero 3 (paginación por nextLink)", len(tbl.Rows))
	}
	// Columnas = orden del $select; SIN las anotaciones @odata.* de control.
	want := []string{"id", "displayName", "userPrincipalName", "accountEnabled"}
	if len(tbl.Columns) != len(want) {
		t.Fatalf("columnas = %+v, quiero %v", tbl.Columns, want)
	}
	ct := map[string]string{}
	for i, c := range tbl.Columns {
		if c.Name != want[i] {
			t.Fatalf("columna %d = %q, quiero %q", i, c.Name, want[i])
		}
		ct[c.Name] = c.Type
	}
	if ct["id"] != "text" || ct["accountEnabled"] != "boolean" {
		t.Fatalf("tipos inferidos mal: %v", ct)
	}
	if tbl.Rows[0][1] != "Ada Lovelace" || tbl.Rows[0][3] != true {
		t.Fatalf("fila 0 = %#v, quiero displayName=Ada Lovelace accountEnabled=true", tbl.Rows[0])
	}
	// El token minteado por el driver va como Bearer (el Runtime NO lo inyecta).
	if *lastAuth != "Bearer graph-app-token-abc123" {
		t.Fatalf("Authorization = %q, quiero Bearer graph-app-token-abc123 (token minteado)", *lastAuth)
	}
	// El token endpoint recibió las credenciales de cliente EN EL CUERPO.
	if tokenForm.Get("grant_type") != "client_credentials" || tokenForm.Get("client_id") != "app-1" || tokenForm.Get("client_secret") != "shh-secret" {
		t.Fatalf("form del token = %v, quiero client_credentials + client_id + client_secret", *tokenForm)
	}
	if tokenForm.Get("scope") != "https://graph.microsoft.com/.default" {
		t.Fatalf("scope = %q, quiero el .default de Graph", tokenForm.Get("scope"))
	}
}

// TestQueryAPIError: un 403 de Graph con cuerpo de error se mapea a un error legible.
func TestQueryAPIError(t *testing.T) {
	allowLoopback(t)
	token := mustFixture(t, "token.json")
	mux := http.NewServeMux()
	mux.HandleFunc("/tenant-1/oauth2/v2.0/token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(token)
	})
	mux.HandleFunc("/v1.0/users", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"Authorization_RequestDenied","message":"Insufficient privileges"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, err := graph.New(sytueoauth2.NewCache()).Query(t.Context(), instance(srv.URL, "shh"), runtime(), []byte(`{"resource":"users"}`))
	if err == nil {
		t.Fatal("un 403 de Graph debería propagarse como error")
	}
	if !strings.Contains(err.Error(), "Insufficient privileges") {
		t.Fatalf("err = %v, quiero el message de Graph", err)
	}
}

// TestQueryPrivateURLBlocked: SIN relajar la barrera, las bases apuntan a loopback ⇒ el
// egress (empezando por el token endpoint) se bloquea con ssrfhttp.ErrBlocked.
func TestQueryPrivateURLBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"x"}`))
	}))
	defer srv.Close()

	_, err := graph.New(sytueoauth2.NewCache()).Query(t.Context(), instance(srv.URL, "shh"), runtime(), []byte(`{"resource":"users"}`))
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("err = %v, quiero ssrfhttp.ErrBlocked (host privado)", err)
	}
}

// TestTestReachability: Test sondea $metadata (reachability, SIN auth: no recibe el
// secreto) y exige que la raíz no sea 5xx.
func TestTestReachability(t *testing.T) {
	allowLoopback(t)
	srv, _, _ := graphServer(t)
	if err := graph.New(sytueoauth2.NewCache()).Test(t.Context(), instance(srv.URL, ""), runtime()); err != nil {
		t.Fatalf("Test camino feliz: %v", err)
	}
}

// TestTestPrivateBlocked: SIN relajar la barrera, Test a loopback ⇒ ssrfhttp.ErrBlocked.
func TestTestPrivateBlocked(t *testing.T) {
	priv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer priv.Close()
	err := graph.New(sytueoauth2.NewCache()).Test(t.Context(), instance(priv.URL, ""), runtime())
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("Test host privado: err = %v, quiero ssrfhttp.ErrBlocked", err)
	}
}

func TestQueryBadSpec(t *testing.T) {
	// Falta el recurso ⇒ ErrBadQuery, sin tocar la red.
	_, err := graph.New(sytueoauth2.NewCache()).Query(t.Context(), instance("https://graph.microsoft.com", "shh"), runtime(), []byte(`{}`))
	if !errors.Is(err, connector.ErrBadQuery) {
		t.Fatalf("err = %v, quiero connector.ErrBadQuery (falta resource)", err)
	}
	// Falta el secreto ⇒ ErrBadQuery.
	noSecret := instance("https://graph.microsoft.com", "")
	_, err = graph.New(sytueoauth2.NewCache()).Query(t.Context(), noSecret, runtime(), []byte(`{"resource":"users"}`))
	if !errors.Is(err, connector.ErrBadQuery) {
		t.Fatalf("err = %v, quiero connector.ErrBadQuery (falta client_secret)", err)
	}
}

// TestQueryReusesTokenAcrossQueries: dos consultas de la MISMA instancia comparten el
// access token. Antes se minteaba uno por consulta —el propio driver lo admitía en un
// comentario—, así que cada consulta del usuario costaba un viaje extra a Entra y
// consumía cuota de egress de la organización.
func TestQueryReusesTokenAcrossQueries(t *testing.T) {
	allowLoopback(t)
	var tokenCalls int
	page := mustFixture(t, "users_page1.json")

	mux := http.NewServeMux()
	mux.HandleFunc("/tenant-1/oauth2/v2.0/token", func(w http.ResponseWriter, _ *http.Request) {
		tokenCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mustFixture(t, "token.json"))
	})
	mux.HandleFunc("/v1.0/users", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(page)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := graph.New(sytueoauth2.NewCache())
	inst := instance(srv.URL, "s3cr3t")
	spec := json.RawMessage(`{"resource":"users","max_rows":1}`)

	for i := range 2 {
		if _, err := d.Query(context.Background(), inst, runtime(), spec); err != nil {
			t.Fatalf("consulta %d: %v", i+1, err)
		}
	}
	if tokenCalls != 1 {
		t.Errorf("llamadas al token endpoint = %d, quiero 1: la caché no se está usando", tokenCalls)
	}
}
