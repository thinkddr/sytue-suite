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

// TestCastDiapositivasHablaElCable comprueba método, ruta y cuerpo contra lo que
// contesta services/cast/internal/http/diapositivas.go.
func TestCastDiapositivasHablaElCable(t *testing.T) {
	var cuerpo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		switch r.Method + " " + r.URL.Path {
		case "PUT /api/v1/lives/v2/slide":
			cuerpo = string(b)
			_, _ = io.WriteString(w, `{"ref":"docs:d1","index":2,"total":9,"title":"Cierre","started_at":"2026-10-04T10:00:00Z","updated_at":"2026-10-04T10:05:00Z","marks":[{"at_ms":1500,"index":1,"title":"Plan"}]}`)
		case "GET /api/v1/videos/v2/slides":
			_, _ = io.WriteString(w, `{"presentation":null}`)
		case "GET /api/v1/videos/v3/slides":
			_, _ = io.WriteString(w, `{"presentation":{"index":0,"total":1,"title":"A","marks":[]}}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c, ctx := NewCast(srv.URL, nil, nil), context.Background()

	e, err := c.AvanzarDiapositiva(ctx, "v2", CastPosicion{Index: 2, Total: 9, Title: "Cierre", Ref: "docs:d1"})
	if err != nil || e.Index != 2 || e.Total != 9 || len(e.Marks) != 1 || e.Marks[0].AtMS != 1500 {
		t.Fatalf("avanzar: %+v, %v", e, err)
	}
	if cuerpo != `{"index":2,"total":9,"title":"Cierre","ref":"docs:d1"}` {
		t.Errorf("cuerpo: %s", cuerpo)
	}
	if p, err := c.Diapositivas(ctx, "v2"); err != nil || p != nil {
		t.Errorf("sin presentación: %+v, %v", p, err)
	}
	if p, err := c.Diapositivas(ctx, "v3"); err != nil || p == nil || p.Title != "A" {
		t.Errorf("con presentación: %+v, %v", p, err)
	}
}
