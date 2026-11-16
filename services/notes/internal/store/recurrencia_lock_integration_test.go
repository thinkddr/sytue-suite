// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// Queue the editor before recurrence behind a held notebook lock. Before the
// fix recurrence holds the note while awaiting the notebook FK; releasing the
// blocker gives the notebook to the editor, which waits for that same note.
func TestRepeatAndEditAcquireNotebookBeforeNote(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx, cancel := context.WithTimeout(tenantCtx("acme"), 30*time.Second)
	defer cancel()
	book, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Routines", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	note, err := st.CreateNote(ctx, book.ID, "ada", domain.NoteInput{Title: "original", Kind: domain.KindTodo})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := st.SetReminder(ctx, note.ID, "ada", recordatorio(t, now.Add(-time.Hour), "FREQ=DAILY", 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateNote(ctx, note.ID, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "original", Checked: siBool(true)}); err != nil {
		t.Fatal(err)
	}
	conn, err := h.Pool.AcquireDirect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
		t.Fatal(err)
	}
	var blocker int32
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blocker); err != nil {
		t.Fatal(err)
	}
	var locked uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM notebooks WHERE tenant='acme' AND id=$1 FOR UPDATE`, book.ID).Scan(&locked); err != nil {
		t.Fatal(err)
	}
	edited := make(chan error, 1)
	go func() {
		_, err := st.UpdateNote(ctx, note.ID, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "updated"})
		edited <- err
	}()
	if err := waitBlockedCount(ctx, tx, blocker, 1, ""); err != nil {
		t.Fatal(err)
	}
	type repeatResult struct {
		note *domain.Note
		err  error
	}
	repeated := make(chan repeatResult, 1)
	go func() { n, err := st.RepetirAlCompletar(ctx, note.ID, now); repeated <- repeatResult{n, err} }()
	if err := waitBlockedCount(ctx, tx, blocker, 2, ""); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-edited:
		if err != nil {
			t.Fatal("concurrent edit", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case got := <-repeated:
		if got.err != nil || got.note == nil || got.note.Title != "updated" {
			t.Fatal("recurrence did not preserve updated task", got)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

// The next occurrence may have been moved to another notebook before undo.
// Its destination participates in the lock order, and an edited/moved copy
// must never be discarded as if it were still the untouched occurrence.
func TestUndoRepeatPreservesCopyMovedToAnotherNotebook(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	origin, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Origin", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	destination, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Destination", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	note, err := st.CreateNote(ctx, origin.ID, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "original"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := st.SetReminder(ctx, note.ID, "ada", recordatorio(t, now.Add(-time.Hour), "FREQ=DAILY", 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateNote(ctx, note.ID, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "original", Checked: siBool(true)}); err != nil {
		t.Fatal(err)
	}
	copy, err := st.RepetirAlCompletar(ctx, note.ID, now)
	if err != nil || copy == nil {
		t.Fatal("repeat", copy, err)
	}
	if err := st.MoveNoteToNotebook(ctx, copy.ID, "ada", destination.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateNote(ctx, note.ID, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "original", Checked: siBool(false)}); err != nil {
		t.Fatal(err)
	}
	removed, err := st.DeshacerRepeticion(ctx, note.ID)
	if err != nil || removed != nil {
		t.Fatal("removed a moved occurrence", removed, err)
	}
	got, err := st.ListNotes(ctx, destination.ID, "ada")
	if err != nil || len(got) != 1 || got[0].ID != copy.ID {
		t.Fatal("lost moved occurrence", got, err)
	}
}

// A stale writer is simulated directly to ensure the post-lock checks fail
// closed even if a legacy writer ignores the shared tree advisory.
func TestUndoRepeatRejectsChangedReferenceWhileWaiting(t *testing.T) {
	for _, change := range []string{"reference", "move", "delete"} {
		t.Run(change, func(t *testing.T) {
			h := pgtest.New(t, db.Migrations, db.MigrationsDir)
			st := store.New(h.Pool)
			ctx, cancel := context.WithTimeout(tenantCtx("acme"), 30*time.Second)
			defer cancel()
			origin, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Origin", domain.VisPersonal))
			if err != nil {
				t.Fatal(err)
			}
			destination, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Destination", domain.VisPersonal))
			if err != nil {
				t.Fatal(err)
			}
			note, err := st.CreateNote(ctx, origin.ID, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "original"})
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC().Truncate(time.Second)
			if _, err := st.SetReminder(ctx, note.ID, "ada", recordatorio(t, now.Add(-time.Hour), "FREQ=DAILY", 0)); err != nil {
				t.Fatal(err)
			}
			if _, err := st.UpdateNote(ctx, note.ID, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "original", Checked: siBool(true)}); err != nil {
				t.Fatal(err)
			}
			copy, err := st.RepetirAlCompletar(ctx, note.ID, now)
			if err != nil || copy == nil {
				t.Fatal("repeat", copy, err)
			}
			if _, err := st.UpdateNote(ctx, note.ID, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "original", Checked: siBool(false)}); err != nil {
				t.Fatal(err)
			}
			conn, err := h.Pool.AcquireDirect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Release()
			block, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = block.Rollback(context.Background()) }()
			if err := pg.SetTenant(ctx, block, "acme"); err != nil {
				t.Fatal(err)
			}
			var blocker int32
			if err := block.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blocker); err != nil {
				t.Fatal(err)
			}
			var locked uuid.UUID
			if err := block.QueryRow(ctx, `SELECT id FROM notebooks WHERE tenant='acme' AND id=$1 FOR UPDATE`, origin.ID).Scan(&locked); err != nil {
				t.Fatal(err)
			}
			type result struct {
				removed *uuid.UUID
				err     error
			}
			undo := make(chan result, 1)
			go func() { removed, err := st.DeshacerRepeticion(ctx, note.ID); undo <- result{removed, err} }()
			if err := waitBlockedCount(ctx, block, blocker, 1, "notes_recurrence_lock_books"); err != nil {
				t.Fatal(err)
			}
			admin, err := pgx.Connect(ctx, h.DSN)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = admin.Close(context.Background()) }()
			write, err := admin.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = write.Rollback(context.Background()) }()
			if err := db.MarkWriteTransaction(ctx, write); err != nil {
				t.Fatal(err)
			}
			wanted := copy.ID
			switch change {
			case "reference":
				wanted = uuid.New()
				_, err = write.Exec(ctx, `UPDATE notes SET data=jsonb_set(data,'{repeated_into}',to_jsonb($2::text)) WHERE id=$1`, note.ID, wanted.String())
			case "move":
				_, err = write.Exec(ctx, `UPDATE notes SET notebook_id=$2 WHERE id=$1`, copy.ID, destination.ID)
			case "delete":
				_, err = write.Exec(ctx, `DELETE FROM notes WHERE id=$1`, copy.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := write.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err := block.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-undo:
				if got.removed != nil || !errors.Is(got.err, domain.ErrNoteChanged) {
					t.Fatal("undo accepted changed source", got)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			var raw []byte
			if err := admin.QueryRow(ctx, `SELECT data FROM notes WHERE id=$1`, note.ID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var data struct {
				Repeated string `json:"repeated_into"`
			}
			if json.Unmarshal(raw, &data) != nil || data.Repeated != wanted.String() {
				t.Fatal("undo erased changed source", string(raw))
			}
		})
	}
}

func TestUndoAndEditCopyCompleteInBothNotebookOrders(t *testing.T) {
	for _, sourceFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("source_first_%t", sourceFirst), func(t *testing.T) {
			h := pgtest.New(t, db.Migrations, db.MigrationsDir)
			st := store.New(h.Pool)
			ctx, cancel := context.WithTimeout(tenantCtx("acme"), 30*time.Second)
			defer cancel()
			first, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("First", domain.VisPersonal))
			if err != nil {
				t.Fatal(err)
			}
			second, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Second", domain.VisPersonal))
			if err != nil {
				t.Fatal(err)
			}
			if (first.ID.String() < second.ID.String()) != sourceFirst {
				first, second = second, first
			}
			note, err := st.CreateNote(ctx, first.ID, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "original"})
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC().Truncate(time.Second)
			if _, err := st.SetReminder(ctx, note.ID, "ada", recordatorio(t, now.Add(-time.Hour), "FREQ=DAILY", 0)); err != nil {
				t.Fatal(err)
			}
			if _, err := st.UpdateNote(ctx, note.ID, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "original", Checked: siBool(true)}); err != nil {
				t.Fatal(err)
			}
			copy, err := st.RepetirAlCompletar(ctx, note.ID, now)
			if err != nil || copy == nil {
				t.Fatal("repeat", copy, err)
			}
			if err := st.MoveNoteToNotebook(ctx, copy.ID, "ada", second.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := st.UpdateNote(ctx, note.ID, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "original", Checked: siBool(false)}); err != nil {
				t.Fatal(err)
			}
			conn, err := h.Pool.AcquireDirect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Release()
			block, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = block.Rollback(context.Background()) }()
			if err := pg.SetTenant(ctx, block, "acme"); err != nil {
				t.Fatal(err)
			}
			var blocker int32
			if err := block.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blocker); err != nil {
				t.Fatal(err)
			}
			var locked uuid.UUID
			if err := block.QueryRow(ctx, `SELECT id FROM notebooks WHERE tenant='acme' AND id=$1 FOR UPDATE`, second.ID).Scan(&locked); err != nil {
				t.Fatal(err)
			}
			edited := make(chan error, 1)
			go func() {
				_, err := st.UpdateNote(ctx, copy.ID, "ada", domain.NoteInput{Kind: domain.KindTodo, Title: "edited"})
				edited <- err
			}()
			if err := waitBlockedCount(ctx, block, blocker, 1, ""); err != nil {
				t.Fatal(err)
			}
			type result struct {
				removed *uuid.UUID
				err     error
			}
			undo := make(chan result, 1)
			go func() { removed, err := st.DeshacerRepeticion(ctx, note.ID); undo <- result{removed, err} }()
			if err := waitBlockedCount(ctx, block, blocker, 2, ""); err != nil {
				t.Fatal(err)
			}
			if err := block.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-edited:
				if err != nil {
					t.Fatal("edit", err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			select {
			case got := <-undo:
				if got.err != nil || got.removed != nil {
					t.Fatal("undo", got)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			got, err := st.ListNotes(ctx, second.ID, "ada")
			if err != nil || len(got) != 1 || got[0].ID != copy.ID || got[0].Title != "edited" {
				t.Fatal("edited copy lost", got, err)
			}
		})
	}
}
