// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// Dos dispositivos abren la misma nota; guarda el primero. El segundo, que editó
// sobre la versión de antes, no pisa lo del primero: recibe el conflicto y la nota
// se queda como la dejó el primero. Con la versión nueva, sí escribe.
func TestEditarSobreUnaVersionViejaDevuelveConflictoSinPisar(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	book, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Libreta", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	nota, err := st.CreateNote(ctx, book.ID, "ada", domain.NoteInput{Title: "t", Content: "original"})
	if err != nil {
		t.Fatal(err)
	}
	vista := nota.UpdatedAt

	primero := domain.NoteInput{Title: "t", Content: "del portátil", ExpectedUpdatedAt: &vista}
	if err := primero.Normalize(false); err != nil {
		t.Fatal(err)
	}
	guardada, _, err := st.ActualizarNota(ctx, nota.ID, "ada", primero)
	if err != nil {
		t.Fatal(err)
	}

	segundo := domain.NoteInput{Title: "t", Content: "del móvil", ExpectedUpdatedAt: &vista}
	if err := segundo.Normalize(false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.ActualizarNota(ctx, nota.ID, "ada", segundo); !errors.Is(err, domain.ErrNoteEditConflict) {
		t.Fatalf("escribir sobre una versión vieja = %v; quiero ErrNoteEditConflict", err)
	}
	lista, err := st.ListNotes(ctx, book.ID, "ada")
	if err != nil || len(lista) != 1 || lista[0].Content != "del portátil" {
		t.Fatalf("el conflicto ha pisado la nota: %+v %v", lista, err)
	}

	segundo.ExpectedUpdatedAt = &guardada.UpdatedAt
	if _, _, err := st.ActualizarNota(ctx, nota.ID, "ada", segundo); err != nil {
		t.Fatalf("con la versión al día debe escribir: %v", err)
	}
	sin := domain.NoteInput{Title: "t", Content: "cliente viejo"}
	if err := sin.Normalize(false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.ActualizarNota(ctx, nota.ID, "ada", sin); err != nil {
		t.Fatalf("sin versión esperada se escribe como antes: %v", err)
	}
}
