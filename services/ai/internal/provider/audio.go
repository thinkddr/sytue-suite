// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// Transcriptor convierte audio en texto. Va aparte de Provider porque NO es el
// mismo modelo ni el mismo endpoint: whisper-large-v3 en /v1/audio/transcriptions,
// y se factura por MINUTO DE AUDIO (0,003 €), no por token. Meterlo en la misma
// interfaz obligaría a que los dos modelos de chat tuvieran un método que no
// saben hacer.
type Transcriptor interface {
	// Transcribir devuelve el texto del audio. `idioma` es una pista opcional en
	// ISO-639-1 ("es"); vacío deja que el modelo lo detecte.
	Transcribir(ctx context.Context, audio []byte, nombre, idioma string) (string, error)
	Modelo() string
}

// transcriptorOpenAI habla el dialecto OpenAI Audio, que es lo que expone
// Scaleway para whisper-large-v3.
type transcriptorOpenAI struct {
	apiKey  string
	model   string
	baseURL string
	hc      *http.Client
}

func (t *transcriptorOpenAI) Modelo() string { return t.model }

// NuevoTranscriptor construye el transcriptor. Devuelve ErrNotConfigured sin
// API key, igual que New: el servicio arranca y responde 503 por petición.
func NuevoTranscriptor(cfg Config) (Transcriptor, error) {
	if cfg.APIKey == "" {
		return nil, ErrNotConfigured
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.Timeout + 5*time.Second}
	}
	return &transcriptorOpenAI{
		apiKey: cfg.APIKey, model: cfg.Model,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"), hc: hc,
	}, nil
}

func (t *transcriptorOpenAI) Transcribir(ctx context.Context, audio []byte, nombre, idioma string) (string, error) {
	// MULTIPART Y NO JSON: este endpoint recibe el fichero como subida, no en
	// base64. Se construye en memoria porque el tope de tamaño ya lo aplica el
	// handler; para audios largos habría que ir en streaming, y entonces esto se
	// convierte en un io.Pipe.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if nombre == "" {
		nombre = "audio.webm"
	}
	parte, err := mw.CreateFormFile("file", nombre)
	if err != nil {
		return "", fmt.Errorf("componer subida: %w", err)
	}
	if _, err := parte.Write(audio); err != nil {
		return "", fmt.Errorf("componer subida: %w", err)
	}
	campos := map[string]string{"model": t.model, "response_format": "json"}
	if idioma != "" {
		// La pista de idioma NO es cosmética: sin ella, whisper con un audio
		// corto en español a veces devuelve la transcripción traducida al inglés.
		campos["language"] = idioma
	}
	for k, v := range campos {
		if err := mw.WriteField(k, v); err != nil {
			return "", fmt.Errorf("componer subida: %w", err)
		}
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("componer subida: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/v1/audio/transcriptions", &buf)
	if err != nil {
		return "", fmt.Errorf("construir petición: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+t.apiKey) // server-only.

	resp, err := t.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 8<<20)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, limited)
		return "", fmt.Errorf("proveedor respondió estado %d", resp.StatusCode)
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(limited).Decode(&out); err != nil {
		return "", fmt.Errorf("decodificar respuesta del proveedor: %w", err)
	}
	texto := strings.TrimSpace(out.Text)
	if texto == "" {
		return "", ErrEmpty
	}
	return texto, nil
}
