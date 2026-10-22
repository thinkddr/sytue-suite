// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// avisosDeLaSuite son los tipos con los que los servicios y la app nativa dejan
// un aviso en el Hub: hubnotify.Notification, peticiones.Aviso y el aviso propio
// del cliente de Fyne.
var avisosDeLaSuite = map[string]bool{"Notification": true, "Aviso": true, "HubAvisoPropio": true}

// TestLosEmisoresUsanOrigenesAdmitidos compara los orígenes que ESCRIBEN los
// emisores con los que el Hub ADMITE. Cast avisaba con "cast", el Hub no lo
// conocía y el aviso «en directo» se descartaba con ErrUnknownSource: el emisor
// lo manda en segundo plano y solo deja un log, así que nada se ponía en rojo.
func TestLosEmisoresUsanOrigenesAdmitidos(t *testing.T) {
	raiz := filepath.Join("..", "..", "..", "..")
	vistos := map[string]string{}
	for _, dir := range []string{"services", "platform", filepath.Join("apps", "fyne")} {
		err := filepath.WalkDir(filepath.Join(raiz, dir), func(ruta string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" || d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(ruta, ".go") || strings.HasSuffix(ruta, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(token.NewFileSet(), ruta, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok || !esAvisoDeLaSuite(lit.Type) {
					return true
				}
				for _, e := range lit.Elts {
					kv, ok := e.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if k, ok := kv.Key.(*ast.Ident); !ok || k.Name != "Source" {
						continue
					}
					if v, ok := kv.Value.(*ast.BasicLit); ok && v.Kind == token.STRING {
						s, _ := strconv.Unquote(v.Value)
						vistos[s] = ruta
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("recorrer %s: %v", dir, err)
		}
	}
	// Si no ve estos, el recorrido no está mirando donde cree.
	for _, s := range []string{"calendar", "photos", "one"} {
		if _, ok := vistos[s]; !ok {
			t.Fatalf("no he visto a nadie avisar con %q: el guardián está ciego", s)
		}
	}
	for s, ruta := range vistos {
		if !ValidNotificationSource(Source(s)) {
			t.Errorf("%s avisa al Hub con el origen %q y el Hub no lo admite: el aviso se descarta con ErrUnknownSource. Añádelo a domain.go", ruta, s)
		}
	}
}

func esAvisoDeLaSuite(e ast.Expr) bool {
	switch t := e.(type) {
	case *ast.SelectorExpr:
		return avisosDeLaSuite[t.Sel.Name]
	case *ast.Ident:
		return avisosDeLaSuite[t.Name]
	}
	return false
}
