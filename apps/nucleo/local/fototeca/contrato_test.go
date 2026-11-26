// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fototeca_test

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/contrato"
)

// abrirEn abre la fototeca de dir como Photos de Sytue Office.
func abrirEn(t *testing.T, dir string) *fototeca.Fototeca {
	t.Helper()
	f, err := fototeca.Abrir(t.Context(), dir, "instalacion-1", fototeca.Opciones{Formatos: []string{"webp", "avif", "bmp"}})
	if err != nil {
		t.Fatal("abrir:", err)
	}
	return f
}

// TestContratoDeLaFototeca (§8.2): el mismo juego de pruebas de comportamiento que pasa
// el repositorio de la conectada sobre el almacén de registros (`photos/contrato`), sobre
// la fototeca de SQLite. Aquí lo borrado para siempre se va del disco al borrarlo.
func TestContratoDeLaFototeca(t *testing.T) {
	contrato.Probar(t, contrato.Puerto{
		Abrir: func(t *testing.T, dir string) (contrato.Fototeca, func()) {
			f := abrirEn(t, dir)
			return f, f.Close
		},
	})
}
