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

// PagesConnector consulta la API de Sytue Web y normaliza los sitios de la organización
// al modelo canónico de Hub. Pages es de organizaciones: una cuenta personal recibe un
// 403 de su verja, y la búsqueda federada lo cuenta como un origen que no aporta (como
// ya pasa con SyERP), no como un fallo.
type PagesConnector struct {
	client    *httpClient
	publicURL string
}

// NewPagesConnector construye el conector de Pages.
func NewPagesConnector(apiBaseURL, publicURL string, timeout time.Duration) *PagesConnector {
	return &PagesConnector{
		client:    newHTTPClient(apiBaseURL, timeout),
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

// Source implementa Searcher.
func (c *PagesConnector) Source() domain.Source { return domain.SourcePages }

type pagesHit struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	UpdatedAt string `json:"updated_at"`
}

type pagesSearchResponse struct {
	Items []pagesHit `json:"items"`
}

// Search llama a GET /api/v1/search?q=… de Pages y normaliza.
func (c *PagesConnector) Search(ctx context.Context, token, query string, limit int) ([]domain.SearchHit, error) {
	return c.consultar(ctx, token, query, limit)
}

// Recent reusa el MISMO endpoint sin término: Pages devuelve los sitios editados más
// recientemente.
func (c *PagesConnector) Recent(ctx context.Context, token string, limit int) ([]domain.SearchHit, error) {
	return c.consultar(ctx, token, "", limit)
}

func (c *PagesConnector) consultar(ctx context.Context, token, query string, limit int) ([]domain.SearchHit, error) {
	q := url.Values{}
	if query != "" {
		q.Set("q", query)
	}
	q.Set("limit", strconv.Itoa(limit))

	var resp pagesSearchResponse
	if err := c.client.getJSON(ctx, token, "/api/v1/search", q, &resp); err != nil {
		return nil, err
	}

	hits := make([]domain.SearchHit, 0, len(resp.Items))
	for _, it := range resp.Items {
		title := strings.TrimSpace(it.Name)
		if title == "" {
			title = it.Slug
		}
		hit := domain.SearchHit{
			Source:  domain.SourcePages,
			Kind:    "site",
			ID:      it.ID,
			Title:   title,
			Snippet: it.Slug,
			// El editor es la puerta natural de «mi web»; la ficha (publicar, dominio,
			// tienda) está a un clic desde ahí. Ruta de apps/frontend/apps/pages/src/pages/edit.
			URL:  c.publicURL + "/edit/" + url.PathEscape(it.ID),
			Icon: "pages",
		}
		if ts, err := time.Parse(time.RFC3339Nano, it.UpdatedAt); err == nil {
			hit.Timestamp = &ts
		}
		hits = append(hits, hit)
	}
	return hits, nil
}
