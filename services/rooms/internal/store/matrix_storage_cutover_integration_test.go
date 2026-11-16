// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestMatrixStorageCutoverSurvivesRestartAndNeverReopensLegacy(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	epoch := "df75d053-b8c8-4eb3-8454-235ef9c532c4"
	provision := func(_ context.Context, room string) (string, error) {
		if room == "" {
			return "!legacy:matrix.test", nil
		}
		return room, nil
	}
	if _, err := st.EnsureMatrixStorageRooms(ctx, "alice", "drive", provision); err != nil {
		t.Fatal(err)
	}
	state, err := st.BeginMatrixStorageCutover(ctx, "alice", "drive", epoch)
	if err != nil || state.Phase != "freezing" || !reflect.DeepEqual(state.SourceRooms, []string{"!legacy:matrix.test"}) {
		t.Fatal("durable begin", state, err)
	}
	if _, err := st.FreezeMatrixStorageCutover(ctx, "alice", "drive", epoch, func(context.Context, string) error { return fmt.Errorf("synthetic remote policy failure") }); err == nil {
		t.Fatal("lost remote failure")
	}
	restarted := store.New(h.Pool, nil)
	state, err = restarted.MatrixStorageCutover(ctx, "alice", "drive")
	if err != nil || state.Phase != "freezing" {
		t.Fatal("restart lost fence", state, err)
	}
	calls := 0
	if _, err := restarted.EnsureMatrixStorageRooms(ctx, "alice", "drive", func(context.Context, string) (string, error) { calls++; return "!legacy:matrix.test", nil }); !errors.Is(err, domain.ErrConflict) || calls != 0 {
		t.Fatal("old callback reopened source", calls, err)
	}
	readOnly := false
	rooms, err := restarted.EnsureMatrixStorageRoomsWithPolicy(ctx, "alice", "drive", storage.LegacyIdentity, func(_ context.Context, room string, frozen bool) (string, error) { readOnly = frozen; return room, nil })
	if err != nil || !readOnly || len(rooms) != 1 {
		t.Fatal("bootstrap did not preserve read-only", readOnly, err)
	}
	freeze := func(_ context.Context, room string) error {
		if room != "!legacy:matrix.test" {
			return fmt.Errorf("foreign source")
		}
		calls++
		return nil
	}
	wrongCalls := 0
	if _, err := restarted.FreezeMatrixStorageCutover(ctx, "alice", "drive", "ad75d053-b8c8-4eb3-8454-235ef9c532c4", func(context.Context, string) error { wrongCalls++; return nil }); !errors.Is(err, domain.ErrConflict) || wrongCalls != 0 {
		t.Fatal("wrong epoch reached remote policy", wrongCalls, err)
	}
	state, err = restarted.FreezeMatrixStorageCutover(ctx, "alice", "drive", epoch, freeze)
	if err != nil || state.Phase != "frozen" {
		t.Fatal("resume freeze", state, err)
	}
	if _, err := restarted.EnsureMatrixStorageRoomsForIdentity(ctx, "alice", "drive", storage.IsolatedIdentity, func(context.Context, string) (string, error) { return "!isolated:matrix.test", nil }); err != nil {
		t.Fatal(err)
	}
	receipt := storage.Reference{RoomID: "!isolated:matrix.test", EventID: "$receipt"}
	validated := 0
	validate := func(context.Context) error { validated++; return nil }
	if _, err := restarted.CompleteMatrixStorageCutover(ctx, "alice", "drive", epoch, storage.Reference{RoomID: "!foreign:matrix.test", EventID: "$receipt"}, freeze, validate); !errors.Is(err, domain.ErrForbidden) || validated != 0 {
		t.Fatal("foreign destination validated", err)
	}
	if _, err := restarted.CompleteMatrixStorageCutover(ctx, "alice", "drive", epoch, receipt, freeze, func(context.Context) error { return domain.ErrForbidden }); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("invalid sender accepted", err)
	}
	state, err = restarted.MatrixStorageCutover(ctx, "alice", "drive")
	if err != nil || state.Phase != "frozen" {
		t.Fatal("failed receipt changed routing", state, err)
	}
	state, err = restarted.CompleteMatrixStorageCutover(ctx, "alice", "drive", epoch, receipt, freeze, validate)
	if err != nil || state.Phase != "isolated" || state.Receipt == nil || *state.Receipt != receipt || validated != 1 {
		t.Fatal("complete", state, err)
	}
	before := calls
	if _, err = restarted.CompleteMatrixStorageCutover(ctx, "alice", "drive", epoch, receipt, freeze, validate); err != nil || calls != before || validated != 1 {
		t.Fatal("receipt retry changed operation", err)
	}
	if _, err = restarted.CompleteMatrixStorageCutover(ctx, "alice", "drive", epoch, storage.Reference{RoomID: receipt.RoomID, EventID: "$different"}, freeze, validate); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("receipt replaced", err)
	}
	if _, err = restarted.BeginMatrixStorageCutover(ctx, "alice", "drive", "ad75d053-b8c8-4eb3-8454-235ef9c532c4"); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("epoch replaced", err)
	}
	for _, scope := range []storage.Scope{{Tenant: "acme", Subject: "bob", Product: "drive"}, {Tenant: "other", Subject: "alice", Product: "drive"}, {Tenant: "acme", Subject: "alice", Product: "mail"}} {
		own, err := restarted.MatrixStorageCutover(tenantCtx(scope.Tenant), scope.Subject, scope.Product)
		if err != nil || own.Phase != "legacy" || len(own.SourceRooms) != 0 {
			t.Fatal("foreign actor/tenant/product saw migration", scope, own, err)
		}
	}
	if err := h.Pool.InTenantTx(tenantCtx("other"), func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_cutovers`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("RLS leaked %d cutovers", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`UPDATE matrix_storage_cutovers SET phase='freezing',receipt_room=NULL,receipt_event=NULL`, `UPDATE matrix_storage_cutovers SET tenant='other'`, `DELETE FROM matrix_storage_cutovers`} {
		if err := h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, query); return err }); err == nil {
			t.Fatal("raw SQL reopened or removed durable fence", query)
		}
	}
	raw, err := fs.ReadFile(db.Migrations, db.MigrationsDir+"/0066_matrix_storage_cutover.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.Connect(t.Context(), h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	if _, err = admin.Exec(t.Context(), string(raw)); err == nil {
		t.Fatal("downgrade removed active cutover fence")
	}
	if _, err = admin.Exec(t.Context(), "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	retained, err := restarted.MatrixStorageCutover(ctx, "alice", "drive")
	if err != nil || retained.Phase != "isolated" {
		t.Fatal("downgrade changed retained source", retained, err)
	}
	if _, err := restarted.EnsureMatrixStorageRoomsWithPolicy(ctx, "alice", "drive", storage.LegacyIdentity, func(_ context.Context, room string, frozen bool) (string, error) {
		if !frozen {
			return "", fmt.Errorf("reopened after complete")
		}
		return room, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMatrixStorageCutoverEmptySourceCannotCreateNewLegacyRoom(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	epoch := "df75d053-b8c8-4eb3-8454-235ef9c532c4"
	if _, err := st.BeginMatrixStorageCutover(ctx, "alice", "notes", epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FreezeMatrixStorageCutover(ctx, "alice", "notes", epoch, func(context.Context, string) error { return fmt.Errorf("no source room should be created") }); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if _, err := st.EnsureMatrixStorageRoomsWithPolicy(ctx, "alice", "notes", storage.LegacyIdentity, func(context.Context, string, bool) (string, error) { calls++; return "!new-legacy:matrix.test", nil }); !errors.Is(err, domain.ErrConflict) || calls != 0 {
		t.Fatal("fence created writable legacy namespace", calls, err)
	}
}
