// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package db_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	notesdb "git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

func beforeWriteContract(t *testing.T) fs.FS {
	t.Helper()
	files := fstest.MapFS{}
	if err := fs.WalkDir(notesdb.Migrations, notesdb.MigrationsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || strings.Contains(path, "/0017_") {
			return err
		}
		data, err := fs.ReadFile(notesdb.Migrations, path)
		if err == nil {
			files[path] = &fstest.MapFile{Data: data}
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func writerContext(t *testing.T) context.Context {
	t.Helper()
	return tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"})
}

func assertWriteFence(t *testing.T, err error) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55000" || pgErr.ConstraintName != notesdb.WriteContractConstraint {
		t.Fatalf("old/unsupported writer was not fenced: %v", err)
	}
}

func TestAlreadyConnectedOldWriterCannotMutateAfterMigration(t *testing.T) {
	h := pgtest.New(t, beforeWriteContract(t), notesdb.MigrationsDir)
	ctx := writerContext(t)
	old, err := h.Pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	var notebook, note uuid.UUID
	tx, err := old.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO notebooks(tenant,owner_sub,title,encrypted,indexable)
		VALUES('acme','ada','Original',true,false) RETURNING id`).Scan(&notebook); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO notes(tenant,notebook_id,owner_sub,content_cipher,data)
		VALUES('acme',$1,'ada','encv1:original','{"attachments":[{"dataUrl":"legacy-original"}]}') RETURNING id`, notebook).Scan(&note); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// Keep the same physical connection open while the new schema is installed.
	if err := pg.MigrateUp(notesdb.Migrations, notesdb.MigrationsDir, h.DSN); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, sql string }{
		{"insert notebook", `INSERT INTO notebooks(tenant,owner_sub,title) VALUES('acme','ada','old')`},
		{"insert note", `INSERT INTO notes(tenant,notebook_id,owner_sub,title) VALUES('acme',$1,'ada','old')`},
		{"overwrite cipher while preserving floor", `UPDATE notes SET content_cipher='encv1:old',min_write_version=min_write_version WHERE id=$2`},
		{"plaintext data", `UPDATE notes SET data='{"attachments":[{"dataUrl":"leak"}]}' WHERE id=$2`},
		{"metadata", `UPDATE notes SET x=42,updated_at=now() WHERE id=$2`},
		{"trash", `UPDATE notes SET deleted_at=now() WHERE id=$2`},
		{"purge", `DELETE FROM notes WHERE id=$2`},
		{"notebook metadata", `UPDATE notebooks SET title='old' WHERE id=$1`},
		{"downgrade encrypted flag", `UPDATE notebooks SET encrypted=false,crypto='{}' WHERE id=$1`},
		{"delete notebook cascade", `DELETE FROM notebooks WHERE id=$1`},
		{"recurrence copy", `INSERT INTO notes(tenant,notebook_id,owner_sub,content_cipher,data)
			SELECT tenant,notebook_id,owner_sub,content_cipher,data FROM notes WHERE id=$2`},
		{"merge flag is no bypass", `UPDATE notes SET tenant='other' WHERE id=$2`},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx, err := old.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
				t.Fatal(err)
			}
			if err := pg.SetMaintenance(ctx, tx); err != nil {
				t.Fatal(err)
			}
			if err := pg.SetMerge(ctx, tx); err != nil {
				t.Fatal(err)
			}
			// NamedArgs avoids unused positional placeholders in these different SQL shapes.
			sql := strings.ReplaceAll(strings.ReplaceAll(test.sql, "$1", "@book"), "$2", "@note")
			_, err = tx.Exec(ctx, sql, pgx.NamedArgs{"book": notebook, "note": note})
			assertWriteFence(t, err)
		})
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var title, cipher, data string
		var bookFloor, noteFloor int
		if err := tx.QueryRow(ctx, `SELECT nb.title,n.content_cipher,n.data::text,nb.min_write_version,n.min_write_version
			FROM notebooks nb JOIN notes n ON n.notebook_id=nb.id WHERE n.id=$1`, note).
			Scan(&title, &cipher, &data, &bookFloor, &noteFloor); err != nil {
			return err
		}
		if title != "Original" || cipher != "encv1:original" || !strings.Contains(data, "legacy-original") || bookFloor != 2 || noteFloor != 2 {
			t.Fatalf("old writer changed the original or migration did not cover existing rows: %q %q %q %d %d", title, cipher, data, bookFloor, noteFloor)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestWriterMarkDoesNotLeakThroughPoolCommitOrRollback(t *testing.T) {
	h := pgtest.New(t, notesdb.Migrations, notesdb.MigrationsDir)
	ctx := writerContext(t)
	pool, err := pg.Connect(ctx, pg.Config{DSN: h.AppDSN, MaxConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	book, err := store.New(pool).CreateNotebook(ctx, "ada", domain.NotebookInput{Title: "Pool", Visibility: domain.VisPersonal, Indexable: boolPointer(true), Color: "#86198f", Layout: domain.LayoutTree})
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("fixture rollback")
	for _, abort := range []bool{false, true} {
		err := pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if err := notesdb.MarkWriteTransaction(ctx, tx); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE notebooks SET title='modern' WHERE id=$1`, book.ID); err != nil {
				return err
			}
			if abort {
				return rollback
			}
			return nil
		})
		if (abort && !errors.Is(err, rollback)) || (!abort && err != nil) {
			t.Fatal(err)
		}
		var empty bool
		assertWriteFence(t, pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if err := tx.QueryRow(ctx, `SELECT NULLIF(current_setting('app.notes_write_version',true),'') IS NULL`).Scan(&empty); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE notebooks SET title='old' WHERE id=$1`, book.ID)
			return err
		}))
		if !empty {
			t.Fatal("previous writer mark leaked through the one-connection pool")
		}
	}
}

