// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package analitica habla con Sytue Analytics: alta de hosts y panel
// (PAGES-ENCARGO §12).
package analitica

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Resumen struct {
	Totals struct {
		Pageviews int64 `json:"pageviews"`
		Visitors  int64 `json:"visitors"`
	} `json:"totals"`
	VisitorsAreDailySum bool `json:"visitors_are_daily_sum"`
}

var allowlistPorDefecto = []string{"add_to_cart", "checkout", "order_paid", "form_sent"}

// Cliente es el cliente de la superficie interna de analytics-api.
type Cliente struct {
	baseURL string
	secret  string
	http    *http.Client
}

// Nuevo construye el cliente de analítica.
func Nuevo(baseURL, secret string) *Cliente {
	return &Cliente{baseURL: strings.TrimRight(baseURL, "/"), secret: secret, http: &http.Client{Timeout: 5 * time.Second}}
}

func (c *Cliente) Disponible() bool { return c != nil && c.baseURL != "" && c.secret != "" }

// Asegurar registra el host canónico de una web y devuelve su identificador de
// Analytics. No acepta allowlists arbitrarias: Pages define el conjunto cerrado.
func (c *Cliente) Asegurar(ctx context.Context, tenant, host string) (string, error) {
	if !c.Disponible() {
		return "", nil
	}
	body, err := json.Marshal(map[string]any{"tenant": tenant, "host": host, "event_allowlist": allowlistPorDefecto})
	if err != nil {
		return "", fmt.Errorf("serializar alta de analítica: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/v1/sites", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("crear alta de analítica: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("alta de analítica: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("alta de analítica: estado inesperado %d", resp.StatusCode)
	}
	var out struct {
		SiteID string `json:"site_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.SiteID == "" {
		return "", fmt.Errorf("alta de analítica: respuesta inválida")
	}
	return out.SiteID, nil
}

func (c *Cliente) Resumen(ctx context.Context, tenant, siteID string) (Resumen, error) {
	var out Resumen
	if !c.Disponible() || siteID == "" {
		return out, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/internal/v1/sites/"+siteID+"/summary", http.NoBody)
	if err != nil {
		return out, fmt.Errorf("crear resumen de analítica: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("X-Sytue-Tenant", tenant)
	resp, err := c.http.Do(req)
	if err != nil {
		return out, fmt.Errorf("resumen de analítica: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("resumen de analítica: estado inesperado %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("decodificar resumen de analítica: %w", err)
	}
	return out, nil
}
