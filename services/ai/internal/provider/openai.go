// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// openAIProvider habla el dialecto OpenAI Chat Completions, que exponen la
// mayoría de servidores de inferencia self-hosted (vLLM, llama.cpp, Ollama,
// TGI, LiteLLM…). Es la vía para enchufar un modelo SOBERANO europeo detrás de
// la misma interfaz: solo cambia AI_PROVIDER=openai-compat + AI_BASE_URL/AI_MODEL.
type openAIProvider struct {
	name      string
	apiKey    string // server-only (Bearer). Puede ser un token cualquiera del gateway soberano.
	model     string
	baseURL   string
	maxTokens int
	// esfuerzo es el `reasoning_effort` por defecto de este proveedor. Lo puede
	// pisar cada petición con Opciones.Esfuerzo.
	esfuerzo string
	hc       *http.Client
}

func (p *openAIProvider) Name() string   { return p.name }
func (p *openAIProvider) Modelo() string { return p.model }

// openAIRequest es el cuerpo de POST /v1/chat/completions. El system prompt y el
// mensaje de usuario van como dos mensajes con sus roles.
type openAIResponseFormat struct {
	Type       string                 `json:"type"`
	JSONSchema *openAIJSONSchemaParam `json:"json_schema,omitempty"`
}

type openAIJSONSchemaParam struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

type openAIRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []openAIMessage `json:"messages"`
	// ReasoningEffort se omite si está vacío: en un modelo que no lo entiende,
	// mandarlo es un 400. Valores válidos en Scaleway: none, low, medium, high.
	ReasoningEffort string                `json:"reasoning_effort,omitempty"`
	ResponseFormat  *openAIResponseFormat `json:"response_format,omitempty"`
}

// openAIMessage lleva contenido de texto O una lista de partes (texto + imagen).
// Los dos campos son excluyentes y por eso ambos son `omitempty`: un mensaje con
// los dos puestos es un 400 del servidor.
type openAIMessage struct {
	Role    string        `json:"role"`
	Content string        `json:"content,omitempty"`
	Partes  []openAIParte `json:"-"`
}

// MarshalJSON emite `content` como cadena o como lista según haya imagen. Se
// hace a mano porque el campo tiene DOS formas en el mismo nombre y el marshal
// por etiquetas no sabe expresar eso.
func (m openAIMessage) MarshalJSON() ([]byte, error) {
	type salida struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}
	if len(m.Partes) > 0 {
		return jsonMarshal(salida{Role: m.Role, Content: m.Partes})
	}
	return jsonMarshal(salida{Role: m.Role, Content: m.Content})
}

// openAIParte es un trozo de un mensaje multimodal.
type openAIParte struct {
	Tipo   string        `json:"type"`
	Texto  string        `json:"text,omitempty"`
	Imagen *openAIImagen `json:"image_url,omitempty"`
}

type openAIImagen struct {
	URL string `json:"url"`
}

// openAIResponse recoge solo lo que necesitamos.
type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (p *openAIProvider) Complete(ctx context.Context, system, user string, opts Opciones) (Resultado, error) {
	maxTokens := p.maxTokens
	if opts.MaxTokens > 0 {
		maxTokens = opts.MaxTokens
	}

	var rf *openAIResponseFormat
	if len(opts.Esquema) > 0 {
		if p.name == ProviderZAI {
			rf = &openAIResponseFormat{Type: "json_object"}
		} else {
			rf = &openAIResponseFormat{
				Type: "json_schema",
				JSONSchema: &openAIJSONSchemaParam{
					Name:   "salida",
					Schema: opts.Esquema,
					Strict: true,
				},
			}
		}
	}

	res, err := p.call(ctx, system, user, opts, rf, maxTokens)
	if err != nil && rf != nil {
		// Si el proveedor rechaza el esquema (400), un reintento sin él (§9.1).
		var statusErr *HTTPStatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusBadRequest {
			return p.call(ctx, system, user, opts, nil, maxTokens)
		}
	}
	return res, err
}

func (p *openAIProvider) call(ctx context.Context, system, user string, opts Opciones, rf *openAIResponseFormat, maxTokens int) (Resultado, error) {
	mensajeUsuario := openAIMessage{Role: "user", Content: user}
	if opts.Imagen != nil {
		mensajeUsuario = openAIMessage{Role: "user", Partes: []openAIParte{
			{Tipo: "text", Texto: user},
			{Tipo: "image_url", Imagen: &openAIImagen{URL: dataURL(opts.Imagen)}},
		}}
	}

	esfuerzo := opts.Esfuerzo
	if esfuerzo == "" {
		esfuerzo = p.esfuerzo
	}

	reqBody := openAIRequest{
		Model:           p.model,
		MaxTokens:       maxTokens,
		ReasoningEffort: esfuerzo,
		ResponseFormat:  rf,
		Messages: []openAIMessage{
			{Role: "system", Content: system},
			mensajeUsuario,
		},
	}
	headers := map[string]string{
		"Authorization": "Bearer " + p.apiKey, // server-only; postJSON nunca lo filtra en errores.
	}

	var out openAIResponse
	endpoint := chatCompletionsURL(p.baseURL)
	if err := postJSON(ctx, p.hc, endpoint, headers, reqBody, &out); err != nil {
		return Resultado{}, err
	}

	res := Resultado{TokensEntrada: out.Usage.PromptTokens, TokensSalida: out.Usage.CompletionTokens}
	if len(out.Choices) == 0 {
		return res, ErrEmpty
	}
	if out.Choices[0].FinishReason == "content_filter" {
		return res, ErrRefused
	}
	res.Texto = strings.TrimSpace(out.Choices[0].Message.Content)
	// EL TRUNCADO SE MIRA ANTES QUE EL VACÍO, y ese orden es el arreglo (§9.1).
	if out.Choices[0].FinishReason == "length" {
		return res, ErrTruncated
	}
	if res.Texto == "" {
		return res, ErrEmpty
	}
	return res, nil
}

// dataURL codifica la imagen como `data:<mime>;base64,<...>`, que es la forma en
// que los servidores compatibles aceptan una imagen sin tener que servirla por
// HTTP desde algún sitio alcanzable por el proveedor —que en nuestro caso no
// existe: las fotos del usuario no salen a una URL pública para que un modelo
// las lea—.
func dataURL(img *Imagen) string {
	tipo := img.Tipo
	if tipo == "" {
		tipo = "image/png"
	}
	return fmt.Sprintf("data:%s;base64,%s", tipo, base64.StdEncoding.EncodeToString(img.Datos))
}

func chatCompletionsURL(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") || strings.HasSuffix(base, "/v4") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}
