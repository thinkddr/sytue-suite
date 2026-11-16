// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

func TestPrivateStorageSharedFenceUpgradesExistingFrozenSource(t *testing.T) {
	old := fstest.MapFS{}
	entries, err := fs.ReadDir(db.Migrations, db.MigrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), "0037_") {
			continue
		}
		path := db.MigrationsDir + "/" + entry.Name()
		body, err := fs.ReadFile(db.Migrations, path)
		if err != nil {
			t.Fatal(err)
		}
		old[path] = &fstest.MapFile{Data: body}
	}
	h := pgtest.New(t, old, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := store.WithOwner(tenantCtx("upgrade"), "alice")
	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatal(err)
	}
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	file := seedFile(t, ctx, st, root.ID, "before shared fence", 7)
	epoch := uuid.New()
	before, err := st.BeginPrivateStorageMigration(ctx, epoch)
	if err != nil {
		t.Fatal(err)
	}
	pageBefore, err := st.PrivateStorageSourcePage(ctx, epoch, store.PrivateSourceCursor{}, 50)
	if err != nil {
		t.Fatal(err)
	}
	rawBefore, _ := json.Marshal(pageBefore)
	if err := pg.MigrateUp(db.Migrations, db.MigrationsDir, h.DSN); err != nil {
		t.Fatal(err)
	}
	var completionConstraints int
	if err := h.Pool.QueryRow(ctx, `SELECT count(*) FROM pg_trigger WHERE tgrelid='drive_private_storage_states'::regclass AND tgname IN ('drive_private_storage_build_completed','drive_private_storage_shared_build_completed') AND tgdeferrable AND tginitdeferred`).Scan(&completionConstraints); err != nil || completionConstraints != 2 {
		t.Fatal("upgrade did not retain the published completion constraint", completionConstraints, err)
	}
	after, err := st.PrivateStorageState(ctx)
	if err != nil || before.Epoch != after.Epoch || before.Rows != after.Rows || before.Blobs != after.Blobs {
		t.Fatal("upgrade changed source state", after, err)
	}
	pageAfter, err := st.PrivateStorageSourcePage(ctx, epoch, store.PrivateSourceCursor{}, 50)
	rawAfter, _ := json.Marshal(pageAfter)
	if err != nil || !bytes.Equal(rawBefore, rawAfter) {
		t.Fatal("upgrade changed frozen source bytes", err)
	}
	if _, err := st.CreateFile(ctx, store.NewFileRecord{FolderID: root.ID, Name: file.Name, StorageKey: "overwrite", SizeBytes: 1}); !errors.Is(store.PrivateStorageError(err), domain.ErrPrivateStorageFrozen) {
		t.Fatal("upgraded guard admitted write", err)
	}
	var definition string
	if err := h.Pool.Pool.QueryRow(context.Background(), `SELECT pg_get_functiondef('drive_private_storage_legacy_guard(text,text)'::regprocedure)`).Scan(&definition); err != nil || !strings.Contains(definition, "sytue_private_storage_guard") {
		t.Fatal("domain guard did not use shared implementation", err)
	}
}

func TestPrivateStorageSharedFenceRejectsOldSnapshotWithInitiallyMissingState(t *testing.T) {
	h, st, ctx, _, file := privateDriveFixture(t)
	if err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM drive_private_storage_states WHERE tenant='acme' AND owner_sub='alice'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	tx, err := h.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant','acme',true),set_config('app.owner_sub','alice',true)`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM files WHERE tenant='acme'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, uuid.New()); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `UPDATE files SET name='escaped old missing state' WHERE tenant='acme' AND id=$1`, file.ID)
	var failure *pgconn.PgError
	if !errors.As(err, &failure) || failure.Code != "40001" && failure.Code != "PST01" {
		t.Fatal("old snapshot missed newly inserted fence", err)
	}
}

