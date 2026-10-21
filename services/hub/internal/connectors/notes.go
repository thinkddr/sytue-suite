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

// NotesConnector consulta la API REST de Sytue Notepads y normaliza las notas
// encontradas al modelo canónico de Hub.
//
// LA DECISIÓN DE QUÉ SE INDEXA NO ES DE AQUÍ: cada cuaderno y cada nota llevan su
// interruptor, y Notes solo devuelve lo que está marcado como indexable y visible
// para quien pregunta. El conector federa lo que le den; filtrarlo también aquí
// sería una segunda verdad que se desincroniza.
type NotesConnector struct {
	client    *httpClient
	publicURL string
}

// NewNotesConnector construye el conector de Notes.
func NewNotesConnector(apiBaseURL, publicURL string, timeout time.Duration) *NotesConnector {
	return &NotesConnector{
		client:    newHTTPClient(apiBaseURL, timeout),
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

// Source implementa Searcher.
func (c *NotesConnector) Source() domain.Source { return domain.SourceNotes }

// notesHit es la forma (parcial) del resultado que devuelve Notes.
type notesHit struct {
	ID         string `json:"id"`
	NotebookID string `json:"notebook_id"`
	Notebook   string `json:"notebook"`
	Title      string `json:"title"`
	Snippet    string `json:"snippet"`
	Kind       string `json:"kind"`
	UpdatedAt  string `json:"updated_at"`
}

type notesSearchResponse struct {
	Notes []notesHit `json:"notes"`
}

// Search llama a GET /api/v1/notes/search?q=... de Notes y normaliza.
func (c *NotesConnector) Search(ctx context.Context, token, query string, limit int) ([]domain.SearchHit, error) {
	return c.consultar(ctx, token, query, limit)
}

// Recent reusa el MISMO endpoint sin término: Notes devuelve entonces las notas
// recientes de quien pregunta. Un endpoint y no dos porque es la misma pregunta
// con y sin filtro, y dos se habrían separado en permisos con el tiempo.
func (c *NotesConnector) Recent(ctx context.Context, token string, limit int) ([]domain.SearchHit, error) {
	return c.consultar(ctx, token, "", limit)
}

func (c *NotesConnector) consultar(ctx context.Context, token, query string, limit int) ([]domain.SearchHit, error) {
	q := url.Values{}
	if query != "" {
		q.Set("q", query)
	}
	q.Set("limit", strconv.Itoa(limit))

	var resp notesSearchResponse
	if err := c.client.getJSON(ctx, token, "/api/v1/notes/search", q, &resp); err != nil {
		return nil, err
	}

	hits := make([]domain.SearchHit, 0, len(resp.Notes))
	for _, n := range resp.Notes {
		title := strings.TrimSpace(n.Title)
		if title == "" {
			// Una nota sin título se identifica por su cuaderno: es lo que la
			// distingue en una lista de resultados mezclados de toda la suite.
			title = n.Notebook
		}
		if title == "" {
			title = "(nota sin título)"
		}
		hit := domain.SearchHit{
			Source:  domain.SourceNotes,
			Kind:    "note",
			ID:      n.ID,
			Title:   title,
			Snippet: strings.TrimSpace(n.Snippet),
			URL:     c.publicURL + "/notebooks/" + url.PathEscape(n.NotebookID) + "?note=" + url.QueryEscape(n.ID),
			Icon:    "notes",
		}
		if ts, err := time.Parse(time.RFC3339, n.UpdatedAt); err == nil {
			hit.Timestamp = &ts
		}
		hits = append(hits, hit)
	}
	return hits, nil
}
