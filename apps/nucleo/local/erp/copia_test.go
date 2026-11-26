// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// La copia en caliente de la base abierta es una base SQLite entera que se abre sola.
func TestCopiarEnCaliente(t *testing.T) {
	_, alm := abrir(t, t.TempDir())
	defer func() { _ = alm.Cerrar() }()
	destino := filepath.Join(t.TempDir(), "erp.sqlite")
	if err := alm.CopiarEnCaliente(context.Background(), destino); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", "file:"+destino+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var version string
	if err := db.QueryRow("SELECT valor FROM meta WHERE clave = 'version'").Scan(&version); err != nil || version == "" {
		t.Fatalf("la copia no trae la base: %q, %v", version, err)
	}
	if filepath.Base(alm.Ruta()) != Fichero {
		t.Fatalf("Ruta: %s", alm.Ruta())
	}
}
