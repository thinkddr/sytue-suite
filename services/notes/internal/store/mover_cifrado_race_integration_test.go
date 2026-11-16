// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
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

// Wait for the actual PostgreSQL row lock, not a sleep that only assumes the
// move has reached its check. The writer holds the encryption update uncommitted.
func waitMoveLock(ctx context.Context, tx pgx.Tx, blocker int32, marker string, result <-chan error) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-result:
			return errors.Join(errors.New("move finished before the notebook lock"), err)
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := tx.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
				return err
			}
			var waiting bool
			err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity
				WHERE $1=ANY(pg_blocking_pids(pid)) AND query LIKE $2)`, blocker, "%"+marker+"%").Scan(&waiting)
			if err != nil {
				return err
			}
			if waiting {
				return nil
			}
		}
	}
}

func waitBlockedCount(ctx context.Context, tx pgx.Tx, blocker int32, count int, marker string) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := tx.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
				return err
			}
			var found int
			if err := tx.QueryRow(ctx, `
				WITH RECURSIVE blocked AS (
					SELECT pid, query FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid))
					UNION
					SELECT a.pid, a.query FROM pg_stat_activity a JOIN blocked b ON b.pid = ANY(pg_blocking_pids(a.pid))
				)
				SELECT count(*) FROM blocked WHERE query LIKE $2`, blocker, "%"+marker+"%").Scan(&found); err != nil {
				return err
			}
			if found >= count {
				return nil
			}
		}
	}
}

func TestMoveRechecksEncryptionAfterConcurrentNotebookChange(t *testing.T) {
	for _, target := range []string{"origin", "destination"} {
		t.Run(target, func(t *testing.T) {
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
			note, err := st.CreateNote(ctx, origin.ID, "ada", notaDeTexto("original"))
			if err != nil {
				t.Fatal(err)
			}
			changed := destination.ID
			if target == "origin" {
				changed = origin.ID
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
			if err := db.MarkWriteTransaction(ctx, tx); err != nil {
				t.Fatal(err)
			}
			var blocker int32
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blocker); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `UPDATE notebooks SET encrypted=true,indexable=false,
				crypto='{"v":1,"kdf":"PBKDF2-SHA256","salt":"abcdefghijklmnopqrstuvwx","iterations":600000,"check":"encv1:fixture"}'::jsonb
				WHERE id=$1 AND tenant='acme'`, changed); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() { result <- st.MoveNoteToNotebook(ctx, note.ID, "ada", destination.ID) }()
			if err := waitMoveLock(ctx, tx, blocker, "notes_move_lock_books", result); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-result:
				if !errors.Is(err, domain.ErrInvalidInput) {
					t.Fatalf("move accepted changed encryption: %v", err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			notes, err := st.ListNotes(ctx, origin.ID, "ada")
			if err != nil || len(notes) != 1 || notes[0].ID != note.ID || notes[0].Title != note.Title {
				t.Fatalf("failed move changed original: %#v %v", notes, err)
			}
		})
	}
}

// Even a legacy inconsistent tree must not move an encrypted descendant into a
// different unencrypted notebook, nor move its plaintext parent partially.
func TestMoveRejectsEncryptedDescendantWithoutMovingFamily(t *testing.T) {
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
	parent, err := st.CreateNote(ctx, origin.ID, "ada", notaDeTexto("parent"))
	if err != nil {
		t.Fatal(err)
	}
	child, err := st.CreateNote(ctx, origin.ID, "ada", notaDeTexto("child"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MoveNote(ctx, child.ID, "ada", 0, 0, 0, 0, &parent.ID); err != nil {
		t.Fatal(err)
	}
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := db.MarkWriteTransaction(ctx, tx); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE notes SET title='',content='',content_cipher='encv1:retained' WHERE tenant='acme' AND id=$1`, child.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MoveNoteToNotebook(ctx, parent.ID, "ada", destination.ID); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatal("moved encrypted child", err)
	}
	notes, err := st.ListNotes(ctx, origin.ID, "ada")
	if err != nil || len(notes) != 2 {
		t.Fatal("family moved despite failure", notes, err)
	}
	var found bool
	for _, n := range notes {
		if n.ID == child.ID && n.ContentCipher == "encv1:retained" && n.GroupID != nil && *n.GroupID == parent.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("ciphertext or parent relationship changed")
	}
	if dest, err := st.ListNotes(ctx, destination.ID, "ada"); err != nil || len(dest) != 0 {
		t.Fatal("destination changed", dest, err)
	}
}

