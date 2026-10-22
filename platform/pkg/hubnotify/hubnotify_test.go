// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubnotify_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
)

func TestClient_Send(t *testing.T) {
	var gotHeader string
	var gotBody hubnotify.Notification

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/notifications" {
			t.Errorf("ruta inesperada: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		gotHeader = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode error: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := hubnotify.New(srv.URL, "secret-123")
	if c == nil {
		t.Fatal("New devolvió nil")
	}

	notif := hubnotify.Notification{
		Tenant:  "acme",
		Subject: "usr-456",
		Source:  "mail",
		Type:    "mail.message.received",
		Title:   "Nuevo correo",
		Body:    "Hola mundo",
		URL:     "https://mail.sytue.localhost/#msg-1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Send(ctx, notif); err != nil {
		t.Fatalf("Send falló: %v", err)
	}

	if gotHeader != "Bearer secret-123" {
		t.Errorf("Authorization header = %q, want 'Bearer secret-123'", gotHeader)
	}
	if gotBody.Tenant != "acme" || gotBody.Subject != "usr-456" || gotBody.Title != "Nuevo correo" {
		t.Errorf("cuerpo inesperado: %+v", gotBody)
	}
	if gotBody.Level != "info" {
		t.Errorf("Level por defecto debería ser 'info', obtuve %q", gotBody.Level)
	}
}

func TestClient_NilClient(t *testing.T) {
	var c *hubnotify.Client
	if err := c.Send(context.Background(), hubnotify.Notification{}); err != nil {
		t.Errorf("Send en cliente nil debería devolver nil, obtuve %v", err)
	}
	c.SendAsync(context.Background(), hubnotify.Notification{}) // No debe hacer pánico
}

func TestClient_EmptyBaseURL(t *testing.T) {
	if c := hubnotify.New("", "secret"); c != nil {
		t.Errorf("New con baseURL vacía debería devolver nil, obtuve %+v", c)
	}
}

func TestMencionados(t *testing.T) {
	got := hubnotify.Mencionados("@ana@x.es hola ana@x.es y (@Ana@X.es) y @luis@y.org.")
	if len(got) != 2 || got[0] != "ana@x.es" || got[1] != "luis@y.org" {
		t.Fatalf("Mencionados = %v", got)
	}
}
