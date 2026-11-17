// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pg_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

const snapshotFixtureDDL = `
-- sytue:private-storage-fence-v1
CREATE TABLE calendar_private_storage_states (
 tenant text NOT NULL,owner_sub text NOT NULL,account_id uuid,
 mode text NOT NULL DEFAULT 'legacy' CHECK(mode IN ('legacy','building','frozen','private')),
 epoch uuid,started_at timestamptz,source_rows bigint NOT NULL DEFAULT 0 CHECK(source_rows>=0),source_blobs bigint NOT NULL DEFAULT 0 CHECK(source_blobs>=0),
 PRIMARY KEY(tenant,owner_sub)
);
CREATE TABLE snapshot_legacy_rows(tenant text NOT NULL,owner_sub text NOT NULL,id integer NOT NULL,data text NOT NULL,PRIMARY KEY(tenant,owner_sub,id));
CREATE TABLE snapshot_retained_rows(tenant text NOT NULL,owner_sub text NOT NULL,epoch uuid NOT NULL,id integer NOT NULL,data text NOT NULL,PRIMARY KEY(tenant,owner_sub,epoch,id),FOREIGN KEY(tenant,owner_sub) REFERENCES calendar_private_storage_states(tenant,owner_sub));
ALTER TABLE calendar_private_storage_states ENABLE ROW LEVEL SECURITY;ALTER TABLE calendar_private_storage_states FORCE ROW LEVEL SECURITY;
CREATE POLICY state_owner ON calendar_private_storage_states USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true)) WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
ALTER TABLE snapshot_legacy_rows ENABLE ROW LEVEL SECURITY;ALTER TABLE snapshot_legacy_rows FORCE ROW LEVEL SECURITY;
CREATE POLICY legacy_owner ON snapshot_legacy_rows USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true)) WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
ALTER TABLE snapshot_retained_rows ENABLE ROW LEVEL SECURITY;ALTER TABLE snapshot_retained_rows FORCE ROW LEVEL SECURITY;
CREATE POLICY retained_owner ON snapshot_retained_rows USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true)) WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE TRIGGER state_scope BEFORE UPDATE OR DELETE ON calendar_private_storage_states FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_state_scope_guard();
CREATE CONSTRAINT TRIGGER build_completed AFTER INSERT OR UPDATE ON calendar_private_storage_states DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_build_completed();
CREATE TRIGGER retained_immutable BEFORE UPDATE OR DELETE ON snapshot_retained_rows FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_snapshot_immutable();
CREATE FUNCTION fixture_snapshot_write_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP<>'INSERT' THEN PERFORM sytue_private_storage_guard('calendar_private_storage_states'::regclass,OLD.tenant,OLD.owner_sub);END IF;
 IF TG_OP<>'DELETE' THEN PERFORM sytue_private_storage_guard('calendar_private_storage_states'::regclass,NEW.tenant,NEW.owner_sub);END IF;
 IF TG_OP='DELETE' THEN RETURN OLD;ELSE RETURN NEW;END IF;
END $$;
CREATE TRIGGER source_write_guard BEFORE INSERT OR UPDATE OR DELETE ON snapshot_legacy_rows FOR EACH ROW EXECUTE FUNCTION fixture_snapshot_write_guard();
`

func newSnapshotHarness(t *testing.T) *pgtest.Harness {
	t.Helper()
	return pgtest.New(t, pg.WithPrivateStorageDDL(fstest.MapFS{"migrations/0001_snapshot.up.sql": &fstest.MapFile{Data: []byte(snapshotFixtureDDL)}}), "migrations")
}

func snapshotScope(owner string) storage.Scope {
	return storage.Scope{Tenant: "acme", Subject: owner, Product: "calendar"}
}

func snapshotContext(scope storage.Scope) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: scope.Tenant})
}

