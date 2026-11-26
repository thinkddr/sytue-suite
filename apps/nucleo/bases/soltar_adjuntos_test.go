// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/bases"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// objetos cuenta los ficheros de objetos/ del almacén.
func objetos(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(filepath.Join(dir, "objetos"), func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// Con SoltarAdjuntos, borrar un adjunto suelta su fichero y la compactación lo quita del
// disco; sin él (la conectada), la ficha en la papelera lo sigue guardando.
func TestBorrarUnAdjuntoSueltaSuFichero(t *testing.T) {
	for _, soltar := range []bool{false, true} {
		dir := t.TempDir()
		ctx := context.Background()
		rec, err := almacen.Abrir(ctx, dir, "bases", "instalacion-1")
		if err != nil {
			t.Fatal(err)
		}
		repo, err := bases.New(rec.Config)
		if err != nil {
			t.Fatal(err)
		}
		repo.SoltarAdjuntos = soltar
		if err := almacen.Preparar(ctx, repo); err != nil {
			t.Fatal(err)
		}
		b, err := api.NewBases("https://sin-red.invalid", nil, nil).WithPrivateData(repo)
		if err != nil {
			t.Fatal(err)
		}
		base, _ := b.CreateBase(ctx, "Fichas")
		tabla, _ := b.CreateTable(ctx, base.ID, "Cosas")
		campo, _ := b.CreateField(ctx, tabla.ID, api.BasesNewField{Name: "Ficheros", Kind: "attachment"})
		fila, _ := b.CreateRecord(ctx, tabla.ID, map[string]any{}, nil)
		adj, err := b.UploadAttachment(ctx, fila.ID, campo.ID, "nota.txt", []byte("un adjunto de prueba"))
		if err != nil {
			t.Fatal(err)
		}
		antes := objetos(t, dir)
		if err := b.DeleteAttachment(ctx, fila.ID, adj.ID); err != nil {
			t.Fatal(err)
		}
		repo.Close()
		if _, err := rec.Compactar(ctx); err != nil {
			t.Fatal(err)
		}
		despues := objetos(t, dir)
		rec.Cerrar()
		if soltar && despues >= antes {
			t.Fatalf("soltando, el fichero sigue en el disco: %d → %d objetos", antes, despues)
		}
		if !soltar && despues != antes {
			t.Fatalf("sin soltar, el adjunto en la papelera perdió su fichero: %d → %d objetos", antes, despues)
		}
	}
}
