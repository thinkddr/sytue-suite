// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package subscriptions

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client obtiene feeds ICS externos con la barrera SSRF y un tope de bytes. Se
// construye una vez y se reutiliza (pool de conexiones del Transport).
type Client struct {
	http     *http.Client
	maxBytes int64
}

// NewClient construye el cliente SSRF-safe de refresco con un tope de respuesta.
// maxBytes <= 0 degrada a 5 MiB.
func NewClient(maxBytes int64) *Client {
	if maxBytes <= 0 {
		maxBytes = 5 << 20
	}
	return &Client{http: newClient(), maxBytes: maxBytes}
}

// Conditional lleva los validadores del último 200 para el GET condicional.
type Conditional struct {
	ETag         string
	LastModified string
}

// FetchResult es el resultado de refrescar un feed. NotModified => el servidor
// respondió 304 (el contenido no cambió: no se retrae ni se re-parsea).
type FetchResult struct {
	NotModified  bool
	Body         string
	ETag         string
	LastModified string
	// MaxAge es el Cache-Control: max-age de la respuesta (0 si ausente): el
	// worker no re-fetchea antes de que expire.
	MaxAge time.Duration
}

// Fetch hace UN GET condicional SSRF-safe del feed. Valida el destino (barrera 1)
// antes de conectar; el dialer re-valida (barrera 2). Respeta If-None-Match /
// If-Modified-Since para no retraer el feed si no cambió, y acota el cuerpo a
// maxBytes (un feed abusivamente grande => error, no OOM). El cuerpo se devuelve
// como DATOS crudos para parsear; nunca se ejecuta.
func (c *Client) Fetch(ctx context.Context, rawURL string, cond Conditional) (FetchResult, error) {
	if err := validateURL(ctx, rawURL); err != nil {
		return FetchResult{}, err
	}
	return doFetch(ctx, c.http, rawURL, cond, c.maxBytes)
}

// doFetch ejecuta el GET condicional y parsea la respuesta (sin la barrera SSRF,
// que ya validó Fetch). Separado para poder probar la lógica condicional/304
// contra un servidor local (que la barrera SSRF bloquearía por ser 127.0.0.1).
func doFetch(ctx context.Context, hc *http.Client, rawURL string, cond Conditional, maxBytes int64) (FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return FetchResult{}, err
	}
	req.Header.Set("Accept", "text/calendar, text/plain;q=0.9, */*;q=0.5")
	req.Header.Set("User-Agent", fetchUserAgent)
	if cond.ETag != "" {
		req.Header.Set("If-None-Match", cond.ETag)
	}
	if cond.LastModified != "" {
		req.Header.Set("If-Modified-Since", cond.LastModified)
	}

	resp, err := hc.Do(req)
	if err != nil {
		return FetchResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return FetchResult{NotModified: true}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return FetchResult{}, fmt.Errorf("ics: estado HTTP %d", resp.StatusCode)
	}

	// Lee hasta maxBytes+1: si excede el tope, es un feed demasiado grande.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return FetchResult{}, fmt.Errorf("ics: leer cuerpo: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return FetchResult{}, fmt.Errorf("ics: feed demasiado grande (> %d bytes)", maxBytes)
	}

	return FetchResult{
		Body:         string(raw),
		ETag:         strings.TrimSpace(resp.Header.Get("ETag")),
		LastModified: strings.TrimSpace(resp.Header.Get("Last-Modified")),
		MaxAge:       parseMaxAge(resp.Header.Get("Cache-Control")),
	}, nil
}

// parseMaxAge extrae el max-age (segundos) de un Cache-Control, 0 si ausente o si
// no-store/no-cache lo invalidan.
func parseMaxAge(cc string) time.Duration {
	if cc == "" {
		return 0
	}
	for _, part := range strings.Split(cc, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "no-store" || part == "no-cache" {
			return 0
		}
		if v, ok := strings.CutPrefix(part, "max-age="); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
				return time.Duration(n) * time.Second
			}
		}
	}
	return 0
}
