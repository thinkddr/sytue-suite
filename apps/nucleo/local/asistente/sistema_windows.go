// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package asistente

import (
	"context"
	"net/http"
	"os/exec"
	"regexp"
	"time"

	provider "git.sytue.eu/sytue/one/apps/nucleo/ia/proveedor"
)

// En Windows la IA del dispositivo que Microsoft abre a otras apps es Foundry Local: el
// runtime de modelos locales (Phi y otros), que usa la NPU de los PC Copilot+ y expone un
// servidor compatible con OpenAI en este equipo. La app de Copilot no tiene API para
// terceros, y las «Windows AI APIs» (Phi Silica) exigen identidad de paquete MSIX y WinRT;
// quedan para cuando la app se empaquete así (README de la edición).

// NombreSistema es el nombre de la IA del sistema en Ajustes.
func NombreSistema() string { return "IA de Windows en el dispositivo (Foundry Local)" }

// EstadoDelSistema es el de la IA del sistema del MÓVIL: en el escritorio no hay estado
// que pintar (Ajustes la nombra por su sistema) y es SistemaComprobando.
func EstadoDelSistema() EstadoSistema { return SistemaComprobando }

var urlFoundry = regexp.MustCompile(`https?://(?:127\.0\.0\.1|localhost):\d+`)

// clienteFoundry habla SOLO con el Foundry de este equipo (urlFoundry no casa otra cosa que
// 127.0.0.1 o localhost), con un transporte propio: el `http.DefaultTransport` de Sytue
// Office lo deniega todo (el portero, `local/red`), y esto no sale del equipo.
var clienteFoundry = &http.Client{Timeout: 125 * time.Second, Transport: &http.Transport{Proxy: nil}}

// direccionFoundry pregunta a `foundry service status` dónde escucha el servicio.
func direccionFoundry(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "foundry", "service", "status").CombinedOutput()
	if err != nil {
		return "", &ErrorEscritorio{Codigo: EscritorioFaltaFoundry}
	}
	u := urlFoundry.FindString(string(out))
	if u == "" {
		return "", &ErrorEscritorio{Codigo: EscritorioFoundryParado}
	}
	return u + "/v1", nil
}

// SistemaDisponible dice si Foundry Local está en marcha.
func SistemaDisponible(ctx context.Context) bool {
	_, err := direccionFoundry(ctx)
	return err == nil
}

func completarSistema(ctx context.Context, cfg Config, system, user string) (string, error) {
	base, err := direccionFoundry(ctx)
	if err != nil {
		return "", err
	}
	modelo := cfg.Modelo
	if modelo == "" {
		modelo = "phi-4-mini"
	}
	p, err := provider.New(provider.Config{Provider: provider.ProviderOpenAICompat, APIKey: "local", Model: modelo, BaseURL: base, MaxTokens: 4096, Timeout: 120 * time.Second, HTTPClient: clienteFoundry})
	if err != nil {
		return "", err
	}
	res, err := p.Complete(ctx, system, user, provider.Opciones{})
	return res.Texto, err
}
