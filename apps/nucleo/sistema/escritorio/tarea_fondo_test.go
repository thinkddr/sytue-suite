// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// El ejecutable de una hermana: el suelto de Linux y Windows, el de dentro del .app de
// macOS, y nada si no se puede ejecutar o no está.
func TestEjecutableDeHermana(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("los permisos de ejecución son de POSIX")
	}
	dir := t.TempDir()
	suelto := filepath.Join(dir, "sytue-photos")
	dentro := filepath.Join(dir, "Sytue Photos.app", "Contents", "MacOS", "sytue-photos")
	sinPermiso := filepath.Join(dir, "sytue-mail")
	for ruta, modo := range map[string]os.FileMode{suelto: 0o755, dentro: 0o755, sinPermiso: 0o644} {
		if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ruta, []byte("#!/bin/sh\n"), modo); err != nil {
			t.Fatal(err)
		}
	}
	casos := map[string]string{
		suelto:                                 suelto,
		filepath.Join(dir, "Sytue Photos.app"): dentro,
		sinPermiso:                             "",
		filepath.Join(dir, "no-está"):          "",
		filepath.Join(dir, "Otra.app"):         "",
	}
	for destino, quiere := range casos {
		if got := ejecutableDe(destino, "sytue-photos"); got != quiere {
			t.Errorf("%s: %q, y quiero %q", destino, got, quiere)
		}
	}
}

// Lanzar sin ejecutable no lanza nada, y un código distinto de cero es un error.
func TestLanzarTareaEnFondo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("el doble es un guion de sh")
	}
	if err := LanzarTareaEnFondo(context.Background(), ""); !errors.Is(err, ErrNoDisponible) {
		t.Errorf("sin ejecutable: %v", err)
	}
	dir := t.TempDir()
	marca := filepath.Join(dir, "argumento")
	bien := filepath.Join(dir, "bien")
	if err := os.WriteFile(bien, []byte("#!/bin/sh\necho \"$1\" > '"+marca+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := LanzarTareaEnFondo(context.Background(), bien); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(marca); string(b) != ArgTareaEnFondo+"\n" {
		t.Errorf("la app recibió %q", b)
	}
	mal := filepath.Join(dir, "mal")
	if err := os.WriteFile(mal, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := LanzarTareaEnFondo(context.Background(), mal); err == nil {
		t.Error("una tarea que falla no da error")
	}
}
