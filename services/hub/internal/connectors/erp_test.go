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

func TestERPConnectorSearchAndRecent(t *testing.T) {
	var gotAuth, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Errorf("ruta inesperada: %s (quiero /api/v1/search)", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[
			{
				"id":"rec-123",
				"modelo":"factura_cliente",
				"ref":"FAC-2026-001",
				"etiqueta":"Acme Corp - Licencias",
				"estado":"confirmado",
				"creado_en":"2026-09-12T10:00:00Z",
				"editado_en":"2026-09-12T10:30:00Z"
			}
		]}`))
	}))
	defer srv.Close()

	c := NewERPConnector(srv.URL, "https://erp.sytue.eu", time.Second)

	// 1. Search con término
	hits, err := c.Search(context.Background(), "tok-xyz", "Acme", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %d, quiero 1", len(hits))
	}
	h := hits[0]
	if h.Source != domain.SourceERP || h.Kind != "factura_cliente" {
		t.Errorf("hit source/kind inesperado: %+v", h)
	}
	if h.Title != "FAC-2026-001 · Acme Corp - Licencias" {
		t.Errorf("title inesperado: %q", h.Title)
	}
	if h.Snippet != "factura_cliente · confirmado" {
		t.Errorf("snippet inesperado: %q", h.Snippet)
	}
	expectedURL := "https://erp.sytue.eu/?m=factura_cliente&r=rec-123"
	if h.URL != expectedURL {
		t.Errorf("url inesperada: %q (esperaba %q)", h.URL, expectedURL)
	}
	if h.Icon != "erp" {
		t.Errorf("icon inesperado: %q", h.Icon)
	}
	if gotAuth != "Bearer tok-xyz" {
		t.Errorf("Authorization = %q, quiero Bearer tok-xyz", gotAuth)
	}
	if gotQuery != "Acme" {
		t.Errorf("query = %q, quiero Acme", gotQuery)
	}

	// 2. Recent sin término
	recentHits, err := c.Recent(context.Background(), "tok-xyz", 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recentHits) != 1 {
		t.Fatalf("recentHits = %d, quiero 1", len(recentHits))
	}
	if gotQuery != "" {
		t.Errorf("Recent query = %q, esperaba vacía", gotQuery)
	}
}