func boolPointer(value bool) *bool { return &value }

func TestWriteFloorIsMonotonicAndMalformedMarkersFailClosed(t *testing.T) {
	h := pgtest.New(t, notesdb.Migrations, notesdb.MigrationsDir)
	ctx := writerContext(t)
	st := store.New(h.Pool)
	in := domain.NotebookInput{Title: "Floor", Visibility: domain.VisPersonal, Indexable: boolPointer(true), Color: "#86198f", Layout: domain.LayoutTree}
	book, err := st.CreateNotebook(ctx, "ada", in)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"", "1", "no", "-2", "2147483648"} {
		assertWriteFence(t, h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config($1,$2,true)`, notesdb.WriteVersionSetting, marker); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE notebooks SET title='old' WHERE id=$1`, book.ID)
			return err
		}))
	}
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := notesdb.MarkWriteTransaction(ctx, tx); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE notebooks SET min_write_version=1 WHERE id=$1`, book.ID)
		return err
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || (pgErr.Code != "55000" && pgErr.Code != "23514") {
		t.Fatalf("the floor could be lowered below its minimum: %v", err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config($1,'3',true)`, notesdb.WriteVersionSetting); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE notebooks SET min_write_version=3 WHERE id=$1`, book.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	assertWriteFence(t, h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config($1,'3',true)`, notesdb.WriteVersionSetting); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE notebooks SET min_write_version=2 WHERE id=$1`, book.ID)
		return err
	}))
	if _, err := st.UpdateNotebook(ctx, book.ID, "ada", in); !errors.Is(err, domain.ErrBackendWriteUpgrade) {
		t.Fatalf("current binary silently adopted a future DB capability: %v", err)
	}
}

func TestWriteContractCannotBeRemovedByOrdinaryDownMigration(t *testing.T) {
	h := pgtest.New(t, notesdb.Migrations, notesdb.MigrationsDir)
	migrator, err := pg.NewMigrator(notesdb.Migrations, notesdb.MigrationsDir, h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	if err := migrator.Steps(-1); err == nil || !strings.Contains(err.Error(), "cannot be rolled back safely") {
		t.Fatalf("a schema rollback could remove the fence: %v", err)
	}
	assertWriteFence(t, h.Pool.InTenantTx(writerContext(t), func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO notebooks(tenant,owner_sub,title) VALUES('acme','ada','old')`)
		return err
	}))
}

func TestWriteContractAlsoFiresForReplicationMode(t *testing.T) {
	h := pgtest.New(t, notesdb.Migrations, notesdb.MigrationsDir)
	ctx := writerContext(t)
	admin := h.PoolSinRLS(t)
	assertWriteFence(t, admin.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role='replica'`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO notebooks(tenant,owner_sub,title) VALUES('acme','ada','old')`)
		return err
	}))
}
