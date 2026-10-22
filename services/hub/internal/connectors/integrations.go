// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// IntegrationsConnector federa en la búsqueda unificada el contenido de los SaaS
// CONECTADOS de la organización. En vez de hablar con cada SaaS, consulta la
// búsqueda del Sytue Integration Gateway (GET /api/v1/search), que a su vez hace
// fan-out a las instancias activas (Microsoft 365, Google Workspace…) y devuelve
// hits ya normalizados; este conector solo los mapea al modelo canónico de Hub.
type IntegrationsConnector struct {
	client *httpClient
}

// NewIntegrationsConnector construye el conector. apiBaseURL es la URL interna del
// gateway (p. ej. http://integrations:8093).
func NewIntegrationsConnector(apiBaseURL string, timeout time.Duration) *IntegrationsConnector {
	return &IntegrationsConnector{client: newHTTPClient(apiBaseURL, timeout)}
}

// Source implementa Searcher.
func (c *IntegrationsConnector) Source() domain.Source { return domain.SourceIntegrations }

// externalHit es la forma de un hit que devuelve el gateway (su contrato
// connector.ExternalHit). Source es la etiqueta del SaaS de procedencia
// ("Microsoft 365"), no el origen de Hub.
type externalHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Kind    string `json:"kind"`
	Source  string `json:"source"`
}

// integrationsSearchResponse es la envoltura de la búsqueda del gateway.
type integrationsSearchResponse struct {
	Hits []externalHit `json:"hits"`
}

// Search llama a GET /api/v1/search del gateway CON el Bearer del usuario
// (passthrough, como el resto de conectores de Hub: el gateway aplica RLS por la org
// del token) y normaliza los hits al modelo canónico.
func (c *IntegrationsConnector) Search(ctx context.Context, token, query string, limit int) ([]domain.SearchHit, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("limit", strconv.Itoa(limit))

	var resp integrationsSearchResponse
	if err := c.client.getJSON(ctx, token, "/api/v1/search", q, &resp); err != nil {
		return nil, err
	}

	hits := make([]domain.SearchHit, 0, len(resp.Hits))
	for _, h := range resp.Hits {
		kind := h.Kind
		if kind == "" {
			kind = "result"
		}
		hits = append(hits, domain.SearchHit{
			Source:  domain.SourceIntegrations,
			Kind:    kind,
			ID:      h.URL, // ExternalHit no trae id upstream; la URL es su referencia estable
			Title:   h.Title,
			Snippet: joinMeta(h.Source, h.Snippet),
			URL:     h.URL,
			Icon:    h.Source, // etiqueta del SaaS de procedencia como pista para el shell
		})
	}
	return hits, nil
}

// Recent implementa Searcher: el gateway no expone actividad reciente de los SaaS
// conectados, así que este origen no aporta recientes (degradación elegante, como
// documenta el contrato Searcher: un origen sin recientes devuelve (nil, nil)).
func (c *IntegrationsConnector) Recent(ctx context.Context, token string, limit int) ([]domain.SearchHit, error) {
	return nil, nil
}

// joinMeta antepone la etiqueta del SaaS de procedencia al snippet
// ("Microsoft 365 · ada@contoso.com"), en el mismo estilo que el conector de
// contactos ("email · organización").
func joinMeta(source, snippet string) string {
	switch {
	case source != "" && snippet != "":
		return source + " · " + snippet
	case source != "":
		return source
	default:
		return snippet
	}
}
