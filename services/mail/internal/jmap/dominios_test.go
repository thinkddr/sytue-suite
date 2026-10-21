// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Los dominios salen solo con el secreto de la suite, y llevan también el principal y
// el de los canales, sin repetir.
func TestDominiosSoloConElSecretoDeLaSuite(t *testing.T) {
	a := newAPI(&fakeStore{dominios: []string{"acme.eu", "sytue.eu"}}, nil, nil)
	a.cfg.HubInternalSecret = "s3creto"
	a.cfg.PrimaryDomain = "Sytue.eu"
	a.cfg.ChannelMailDomain = "canales.sytue.eu"
	r := chi.NewRouter()
	a.Mount(r, nil)
	pedir := func(secreto string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, RutaDominios, http.NoBody)
		req.Header.Set("Authorization", "Bearer "+secreto)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := pedir("otro"); w.Code != http.StatusUnauthorized {
		t.Fatalf("sin el secreto: %d", w.Code)
	}
	w := pedir("s3creto")
	if want := `{"domains":["acme.eu","canales.sytue.eu","sytue.eu"]}`; w.Code != http.StatusOK || w.Body.String() != want+"\n" {
		t.Fatalf("con el secreto: %d %q, quiero %s", w.Code, w.Body.String(), want)
	}
}
