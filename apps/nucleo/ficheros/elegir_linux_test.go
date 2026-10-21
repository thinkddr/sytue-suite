// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package ficheros

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestElSelectorFiltraPorLoQueSePide es el `accept` de la web: con un zenity fingido que
// apunta sus argumentos, ElegirConTipos le pasa el filtro con esas extensiones —y solo
// esas— y devuelve la ruta que el selector escribe.
func TestElSelectorFiltraPorLoQueSePide(t *testing.T) {
	dir := t.TempDir()
	apunte := filepath.Join(dir, "apunte")
	guion := "#!/bin/sh\nfor a in \"$@\"; do echo \"$a\" >> " + apunte + "; done\necho /home/ana/datos.xfdf\n"
	if err := os.WriteFile(filepath.Join(dir, "zenity"), []byte(guion), 0o755); err != nil { //nolint:gosec // G306: tiene que poder ejecutarse
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	rutas, err := ElegirConTipos("Importar datos", "xfdf", "xml")
	if err != nil || !slices.Equal(rutas, []string{"/home/ana/datos.xfdf"}) {
		t.Fatalf("ElegirConTipos = %q, %v", rutas, err)
	}
	datos, _ := os.ReadFile(apunte) //nolint:gosec // G304: fichero del propio test
	if !strings.Contains(string(datos), "--file-filter=Importar datos | *.xfdf *.xml\n") {
		t.Errorf("zenity recibió:\n%s", datos)
	}
}

// TestAbrirConEjecutaLaAplicacionElegida: la aplicación que se señaló recibe el fichero
// como argumento; una que ya no está se dice, y no se ejecuta nada.
func TestAbrirConEjecutaLaAplicacionElegida(t *testing.T) {
	dir := t.TempDir()
	apunte := filepath.Join(dir, "apunte")
	app := filepath.Join(dir, "autofirma")
	if err := os.WriteFile(app, []byte("#!/bin/sh\necho \"$1\" > "+apunte+"\n"), 0o755); err != nil { //nolint:gosec // G306: tiene que poder ejecutarse
		t.Fatal(err)
	}
	doc := filepath.Join(dir, "contrato.pdf")
	if err := os.WriteFile(doc, []byte("%PDF-"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AbrirCon(app, doc); err != nil {
		t.Fatal(err)
	}
	for range 100 {
		if datos, _ := os.ReadFile(apunte); strings.TrimSpace(string(datos)) == doc { //nolint:gosec // G304: fichero del propio test
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if datos, _ := os.ReadFile(apunte); strings.TrimSpace(string(datos)) != doc { //nolint:gosec // G304: fichero del propio test
		t.Errorf("la aplicación recibió %q", datos)
	}
	if err := AbrirCon(filepath.Join(dir, "no-esta"), doc); err == nil {
		t.Error("una aplicación que ya no está no da error")
	}
}

// TestElRecorteVaAlPortapapeles: en X11 el PNG entero le llega a xclip por la entrada,
// con el tipo de imagen; sin xclip ni wl-copy, ErrNoDisponible (y la app lo descarga).
func TestElRecorteVaAlPortapapeles(t *testing.T) {
	dir := t.TempDir()
	apunte := filepath.Join(dir, "apunte")
	guion := "#!/bin/sh\necho \"$*\" > " + apunte + "\nwhile IFS= read -r l; do echo \"$l\" >> " + apunte + "; done\n"
	if err := os.WriteFile(filepath.Join(dir, "xclip"), []byte(guion), 0o755); err != nil { //nolint:gosec // G306: tiene que poder ejecutarse
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "")
	if err := CopiarImagen([]byte("PNG\n")); err != nil {
		t.Fatal(err)
	}
	datos, _ := os.ReadFile(apunte) //nolint:gosec // G304: fichero del propio test
	if string(datos) != "-selection clipboard -t image/png -i\nPNG\n" {
		t.Errorf("xclip recibió %q", datos)
	}
	t.Setenv("PATH", t.TempDir())
	if err := CopiarImagen([]byte("PNG")); !errors.Is(err, ErrNoDisponible) {
		t.Errorf("sin xclip ni wl-copy CopiarImagen da %v", err)
	}
}
