// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package scimout

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCreateAndFindUser(t *testing.T) {
	m := newSCIMMock()
	srv := httptest.NewServer(m)
	defer srv.Close()
	c := NewClient(srv.Client(), srv.URL)
	ctx := t.Context()

	id, err := c.CreateUser(ctx, UserInput{UserName: "alice@acme.test", Email: "alice@acme.test", Active: true})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if id == "" {
		t.Fatal("CreateUser devolvió id vacío")
	}
	if m.posts != 1 {
		t.Fatalf("POSTs = %d, quiero 1", m.posts)
	}

	got, found, err := c.FindUser(ctx, "alice@acme.test")
	if err != nil || !found || got != id {
		t.Fatalf("FindUser(alice) = (%q, %v, %v), quiero (%q, true, nil)", got, found, err, id)
	}
	if _, found, _ := c.FindUser(ctx, "nadie@acme.test"); found {
		t.Fatal("FindUser(nadie) no debería encontrar nada")
	}
}

func TestSetUserActiveDeactivates(t *testing.T) {
	m := newSCIMMock()
	srv := httptest.NewServer(m)
	defer srv.Close()
	c := NewClient(srv.Client(), srv.URL)

	uid := m.seedUser("bob@acme.test")
	if err := c.SetUserActive(t.Context(), uid, false); err != nil {
		t.Fatalf("SetUserActive: %v", err)
	}
	m.mu.Lock()
	active := m.users[uid]["active"]
	m.mu.Unlock()
	if active != false {
		t.Fatalf("active tras desactivar = %v, quiero false", active)
	}
	if m.patches != 1 {
		t.Fatalf("PATCHes = %d, quiero 1", m.patches)
	}
}

func TestRetryOn429RespectsRetryAfter(t *testing.T) {
	m := newSCIMMock()
	m.fails = []failResp{{status: 429, retryAfter: "2"}} // un 429 con Retry-After: 2 antes del éxito.
	srv := httptest.NewServer(m)
	defer srv.Close()
	c := NewClient(srv.Client(), srv.URL)

	var slept time.Duration
	c.sleep = func(d time.Duration) { slept = d } // captura la espera sin dormir de verdad.

	id, err := c.CreateUser(t.Context(), UserInput{UserName: "carol@acme.test", Active: true})
	if err != nil {
		t.Fatalf("CreateUser tras 429: %v", err)
	}
	if id == "" {
		t.Fatal("no se creó el usuario tras reintentar el 429")
	}
	if slept != 2*time.Second {
		t.Fatalf("espera = %v, quiero 2s (Retry-After respetado)", slept)
	}
	if m.posts != 1 {
		t.Fatalf("POSTs = %d, quiero 1 (el 429 no llegó al handler)", m.posts)
	}
}

func TestRetryOn5xxThenSucceeds(t *testing.T) {
	m := newSCIMMock()
	m.fails = []failResp{{status: 503}}
	srv := httptest.NewServer(m)
	defer srv.Close()
	c := NewClient(srv.Client(), srv.URL)
	c.sleep = func(time.Duration) {} // no dormir en el backoff exponencial.

	if _, err := c.CreateUser(t.Context(), UserInput{UserName: "dave@acme.test", Active: true}); err != nil {
		t.Fatalf("CreateUser tras 503: %v", err)
	}
	if m.posts != 1 {
		t.Fatalf("POSTs = %d, quiero 1", m.posts)
	}
}

func TestConflictSurfacesErrConflict(t *testing.T) {
	m := newSCIMMock()
	m.fails = []failResp{{status: 409}}
	srv := httptest.NewServer(m)
	defer srv.Close()
	c := NewClient(srv.Client(), srv.URL)

	if _, err := c.CreateUser(t.Context(), UserInput{UserName: "erin@acme.test", Active: true}); !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, quiero ErrConflict", err)
	}
	if m.posts != 0 {
		t.Fatalf("un 409 no debe crear nada; POSTs = %d", m.posts)
	}
}
