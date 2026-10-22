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

// TestIntegrationsConnectorMapsHitsAndForwardsToken fija que el conector consulta
// GET /api/v1/search del gateway, PROPAGA el Bearer del usuario (passthrough) y mapea
// los ExternalHit del gateway al modelo canónico de Hub (origen "integrations",
// snippet con la etiqueta del SaaS al frente, la URL como id estable).
func TestIntegrationsConnectorMapsHitsAndForwardsToken(t *testing.T) {
	var gotAuth, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Errorf("ruta inesperada: %s (quiero /api/v1/search)", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":[
			{"title":"Ada Lovelace","url":"mailto:ada@contoso.com","snippet":"ada@contoso.com","kind":"person","source":"Microsoft 365"}
		]}`))
	}))
	defer srv.Close()

	c := NewIntegrationsConnector(srv.URL, time.Second)
	hits, err := c.Search(context.Background(), "tok-123", "ada", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %d, quiero 1", len(hits))
	}
	h := hits[0]
	if h.Source != domain.SourceIntegrations || h.Kind != "person" || h.Title != "Ada Lovelace" {
		t.Errorf("hit inesperado: %+v", h)
	}
	if h.URL != "mailto:ada@contoso.com" || h.ID != "mailto:ada@contoso.com" {
		t.Errorf("url/id inesperados: url=%q id=%q", h.URL, h.ID)
	}
	if h.Snippet != "Microsoft 365 · ada@contoso.com" || h.Icon != "Microsoft 365" {
		t.Errorf("mapeo de metadatos inesperado: snippet=%q icon=%q", h.Snippet, h.Icon)
	}
	// Passthrough del token del usuario (el gateway aplica RLS por la org del token).
	if gotAuth != "Bearer tok-123" {
		t.Errorf("Authorization propagada = %q, quiero Bearer tok-123", gotAuth)
	}
	if gotQuery != "ada" {
		t.Errorf("q propagada = %q, quiero ada", gotQuery)
	}
}

// TestIntegrationsConnectorRecentEmpty: el gateway no expone recientes ⇒ (nil, nil)
// sin tocar la red (degradación elegante del contrato Searcher).
func TestIntegrationsConnectorRecentEmpty(t *testing.T) {
	c := NewIntegrationsConnector("http://unused.invalid", time.Second)
	hits, err := c.Recent(context.Background(), "tok", 5)
	if err != nil || hits != nil {
		t.Fatalf("Recent = (%v, %v), quiero (nil, nil)", hits, err)
	}
}
