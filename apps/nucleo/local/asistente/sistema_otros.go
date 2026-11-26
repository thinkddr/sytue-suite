// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !windows && !darwin && !android

package asistente

import "context"

// En Linux no hay IA del sistema: el modelo en este equipo (Ollama) cumple ese papel.
// iOS (Foundation Models, por un puente Swift) y Android (Gemini Nano, por ML Kit en Java)
// tienen el suyo: sistema_ios.go y sistema_android.go.

// NombreSistema es el nombre de la IA del sistema en Ajustes.
func NombreSistema() string { return "IA del sistema (no disponible en este sistema)" }

// EstadoDelSistema es el de la IA del sistema del MÓVIL: en el escritorio no hay estado
// que pintar (Ajustes la nombra por su sistema) y es SistemaComprobando.
func EstadoDelSistema() EstadoSistema { return SistemaComprobando }

// SistemaDisponible: no hay.
func SistemaDisponible(context.Context) bool { return false }

func completarSistema(context.Context, Config, string, string) (string, error) {
	return "", ErrSinIANativa
}
