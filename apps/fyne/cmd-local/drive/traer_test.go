// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"path/filepath"
	"testing"
)

// Lo que ya está en Drive se enseña en su carpeta; lo de fuera se sube.
func TestUnFicheroDeDentroDeDriveNoSeSube(t *testing.T) {
	raiz := filepath.Join(t.TempDir(), "Drive")
	for ruta, quiero := range map[string]string{
		filepath.Join(raiz, "Proyectos", "plan.zip"): "/Proyectos/plan.zip",
		filepath.Join(raiz, "nota.bin"):              "/nota.bin",
	} {
		if got, ok := dentroDeDrive(raiz, filepath.ToSlash(ruta)); !ok || got != quiero {
			t.Errorf("%s = %q %v; quiero %q", ruta, got, ok, quiero)
		}
	}
	for _, ruta := range []string{raiz, filepath.Join(filepath.Dir(raiz), "fuera.zip"), filepath.Join(raiz+"2", "x.zip")} {
		if got, ok := dentroDeDrive(raiz, filepath.ToSlash(ruta)); ok {
			t.Errorf("%s se toma por de Drive: %q", ruta, got)
		}
	}
}