func TestPrivateStorageSharedGuardActuallyHoldsTheStateRow(t *testing.T) {
	h, _, ctx, _, _ := privateDriveFixture(t)
	writer, err := h.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Rollback(ctx) }()
	if _, err := writer.Exec(ctx, `SELECT set_config('app.tenant','acme',true),set_config('app.owner_sub','alice',true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(ctx, `SELECT sytue_private_storage_guard('public.drive_private_storage_states'::regclass,'acme','alice')`); err != nil {
		t.Fatal(err)
	}
	blocked := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub','alice',true),set_config('lock_timeout','100ms',true)`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `SELECT mode FROM drive_private_storage_states WHERE tenant='acme' AND owner_sub='alice' FOR UPDATE`)
		return err
	})
	var failure *pgconn.PgError
	if !errors.As(blocked, &failure) || failure.Code != "55P03" {
		t.Fatal("shared guard did not lock state", blocked)
	}
}

func TestPrivateStorageSharedGuardDoesNotRecreateMergedTenant(t *testing.T) {
	h, _, ctx, _, _ := privateDriveFixture(t)
	err := h.Pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		for _, query := range []string{`UPDATE drive_private_storage_states SET tenant='merged' WHERE tenant='acme'`, `UPDATE files SET tenant='merged' WHERE tenant='acme'`, `UPDATE folders SET tenant='merged' WHERE tenant='acme'`} {
			if _, err := tx.Exec(ctx, query); err != nil {
				return err
			}
		}
		var old int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM drive_private_storage_states WHERE tenant='acme'`).Scan(&old); err != nil {
			return err
		}
		if old != 0 {
			return fmt.Errorf("merge recreated an old tenant state: %d", old)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPrivateStorageSharedGuardRejectsUnregisteredTables(t *testing.T) {
	h, _, ctx, _, _ := privateDriveFixture(t)
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `SELECT sytue_private_storage_guard('public.files'::regclass,'acme','alice')`)
		return err
	})
	var failure *pgconn.PgError
	if !errors.As(err, &failure) || failure.Code != "22023" {
		t.Fatal("unregistered state table accepted", err)
	}
}

func TestPrivateStorageSharedRoutingSeesFrozenOwnerWithoutOpeningStateRLS(t *testing.T) {
	h, st, ctx, _, _ := privateDriveFixture(t)
	if _, err := st.BeginPrivateStorageMigration(ctx, uuid.New()); err != nil {
		t.Fatal(err)
	}
	bob := store.WithOwner(tenantCtx("acme"), "bob")
	if _, err := st.EnsureRoot(bob); err != nil {
		t.Fatal(err)
	}
	err := h.Pool.InTenantTx(tenantCtx("acme"), func(tx pgx.Tx) error {
		var visible int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM drive_private_storage_states WHERE tenant='acme'`).Scan(&visible); err != nil {
			return err
		}
		if visible != 0 {
			return fmt.Errorf("owner-less maintenance query could see states: %d", visible)
		}
		var alice, bob, other bool
		if err := tx.QueryRow(ctx, `SELECT sytue_private_storage_frozen('public.drive_private_storage_states'::regclass,'acme','alice'),sytue_private_storage_frozen('public.drive_private_storage_states'::regclass,'acme','bob'),sytue_private_storage_frozen('public.drive_private_storage_states'::regclass,'other','alice')`).Scan(&alice, &bob, &other); err != nil {
			return err
		}
		if !alice || bob || other {
			return fmt.Errorf("routing predicate crossed scope or missed frozen owner: %v %v %v", alice, bob, other)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPrivateStorageSharedStateGuardKeepsFrozenInventoryIdentity(t *testing.T) {
	h, st, ctx, _, _ := privateDriveFixture(t)
	if _, err := st.BeginPrivateStorageMigration(ctx, uuid.New()); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`UPDATE drive_private_storage_states SET source_rows=source_rows+1 WHERE tenant='acme' AND owner_sub='alice'`, `UPDATE drive_private_storage_states SET started_at=clock_timestamp() WHERE tenant='acme' AND owner_sub='alice'`, `UPDATE drive_private_storage_states SET mode='building' WHERE tenant='acme' AND owner_sub='alice'`} {
		err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, query); return err })
		if !errors.Is(store.PrivateStorageError(err), domain.ErrPrivateStorageFrozen) {
			t.Fatal("rewrote frozen inventory identity", query, err)
		}
	}
}
