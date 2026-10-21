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

func TestDocActividadTextoComoLaWeb(t *testing.T) {
	casos := []struct {
		a    DocActividad
		quep string
		desc string
	}{
		{DocActividad{AuthorSub: "user-ada-123456", Count: 3, Paths: []string{"sheets/h1/cells/B2", "sheets/h1/cells/B3"}}, "B2, B3", "user-ada — B2, B3 ×3"},
		{DocActividad{AuthorSub: "u", Count: 1, Paths: []string{"slides/s1/boxes/b1/html"}}, "boxes·html", "u — boxes·html ×1"},
		{DocActividad{Kind: "set", Key: "doc:sheet", Count: 1, Paths: []string{"sheets/h1/freeze"}}, "freeze", "set"},
		{DocActividad{Kind: "set", Key: "doc:sheet", Count: 2}, "doc:sheet", ""},
		{DocActividad{Kind: "insert", Count: 5}, "insert", ""},
		{DocActividad{Paths: []string{"a", "b", "c", "d"}}, "a, b, c, …", ""},
	}
	for _, c := range casos {
		if got := c.a.QueToco(); got != c.quep {
			t.Errorf("QueToco(%+v) = %q, quiero %q", c.a, got, c.quep)
		}
	}
	if got := casos[0].a.Descripcion(); got != casos[0].desc {
		t.Errorf("Descripcion = %q, quiero %q", got, casos[0].desc)
	}
}

func TestDocsActividad(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/documents/d1/audit" || r.URL.Query().Get("limit") != "200" {
			t.Errorf("petición inesperada: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"entries":[{"seq_desde":4,"seq_hasta":6,"author_sub":"u-ada","kind":"set","key":"doc:sheet","count":3,"at":"2026-10-03T10:00:00Z","paths":["sheets/h1/cells/A1"]}]}`)
	}))
	defer srv.Close()
	es, err := NewDocs(srv.URL, nil, nil).Actividad(context.Background(), "d1", 0)
	if err != nil || len(es) != 1 || es[0].Count != 3 || es[0].QueToco() != "A1" {
		t.Fatalf("Actividad = %+v, %v", es, err)
	}
}
