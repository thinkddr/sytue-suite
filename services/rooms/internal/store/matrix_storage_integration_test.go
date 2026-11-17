// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
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

func TestMatrixStorageIdentityVersionsPreserveLegacyAndRevokeBoth(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	gate := &matrixGate{}
	st.SetMatrixGate(gate)
	ctx := tenantCtx("acme")
	calls := 0
	provision := func(_ context.Context, room string) (string, error) {
		if room == "" {
			calls++
			room = fmt.Sprintf("!isolated%d:matrix.test", calls)
		}
		return room, nil
	}
	for _, product := range storage.Products() {
		old, err := st.EnsureMatrixStorageRooms(ctx, "alice", product, provision)
		if err != nil {
			t.Fatal(err)
		}
		scoped, err := st.EnsureMatrixStorageRoomsForIdentity(ctx, "alice", product, storage.IsolatedIdentity, provision)
		if err != nil || len(scoped) != 1 || scoped[0] == old[0] {
			t.Fatal("isolated identity reused legacy room", product, err)
		}
		again, err := st.EnsureMatrixStorageRoomsForIdentity(ctx, "alice", product, storage.IsolatedIdentity, provision)
		if err != nil || !slices.Equal(scoped, again) {
			t.Fatal("isolated bootstrap lost idempotence", err)
		}
		legacy, err := st.LegacyMatrixStorageRooms(ctx, "alice", product)
		if err != nil || !slices.Equal(old, legacy) {
			t.Fatal("old history was hidden or overwritten", err)
		}
		alien, err := st.LegacyMatrixStorageRooms(ctx, "bob", product)
		if err != nil || len(alien) != 0 {
			t.Fatal("legacy migration exposed another owner", err)
		}
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_rooms WHERE tenant='acme' AND subject='alice'`).Scan(&n); err != nil {
			return err
		}
		if n != len(storage.Products()) {
			return fmt.Errorf("old server can select isolated routing: %d", n)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(tenantCtx("other"), func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_isolated_rooms`).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			return fmt.Errorf("isolated routing bypassed tenant RLS: %d", n)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 2*len(storage.Products()) {
		t.Fatal("unexpected reprovisioning", calls)
	}
	before := calls
	for _, v := range []storage.IdentityVersion{0, 3, -1} {
		if _, err := st.EnsureMatrixStorageRoomsForIdentity(ctx, "alice", "drive", v, provision); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatal("invalid version reached provision", err)
		}
	}
	if calls != before {
		t.Fatal("invalid request changed remote rooms")
	}
	if err := st.ApplyMatrixTenantAccess(ctx, "alice", "removed", 8, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if len(gate.rooms) != calls {
		t.Fatal("revocation missed an identity version", len(gate.rooms), calls)
	}
	if _, err := st.LegacyMatrixStorageRooms(ctx, "alice", "drive"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("denied actor read migration source", err)
	}
	if _, err := st.EnsureMatrixStorageRoomsForIdentity(ctx, "alice", "drive", storage.IsolatedIdentity, provision); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("removed actor reactivated isolated storage", err)
	}
}

func TestMatrixStorageIdentityRollbackRefusesRoutingOrPendingGrantLoss(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenantCtx("acme")
	st := store.New(h.Pool, testCipher(t))
	sql, err := fs.ReadFile(db.Migrations, db.MigrationsDir+"/0064_matrix_storage_identity.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.Connect(t.Context(), h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.WithoutCancel(t.Context())) })
	var database, original string
	if err := admin.QueryRow(t.Context(), `SELECT current_database(),current_user`).Scan(&database, &original); err != nil {
		t.Fatal(err)
	}
	owner := pgx.Identifier{"storage_rollback_" + database}.Sanitize()
	if _, err := admin.Exec(t.Context(), `CREATE ROLE `+owner+` NOLOGIN NOSUPERUSER NOBYPASSRLS`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup := context.WithoutCancel(t.Context())
		if _, err := admin.Exec(cleanup, `ROLLBACK; RESET ROLE; REASSIGN OWNED BY `+owner+` TO `+pgx.Identifier{original}.Sanitize()+`; DROP OWNED BY `+owner+`; DROP ROLE `+owner); err != nil {
			t.Error("rollback fixture cleanup", err)
		}
	})
	for _, table := range []string{"matrix_storage_isolated_rooms", "matrix_native_consents"} {
		if _, err := admin.Exec(t.Context(), `ALTER TABLE `+pgx.Identifier{table}.Sanitize()+` OWNER TO `+owner); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := admin.Exec(t.Context(), `GRANT SELECT ON matrix_storage_rooms TO `+owner); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(t.Context(), `SET ROLE `+owner); err != nil {
		t.Fatal(err)
	}
	checkRefused := func() {
		t.Helper()
		if _, err := admin.Exec(t.Context(), string(sql)); err == nil {
			t.Fatal("rollback discarded isolated storage")
		}
		if _, err := admin.Exec(t.Context(), "ROLLBACK"); err != nil {
			t.Fatal(err)
		}
	}
	state, err := st.CreateMatrixNativeConsentForIdentity(ctx, "alice", "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", "drive", storage.IsolatedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	checkRefused()
	if _, err := st.PreviewMatrixNativeDestination(ctx, "alice", state); err != nil {
		t.Fatal("failed rollback destroyed consent", err)
	}
	if _, err := admin.Exec(t.Context(), `BEGIN; SELECT set_config('app.merge','on',true); DELETE FROM matrix_native_consents; COMMIT;`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnsureMatrixStorageRoomsForIdentity(ctx, "alice", "drive", storage.IsolatedIdentity, func(context.Context, string) (string, error) { return "!kept:matrix.test", nil }); err != nil {
		t.Fatal(err)
	}
	checkRefused()
	if _, err := admin.Exec(t.Context(), `SELECT set_config('app.tenant','acme',false)`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := admin.QueryRow(t.Context(), `SELECT count(*) FROM matrix_storage_isolated_rooms`).Scan(&count); err != nil || count != 1 {
		t.Fatal("rollback hid new routing", err)
	}
	if _, err := admin.Exec(t.Context(), `BEGIN; SELECT set_config('app.maintenance','on',true); DELETE FROM matrix_storage_isolated_rooms; COMMIT;`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnsureMatrixStorageRooms(ctx, "alice", "drive", func(context.Context, string) (string, error) { return "!legacy:matrix.test", nil }); err != nil {
		t.Fatal(err)
	}
	// Even an empty isolated table cannot prove absence through newer restrictive
	// policies. The restricted owner must refuse; only the migration administrator
	// can inspect all tenants before a destructive rollback.
	checkRefused()
	if _, err := admin.Exec(t.Context(), `RESET ROLE`); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(t.Context(), string(sql)); err != nil {
		t.Fatal("legacy-only rollback failed", err)
	}
	if err := admin.QueryRow(t.Context(), `SELECT count(*) FROM matrix_storage_rooms WHERE room_id='!legacy:matrix.test'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("legacy routing was lost", err)
	}
}
