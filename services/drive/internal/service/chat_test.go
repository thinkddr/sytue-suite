// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// TestChatClientContract fija la costura drive→chat: ruta, parámetros y secreto que
// espera services/rooms (GET /internal/channel-access), y el mapeo de su respuesta.
// Si alguien cambia un lado sin el otro, los adjuntos de canal dejarían de servirse
// (fail-closed) y este test lo caza antes.
func TestChatClientContract(t *testing.T) {
	cid := uuid.New()
	var gotPath, gotAuth string
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotQuery = r.URL.Path, r.Header.Get("Authorization"), r.URL.Query()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewChatClient(srv.URL, "secreto")
	if c == nil {
		t.Fatal("NewChatClient con URL y secreto no debe ser nil")
	}
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	if err := c.CanAccessChannel(ctx, cid, "alice"); err != nil {
		t.Fatalf("204 debe ser acceso concedido, got %v", err)
	}
	if gotPath != "/internal/channel-access" {
		t.Fatalf("ruta = %q", gotPath)
	}
	if gotAuth != "Bearer secreto" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	for k, want := range map[string]string{"tenant": "acme", "channel": cid.String(), "subject": "alice"} {
		if got := gotQuery.Get(k); got != want {
			t.Fatalf("query %s = %q, quiero %q", k, got, want)
		}
	}
}

// TestChatClientMapping: 404/403 = no es miembro (ErrNotFound, sin filtrar
// existencia); 5xx o red caída = error (el llamador NO sirve el adjunto).
func TestChatClientMapping(t *testing.T) {
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	for _, code := range []int{http.StatusNotFound, http.StatusForbidden} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		}))
		err := NewChatClient(srv.URL, "s").CanAccessChannel(ctx, uuid.New(), "bob")
		srv.Close()
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("%d => %v, quiero ErrNotFound", code, err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	err := NewChatClient(srv.URL, "s").CanAccessChannel(ctx, uuid.New(), "bob")
	if err == nil || errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("500 => %v, quiero un error distinto de ErrNotFound (fail-closed, no «no miembro»)", err)
	}
}

// TestChatClientNilSinConfiguracion: sin URL o sin secreto NO hay cliente, y el
// servicio deja la superficie de adjuntos de canal cerrada.
func TestChatClientNilSinConfiguracion(t *testing.T) {
	if NewChatClient("", "secreto") != nil || NewChatClient("http://chat:8085", "") != nil {
		t.Fatal("sin URL o sin secreto, NewChatClient debe devolver nil (fail-closed)")
	}
	// Servicio sin verja: ni lee ni sube adjuntos de canal.
	s := &Service{}
	if _, err := s.ChannelFile(context.Background(), uuid.New(), uuid.New(), "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sin verja, leer un adjunto de canal => %v, quiero ErrNotFound", err)
	}
	if _, err := s.ChannelUploadFile(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}),
		uuid.New(), "alice", "a.txt", "text/plain", http.NoBody); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sin verja, subir un adjunto de canal => %v, quiero ErrNotFound", err)
	}
}
