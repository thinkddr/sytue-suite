// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

func privateDriveFixture(t *testing.T) (*pgtest.Harness, *store.Store, context.Context, domain.Folder, domain.File) {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := store.WithOwner(tenantCtx("acme"), "alice")
	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatal(err)
	}
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	file := seedFile(t, ctx, st, root.ID, "private source.txt", 7)
	return h, st, ctx, root, file
}

func privateRows(ctx context.Context, t *testing.T, st *store.Store, epoch uuid.UUID, limit int) []storage.LegacySourceRow {
	t.Helper()
	cursor := store.PrivateSourceCursor{}
	rows := []storage.LegacySourceRow{}
	seen := map[store.PrivateSourceCursor]bool{}
	for {
		page, err := st.PrivateStorageSourcePage(ctx, epoch, cursor, limit)
		if err != nil {
			t.Fatal(err)
		}
		if err := page.Validate(storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}, epoch.String()); err != nil {
			t.Fatal("invalid exported frame", err)
		}
		rows = append(rows, page.Rows...)
		if page.Next == nil {
			return rows
		}
		if seen[*page.Next] {
			t.Fatal("source cursor loop")
		}
		seen[*page.Next] = true
		cursor = *page.Next
	}
}

func TestPrivateStorageSnapshotPreservesHierarchyVersionsTrashRelationshipsAndExactJSON(t *testing.T) {
	_, st, ctx, root, file := privateDriveFixture(t)
	folder, err := st.CreateFolder(ctx, root.ID, "private folder", "alice", false)
	if err != nil {
		t.Fatal(err)
	}
	trashed := seedFile(t, ctx, st, folder.ID, "retained trash.txt", 3)
	if _, err := st.DeleteFile(ctx, trashed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFile(ctx, store.NewFileRecord{FolderID: root.ID, Name: file.Name, StorageKey: "files/private-new", SizeBytes: 9, Checksum: strings.Repeat("a", 64)}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateShare(ctx, store.NewShareRecord{FileID: &file.ID, Kind: domain.ShareKind("user"), Subject: "bob", Permission: domain.PermRead, CreatedBy: "alice"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetFavorite(ctx, "alice", &file.ID, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PedirEdicion(ctx, file.ID, peticiones.Peticion{Sujeto: "bob", Nombre: "private requester"}); err != nil {
		t.Fatal(err)
	}
	if err := st.GuardarProcedencia(ctx, file.ID, 2, "sytue-bases", json.RawMessage(`{"integer":900719925474099512345}`), "alice"); err != nil {
		t.Fatal(err)
	}
	epoch := uuid.New()
	state, err := st.BeginPrivateStorageMigration(ctx, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if state.Mode != "frozen" || state.Epoch != epoch.String() || state.Rows != 9 || state.Blobs != 3 {
		t.Fatal("incomplete source snapshot", state, err)
	}
	if err := state.Validate(storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}); err != nil {
		t.Fatal(err)
	}
	rows := privateRows(ctx, t, st, epoch, 2)
	kinds := map[string]int{}
	for _, row := range rows {
		kinds[row.Kind]++
		if bytes.Contains(row.Payload, []byte("storage_key")) || bytes.Contains(row.Payload, []byte("files/private-new")) {
			t.Fatal("internal provider routing leaked into export")
		}
		if row.Kind == "provenance" && !bytes.Contains(row.Payload, []byte("900719925474099512345")) {
			t.Fatal("domain integer rounded")
		}
	}
	if len(kinds) != 7 || len(rows) != 9 {
		t.Fatal("missing source collection", kinds)
	}
	again, err := st.BeginPrivateStorageMigration(ctx, epoch)
	if err != nil || again.Rows != state.Rows || again.Epoch != state.Epoch {
		t.Fatal("retry replaced snapshot", err)
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, uuid.New()); !errors.Is(err, domain.ErrPrivateStorageEpoch) {
		t.Fatal("another epoch replaced source", err)
	}
	blob, err := st.PrivateStorageSourceBlob(ctx, epoch, "file", file.ID.String())
	if err != nil || blob.StorageKey != "files/private-new" || blob.SizeBytes != 9 {
		t.Fatal("source blob changed", err)
	}
	if _, err := st.GetFile(ctx, file.ID); err != nil {
		t.Fatal("original removed before verification", err)
	}
	for _, other := range []context.Context{store.WithOwner(tenantCtx("acme"), "bob"), store.WithOwner(tenantCtx("other"), "alice")} {
		if _, err := st.PrivateStorageSourcePage(other, epoch, store.PrivateSourceCursor{}, 1); !errors.Is(err, domain.ErrPrivateStorageEpoch) {
			t.Fatal("snapshot crossed identity", err)
		}
	}
}

func TestPrivateStorageFencesLegacyMetadataWithoutBlockingCiphertextOrOtherOwners(t *testing.T) {
	h, st, ctx, root, file := privateDriveFixture(t)
	epoch := uuid.New()
	if _, err := st.BeginPrivateStorageMigration(ctx, epoch); err != nil {
		t.Fatal(err)
	}
	mutations := []func() error{
		func() error { _, err := st.RenameFile(ctx, file.ID, "changed.txt"); return err },
		func() error { _, err := st.CreateFolder(ctx, root.ID, "changed", "alice", false); return err },
		func() error { _, err := st.DeleteFile(ctx, file.ID); return err },
		func() error {
			_, err := st.CreateFile(ctx, store.NewFileRecord{FolderID: root.ID, Name: file.Name, StorageKey: "files/changed", SizeBytes: 3})
			return err
		},
		func() error { return st.SetFavorite(ctx, "alice", &file.ID, nil, true) },
		func() error {
			_, err := st.CreateShare(ctx, store.NewShareRecord{FileID: &file.ID, Kind: domain.ShareKind("user"), Subject: "bob", Permission: domain.PermRead, CreatedBy: "alice"})
			return err
		},
		func() error {
			return st.GuardarProcedencia(ctx, file.ID, 1, "sytue-bases", json.RawMessage(`{}`), "alice")
		},
		func() error { _, err := st.PedirEdicion(ctx, file.ID, peticiones.Peticion{Sujeto: "bob"}); return err },
	}
	for i, change := range mutations {
		if err := store.PrivateStorageError(change()); !errors.Is(err, domain.ErrPrivateStorageFrozen) {
			t.Fatalf("legacy writer %d bypassed frozen scope: %v", i, err)
		}
	}
	if err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE files SET owner_sub='bob' WHERE tenant='acme' AND id=$1`, file.ID)
		return err
	}); !errors.Is(store.PrivateStorageError(err), domain.ErrPrivateStorageFrozen) {
		t.Fatal("owner transfer escaped fence", err)
	}
	bob := store.WithOwner(tenantCtx("acme"), "bob")
	bobRoot, err := st.EnsureRoot(bob)
	if err != nil {
		t.Fatal("unrelated owner blocked", err)
	}
	seedFile(t, bob, st, bobRoot.ID, "other private source.txt", 4)
	blob := domain.CipherBlob{BlobReceipt: storage.BlobReceipt{ID: strings.Repeat("A", 26), Product: "drive", SizeBytes: 2, SHA256: strings.Repeat("a", 64)}, OwnerSub: "alice", StorageKey: "cipher/migration-fixture"}
	if err := st.ReservarSubida(ctx, blob.StorageKey, blob.SizeBytes, time.Now().Add(time.Minute), true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateCipherBlob(ctx, blob); err != nil {
		t.Fatal("ciphertext path frozen with legacy writes", err)
	}
}

func TestPrivateStorageSnapshotIsImmutableAndRLSDoesNotLeakThroughTriggerContext(t *testing.T) {
	h, st, ctx, _, _ := privateDriveFixture(t)
	epoch := uuid.New()
	if _, err := st.BeginPrivateStorageMigration(ctx, epoch); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub','alice',true)`); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE drive_private_storage_snapshot SET payload='{}'::bytea WHERE tenant='acme'`)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 0 {
			return fmt.Errorf("snapshot mutated")
		}
		tag, err = tx.Exec(ctx, `DELETE FROM drive_private_storage_snapshot WHERE tenant='acme'`)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 0 {
			return fmt.Errorf("snapshot discarded before verification")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"bob", ""} {
		if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true)`, owner); err != nil {
				return err
			}
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM drive_private_storage_snapshot`).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return fmt.Errorf("snapshot RLS leaked %d rows", count)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	bob := store.WithOwner(tenantCtx("acme"), "bob")
	if _, err := st.EnsureRoot(bob); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(bob, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub','bob',true)`); err != nil {
			return err
		}
		var count int
		var maintenance *string
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM folders WHERE tenant='acme'`).Scan(&count); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT current_setting('app.maintenance',true)`).Scan(&maintenance); err != nil {
			return err
		}
		if count != 1 || (maintenance != nil && *maintenance == "on") {
			return fmt.Errorf("trigger retained elevated RLS context")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateStorageBeginWaitsForLegacyWriterAndCapturesItsCommit(t *testing.T) {
	h, st, ctx, _, file := privateDriveFixture(t)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	writer := make(chan error, 1)
	go func() {
		writer <- h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub','alice',true)`); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE files SET name='committed before freeze.txt' WHERE tenant='acme' AND id=$1`, file.ID); err != nil {
				return err
			}
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-entered:
	case err := <-writer:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	frozen := make(chan error, 1)
	epoch := uuid.New()
	go func() { _, err := st.BeginPrivateStorageMigration(ctx, epoch); frozen <- err }()
	select {
	case err := <-frozen:
		close(release)
		t.Fatal("begin skipped active writer", err)
	case <-time.After(75 * time.Millisecond):
	}
	close(release)
	if err := <-writer; err != nil {
		t.Fatal(err)
	}
	if err := <-frozen; err != nil {
		t.Fatal(err)
	}
	rows := privateRows(ctx, t, st, epoch, 1)
	found := false
	for _, row := range rows {
		if row.Kind == "file" && bytes.Contains(row.Payload, []byte("committed before freeze.txt")) {
			found = true
		}
	}
	if !found {
		t.Fatal("committed source edit was omitted")
	}
}

func TestPrivateStorageOldRepeatableReadCannotWritePastFreeze(t *testing.T) {
	h, st, ctx, _, file := privateDriveFixture(t)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- h.Pool.InTxOpts(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead}, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant','acme',true),set_config('app.owner_sub','alice',true)`); err != nil {
				return err
			}
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM files WHERE tenant='acme'`).Scan(&count); err != nil {
				return err
			}
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			_, err := tx.Exec(ctx, `UPDATE files SET name='must not commit' WHERE tenant='acme' AND id=$1`, file.ID)
			return err
		})
	}()
	select {
	case <-entered:
	case err := <-result:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, uuid.New()); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	err := <-result
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || (pgErr.Code != "40001" && pgErr.Code != "PST01") {
		t.Fatal("old transaction bypassed freeze", err)
	}
}

func TestPrivateStorageRetainsFrozenTrashAndForeignFavoriteDuringOtherOwnerGC(t *testing.T) {
	_, st, ctx, _, file := privateDriveFixture(t)
	bob := store.WithOwner(tenantCtx("acme"), "bob")
	bobRoot, err := st.EnsureRoot(bob)
	if err != nil {
		t.Fatal(err)
	}
	foreign := seedFile(t, bob, st, bobRoot.ID, "unrelated expired.txt", 3)
	if _, err := st.CreateShare(bob, store.NewShareRecord{FileID: &foreign.ID, Kind: domain.ShareKind("user"), Subject: "alice", Permission: domain.PermRead, CreatedBy: "bob"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetFavorite(bob, "alice", &foreign.ID, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteFile(ctx, file.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteFile(bob, foreign.ID); err != nil {
		t.Fatal(err)
	}
	epoch := uuid.New()
	if _, err := st.BeginPrivateStorageMigration(ctx, epoch); err != nil {
		t.Fatal(err)
	}
	count, _, err := st.ClaimAndPurgeExpiredTrash(ctx, time.Now().Add(time.Minute), 20)
	if err != nil || count != 1 {
		t.Fatal("frozen owner blocked unrelated GC or lost its source", count, err)
	}
	if _, err := st.GetFile(bob, foreign.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("unrelated GC did not finish", err)
	}
	rows := privateRows(ctx, t, st, epoch, 1)
	kinds := map[string]int{}
	for _, row := range rows {
		kinds[row.Kind]++
	}
	if kinds["file"] != 1 || kinds["favorite"] != 1 {
		t.Fatal("snapshot lost frozen trash or cascaded relationship", kinds)
	}
	source, err := st.PrivateStorageSourceBlob(ctx, epoch, "file", file.ID.String())
	if err != nil || source.StorageKey == "" {
		t.Fatal("frozen original lost routing", err)
	}
}

func TestPrivateStorageCaptureFailureRollsBackFenceAndPartialSnapshot(t *testing.T) {
	h, st, ctx, _, file := privateDriveFixture(t)
	payload, _ := json.Marshal(map[string]string{"oversized": strings.Repeat("x", storage.MaxLegacySourceBytes)})
	if err := st.GuardarProcedencia(ctx, file.ID, 1, "sytue-bases", payload, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, uuid.New()); err == nil {
		t.Fatal("oversized metadata snapshot committed")
	}
	state, err := st.PrivateStorageState(ctx)
	if err != nil || state.Mode != "legacy" {
		t.Fatal("failed capture left owner frozen", state, err)
	}
	if _, err := st.RenameFile(ctx, file.ID, "still usable.txt"); err != nil {
		t.Fatal("failed migration disabled legacy data", err)
	}
	if err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		var rows int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM drive_private_storage_snapshot WHERE tenant='acme'`).Scan(&rows); err != nil {
			return err
		}
		if rows != 0 {
			return fmt.Errorf("partial snapshot escaped rollback")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateStorageCannotCommitUnfinishedBuild(t *testing.T) {
	h, _, ctx, _, _ := privateDriveFixture(t)
	err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE drive_private_storage_states SET mode='building',epoch=$1,started_at=clock_timestamp() WHERE tenant='acme' AND owner_sub='alice'`, uuid.New())
		return err
	})
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != "PST02" {
		t.Fatal("unfinished source snapshot committed", err)
	}
}
