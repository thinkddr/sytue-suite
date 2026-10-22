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

// RoomsConnector consulta la API REST de Sytue Rooms (búsqueda de mensajes) y
// normaliza los mensajes encontrados al modelo canónico de Hub.
type RoomsConnector struct {
	client    *httpClient
	publicURL string
}

// NewRoomsConnector construye el conector de Chat.
func NewRoomsConnector(apiBaseURL, publicURL string, timeout time.Duration) *RoomsConnector {
	return &RoomsConnector{
		client:    newHTTPClient(apiBaseURL, timeout),
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

// Source implementa Searcher.
func (c *RoomsConnector) Source() domain.Source { return domain.SourceChat }

// chatMessage es la forma (parcial) del mensaje que devuelve Chat.
type chatMessage struct {
	ID          string     `json:"id"`
	ChannelID   string     `json:"channel_id"`
	ChannelName string     `json:"channel_name"`
	AuthorName  string     `json:"author_name"`
	Body        string     `json:"body"`
	CreatedAt   *time.Time `json:"created_at"`
}

// chatSearchResponse es la envoltura de resultados de Chat.
type chatSearchResponse struct {
	Messages []chatMessage `json:"messages"`
}

// Search llama a GET /api/v1/search?q=... de Chat y normaliza.
func (c *RoomsConnector) Search(ctx context.Context, token, query string, limit int) ([]domain.SearchHit, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("limit", strconv.Itoa(limit))

	var resp chatSearchResponse
	if err := c.client.getJSON(ctx, token, "/api/v1/search", q, &resp); err != nil {
		return nil, err
	}

	hits := make([]domain.SearchHit, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		title := m.AuthorName
		if m.ChannelName != "" {
			title = joinNonEmpty(" en #", m.AuthorName, m.ChannelName)
		}
		// Deep-link al canal anclado al mensaje.
		u := c.publicURL + "/channels/" + url.PathEscape(m.ChannelID)
		if m.ID != "" {
			u += "?message=" + url.QueryEscape(m.ID)
		}
		hits = append(hits, domain.SearchHit{
			Source:    domain.SourceChat,
			Kind:      "message",
			ID:        m.ID,
			Title:     title,
			Snippet:   truncate(m.Body, 160),
			URL:       u,
			Icon:      "chat",
			Timestamp: m.CreatedAt,
		})
	}
	return hits, nil
}

// Recent no aporta actividad todavía: Chat no expone un listado de mensajes
// recientes del usuario (SearchAll exige q; no hay «recientes cross-canal»).
// ponytail: (nil,nil) evita una llamada que fallaría; cablear cuando Chat exponga
// GET /api/v1/recent (mensajes de mis canales por created_at DESC).
func (c *RoomsConnector) Recent(context.Context, string, int) ([]domain.SearchHit, error) {
	return nil, nil
}

// truncate recorta s a n runas añadiendo elipsis si se acorta.
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}
