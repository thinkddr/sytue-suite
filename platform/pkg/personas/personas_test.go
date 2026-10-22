// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package personas

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDirectorioID(t *testing.T) {
	var visto struct {
		auth  string
		ruta  string
		query string
		id    string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visto.auth = r.Header.Get("Authorization")
		visto.ruta = r.URL.Path
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		visto.query, visto.id = in["query"], in["user_id"]
		switch in["query"] {
		case "@nadie":
			w.WriteHeader(http.StatusNotFound)
		case "???":
			w.WriteHeader(http.StatusBadRequest)
		case "@rapido":
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			_ = json.NewEncoder(w).Encode(map[string]string{"user_id": "u-1", "name": "Ana", "username": "ana"})
		}
	}))
	defer srv.Close()

	d := NewDirectorioID(srv.URL + "/")
	if d == nil || NewDirectorioID("") != nil {
		t.Fatal("NewDirectorioID: vacío = nil, no vacío = cliente")
	}
	ctx := ConBearer(context.Background(), "tok")

	p, err := d.Resolver(ctx, "@ana")
	if err != nil || p.UserID != "u-1" || p.Username != "ana" {
		t.Fatalf("Resolver: %+v %v", p, err)
	}
	if visto.auth != "Bearer tok" || visto.ruta != "/api/v1/people:resolve" || visto.query != "@ana" {
		t.Fatalf("petición: %+v", visto)
	}
	if _, err := d.PorID(ctx, "u-1"); err != nil || visto.id != "u-1" {
		t.Fatalf("PorID: %v %+v", err, visto)
	}
	for consulta, esperado := range map[string]error{"@nadie": ErrNoExiste, "???": ErrEntradaInvalida, "@rapido": ErrDemasiadas} {
		if _, err := d.Resolver(ctx, consulta); !errors.Is(err, esperado) {
			t.Errorf("%s: %v, esperaba %v", consulta, err, esperado)
		}
	}
	// Sin token en el contexto no se pregunta: el límite de Sytue ID es por persona.
	if _, err := d.Resolver(context.Background(), "@ana"); !errors.Is(err, ErrSinSesion) {
		t.Fatalf("sin bearer: %v", err)
	}
	if BearerDe(ConBearer(context.Background(), "")) != "" {
		t.Fatal("ConBearer con vacío no debería dejar nada")
	}
}

func TestVisible(t *testing.T) {
	if (Persona{Name: " Ana "}).Visible() != " Ana " || (Persona{Username: "ana"}).Visible() != "@ana" || (Persona{UserID: "u"}).Visible() != "u" {
		t.Fatal("Visible")
	}
}
