// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestMatrixStorageRoomsIsolateProductActorTenantAndRevoke(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	gate := &matrixGate{}
	st.SetMatrixGate(gate)
	ctx := tenantCtx("acme")
	created := 0
	provision := func(_ context.Context, room string) (string, error) {
		if room == "" {
			created++
			return fmt.Sprintf("!storage%d:matrix.test", created), nil
		}
		return room, nil
	}
	rooms := map[string]string{}
	for _, product := range []string{"drive", "mail", "cast"} {
		got, err := st.EnsureMatrixStorageRooms(ctx, "alice", product, provision)
		if err != nil || len(got) != 1 {
			t.Fatal(product, got, err)
		}
		rooms[product] = got[0]
		again, err := st.EnsureMatrixStorageRooms(ctx, "alice", product, provision)
		if err != nil || !slices.Equal(got, again) {
			t.Fatal("bootstrap is not idempotent", err)
		}
	}
	if created != 3 {
		t.Fatal("products reused one private room", created)
	}
	for _, actor := range []string{"bob", "carol"} {
		got, err := st.EnsureMatrixStorageRooms(ctx, actor, "drive", provision)
		if err != nil || len(got) != 1 || got[0] == rooms["drive"] {
			t.Fatal("actor isolation", err)
		}
	}
	other, err := st.EnsureMatrixStorageRooms(tenantCtx("other"), "alice", "drive", provision)
	if err != nil || len(other) != 1 || other[0] == rooms["drive"] {
		t.Fatal("tenant isolation", err)
	}
	if err := h.Pool.InTenantTx(tenantCtx("unrelated"), func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM matrix_storage_rooms`).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Fatal("RLS exposed another tenant's private routing", n)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnsureMatrixStorageRooms(ctx, "alice", "../drive", provision); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatal("invalid product provisioned", err)
	}
	if err := st.ApplyMatrixTenantAccess(ctx, "alice", "removed", 7, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	for _, room := range rooms {
		if !slices.Contains(gate.rooms, room) {
			t.Fatal("private storage not revoked", room)
		}
	}
	if slices.Contains(gate.rooms, other[0]) {
		t.Fatal("organization removal revoked another tenant")
	}
	for _, members := range gate.members {
		if len(members) != 0 {
			t.Fatal("revocation retained a reader")
		}
	}
	if _, err := st.EnsureMatrixStorageRooms(ctx, "alice", "drive", provision); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("removed actor re-provisioned storage", err)
	}
	if _, err := st.EnsureMatrixStorageRooms(tenantCtx("other"), "alice", "drive", provision); err != nil {
		t.Fatal("other tenant blocked", err)
	}
}
