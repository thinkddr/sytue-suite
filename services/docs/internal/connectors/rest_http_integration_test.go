// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

// Tests de INTEGRACIÓN del conector REST contra servidores HTTP reales (httptest):
// validan el camino que los tests unitarios no cubren —el round-trip HTTP de verdad—:
// que cada esquema de auth llega bien al origen y que el flujo OAuth2
// client_credentials (obtener token → Bearer → refresco+reintento tras 401 → caché)
// funciona end-to-end. httptest escucha en loopback, que la barrera SSRF bloquea por
// diseño; por eso se relaja con el seam ssrfhttp.SetIPPolicyForTest (y se restaura).
// Estos tests NO usan t.Parallel(): la política SSRF es una var de proceso.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// allowLoopbackSSRF relaja la barrera SSRF durante el test (destino = httptest en
// loopback) y la restaura al terminar.
func allowLoopbackSSRF(t *testing.T) {
	t.Helper()
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
}

func mustJSONRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// execRESTQuery ejecuta una consulta REST end-to-end (Execute) con un Executor nuevo.
func execRESTQuery(t *testing.T, cfg RestConfig, secret string, q RestQuery) (*Result, error) {
	t.Helper()
	return NewExecutor(Limits{}, "").Execute(
		context.Background(), KindREST, mustJSONRaw(t, cfg), Credentials{Secret: secret}, QuerySpec{REST: &q},
	)
}

