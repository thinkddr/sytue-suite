// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build medicion

package provider_test

import (
	"context"
	"os"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/ai/internal/provider"
)

// TestMedicionModelosCodigo evalúa los candidatos para AI_MODEL_CODIGO (§9.1).
// Ejecutado por el coordinador con la clave de Scaleway:
//
//	AI_API_KEY=... go test -v -tags medicion -run TestMedicionModelosCodigo ./services/ai/internal/provider/...
func TestMedicionModelosCodigo(t *testing.T) {
	apiKey := os.Getenv("AI_API_KEY")
	if apiKey == "" {
		t.Skip("Se requiere AI_API_KEY para ejecutar la batería de medición")
	}
	baseURL := os.Getenv("AI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.scaleway.ai"
	}

	modelos := []string{
		"qwen3.5-397b-a17b",
		"glm-5.2",
		"mistral-medium-3.5-128b",
		"gpt-oss-120b",
		"qwen3.8-27b",
		// REGLA §9.1: NUNCA qwen3-coder-30b ni devstral-2.
	}

	tareas := []struct {
		nombre      string
		action      string
		instruction string
		contexto    string
		kind        string
	}{
		{
			nombre:      "sección Astro desde IR",
			action:      "generar_componente",
			instruction: "Genera un componente Astro (.astro) a partir del IR de sección proporcionado, con TypeScript y accesibilidad semántica.",
			contexto:    `{"type":"section","kind":"hero","props":{"title":"Título Hero","cta":{"text":"Contactar","url":"/contacto"}}}`,
			kind:        "astro",
		},
		{
			nombre:      "tema Shopify",
			action:      "generar_liquid",
			instruction: "Genera una sección Liquid para Shopify OS 2.0 con esquema de configuración JSON compatible.",
			contexto:    `{"name":"Banner Promocional","settings":[{"type":"text","id":"heading","label":"Título"}]}`,
			kind:        "shopify",
		},
		{
			nombre:      "pantalla SwiftUI",
			action:      "generar_swiftui",
			instruction: "Genera una vista SwiftUI idiomática y responsive a partir de la estructura indicada.",
			contexto:    `{"screen":"Profile","elements":[{"type":"avatar"},{"type":"text","value":"Nombre"},{"type":"button","label":"Guardar"}]}`,
			kind:        "swiftui",
		},
	}

	const repeticiones = 3

	for _, modelo := range modelos {
		t.Run(modelo, func(t *testing.T) {
			p, err := provider.New(provider.Config{
				Provider:  provider.ProviderScaleway,
				APIKey:    apiKey,
				Model:     modelo,
				BaseURL:   baseURL,
				MaxTokens: 4096,
				Timeout:   180 * time.Second,
			})
			if err != nil {
				t.Fatalf("crear proveedor para %s: %v", modelo, err)
			}

			for _, tarea := range tareas {
				t.Run(tarea.nombre, func(t *testing.T) {
					for rep := 1; rep <= repeticiones; rep++ {
						ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
						userMsg := provider.BuildUserMessage(provider.Request{
							Action:      tarea.action,
							Instruction: tarea.instruction,
							Context:     tarea.contexto,
							Kind:        tarea.kind,
						})

						start := time.Now()
						res, err := p.Complete(ctx, provider.SystemPromptPages, userMsg, provider.Opciones{})
						elapsed := time.Since(start)
						cancel()

						if err != nil {
							t.Errorf("rep %d falló: %v", rep, err)
							continue
						}
						t.Logf("rep %d: latencia=%v, tokens_entrada=%d, tokens_salida=%d, len=%d",
							rep, elapsed, res.TokensEntrada, res.TokensSalida, len(res.Texto))
					}
				})
			}
		})
	}
}
