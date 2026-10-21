// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

func TestBasesConnectorSearchAndRecent(t *testing.T) {
	var gotAuth, gotQ string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Errorf("ruta inesperada: %s (quiero /api/v1/search)", r.URL.Path)
		}
		gotAuth, gotQ = r.Header.Get("Authorization"), r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[
			{"id":"6f1c0b7e-0000-4000-8000-000000000001","name":" Clientes ","description":"Altas y pedidos",
			 "updated_at":"2026-10-04T09:30:00.123456Z"},
			{"id":"6f1c0b7e-0000-4000-8000-000000000002","name":"Stock","updated_at":"nunca"}
		]}`))
	}))
	defer srv.Close()

	c := NewBasesConnector(srv.URL, "https://bases.sytue.eu/bases/", time.Second)
	if c.Source() != domain.SourceBases {
		t.Fatalf("Source() = %q", c.Source())
	}
	hits, err := c.Search(context.Background(), "tok", "cli", 5)
	if err != nil || len(hits) != 2 {
		t.Fatalf("Search: %v, %d hits", err, len(hits))
	}
	if gotAuth != "Bearer tok" || gotQ != "cli" {
		t.Errorf("Authorization=%q q=%q", gotAuth, gotQ)
	}
	h := hits[0]
	if h.Title != "Clientes" || h.Snippet != "Altas y pedidos" || h.Kind != "base" || h.Icon != "bases" {
		t.Errorf("hit inesperado: %+v", h)
	}
	if h.URL != "https://bases.sytue.eu/bases/base/6f1c0b7e-0000-4000-8000-000000000001" {
		t.Errorf("URL = %q", h.URL)
	}
	if h.Timestamp == nil || h.Timestamp.Year() != 2026 || hits[1].Timestamp != nil {
		t.Errorf("fechas: %v %v", h.Timestamp, hits[1].Timestamp)
	}
	if _, err := c.Recent(context.Background(), "tok", 5); err != nil || gotQ != "" {
		t.Errorf("Recent: err=%v q=%q (debe ir sin término)", err, gotQ)
	}
}
