// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestElAtajoEsElDelSistema: en un Mac, «⌘»; en lo demás, tal cual.
func TestElAtajoEsElDelSistema(t *testing.T) {
	for _, c := range []struct{ goos, atajo, quiero string }{
		{"darwin", "Ctrl+G", "⌘G"},
		{"darwin", "Ctrl K", "⌘K"},
		{"darwin", "Ctrl+↵", "⌘↵"},
		{"darwin", "Ctrl+Y", "⌘⇧Z"},
		{"ios", "Ctrl+B", "⌘B"},
		{"darwin", "Atajos: Ctrl+Z deshacer · Ctrl+S guardar", "Atajos: ⌘Z deshacer · ⌘S guardar"},
		{"linux", "Ctrl+G", "Ctrl+G"},
		{"windows", "Ctrl K", "Ctrl K"},
	} {
		if got := atajoPara(c.goos, c.atajo); got != c.quiero {
			t.Errorf("%s, %q: %q; quiero %q", c.goos, c.atajo, got, c.quiero)
		}
	}
}

// TestLasPistasDeAtajoPasanPorAtajoDelSistema: toda cadena con «Ctrl» o «⌘» en el código
// de las apps va por `AtajoDelSistema` en la MISMA línea. El 2026-09-28, en un Mac, Rooms
// decía «Ctrl+G» junto al «⌘K» del buscador, y el ERP decía «⌘K» también en Windows.
// Se mira el código fuente porque la prueba corre en Linux, donde «Ctrl» es lo correcto.
//
// VISTO EN ROJO con el `"Ctrl+G"` de `apps/rooms/piezas.go` escrito a pelo otra vez.
func TestLasPistasDeAtajoPasanPorAtajoDelSistema(t *testing.T) {
	cadena := regexp.MustCompile(`"[^"]*(Ctrl[+ ]|⌘)[^"]*"`)
	var malos []string
	err := filepath.WalkDir("..", func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(strings.TrimPrefix(ruta, "../"))
		if d.IsDir() {
			if rel == "upstream" || (ruta != ".." && strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") || rel == "ui/tecla.go" {
			return nil
		}
		fuente, err := os.ReadFile(ruta) //nolint:gosec // G304: ficheros del propio módulo
		if err != nil {
			return err
		}
		for n, linea := range strings.Split(string(fuente), "\n") {
			codigo, _, _ := strings.Cut(linea, "//")
			if cadena.MatchString(codigo) && !strings.Contains(codigo, "AtajoDelSistema(") {
				malos = append(malos, rel+":"+strconv.Itoa(n+1)+": "+strings.TrimSpace(codigo))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range malos {
		t.Errorf("pista de atajo sin ui.AtajoDelSistema: %s", m)
	}
}
