// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package catalogos

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// El guardián común de los textos PROPIOS de Sytue Office (PLAN-EDICION-LOCAL §5.14,
// C-12): los que no tienen clave en el catálogo de la web van en un `i18n.Propios`
// (`apps/nucleo/i18n/propios.go`; `local.Propios` es su alias), con el español y el inglés
// en una tabla `map[string][2]string` y los otros veinticuatro idiomas en su mapa de
// traducciones, en el mismo paquete. Antes de él, cada lote de traducciones dejaba alguna
// tabla es/en con su `strings.HasPrefix(idioma, "es")` al lado, y en los otros veinticuatro
// idiomas la pantalla salía en inglés sin que nada avisara.
//
// Recorre por AST el código de la edición: `apps/fyne/cmd-local/**`, `apps/fyne/local/**`,
// `apps/fyne/editor/enfichero`, los `edicionlocal*.go`, `apps/fyne/apps/**` (donde viven las
// tablas propias de ERP y CertiPDF) y `apps/nucleo/local/**`.
//
// VISTO EN ROJO (2026-11-25): con un fichero de prueba en `cmd-local/notes` que tenía una
// tabla `map[string][2]string` sin `Propios`, un `strings.HasPrefix(idioma, "es")`, un
// `idioma == "en"`, un `case "es":` y un `map[string]string{"es": …, "en": …}[idioma]`,
// TestCadaTablaEsEnEsUnPropios nombró la tabla y TestNadieEligeElIdiomaAMano las cuatro
// elecciones, con su fichero y su línea; quitando la fila de `exportarGoogle` de
// `datosQueNoSonTextos`, el primero la nombró como tabla de textos, y con una fila que no
// casaba con nada, TestLasExcepcionesDeDatosSiguenVivas la nombró. Deshecho.

// datosQueNoSonTextos son las tablas `[2]string` y los mapas por idioma del ámbito que son
// DATOS, no textos que se enseñan: no van en un `Propios`. Clave «fichero:nombre» (el
// fichero, relativo a la raíz del repositorio; el nombre, el de la variable o, dentro de
// una función, el de la función). Se compara POR IGUALDAD: una fila que ya no corresponde a
// nada es una excepción huérfana esperando a que alguien la reutilice sin pensar.
var datosQueNoSonTextos = map[string]string{
	"apps/fyne/apps/rooms/comandos.go:fallos":         "pares de CLAVES del catálogo de Rooms (qué se dice si no hay nadie y si falla), que traduce el catálogo",
	"apps/nucleo/local/nube/google.go:exportarGoogle": "tipo MIME de un documento nativo de Google → tipo MIME y extensión del formato abierto al que se exporta",
	"apps/fyne/apps/erp/asistente.go:paisPorIdioma":   "el país que el asistente propone según el idioma de la pantalla (un código de país, no un texto)",
}

// TestCadaTablaEsEnEsUnPropios: cada tabla `map[string][2]string` con textos del ámbito es
// la Base de un `Propios` de su paquete que lleva su mapa de traducciones
// (`map[i18n.Locale]map[string]string`, también de su paquete). Una tabla es/en suelta
// sale en inglés en veinticuatro idiomas.
func TestCadaTablaEsEnEsUnPropios(t *testing.T) {
	a := analizarEdicion(t)
	for _, tb := range a.tablas {
		if _, dato := datosQueNoSonTextos[tb.clave()]; dato {
			continue
		}
		p, ok := a.propiosDe(tb)
		switch {
		case !ok:
			t.Errorf("%s: la tabla %q (español, inglés) no es la Base de ningún i18n.Propios de su paquete:\n"+
				"  en los otros veinticuatro idiomas saldría en inglés. Ponla en un Propios con sus\n"+
				"  traducciones en un *_idiomas.go hermano o, si es un DATO y no un texto, en datosQueNoSonTextos.",
				tb.donde(), tb.nombre)
		case !p.traducida:
			t.Errorf("%s: el Propios de la tabla %q no lleva un mapa de traducciones\n"+
				"  (map[i18n.Locale]map[string]string) de su paquete", tb.donde(), tb.nombre)
		}
	}
}

