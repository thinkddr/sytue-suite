// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// El catálogo de la organización habla con las rutas y los nombres del servicio.
//
// VISTO EN ROJO (2026-09-24, en el frontal, saboteado a mano): con `"fileId"` en
// CreateCustomEmoji, el cuerpo no casa.
func TestElCatalogoDeLaOrganizacion(t *testing.T) {
	var pedidas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := io.ReadAll(r.Body)
		pedidas = append(pedidas, r.Method+" "+r.URL.Path+" "+string(c))
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"emoji":[{"name":"baile","file_id":"f1","content_type":"image/gif"}]}`)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"name":"hola","file_id":"f2","content_type":"image/gif"}`)
	}))
	defer srv.Close()
	r, ctx := NewRooms(srv.URL, nil, nil), context.Background()
	es, err := r.CustomEmoji(ctx)
	if err != nil || len(es) != 1 || es[0].FileID != "f1" || es[0].ContentType != "image/gif" {
		t.Fatalf("catálogo: %+v, %v", es, err)
	}
	if e, err := r.CreateCustomEmoji(ctx, "hola", "f2"); err != nil || e.Name != "hola" {
		t.Errorf("crear: %+v, %v", e, err)
	}
	quiero := "GET /api/v1/emoji \n" + `POST /api/v1/emoji {"file_id":"f2","name":"hola"}`
	if got := strings.Join(pedidas, "\n"); got != quiero {
		t.Errorf("pedidas:\n%s", got)
	}
	rutas := webref.Read(t, "services/rooms/internal/http/server.go")
	for _, ruta := range []string{`api.Get("/emoji", h.ListCustomEmoji)`, `api.Post("/emoji", h.CreateCustomEmoji)`} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("el servicio ya no sirve %s", ruta)
		}
	}
}
