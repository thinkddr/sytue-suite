// SPDX-License-Identifier: Apache-2.0
package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// Inspect the canonical wire declarations AND the backend's actual aliases.
// A disconnected copy of a model must never satisfy a transport contract.
func sharedServiceModelContract(t *testing.T, backend, model string, types, constants map[string]string, canonical ...string) string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), backend, webref.Read(t, backend), 0)
	if err != nil {
		t.Fatal(err)
	}
	qualifier := ""
	for _, spec := range file.Imports {
		value, err := strconv.Unquote(spec.Path.Value)
		if err == nil && value == model {
			qualifier = path.Base(model)
			if spec.Name != nil {
				qualifier = spec.Name.Name
			}
		}
	}
	if qualifier == "" || qualifier == "_" || qualifier == "." {
		t.Fatalf("%s no longer imports the shared model %s", backend, model)
	}
	typeAliases, constantAliases := sharedModelAliases(file, qualifier)
	for name, expected := range types {
		if typeAliases[name] != expected {
			t.Errorf("%s must alias shared type %s = %s.%s", backend, name, qualifier, expected)
		}
	}
	for name, expected := range constants {
		if constantAliases[name] != expected {
			t.Errorf("%s must use shared constant %s = %s.%s", backend, name, qualifier, expected)
		}
	}
	var source strings.Builder
	for _, filename := range canonical {
		source.WriteString(webref.Read(t, filename))
		source.WriteByte('\n')
	}
	return source.String()
}

func sharedModelAliases(file *ast.File, qualifier string) (map[string]string, map[string]string) {
	types, constants := map[string]string{}, map[string]string{}
	for _, declaration := range file.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range group.Specs {
			if typ, ok := spec.(*ast.TypeSpec); ok && group.Tok == token.TYPE && typ.Assign.IsValid() {
				if name := selectedModelName(typ.Type, qualifier); name != "" {
					types[typ.Name.Name] = name
				}
			}
			if value, ok := spec.(*ast.ValueSpec); ok && group.Tok == token.CONST && len(value.Names) == len(value.Values) {
				addModelConstants(value, qualifier, constants)
			}
		}
	}
	return types, constants
}

func addModelConstants(value *ast.ValueSpec, qualifier string, constants map[string]string) {
	for i, name := range value.Names {
		if selected := selectedModelName(value.Values[i], qualifier); selected != "" {
			constants[name.Name] = selected
		}
	}
}

func selectedModelName(expression ast.Expr, qualifier string) string {
	selected, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	name, ok := selected.X.(*ast.Ident)
	if !ok || name.Name != qualifier {
		return ""
	}
	return selected.Sel.Name
}
