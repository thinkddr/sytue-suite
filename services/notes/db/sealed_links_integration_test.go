// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package db_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	notesdb "git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

func TestDormantSealedLinksFenceProtectsSourcesAndOldWriters(t *testing.T) {
	h := pgtest.New(t, notesdb.Migrations, notesdb.MigrationsDir)
	ctx := writerContext(t)
	st := store.New(h.Pool)
	bookInput := domain.NotebookInput{Title: "book", Visibility: domain.VisPersonal}
	if err := bookInput.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, "ada", bookInput)
	if err != nil {
		t.Fatal(err)
	}
	noteInput := domain.NoteInput{Title: "private", Links: []domain.NoteLink{{TargetKind: "note", TargetID: "one", Label: "secret"}}}
	if err := noteInput.Normalize(false); err != nil {
		t.Fatal(err)
	}
	note, err := st.CreateNote(ctx, book.ID, "ada", noteInput)
	if err != nil {
		t.Fatal(err)
	}
	run := func(version, sql string) error {
		return h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config('app.notes_write_version',$1,true)`, version); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, sql, note.ID)
			return err
		})
	}
	requireFence := func(err error, name string) {
		t.Helper()
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "55000" || pgErr.ConstraintName != name {
			t.Fatalf("expected %s, got %v", name, err)
		}
	}
	seal := `UPDATE notes SET title='',content='',content_cipher='encv1:fixture', min_write_version=4, private_links_sealed=true WHERE id=$1`
	requireFence(run("4", seal), "notes_sealed_links") // source must still exist
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.notes_write_version','4',true)`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM note_links WHERE note_id=$1`, note.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, seal, note.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	requireFence(run("2", `UPDATE notes SET content_cipher='encv1:old' WHERE id=$1`), "notes_write_contract")
	requireFence(run("3", `UPDATE notes SET content_cipher='encv1:structural-only' WHERE id=$1`), "notes_write_contract")
	requireFence(run("4", `UPDATE notes SET private_links_sealed=false WHERE id=$1`), "notes_sealed_links")
	requireFence(run("4", `INSERT INTO note_links(tenant,note_id,target_kind,target_id,label) VALUES('acme',$1,'note','two','secret')`), "notes_sealed_links")
	// Deletion of the note must still be able to cascade and satisfy retention.
	if err := run("4", `DELETE FROM notes WHERE id=$1`); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentClearLinkCannotRaceMarkerActivation(t *testing.T) {
	h := pgtest.New(t, notesdb.Migrations, notesdb.MigrationsDir)
	ctx := writerContext(t)
	st := store.New(h.Pool)
	bookInput := domain.NotebookInput{Title: "book", Visibility: domain.VisPersonal}
	if err := bookInput.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, "ada", bookInput)
	if err != nil {
		t.Fatal(err)
	}
	noteInput := domain.NoteInput{Title: "private"}
	if err := noteInput.Normalize(false); err != nil {
		t.Fatal(err)
	}
	note, err := st.CreateNote(ctx, book.ID, "ada", noteInput)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := h.Pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = pg.SetTenant(ctx, tx, "acme"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO note_links(tenant,note_id,target_kind,target_id) VALUES('acme',$1,'note','new')`, note.ID); err != nil {
		t.Fatal(err)
	}
	admin := h.PoolSinRLS(t)
	var holderPID int32
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
		t.Fatal(err)
	}
	waitLock := func(pid int32) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			var waiting bool
			if err := admin.QueryRow(ctx, `SELECT $1 = ANY(pg_blocking_pids($2))`, holderPID, pid).Scan(&waiting); err != nil {
				t.Fatal(err)
			}
			if waiting {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("backend %d did not wait on a lock", pid)
	}
	started, done := make(chan int32, 1), make(chan error, 1)
	go func() {
		done <- h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			var pid int32
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			started <- pid
			if _, err := tx.Exec(ctx, `SELECT set_config('app.notes_write_version','4',true)`); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE notes SET title='',content='',content_cipher='encv1:fixture',min_write_version=4,private_links_sealed=true WHERE id=$1`, note.ID)
			return err
		})
	}()
	var pid int32
	select {
	case pid = <-started:
	case err := <-done:
		t.Fatalf("activation exited before locking: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("activation did not start")
	}
	waitLock(pid)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != "notes_sealed_links" {
			t.Fatalf("concurrent link was not detected: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("activation did not finish")
	}

	// Reverse interleaving: activation owns the row first; an old writer's
	// insert must wait and then observe the committed marker.
	tx, err = conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = pg.SetTenant(ctx, tx, "acme"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('app.notes_write_version','4',true)`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM note_links WHERE note_id=$1`, note.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE notes SET title='',content='',content_cipher='encv1:fixture',min_write_version=4,private_links_sealed=true WHERE id=$1`, note.ID); err != nil {
		t.Fatal(err)
	}
	started2, done2 := make(chan int32, 1), make(chan error, 1)
	go func() {
		done2 <- h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			var pid int32
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			started2 <- pid
			_, err := tx.Exec(ctx, `INSERT INTO note_links(tenant,note_id,target_kind,target_id) VALUES('acme',$1,'note','late')`, note.ID)
			return err
		})
	}()
	select {
	case pid = <-started2:
	case err := <-done2:
		t.Fatalf("child exited before locking: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("child did not start")
	}
	waitLock(pid)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done2:
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != "notes_sealed_links" {
			t.Fatalf("late clear link was not fenced: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("link insert did not finish")
	}
}

func TestSealedLinksRejectsStaleRepeatableReadSnapshot(t *testing.T) {
	h := pgtest.New(t, notesdb.Migrations, notesdb.MigrationsDir)
	ctx := writerContext(t)
	st := store.New(h.Pool)
	bookInput := domain.NotebookInput{Title: "book", Visibility: domain.VisPersonal}
	if err := bookInput.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, "ada", bookInput)
	if err != nil {
		t.Fatal(err)
	}
	noteInput := domain.NoteInput{Title: "private"}
	if err := noteInput.Normalize(false); err != nil {
		t.Fatal(err)
	}
	note, err := st.CreateNote(ctx, book.ID, "ada", noteInput)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := h.Pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = pg.SetTenant(ctx, tx, "acme"); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notes WHERE id=$1)`, note.ID).Scan(&exists); err != nil || !exists {
		t.Fatalf("snapshot: %v %v", exists, err)
	}
	if err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.notes_write_version','4',true)`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE notes SET title='',content='',content_cipher='encv1:fixture',min_write_version=4,private_links_sealed=true WHERE id=$1`, note.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO note_links(tenant,note_id,target_kind,target_id) VALUES('acme',$1,'note','late')`, note.ID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "40001" {
		t.Fatalf("stale snapshot did not abort: %v", err)
	}
}