// TestExecuteREST_AuthAndParse comprueba, contra un servidor real, que cada auth
// NO-OAuth llega al origen como toca y que el JSON de respuesta se aplana bien.
func TestExecuteREST_AuthAndParse(t *testing.T) {
	allowLoopbackSSRF(t)

	var (
		mu      sync.Mutex
		hAuth   string
		hAPIKey string
		qAPIKey string
		hAccept string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hAuth = r.Header.Get("Authorization")
		hAPIKey = r.Header.Get("X-API-Key")
		qAPIKey = r.URL.Query().Get("apikey")
		hAccept = r.Header.Get("Accept")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"name":"Ana"},{"id":2,"name":"Beto"}]`))
	}))
	defer srv.Close()
	base := srv.URL + "/"

	t.Run("bearer + parseo del JSON", func(t *testing.T) {
		res, err := execRESTQuery(t, RestConfig{BaseURL: base, Auth: RestAuth{Type: RestAuthBearer}}, "tok-123", RestQuery{Path: "data"})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if hAuth != "Bearer tok-123" {
			t.Errorf("Authorization = %q, quiero Bearer tok-123", hAuth)
		}
		if hAccept != "application/json, text/csv;q=0.9" {
			t.Errorf("Accept = %q, quiero JSON y, si no, CSV", hAccept)
		}
		if len(res.Rows) != 2 || len(res.Columns) != 2 {
			t.Fatalf("resultado = %d filas × %d cols, quiero 2×2", len(res.Rows), len(res.Columns))
		}
		if res.Columns[0].Name != "id" || res.Columns[1].Name != "name" { // unión de claves, alfabético
			t.Errorf("columnas = %+v", res.Columns)
		}
	})

	t.Run("api_key en cabecera", func(t *testing.T) {
		if _, err := execRESTQuery(t, RestConfig{BaseURL: base, Auth: RestAuth{Type: RestAuthAPIKey, In: "header", Name: "X-API-Key"}}, "k-abc", RestQuery{Path: "data"}); err != nil {
			t.Fatalf("err: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if hAPIKey != "k-abc" {
			t.Errorf("X-API-Key = %q, quiero k-abc", hAPIKey)
		}
	})

	t.Run("api_key en query", func(t *testing.T) {
		if _, err := execRESTQuery(t, RestConfig{BaseURL: base, Auth: RestAuth{Type: RestAuthAPIKey, In: "query", Name: "apikey"}}, "k-xyz", RestQuery{Path: "data"}); err != nil {
			t.Fatalf("err: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if qAPIKey != "k-xyz" {
			t.Errorf("apikey (query) = %q, quiero k-xyz", qAPIKey)
		}
	})

	t.Run("basic", func(t *testing.T) {
		if _, err := execRESTQuery(t, RestConfig{BaseURL: base, Auth: RestAuth{Type: RestAuthBasic, Username: "user"}}, "pass", RestQuery{Path: "data"}); err != nil {
			t.Fatalf("err: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
		if hAuth != want {
			t.Errorf("Authorization = %q, quiero %q", hAuth, want)
		}
	})
}

// oauthRec cuenta llamadas y captura lo recibido por el doble OAuth2 + API.
type oauthRec struct {
	mu          sync.Mutex
	tokenCalls  int
	dataCalls   int
	lastBearer  string     // Authorization visto en /data
	lastForm    url.Values // form recibido en /token (client_secret_post)
	lastTokAuth string     // Authorization visto en /token (client_secret_basic)
}

// newOAuthServer levanta un proveedor OAuth2 + API en un solo httptest: /token emite
// un token nuevo por llamada (AT1, AT2, …) y /data responde con el status que dicte
// `dataStatus(call)` (200 => 1 fila).
func newOAuthServer(dataStatus func(call int) int) (*httptest.Server, *oauthRec) {
	rec := &oauthRec{}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		rec.mu.Lock()
		rec.tokenCalls++
		n := rec.tokenCalls
		rec.lastForm = r.PostForm
		rec.lastTokAuth = r.Header.Get("Authorization")
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"AT%d","token_type":"Bearer","expires_in":3600}`, n)
	})
	mux.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.dataCalls++
		call := rec.dataCalls
		rec.lastBearer = r.Header.Get("Authorization")
		rec.mu.Unlock()
		if st := dataStatus(call); st != http.StatusOK {
			w.WriteHeader(st)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"v":1}]`))
	})
	return httptest.NewServer(mux), rec
}

func oauthCfg(srv *httptest.Server, clientAuth string) RestConfig {
	return RestConfig{
		BaseURL: srv.URL + "/",
		Auth: RestAuth{
			Type:       RestAuthOAuth2ClientCreds,
			TokenURL:   srv.URL + "/token",
			ClientID:   "cid",
			ClientAuth: clientAuth,
		},
	}
}

// TestExecuteREST_OAuth2 valida el flujo client_credentials contra servidores reales.
func TestExecuteREST_OAuth2(t *testing.T) {
	allowLoopbackSSRF(t)
	restQ := QuerySpec{REST: &RestQuery{Path: "data"}}

	t.Run("client_secret_post: obtiene token y lo usa como Bearer", func(t *testing.T) {
		srv, rec := newOAuthServer(func(int) int { return http.StatusOK })
		defer srv.Close()
		res, err := NewExecutor(Limits{}, "").Execute(context.Background(), KindREST, mustJSONRaw(t, oauthCfg(srv, "body")), Credentials{Secret: "secret"}, restQ)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		rec.mu.Lock()
		defer rec.mu.Unlock()
		if rec.tokenCalls != 1 {
			t.Errorf("token endpoint llamado %d veces, quiero 1", rec.tokenCalls)
		}
		if rec.lastBearer != "Bearer AT1" {
			t.Errorf("Bearer en /data = %q, quiero Bearer AT1", rec.lastBearer)
		}
		if rec.lastForm.Get("grant_type") != "client_credentials" ||
			rec.lastForm.Get("client_id") != "cid" || rec.lastForm.Get("client_secret") != "secret" {
			t.Errorf("form del token = %v", rec.lastForm)
		}
		if len(res.Rows) != 1 {
			t.Errorf("filas = %d, quiero 1", len(res.Rows))
		}
	})

	t.Run("client_secret_basic: credenciales en Authorization, no en el form", func(t *testing.T) {
		srv, rec := newOAuthServer(func(int) int { return http.StatusOK })
		defer srv.Close()
		if _, err := NewExecutor(Limits{}, "").Execute(context.Background(), KindREST, mustJSONRaw(t, oauthCfg(srv, "basic")), Credentials{Secret: "sh!h"}, restQ); err != nil {
			t.Fatalf("err: %v", err)
		}
		rec.mu.Lock()
		defer rec.mu.Unlock()
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte(url.QueryEscape("cid")+":"+url.QueryEscape("sh!h")))
		if rec.lastTokAuth != want {
			t.Errorf("Authorization del token = %q, quiero %q", rec.lastTokAuth, want)
		}
		if rec.lastForm.Get("client_secret") != "" {
			t.Errorf("con basic, client_secret NO debe ir en el form: %v", rec.lastForm)
		}
	})

	t.Run("401 en datos → refresca token y reintenta con el nuevo", func(t *testing.T) {
		srv, rec := newOAuthServer(func(call int) int {
			if call == 1 {
				return http.StatusUnauthorized
			}
			return http.StatusOK
		})
		defer srv.Close()
		res, err := NewExecutor(Limits{}, "").Execute(context.Background(), KindREST, mustJSONRaw(t, oauthCfg(srv, "body")), Credentials{Secret: "secret"}, restQ)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		rec.mu.Lock()
		defer rec.mu.Unlock()
		if rec.tokenCalls != 2 {
			t.Errorf("token endpoint llamado %d veces, quiero 2 (fetch + refresh)", rec.tokenCalls)
		}
		if rec.dataCalls != 2 {
			t.Errorf("/data llamado %d veces, quiero 2 (401 + reintento)", rec.dataCalls)
		}
		if rec.lastBearer != "Bearer AT2" {
			t.Errorf("Bearer del reintento = %q, quiero Bearer AT2 (token nuevo)", rec.lastBearer)
		}
		if len(res.Rows) != 1 {
			t.Errorf("filas = %d, quiero 1", len(res.Rows))
		}
	})

	t.Run("token cacheado entre ejecuciones del mismo Executor", func(t *testing.T) {
		srv, rec := newOAuthServer(func(int) int { return http.StatusOK })
		defer srv.Close()
		ex := NewExecutor(Limits{}, "")
		for i := 0; i < 2; i++ {
			if _, err := ex.Execute(context.Background(), KindREST, mustJSONRaw(t, oauthCfg(srv, "body")), Credentials{Secret: "secret"}, restQ); err != nil {
				t.Fatalf("ejecución %d: %v", i, err)
			}
		}
		rec.mu.Lock()
		defer rec.mu.Unlock()
		if rec.tokenCalls != 1 {
			t.Errorf("token endpoint llamado %d veces, quiero 1 (cacheado en la 2.ª)", rec.tokenCalls)
		}
		if rec.dataCalls != 2 {
			t.Errorf("/data llamado %d veces, quiero 2", rec.dataCalls)
		}
	})

	t.Run("token 4xx → ErrInvalid sin filtrar el cuerpo remoto", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_client","secret_leak":"nunca"}`))
		}))
		defer srv.Close()
		_, err := NewExecutor(Limits{}, "").Execute(context.Background(), KindREST, mustJSONRaw(t, oauthCfg(srv, "body")), Credentials{Secret: "secret"}, restQ)
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, quiero ErrInvalid", err)
		}
		if strings.Contains(err.Error(), "secret_leak") {
			t.Errorf("el error filtra el cuerpo remoto: %v", err)
		}
	})
}

// TestExecuteREST_UpstreamError comprueba el mapeo de un 5xx del origen a ErrUpstream.
func TestExecuteREST_UpstreamError(t *testing.T) {
	allowLoopbackSSRF(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, err := execRESTQuery(t, RestConfig{BaseURL: srv.URL + "/", Auth: RestAuth{Type: RestAuthNone}}, "", RestQuery{Path: "data"})
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, quiero ErrUpstream", err)
	}
}