// TestNadieEligeElIdiomaAMano: fuera de `i18n.Propios`, nadie escoge entre el español y el
// inglés comparando el idioma (`== "es"`, `strings.HasPrefix(idioma, "es")`, `case "en":`)
// ni con un mapa propio por idioma (`map[...]{"es": …, "en": …}`): eso es lo que deja los
// otros veinticuatro idiomas en inglés. `Propios.En` ya resuelve el idioma, su variante
// regional y el respaldo inglés.
func TestNadieEligeElIdiomaAMano(t *testing.T) {
	a := analizarEdicion(t)
	for _, e := range a.elecciones {
		if _, dato := datosQueNoSonTextos[e.clave()]; dato {
			continue
		}
		t.Errorf("%s (%s): %s. Los textos propios van en un i18n.Propios y se piden con .En(idioma, clave)",
			e.donde(), e.nombre, e.que)
	}
}

// TestLasExcepcionesDeDatosSiguenVivas: cada fila de datosQueNoSonTextos corresponde a una
// tabla o un mapa por idioma que sigue ahí.
func TestLasExcepcionesDeDatosSiguenVivas(t *testing.T) {
	a := analizarEdicion(t)
	vistas := map[string]bool{}
	for _, h := range slices.Concat(a.tablas, a.elecciones) {
		vistas[h.clave()] = true
	}
	for _, c := range slices.Sorted(maps.Keys(datosQueNoSonTextos)) {
		if !vistas[c] {
			t.Errorf("datosQueNoSonTextos guarda %q y ya no hay ahí ninguna tabla ni mapa por idioma: bórrala.", c)
		}
	}
}

// ---- Análisis ----

// hallazgo es una tabla es/en o una elección de idioma a mano: dónde y cómo se llama.
type hallazgo struct {
	fichero, nombre, que string
	pos                  token.Position
	lit                  *ast.CompositeLit
}

func (h hallazgo) clave() string { return h.fichero + ":" + h.nombre }

// donde es «fichero:línea», con el fichero relativo a la raíz del repositorio.
func (h hallazgo) donde() string { return fmt.Sprintf("%s:%d", h.fichero, h.pos.Line) }

// usoPropios es un literal `Propios{Base: …, Traducciones: …}`.
type usoPropios struct {
	base      string            // la variable de la Base, si es un identificador
	baseLit   *ast.CompositeLit // la Base escrita en el sitio
	traducida bool              // lleva un mapa de traducciones de su paquete
}

type analisis struct {
	tablas, elecciones []hallazgo
	propios            map[string][]usoPropios // por directorio
	ficheros           int
}

// propiosDe es el Propios que usa la tabla como Base, en su mismo directorio.
func (a *analisis) propiosDe(tb hallazgo) (usoPropios, bool) {
	for _, p := range a.propios[filepath.Dir(tb.fichero)] {
		if p.baseLit == tb.lit || (p.base != "" && p.base == tb.nombre) {
			return p, true
		}
	}
	return usoPropios{}, false
}

// ambitoEdicion son las raíces del código de la edición, relativas a este directorio.
var ambitoEdicion = []string{
	"../cmd-local", "../local", "../editor/enfichero", "../apps", "../../nucleo/local",
}

// elAnalisis es el de la primera prueba que lo pide: las tres miran el mismo árbol.
var elAnalisis struct {
	sync.Once
	a *analisis
}

func analizarEdicion(t *testing.T) *analisis {
	t.Helper()
	elAnalisis.Do(func() { elAnalisis.a = analizar(t) })
	if elAnalisis.a == nil {
		t.FailNow()
	}
	return elAnalisis.a
}

