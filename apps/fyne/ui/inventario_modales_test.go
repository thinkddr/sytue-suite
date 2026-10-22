// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// # El inventario de modales, sacado del código
//
// Recorre TODO el módulo (las trece apps y sus paquetes) y cuenta cada sitio donde se
// abre algo encima. No es una lista a mano: si alguien abre un diálogo nuevo, sale aquí
// solo. Y vigila las tres formas en que el barrido del 2026-09-25 encontró modales que no
// se cerraban:
//
//  1. Una capa abierta por fuera de `ui` (`Overlays().Add`, `widget.NewPopUp`, el
//     `dialog` de Fyne): no pasa por `mostrar`, así que ni Escape sin foco ni el «atrás»
//     del teléfono llegan a ella. Solo valen las excepciones de abajo, cada una con su
//     porqué.
//  2. Un `AlCerrar` escrito como AVISO («se ha cerrado») donde es la PETICIÓN de cierre:
//     el aspa, Escape y el velo lo llaman y el diálogo se queda puesto. Así estaban el
//     selector de GIF y las integraciones de Rooms (este último, sin pie: no había forma
//     de cerrarlo).
//  3. Un control enfocable cuyo `TypedKey` se traga Escape: con el foco en él, el diálogo
//     de encima no se cierra con el teclado. Tiene que llamar a `ui.SubirEscape`, decir
//     quién atiende Escape, o delegar en otro `TypedKey`.

// capasPropias son las superposiciones abiertas por fuera de `ui`, con su porqué. Se
// comparan por IGUALDAD: una que desaparezca también pone esto en rojo.
var capasPropias = []string{
	// El visor, el editor y la foto pública de SyPhotos: capas a pantalla completa que
	// implementan `ui.Descartable` y cuyas teclas atiende `teclaSuelta` (sin foco, a
	// propósito: las flechas, Espacio, «i» y «z» son del visor).
	// El reproductor de Cast a pantalla completa en el teléfono: una capa a toda la
	// pantalla, por encima de la barra de la suite, que no es un diálogo (sin velo ni
	// foco atrapado). Es `ui.Descartable`, así que «atrás» y Escape la cierran.
	"apps/cast/app_movil.go: Overlays().Add",
	"apps/photos/app_editor.go: Overlays().Add",
	"apps/photos/app_publico.go: Overlays().Add",
	"apps/photos/app_visor.go: Overlays().Add",
	// Los selectores de ficheros del SISTEMA en el teléfono: el `dialog` de Fyne abre ahí
	// el del sistema, que se cierra con su propio «atrás».
	"cmd/drive/selector_movil.go: dialog.ShowFileOpenMultiple",
	"sesion/elegir_movil.go: dialog.NewFileOpen",
}

// aperturasSuite son las llamadas de `ui` que ponen algo encima.
var aperturasSuite = regexp.MustCompile(`^(NuevoDialogo|Confirmacion|Pregunta|PreguntaOpcional|MostrarPopover|MostrarEmergente|MostrarEmergenteEn|MostrarCalendario|NuevoMenu|NuevoFlotante)$`)

