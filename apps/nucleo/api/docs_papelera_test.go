// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPapeleraDeDocs: las cuatro rutas de la papelera, con el `?kind=` y el cursor de
// vaciar, y el código del 409 legible para decir por qué no se restauró.
func TestPapeleraDeDocs(t *testing.T) {
	var pedidos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidos = append(pedidos, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/trash":
			_, _ = io.WriteString(w, `{"documents":[{"id":"d1","kind":"spreadsheet","title":"Cuentas","deleted_at":"2026-09-28T10:00:00Z"}]}`)
		case "POST /api/v1/trash/d1/restore":
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"file_name_taken","message":"x"}}`)
		case "DELETE /api/v1/trash/d1":
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /api/v1/trash":
			_, _ = io.WriteString(w, `{"purged":49,"failed":1,"next":"2026-09-28T10:00:00Z|d9"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	d := NewDocs(srv.URL, nil, nil)
	ctx := context.Background()

	docs, err := d.Papelera(ctx, DocSheet)
	if err != nil || len(docs) != 1 || docs[0].DeletedAt == nil || docs[0].Kind != DocSheet {
		t.Fatalf("papelera = %+v, %v", docs, err)
	}
	var e *Error
	if err := d.Restaurar(ctx, "d1"); !errors.As(err, &e) || e.Code() != "file_name_taken" {
		t.Fatalf("restaurar con el nombre ocupado = %v", err)
	}
	if err := d.Purgar(ctx, "d1"); err != nil {
		t.Fatal(err)
	}
	fallidos, sig, err := d.Vaciar(ctx, DocSheet, "2026-09-28T09:00:00Z|d8")
	if err != nil || fallidos != 1 || sig != "2026-09-28T10:00:00Z|d9" {
		t.Fatalf("vaciar = %d, %q, %v", fallidos, sig, err)
	}
	quiero := []string{
		"GET /api/v1/trash?kind=spreadsheet",
		"POST /api/v1/trash/d1/restore?",
		"DELETE /api/v1/trash/d1?",
		"DELETE /api/v1/trash?after=2026-09-28T09%3A00%3A00Z%7Cd8&kind=spreadsheet",
	}
	for i, q := range quiero {
		if i >= len(pedidos) || pedidos[i] != q {
			t.Fatalf("pedidos = %q, quiero %q", pedidos, quiero)
		}
	}
}
