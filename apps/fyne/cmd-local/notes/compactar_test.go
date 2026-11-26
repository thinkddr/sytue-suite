// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	privateNotes "git.sytue.eu/sytue/one/apps/nucleo/notes"
)

// TestBorrarDelTodoCompactaEnElActo: «Eliminar para siempre» por la fachada de la
// pantalla (`api.Notes` con el repositorio local envuelto) quita la nota del diario del
// almacén sin cerrar la app, y lo demás se sigue leyendo y escribiendo.
func TestBorrarDelTodoCompactaEnElActo(t *testing.T) {
	antes := almacen.EsperaCompactarYa
	almacen.EsperaCompactarYa = 20 * time.Millisecond
	t.Cleanup(func() { almacen.EsperaCompactarYa = antes })

	dir := t.TempDir()
	ctx := t.Context()
	rec, err := almacen.Abrir(context.Background(), dir, "notes", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := privateNotes.NewLocal(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := almacen.Preparar(ctx, repo); err != nil {
		t.Fatal(err)
	}
	defer rec.Cerrar()
	defer repo.Close()
	defer rec.PararCompactar()
	svc, err := api.NewNotes("", nil, nil).WithPrivateData(notasLocales{repo, rec})
	if err != nil {
		t.Fatal(err)
	}
	libro, err := svc.CreateNotebook(ctx, api.NotebookInput{Title: "Casa"})
	if err != nil {
		t.Fatal(err)
	}
	viva, err := svc.CreateNote(ctx, libro.ID, api.NoteInput{Kind: "text", Content: "<p>se queda</p>"})
	if err != nil {
		t.Fatal(err)
	}
	borrada, err := svc.CreateNote(ctx, libro.ID, api.NoteInput{Kind: "text", Content: "<p>clave del wifi</p>"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteNote(ctx, borrada.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.PurgeNote(ctx, borrada.ID); err != nil {
		t.Fatal(err)
	}
	limite := time.Now().Add(10 * time.Second)
	for {
		raw, _ := os.ReadFile(filepath.Join(dir, "registros.jsonl"))
		if !strings.Contains(string(raw), "clave del wifi") {
			break
		}
		if time.Now().After(limite) {
			t.Fatal("lo borrado del todo sigue en el disco con la app abierta")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := svc.UpdateNote(ctx, viva.ID, libro.ID, api.NoteInput{Kind: "text", Content: "<p>se queda y cambia</p>"}); err != nil {
		t.Fatal("escribir tras compactar:", err)
	}
	ns, err := svc.ListNotes(ctx, libro.ID)
	if err != nil || len(ns) != 1 || ns[0].Content != "<p>se queda y cambia</p>" {
		t.Fatal(ns, err)
	}
}
