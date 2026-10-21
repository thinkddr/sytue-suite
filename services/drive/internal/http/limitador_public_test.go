// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"strings"
	"testing"
)

// TestElLimitadorNoGuardaElToken (FILE-13): la clave del limitador de intentos
// de contraseña es de tamaño fijo y no contiene el token.
func TestElLimitadorNoGuardaElToken(t *testing.T) {
	largo := strings.Repeat("x", 8000)
	if k := claveDelLimitador(largo); len(k) != 64 || strings.Contains(k, "xxxx") {
		t.Fatalf("clave: %d bytes", len(k))
	}
	if claveDelLimitador("a") == claveDelLimitador("b") {
		t.Fatal("dos tokens, dos claves")
	}
}
