//go:build !linux && !android

// SPDX-License-Identifier: Apache-2.0
package ajustes

import (
	"errors"
	"io/fs"
	"os"
	"testing"
)

func TestRaizDurableNoSoportadaNoMuta(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := AbrirRaizDurable(root, "nuevo"); !errors.Is(err, ErrPersistenciaNoSoportada) {
		t.Fatal(err)
	}
	if _, err := root.Stat("nuevo"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}
