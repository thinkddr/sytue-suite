// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// TestElCanalEnVivoSaludaYEntrega: el canal va a `/documents/{id}/ws` con el token en la
// cabecera, abre con `hello` y la última seq, y entrega lo que manda el servicio.
func TestElCanalEnVivoSaludaYEntrega(t *testing.T) {
	var hola map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/documents/d1/ws" {
			t.Errorf("ruta %s", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		if err := wsjson.Read(r.Context(), c, &hola); err != nil {
			return
		}
		_ = wsjson.Write(r.Context(), c, map[string]any{"type": "welcome", "role": "editor", "seq": 12})
		_ = wsjson.Write(r.Context(), c, map[string]any{"type": "ops", "ops": []map[string]any{
			{"seq": 13, "actor": "web-1", "op": map[string]any{"kind": "insert", "dot": map[string]any{"a": "web-1", "c": 3}, "value": "x", "lamport": 9}},
		}})
		_ = c.Close(websocket.StatusNormalClosure, "")
	}))
	defer srv.Close()
	d := NewDocs(srv.URL, sesionFijaAPI{}, srv.Client())
	var tipos []string
	var seq int64
	err := d.Coeditar(context.Background(), "d1", 7, func(m DocMensaje) {
		tipos = append(tipos, m.Type)
		for _, o := range m.Ops {
			seq = o.Seq
		}
	})
	if err == nil || hola["type"] != "hello" || hola["version"] != float64(DocProtocolVersion) || hola["last_seq"] != float64(7) || auth != "Bearer t" {
		t.Fatalf("saludo %v, cabecera %q, fin %v", hola, auth, err)
	}
	if strings.Join(tipos, " ") != "welcome ops" || seq != 13 {
		t.Errorf("entregado: %v, seq %d", tipos, seq)
	}
}

type sesionFijaAPI struct{}

func TestElCanalNoEntregaOperacionesDeOtraVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		var hola map[string]any
		if err := wsjson.Read(r.Context(), c, &hola); err != nil {
			return
		}
		_ = wsjson.Write(r.Context(), c, map[string]any{"type": "ops", "version": DocProtocolVersion + 1})
	}))
	defer srv.Close()
	d := NewDocs(srv.URL, nil, srv.Client())
	entregadas := 0
	err := d.Coeditar(context.Background(), "d1", 0, func(DocMensaje) { entregadas++ })
	if err == nil || !strings.Contains(err.Error(), "protocol version") || entregadas != 0 {
		t.Fatalf("otro contrato llegó al editor: %d, %v", entregadas, err)
	}
}

func (sesionFijaAPI) AccessToken(context.Context) (string, error) { return "t", nil }
func (sesionFijaAPI) Invalidate(string)                           {}
