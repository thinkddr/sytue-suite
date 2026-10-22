// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// Archivar saca la rama del árbol y de la búsqueda (también de la que federa el
// Hub, que no pide `archivados`), y desarchivar la devuelve ÍNTEGRA: con sus
// notas, y sin arrastrar a un hijo que ya estaba archivado por su cuenta.
func TestArchivoSacaDeLaBusquedaYVuelveIntegro(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	padre, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Proyectos", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	hijoIn := entradaCuaderno("Viejo", domain.VisPersonal)
	hijoIn.ParentID = &padre.ID
	hijo, err := st.CreateNotebook(ctx, "ada", hijoIn)
	if err != nil {
		t.Fatal(err)
	}
	hermanoIn := entradaCuaderno("Activo", domain.VisPersonal)
	hermanoIn.ParentID = &padre.ID
	hermano, err := st.CreateNotebook(ctx, "ada", hermanoIn)
	if err != nil {
		t.Fatal(err)
	}
	nota := domain.NoteInput{Kind: domain.KindText, Title: "berilio", Content: "x", Pinned: siBool(true)}
	_ = nota.Normalize(false)
	if _, err := st.CreateNote(ctx, hermano.ID, "ada", nota); err != nil {
		t.Fatal(err)
	}

	cuenta := func(archivados bool) int {
		t.Helper()
		hits, err := st.SearchNotes(ctx, "ada", "berilio", 10, archivados)
		if err != nil {
			t.Fatal(err)
		}
		return len(hits)
	}
	recientes := func() int {
		t.Helper()
		hits, err := st.RecentNotes(ctx, "ada", 10, false)
		if err != nil {
			t.Fatal(err)
		}
		return len(hits)
	}
	fijadas := func() int {
		t.Helper()
		hits, err := st.PinnedNotes(ctx, "ada", 10)
		if err != nil {
			t.Fatal(err)
		}
		return len(hits)
	}
	if cuenta(false) != 1 || fijadas() != 1 {
		t.Fatal("antes de archivar la nota tiene que salir en la búsqueda y en fijadas")
	}

	// El hijo se archiva solo, primero; luego toda la rama.
	if err := st.ArchiveNotebook(ctx, hijo.ID, "ada", true); err != nil {
		t.Fatal(err)
	}
	if err := st.ArchiveNotebook(ctx, padre.ID, "bob", true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra persona archiva un cuaderno ajeno: %v", err)
	}
	if err := st.ArchiveNotebook(ctx, padre.ID, "ada", true); err != nil {
		t.Fatal(err)
	}
	if cuenta(false) != 0 || recientes() != 0 || fijadas() != 0 {
		t.Fatal("lo archivado sigue saliendo en la búsqueda, en recientes o en fijadas")
	}
	if cuenta(true) != 1 {
		t.Fatal("con el filtro, la búsqueda tiene que incluir lo archivado")
	}

	archivado := func() map[uuid.UUID]bool {
		t.Helper()
		nbs, err := st.ListNotebooks(ctx, "ada")
		if err != nil {
			t.Fatal(err)
		}
		m := map[uuid.UUID]bool{}
		for _, nb := range nbs {
			m[nb.ID] = nb.ArchivedAt != nil
		}
		return m
	}
	if a := archivado(); !a[padre.ID] || !a[hijo.ID] || !a[hermano.ID] {
		t.Fatalf("archivar no marca la rama entera: %v", a)
	}

	// Con el padre archivado, ningún hijo vuelve suelto: ni el que cayó con la rama
	// ni el que ya estaba archivado antes. Quedaría huérfano fuera del árbol.
	for _, h := range []uuid.UUID{hermano.ID, hijo.ID} {
		if err := st.ArchiveNotebook(ctx, h, "ada", false); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("se desarchiva un hijo con el padre archivado: %v", err)
		}
	}
	if a := archivado(); !a[hermano.ID] || !a[hijo.ID] {
		t.Fatalf("el intento fallido cambió la rama: %v", a)
	}

	if err := st.ArchiveNotebook(ctx, padre.ID, "ada", false); err != nil {
		t.Fatal(err)
	}
	a := archivado()
	if a[padre.ID] || a[hermano.ID] {
		t.Fatalf("desarchivar no devuelve la rama: %v", a)
	}
	if !a[hijo.ID] {
		t.Fatal("desarchivar al padre arrastra a un hijo que ya estaba archivado por su cuenta")
	}
	if cuenta(false) != 1 || fijadas() != 1 {
		t.Fatal("la nota no vuelve a la búsqueda o a fijadas al desarchivar")
	}
	notas, err := st.ListNotes(ctx, hermano.ID, "ada")
	if err != nil || len(notas) != 1 || !notas[0].Pinned {
		t.Fatalf("la nota no vuelve íntegra: %+v %v", notas, err)
	}
}

// Un diario no se archiva: ya vive fuera del árbol, y la 0015 lo prohíbe además en
// la base de datos.
func TestElDiarioNoSeArchiva(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	dia, err := st.DailyNotebook(ctx, "ada", "2026-10-01", "Hoy", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ArchiveNotebook(ctx, dia.ID, "ada", true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("el diario se ha archivado: %v", err)
	}
}
