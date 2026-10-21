// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// El directorio pregunta a Sytue ID con el token de QUIEN LLAMA —nunca con uno de
// servicio, o se saltaría el límite por persona— y traduce sus respuestas a los
// errores de Rooms sin confundir «no existe» con «vas demasiado deprisa».
func TestDirectorioID(t *testing.T) {
	var gotAuth string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotBody = map[string]string{}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		if r.URL.Path != "/api/v1/people:resolve" {
			http.NotFound(w, r)
			return
		}
		switch gotBody["query"] + gotBody["user_id"] {
		case "@bea":
			_, _ = w.Write([]byte(`{"user_id":"u-9","name":"Bea","username":"bea"}`))
		case "rapido":
			w.WriteHeader(http.StatusTooManyRequests)
		case "raro":
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	d := NewDirectorioID(srv.URL + "/")
	ctx := WithBearer(context.Background(), "tok-de-ana")

	p, err := d.Resolver(ctx, "@bea")
	if err != nil || p.UserID != "u-9" || p.Username != "bea" {
		t.Fatalf("Resolver: %+v %v", p, err)
	}
	if gotAuth != "Bearer tok-de-ana" {
		t.Fatalf("debe viajar el token de quien llama: %q", gotAuth)
	}
	if _, err := d.PorID(ctx, "nadie"); !errors.Is(err, domain.ErrNotFound) || gotBody["user_id"] != "nadie" {
		t.Fatalf("PorID de quien no existe: %v (%v)", err, gotBody)
	}
	if _, err := d.Resolver(ctx, "rapido"); !errors.Is(err, domain.ErrTooManyLookups) {
		t.Fatalf("429: %v", err)
	}
	if _, err := d.Resolver(ctx, "raro"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("400: %v", err)
	}
	// Sin token no se pregunta: fail-closed.
	if _, err := d.Resolver(context.Background(), "@bea"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("sin token: %v", err)
	}
	if NewDirectorioID("") != nil {
		t.Fatal("sin URL no hay directorio")
	}
}
