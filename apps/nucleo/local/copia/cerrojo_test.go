// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package copia

import (
	"errors"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// Mientras la copia tiene los almacenes, nadie los compacta; al acabar, sí.
func TestLaCopiaNoDejaCompactarMientrasCopia(t *testing.T) {
	datos := t.TempDir()
	rec, err := almacen.Abrir(t.Context(), filepath.Join(datos, "Datos", "notes"), "notes", "persona")
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Cerrar()
	soltar, err := cerrarAlmacenes(datos)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Compactar(t.Context()); !errors.Is(err, almacen.ErrEnUso) {
		soltar()
		t.Fatalf("compactó con la copia en marcha: %v", err)
	}
	soltar()
	if _, err := rec.Compactar(t.Context()); err != nil {
		t.Fatalf("sin la copia, compacta: %v", err)
	}
}
