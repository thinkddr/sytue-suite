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
			Connection conexion `json:"connection"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out.Connection
	}
	const (
		aqui      = `"config":{"base_url":"https://api.example.com/v1","auth":{"type":"bearer"}}`
		alli      = `"config":{"base_url":"https://otro.example.org/v1","auth":{"type":"bearer"}}`
		conSecret = `,"credentials":{"secret":"s3creto"}}`
	)
	admin := []string{"admin"}

	c, creada := llamar(hd.createConnection, "POST", "ana", nil, "", `{"name":"ERP","kind":"rest",`+aqui+conSecret)
	if c != http.StatusCreated || !creada.HasCredentials {
		t.Fatalf("crear: %d %+v", c, creada)
	}
	id := creada.ID

	// Otro miembro la ve, pero ni la cambia ni la borra.
	if c, con := llamar(hd.getConnection, "GET", "luis", nil, id, ""); c != http.StatusOK || con.ID != id {
		t.Errorf("otro miembro no ve la conexión: %d", c)
	}
	if c, _ := llamar(hd.updateConnection, "PATCH", "luis", nil, id, `{"name":"De Luis",`+alli+`}`); c != http.StatusForbidden {
		t.Errorf("otro miembro cambia la conexión: %d", c)
	}
	if c, _ := llamar(hd.deleteConnection, "DELETE", "luis", nil, id, ""); c != http.StatusForbidden {
		t.Errorf("otro miembro borra la conexión: %d", c)
	}
	if c, con := llamar(hd.getConnection, "GET", "ana", nil, id, ""); c != http.StatusOK || con.Name != "ERP" || !con.HasCredentials {
		t.Fatalf("la conexión cambió: %d %+v", c, con)
	}

	// Quien la creó: cambiarle el nombre no toca la credencial…
	if c, con := llamar(hd.updateConnection, "PATCH", "ana", nil, id, `{"name":"ERP nuevo",`+aqui+`}`); c != http.StatusOK || con.Name != "ERP nuevo" || !con.HasCredentials {
		t.Errorf("renombrar: %d %+v", c, con)
	}
	// …cambiarle el destino sin volver a escribirla, la invalida…
	if c, con := llamar(hd.updateConnection, "PATCH", "ana", nil, id, `{"name":"ERP nuevo",`+alli+`}`); c != http.StatusOK || con.HasCredentials {
		t.Errorf("cambiar el destino conservó la credencial: %d %+v", c, con)
	}
	// …y escribiéndola de nuevo, vale.
	if c, con := llamar(hd.updateConnection, "PATCH", "ana", nil, id, `{"name":"ERP nuevo",`+alli+conSecret); c != http.StatusOK || !con.HasCredentials {
		t.Errorf("cambiar el destino con la credencial: %d %+v", c, con)
	}

	// Quien administra la organización, con la misma regla.
	if c, con := llamar(hd.updateConnection, "PATCH", "jefa", admin, id, `{"name":"ERP nuevo",`+aqui+`}`); c != http.StatusOK || con.HasCredentials {
		t.Errorf("quien administra cambia el destino y conserva la credencial: %d %+v", c, con)
	}
	if c, _ := llamar(hd.deleteConnection, "DELETE", "jefa", admin, id, ""); c != http.StatusNoContent {
		t.Errorf("quien administra no puede borrarla: %d", c)
	}
}
