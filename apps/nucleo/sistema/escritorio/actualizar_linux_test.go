// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package escritorio

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestElBundleDeLinuxReemplazaAlEjecutable: del .tar.xz del bundle sale SU ejecutable
// (`sytue-one/bin/`) y queda en lugar del instalado, ejecutable, y se abre la app nueva;
// vale también el paquete suelto de Fyne de antes (`sytue-hub/usr/local/bin/`). Uno que
// no lo trae no toca nada. VISTO EN ROJO buscando el ejecutable en `usr/local/bin` desde
// la raíz, que es donde no lo pone ninguno de los dos.
func TestElBundleDeLinuxReemplazaAlEjecutable(t *testing.T) {
	if _, err := exec.LookPath("xz"); err != nil {
		t.Skip("sin xz no se puede hacer el paquete de prueba")
	}
	raiz := t.TempDir()
	// paquete hace un .tar.xz con `ficheros` (ruta dentro → contenido).
	paquete := func(nombre string, ficheros map[string]string) string {
		t.Helper()
		dir := filepath.Join(raiz, nombre+".d")
		for ruta, contenido := range ficheros {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, ruta)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ruta), []byte(contenido), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		txz := filepath.Join(raiz, nombre)
		if salida, err := exec.Command("tar", "-cJf", txz, "-C", dir, ".").CombinedOutput(); err != nil {
			t.Fatalf("tar: %v %s", err, salida)
		}
		return txz
	}
	bundle := paquete("sytue-one-linux-amd64.tar.xz", map[string]string{
		"sytue-one/bin/sytue-hub": "hub nuevo", "sytue-one/bin/sytue-mail": "mail nuevo",
		"sytue-one/instalar.sh": "#!/bin/sh",
	})
	suelto := paquete("sytue-notes-linux-amd64.tar.xz", map[string]string{"sytue-notes/usr/local/bin/sytue-notes": "notes nuevo"})

	bin := filepath.Join(raiz, "bin")
	_ = os.MkdirAll(bin, 0o755)
	instalada := func(app string) string {
		exe := filepath.Join(bin, "sytue-"+app)
		if err := os.WriteFile(exe, []byte("viejo"), 0o755); err != nil {
			t.Fatal(err)
		}
		return exe
	}
	for _, c := range []struct{ txz, app, quiero string }{
		{bundle, "hub", "hub nuevo"}, {bundle, "mail", "mail nuevo"}, {suelto, "notes", "notes nuevo"},
	} {
		exe := instalada(c.app)
		var abierto string
		if err := instalarEn(context.Background(), c.txz, exe, func(e string) { abierto = e }); err != nil {
			t.Fatalf("%s: %v", c.app, err)
		}
		if b, _ := os.ReadFile(exe); string(b) != c.quiero {
			t.Errorf("%s sigue siendo %q", c.app, b)
		}
		if info, _ := os.Stat(exe); info.Mode().Perm()&0o100 == 0 {
			t.Errorf("%s nuevo no se puede ejecutar: %v", c.app, info.Mode())
		}
		if abierto != exe {
			t.Errorf("se ha abierto %q", abierto)
		}
	}
	if restos, _ := filepath.Glob(filepath.Join(bin, ".sytue-actualizar-*")); len(restos) != 0 {
		t.Errorf("quedan restos: %v", restos)
	}

	otro := instalada("drive")
	if err := instalarEn(context.Background(), bundle, otro, nil); err == nil {
		t.Error("un bundle que no la lleva no puede instalarse encima de esta")
	}
	if b, _ := os.ReadFile(otro); string(b) != "viejo" {
		t.Errorf("un paquete que no vale ha tocado la app: %q", b)
	}
}

// TestLasHermanasDeLinuxSonLosEjecutablesDeLaSuite: en la carpeta del Hub, los
// `sytue-<app>` que se pueden ejecutar; ni él mismo ni un fichero cualquiera. VISTO EN
// ROJO sin mirar el permiso de ejecución.
func TestLasHermanasDeLinuxSonLosEjecutablesDeLaSuite(t *testing.T) {
	dir := t.TempDir()
	for nombre, permiso := range map[string]os.FileMode{
		"sytue-hub": 0o755, "sytue-mail": 0o755, "sytue-drive": 0o644, "sytue-notas.txt": 0o755, "otra": 0o755,
	} {
		if err := os.WriteFile(filepath.Join(dir, nombre), []byte("x"), permiso); err != nil {
			t.Fatal(err)
		}
	}
	got := hermanasDe(filepath.Join(dir, "sytue-hub"))
	if len(got) != 1 || got[0].App != "mail" || got[0].Destino != filepath.Join(dir, "sytue-mail") {
		t.Fatalf("hermanas = %+v", got)
	}
}
