// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package provider

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Embebedor convierte texto en vectores. Aparte de Provider por lo mismo que el
// Transcriptor: otro modelo (qwen3-embedding-8b), otro endpoint
// (/v1/embeddings) y otra tarifa (0,10 €/millón, y solo de entrada, porque no
// hay salida que cobrar).
//
// NO LO USA NADIE TODAVÍA y se deja escrito igualmente: es la pieza que necesita
// la búsqueda semántica de Drive, Mail y Notepads, y hacerla ahora cuesta veinte
// líneas porque el cliente HTTP y la credencial ya están montados.
type Embebedor interface {
	// Embeder devuelve un vector por texto, en el mismo orden, y los tokens de
	// entrada consumidos (para cobrarlos al presupuesto).
	Embeder(ctx context.Context, textos []string) ([][]float32, int, error)
	Modelo() string
}

type embebedorOpenAI struct {
	apiKey  string
	model   string
	baseURL string
	hc      *http.Client
}

func (e *embebedorOpenAI) Modelo() string { return e.model }

// NuevoEmbebedor construye el embebedor. ErrNotConfigured sin API key.
func NuevoEmbebedor(cfg Config) (Embebedor, error) {
	if cfg.APIKey == "" {
		return nil, ErrNotConfigured
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.Timeout + 5*time.Second}
	}
	return &embebedorOpenAI{
		apiKey: cfg.APIKey, model: cfg.Model,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"), hc: hc,
	}, nil
}

func (e *embebedorOpenAI) Embeder(ctx context.Context, textos []string) ([][]float32, int, error) {
	if len(textos) == 0 {
		return nil, 0, nil
	}
	body := struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}{Model: e.model, Input: textos}

	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
		} `json:"usage"`
	}
	headers := map[string]string{"Authorization": "Bearer " + e.apiKey} // server-only.
	if err := postJSON(ctx, e.hc, e.baseURL+"/v1/embeddings", headers, body, &out); err != nil {
		return nil, 0, err
	}
	if len(out.Data) != len(textos) {
		return nil, out.Usage.PromptTokens, ErrEmpty
	}
	// SE ORDENA POR `index` Y NO SE CONFÍA EN EL ORDEN DE LLEGADA: la respuesta
	// declara el índice justamente porque no está garantizado, y un vector
	// asignado al texto equivocado no da error en ningún sitio — sale como una
	// búsqueda que devuelve resultados absurdos y nadie sabe por qué.
	vectores := make([][]float32, len(textos))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(vectores) {
			return nil, out.Usage.PromptTokens, ErrEmpty
		}
		vectores[d.Index] = d.Embedding
	}
	return vectores, out.Usage.PromptTokens, nil
}
