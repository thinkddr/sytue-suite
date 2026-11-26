// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// puertosDeNotas son los dos puertos sobre los que corre el repositorio de Notepads: el
// de las pruebas en memoria de la edición conectada (el publicador privado de mentira,
// tras importar la instantánea) y el almacén en claro de Sytue Office en disco
// (`local/almacen`, con NewLocal, como lo abre `cmd-local/notes`).
var puertosDeNotas = map[string]func(t *testing.T) *Repository{
	"en memoria": func(t *testing.T) *Repository {
		r, _, _ := notesFixture(t)
		if err := r.ImportLegacySnapshot(t.Context()); err != nil {
			t.Fatal(err)
		}
		return r
	},
	"en disco": func(t *testing.T) *Repository {
		rec, err := almacen.Abrir(context.Background(), t.TempDir(), "notes", "instalacion-1")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(rec.Cerrar)
		r, err := NewLocal(rec.Config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(r.Close)
		if err := almacen.Preparar(t.Context(), r); err != nil {
			t.Fatal(err)
		}
		return r
	},
}

// TestContratoDeNotas (§8.2 del plan de la edición local): el MISMO juego de pruebas del
// repositorio de dominio de Notepads sobre los dos puertos. Es lo que hace segura la
// decisión D-06 —Sytue Office usa el repositorio de la conectada sin tocarlo—: crear,
// normalizar, editar, mover, archivar, mandar a la papelera, restaurar y borrar del todo
// se comportan igual en memoria que en el almacén en claro.
//
// Los de los demás: Photos en `photos/contrato`, Contacts en
// `local/libreta/contrato_test.go`, y Calendar y Bases en su `contrato_test.go`, sobre
// el puerto en memoria del motor (`storage/storagetest.Memoria`) y el almacén en disco.
//
// VISTO EN ROJO (2026-11-25): esperando otro título tras editar, y sin borrar del todo,
// falló en los dos puertos con el mismo mensaje.
func TestContratoDeNotas(t *testing.T) {
	for nombre, abrir := range puertosDeNotas {
		t.Run(nombre, func(t *testing.T) { contratoDeNotas(t, abrir(t)) })
	}
}

func contratoDeNotas(t *testing.T, r *Repository) {
	ctx := t.Context()
	// Cuadernos: se normalizan en el dispositivo igual en los dos.
	libro, err := r.CreateNotebook(ctx, api.NotebookInput{Title: "  Ideas  ", Visibility: "personal"})
	if err != nil || libro.Title != "Ideas" || libro.Layout != "tree" || libro.Color == "" {
		t.Fatalf("crear cuaderno: %+v %v", libro, err)
	}
	otro, err := r.CreateNotebook(ctx, api.NotebookInput{Title: "Archivo"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.CreateNotebook(ctx, api.NotebookInput{Title: "mal", Color: "no-es-un-color"}); err == nil {
		t.Error("un color que no es un color entra")
	}
	// Notas: el contenido se limpia y los valores por defecto son los mismos.
	nota, err := r.CreateNote(ctx, libro.ID, api.NoteInput{Title: "  Lista  ", Content: `<p onclick="robar()">pan<script>robar()</script></p>`})
	if err != nil || nota.Title != "Lista" || nota.Kind != "text" || nota.Width != 260 || strings.Contains(nota.Content, "robar") || !strings.Contains(nota.Content, "pan") {
		t.Fatalf("crear nota: %+v %v", nota, err)
	}
	if _, err := r.CreateNote(ctx, libro.ID, api.NoteInput{Kind: "desconocida"}); err == nil {
		t.Error("una clase de nota desconocida entra")
	}
	ancho := 1
	if _, err := r.CreateNote(ctx, libro.ID, api.NoteInput{Width: &ancho}); err == nil {
		t.Error("un ancho imposible entra")
	}
	// Editar.
	in := nota.Input()
	in.Title, in.Content = "Lista de la compra", "<p>pan y aceite</p>"
	editada, err := r.UpdateNote(ctx, nota.ID, libro.ID, in)
	if err != nil || editada.Title != "Lista de la compra" {
		t.Fatalf("editar: %+v %v", editada, err)
	}
	ns, err := r.ListNotes(ctx, libro.ID)
	if err != nil || len(ns) != 1 || ns[0].ID != nota.ID || ns[0].Title != "Lista de la compra" || !strings.Contains(ns[0].Content, "pan y aceite") {
		t.Fatalf("listar tras editar: %+v %v", ns, err)
	}
	// Mover a otro cuaderno.
	if err := r.MoveNoteToNotebook(ctx, nota.ID, otro.ID); err != nil {
		t.Fatalf("mover: %v", err)
	}
	if ns, _ := r.ListNotes(ctx, libro.ID); len(ns) != 0 {
		t.Errorf("tras mover sigue en el cuaderno de origen: %+v", ns)
	}
	if ns, _ := r.ListNotes(ctx, otro.ID); len(ns) != 1 || ns[0].ID != nota.ID {
		t.Errorf("tras mover no está en el de destino: %+v", ns)
	}
	// Archivar el cuaderno: deja de salir en la lista.
	if err := r.ArchiveNotebook(ctx, libro.ID, true); err != nil {
		t.Fatalf("archivar: %v", err)
	}
	libros, err := r.ListNotebooks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(libros, func(n api.Notebook) bool { return n.ID == libro.ID }); i >= 0 && libros[i].ArchivedAt == nil {
		t.Errorf("el cuaderno archivado sale sin archivar: %+v", libros[i])
	}
	// Papelera, restaurar y borrar del todo.
	if err := r.DeleteNote(ctx, nota.ID); err != nil {
		t.Fatalf("a la papelera: %v", err)
	}
	if ns, _ := r.ListNotes(ctx, otro.ID); len(ns) != 0 {
		t.Errorf("en la papelera sigue en su cuaderno: %+v", ns)
	}
	papelera, err := r.ListTrash(ctx, 50)
	if err != nil || !slices.ContainsFunc(papelera, func(n api.TrashedNote) bool { return n.ID == nota.ID }) {
		t.Fatalf("la papelera no la tiene: %+v %v", papelera, err)
	}
	if err := r.RestoreNote(ctx, nota.ID); err != nil {
		t.Fatalf("restaurar: %v", err)
	}
	if ns, _ := r.ListNotes(ctx, otro.ID); len(ns) != 1 {
		t.Errorf("restaurada no vuelve a su cuaderno: %+v", ns)
	}
	if err := r.DeleteNote(ctx, nota.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.PurgeNote(ctx, nota.ID); err != nil {
		t.Fatalf("borrar del todo: %v", err)
	}
	if papelera, _ := r.ListTrash(ctx, 50); slices.ContainsFunc(papelera, func(n api.TrashedNote) bool { return n.ID == nota.ID }) {
		t.Error("borrada del todo sigue en la papelera")
	}
}
