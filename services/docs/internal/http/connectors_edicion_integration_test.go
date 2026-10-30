// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	docsdb "git.sytue.eu/sytue/one/services/docs/db"
	"git.sytue.eu/sytue/one/services/docs/internal/connectors"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// Una conexión de datos la cambia o la borra quien la creó o quien administra la
// organización, y cambiarle el destino sin volver a escribir su credencial la invalida.
// Usarla sigue siendo de cualquier miembro.
func TestEditarUnaConexionDeDatos(t *testing.T) {
	h := pgtest.New(t, docsdb.Migrations, docsdb.MigrationsDir)
	sellador, err := connectors.NewSealer(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	hd := &Handler{connStore: store.New(h.Pool), sealer: sellador}

	type conexion struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		HasCredentials bool   `json:"has_credentials"`
		CanEdit        bool   `json:"can_edit"`
	}
	llamar := func(f http.HandlerFunc, metodo, sub string, roles []string, id, cuerpo string) (int, conexion) {
		t.Helper()
		var params map[string]string
		if id != "" {
			params = map[string]string{"connId": id}
		}
		r := req(t, metodo, "/v1/connectors", &authn.Claims{Subject: sub, Tenant: "acme", Roles: roles}, params, cuerpo)
		r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
		rec := httptest.NewRecorder()
		f(rec, r)
		var out struct {
			Connection  conexion   `json:"connection"`
			Connections []conexion `json:"connections"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		if len(out.Connections) == 1 {
			out.Connection = out.Connections[0]
		}
		return rec.Code, out.Connection
	}
	const (
		aqui      = `"config":{"base_url":"https://api.example.com/v1","auth":{"type":"bearer"}}`
		alli      = `"config":{"base_url":"https://otro.example.org/v1","auth":{"type":"bearer"}}`
		conSecret = `,"credentials":{"secret":"s3creto"}}`
	)
	admin := []string{"admin"}

	c, creada := llamar(hd.createConnection, "POST", "ana", nil, "", `{"name":"ERP","kind":"rest",`+aqui+conSecret)
	if c != http.StatusCreated || !creada.HasCredentials || !creada.CanEdit {
		t.Fatalf("crear: %d %+v", c, creada)
	}
	id := creada.ID

	// La lista que consume el editor informa el permiso de la sesión actual.
	for _, actor := range []struct {
		sub      string
		roles    []string
		editable bool
	}{
		{"ana", nil, true},
		{"luis", nil, false},
		{"jefa", admin, true},
		{"fundadora", []string{"owner"}, true},
	} {
		c, con := llamar(hd.listConnections, "GET", actor.sub, actor.roles, "", "")
		if c != http.StatusOK || con.ID != id || con.CanEdit != actor.editable {
			t.Errorf("lista para %s: %d %+v, editable=%v", actor.sub, c, con, actor.editable)
		}
	}

	// Otro miembro la ve, pero ni la cambia ni la borra, y recibe can_edit: false.
	if c, con := llamar(hd.getConnection, "GET", "luis", nil, id, ""); c != http.StatusOK || con.ID != id || con.CanEdit {
		t.Errorf("otro miembro no ve la conexión o la ve editable: %d %+v", c, con)
	}
	if c, _ := llamar(hd.updateConnection, "PATCH", "luis", nil, id, `{"name":"De Luis",`+alli+`}`); c != http.StatusForbidden {
		t.Errorf("otro miembro cambia la conexión: %d", c)
	}
	if c, _ := llamar(hd.deleteConnection, "DELETE", "luis", nil, id, ""); c != http.StatusForbidden {
		t.Errorf("otro miembro borra la conexión: %d", c)
	}
	if c, con := llamar(hd.getConnection, "GET", "ana", nil, id, ""); c != http.StatusOK || con.Name != "ERP" || !con.HasCredentials || !con.CanEdit {
		t.Fatalf("la conexión cambió: %d %+v", c, con)
	}

	// Quien la creó: cambiarle el nombre no toca la credencial…
	if c, con := llamar(hd.updateConnection, "PATCH", "ana", nil, id, `{"name":"ERP nuevo",`+aqui+`}`); c != http.StatusOK || con.Name != "ERP nuevo" || !con.HasCredentials || !con.CanEdit {
		t.Errorf("renombrar: %d %+v", c, con)
	}
	// …cambiarle el destino sin volver a escribirla, la invalida…
	if c, con := llamar(hd.updateConnection, "PATCH", "ana", nil, id, `{"name":"ERP nuevo",`+alli+`}`); c != http.StatusOK || con.HasCredentials || !con.CanEdit {
		t.Errorf("cambiar el destino conservó la credencial: %d %+v", c, con)
	}
	// …y escribiéndola de nuevo, vale.
	if c, con := llamar(hd.updateConnection, "PATCH", "ana", nil, id, `{"name":"ERP nuevo",`+alli+conSecret); c != http.StatusOK || !con.HasCredentials || !con.CanEdit {
		t.Errorf("cambiar el destino con la credencial: %d %+v", c, con)
	}

	// Quien administra la organización, con la misma regla.
	if c, con := llamar(hd.updateConnection, "PATCH", "jefa", admin, id, `{"name":"ERP nuevo",`+aqui+`}`); c != http.StatusOK || con.HasCredentials || !con.CanEdit {
		t.Errorf("quien administra cambia el destino y conserva la credencial: %d %+v", c, con)
	}
	if c, _ := llamar(hd.deleteConnection, "DELETE", "jefa", admin, id, ""); c != http.StatusNoContent {
		t.Errorf("quien administra no puede borrarla: %d", c)
	}
}