func TestLegacyLinksRespectRLSAndMoveInEitherMergeOrder(t *testing.T) {
	h := pgtest.New(t, notesdb.Migrations, notesdb.MigrationsDir)
	ctx := writerContext(t)
	st := store.New(h.Pool)
	bookInput := domain.NotebookInput{Title: "book", Visibility: domain.VisPersonal}
	if err := bookInput.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, "ada", bookInput)
	if err != nil {
		t.Fatal(err)
	}
	noteInput := domain.NoteInput{Title: "private", Links: []domain.NoteLink{{TargetKind: "note", TargetID: "one"}}}
	if err := noteInput.Normalize(false); err != nil {
		t.Fatal(err)
	}
	a, err := st.CreateNote(ctx, book.ID, "ada", noteInput)
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateNote(ctx, book.ID, "ada", noteInput)
	if err != nil {
		t.Fatal(err)
	}
	other := tenancy.Into(t.Context(), tenancy.Tenant{Slug: "other"})
	if err := h.Pool.InTenantTx(other, func(tx pgx.Tx) error {
		_, err := tx.Exec(other, `INSERT INTO note_links(tenant,note_id,target_kind,target_id) VALUES('other',$1,'note','foreign')`, a.ID)
		return err
	}); err == nil {
		t.Fatal("another tenant attached a link to an invisible note")
	}
	if err := h.Pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		if err := notesdb.MarkWriteTransaction(ctx, tx); err != nil {
			return err
		}
		for _, q := range []struct {
			sql string
			id  any
		}{
			{`UPDATE note_links SET tenant='other' WHERE note_id=$1`, a.ID},
			{`UPDATE notes SET tenant='other' WHERE id=$1`, a.ID},
			{`UPDATE notes SET tenant='other' WHERE id=$1`, b.ID},
			{`UPDATE note_links SET tenant='other' WHERE note_id=$1`, b.ID},
			{`UPDATE notebooks SET tenant='other' WHERE id=$1`, book.ID},
		} {
			if _, err := tx.Exec(ctx, q.sql, q.id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLinkMutationsLockBothParents(t *testing.T) {
	for _, tc := range []struct {
		name            string
		sql             string
		lockDestination bool
	}{
		{"delete", `DELETE FROM note_links WHERE note_id=$1`, false},
		{"update-source", `UPDATE note_links SET note_id=$2 WHERE note_id=$1`, false},
		{"update-destination", `UPDATE note_links SET note_id=$2 WHERE note_id=$1`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := pgtest.New(t, notesdb.Migrations, notesdb.MigrationsDir)
			ctx := writerContext(t)
			st := store.New(h.Pool)
			bookInput := domain.NotebookInput{Title: "book", Visibility: domain.VisPersonal}
			if err := bookInput.Normalize(); err != nil {
				t.Fatal(err)
			}
			book, err := st.CreateNotebook(ctx, "ada", bookInput)
			if err != nil {
				t.Fatal(err)
			}
			input := domain.NoteInput{Title: "source", Links: []domain.NoteLink{{TargetKind: "note", TargetID: "reference"}}}
			if err := input.Normalize(false); err != nil {
				t.Fatal(err)
			}
			source, err := st.CreateNote(ctx, book.ID, "ada", input)
			if err != nil {
				t.Fatal(err)
			}
			input.Links = nil
			destination, err := st.CreateNote(ctx, book.ID, "ada", input)
			if err != nil {
				t.Fatal(err)
			}
			conn, err := h.Pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Release()
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
				t.Fatal(err)
			}
			locked := source.ID
			if tc.lockDestination {
				locked = destination.ID
			}
			var holder int32
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holder); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `SELECT id FROM notes WHERE id=$1 FOR UPDATE`, locked); err != nil {
				t.Fatal(err)
			}
			started, done := make(chan int32, 1), make(chan error, 1)
			go func() {
				done <- h.Pool.InTenantTx(ctx, func(child pgx.Tx) error {
					var pid int32
					if err := child.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
						return err
					}
					started <- pid
					if tc.name == "delete" {
						_, err := child.Exec(ctx, tc.sql, source.ID)
						return err
					}
					_, err := child.Exec(ctx, tc.sql, source.ID, destination.ID)
					return err
				})
			}()
			var pid int32
			select {
			case pid = <-started:
			case err := <-done:
				t.Fatalf("mutation exited early: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("mutation did not start")
			}
			admin := h.PoolSinRLS(t)
			deadline := time.Now().Add(5 * time.Second)
			blocked := false
			for time.Now().Before(deadline) {
				if err := admin.QueryRow(ctx, `SELECT $1 = ANY(pg_blocking_pids($2))`, holder, pid).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !blocked {
				t.Fatal("child mutation did not lock the expected parent")
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("mutation did not finish")
			}
		})
	}
}

