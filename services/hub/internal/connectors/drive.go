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

// DriveConnector consulta la API REST de Sytue Drive (GET /api/v1/search) y
// normaliza los ficheros encontrados al modelo canónico de Hub.
type DriveConnector struct {
	client    *httpClient
	publicURL string
}

// NewDriveConnector construye el conector de Drive. apiBaseURL es la URL interna
// del servicio; publicURL es la base de cara al usuario para los deep-links.
func NewDriveConnector(apiBaseURL, publicURL string, timeout time.Duration) *DriveConnector {
	return &DriveConnector{
		client:    newHTTPClient(apiBaseURL, timeout),
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

// Source implementa Searcher.
func (c *DriveConnector) Source() domain.Source { return domain.SourceDrive }

// driveFile es la forma (parcial) del recurso fichero que devuelve Drive.
type driveFile struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	ContentType string     `json:"content_type"`
	SizeBytes   int64      `json:"size_bytes"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

// driveSearchResponse es la envoltura de resultados de Drive: GET /api/v1/search
// responde {"files":[...]} (no un array desnudo).
type driveSearchResponse struct {
	Files []driveFile `json:"files"`
}

// Search llama a GET /api/v1/search?q=... de Drive y normaliza.
func (c *DriveConnector) Search(ctx context.Context, token, query string, limit int) ([]domain.SearchHit, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("limit", strconv.Itoa(limit))

	var resp driveSearchResponse
	if err := c.client.getJSON(ctx, token, "/api/v1/search", q, &resp); err != nil {
		return nil, err
	}

	hits := make([]domain.SearchHit, 0, len(resp.Files))
	for _, f := range resp.Files {
		hits = append(hits, domain.SearchHit{
			Source:    domain.SourceDrive,
			Kind:      "file",
			ID:        f.ID,
			Title:     f.Name,
			Snippet:   humanizeBytes(f.SizeBytes),
			SizeBytes: &f.SizeBytes,
			URL:       c.publicURL + "/files/" + url.PathEscape(f.ID),
			Icon:      f.ContentType,
			Timestamp: f.UpdatedAt,
		})
	}
	return hits, nil
}

// driveNode es la forma (parcial) del nodo que devuelve GET /api/v1/recent de
// Drive (envoltura {"items":[...]}, ficheros y carpetas del tenant por
// updated_at DESC; updated_at siempre presente, no puntero).
type driveNode struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	IsDir       bool      `json:"is_directory"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// driveRecentResponse es la envoltura de recientes de Drive: {"items":[...]}.
type driveRecentResponse struct {
	Items []driveNode `json:"items"`
}

// Recent llama a GET /api/v1/recent?limit=... de Drive (updated_at DESC) y
// normaliza. Solo ficheros: la actividad reciente del launcher enlaza a un
// recurso abrible con el mismo deep-link que la búsqueda (/files/{id}); las
// carpetas se omiten (su updated_at rebota por cambios de hijos: ruido).
func (c *DriveConnector) Recent(ctx context.Context, token string, limit int) ([]domain.SearchHit, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))

	var resp driveRecentResponse
	if err := c.client.getJSON(ctx, token, "/api/v1/recent", q, &resp); err != nil {
		return nil, err
	}

	hits := make([]domain.SearchHit, 0, len(resp.Items))
	for _, f := range resp.Items {
		if f.IsDir {
			continue
		}
		ts := f.UpdatedAt
		hits = append(hits, domain.SearchHit{
			Source:    domain.SourceDrive,
			Kind:      "file",
			ID:        f.ID,
			Title:     f.Name,
			Snippet:   humanizeBytes(f.SizeBytes),
			SizeBytes: &f.SizeBytes,
			URL:       c.publicURL + "/files/" + url.PathEscape(f.ID),
			Icon:      f.ContentType,
			Timestamp: &ts,
		})
	}
	return hits, nil
}

// humanizeBytes formatea un tamaño en una etiqueta corta para el snippet.
func humanizeBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	val := float64(n) / float64(div)
	return strconv.FormatFloat(val, 'f', 1, 64) + " " + units[exp]
}
