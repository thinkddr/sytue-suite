// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la retención de papelera.
//
// Lo que verifican es justo lo que puede fallar en silencio: que la purga
// atraviesa organizaciones —sin la política de la 0005 borraría CERO filas y el
// worker informaría de éxito para siempre— y que respeta el plazo, porque una
// purga que se lleva lo de ayer es pérdida de datos, no limpieza.
package store_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// enPapeleraDesde antigua el `deleted_at` de una nota para simular el paso del
// tiempo. Es la única forma de probar un plazo de treinta días sin esperarlos, y
// va por SQL directo a propósito: el store no expone «cambiar la fecha de
// borrado» y no debería, porque nadie más lo necesita.
func enPapeleraDesde(t *testing.T, h *pgtest.Harness, id string, hace time.Duration) {
	t.Helper()
	// VA EN UNA TRANSACCIÓN DE MANTENIMIENTO, no a pelo contra el pool. `notes`
	// tiene RLS con FORCE, así que un UPDATE sin `app.tenant` ni `app.maintenance`
	// fijados no casa con ninguna política y afecta a CERO filas —sin error—. Este
	// helper hacía justo eso: la nota no se antiguaba, la purga no encontraba nada
	// que hubiera cumplido el plazo y el test fallaba con «purgó 0», que parecía
	// culpa de la purga.
	err := h.Pool.InMaintenanceTx(t.Context(), func(tx pgx.Tx) error {
		tag, err := tx.Exec(t.Context(),
			`UPDATE notes SET deleted_at = now() - $2::interval WHERE id = $1`,
			id, hace.String())
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			t.Fatalf("antiguar deleted_at afectó a %d filas, no a 1", tag.RowsAffected())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("antiguar deleted_at: %v", err)
	}
}

// La purga respeta el plazo: lo reciente se queda.
func TestLaPurgaRespetaLaRetencion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Cuaderno", domain.VisPersonal))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	vieja, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "vieja"})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	nueva, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "nueva"})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	for _, n := range []domain.Note{vieja, nueva} {
		if err := st.DeleteNote(ctx, n.ID, "ada"); err != nil {
			t.Fatalf("DeleteNote: %v", err)
		}
	}
	enPapeleraDesde(t, h, vieja.ID.String(), 40*24*time.Hour)

	n, err := st.PurgeExpiredTrash(t.Context(), 30*24*time.Hour)
	if err != nil {
		t.Fatalf("PurgeExpiredTrash: %v", err)
	}
	if n != 1 {
		t.Fatalf("debía purgar solo la vieja (1 fila) y purgó %d", n)
	}

	papelera, err := st.ListTrash(ctx, "ada", 50)
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(papelera) != 1 || papelera[0].ID != nueva.ID {
		t.Fatalf("en la papelera debía quedar solo la nueva, y hay %d", len(papelera))
	}
}

// LO QUE NO PUEDE FALLAR: la purga atraviesa organizaciones. Sin la política de
// mantenimiento de la 0005, este DELETE afecta a cero filas y el worker informa
// de éxito eternamente mientras la papelera crece.
func TestLaPurgaAlcanzaATodasLasOrganizaciones(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)

	for _, org := range []string{"acme", "otra"} {
		ctx := tenantCtx(org)
		nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("C", domain.VisPersonal))
		if err != nil {
			t.Fatalf("CreateNotebook(%s): %v", org, err)
		}
		n, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "x"})
		if err != nil {
			t.Fatalf("CreateNote(%s): %v", org, err)
		}
		if err := st.DeleteNote(ctx, n.ID, "ada"); err != nil {
			t.Fatalf("DeleteNote(%s): %v", org, err)
		}
		enPapeleraDesde(t, h, n.ID.String(), 40*24*time.Hour)
	}

	n, err := st.PurgeExpiredTrash(t.Context(), 30*24*time.Hour)
	if err != nil {
		t.Fatalf("PurgeExpiredTrash: %v", err)
	}
	if n != 2 {
		t.Fatalf("debía purgar una nota de CADA organización (2) y purgó %d; "+
			"si es 0, falta la política de mantenimiento de la 0005", n)
	}
}

// Con la retención apagada no se borra nada, que es el interruptor para quien
// deba conservarlo todo.
func TestRetencionCeroNoPurgaNada(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	nb, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("C", domain.VisPersonal))
	if err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}
	n, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "x"})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if err := st.DeleteNote(ctx, n.ID, "ada"); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	enPapeleraDesde(t, h, n.ID.String(), 10*365*24*time.Hour)

	got, err := st.PurgeExpiredTrash(t.Context(), 0)
	if err != nil {
		t.Fatalf("PurgeExpiredTrash: %v", err)
	}
	if got != 0 {
		t.Fatalf("con retención 0 no debe purgar nada, y purgó %d", got)
	}
}