func TestLinkFenceAllowsPurgeAndGroupCascade(t *testing.T) {
	h := pgtest.New(t, notesdb.Migrations, notesdb.MigrationsDir)
	ctx := writerContext(t)
	st := store.New(h.Pool)
	bookInput := domain.NotebookInput{Title: "book", Visibility: domain.VisPersonal}
	if err := bookInput.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, "ada", bookInput)
	if err != nil {
		t.Fatal(err)
	}
	parentInput := domain.NoteInput{Title: "parent", Links: []domain.NoteLink{{TargetKind: "note", TargetID: "reference"}}}
	if err := parentInput.Normalize(false); err != nil {
		t.Fatal(err)
	}
	parent, err := st.CreateNote(ctx, book.ID, "ada", parentInput)
	if err != nil {
		t.Fatal(err)
	}
	childInput := domain.NoteInput{Title: "child", GroupID: &parent.ID}
	if err := childInput.Normalize(false); err != nil {
		t.Fatal(err)
	}
	child, err := st.CreateNote(ctx, book.ID, "ada", childInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := notesdb.MarkWriteTransaction(ctx, tx); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM notes WHERE id=$1`, parent.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var detached bool
		if err := tx.QueryRow(ctx, `SELECT group_id IS NULL FROM notes WHERE id=$1`, child.ID).Scan(&detached); err != nil {
			return err
		}
		if !detached {
			return errors.New("group child remained attached")
		}
		var links int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM note_links WHERE note_id=$1`, parent.ID).Scan(&links); err != nil {
			return err
		}
		if links != 0 {
			return errors.New("links survived purge")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
