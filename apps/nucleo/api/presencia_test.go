// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPresencia(t *testing.T) {
	var puesto string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/presence":
			if q := r.URL.Query().Get("subjects"); q != "ada,grace" {
				t.Errorf("subjects: %q", q)
			}
			_, _ = io.WriteString(w, `{"presence":[{"subject":"ada","state":"busy","status":"en una reunión","last_seen":"2026-09-22T10:00:00Z"}]}`)
		case "PUT /api/v1/presence":
			cuerpo, _ := io.ReadAll(r.Body)
			puesto = string(cuerpo)
			_, _ = io.WriteString(w, `{"ok":true,"heartbeat_seconds":22}`)
		case "DELETE /api/v1/presence":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	h, ctx := NewHub(srv.URL, nil, nil), context.Background()

	quien, err := h.Presencias(ctx, "ada", "grace")
	if err != nil || len(quien) != 1 || quien[0].Estado != NoMolestar || quien[0].Mensaje != "en una reunión" {
		t.Fatalf("presencias: %+v, %v", quien, err)
	}

	// El latido tiene que ir por PUT (el servicio no tiene POST /presence) y devolver
	// cada cuánto repetirlo, que es lo que evita caducar a los 45 s.
	cada, err := h.PonerPresencia(ctx, NoMolestar, "en una reunión")
	if err != nil || cada != 22*time.Second {
		t.Fatalf("poner: %v, %v", cada, err)
	}
	if puesto != `{"state":"busy","status":"en una reunión"}` {
		t.Errorf("mandó %s", puesto)
	}
	if err := h.Desconectar(ctx); err != nil {
		t.Fatal(err)
	}
}

// Más de MaxPresencias personas van en varias consultas, porque el Hub contesta 400 a
// partir de 200, y lo de todas se junta.
//
// VISTO EN ROJO (2026-09-26, en el frontal, saboteado a mano): con una sola consulta
// sale una petición de 250 sujetos.
func TestLasPresenciasVanDeDoscientosEnDoscientos(t *testing.T) {
	var tandas []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subs := strings.Split(r.URL.Query().Get("subjects"), ",")
		tandas = append(tandas, len(subs))
		if len(subs) > MaxPresencias {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"presence":[{"subject":"`+subs[0]+`","state":"online"}]}`)
	}))
	defer srv.Close()
	subs := make([]string, 250)
	for i := range subs {
		subs[i] = "s" + strconv.Itoa(i)
	}
	ps, err := NewHub(srv.URL, nil, nil).Presencias(context.Background(), subs...)
	if err != nil || len(ps) != 2 || ps[1].Subject != "s200" || len(tandas) != 2 || tandas[0] != 200 || tandas[1] != 50 {
		t.Errorf("tandas %v, presencias %+v, %v", tandas, ps, err)
	}
}
