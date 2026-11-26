// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package almacen_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// arbol es la lista de ficheros de dir con su tamaño: lo que Leer no puede cambiar.
func arbol(t *testing.T, dir string) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	_ = filepath.WalkDir(dir, func(ruta string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			info, _ := d.Info()
			out[ruta] = info.Size()
		}
		return err
	})
	return out
}

// TestLeerVeLaUltimaRevisionDeLoVivoSinEscribir: con el repositorio de notas ABIERTO
// (otra app escribiendo a la vez), Leer da cada entidad viva en su última revisión, sin
// la borrada, y no toca la carpeta.
func TestLeerVeLaUltimaRevisionDeLoVivoSinEscribir(t *testing.T) {
	dir := t.TempDir()
	repo, rec := abrirNotas(t, dir)
	defer rec.Cerrar()
	defer repo.Close()
	ctx := t.Context()
	libro, err := repo.CreateNotebook(ctx, api.NotebookInput{Title: "Ideas"})
	if err != nil {
		t.Fatal(err)
	}
	nota, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Title: "Lista", Content: "<p>comprar pan</p>", Kind: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateNote(ctx, nota.ID, libro.ID, api.NoteInput{Title: "Lista de la compra", Content: "<p>comprar pan</p>", Kind: "text"}); err != nil {
		t.Fatal(err)
	}
	borrada, err := repo.CreateNote(ctx, libro.ID, api.NoteInput{Title: "Tirar", Kind: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteNote(ctx, borrada.ID); err != nil {
		t.Fatal(err)
	}

	antes := arbol(t, dir)
	filas, err := almacen.Leer(dir)
	if err != nil {
		t.Fatal(err)
	}
	if despues := arbol(t, dir); len(despues) != len(antes) {
		t.Fatalf("Leer ha cambiado la carpeta: %d ficheros antes, %d después", len(antes), len(despues))
	}
	var titulos []string
	for _, f := range filas {
		if f.Entidad != "note" {
			continue
		}
		var n struct{ Title string }
		if err := json.Unmarshal(f.JSON, &n); err != nil {
			t.Fatal(err)
		}
		if f.Tipo != "org.sytue.notes.entity.v1" || f.ID == "" || f.Fecha.IsZero() {
			t.Errorf("fila incompleta: %+v", f)
		}
		titulos = append(titulos, n.Title)
	}
	if !slices.Equal(titulos, []string{"Lista de la compra"}) {
		t.Fatalf("notas leídas = %q, quiero solo la última revisión de la viva", titulos)
	}
	if !slices.ContainsFunc(filas, func(f almacen.Fila) bool { return f.Entidad == "notebook" }) {
		t.Error("falta el cuaderno")
	}
}

// TestLeerAguantaUnaLineaAMediasYUnaCarpetaVacia: la app dueña puede estar a mitad de
// escribir una línea; y una app que nunca se abrió no tiene almacén, ni se le crea.
func TestLeerAguantaUnaLineaAMediasYUnaCarpetaVacia(t *testing.T) {
	dir := t.TempDir()
	repo, rec := abrirNotas(t, dir)
	if _, err := repo.CreateNotebook(t.Context(), api.NotebookInput{Title: "A"}); err != nil {
		t.Fatal(err)
	}
	repo.Close()
	rec.Cerrar()
	f, err := os.OpenFile(filepath.Join(dir, "registros.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"t":"registro","ref":{"room_id":"!x`)
	f.Close()
	if filas, err := almacen.Leer(dir); err != nil || len(filas) != 1 {
		t.Fatalf("con una línea a medias: %d filas, %v", len(filas), err)
	}

	nada := filepath.Join(t.TempDir(), "notes")
	if filas, err := almacen.Leer(nada); err != nil || filas != nil {
		t.Fatalf("sin almacén: %v, %v", filas, err)
	}
	if _, err := os.Stat(nada); !os.IsNotExist(err) {
		t.Fatal("Leer ha creado la carpeta de una app que nunca se abrió")
	}
}
