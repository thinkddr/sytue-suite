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

// CalendarConnector consulta la API REST de Sytue Calendar (búsqueda de eventos)
// y normaliza los eventos encontrados al modelo canónico de Hub.
type CalendarConnector struct {
	client    *httpClient
	publicURL string
}

// NewCalendarConnector construye el conector de Calendar.
func NewCalendarConnector(apiBaseURL, publicURL string, timeout time.Duration) *CalendarConnector {
	return &CalendarConnector{
		client:    newHTTPClient(apiBaseURL, timeout),
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

// Source implementa Searcher.
func (c *CalendarConnector) Source() domain.Source { return domain.SourceCalendar }

// calendarEvent es la forma (parcial) del evento que devuelve Calendar.
type calendarEvent struct {
	ID       string     `json:"id"`
	Summary  string     `json:"summary"`
	Location string     `json:"location"`
	Start    *time.Time `json:"dtstart"` // Calendar lo llama así; con "start" salía vacío
}

// calendarSearchResponse es la envoltura de resultados de Calendar.
type calendarSearchResponse struct {
	Events []calendarEvent `json:"events"`
}

// Search llama a GET /api/v1/events/search?q=... de Calendar y normaliza.
func (c *CalendarConnector) Search(ctx context.Context, token, query string, limit int) ([]domain.SearchHit, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("limit", strconv.Itoa(limit))

	var resp calendarSearchResponse
	if err := c.client.getJSON(ctx, token, "/api/v1/events/search", q, &resp); err != nil {
		return nil, err
	}

	hits := make([]domain.SearchHit, 0, len(resp.Events))
	for _, e := range resp.Events {
		title := e.Summary
		if title == "" {
			title = "(evento sin título)"
		}
		hits = append(hits, domain.SearchHit{
			Source:    domain.SourceCalendar,
			Kind:      "event",
			ID:        e.ID,
			Title:     title,
			Snippet:   strings.TrimSpace(e.Location),
			URL:       c.publicURL + "/events/" + url.PathEscape(e.ID),
			Icon:      "calendar",
			Timestamp: e.Start,
		})
	}
	return hits, nil
}

// Recent no aporta actividad todavía: Calendar no expone un listado de eventos
// recientes global (ListInstances exige un calendar id + rango de fechas; no hay
// «recientes de todos los calendarios»).
// ponytail: (nil,nil) evita una llamada que fallaría; cablear cuando Calendar
// exponga recientes/próximos globales (o el conector oriente listCalendars→instances).
func (c *CalendarConnector) Recent(context.Context, string, int) ([]domain.SearchHit, error) {
	return nil, nil
}
