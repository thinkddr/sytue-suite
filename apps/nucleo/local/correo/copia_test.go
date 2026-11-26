// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// La copia en caliente del índice abierto es una base SQLite entera que se abre sola.
func TestCopiarIndice(t *testing.T) {
	b, err := Abrir(t.TempDir(), &secretosEnMemoria{m: map[string]string{}}, Red{Marcar: marcarDirecto})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Cerrar()
	destino := filepath.Join(t.TempDir(), FicheroIndice)
	if err := b.CopiarIndice(context.Background(), destino); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", "file:"+destino+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'table'").Scan(&n); err != nil || n == 0 {
		t.Fatalf("la copia no trae el índice: %d tablas, %v", n, err)
	}
	if filepath.Base(b.RutaIndice()) != FicheroIndice {
		t.Fatalf("RutaIndice: %s", b.RutaIndice())
	}
}
