// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// TestBasesHablaElCableDelServicio recorre el cliente contra un doble y comprueba método,
// ruta, consulta, cabecera de idempotencia y cuerpo: lo mismo que manda
// `packages/api-clients/src/bases/index.ts`.
func TestBasesHablaElCableDelServicio(t *testing.T) {
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		linea := r.Method + " " + r.URL.RequestURI() + " " + string(cuerpo)
		if k := r.Header.Get("Idempotency-Key"); k != "" {
			linea += " op=" + k
		}
		vistas = append(vistas, linea)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/bases":
			_, _ = io.WriteString(w, `[{"id":"b1","name":"Clientes","description":"","owner_sub":"u1","role":"owner","created_at":"2026-10-01T09:00:00Z","updated_at":"2026-10-02T09:00:00Z"},
{"id":"b2","name":"Pedidos","role":"","host_org":"acme","created_at":"2026-10-01T09:00:00Z","updated_at":"2026-10-02T09:00:00Z"}]`)
		case "POST /api/v1/bases", "GET /api/v1/bases/b1":
			_, _ = io.WriteString(w, `{"id":"b1","name":"Clientes","role":"owner","created_at":"2026-10-01T09:00:00Z","updated_at":"2026-10-02T09:00:00Z"}`)
		case "GET /api/v1/tables/t1/records":
			_, _ = io.WriteString(w, `{"fields":[{"id":"f1","table_id":"t1","name":"Nombre","kind":"text","position":0}],
"records":[{"id":"r1","table_id":"t1","data":{"f1":"Ana","f2":3.5,"f3":true,"f4":["a","b"],"f5":null},"links":{"f6":["r9"]},"rev":2,
"created_at":"2026-10-01T09:00:00Z","updated_at":"2026-10-02T09:00:00Z"}],"role":"editor","next_cursor":"c2","recalculating":true}`)
		case "PUT /api/v1/records/r1/cells/f1":
			_, _ = io.WriteString(w, `{"record":{"id":"r1","table_id":"t1","data":{"f1":"Eva"},"links":{},"rev":3,"created_at":"2026-10-01T09:00:00Z","updated_at":"2026-10-02T09:00:00Z"},"conflict":true,"replayed":false}`)
		case "GET /api/v1/groups":
			_, _ = io.WriteString(w, `{"groups":["ventas"]}`)
		case "GET /api/v1/tables/t1/export":
			_, _ = io.WriteString(w, "Nombre\nAna\n")
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	b, ctx := NewBases(srv.URL, nil, nil), context.Background()

	bases, err := b.ListBases(ctx)
	if err != nil || len(bases) != 2 || bases[0].Role != BasesOwner || bases[1].HostOrg != "acme" || bases[0].UpdatedAt.IsZero() {
		t.Fatalf("ListBases = %+v, %v", bases, err)
	}
	if nueva, err := b.CreateBase(ctx, "Clientes"); err != nil || nueva.ID != "b1" {
		t.Fatalf("CreateBase = %+v, %v", nueva, err)
	}
	if _, err := b.GetBase(ctx, "b1"); err != nil {
		t.Fatal(err)
	}
	pag, err := b.ListRecords(ctx, "t1", "v1", "c1", 100)
	if err != nil || pag.NextCursor != "c2" || !pag.Recalculating || pag.Role != BasesEditor || len(pag.Records) != 1 {
		t.Fatalf("ListRecords = %+v, %v", pag, err)
	}
	r := pag.Records[0]
	if r.Data["f1"] != "Ana" || r.Data["f2"] != 3.5 || r.Data["f3"] != true || r.Data["f5"] != nil || !slices.Equal(r.Links["f6"], []string{"r9"}) {
		t.Fatalf("el registro no trae sus valores: %+v", r)
	}
	if _, err := b.ListRecords(ctx, "t1", "", "", 0); err != nil {
		t.Fatal(err)
	}
	res, err := b.WriteCell(ctx, "r1", "f1", BasesCell{Value: "Eva", Expected: "Ana"}, "op-1")
	if err != nil || !res.Conflict || res.Record.Rev != 3 {
		t.Fatalf("WriteCell = %+v, %v", res, err)
	}
	if _, err := b.WriteCell(ctx, "r1", "f1", BasesCell{Links: []string{}, Expected: []string{"r9"}}, "op-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.WriteCell(ctx, "r1", "f1", BasesCell{}, "op-3"); err != nil {
		t.Fatal(err)
	}
	grupos, err := b.MyGroups(ctx)
	if err != nil || !slices.Equal(grupos, []string{"ventas"}) {
		t.Fatalf("MyGroups = %v, %v", grupos, err)
	}
	csv, err := b.Export(ctx, "t1", "csv")
	if err != nil {
		t.Fatal(err)
	}
	datos, _ := io.ReadAll(csv)
	_ = csv.Close()
	if string(datos) != "Nombre\nAna\n" {
		t.Fatalf("Export = %q", datos)
	}
	for _, f := range []func() error{
		func() error { return b.UpdateBase(ctx, "b1", "Clientes 2", "d") },
		func() error { return b.DeleteBase(ctx, "b1") },
		func() error { return b.RenameTable(ctx, "t1", "Altas") },
		func() error { return b.DeleteTable(ctx, "t1") },
		func() error { return b.DeleteRecord(ctx, "r1", false) },
		func() error { return b.DeleteRecord(ctx, "r1", true) },
		func() error { return b.DeleteView(ctx, "v1") },
		func() error { return b.TransferBase(ctx, "b1", "eva@acme.example") },
		func() error { return b.RevokePermission(ctx, "b1", "p1") },
	} {
		if err := f(); err != nil {
			t.Fatal(err)
		}
	}

	quiero := []string{
		"GET /api/v1/bases ",
		`POST /api/v1/bases {"name":"Clientes","description":""}`,
		"GET /api/v1/bases/b1 ",
		"GET /api/v1/tables/t1/records?cursor=c1&limit=100&view=v1 ",
		"GET /api/v1/tables/t1/records ",
		`PUT /api/v1/records/r1/cells/f1?v=1 {"expected":"Ana","value":"Eva"} op=op-1`,
		`PUT /api/v1/records/r1/cells/f1?v=1 {"expected":["r9"],"links":[]} op=op-2`,
		`PUT /api/v1/records/r1/cells/f1?v=1 {"expected":null,"value":null} op=op-3`,
		"GET /api/v1/groups ",
		"GET /api/v1/tables/t1/export?format=csv ",
		`PATCH /api/v1/bases/b1 {"name":"Clientes 2","description":"d"}`,
		"DELETE /api/v1/bases/b1 ",
		`PATCH /api/v1/tables/t1 {"name":"Altas"}`,
		"DELETE /api/v1/tables/t1 ",
		"DELETE /api/v1/records/r1 ",
		"DELETE /api/v1/records/r1?force=true ",
		"DELETE /api/v1/views/v1 ",
		`POST /api/v1/bases/b1/transfer {"email":"eva@acme.example"}`,
		"DELETE /api/v1/bases/b1/permissions/p1 ",
	}
	if !slices.Equal(vistas, quiero) {
		t.Errorf("peticiones:\n%q\nquiero:\n%q", vistas, quiero)
	}
}
