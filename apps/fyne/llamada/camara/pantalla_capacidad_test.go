// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package camara

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// Go decide qué backend entra en cada app: Android hereda linux, iOS hereda darwin.
// Se comprueba la capacidad sobre esos mismos archivos, sin SDK ni cámara reales.
func TestPantallaDisponiblePorBackend(t *testing.T) {
	archivos, err := filepath.Glob("pantalla_*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, caso := range []struct {
		so, backend string
		disponible  bool
	}{
		{"android", "pantalla_android.go", true},
		{"darwin", "pantalla_darwin.go", true},
		{"linux", "pantalla_linux.go", true},
		{"windows", "pantalla_windows.go", true},
		{"ios", "pantalla_otros.go", false},
		{"freebsd", "pantalla_otros.go", false},
	} {
		t.Run(caso.so, func(t *testing.T) {
			contexto := build.Default
			contexto.GOOS, contexto.GOARCH, contexto.CgoEnabled = caso.so, "arm64", true
			var elegidos []string
			for _, archivo := range archivos {
				if strings.HasSuffix(archivo, "_test.go") {
					continue
				}
				entra, err := contexto.MatchFile(".", archivo)
				if err != nil {
					t.Fatal(err)
				}
				if entra {
					elegidos = append(elegidos, archivo)
				}
			}
			if len(elegidos) != 1 || elegidos[0] != caso.backend {
				t.Fatalf("backends de %s: %v", caso.so, elegidos)
			}
			if got := capacidadDePantalla(t, caso.backend); got != caso.disponible {
				t.Fatalf("%s declara capacidad %v, debe ser %v", caso.so, got, caso.disponible)
			}
		})
	}
}

func capacidadDePantalla(t *testing.T, archivo string) bool {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), archivo, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var valor *ast.Ident
	ast.Inspect(f, func(n ast.Node) bool {
		if v, ok := n.(*ast.ValueSpec); ok && len(v.Names) == 1 && v.Names[0].Name == "PantallaDisponible" && len(v.Values) == 1 {
			valor, _ = v.Values[0].(*ast.Ident)
		}
		return true
	})
	if valor == nil || (valor.Name != "true" && valor.Name != "false") {
		t.Fatalf("%s no declara la capacidad de pantalla de su backend", archivo)
	}
	return valor.Name == "true"
}
