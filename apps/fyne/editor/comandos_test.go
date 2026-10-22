// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// RegistroWeb es el registro común de comandos de la cinta: una fila por comando, con
// su rótulo, atajo, tipos de documento, pestaña y grupo. La fuente vive junto al
// catálogo de descargas (apps/nucleo/office) y scripts/sync-office-catalog.sh publica
// su copia de build para la web.
const RegistroWeb = "apps/nucleo/office/comandos.json"

// CopiaWeb es esa copia de build: el test la compara con la fuente, igual que
// hace el CI con `sync-office-catalog.sh --check`.
const CopiaWeb = "apps/frontend/packages/office/src/editor/comandos.json"

// TestCadaComandoNativoEstaEnElRegistro: toda constante de tipo [Comando] de la app
// nativa —las de aquí y las de cada producto (`calcus`, `docus`…)— tiene su fila en el
// registro de la web. Es la mitad Go del gate; la de TypeScript la pone el compilador
// (`ComandosSinRegistrar` en `editor/comandos.ts`).
//
// Se leen las FUENTES y no una lista a mano: una lista a mano es justo lo que se olvida
// al añadir el comando.
//
// VISTO EN ROJO (2026-10-01, en el frontal): con `"pageBreak"` borrado del JSON, el test
// lo nombra junto al fichero que lo declara; repuesto, verde.
func TestCadaComandoNativoEstaEnElRegistro(t *testing.T) {
	var registro map[string]json.RawMessage
	if err := json.Unmarshal([]byte(webref.Read(t, RegistroWeb)), &registro); err != nil {
		t.Fatalf("%s: %v", RegistroWeb, err)
	}

	if webref.Read(t, CopiaWeb) != webref.Read(t, RegistroWeb) {
		t.Errorf("%s no es copia de %s: bash scripts/sync-office-catalog.sh", CopiaWeb, RegistroWeb)
	}

	nativos := comandosNativos(t, "..")
	// Sin esto, un parser que dejara de ver las constantes pasaría en verde.
	if len(nativos) < 100 {
		t.Fatalf("solo se ven %d comandos nativos: el recorrido ha dejado de encontrarlos", len(nativos))
	}
	var faltan []string
	for id, donde := range nativos {
		if _, ok := registro[id]; !ok {
			faltan = append(faltan, id+" ("+donde+")")
		}
	}
	sort.Strings(faltan)
	if len(faltan) > 0 {
		t.Errorf("comandos nativos sin fila en %s:\n  %s", RegistroWeb, strings.Join(faltan, "\n  "))
	}
}

// comandosNativos son los valores de las constantes `Comando` (o `editor.Comando`) del
// módulo, con el fichero que las declara.
func comandosNativos(t *testing.T, raiz string) map[string]string {
	t.Helper()
	out := map[string]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(raiz, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(ruta, ".go") || strings.HasSuffix(ruta, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, ruta, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs := spec.(*ast.ValueSpec)
				if !esComando(vs.Type) {
					continue
				}
				for _, v := range vs.Values {
					if lit, ok := v.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						id, _ := strconv.Unquote(lit.Value)
						out[id] = filepath.ToSlash(ruta)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func esComando(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name == "Comando"
	case *ast.SelectorExpr:
		return x.Sel.Name == "Comando"
	}
	return false
}

// TestLaCopiaWebDeLasPestanasEsLaFuente: la web pinta SU copia; si se queda atrás de
// `apps/nucleo/office/pestanas.json`, las dos cintas dejan de coincidir sin que nada rompa.
func TestLaCopiaWebDeLasPestanasEsLaFuente(t *testing.T) {
	if webref.Read(t, PestanasWeb) != webref.Read(t, "apps/nucleo/office/pestanas.json") {
		t.Errorf("%s no es copia de la fuente: bash scripts/sync-office-catalog.sh", PestanasWeb)
	}
}

// TestCotejarPestanasCazaLasDiferencias: el cotejo cae con una pestaña de menos o un rótulo
// distinto; las pestañas reales de cada producto las cotejan sus paquetes con `CotejarPestanas`.
func TestCotejarPestanasCazaLasDiferencias(t *testing.T) {
	web := []byte(webref.Read(t, PestanasWeb))
	r := Genericas()
	r.Anadir("datos", func(*Contexto) fyne.CanvasObject { return nil })
	r.Anadir("formulas", func(*Contexto) fyne.CanvasObject { return nil })
	if p := CotejarPestanas(r, Hoja, web); len(p) != 0 {
		t.Errorf("una hoja con Datos y Fórmulas es la de la web: %v", p)
	}
	// Un documento sin Insertar, Diseño y Referencias NO es el de la web: el guardián habla.
	if p := CotejarPestanas(Genericas(), Documento, web); len(p) == 0 {
		t.Error("faltan pestañas del documento y el cotejo no lo dice")
	}
	// Mismo número de pestañas pero rótulo cambiado: tampoco vale.
	r.pestanas[0].Rotulo = "ribbon.tab.otra"
	if p := CotejarPestanas(r, Hoja, web); len(p) == 0 {
		t.Error("un rótulo distinto no se detecta")
	}
}
