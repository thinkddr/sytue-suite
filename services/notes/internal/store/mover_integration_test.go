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

// Mover una nota de cuaderno: se lleva a las agrupadas, aterriza suelta al final
// de la primera columna del destino, y no cruza a cuadernos de otra persona.
func TestMoverNotaDeCuaderno(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	origen, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Origen", domain.VisPersonal))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	destino, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Destino", domain.VisPersonal))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	ajeno, err := st.CreateNotebook(ctx, "bob", entradaCuaderno("De Bob", domain.VisPersonal))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	ya, _ := st.CreateNote(ctx, destino.ID, "ada", notaDeTexto("Ya estaba"))
	padre, _ := st.CreateNote(ctx, origen.ID, "ada", notaDeTexto("Padre"))
	hija, _ := st.CreateNote(ctx, origen.ID, "ada", notaDeTexto("Hija"))
	if err := st.MoveNote(ctx, hija.ID, "ada", hija.X, hija.Y, hija.Col, hija.Position, &padre.ID); err != nil {
		t.Fatalf("agrupar: %v", err)
	}
	// Padre pasa a estar agrupado bajo otra nota del origen, para comprobar que
	// llega SUELTO al destino.
	tia, _ := st.CreateNote(ctx, origen.ID, "ada", notaDeTexto("Tía"))
	if err := st.MoveNote(ctx, padre.ID, "ada", padre.X, padre.Y, padre.Col, padre.Position, &tia.ID); err != nil {
		t.Fatalf("agrupar padre: %v", err)
	}

	if err := st.MoveNoteToNotebook(ctx, padre.ID, "ada", destino.ID); err != nil {
		t.Fatalf("MoveNoteToNotebook: %v", err)
	}
	enOrigen, _ := st.ListNotes(ctx, origen.ID, "ada")
	enDestino, _ := st.ListNotes(ctx, destino.ID, "ada")
	if len(enOrigen) != 1 || enOrigen[0].ID != tia.ID {
		t.Errorf("en el origen queda %d nota(s); quería solo «Tía»", len(enOrigen))
	}
	if len(enDestino) != 3 {
		t.Fatalf("en el destino hay %d notas; quería 3", len(enDestino))
	}
	var p, hh domain.Note
	for _, n := range enDestino {
		switch n.ID {
		case padre.ID:
			p = n
		case hija.ID:
			hh = n
		}
	}
	if p.GroupID != nil {
		t.Errorf("el padre llega agrupado (%v); quería suelto", *p.GroupID)
	}
	if p.Col != 0 || p.Position <= ya.Position {
		t.Errorf("el padre aterriza en col=%d pos=%d; quería col 0 detrás de pos %d", p.Col, p.Position, ya.Position)
	}
	if hh.GroupID == nil || *hh.GroupID != padre.ID {
		t.Errorf("la hija pierde su grupo (%v); quería seguir bajo el padre", hh.GroupID)
	}

	// A un cuaderno de otra persona, no: y se contesta como si no existiera.
	if err := st.MoveNoteToNotebook(ctx, tia.ID, "ada", ajeno.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("mover al cuaderno de Bob: err=%v; quería ErrNotFound", err)
	}
	// Al mismo cuaderno, tampoco es un movimiento.
	if err := st.MoveNoteToNotebook(ctx, tia.ID, "ada", origen.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("mover al mismo cuaderno: err=%v; quería ErrNotFound", err)
	}
	// Una nota que no existe.
	if err := st.MoveNoteToNotebook(ctx, uuid.New(), "ada", destino.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("nota inexistente: err=%v; quería ErrNotFound", err)
	}
}
