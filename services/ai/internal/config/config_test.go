// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// setBaseEnv fija un entorno limpio y válido para cada caso (t.Setenv restaura
// al terminar). Sin PORT/HTTP_ADDR para poder comprobar el default :8084.
func setBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ENV", "dev")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("PORT", "")
}

func TestLoad_ScalewayDefaults(t *testing.T) {
	setBaseEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != ProviderScaleway {
		t.Errorf("Provider = %q, want scaleway", cfg.Provider)
	}
	if cfg.Model != "qwen3.8-27b" {
		t.Errorf("Model = %q", cfg.Model)
	}
	if cfg.BaseURL != "https://api.scaleway.ai" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.HTTPAddr != ":8084" {
		t.Errorf("HTTPAddr = %q, want :8084", cfg.HTTPAddr)
	}
	if cfg.APIKey != "" {
		t.Errorf("APIKey debería estar vacío por defecto")
	}
}

func TestLoad_PortOverridesAddr(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("PORT", "9999")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":9999" {
		t.Errorf("HTTPAddr = %q, want :9999", cfg.HTTPAddr)
	}
}

func TestLoad_InvalidProvider(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("AI_PROVIDER", "gpt")

	if _, err := Load(); err == nil {
		t.Fatal("se esperaba error por AI_PROVIDER inválido")
	}
}

func TestLoad_OpenAICompatRequiresBaseURL(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("AI_PROVIDER", "openai-compat")
	t.Setenv("AI_MODEL", "mi-modelo-soberano")
	// Sin AI_BASE_URL: debe fallar.

	_, err := Load()
	if err == nil {
		t.Fatal("se esperaba error por AI_BASE_URL ausente en openai-compat")
	}
	if !strings.Contains(err.Error(), "AI_BASE_URL") {
		t.Errorf("error = %q, debería mencionar AI_BASE_URL", err)
	}
}

func TestLoad_OpenAICompatConfigured(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("AI_PROVIDER", "openai-compat")
	t.Setenv("AI_MODEL", "mistral-large")
	t.Setenv("AI_BASE_URL", "https://llm.soberano.eu/")
	t.Setenv("AI_API_KEY", "sk-secreto")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != ProviderOpenAICompat {
		t.Errorf("Provider = %q", cfg.Provider)
	}
	// La barra final se recorta.
	if cfg.BaseURL != "https://llm.soberano.eu" {
		t.Errorf("BaseURL = %q, se esperaba sin barra final", cfg.BaseURL)
	}
}

// TestLogValue_NoFiltraApiKey es un test de SEGURIDAD: registrar la Config no
// debe filtrar jamás la API key.
func TestLogValue_NoFiltraApiKey(t *testing.T) {
	setBaseEnv(t)
	const secret = "sk-super-secreto-que-no-debe-aparecer"
	t.Setenv("AI_API_KEY", secret)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logger.Info("config", "config", cfg)

	out := buf.String()
	if strings.Contains(out, secret) {
		t.Fatalf("la API key se filtró en el log: %s", out)
	}
	if !strings.Contains(out, "api_key_set=true") {
		t.Errorf("el log debería indicar api_key_set=true; log=%s", out)
	}
	if !strings.Contains(out, "scaleway") {
		t.Errorf("el log debería incluir el proveedor; log=%s", out)
	}
}

// Los dos modelos por defecto son la decisión medida del 2026-09-09, así que se
// fijan aquí: si alguien cambia el rápido por uno que razona, la latencia del
// panel se multiplica por veinte sin que nada lo avise.
func TestLoad_DosModelosPorDefecto(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("AI_PROVIDER", "scaleway")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Model != "qwen3.8-27b" {
		t.Errorf("modelo interactivo = %q", cfg.Model)
	}
	if cfg.ModelCalidad != "gemma-4-26b-a4b-it" {
		t.Errorf("modelo de calidad = %q", cfg.ModelCalidad)
	}
	// El tope del de calidad tiene que dar aire al razonamiento: con 1024
	// —el del interactivo— gemma-4 devuelve vacío. Medido.
	if cfg.MaxTokensCalidad <= cfg.MaxTokens {
		t.Errorf("MaxTokensCalidad (%d) debe superar a MaxTokens (%d)", cfg.MaxTokensCalidad, cfg.MaxTokens)
	}
	if cfg.TimeoutCalidad <= cfg.Timeout {
		t.Errorf("TimeoutCalidad (%v) debe superar a Timeout (%v)", cfg.TimeoutCalidad, cfg.Timeout)
	}
}

// En un endpoint soberano no se puede suponer qué modelos hay: sin declararlo,
// no hay modo calidad, y eso NO debe impedir arrancar.
func TestLoad_SinModeloCalidadEnOpenAICompat(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("AI_PROVIDER", "openai-compat")
	t.Setenv("AI_BASE_URL", "https://llm.interno")
	t.Setenv("AI_MODEL", "lo-que-sirva")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ModelCalidad != "" {
		t.Errorf("ModelCalidad = %q, se esperaba vacío", cfg.ModelCalidad)
	}
}
