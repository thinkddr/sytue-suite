// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos_test

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/contrato"
)

// TestContratoDeLaFototeca (§8.2 del plan de la edición local): el juego de pruebas de
// comportamiento de la fototeca (`photos/contrato`) sobre este repositorio y el almacén
// en claro, abierto como lo abría `cmd-local/photos` (los formatos de Sytue Office y
// soltar los originales al purgar; los bytes se van al compactar). El mismo juego corre
// sobre la fototeca de SQLite (`local/fototeca`).
func TestContratoDeLaFototeca(t *testing.T) {
	contrato.Probar(t, contrato.Puerto{
		Abrir: func(t *testing.T, dir string) (contrato.Fototeca, func()) {
			t.Helper()
			rec, err := almacen.Abrir(context.Background(), dir, "photos", "instalacion-1")
			if err != nil {
				t.Fatal(err)
			}
			repo, err := photos.New(rec.Config)
			if err != nil {
				rec.Cerrar()
				t.Fatal(err)
			}
			repo.AdmitirFormatos("webp", "avif", "bmp")
			repo.LiberarAlPurgar()
			if err := almacen.Preparar(t.Context(), repo); err != nil {
				repo.Close()
				rec.Cerrar()
				t.Fatal("preparar:", err)
			}
			return repo, func() { repo.Close(); rec.Cerrar() }
		},
		Liberar: func(t *testing.T, dir string) {
			t.Helper()
			rec, err := almacen.Abrir(context.Background(), dir, "photos", "instalacion-1")
			if err != nil {
				t.Fatal(err)
			}
			defer rec.Cerrar()
			if _, err := rec.Compactar(t.Context()); err != nil {
				t.Fatal("compactar:", err)
			}
		},
	})
}