func applySnapshotOwner(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
	if err := pg.SetTenant(ctx, tx, scope.Tenant); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true)`, scope.Subject)
	return err
}

func runSnapshot(ctx context.Context, pool *pg.Pool, cfg pg.PrivateSnapshotConfig) (pg.PrivateStorageState, error) {
	var state pg.PrivateStorageState
	err := pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := applySnapshotOwner(ctx, tx, cfg.Scope); err != nil {
			return err
		}
		var err error
		state, err = pg.BeginPrivateStorageSnapshot(ctx, tx, cfg)
		return err
	})
	return state, err
}

func copySnapshotRows(scope storage.Scope, epoch uuid.UUID) func(context.Context, pgx.Tx) (pg.PrivateSnapshotCounts, error) {
	return func(ctx context.Context, tx pgx.Tx) (pg.PrivateSnapshotCounts, error) {
		result, err := tx.Exec(ctx, `INSERT INTO snapshot_retained_rows(tenant,owner_sub,epoch,id,data) SELECT tenant,owner_sub,$3,id,data FROM snapshot_legacy_rows WHERE tenant=$1 AND owner_sub=$2`, scope.Tenant, scope.Subject, epoch)
		return pg.PrivateSnapshotCounts{Rows: result.RowsAffected()}, err
	}
}

func seedSnapshotOwner(t *testing.T, h *pgtest.Harness, scope storage.Scope) {
	t.Helper()
	ctx := snapshotContext(scope)
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := applySnapshotOwner(ctx, tx, scope); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO snapshot_legacy_rows(tenant,owner_sub,id,data) VALUES($1,$2,1,'original private source')`, scope.Tenant, scope.Subject)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func inspectSnapshot(ctx context.Context, pool *pg.Pool, scope storage.Scope) (pg.PrivateStorageState, int, error) {
	var state pg.PrivateStorageState
	var count int
	err := pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := applySnapshotOwner(ctx, tx, scope); err != nil {
			return err
		}
		var err error
		state, err = pg.ReadPrivateStorageState(ctx, tx, "calendar_private_storage_states", scope.Tenant, scope.Subject)
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM snapshot_retained_rows WHERE tenant=$1 AND owner_sub=$2`, scope.Tenant, scope.Subject).Scan(&count)
	})
	return state, count, err
}

type untouchedSnapshotTx struct{ pgx.Tx }

func (untouchedSnapshotTx) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("snapshot attempted SQL before scope validation")
}

func TestPrivateSnapshotValidatesRegisteredScopeBeforeIO(t *testing.T) {
	cfg := pg.PrivateSnapshotConfig{Scope: snapshotScope("alice"), Epoch: uuid.New(), Capture: func(context.Context, pgx.Tx) (pg.PrivateSnapshotCounts, error) {
		t.Fatal("invalid scope captured data")
		return pg.PrivateSnapshotCounts{}, nil
	}}
	cases := []pg.PrivateSnapshotConfig{cfg, cfg, cfg, cfg, cfg, cfg, cfg}
	cases[0].Scope.Product = "calendar;DROP TABLE calendars"
	cases[6].Scope.Product = "unregistered"
	cases[1].Scope.Tenant = "other"
	cases[2].Scope.Subject = ""
	cases[3].Epoch = uuid.Nil
	cases[4].Capture = nil
	nilAccount := uuid.Nil
	cases[5].Account = &nilAccount
	for _, invalid := range cases {
		if _, err := pg.BeginPrivateStorageSnapshot(snapshotContext(cfg.Scope), untouchedSnapshotTx{}, invalid); !errors.Is(err, pg.ErrPrivateStorageScope) {
			t.Fatal("invalid configuration reached IO", err)
		}
	}
}

func TestPrivateSnapshotAtomicCaptureRollbackRetryAndScope(t *testing.T) {
	h := newSnapshotHarness(t)
	scope := snapshotScope("alice")
	ctx := snapshotContext(scope)
	seedSnapshotOwner(t, h, scope)
	epoch := uuid.New()
	expected := errors.New("domain preflight or capture incomplete")
	cfg := pg.PrivateSnapshotConfig{Scope: scope, Epoch: epoch, Capture: func(opctx context.Context, tx pgx.Tx) (pg.PrivateSnapshotCounts, error) {
		if opctx != ctx {
			t.Fatal("capture context replaced")
		}
		if _, err := copySnapshotRows(scope, epoch)(opctx, tx); err != nil {
			return pg.PrivateSnapshotCounts{}, err
		}
		return pg.PrivateSnapshotCounts{}, expected
	}}
	if _, err := runSnapshot(ctx, h.Pool, cfg); !errors.Is(err, expected) {
		t.Fatal("partial capture error hidden", err)
	}
	state, count, err := inspectSnapshot(ctx, h.Pool, scope)
	if err != nil || state.Mode != "legacy" || state.Epoch != "" || state.StartedAt != nil || count != 0 {
		t.Fatal("incomplete capture left a fence or retained partial rows", state, count, err)
	}
	var captures int
	cfg.Capture = func(ctx context.Context, tx pgx.Tx) (pg.PrivateSnapshotCounts, error) {
		captures++
		return copySnapshotRows(scope, epoch)(ctx, tx)
	}
	state, err = runSnapshot(ctx, h.Pool, cfg)
	if err != nil || state.Mode != "frozen" || state.Epoch != epoch.String() || state.Rows != 1 || state.Blobs != 0 || state.StartedAt == nil {
		t.Fatal("complete capture did not freeze", state, err)
	}
	again, err := runSnapshot(ctx, h.Pool, cfg)
	if err != nil || captures != 1 || again.Epoch != state.Epoch || again.Rows != state.Rows || !again.StartedAt.Equal(*state.StartedAt) {
		t.Fatal("same epoch recaptured or changed the source", again, captures, err)
	}
	cfg.Epoch = uuid.New()
	if _, err := runSnapshot(ctx, h.Pool, cfg); !errors.Is(err, pg.ErrPrivateSnapshotEpoch) {
		t.Fatal("different epoch accepted", err)
	}
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := applySnapshotOwner(ctx, tx, scope); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO snapshot_legacy_rows VALUES($1,$2,2,'must remain blocked')`, scope.Tenant, scope.Subject)
		return err
	})
	var denied *pgconn.PgError
	if !errors.As(err, &denied) || denied.Code != "PST01" {
		t.Fatal("legacy writer bypassed frozen source", err)
	}
	seedSnapshotOwner(t, h, snapshotScope("bob"))
}

