// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package odoo_test

// Tests OFFLINE del driver Odoo JSON-RPC contra un httptest (sin red ni Odoo real).
// El egress va por el Runtime real de la pasarela (policy.BuildRuntime → ssrfhttp).
// El camino feliz relaja la barrera SSRF para loopback (SetIPPolicyForTest) y la
// restaura; el test de URL privada NO la relaja y afirma ErrBlocked. Sin
// t.Parallel(): la política SSRF es una var de proceso.

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/odoo"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

func allowLoopback(t *testing.T) {
	t.Helper()
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
}

func runtime() connector.Runtime {
	// Odoo NO usa Bearer (credencial en el cuerpo): sin inyección de cabecera.
	return policy.BuildRuntime(policy.RuntimeParams{OrgID: "org-1", InstanceID: "inst-1", Log: slog.Default()})
}

// rpcIn captura lo esencial de una petición JSON-RPC recibida.
type rpcIn struct {
	Params struct {
		Service string            `json:"service"`
		Method  string            `json:"method"`
		Args    []json.RawMessage `json:"args"`
	} `json:"params"`
}

// odooServer levanta un Odoo JSON-RPC mínimo: authenticate → uid; execute_kw →
// el fixture search_read. Registra las peticiones para afirmar el flujo de auth.
func odooServer(t *testing.T) (*httptest.Server, *[]rpcIn) {
	t.Helper()
	fixture, err := os.ReadFile("testdata/search_read.json")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	var mu sync.Mutex
	var reqs []rpcIn

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jsonrpc" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var in rpcIn
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		reqs = append(reqs, in)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch in.Params.Method {
		case "authenticate":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":7}`))
		case "execute_kw":
			_, _ = w.Write(fixture)
		default: // version u otros
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"server_version":"17.0"}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs
}

func instance(baseURL string) connector.Instance {
	return connector.Instance{
		ID: "inst-1", OrgID: "org-1", TemplateKey: odoo.Kind,
		Config:  map[string]string{odoo.ConfigBaseURL: baseURL, odoo.ConfigDatabase: "acme", odoo.ConfigLogin: "admin"},
		Secrets: map[string]string{"api_key": "key-xyz"},
	}
}

func TestQueryAuthenticatesAndReadsModel(t *testing.T) {
	allowLoopback(t)
	srv, reqs := odooServer(t)

	spec := []byte(`{"model":"res.partner","fields":["id","name","email","is_company"],"domain":[["is_company","=",true]],"limit":10}`)
	tbl, err := odoo.New().Query(t.Context(), instance(srv.URL), runtime(), spec)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("filas = %d, quiero 2", len(tbl.Rows))
	}
	// Columnas: los fields pedidos en orden + extras (credit_limit) al final.
	want := []string{"id", "name", "email", "is_company", "credit_limit"}
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
	if ct["id"] != "number" || ct["name"] != "text" || ct["is_company"] != "boolean" || ct["credit_limit"] != "number" {
		t.Fatalf("tipos inferidos mal: %v", ct)
	}

	// Flujo: authenticate (uid) + execute_kw, ambos con la credencial EN EL CUERPO.
	if len(*reqs) != 2 {
		t.Fatalf("peticiones = %d, quiero 2 (authenticate + execute_kw)", len(*reqs))
	}
	auth, exec := (*reqs)[0], (*reqs)[1]
	if auth.Params.Method != "authenticate" || argString(t, auth.Params.Args, 2) != "key-xyz" {
		t.Fatalf("authenticate no llevó la credencial: %+v", auth.Params)
	}
	if exec.Params.Method != "execute_kw" {
		t.Fatalf("2ª petición = %q, quiero execute_kw", exec.Params.Method)
	}
	if argString(t, exec.Params.Args, 2) != "key-xyz" {
		t.Fatalf("execute_kw no llevó la credencial en args[2]")
	}
	if argString(t, exec.Params.Args, 3) != "res.partner" || argString(t, exec.Params.Args, 4) != "search_read" {
		t.Fatalf("execute_kw model/method inesperados: %+v", exec.Params.Args)
	}
}

func TestQueryRPCError(t *testing.T) {
	allowLoopback(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":200,"message":"Odoo Server Error","data":{"message":"AccessError"}}}`))
	}))
	defer srv.Close()
	_, err := odoo.New().Query(t.Context(), instance(srv.URL), runtime(), []byte(`{"model":"res.partner"}`))
	if err == nil {
		t.Fatal("un error JSON-RPC debería propagarse")
	}
}

func TestQueryPrivateURLBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":7}`))
	}))
	defer srv.Close()
	_, err := odoo.New().Query(t.Context(), instance(srv.URL), runtime(), []byte(`{"model":"res.partner"}`))
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("err = %v, quiero ssrfhttp.ErrBlocked (URL privada)", err)
	}
}

func TestQueryBadSpec(t *testing.T) {
	_, err := odoo.New().Query(t.Context(), instance("https://acme.odoo.com"), runtime(), []byte(`{}`))
	if !errors.Is(err, connector.ErrBadQuery) {
		t.Fatalf("err = %v, quiero connector.ErrBadQuery (falta model)", err)
	}
}

func argString(t *testing.T, args []json.RawMessage, i int) string {
	t.Helper()
	if i >= len(args) {
		t.Fatalf("args[%d] fuera de rango (len %d)", i, len(args))
	}
	var s string
	if err := json.Unmarshal(args[i], &s); err != nil {
		t.Fatalf("args[%d] no es string: %v", i, err)
	}
	return s
}
