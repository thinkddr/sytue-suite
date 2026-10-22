// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// anthropicProvider implementa Anthropic Messages API (POST /v1/messages) (§9.1).
type anthropicProvider struct {
	name      string
	apiKey    string
	model     string
	baseURL   string
	maxTokens int
	hc        *http.Client
}

func (p *anthropicProvider) Name() string   { return p.name }
func (p *anthropicProvider) Modelo() string { return p.model }

type anthropicRequest struct {
	Model        string                 `json:"model"`
	MaxTokens    int                    `json:"max_tokens"`
	System       string                 `json:"system,omitempty"`
	Messages     []anthropicMessage     `json:"messages"`
	OutputConfig *anthropicOutputConfig `json:"output_config,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicOutputConfig struct {
	Format anthropicFormat `json:"format"`
}

type anthropicFormat struct {
	Type   string          `json:"type"`
	Schema json.RawMessage `json:"schema"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p *anthropicProvider) Complete(ctx context.Context, system, user string, opts Opciones) (Resultado, error) {
	maxTokens := p.maxTokens
	if opts.MaxTokens > 0 {
		maxTokens = opts.MaxTokens
	}

	var outCfg *anthropicOutputConfig
	if len(opts.Esquema) > 0 {
		outCfg = &anthropicOutputConfig{
			Format: anthropicFormat{
				Type:   "json_schema",
				Schema: opts.Esquema,
			},
		}
	}

	res, err := p.call(ctx, system, user, outCfg, maxTokens)
	if err != nil && outCfg != nil {
		// Si el proveedor rechaza el esquema (400), un reintento sin él (§9.1).
		var statusErr *HTTPStatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusBadRequest {
			return p.call(ctx, system, user, nil, maxTokens)
		}
	}
	return res, err
}

func (p *anthropicProvider) call(ctx context.Context, system, user string, outCfg *anthropicOutputConfig, maxTokens int) (Resultado, error) {
	reqBody := anthropicRequest{
		Model:        p.model,
		MaxTokens:    maxTokens,
		System:       system,
		OutputConfig: outCfg,
		Messages: []anthropicMessage{
			{Role: "user", Content: user},
		},
	}
	headers := map[string]string{
		"x-api-key":         p.apiKey,
		"anthropic-version": "2023-06-01",
	}

	var out anthropicResponse
	endpoint := strings.TrimRight(p.baseURL, "/") + "/messages"
	if err := postJSON(ctx, p.hc, endpoint, headers, reqBody, &out); err != nil {
		return Resultado{}, err
	}

	res := Resultado{TokensEntrada: out.Usage.InputTokens, TokensSalida: out.Usage.OutputTokens}

	var sb strings.Builder
	for _, part := range out.Content {
		sb.WriteString(part.Text)
	}
	res.Texto = strings.TrimSpace(sb.String())

	// Truncado antes que vacío (§9.1).
	if out.StopReason == "max_tokens" {
		return res, ErrTruncated
	}
	if res.Texto == "" {
		return res, ErrEmpty
	}
	return res, nil
}