func TestMoveRechecksEditorAfterConcurrentRevocation(t *testing.T) {
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
	for _, id := range []uuid.UUID{origin.ID, destination.ID} {
		if _, _, err := st.CompartirCuaderno(ctx, id, "ada", "bob", true); err != nil {
			t.Fatal(err)
		}
	}
	note, err := st.CreateNote(ctx, origin.ID, "ada", notaDeTexto("original"))
	if err != nil {
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
	tag, err := tx.Exec(ctx, `DELETE FROM notebook_editores WHERE tenant='acme' AND notebook_id=$1 AND sujeto='bob'`, destination.ID)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatal("fixture grant missing", err)
	}
	result := make(chan error, 1)
	go func() { result <- st.MoveNoteToNotebook(ctx, note.ID, "bob", destination.ID) }()
	if err := waitMoveLock(ctx, tx, blocker, "notes_move_lock_grants", result); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("move accepted revoked editor", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	notes, err := st.ListNotes(ctx, origin.ID, "ada")
	if err != nil || len(notes) != 1 || notes[0].ID != note.ID {
		t.Fatal("revoked move changed origin", notes, err)
	}
}

func TestMoveRejectsLegacyTreeBeyondDepthLimit(t *testing.T) {
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
	root := uuid.New()
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := db.MarkWriteTransaction(ctx, tx); err != nil {
			return err
		}
		var parent *uuid.UUID
		for i := 0; i < 66; i++ {
			id := uuid.New()
			if i == 0 {
				id = root
			}
			_, err := tx.Exec(ctx, `INSERT INTO notes(id,tenant,notebook_id,owner_sub,title,group_id)
				VALUES($1,'acme',$2,'ada','retained',$3)`, id, origin.ID, parent)
			if err != nil {
				return err
			}
			parent = &id
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MoveNoteToNotebook(ctx, root, "ada", destination.ID); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatal("partial deep tree accepted", err)
	}
	if notes, err := st.ListNotes(ctx, origin.ID, "ada"); err != nil || len(notes) != 66 {
		t.Fatal("deep source changed", len(notes), err)
	}
	if notes, err := st.ListNotes(ctx, destination.ID, "ada"); err != nil || len(notes) != 0 {
		t.Fatal("deep destination changed", len(notes), err)
	}
}

func TestMoveAndConcurrentGroupingRemainInOneNotebook(t *testing.T) {
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
	parent, err := st.CreateNote(ctx, origin.ID, "ada", notaDeTexto("parent"))
	if err != nil {
		t.Fatal(err)
	}
	child, err := st.CreateNote(ctx, origin.ID, "ada", notaDeTexto("child"))
	if err != nil {
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
	var blocker int32
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blocker); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('sytue:notes:tree:' || $1,0))`, "acme"); err != nil {
		t.Fatal(err)
	}
	moved, grouped := make(chan error, 1), make(chan error, 1)
	go func() { moved <- st.MoveNoteToNotebook(ctx, parent.ID, "ada", destination.ID) }()
	go func() { grouped <- st.MoveNote(ctx, child.ID, "ada", 0, 0, 0, 0, &parent.ID) }()
	if err := waitBlockedCount(ctx, tx, blocker, 2, "notes_tree_guard"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-moved:
		if err != nil {
			t.Fatal("move", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var groupErr error
	select {
	case groupErr = <-grouped:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if groupErr != nil && !errors.Is(groupErr, domain.ErrInvalidInput) {
		t.Fatal("group", groupErr)
	}
	old, err := st.ListNotes(ctx, origin.ID, "ada")
	if err != nil {
		t.Fatal(err)
	}
	next, err := st.ListNotes(ctx, destination.ID, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if groupErr == nil {
		if len(old) != 0 || len(next) != 2 {
			t.Fatal("grouped family split", old, next)
		}
		for _, n := range next {
			if n.ID == child.ID && (n.GroupID == nil || *n.GroupID != parent.ID) {
				t.Fatal("child lost its parent")
			}
		}
	} else {
		if len(old) != 1 || old[0].ID != child.ID || old[0].GroupID != nil || len(next) != 1 || next[0].ID != parent.ID {
			t.Fatal("stale grouping changed tree", old, next)
		}
	}
}

func TestMoveAndConcurrentEditFinishWithoutLockInversion(t *testing.T) {
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
	note, err := st.CreateNote(ctx, origin.ID, "ada", notaDeTexto("original"))
	if err != nil {
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
	if err := tx.QueryRow(ctx, `SELECT id FROM notebooks WHERE tenant='acme' AND id=$1 FOR UPDATE`, origin.ID).Scan(&locked); err != nil {
		t.Fatal(err)
	}
	moved, edited := make(chan error, 1), make(chan error, 1)
	go func() { moved <- st.MoveNoteToNotebook(ctx, note.ID, "ada", destination.ID) }()
	go func() { _, err := st.UpdateNote(ctx, note.ID, "ada", notaDeTexto("updated")); edited <- err }()
	if err := waitBlockedCount(ctx, tx, blocker, 2, ""); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-moved:
		if err != nil {
			t.Fatal("move", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case err := <-edited:
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			t.Fatal("edit lock failure", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	notes, err := st.ListNotes(ctx, destination.ID, "ada")
	if err != nil || len(notes) != 1 || notes[0].ID != note.ID {
		t.Fatal("moved note missing", notes, err)
	}
}

func TestMoveAndTrashPurgeSerializePerTenant(t *testing.T) {
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
	parent, err := st.CreateNote(ctx, origin.ID, "ada", notaDeTexto("parent"))
	if err != nil {
		t.Fatal(err)
	}
	child, err := st.CreateNote(ctx, origin.ID, "ada", notaDeTexto("child"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MoveNote(ctx, child.ID, "ada", 0, 0, 0, 0, &parent.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteNote(ctx, child.ID, "ada"); err != nil {
		t.Fatal(err)
	}
	enPapeleraDesde(t, h, child.ID.String(), 40*24*time.Hour)
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
	var blocker int32
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blocker); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('sytue:notes:tree:' || $1,0))`, "acme"); err != nil {
		t.Fatal(err)
	}
	type purgeResult struct {
		count int64
		err   error
	}
	moved, purged := make(chan error, 1), make(chan purgeResult, 1)
	go func() { moved <- st.MoveNoteToNotebook(ctx, parent.ID, "ada", destination.ID) }()
	go func() { count, err := st.PurgeExpiredTrash(ctx, 30*24*time.Hour); purged <- purgeResult{count, err} }()
	if err := waitBlockedCount(ctx, tx, blocker, 1, "notes_tree_guard"); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-purged:
		if result.err == nil || result.count != 0 {
			t.Fatal("purge did not defer busy tree", result)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-moved:
		if err != nil {
			t.Fatal("move", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if count, err := st.PurgeExpiredTrash(ctx, 30*24*time.Hour); err != nil || count != 1 {
		t.Fatal("purge retry", count, err)
	}
	if notes, err := st.ListNotes(ctx, destination.ID, "ada"); err != nil || len(notes) != 1 || notes[0].ID != parent.ID {
		t.Fatal("parent lost", notes, err)
	}
	if trash, err := st.ListTrash(ctx, "ada", 50); err != nil || len(trash) != 0 {
		t.Fatal("expired child not purged", trash, err)
	}
}

func TestTrashPurgeReportsCommittedTenantsAndRetriesTheRest(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	for _, tenant := range []string{"a", "b"} {
		ctx := tenantCtx(tenant)
		book, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Book", domain.VisPersonal))
		if err != nil {
			t.Fatal(err)
		}
		note, err := st.CreateNote(ctx, book.ID, "ada", notaDeTexto("trash"))
		if err != nil {
			t.Fatal(err)
		}
		if err := st.DeleteNote(ctx, note.ID, "ada"); err != nil {
			t.Fatal(err)
		}
		enPapeleraDesde(t, h, note.ID.String(), 40*24*time.Hour)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
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
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('sytue:notes:tree:' || $1,0))`, "a"); err != nil {
		t.Fatal(err)
	}
	count, err := st.PurgeExpiredTrash(ctx, 30*24*time.Hour)
	if count != 1 || err == nil {
		t.Fatal("busy tenant hid later progress", count, err)
	}
	if rows, err := st.ListTrash(tenantCtx("a"), "ada", 50); err != nil || len(rows) != 1 {
		t.Fatal("busy tenant was changed", rows, err)
	}
	if rows, err := st.ListTrash(tenantCtx("b"), "ada", 50); err != nil || len(rows) != 0 {
		t.Fatal("later tenant was starved", rows, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if count, err := st.PurgeExpiredTrash(ctx, 30*24*time.Hour); err != nil || count != 1 {
		t.Fatal("remaining tenant not retried", count, err)
	}
	for _, tenant := range []string{"a", "b"} {
		if rows, err := st.ListTrash(tenantCtx(tenant), "ada", 50); err != nil || len(rows) != 0 {
			t.Fatal("trash remained", tenant, rows, err)
		}
	}
}

func TestTrashPurgeContinuesPastFailedTenantAndRetries(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	for _, tenant := range []string{"a", "b"} {
		ctx := tenantCtx(tenant)
		book, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Book", domain.VisPersonal))
		if err != nil {
			t.Fatal(err)
		}
		note, err := st.CreateNote(ctx, book.ID, "ada", notaDeTexto("trash"))
		if err != nil {
			t.Fatal(err)
		}
		if err := st.DeleteNote(ctx, note.ID, "ada"); err != nil {
			t.Fatal(err)
		}
		enPapeleraDesde(t, h, note.ID.String(), 40*24*time.Hour)
	}
	admin, err := pgx.Connect(t.Context(), h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	_, err = admin.Exec(t.Context(), `CREATE FUNCTION fixture_purge_failure() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF OLD.tenant='a' THEN RAISE EXCEPTION 'synthetic purge failure'; END IF; RETURN OLD; END $$;
		CREATE TRIGGER fixture_purge_failure BEFORE DELETE ON notes FOR EACH ROW EXECUTE FUNCTION fixture_purge_failure()`)
	if err != nil {
		t.Fatal(err)
	}
	count, err := st.PurgeExpiredTrash(t.Context(), 30*24*time.Hour)
	if err == nil || count != 1 {
		t.Fatal("failure hid another tenant's committed purge", count, err)
	}
	if rows, err := st.ListTrash(tenantCtx("a"), "ada", 50); err != nil || len(rows) != 1 {
		t.Fatal("failed tenant did not roll back", rows, err)
	}
	if rows, err := st.ListTrash(tenantCtx("b"), "ada", 50); err != nil || len(rows) != 0 {
		t.Fatal("later tenant was starved", rows, err)
	}
	if _, err := admin.Exec(t.Context(), `DROP TRIGGER fixture_purge_failure ON notes`); err != nil {
		t.Fatal(err)
	}
	if count, err := st.PurgeExpiredTrash(t.Context(), 30*24*time.Hour); err != nil || count != 1 {
		t.Fatal("failed tenant was not retried", count, err)
	}
}

func TestTrashPurgePreservesCrossTenantLegacyReferences(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	a, b := tenantCtx("a"), tenantCtx("b")
	bookA, err := st.CreateNotebook(a, "ada", entradaCuaderno("A", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	bookB, err := st.CreateNotebook(b, "bea", entradaCuaderno("B", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	parent, err := st.CreateNote(a, bookA.ID, "ada", notaDeTexto("parent"))
	if err != nil {
		t.Fatal(err)
	}
	foreignGroup, err := st.CreateNote(b, bookB.ID, "bea", notaDeTexto("foreign group"))
	if err != nil {
		t.Fatal(err)
	}
	foreignBook, err := st.CreateNote(b, bookB.ID, "bea", notaDeTexto("foreign book"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteNote(a, parent.ID, "ada"); err != nil {
		t.Fatal(err)
	}
	enPapeleraDesde(t, h, parent.ID.String(), 40*24*time.Hour)
	if err := st.DeleteNotebook(a, bookA.ID, "ada"); err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.Connect(t.Context(), h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	tx, err := admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := db.MarkWriteTransaction(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE notebooks SET deleted_at=now()-interval '40 days' WHERE id=$1`, []any{bookA.ID}},
		{`UPDATE notebooks SET parent_id=$1 WHERE id=$2`, []any{bookA.ID, bookB.ID}},
		{`UPDATE notes SET group_id=$1 WHERE id=$2`, []any{parent.ID, foreignGroup.ID}},
		{`UPDATE notes SET notebook_id=$1 WHERE id=$2`, []any{bookA.ID, foreignBook.ID}},
	} {
		if _, err := tx.Exec(t.Context(), q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if count, err := st.PurgeExpiredTrash(t.Context(), 30*24*time.Hour); err != nil || count != 0 {
		t.Fatal("purged unsafe cross-tenant references", count, err)
	}
	var notebooks, notes, groups, misplaced int
	if err := admin.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM notebooks WHERE id=ANY($1::uuid[])),
		(SELECT count(*) FROM notes WHERE id=ANY($2::uuid[])),
		(SELECT count(*) FROM notes WHERE id=$3 AND group_id=$4),
		(SELECT count(*) FROM notes WHERE id=$5 AND notebook_id=$6)`,
		[]uuid.UUID{bookA.ID, bookB.ID}, []uuid.UUID{parent.ID, foreignGroup.ID, foreignBook.ID}, foreignGroup.ID, parent.ID, foreignBook.ID, bookA.ID).Scan(&notebooks, &notes, &groups, &misplaced); err != nil {
		t.Fatal(err)
	}
	if notebooks != 2 || notes != 3 || groups != 1 || misplaced != 1 {
		t.Fatal("cross-tenant source changed", notebooks, notes, groups, misplaced)
	}
}
