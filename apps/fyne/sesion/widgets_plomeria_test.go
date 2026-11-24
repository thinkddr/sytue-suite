// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestNadiePublicaWidgetsDesdeLaVista: el widget nativo de cada app lo publica SOLO su
// Publicador (apps/nucleo/widgets/publicador.go), con una Fuente que no depende de la
// pantalla. Hasta el 2026-11-06 lo publicaban las cargas de cada pantalla con lo que
// tuvieran a la vista, y el widget de Calendar enseñaba el mes que se estuviera
// mirando (ADR 0078, enmienda). Una publicación nueva desde una pantalla pone esto en
// rojo.
func TestNadiePublicaWidgetsDesdeLaVista(t *testing.T) {
	publicar := regexp.MustCompile(`\bPublicarDetalle\(|\.Publicar\(|\bPublishDetail\(`)
	permitidos := []string{"nucleo/widgets/publicador.go", "nucleo/widgets/widgets.go"}
	for _, raiz := range []string{"..", filepath.Join("..", "..", "nucleo")} {
		err := filepath.WalkDir(raiz, func(ruta string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(ruta, ".go") || strings.HasSuffix(ruta, "_test.go") {
				return err
			}
			abs, _ := filepath.Abs(ruta)
			for _, p := range permitidos {
				if strings.HasSuffix(filepath.ToSlash(abs), p) {
					return nil
				}
			}
			fuente, err := os.ReadFile(ruta)
			if err != nil {
				return err
			}
			for n, linea := range strings.Split(string(fuente), "\n") {
				if publicar.MatchString(linea) && !strings.HasPrefix(strings.TrimSpace(linea), "//") {
					t.Errorf("%s:%d publica un widget fuera del Publicador: %s", ruta, n+1, strings.TrimSpace(linea))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestLosCmdUsanLaFuenteRegistrada: cada `cmd` monta su widget con Suite.FuenteWidget,
// que es fuentes.Registrada de su producto: la MISMA Fuente que lee el refresco sin
// ventana de Android, que no corre ningún `main`. Una Fuente montada a mano en un `cmd`
// haría que el widget enseñara una cosa con la app abierta y otra con ella cerrada.
//
// VISTO EN ROJO con `s.PublicadorWidget(fuentes.Mail(...))` en cmd/mail.
func TestLosCmdUsanLaFuenteRegistrada(t *testing.T) {
	mains, err := filepath.Glob(filepath.Join("..", "cmd", "*", "main.go"))
	if err != nil || len(mains) < 15 {
		t.Fatalf("no encuentro los cmd: %v (%d)", err, len(mains))
	}
	propia := regexp.MustCompile(`\bfuentes\.[A-Z][A-Za-z]*\(`)
	montan := 0
	for _, ruta := range mains {
		fuente, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}
		if m := propia.Find(fuente); m != nil {
			t.Errorf("%s construye su Fuente a mano (%s): usa s.FuenteWidget(t)", ruta, m)
		}
		if strings.Contains(string(fuente), "s.PublicadorWidget(s.FuenteWidget(") {
			montan++
		}
	}
	if montan != 15 {
		t.Fatalf("%d cmd montan el widget con FuenteWidget; son quince", montan)
	}
}

// sinAvisoDeEscritura son los `cmd` con widget que no envuelven ningún cliente con
// ClienteConAviso, y por qué. Uno nuevo con widget o lo envuelve o se apunta aquí.
var sinAvisoDeEscritura = map[string]string{
	// Escriben solo en el Drive privado (bóveda o Matrix): nada sale por HTTP.
	"drive":    "repositorio privado",
	"certipdf": "repositorio privado",
	// Su widget lo mueve el vivo del buzón (AvisarWidget), no sus escrituras.
	"mail": "vivo de Mail",
}

// TestLasVistasPorLaAPIAvisanAlWidget: una app de registros con la fuente del
// servidor en `legacy` monta su vista sobre la API (cifrado.AplicacionRegistros) y
// escribe con el cliente del campo `API`. Ese cliente tiene que ser el de
// ClienteConAviso: si no, crear un evento, un contacto o una nota no vuelve a publicar
// el widget, y al salir a la pantalla de inicio no queda nada pendiente que publicar.
// Es el fallo del dueño «siguen sin actualizarse» (2026-11-24): solo Rooms lo pasaba.
//
// VISTO EN ROJO con `api.NewCalendars(s.URL("calendar"), cuenta.HTTP(), nil)` en
// cmd/calendar: «cmd/calendar: el cliente api.NewCalendars de la vista por la API no
// avisa al widget».
func TestLasVistasPorLaAPIAvisanAlWidget(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("..", "cmd", "*"))
	if err != nil {
		t.Fatal(err)
	}
	conVistaAPI := 0
	for _, dir := range dirs {
		app := filepath.Base(dir)
		fuentes, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		var widget, aviso bool
		for _, ruta := range fuentes {
			if strings.HasSuffix(ruta, "_test.go") {
				continue
			}
			texto, err := os.ReadFile(ruta)
			if err != nil {
				t.Fatal(err)
			}
			widget = widget || strings.Contains(string(texto), "s.PublicadorWidget(")
			aviso = aviso || strings.Contains(string(texto), ".ClienteConAviso(")
			f, err := parser.ParseFile(token.NewFileSet(), ruta, texto, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				campo, ok := n.(*ast.KeyValueExpr)
				if !ok {
					return true
				}
				nombre, ok := campo.Key.(*ast.Ident)
				vista, esFuncion := campo.Value.(*ast.FuncLit)
				if !ok || nombre.Name != "API" || !esFuncion {
					return true
				}
				conVistaAPI++
				ast.Inspect(vista.Body, func(n ast.Node) bool {
					if llamada, ok := n.(*ast.CallExpr); ok && clienteDeCuentaSinAviso(llamada) {
						t.Errorf("cmd/%s: el cliente %s de la vista por la API no avisa al widget: pásale s.ClienteConAviso(…)", app, expresion(llamada.Fun))
					}
					return true
				})
				return false
			})
		}
		if _, exento := sinAvisoDeEscritura[app]; widget && !aviso && !exento {
			t.Errorf("cmd/%s publica un widget y ninguna de sus escrituras le avisa (ClienteConAviso)", app)
		}
	}
	if conVistaAPI < 11 {
		t.Fatalf("solo %d vistas por la API; las apps de registros son once", conVistaAPI)
	}
	if !slices.Equal(slices.Sorted(maps.Keys(sinAvisoDeEscritura)), []string{"certipdf", "drive", "mail"}) {
		t.Fatal("la lista de excepciones ha cambiado: revisa por qué esa app no avisa")
	}
}

// clienteDeCuentaSinAviso: `api.NewX(…, cuenta.HTTP(), cliente, …)` cuyo cliente no
// es una llamada a ClienteConAviso.
func clienteDeCuentaSinAviso(llamada *ast.CallExpr) bool {
	sel, ok := llamada.Fun.(*ast.SelectorExpr)
	if !ok || !strings.HasPrefix(sel.Sel.Name, "New") {
		return false
	}
	if paquete, ok := sel.X.(*ast.Ident); !ok || paquete.Name != "api" {
		return false
	}
	for i, arg := range llamada.Args {
		sesion, ok := arg.(*ast.CallExpr)
		if !ok || expresion(sesion.Fun) != "cuenta.HTTP" || i+1 >= len(llamada.Args) {
			continue
		}
		cliente, ok := llamada.Args[i+1].(*ast.CallExpr)
		return !ok || !strings.HasSuffix(expresion(cliente.Fun), ".ClienteConAviso")
	}
	return false
}

func expresion(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return expresion(v.X) + "." + v.Sel.Name
	}
	return "?"
}
