// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// BasesConnector consulta la API de Sytue Bases y normaliza las bases de la persona al
// modelo canónico de Hub. Bases aplica su ACL con el token de quien busca: solo salen las
// bases a las que tiene acceso. Es de organizaciones: una cuenta personal recibe un 403
// de su verja, que la búsqueda federada cuenta como un origen que no aporta.
type BasesConnector struct {
	client    *httpClient
	publicURL string
}

// NewBasesConnector construye el conector de Bases.
func NewBasesConnector(apiBaseURL, publicURL string, timeout time.Duration) *BasesConnector {
	return &BasesConnector{
		client:    newHTTPClient(apiBaseURL, timeout),
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

// Source implementa Searcher.
func (c *BasesConnector) Source() domain.Source { return domain.SourceBases }

type basesSearchResponse struct {
	Items []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		UpdatedAt   string `json:"updated_at"`
	} `json:"items"`
}

// Search llama a GET /api/v1/search?q=… de Bases y normaliza.
func (c *BasesConnector) Search(ctx context.Context, token, query string, limit int) ([]domain.SearchHit, error) {
	return c.consultar(ctx, token, query, limit)
}

// Recent reusa el MISMO endpoint sin término: Bases devuelve las editadas más recientemente.
func (c *BasesConnector) Recent(ctx context.Context, token string, limit int) ([]domain.SearchHit, error) {
	return c.consultar(ctx, token, "", limit)
}

func (c *BasesConnector) consultar(ctx context.Context, token, query string, limit int) ([]domain.SearchHit, error) {
	q := url.Values{}
	if query != "" {
		q.Set("q", query)
	}
	q.Set("limit", strconv.Itoa(limit))

	var resp basesSearchResponse
	if err := c.client.getJSON(ctx, token, "/api/v1/search", q, &resp); err != nil {
		return nil, err
	}
	hits := make([]domain.SearchHit, 0, len(resp.Items))
	for _, it := range resp.Items {
		hit := domain.SearchHit{
			Source:  domain.SourceBases,
			Kind:    "base",
			ID:      it.ID,
			Title:   strings.TrimSpace(it.Name),
			Snippet: it.Description,
			// Ruta de apps/frontend/apps/bases/src/pages/base/[id].astro.
			URL:  c.publicURL + "/base/" + url.PathEscape(it.ID),
			Icon: "bases",
		}
		if ts, err := time.Parse(time.RFC3339Nano, it.UpdatedAt); err == nil {
			hit.Timestamp = &ts
		}
		hits = append(hits, hit)
	}
	return hits, nil
}