func TestPrivateSnapshotRejectsInvalidCountsAndWrongAccount(t *testing.T) {
	h := newSnapshotHarness(t)
	for _, tc := range []struct {
		name   string
		counts pg.PrivateSnapshotCounts
	}{
		{"negative", pg.PrivateSnapshotCounts{Rows: -1}},
		{"blob-count", pg.PrivateSnapshotCounts{Rows: 1, Blobs: 2}},
		{"too-many", pg.PrivateSnapshotCounts{Rows: 1 << 53}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope := snapshotScope("invalid-" + uuid.NewString())
			ctx := snapshotContext(scope)
			seedSnapshotOwner(t, h, scope)
			cfg := pg.PrivateSnapshotConfig{Scope: scope, Epoch: uuid.New(), Capture: func(context.Context, pgx.Tx) (pg.PrivateSnapshotCounts, error) { return tc.counts, nil }}
			if _, err := runSnapshot(ctx, h.Pool, cfg); !errors.Is(err, pg.ErrPrivateSnapshotCapture) {
				t.Fatal("invalid capture counters accepted", tc.name, err)
			}
			state, n, err := inspectSnapshot(ctx, h.Pool, scope)
			if err != nil || state.Mode != "legacy" || n != 0 {
				t.Fatal("invalid counters froze source", state, n, err)
			}
		})
	}

	scope := snapshotScope("account")
	ctx := snapshotContext(scope)
	account := uuid.New()
	cfg := pg.PrivateSnapshotConfig{Scope: scope, Epoch: uuid.New(), Account: &account, Capture: func(context.Context, pgx.Tx) (pg.PrivateSnapshotCounts, error) {
		return pg.PrivateSnapshotCounts{}, nil
	}}
	if _, err := runSnapshot(ctx, h.Pool, cfg); err != nil {
		t.Fatal(err)
	}
	other := uuid.New()
	cfg.Account = &other
	if _, err := runSnapshot(ctx, h.Pool, cfg); !errors.Is(err, pg.ErrPrivateStorageScope) {
		t.Fatal("account identity was substituted", err)
	}
}

