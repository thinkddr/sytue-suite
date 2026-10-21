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

// openAIResponsesProvider implementa OpenAI Responses API (POST /v1/responses) (§9.1).
type openAIResponsesProvider struct {
	name      string
	apiKey    string
	model     string
	baseURL   string
	maxTokens int
	hc        *http.Client
}

func (p *openAIResponsesProvider) Name() string   { return p.name }
func (p *openAIResponsesProvider) Modelo() string { return p.model }

type openAIResponsesRequest struct {
	Model           string                 `json:"model"`
	Input           []openAIMessage        `json:"input"`
	MaxOutputTokens int                    `json:"max_output_tokens,omitempty"`
	Text            *openAIResponsesFormat `json:"text,omitempty"`
}

type openAIResponsesFormat struct {
	Format openAIResponsesJSONSchema `json:"format"`
}

type openAIResponsesJSONSchema struct {
	Type   string          `json:"type"`
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

type openAIResponsesResponse struct {
	Output []struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Status            string `json:"status"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p *openAIResponsesProvider) Complete(ctx context.Context, system, user string, opts Opciones) (Resultado, error) {
	maxTokens := p.maxTokens
	if opts.MaxTokens > 0 {
		maxTokens = opts.MaxTokens
	}

	var fmtParam *openAIResponsesFormat
	if len(opts.Esquema) > 0 {
		fmtParam = &openAIResponsesFormat{
			Format: openAIResponsesJSONSchema{
				Type:   "json_schema",
				Name:   "salida",
				Schema: opts.Esquema,
				Strict: true,
			},
		}
	}

	res, err := p.call(ctx, system, user, opts, fmtParam, maxTokens)
	if err != nil && fmtParam != nil {
		// Si el proveedor rechaza el esquema (400), un reintento sin él (§9.1).
		var statusErr *HTTPStatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusBadRequest {
			return p.call(ctx, system, user, opts, nil, maxTokens)
		}
	}
	return res, err
}

func (p *openAIResponsesProvider) call(ctx context.Context, system, user string, opts Opciones, fmtParam *openAIResponsesFormat, maxTokens int) (Resultado, error) {
	mensajeUsuario := openAIMessage{Role: "user", Content: user}
	if opts.Imagen != nil {
		mensajeUsuario = openAIMessage{Role: "user", Partes: []openAIParte{
			{Tipo: "text", Texto: user},
			{Tipo: "image_url", Imagen: &openAIImagen{URL: dataURL(opts.Imagen)}},
		}}
	}

	reqBody := openAIResponsesRequest{
		Model:           p.model,
		MaxOutputTokens: maxTokens,
		Text:            fmtParam,
		Input: []openAIMessage{
			{Role: "system", Content: system},
			mensajeUsuario,
		},
	}
	headers := map[string]string{
		"Authorization": "Bearer " + p.apiKey,
	}

	var out openAIResponsesResponse
	endpoint := strings.TrimRight(p.baseURL, "/") + "/responses"
	if err := postJSON(ctx, p.hc, endpoint, headers, reqBody, &out); err != nil {
		return Resultado{}, err
	}

	res := Resultado{TokensEntrada: out.Usage.InputTokens, TokensSalida: out.Usage.OutputTokens}

	// Truncado antes que vacío (§9.1).
	isTruncated := out.Status == "incomplete" ||
		(out.IncompleteDetails != nil && out.IncompleteDetails.Reason == "max_output_tokens")

	var sb strings.Builder
	for _, item := range out.Output {
		for _, part := range item.Content {
			sb.WriteString(part.Text)
		}
	}
	res.Texto = strings.TrimSpace(sb.String())

	if isTruncated {
		return res, ErrTruncated
	}
	if res.Texto == "" {
		return res, ErrEmpty
	}
	return res, nil
}
