// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestSoloAvisanLasEscriturasDelProducto: un 2xx de POST/PUT/PATCH/DELETE del cliente
// envuelto avisa; un GET, un 4xx, una consulta por POST, una escritura marcada SinAviso
// y lo que corre con SinAviso(ctx) no. Un cliente sin envoltorio (presencia, otro
// producto) no avisa nunca.
func TestSoloAvisanLasEscriturasDelProducto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/falla" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	var avisos atomic.Int32
	conAviso := New(srv.URL, nil, ConAvisoDeEscritura(nil, func() { avisos.Add(1) }))
	ctx := context.Background()
	hacer := func(c *Client, ctx context.Context, r Request) int32 {
		t.Helper()
		antes := avisos.Load()
		_, _ = JSON[map[string]any](ctx, c, r)
		return avisos.Load() - antes
	}
	for _, metodo := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if n := hacer(conAviso, ctx, Request{Method: metodo, Path: "/eventos"}); n != 1 {
			t.Fatalf("%s 2xx avisa %d veces", metodo, n)
		}
	}
	casos := map[string]struct {
		c   *Client
		ctx context.Context
		r   Request
	}{
		"GET":                      {conAviso, ctx, Request{Path: "/eventos"}},
		"4xx":                      {conAviso, ctx, Request{Method: http.MethodPost, Path: "/falla"}},
		"consulta por POST":        {conAviso, ctx, Request{Method: http.MethodPost, Path: "/buscar", Lectura: true}},
		"borrador":                 {conAviso, ctx, Request{Method: http.MethodPut, Path: "/drafts/1", SinAviso: true}},
		"lectura del widget":       {conAviso, SinAviso(ctx), Request{Method: http.MethodPost, Path: "/eventos"}},
		"PUT /presence, sin aviso": {New(srv.URL, nil, nil), ctx, Request{Method: http.MethodPut, Path: "/presence"}},
	}
	for nombre, c := range casos {
		if n := hacer(c.c, c.ctx, c.r); n != 0 {
			t.Errorf("%s avisa de una escritura (%d)", nombre, n)
		}
	}
	if _, err := PostJSONLectura[map[string]any](ctx, conAviso, "/people:resolve", map[string]string{"q": "a"}); err != nil || avisos.Load() != 4 {
		t.Fatalf("PostJSONLectura cuenta como escritura: %v (%d avisos)", err, avisos.Load())
	}
	// El latido de presencia de Docus (cada 12 s con un documento abierto) y pedir una
	// exportación no son escrituras para el widget.
	docs := NewDocs(srv.URL, nil, ConAvisoDeEscritura(nil, func() { avisos.Add(1) }))
	if _, err := docs.Presencia(ctx, "d1", "c1"); err != nil {
		t.Fatal(err)
	}
	if _, err := docs.ExportPDFSimple(ctx, "d1"); err != nil {
		t.Fatal(err)
	}
	if n := avisos.Load(); n != 4 {
		t.Fatalf("la presencia o la exportación de Docs avisan al widget (%d avisos)", n)
	}
	// El borrador del compositor de Rooms se autoguarda a ritmo de tecla.
	rooms := &Rooms{c: New(srv.URL, nil, ConAvisoDeEscritura(nil, func() { avisos.Add(1) }))}
	if err := rooms.PutDraft(ctx, "c1", "", "a medias"); err != nil {
		t.Fatal(err)
	}
	if n := avisos.Load(); n != 4 {
		t.Fatalf("el borrador de Rooms avisa al widget (%d avisos)", n)
	}
}
