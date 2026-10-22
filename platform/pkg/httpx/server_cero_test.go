// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Un `ReadTimeout: 0` escrito a mano en un ServerConfig nunca hace lo que parece:
// 0 es «el valor por defecto», igual que no escribirlo. Quien lo escribe quería
// quitar el límite —Drive y Rooms lo tenían así, con las subidas grandes y
// los WebSocket cortados a los 30 s—, y para eso está SinTimeout.
func TestNadiePasaCeroCreyendoQuitarElLimite(t *testing.T) {
	raiz := raizDelModulo(t)
	vigilados := map[string]bool{"ReadTimeout": true, "WriteTimeout": true, "IdleTimeout": true}
	var hallazgos []string
	vistos := 0
	err := filepath.WalkDir(filepath.Join(raiz, "services"), func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(ruta, ".go") || strings.HasSuffix(ruta, "_test.go") {
			return err
		}
		fset := token.NewFileSet()
		arbol, err := parser.ParseFile(fset, ruta, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(arbol, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "ServerConfig" {
				return true
			}
			vistos++
			for _, e := range lit.Elts {
				kv, ok := e.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				clave, ok := kv.Key.(*ast.Ident)
				if !ok || !vigilados[clave.Name] {
					continue
				}
				if v, ok := kv.Value.(*ast.BasicLit); ok && v.Kind == token.INT && v.Value == "0" {
					rel, _ := filepath.Rel(raiz, ruta)
					hallazgos = append(hallazgos, rel+":"+strconv.Itoa(fset.Position(kv.Pos()).Line)+" "+clave.Name+": 0")
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Si el recorrido no ve ningún ServerConfig, el guardián está ciego (ruta mal
	// resuelta, tipo renombrado) y un verde no significaría nada.
	if vistos == 0 {
		t.Fatal("no se encontró ningún ServerConfig en services/: el test no está mirando nada")
	}
	if len(hallazgos) > 0 {
		t.Fatalf("timeout a 0 en un ServerConfig: 0 es el valor por defecto, no «sin límite». "+
			"Usa httpx.SinTimeout o quita la línea:\n  %s", strings.Join(hallazgos, "\n  "))
	}
}

func raizDelModulo(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		padre := filepath.Dir(dir)
		if padre == dir {
			t.Fatal("no se encontró go.mod subiendo desde el directorio del test")
		}
		dir = padre
	}
}
