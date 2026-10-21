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

// ERPConnector consulta la API REST de SyERP y normaliza los registros
// encontrados al modelo canónico de Hub.
type ERPConnector struct {
	client    *httpClient
	publicURL string
}

// NewERPConnector construye el conector de SyERP.
func NewERPConnector(apiBaseURL, publicURL string, timeout time.Duration) *ERPConnector {
	return &ERPConnector{
		client:    newHTTPClient(apiBaseURL, timeout),
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

// Source implementa Searcher.
func (c *ERPConnector) Source() domain.Source { return domain.SourceERP }

type erpHit struct {
	ID        string `json:"id"`
	Modelo    string `json:"modelo"`
	Ref       string `json:"ref,omitempty"`
	Etiqueta  string `json:"etiqueta"`
	Estado    string `json:"estado,omitempty"`
	CreadoEn  string `json:"creado_en"`
	EditadoEn string `json:"editado_en"`
}

type erpSearchResponse struct {
	Items []erpHit `json:"items"`
}

// Search llama a GET /api/v1/search?q=... de SyERP y normaliza.
func (c *ERPConnector) Search(ctx context.Context, token, query string, limit int) ([]domain.SearchHit, error) {
	return c.consultar(ctx, token, query, limit)
}

// Recent reusa el MISMO endpoint sin término: SyERP devuelve los registros más recientes.
func (c *ERPConnector) Recent(ctx context.Context, token string, limit int) ([]domain.SearchHit, error) {
	return c.consultar(ctx, token, "", limit)
}

func (c *ERPConnector) consultar(ctx context.Context, token, query string, limit int) ([]domain.SearchHit, error) {
	q := url.Values{}
	if query != "" {
		q.Set("q", query)
	}
	q.Set("limit", strconv.Itoa(limit))

	var resp erpSearchResponse
	if err := c.client.getJSON(ctx, token, "/api/v1/search", q, &resp); err != nil {
		return nil, err
	}

	hits := make([]domain.SearchHit, 0, len(resp.Items))
	for _, it := range resp.Items {
		title := strings.TrimSpace(it.Etiqueta)
		if it.Ref != "" {
			if title != "" {
				title = it.Ref + " · " + title
			} else {
				title = it.Ref
			}
		}
		if title == "" {
			title = "(registro sin nombre)"
		}

		snippet := it.Modelo
		if it.Estado != "" {
			snippet += " · " + it.Estado
		}

		// Deep-link exacto soportado por apps/frontend/apps/erp/src/pages/index.astro (?m=modelo&r=id)
		hitURL := c.publicURL + "/?m=" + url.QueryEscape(it.Modelo) + "&r=" + url.QueryEscape(it.ID)

		hit := domain.SearchHit{
			Source:  domain.SourceERP,
			Kind:    it.Modelo,
			ID:      it.ID,
			Title:   title,
			Snippet: snippet,
			URL:     hitURL,
			Icon:    "erp",
		}
		if ts, err := time.Parse(time.RFC3339, it.EditadoEn); err == nil {
			hit.Timestamp = &ts
		} else if ts, err := time.Parse(time.RFC3339Nano, it.EditadoEn); err == nil {
			hit.Timestamp = &ts
		} else if ts, err := time.Parse(time.RFC3339, it.CreadoEn); err == nil {
			hit.Timestamp = &ts
		}
		hits = append(hits, hit)
	}
	return hits, nil
}
