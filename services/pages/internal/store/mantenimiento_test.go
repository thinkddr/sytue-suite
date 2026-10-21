// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// desdeHTTP son las ÚNICAS funciones del store en modo mantenimiento que puede
// llamar la capa HTTP: los webhooks llegan sin sesión y sin el Host de la web, y
// con ellas averiguan el tenant; todo lo demás lo hacen después en enTenant.
var desdeHTTP = map[string]bool{"TenantDePago": true, "TenantDeSitio": true}

// Una transacción de mantenimiento cruza organizaciones. Si un manejador HTTP la
// abre —directamente o por una función del store que la use—, una petición de la
// web A puede leer o escribir datos de la B. Este test lo impide por construcción:
// calcula qué funciones del store acaban en enMantenimiento (también a través de
// otras) y falla si internal/http llama a alguna que no sea de desdeHTTP, o si toca
// el modo mantenimiento por su cuenta.
func TestMantenimientoSoloDondeToca(t *testing.T) {
	enMant := funcionesEnMantenimiento(t)
	for nombre := range desdeHTTP {
		if !enMant[nombre] {
			t.Errorf("%s ya no usa enMantenimiento: o se quita de desdeHTTP o el test mira mal", nombre)
		}
	}

	ficheros := fuentes(t, filepath.Join("..", "http"))
	for ruta, src := range ficheros {
		for _, prohibido := range []string{"enMantenimiento", "InMaintenanceTx", "SetMaintenance", "app.maintenance"} {
			if strings.Contains(src, prohibido) {
				t.Errorf("%s usa %s: la capa HTTP no abre transacciones de mantenimiento", ruta, prohibido)
			}
		}
		arbol, err := parser.ParseFile(token.NewFileSet(), ruta, src, 0)
		if err != nil {
			t.Fatalf("%s: %v", ruta, err)
		}
		ast.Inspect(arbol, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && enMant[sel.Sel.Name] && !desdeHTTP[sel.Sel.Name] {
				t.Errorf("%s llama a store.%s, que cruza organizaciones: desde HTTP solo %v",
					ruta, sel.Sel.Name, claves(desdeHTTP))
			}
			return true
		})
	}

	// Y `app.pages_publico` lo fija una sola función en todo el servicio: enPublico.
	fijan := map[string]bool{}
	for ruta, src := range fuentes(t, filepath.Join("..", "..")) {
		arbol, err := parser.ParseFile(token.NewFileSet(), ruta, src, 0)
		if err != nil {
			t.Fatalf("%s: %v", ruta, err)
		}
		ast.Inspect(arbol, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			ast.Inspect(fn.Body, func(m ast.Node) bool {
				if lit, ok := m.(*ast.BasicLit); ok && strings.Contains(lit.Value, "app.pages_publico") {
					fijan[filepath.Base(ruta)+":"+fn.Name.Name] = true
				}
				return true
			})
			return false
		})
	}
	if len(fijan) != 1 || !fijan["store.go:enPublico"] {
		t.Errorf("app.pages_publico se fija en %v; solo puede hacerlo enPublico", claves(fijan))
	}
}

// funcionesEnMantenimiento devuelve las funciones del paquete store que abren una
// transacción de mantenimiento, directamente o llamando a otra que lo haga.
func funcionesEnMantenimiento(t *testing.T) map[string]bool {
	t.Helper()
	llama := map[string][]string{} // función → nombres a los que llama
	for ruta, src := range fuentes(t, ".") {
		arbol, err := parser.ParseFile(token.NewFileSet(), ruta, src, 0)
		if err != nil {
			t.Fatalf("%s: %v", ruta, err)
		}
		for _, d := range arbol.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.SelectorExpr:
					llama[fn.Name.Name] = append(llama[fn.Name.Name], x.Sel.Name)
				case *ast.Ident:
					llama[fn.Name.Name] = append(llama[fn.Name.Name], x.Name)
				}
				return true
			})
		}
	}
	en := map[string]bool{"enMantenimiento": true}
	for cambio := true; cambio; {
		cambio = false
		for fn, nombres := range llama {
			if en[fn] || fn == "enMantenimiento" {
				continue
			}
			for _, n := range nombres {
				if en[n] {
					en[fn], cambio = true, true
					break
				}
			}
		}
	}
	delete(en, "enMantenimiento")
	if len(en) == 0 {
		t.Fatal("ninguna función del store usa enMantenimiento: el test no mira donde cree")
	}
	return en
}

// fuentes lee los .go que no son de test bajo dir.
func fuentes(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(ruta string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(ruta, ".go") || strings.HasSuffix(ruta, "_test.go") {
			return err
		}
		b, err := os.ReadFile(ruta)
		out[ruta] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatalf("ningún .go en %s: el test no mira donde cree", dir)
	}
	return out
}

func claves(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
