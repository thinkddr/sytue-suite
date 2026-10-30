// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ajustes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEscribirEnRaizNoSaleDelContenedor(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := EscribirEnRaiz(root, "nested/data", []byte("encrypted artificial data")); err != nil {
		t.Fatal(err)
	}
	if err := EscribirEnRaiz(root, "../outside", []byte("escape")); err == nil {
		t.Fatal("write escaped root")
	}
	data, err := os.ReadFile(filepath.Join(dir, "nested", "data"))
	if err != nil || string(data) != "encrypted artificial data" {
		t.Fatal("atomic write failed", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "nested"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "data" {
		t.Fatal("temporary retained")
	}
}
