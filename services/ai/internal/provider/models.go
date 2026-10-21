// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ListModels consulta la lista de modelos disponibles para un proveedor y clave dados (§9.1).
// Se usa tanto para verificar la validez de una clave BYOK como para ofrecer el catálogo disponible.
// SEGURIDAD: jamás se registra ni se incluye la clave en errores o respuestas.
func ListModels(ctx context.Context, hc *http.Client, provName, apiKey string) ([]string, error) {
	prov := strings.ToLower(strings.TrimSpace(provName))
	baseURL, ok := BaseURLs[prov]
	if !ok {
		return nil, fmt.Errorf("proveedor desconocido o no permitido: %q", provName)
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, errors.New("la clave no puede estar vacía")
	}

	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	url := baseURL + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("construir petición de modelos: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if prov == ProviderAnthropic {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, 4<<20)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, limited)
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode}
	}

	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if err := json.NewDecoder(limited).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decodificar lista de modelos: %w", err)
	}

	seen := make(map[string]bool)
	var result []string
	for _, item := range parsed.Data {
		id := strings.TrimSpace(item.ID)
		if id != "" && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	for _, item := range parsed.Models {
		id := strings.TrimSpace(item.ID)
		if id != "" && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}

	sort.Strings(result)
	return result, nil
}
