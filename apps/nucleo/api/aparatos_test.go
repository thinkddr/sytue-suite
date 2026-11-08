// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAparatosApple(t *testing.T) {
	var vistos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		vistos = append(vistos, r.Method+" "+r.URL.Path+" "+string(b))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	h := NewHub(srv.URL, sesionFija("tok"), nil)
	ctx := context.Background()
	if err := h.RegistrarAparatoApple(ctx, "eu.sytue.mail", "ab01"); err != nil {
		t.Fatal(err)
	}
	if err := h.OlvidarAparatoApple(ctx, "ab01"); err != nil {
		t.Fatal(err)
	}
	if err := h.OlvidarAparatoApple(ctx, ""); err != nil {
		t.Fatal(err)
	}
	quiere := []string{
		`POST /api/v1/me/push/apns {"bundle":"eu.sytue.mail","token":"ab01"}`,
		`POST /api/v1/me/push/apns/forget {"token":"ab01"}`,
		`POST /api/v1/me/push/apns/forget {}`,
	}
	if len(vistos) != len(quiere) {
		t.Fatalf("peticiones = %q", vistos)
	}
	for i := range quiere {
		if vistos[i] != quiere[i] {
			t.Errorf("%d: %q, quiero %q", i, vistos[i], quiere[i])
		}
	}
}
