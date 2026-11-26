// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package ficheros

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGuardarComoPideNombreYTipo: con un zenity fingido, GuardarComo le pasa el modo de
// guardar, el nombre propuesto con su extensión y el filtro, y devuelve la ruta que el
// selector escribe con la extensión puesta si la persona la quitó.
func TestGuardarComoPideNombreYTipo(t *testing.T) {
	dir := t.TempDir()
	apunte := filepath.Join(dir, "apunte")
	guion := "#!/bin/sh\nfor a in \"$@\"; do echo \"$a\" >> " + apunte + "; done\necho /home/ana/Informe\n"
	if err := os.WriteFile(filepath.Join(dir, "zenity"), []byte(guion), 0o755); err != nil { //nolint:gosec // G306: tiene que poder ejecutarse
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	ruta, err := GuardarComo("Guardar como", "Informe / Q3", "docus", "docx")
	if err != nil || ruta != "/home/ana/Informe.docus" {
		t.Fatalf("GuardarComo = %q, %v", ruta, err)
	}
	datos, _ := os.ReadFile(apunte) //nolint:gosec // G304: fichero del propio test
	for _, quiero := range []string{"--save\n", "--filename=Informe - Q3.docus\n", "--file-filter=Guardar como (*.docx) | *.docx\n"} {
		if !strings.Contains(string(datos), quiero) {
			t.Errorf("zenity no recibió %q:\n%s", quiero, datos)
		}
	}
}

// TestGuardarComoCancelar: un selector que sale con código distinto de cero es cancelar.
func TestGuardarComoCancelar(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "zenity"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil { //nolint:gosec // G306: tiene que poder ejecutarse
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if ruta, err := GuardarComo("Guardar", "x", "calcus"); ruta != "" || err != nil {
		t.Fatalf("cancelar = %q, %v", ruta, err)
	}
	if ruta, err := GuardarComo("Guardar", "x.CALCUS", "calcus"); ruta != "" || err != nil {
		t.Fatalf("cancelar = %q, %v", ruta, err)
	}
	if got := conExtension("/a/b.CALCUS", []string{"calcus"}); got != "/a/b.CALCUS" {
		t.Errorf("conExtension = %q", got)
	}
}

// TestGuardarComoWindowsVaEnBase64: ni el título, ni el nombre, ni el filtro van como
// literal de PowerShell (FYS-12).
func TestGuardarComoWindowsVaEnBase64(t *testing.T) {
	g := guionGuardarWindows("L’app", "Informe’s", []string{"docus"})
	if strings.Contains(g, "’") || !strings.Contains(g, "SaveFileDialog") || !strings.Contains(g, "OverwritePrompt = $true") {
		t.Errorf("guion: %s", g)
	}
}
