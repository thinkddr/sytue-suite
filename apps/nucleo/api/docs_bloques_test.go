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

func TestDocsBloquesHablaElCableDelServicio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.RequestURI() {
		case "GET /api/v1/blocks?kind=presentation&locale=es":
			_, _ = io.WriteString(w, `{"blocks":[{"key":"cita","kind":"presentation","name":"Cita","description":""}],"org_blocks":[{"key":"org:d1","name":"Cierre","versions":[{"no":2,"id":"v2"}]}]}`)
		case "GET /api/v1/blocks/org:d1?locale=es":
			_, _ = io.WriteString(w, `{"key":"org:d1","kind":"presentation","content":"{\"boxes\":[]}","version":2,"brand_version":3}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.RequestURI())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	d, ctx := NewDocs(srv.URL, nil, nil), context.Background()
	l, err := d.Blocks(ctx, DocSlides, "es")
	if err != nil || len(l.Blocks) != 1 || len(l.OrgBlocks) != 1 || l.OrgBlocks[0].Versions[0].No != 2 {
		t.Fatalf("lista: %+v, %v", l, err)
	}
	c, err := d.Block(ctx, "org:d1", "es")
	if err != nil || c != `{"boxes":[]}` {
		t.Fatalf("bloque: %q, %v", c, err)
	}
}
