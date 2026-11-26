// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios && !cgo

package asistente

import "context"

// Sin cgo no se llega al puente de Swift (sistema_ios.go): solo sirve para comprobar
// que el paquete compila para iOS fuera de un Mac.

// NombreSistema es el nombre de la IA del sistema en español (ver sistema_ios.go).
func NombreSistema() string { return "Apple Intelligence (en el dispositivo)" }

// EstadoDelSistema: sin cgo no hay puente.
func EstadoDelSistema() EstadoSistema { return SistemaSinPuente }

// SistemaDisponible: sin cgo, no.
func SistemaDisponible(context.Context) bool { return false }

func completarSistema(context.Context, Config, string, string) (string, error) {
	return "", &ErrorSistema{SistemaSinPuente}
}
