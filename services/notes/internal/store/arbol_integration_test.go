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

// Anidar notas es lo que sostiene la vista de árbol. Lo que aquí se prueba es lo
// que la interfaz no puede arreglar: un ciclo en group_id, un padre de otro
// cuaderno, y una edición corriente que borraba el anidamiento.

func notaDeTexto(titulo string) domain.NoteInput {
	in := domain.NoteInput{Kind: domain.KindText, Title: titulo, Content: "<p>x</p>"}
	if err := in.Normalize(false); err != nil {
		panic(err)
	}
	return in
}

func TestAnidarNotasSinCiclosNiCruces(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Mudanza", domain.VisPersonal))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	otro, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Otro", domain.VisPersonal))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	a, _ := st.CreateNote(ctx, nb.ID, "ada", notaDeTexto("A"))
	b, _ := st.CreateNote(ctx, nb.ID, "ada", notaDeTexto("B"))
	ajena, _ := st.CreateNote(ctx, otro.ID, "ada", notaDeTexto("Ajena"))

	mover := func(n domain.Note, grupo *uuid.UUID) error {
		return st.MoveNote(ctx, n.ID, "ada", n.X, n.Y, n.Col, n.Position, grupo)
	}
	// B dentro de A: bien.
	if err := mover(b, &a.ID); err != nil {
		t.Fatalf("B dentro de A: %v", err)
	}
	// A dentro de B sería un ciclo.
	if err := mover(a, &b.ID); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("A dentro de B (ciclo): err=%v; quiero ErrInvalidInput", err)
	}
	// Una nota dentro de sí misma, también.
	if err := mover(a, &a.ID); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("A dentro de A: err=%v; quiero ErrInvalidInput", err)
	}
	// Un padre de otro cuaderno: la nota se vería en un sitio y colgaría de otro.
	if err := mover(a, &ajena.ID); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("A dentro de una nota ajena: err=%v; quiero ErrInvalidInput", err)
	}
	// Un padre que no existe.
	inexistente := uuid.New()
	if err := mover(a, &inexistente); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("A dentro de un id inexistente: err=%v; quiero ErrInvalidInput", err)
	}

	// EDITAR NO DESANIDA. El cliente edita sin mandar group_id, y antes eso lo
	// ponía a NULL: la primera corrección de una nota anidada la sacaba del árbol.
	editada, err := st.UpdateNote(ctx, b.ID, "ada", notaDeTexto("B corregida"))
	if err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}
	if editada.GroupID == nil || *editada.GroupID != a.ID {
		t.Errorf("tras editar, group_id=%v; tenía que seguir siendo A", editada.GroupID)
	}
	// Y sacarla es explícito: MoveNote con grupo nulo.
	if err := mover(b, nil); err != nil {
		t.Fatalf("sacar B: %v", err)
	}
	notas, _ := st.ListNotes(ctx, nb.ID, "ada")
	for _, n := range notas {
		if n.ID == b.ID && n.GroupID != nil {
			t.Error("B sigue anidada tras sacarla")
		}
	}
}
