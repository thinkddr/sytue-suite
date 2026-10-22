// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ir_test

import (
	"bytes"
	"os"
	"testing"

	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

func TestEsquemaIAAlDia(t *testing.T) {
	generado, err := ir.GenerarEsquemaIA()
	if err != nil {
		t.Fatalf("GenerarEsquemaIA falló: %v", err)
	}

	disco, err := os.ReadFile("esquema_ia.json")
	if err != nil {
		t.Fatalf("no se pudo leer esquema_ia.json: %v", err)
	}

	if !bytes.Equal(generado, disco) {
		t.Fatal("esquema_ia.json en disco no coincide con GenerarEsquemaIA() en memoria (ejecuta go generate)")
	}
}
