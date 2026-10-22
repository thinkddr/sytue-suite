// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

// Test OFFLINE del origen GATEWAY contra un httptest que imita
// `POST /api/v1/instances/{id}/data/query` del Sytue Integration Gateway. El
// destino es interno ⇒ cliente normal (SIN barrera SSRF), por eso no se relaja
// ninguna política. Verifica que el Table del gateway entra en el pipeline
// (transform «pasos aplicados» aplicado) y que el token del usuario va passthrough.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExecuteGateway(t *testing.T) {
	var (
		gotAuth string
		gotPath string
		gotBody []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"columns":[{"name":"id","type":"number"},{"name":"name","type":"text"}],"rows":[[1,"Ana"],[2,"Beto"]]}`))
	}))
	defer srv.Close()

	// La raíz del gateway es config de SERVICIO (inyectada al Executor), no de
	// conexión: aquí se apunta al httptest. La conexión solo lleva el InstanceID.
	cfg := GatewayConfig{InstanceID: "inst-abc"}
	spec := QuerySpec{
		Gateway:   &GatewayQuery{Spec: json.RawMessage(`{"dataset":"Products","top":2}`)},
		Transform: &Transform{Rename: map[string]string{"name": "nombre"}}, // pasos aplicados
	}
	ctx := WithUserToken(context.Background(), "user-jwt")

	res, err := NewExecutor(Limits{}, srv.URL).Execute(ctx, KindGateway, mustJSONRaw(t, cfg), Credentials{}, spec)
	if err != nil {
		t.Fatalf("Execute gateway: %v", err)
	}
	// El Table del gateway entró en el pipeline y el transform se aplicó (name→nombre).
	if len(res.Rows) != 2 || len(res.Columns) != 2 {
		t.Fatalf("resultado = %d filas × %d cols, quiero 2×2", len(res.Rows), len(res.Columns))
	}
	if res.Columns[0].Name != "id" || res.Columns[1].Name != "nombre" {
		t.Fatalf("transform no aplicado: columnas = %+v", res.Columns)
	}
	// Passthrough del token del usuario + endpoint + spec reenviada verbatim.
	if gotAuth != "Bearer user-jwt" {
		t.Errorf("Authorization = %q, quiero Bearer user-jwt", gotAuth)
	}
	if gotPath != "/api/v1/instances/inst-abc/data/query" {
		t.Errorf("path = %q", gotPath)
	}
	if string(gotBody) != `{"dataset":"Products","top":2}` {
		t.Errorf("cuerpo reenviado = %q", gotBody)
	}
}

// NEG-06: quien no está en la lista de la conexión recibe de la pasarela un 403 con
// código propio. No es un fallo del origen: llega como ErrNotAllowed para que la
// interfaz lo diga tal cual, en el idioma de quien consulta.
func TestExecuteGatewayNoEnLaLista(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"connection_not_allowed","message":"No estás en la lista."}}`))
	}))
	defer srv.Close()
	cfg := GatewayConfig{InstanceID: "inst-abc"}
	spec := QuerySpec{Gateway: &GatewayQuery{Spec: json.RawMessage(`{"dataset":"X"}`)}}
	ctx := WithUserToken(context.Background(), "user-jwt")
	_, err := NewExecutor(Limits{}, srv.URL).Execute(ctx, KindGateway, mustJSONRaw(t, cfg), Credentials{}, spec)
	if !errors.Is(err, ErrNotAllowed) || errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, quiero ErrNotAllowed (y no ErrUpstream)", err)
	}
}

func TestExecuteGatewayMissingToken(t *testing.T) {
	cfg := GatewayConfig{InstanceID: "inst-abc"}
	spec := QuerySpec{Gateway: &GatewayQuery{Spec: json.RawMessage(`{"dataset":"X"}`)}}
	// Sin WithUserToken en el contexto ⇒ ErrInvalid (400), sin tocar la red.
	_, err := NewExecutor(Limits{}, "http://integrations:8093").Execute(context.Background(), KindGateway, mustJSONRaw(t, cfg), Credentials{}, spec)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, quiero ErrInvalid (falta el token de usuario)", err)
	}
}

func TestSanitizeConfigGateway(t *testing.T) {
	// La config de conexión gateway es SOLO el instance_id (la raíz es de servicio).
	out, err := SanitizeConfig(KindGateway, json.RawMessage(`{"instance_id":"abc","junk":"drop"}`))
	if err != nil {
		t.Fatalf("SanitizeConfig gateway válida: %v", err)
	}
	var c GatewayConfig
	if err := json.Unmarshal(out, &c); err != nil || c.InstanceID != "abc" {
		t.Fatalf("config no canónica: %+v err=%v", c, err)
	}
	bad := []json.RawMessage{
		json.RawMessage(`{}`),                    // sin instance_id
		json.RawMessage(`{"instance_id":""}`),    // instance_id vacío
		json.RawMessage(`{"instance_id":"a/b"}`), // id con separador de ruta
		json.RawMessage(`{"instance_id":"a?b"}`), // id con carácter de query
	}
	for _, b := range bad {
		if _, err := SanitizeConfig(KindGateway, b); !errors.Is(err, ErrInvalid) {
			t.Errorf("SanitizeConfig gateway(%s) = %v; quiero ErrInvalid", b, err)
		}
	}
}
