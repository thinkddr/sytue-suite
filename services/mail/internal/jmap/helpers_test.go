// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/services/mail/internal/config"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

func TestToRawArgsAndInto(t *testing.T) {
	// nil -> nil
	if toRawArgs(nil) != nil {
		t.Fatal("toRawArgs(nil) debe ser nil")
	}
	// valor serializable -> JSON -> into
	raw := toRawArgs(map[string]any{"limit": 7})
	var dst struct {
		Limit int `json:"limit"`
	}
	raw.into(&dst)
	if dst.Limit != 7 {
		t.Fatalf("into no deserializó: %d", dst.Limit)
	}
	// into sobre rawArgs vacío es no-op (no panic).
	var empty rawArgs
	empty.into(&dst)

	// valor no serializable -> nil.
	if toRawArgs(make(chan int)) != nil {
		t.Fatal("valor no serializable debe dar nil")
	}
}

func TestQueryInt(t *testing.T) {
	mk := func(q string) *http.Request {
		return httptest.NewRequest(http.MethodGet, "/?"+q, http.NoBody)
	}
	if got := queryInt(mk("limit=25"), 10); got != 25 {
		t.Fatalf("queryInt válido = %d", got)
	}
	if got := queryInt(mk(""), 10); got != 10 {
		t.Fatalf("queryInt ausente = %d (quiero default)", got)
	}
	if got := queryInt(mk("limit=abc"), 10); got != 10 {
		t.Fatalf("queryInt no numérico = %d (quiero default)", got)
	}
}

func TestErrUnsupportedMethod(t *testing.T) {
	err := errUnsupportedMethod("Foo/bar")
	if err == nil || !containsStr(err.Error(), "Foo/bar") {
		t.Fatalf("errUnsupportedMethod: %v", err)
	}
}

func TestMapErr(t *testing.T) {
	cases := []struct {
		in   error
		code int
	}{
		{domain.ErrNotFound, http.StatusNotFound},
		{domain.ErrMailboxExists, http.StatusConflict},
		{domain.ErrInvalidAddress, http.StatusBadRequest},
		{domain.ErrQuotaExceeded, http.StatusInsufficientStorage},
		{domain.ErrMessageTooLarge, http.StatusBadRequest},
	}
	for _, c := range cases {
		got := mapErr(c.in)
		if got == nil {
			t.Fatalf("mapErr(%v) devolvió nil", c.in)
		}
	}
	// Error desconocido se devuelve tal cual.
	base := errors.New("x")
	if !errors.Is(mapErr(base), base) {
		t.Fatal("error desconocido debe pasar sin traducir")
	}
}

func TestMax0(t *testing.T) {
	if max0(-1) != 0 || max0(5) != 5 {
		t.Fatal("max0 incorrecto")
	}
}

func TestHandleSessionRedirect(t *testing.T) {
	a := New(config.Config{PublicURL: "https://mail.sytue.eu"}, nil, nil, nil, slog.Default())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/jmap", http.NoBody)
	a.handleSessionRedirect(rec, req)
	if rec.Code != http.StatusPermanentRedirect {
		t.Fatalf("código = %d, quiero 308", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://mail.sytue.eu/jmap/session" {
		t.Fatalf("Location = %q", loc)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
