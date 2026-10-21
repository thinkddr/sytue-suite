// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxResponseBytes acota el cuerpo de respuesta upstream que Hub deserializa
// (defensa frente a respuestas anómalas; 4 MiB).
const maxResponseBytes = 4 << 20

// httpClient es el cliente HTTP base compartido por los conectores. Reutiliza
// conexiones (keep-alive) y aplica un timeout global por petición.
type httpClient struct {
	baseURL string
	hc      *http.Client
}

// newHTTPClient construye un cliente para un servicio upstream. baseURL es la URL
// interna del servicio (p. ej. http://drive:8082); timeout acota cada llamada.
func newHTTPClient(baseURL string, timeout time.Duration) *httpClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &httpClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        50,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				ForceAttemptHTTP2:   true,
			},
		},
	}
}

// getJSON hace GET path?query con el Bearer token del usuario y deserializa la
// respuesta JSON en dst. Propaga errores HTTP no-2xx como error.
func (c *httpClient) getJSON(ctx context.Context, token, path string, query url.Values, dst any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return fmt.Errorf("conector: construir petición: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("conector: %s: %w", u, err)
	}
	defer resp.Body.Close()

	body := io.LimitReader(resp.Body, maxResponseBytes)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Drenar (acotado) para reutilizar la conexión.
		_, _ = io.Copy(io.Discard, body)
		return fmt.Errorf("conector: %s devolvió %d", u, resp.StatusCode)
	}
	if dst == nil {
		_, _ = io.Copy(io.Discard, body)
		return nil
	}
	if err := json.NewDecoder(body).Decode(dst); err != nil {
		return fmt.Errorf("conector: decodificar respuesta de %s: %w", u, err)
	}
	return nil
}
