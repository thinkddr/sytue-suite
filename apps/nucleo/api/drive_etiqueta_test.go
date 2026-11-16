// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Las tres llamadas de «Clasificar» van a las rutas del servicio y quitar la etiqueta es
// un DELETE sin cuerpo, no un PUT con el id vacío (que el servicio rechazaría con 400).
func TestDriveEtiquetas(t *testing.T) {
	var pedidas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		pedidas = append(pedidas, r.Method+" "+r.URL.Path+" "+string(cuerpo))
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"labels":[{"id":"l1","name":"Confidencial","rank":2,"block_external_share":true},{"id":"l2","name":"Interna","rank":1}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"f1"}`)
	}))
	defer srv.Close()
	ctx := context.Background()
	d := NewDrive(srv.URL, &sesionFalsa{tokens: []string{"tok"}}, nil)

	ls, err := d.SensitivityLabels(ctx)
	if err != nil || len(ls) != 2 || !ls[0].BlockExternalShare || ls[1].BlockExternalShare || ls[0].Name != "Confidencial" {
		t.Fatalf("SensitivityLabels = %+v, %v", ls, err)
	}
	if err := d.SetLabel(ctx, "f1", "l1"); err != nil {
		t.Fatal(err)
	}
	if err := d.SetLabel(ctx, "f1", ""); err != nil {
		t.Fatal(err)
	}
	quiero := []string{
		"GET /api/v1/sensitivity-labels ",
		`PUT /api/v1/files/f1/label {"label_id":"l1"}`,
		"DELETE /api/v1/files/f1/label ",
	}
	if len(pedidas) != len(quiero) {
		t.Fatalf("peticiones = %q", pedidas)
	}
	for i := range quiero {
		if pedidas[i] != quiero[i] {
			t.Errorf("petición %d = %q; quiero %q", i, pedidas[i], quiero[i])
		}
	}
}

func TestPrivateSensitivityTaxonomyPinsScopeWithoutSendingAFileOrAssignment(t *testing.T) {
	scope := storage.Scope{Tenant: "org", Subject: "owner", Product: "drive"}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		got, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader))
		if err != nil || got != scope || r.Method != http.MethodGet || r.URL.Path != "/api/v1/sensitivity-labels" || r.URL.RawQuery != "" || r.Header.Get("Cache-Control") != "no-store" || r.ContentLength > 0 {
			t.Error("private policy request lost scope, freshness or content privacy", err)
		}
		_, _ = io.WriteString(w, `{"labels":[{"id":"l1","name":"Restricted","rank":30,"block_external_share":true}]}`)
	}))
	defer server.Close()
	client := NewDrive(server.URL, nil, server.Client())
	labels, err := client.SensitivityLabelsForScope(t.Context(), scope)
	if err != nil || len(labels) != 1 || !labels[0].BlockExternalShare {
		t.Fatal(labels, err)
	}
	for _, invalid := range []storage.Scope{{Tenant: "org", Product: "drive"}, {Tenant: "org", Subject: "owner", Product: "mail"}} {
		if _, err := client.SensitivityLabelsForScope(t.Context(), invalid); err == nil {
			t.Fatal("invalid policy scope accepted")
		}
	}
	if calls != 1 {
		t.Fatal("invalid policy scope reached the network", calls)
	}
}
