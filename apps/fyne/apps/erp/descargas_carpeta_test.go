// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSinDescargasNoVaAlTmpCompartido (FYS-06): sin carpeta de descargas, lo que se
// baja va a una carpeta nueva solo de quien la crea, no al /tmp de todos.
func TestSinDescargasNoVaAlTmpCompartido(t *testing.T) {
	casa := t.TempDir()
	t.Setenv("HOME", casa)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(casa, ".config"))
	t.Setenv("XDG_DOWNLOAD_DIR", "")
	t.Setenv("TMPDIR", t.TempDir())
	d := carpetaDescargas()
	if filepath.Clean(d) == filepath.Clean(os.TempDir()) {
		t.Fatalf("cae en la temporal compartida: %s", d)
	}
	if fi, err := os.Stat(d); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("la carpeta de respaldo: %v %v", fi, err)
	}
}