func analizar(t *testing.T) *analisis {
	t.Helper()
	raizRepo, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	vistos := map[string]bool{}
	var rutas []string
	anadir := func(ruta string) {
		abs, _ := filepath.Abs(ruta)
		if !vistos[abs] {
			vistos[abs] = true
			rutas = append(rutas, abs)
		}
	}
	for _, raiz := range ambitoEdicion {
		err := filepath.WalkDir(raiz, func(p string, d fs.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case d.IsDir() && (d.Name() == "testdata" || d.Name() == "build"):
				return fs.SkipDir
			case !d.IsDir() && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go"):
				anadir(p)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("recorriendo %s: %v", raiz, err)
		}
	}
	// Los `edicionlocal*.go` de cualquier sitio de apps/fyne.
	err = filepath.WalkDir("..", func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && (d.Name() == "testdata" || d.Name() == "build" || d.Name() == "upstream"):
			return fs.SkipDir
		case !d.IsDir() && strings.HasPrefix(d.Name(), "edicionlocal") && strings.HasSuffix(p, ".go") &&
			!strings.HasSuffix(p, "_test.go"):
			anadir(p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	a := &analisis{propios: map[string][]usoPropios{}}
	traducciones := map[string]map[string]bool{} // por directorio, las variables de traducciones
	type pendiente struct {
		dir string
		lit *ast.CompositeLit
	}
	var usos []pendiente
	fset := token.NewFileSet()
	for _, abs := range rutas {
		arbol, err := parser.ParseFile(fset, abs, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		a.ficheros++
		rel, _ := filepath.Rel(raizRepo, abs)
		rel = filepath.ToSlash(rel)
		dir := filepath.Dir(rel)
		recorrerConNombre(arbol, func(n ast.Node, nombre string) {
			switch x := n.(type) {
			case *ast.CompositeLit:
				switch {
				case esTablaEsEn(x) && len(x.Elts) > 0:
					a.tablas = append(a.tablas, hallazgo{fichero: rel, nombre: nombre, pos: fset.Position(x.Pos()), lit: x})
				case esMapaDeTraducciones(x) && len(x.Elts) > 0:
					if traducciones[dir] == nil {
						traducciones[dir] = map[string]bool{}
					}
					traducciones[dir][nombre] = true
				case esPropios(x.Type):
					usos = append(usos, pendiente{dir, x})
				case listaDePropios(x.Type):
					// `[]Propios{{Base: …}, …}`: los de dentro no repiten el tipo.
					for _, e := range x.Elts {
						if kv, ok := e.(*ast.KeyValueExpr); ok {
							e = kv.Value
						}
						if c, ok := e.(*ast.CompositeLit); ok && c.Type == nil {
							usos = append(usos, pendiente{dir, c})
						}
					}
				}
				if mapaPorIdioma(x) {
					a.elecciones = append(a.elecciones, hallazgo{
						fichero: rel, nombre: nombre, pos: fset.Position(x.Pos()),
						que: "un mapa con un valor para \"es\" y otro para \"en\" escoge el idioma a mano",
					})
				}
			case *ast.BinaryExpr:
				if (x.Op == token.EQL || x.Op == token.NEQ) && (esIdiomaBase(x.X) || esIdiomaBase(x.Y)) {
					a.elecciones = append(a.elecciones, hallazgo{
						fichero: rel, nombre: nombre, pos: fset.Position(x.Pos()),
						que: "compara el idioma con «es» o «en»",
					})
				}
			case *ast.CallExpr:
				if f, ok := x.Fun.(*ast.SelectorExpr); ok && esIdent(f.X, "strings") &&
					(f.Sel.Name == "HasPrefix" || f.Sel.Name == "EqualFold") &&
					slices.ContainsFunc(x.Args, esIdiomaBase) {
					a.elecciones = append(a.elecciones, hallazgo{
						fichero: rel, nombre: nombre, pos: fset.Position(x.Pos()),
						que: fmt.Sprintf("strings.%s con «es» o «en» escoge el idioma a mano", f.Sel.Name),
					})
				}
			case *ast.CaseClause:
				if slices.ContainsFunc(x.List, esIdiomaBase) {
					a.elecciones = append(a.elecciones, hallazgo{
						fichero: rel, nombre: nombre, pos: fset.Position(x.Pos()),
						que: "un `case \"es\"` / `case \"en\"` escoge el idioma a mano",
					})
				}
			}
		})
	}
	for _, u := range usos {
		var p usoPropios
		for _, e := range u.lit.Elts {
			kv, ok := e.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			switch {
			case esIdent(kv.Key, "Base"):
				switch v := kv.Value.(type) {
				case *ast.Ident:
					p.base = v.Name
				case *ast.CompositeLit:
					p.baseLit = v
				}
			case esIdent(kv.Key, "Traducciones"):
				switch v := kv.Value.(type) {
				case *ast.Ident:
					p.traducida = traducciones[u.dir][v.Name]
				case *ast.CompositeLit:
					p.traducida = esMapaDeTraducciones(v) && len(v.Elts) > 0
				}
			}
		}
		a.propios[u.dir] = append(a.propios[u.dir], p)
	}
	// Que el recorrido mira donde debe: hoy hay una docena larga de Propios.
	n := len(a.propios)
	if a.ficheros < 100 || n < 10 || len(a.tablas) < 15 {
		t.Fatalf("solo se han visto %d ficheros, %d paquetes con Propios y %d tablas es/en: el recorrido mira el árbol equivocado",
			a.ficheros, n, len(a.tablas))
	}
	t.Logf("%d ficheros, %d tablas es/en, Propios en %d paquetes, %d mapas o comparaciones por idioma",
		a.ficheros, len(a.tablas), n, len(a.elecciones))
	return a
}

// recorrerConNombre visita cada nodo con el nombre de lo que lo contiene: la variable a la
// que se asigna (`var x = …`, `x := …`) o, si no, la función.
func recorrerConNombre(f *ast.File, visita func(n ast.Node, nombre string)) {
	var ver func(n ast.Node, nombre string)
	ver = func(n ast.Node, nombre string) {
		ast.Inspect(n, func(m ast.Node) bool {
			switch x := m.(type) {
			case nil:
				return false
			case *ast.FuncDecl:
				if x != n {
					ver(x, x.Name.Name)
					return false
				}
			case *ast.ValueSpec:
				if x != n {
					for i, v := range x.Values {
						ver(v, x.Names[min(i, len(x.Names)-1)].Name)
					}
					return false
				}
			case *ast.AssignStmt:
				if x != n && len(x.Lhs) == len(x.Rhs) {
					for i, v := range x.Rhs {
						if id, ok := x.Lhs[i].(*ast.Ident); ok {
							ver(v, id.Name)
						} else {
							ver(v, nombre)
						}
					}
					return false
				}
			}
			visita(m, nombre)
			return true
		})
	}
	ver(f, "")
}

// esTablaEsEn: `map[string][2]string`.
func esTablaEsEn(x *ast.CompositeLit) bool {
	m, ok := x.Type.(*ast.MapType)
	if !ok || !esIdent(m.Key, "string") {
		return false
	}
	v, ok := m.Value.(*ast.ArrayType)
	if !ok || !esIdent(v.Elt, "string") {
		return false
	}
	l, ok := v.Len.(*ast.BasicLit)
	return ok && l.Value == "2"
}

// esMapaDeTraducciones: `map[i18n.Locale]map[string]string` (o `map[Locale]…` en i18n).
func esMapaDeTraducciones(x *ast.CompositeLit) bool {
	m, ok := x.Type.(*ast.MapType)
	if !ok || !esLocale(m.Key) {
		return false
	}
	v, ok := m.Value.(*ast.MapType)
	return ok && esIdent(v.Key, "string") && esIdent(v.Value, "string")
}

func esLocale(e ast.Expr) bool {
	if s, ok := e.(*ast.SelectorExpr); ok {
		return s.Sel.Name == "Locale"
	}
	return esIdent(e, "Locale")
}

// esPropios: `i18n.Propios`, `local.Propios` o `Propios` (dentro de esos paquetes).
func esPropios(e ast.Expr) bool {
	if s, ok := e.(*ast.SelectorExpr); ok {
		return s.Sel.Name == "Propios" && (esIdent(s.X, "i18n") || esIdent(s.X, "local"))
	}
	return esIdent(e, "Propios")
}

// listaDePropios: `[]Propios`, `[N]Propios` o `map[…]Propios` (de cualquiera de los tres).
func listaDePropios(e ast.Expr) bool {
	switch t := e.(type) {
	case *ast.ArrayType:
		return esPropios(t.Elt)
	case *ast.MapType:
		return esPropios(t.Value)
	}
	return false
}

// esIdiomaBase: un literal «es» o «en», o una variante suya («es-ES», «en-GB»). En
// minúscula, como los idiomas de la suite: «ES» es el código de un país.
func esIdiomaBase(e ast.Expr) bool {
	l, ok := e.(*ast.BasicLit)
	if !ok || l.Kind != token.STRING {
		return false
	}
	s, err := strconv.Unquote(l.Value)
	if err != nil {
		return false
	}
	base, _, _ := strings.Cut(s, "-")
	return base == "es" || base == "en"
}

// mapaPorIdioma: un mapa escrito con una clave «es» y otra «en».
func mapaPorIdioma(x *ast.CompositeLit) bool {
	if _, ok := x.Type.(*ast.MapType); !ok {
		return false
	}
	var es, en bool
	for _, e := range x.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if l, ok := kv.Key.(*ast.BasicLit); ok && l.Kind == token.STRING {
			s, _ := strconv.Unquote(l.Value)
			es, en = es || s == "es", en || s == "en"
		}
	}
	return es && en
}
