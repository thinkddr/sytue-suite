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

func TestPagesConnectorSearchAndRecent(t *testing.T) {
	var gotAuth, gotQ string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Errorf("ruta inesperada: %s (quiero /api/v1/search)", r.URL.Path)
		}
		gotAuth, gotQ = r.Header.Get("Authorization"), r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[
			{"id":"6f1c0b7e-0000-4000-8000-000000000001","slug":"aurora","name":"Panadería Aurora",
			 "published_number":3,"updated_at":"2026-10-01T09:30:00.123456Z"},
			{"id":"6f1c0b7e-0000-4000-8000-000000000002","slug":"luna","name":"  ","updated_at":"nunca"}
		]}`))
	}))
	defer srv.Close()

	c := NewPagesConnector(srv.URL, "https://web.sytue.eu/web/", time.Second)
	if c.Source() != domain.SourcePages {
		t.Fatalf("Source() = %q", c.Source())
	}

	hits, err := c.Search(context.Background(), "tok", "aurora", 5)
	if err != nil || len(hits) != 2 {
		t.Fatalf("Search: %v, %d hits", err, len(hits))
	}
	if gotAuth != "Bearer tok" || gotQ != "aurora" {
		t.Errorf("Authorization=%q q=%q", gotAuth, gotQ)
	}
	h := hits[0]
	if h.Title != "Panadería Aurora" || h.Snippet != "aurora" || h.Kind != "site" || h.Icon != "pages" {
		t.Errorf("hit inesperado: %+v", h)
	}
	if h.URL != "https://web.sytue.eu/web/edit/6f1c0b7e-0000-4000-8000-000000000001" {
		t.Errorf("URL = %q", h.URL)
	}
	if h.Timestamp == nil || h.Timestamp.Year() != 2026 {
		t.Errorf("Timestamp = %v", h.Timestamp)
	}
	// Sin nombre, el slug da título; una fecha ilegible no tumba el resultado.
	if hits[1].Title != "luna" || hits[1].Timestamp != nil {
		t.Errorf("segundo hit inesperado: %+v", hits[1])
	}

	if _, err := c.Recent(context.Background(), "tok", 5); err != nil || gotQ != "" {
		t.Errorf("Recent: err=%v q=%q (debe ir sin término)", err, gotQ)
	}
}
