// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNuevoVacioErrorEsElTonoDeError: el atajo deja el `tone="error"` puesto.
func TestNuevoVacioErrorEsElTonoDeError(t *testing.T) {
	v := NuevoVacioError("No se pudo abrir", "")
	if v.Tono != VacioError || v.Titulo != "No se pudo abrir" {
		t.Fatalf("NuevoVacioError = tono %v, título %q", v.Tono, v.Titulo)
	}
}

// TestElFalloDeLaBovedaEsUnVacioDeError (DS-06): la pantalla de «no se pudo abrir el
// almacenamiento cifrado» (`matrix.failed`) de cada `cmd/*/main.go` era un estado vacío
// NEUTRO, sin marca: igual que una lista vacía. Es un fallo y va con NuevoVacioError.
// VISTO EN ROJO con los trece `ui.NuevoVacio(t.T("matrix.failed", nil), "")` de antes.
func TestElFalloDeLaBovedaEsUnVacioDeError(t *testing.T) {
	neutro := regexp.MustCompile(`ui\.NuevoVacio\([^()]*\(\s*"matrix\.failed"`)
	vistos := 0
	err := filepath.WalkDir("..", func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(ruta, ".go") || strings.HasSuffix(ruta, "_test.go") {
			return err
		}
		b, err := os.ReadFile(ruta)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), `"matrix.failed"`) {
			vistos++
		}
		if neutro.Match(b) {
			t.Errorf("%s pinta `matrix.failed` con un vacío neutro: es ui.NuevoVacioError", ruta)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if vistos < 10 {
		t.Fatalf("solo %d ficheros con matrix.failed: el guardián ya no los encuentra", vistos)
	}
}
