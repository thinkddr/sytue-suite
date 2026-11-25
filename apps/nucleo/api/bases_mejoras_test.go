// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// Duplicar, el pie, el defecto de un campo, exportar lo que se ve y el día del alta
// hablan el mismo cable que `packages/api-clients/src/bases/index.ts`.
func TestBasesDuplicarResumirDefectoYExportarLaVista(t *testing.T) {
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		vistas = append(vistas, r.Method+" "+r.URL.RequestURI()+" "+string(cuerpo))
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/records/r1/duplicate", "POST /api/v1/tables/t1/records":
			_, _ = io.WriteString(w, `{"id":"r2","table_id":"t1","data":{"f1":"Ana"},"links":{},"rev":1,"created_at":"2026-10-01T09:00:00Z","updated_at":"2026-10-01T09:00:00Z"}`)
		case "GET /api/v1/tables/t1/summary":
			_, _ = io.WriteString(w, `{"count":3,"fields":{"f2":{"filled":2,"sum":15.5,"avg":7.75,"min":5.5,"max":10},"f1":{"filled":3}}}`)
		case "GET /api/v1/tables/t1/export":
			_, _ = io.WriteString(w, "Nombre\nAna\n")
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	b, ctx := NewBases(srv.URL, nil, nil), context.Background()

	if r, err := b.DuplicateRecord(ctx, "r1"); err != nil || r.ID != "r2" {
		t.Fatalf("DuplicateRecord = %+v, %v", r, err)
	}
	lectura := BasesLectura{Q: "ana", Today: "2026-11-25"}
	res, err := b.Summary(ctx, "t1", "v1", lectura)
	if err != nil || res.Count != 3 || res.Fields["f2"].Sum == nil || *res.Fields["f2"].Sum != 15.5 || res.Fields["f1"].Sum != nil {
		t.Fatalf("Summary = %+v, %v", res, err)
	}
	if err := b.SetFieldDefault(ctx, "f3", BasesDefaultToday); err != nil {
		t.Fatal(err)
	}
	if err := b.SetFieldDefault(ctx, "f3", nil); err != nil {
		t.Fatal(err)
	}
	cuerpo, err := b.ExportCon(ctx, "t1", "csv", "v1", lectura)
	if err != nil {
		t.Fatal(err)
	}
	_ = cuerpo.Close()
	if _, err := b.CreateRecord(ctx, "t1", nil, nil); err != nil {
		t.Fatal(err)
	}

	hoy := time.Now().Format(time.DateOnly)
	quiero := []string{
		"POST /api/v1/records/r1/duplicate ",
		"GET /api/v1/tables/t1/summary?q=ana&today=2026-11-25&view=v1 ",
		`PATCH /api/v1/fields/f3 {"default":"today"}`,
		`PATCH /api/v1/fields/f3 {"default":null}`,
		"GET /api/v1/tables/t1/export?format=csv&q=ana&today=2026-11-25&view=v1 ",
		`POST /api/v1/tables/t1/records?today=` + hoy + ` {"data":{}}`,
	}
	if !slices.Equal(vistas, quiero) {
		t.Fatalf("el cable:\n%s\nquería:\n%s", strings.Join(vistas, "\n"), strings.Join(quiero, "\n"))
	}
}
