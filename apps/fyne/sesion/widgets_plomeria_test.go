// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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
