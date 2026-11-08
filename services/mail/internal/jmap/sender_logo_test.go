// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/mail/internal/logo"
)

func TestSenderLogoNormalizaElDominioY404SinLogo(t *testing.T) {
	var pedidos []string
	a := &API{logos: &logo.Cache{
		Ahora: time.Now,
		Buscar: func(_ context.Context, d string) ([]byte, error) {
			pedidos = append(pedidos, d)
			if d == "con.com" {
				return []byte("\x89PNG"), nil
			}
			return nil, logo.ErrSinLogo
		},
	}}
	pedir := func(d string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		a.getSenderLogo(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sender-logo?d="+d, http.NoBody))
		return rec
	}
	if rec := pedir("con.com"); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" || rec.Body.String() != "\x89PNG" {
		t.Fatalf("con logo: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec := pedir("sin.com"); rec.Code != 404 || rec.Header().Get("Cache-Control") == "no-store" {
		t.Fatalf("sin logo: %d, cache %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	// Un subdominio cae en la entrada de su dominio: no hay segunda búsqueda.
	if rec := pedir("Corp.CON.com"); rec.Code != 200 {
		t.Fatalf("subdominio: %d", rec.Code)
	}
	// Buzón personal, IP o basura: 404 y no se busca nada.
	for _, d := range []string{"gmail.com", "127.0.0.1", "", "x"} {
		if rec := pedir(d); rec.Code != 404 {
			t.Fatalf("%q: %d, quiero 404", d, rec.Code)
		}
	}
	if want := []string{"con.com", "sin.com"}; len(pedidos) != 2 || pedidos[0] != want[0] || pedidos[1] != want[1] {
		t.Fatalf("búsquedas %q, quiero %q", pedidos, want)
	}
}
