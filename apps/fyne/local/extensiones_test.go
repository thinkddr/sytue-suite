// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLasAppsDeOfficeDeclaranSusFormatos es, para el árbol de Sytue Office, lo que
// `sesion.TestCadaAppDeclaraLasExtensionesDeSuProducto` es para `cmd/*`: cada editor de
// `cmd-local` se ofrece en «Abrir con» para TODO lo que `api.ExtensionesOffice` dice que
// abre —lo de la web más su formato nativo— (puede declarar más: `.txt`, `.csv`…), y
// atiende lo que llega (`AceptarFicheros`).
func TestLasAppsDeOfficeDeclaranSusFormatos(t *testing.T) {
	for dir, producto := range map[string]string{"docus": "docs", "calcus": "sheets", "diapos": "slides", "certipdf": "pdf"} {
		toml, err := os.ReadFile(filepath.Join("..", "cmd-local", dir, "FyneApp.toml"))
		if err != nil {
			t.Fatal(err)
		}
		principal, err := os.ReadFile(filepath.Join("..", "cmd-local", dir, "main.go"))
		if err != nil {
			t.Fatal(err)
		}
		var declaradas []string
		for _, linea := range strings.Split(string(toml), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(linea), "extensiones = "); ok {
				declaradas = strings.Split(strings.Trim(v, `"`), ",")
			}
		}
		for _, ext := range api.ExtensionesOffice(producto) {
			if !slices.Contains(declaradas, ext) {
				t.Errorf("cmd-local/%s no declara .%s, que abre %s", dir, ext, producto)
			}
		}
		if !strings.Contains(string(principal), ".AceptarFicheros(") {
			t.Errorf("cmd-local/%s declara extensiones y no llama a AceptarFicheros", dir)
		}
	}
}