func inventario(t *testing.T) (suite map[string]int, propias, avisos, tragones []string) {
	t.Helper()
	raiz, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	suite = map[string]int{}
	type dudoso struct{ dir, sitio, cuerpo string }
	var dudosos []dudoso
	ayudantes := map[string][]string{}
	fset := token.NewFileSet()
	err = filepath.WalkDir(raiz, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "upstream" || d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(raiz, p)
		f, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		fuente := func(n ast.Node) string {
			return string(leer(t, p)[fset.Position(n.Pos()).Offset:fset.Position(n.End()).Offset])
		}
		enUI := strings.HasPrefix(rel, "ui/")
		ast.Inspect(f, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.CallExpr:
				sel, es := v.Fun.(*ast.SelectorExpr)
				if !es {
					return true
				}
				x, _ := sel.X.(*ast.Ident)
				switch {
				case x != nil && x.Name == "ui" && aperturasSuite.MatchString(sel.Sel.Name):
					suite[filepath.Dir(rel)+" "+sel.Sel.Name]++
				case enUI && x == nil && aperturasSuite.MatchString(sel.Sel.Name):
				case x != nil && x.Name == "dialog" && (strings.HasPrefix(sel.Sel.Name, "New") || strings.HasPrefix(sel.Sel.Name, "Show")):
					propias = append(propias, rel+": dialog."+sel.Sel.Name)
				case x != nil && x.Name == "widget" && strings.Contains(sel.Sel.Name, "PopUp"):
					propias = append(propias, rel+": widget."+sel.Sel.Name)
				case sel.Sel.Name == "Add" && !enUI:
					if in, es := sel.X.(*ast.CallExpr); es {
						if s2, es := in.Fun.(*ast.SelectorExpr); es && s2.Sel.Name == "Overlays" {
							propias = append(propias, rel+": Overlays().Add")
						}
					}
				}
			case *ast.AssignStmt:
				// `x.AlCerrar = func() { … }` sin nada que cierre.
				for i, l := range v.Lhs {
					sel, es := l.(*ast.SelectorExpr)
					if !es || sel.Sel.Name != "AlCerrar" || i >= len(v.Rhs) {
						continue
					}
					if lit, es := v.Rhs[i].(*ast.FuncLit); es && !cierra(fuente(lit.Body)) {
						avisos = append(avisos, rel+": "+fuente(v))
					}
				}
			case *ast.FuncDecl:
				// Una función suelta que sube Escape (`pulsarConTecla` de Calendar) vale
				// por su `TypedKey`.
				if v.Recv == nil && v.Body != nil && sube.MatchString(fuente(v.Body)) {
					ayudantes[filepath.Dir(rel)] = append(ayudantes[filepath.Dir(rel)], v.Name.Name)
				}
				if v.Name.Name != "TypedKey" || v.Recv == nil || v.Body == nil {
					return true
				}
				cuerpo := fuente(v.Body)
				// Los comentarios del cuerpo también cuentan: «Escape es de …» es decir
				// quién lo atiende.
				for _, g := range f.Comments {
					if g.Pos() > v.Body.Pos() && g.End() < v.Body.End() {
						cuerpo += g.Text()
					}
				}
				if !sube.MatchString(cuerpo) {
					dudosos = append(dudosos, dudoso{filepath.Dir(rel), rel + ": " + fuente(v.Recv), cuerpo})
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dudosos {
		if !slices.ContainsFunc(ayudantes[d.dir], func(n string) bool { return strings.Contains(d.cuerpo, n+"(") }) {
			tragones = append(tragones, d.sitio)
		}
	}
	sort.Strings(propias)
	return suite, slices.Compact(propias), avisos, tragones
}

var leidos = map[string][]byte{}

func leer(t *testing.T, p string) []byte {
	if b, es := leidos[p]; es {
		return b
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	leidos[p] = b
	return b
}

var sube = regexp.MustCompile(`Escape|EsCierre|KeyBack|Descartar|\.TypedKey\(`)

func cierra(cuerpo string) bool {
	return regexp.MustCompile(`(?i)cerrar\w*\(|Descartar\(`).MatchString(cuerpo)
}

// TestInventarioDeModales: el recuento sale al registro con -v y las tres reglas, en rojo.
func TestInventarioDeModales(t *testing.T) {
	suite, propias, avisos, tragones := inventario(t)
	total := 0
	claves := make([]string, 0, len(suite))
	for k, n := range suite {
		claves = append(claves, k)
		total += n
	}
	sort.Strings(claves)
	for _, k := range claves {
		t.Logf("%-40s %d", k, suite[k])
	}
	t.Logf("%d aperturas de la suite en %d sitios", total, len(claves))
	if total < 150 {
		t.Errorf("solo salen %d aperturas: el recorrido del módulo no está viendo las apps", total)
	}
	if !slices.Equal(propias, capasPropias) {
		t.Errorf("capas abiertas por fuera de `ui`:\n  hay:   %q\n  lista: %q\nUna nueva no se cierra ni con Escape sin foco ni con «atrás»: ábrela con `ui`.", propias, capasPropias)
	}
	for _, a := range avisos {
		t.Errorf("`AlCerrar` es la PETICIÓN de cierre y este no cierra nada, así que el aspa, Escape y el velo no cierran: %s", a)
	}
	for _, s := range tragones {
		t.Errorf("%s: su `TypedKey` se traga Escape y el «atrás»; con el foco ahí el diálogo de encima no se cierra. Llama a `ui.SubirEscape`.", s)
	}
}

// TestNadiePreguntaAlDispositivo: `fyne.CurrentDevice()` sin app es un puntero nulo
// —`CurrentApp()` devuelve nil y `.Driver()` revienta, así que un `d != nil` detrás no
// salva nada—, y tumbó Sytue Mail al arrancar (#1062). La pregunta «¿teléfono?» es
// `ui.EsTactil`, que se contesta con el sistema.
func TestNadiePreguntaAlDispositivo(t *testing.T) {
	raiz, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	llamada := regexp.MustCompile(`fyne\.CurrentDevice\(\)`)
	err = filepath.WalkDir(raiz, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") || strings.Contains(p, "/upstream/") {
			return err
		}
		for i, l := range strings.Split(string(leer(t, p)), "\n") {
			if llamada.MatchString(l) && !strings.HasPrefix(strings.TrimSpace(l), "//") {
				rel, _ := filepath.Rel(raiz, p)
				t.Errorf("%s:%d llama a `fyne.CurrentDevice()`: usa `ui.EsTactil()`", rel, i+1)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
