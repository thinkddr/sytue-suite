// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package operador

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Solo un 204 de Sytue ID es «sí»; todo lo demás —un 403, un 500, no poder
// llegar, no tener token o no tener a quién preguntar— es «no».
func TestSytueID_SoloUn204Abre(t *testing.T) {
	var recibido map[string]any
	codigo := http.StatusNoContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/platform/moderation/access" || r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusTeapot)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&recibido)
		w.WriteHeader(codigo)
	}))
	defer srv.Close()
	ctx := context.Background()
	a := Acceso{Servicio: "cast", Accion: "report.resolve", Org: "acme", Objetivo: "r-1", Detalle: map[string]string{"action": "dismiss"}}

	if err := SytueID(srv.URL+"/")(ctx, "tok", a); err != nil {
		t.Fatalf("204: %v", err)
	}
	if recibido["service"] != "cast" || recibido["org"] != "acme" || recibido["target"] != "r-1" {
		t.Fatalf("cuerpo: %v", recibido)
	}
	for _, c := range []int{http.StatusForbidden, http.StatusUnauthorized} {
		codigo = c
		if err := SytueID(srv.URL)(ctx, "tok", a); !errors.Is(err, ErrNoEsOperador) {
			t.Fatalf("%d: %v", c, err)
		}
	}
	for _, c := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		codigo = c
		if err := SytueID(srv.URL)(ctx, "tok", a); err == nil {
			t.Fatalf("%d abre", c)
		}
	}
	codigo = http.StatusNoContent
	if err := SytueID(srv.URL)(ctx, "", a); !errors.Is(err, ErrNoEsOperador) {
		t.Fatalf("sin token: %v", err)
	}
	if err := SytueID("")(ctx, "tok", a); !errors.Is(err, ErrNoEsOperador) {
		t.Fatalf("sin Sytue ID: %v", err)
	}
	if !DeLaConsola([]string{"https://id", "sytue-platform"}) || DeLaConsola([]string{"sytue-cast"}) {
		t.Fatal("DeLaConsola")
	}
}