func TestPrivateSnapshotRejectsAnAlreadyOpenRepeatableSnapshot(t *testing.T) {
	h := newSnapshotHarness(t)
	scope := snapshotScope("repeatable")
	ctx := snapshotContext(scope)
	seedSnapshotOwner(t, h, scope)
	tx, err := h.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := applySnapshotOwner(ctx, tx, scope); err != nil {
		t.Fatal(err)
	}
	var original int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM snapshot_legacy_rows`).Scan(&original); err != nil || original != 1 {
		t.Fatal(original, err)
	}
	if err := h.Pool.InTenantTx(ctx, func(writer pgx.Tx) error {
		if err := applySnapshotOwner(ctx, writer, scope); err != nil {
			return err
		}
		_, err := writer.Exec(ctx, `INSERT INTO snapshot_legacy_rows VALUES($1,$2,2,'committed after the repeatable snapshot')`, scope.Tenant, scope.Subject)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	called := false
	cfg := pg.PrivateSnapshotConfig{Scope: scope, Epoch: uuid.New(), Capture: func(context.Context, pgx.Tx) (pg.PrivateSnapshotCounts, error) {
		called = true
		return pg.PrivateSnapshotCounts{}, nil
	}}
	if _, err := pg.BeginPrivateStorageSnapshot(ctx, tx, cfg); !errors.Is(err, pg.ErrPrivateSnapshotIsolation) || called {
		t.Fatal("stale repeatable snapshot was allowed to freeze", err)
	}
}

func waitForSnapshotBlock(ctx context.Context, t *testing.T, pool *pg.Pool, blocked, blocker int32) {
	t.Helper()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT $2::integer=ANY(pg_blocking_pids($1::integer))`, blocked, blocker).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("migration never blocked on the active writer", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestPrivateSnapshotWaitsForWriterAndCapturesItsCommittedData(t *testing.T) {
	h := newSnapshotHarness(t)
	scope := snapshotScope("writer")
	seedSnapshotOwner(t, h, scope)
	ctx, cancel := context.WithTimeout(snapshotContext(scope), 10*time.Second)
	defer cancel()
	writer, err := h.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Rollback(context.Background()) }()
	if err := applySnapshotOwner(ctx, writer, scope); err != nil {
		t.Fatal(err)
	}
	var writerPID int32
	if err := writer.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&writerPID); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(ctx, `INSERT INTO snapshot_legacy_rows VALUES($1,$2,2,'writer committed before freeze')`, scope.Tenant, scope.Subject); err != nil {
		t.Fatal(err)
	}
	epoch := uuid.New()
	pid := make(chan int32, 1)
	done := make(chan error, 1)
	var state pg.PrivateStorageState
	go func() {
		done <- h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if err := applySnapshotOwner(ctx, tx, scope); err != nil {
				return err
			}
			var current int32
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&current); err != nil {
				return err
			}
			pid <- current
			var err error
			state, err = pg.BeginPrivateStorageSnapshot(ctx, tx, pg.PrivateSnapshotConfig{Scope: scope, Epoch: epoch, Capture: copySnapshotRows(scope, epoch)})
			return err
		})
	}()
	var migrationPID int32
	select {
	case migrationPID = <-pid:
	case err := <-done:
		t.Fatal("migration ended before acquiring the state lock", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitForSnapshotBlock(ctx, t, h.Pool, migrationPID, writerPID)
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil || state.Rows != 2 {
		t.Fatal("committed writer was omitted from the retained inventory", state, err)
	}
}

func TestPrivateSnapshotConcurrentSameEpochCapturesExactlyOnce(t *testing.T) {
	h := newSnapshotHarness(t)
	scope := snapshotScope("concurrent")
	seedSnapshotOwner(t, h, scope)
	ctx, cancel := context.WithTimeout(snapshotContext(scope), 10*time.Second)
	defer cancel()
	epoch := uuid.New()
	entered := make(chan struct{})
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	defer release()
	var calls atomic.Int32
	cfg := pg.PrivateSnapshotConfig{Scope: scope, Epoch: epoch, Capture: func(ctx context.Context, tx pgx.Tx) (pg.PrivateSnapshotCounts, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-gate:
		case <-ctx.Done():
			return pg.PrivateSnapshotCounts{}, ctx.Err()
		}
		return copySnapshotRows(scope, epoch)(ctx, tx)
	}}
	type result struct {
		state pg.PrivateStorageState
		err   error
	}
	start := func(pid chan int32, done chan result) {
		go func() {
			var state pg.PrivateStorageState
			err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
				if err := applySnapshotOwner(ctx, tx, scope); err != nil {
					return err
				}
				var current int32
				if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&current); err != nil {
					return err
				}
				pid <- current
				var err error
				state, err = pg.BeginPrivateStorageSnapshot(ctx, tx, cfg)
				return err
			})
			done <- result{state: state, err: err}
		}()
	}
	firstPID, secondPID := make(chan int32, 1), make(chan int32, 1)
	first, second := make(chan result, 1), make(chan result, 1)
	start(firstPID, first)
	var one int32
	select {
	case one = <-firstPID:
	case value := <-first:
		t.Fatal("first migration did not start", value.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-entered:
	case value := <-first:
		t.Fatal(value.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	start(secondPID, second)
	var two int32
	select {
	case two = <-secondPID:
	case value := <-second:
		t.Fatal("second migration did not start", value.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitForSnapshotBlock(ctx, t, h.Pool, two, one)
	release()
	a, b := <-first, <-second
	if a.err != nil || b.err != nil || calls.Load() != 1 || a.state.Epoch != b.state.Epoch || a.state.Rows != b.state.Rows {
		t.Fatal("same-epoch concurrent retry did not reuse one complete capture", a, b, calls.Load())
	}
}
