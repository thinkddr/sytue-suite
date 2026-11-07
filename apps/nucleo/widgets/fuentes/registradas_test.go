// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

// TestHayUnaFuentePorApp: cada app con widget (su esquema en FyneApp.toml, menos la
// demo) tiene su Fábrica, y no hay ninguna de una app que no existe. Sin esto, el
// refresco sin ventana de Android —que no corre el `main` de la app— no sabría leer el
// widget de una app nueva, y nadie lo notaría en el escritorio.
func TestHayUnaFuentePorApp(t *testing.T) {
	tomls, err := filepath.Glob("../../../fyne/cmd/*/FyneApp.toml")
	if err != nil || len(tomls) == 0 {
		t.Fatalf("sin los FyneApp.toml de las apps: %v", err)
	}
	esquema := regexp.MustCompile(`(?m)^\s*esquema\s*=\s*"sytue-([a-z-]+)"`)
	var apps []string
	for _, ruta := range tomls {
		datos, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}
		m := esquema.FindSubmatch(datos)
		if m == nil {
			t.Fatalf("%s no declara su esquema", ruta)
		}
		if p := string(m[1]); p != "fyne-demo" {
			apps = append(apps, p)
		}
	}
	todas := Registradas()
	registrados := make([]string, 0, len(todas))
	for p := range todas {
		registrados = append(registrados, p)
	}
	slices.Sort(apps)
	slices.Sort(registrados)
	if !slices.Equal(apps, registrados) {
		t.Fatalf("Fábricas %v; apps con widget %v", registrados, apps)
	}
	if _, ok := Registrada("fyne-demo"); ok {
		t.Fatal("la demo no tiene widget")
	}
}
