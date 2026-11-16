// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pgtest_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
)

func admissionFixture(t *testing.T) *pgtest.Harness {
	t.Helper()
	h := scopeMergeFixture(t)
	ddl, err := fs.ReadFile(pg.WithScopeAdmissionDDL(pg.WithPrivateStorageDDL(fstest.MapFS{"admission.up.sql": &fstest.MapFile{Data: []byte(`
-- sytue:private-storage-fence-v1
-- sytue:scope-admission-v1
SELECT sytue_scope_admission_register('bound_rows'::regclass);
CREATE TABLE admission_private_storage_states(tenant text PRIMARY KEY,mode text NOT NULL DEFAULT 'legacy',owner_sub text NOT NULL DEFAULT 'alice',epoch uuid,started_at timestamptz,source_rows bigint NOT NULL DEFAULT 0,source_blobs bigint NOT NULL DEFAULT 0);
CREATE TRIGGER admission_state_scope_immutable BEFORE UPDATE OR DELETE ON admission_private_storage_states FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_state_scope_guard();
ALTER TABLE admission_private_storage_states ENABLE ROW LEVEL SECURITY;
ALTER TABLE admission_private_storage_states FORCE ROW LEVEL SECURITY;
CREATE POLICY admission_states_owner ON admission_private_storage_states USING(tenant=current_setting('app.tenant',true)) WITH CHECK(tenant=current_setting('app.tenant',true));
CREATE POLICY admission_states_merge ON admission_private_storage_states USING(current_setting('app.merge',true)='on') WITH CHECK(current_setting('app.merge',true)='on');
SELECT sytue_scope_admission_register('admission_private_storage_states'::regclass,true);
`)}})), "admission.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.PoolSinRLS(t).Exec(t.Context(), string(ddl)); err != nil {
		t.Fatal(err)
	}
	return h
}

func admissionTransition(ctx context.Context, h *pgtest.Harness, job uuid.UUID, phase string) error {
	return h.Pool.InTx(ctx, func(tx pgx.Tx) error { return pg.TransitionScopeAdmission(ctx, tx, job, "source", "target", phase) })
}

func admissionInsert(ctx context.Context, h *pgtest.Harness, tenant string) error {
	return h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO bound_rows VALUES($1,'immutable scope')`, tenant)
		return err
	})
}

func admissionSQLState(t *testing.T, err error, code string) {
	t.Helper()
	var failure *pgconn.PgError
	if !errors.As(err, &failure) || failure.Code != code {
		t.Fatalf("scope admission SQL state = %v, want %s", err, code)
	}
}

func TestScopeAdmissionPersistsRetirementAndReopensOnlyFreshTargetTransactions(t *testing.T) {
	h := admissionFixture(t)
	job := uuid.New()
	if err := admissionTransition(t.Context(), h, job, "close"); err != nil {
		t.Fatal(err)
	}
	admissionSQLState(t, admissionInsert(t.Context(), h, "source"), "PST03")
	admissionSQLState(t, admissionInsert(t.Context(), h, "target"), "PST03")
	// Ordinary legacy movement must not wait on its own closed admission fence.
	if err := h.Pool.InMergeTx(t.Context(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `INSERT INTO admission_private_storage_states(tenant,mode) VALUES('source','legacy')`); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `UPDATE admission_private_storage_states SET tenant='target' WHERE tenant='source'`)
		return err
	}); err != nil {
		t.Fatal("closed admission blocked legacy movement", err)
	}
	err := h.Pool.InTx(t.Context(), func(tx pgx.Tx) error {
		if err := pg.SetTenant(t.Context(), tx, "target"); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `UPDATE admission_private_storage_states SET mode='building' WHERE tenant='target'`)
		return err
	})
	admissionSQLState(t, err, "PST03")
	if err := admissionTransition(t.Context(), h, job, "complete"); err != nil {
		t.Fatal(err)
	}
	admissionSQLState(t, admissionInsert(t.Context(), h, "source"), "PST03")
	if err := admissionInsert(t.Context(), h, "target"); err != nil {
		t.Fatal("fresh target remained closed", err)
	}
	if err := admissionTransition(t.Context(), h, job, "abort"); err != nil {
		t.Fatal(err)
	}
	admissionSQLState(t, admissionInsert(t.Context(), h, "source"), "PST03")
	if err := admissionTransition(t.Context(), h, uuid.New(), "close"); err == nil {
		t.Fatal("retired source was enrolled in another merge")
	}
	var n int
	if err := h.Pool.QueryRow(t.Context(), `SELECT count(*) FROM private_scope_admission_control`).Scan(&n); err != nil || n != 0 {
		t.Fatal("admission infrastructure leaked through ordinary RLS", n, err)
	}
}

func TestScopeAdmissionRejectsTransactionThatWaitedForReopeningCommit(t *testing.T) {
	h := admissionFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	job := uuid.New()
	if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `SELECT sytue_scope_admission_check('target')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	stale, err := h.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stale.Rollback(ctx) }()
	if err := pg.BindScopeAdmissionSnapshot(ctx, stale); err != nil {
		t.Fatal(err)
	}
	if err := pg.SetTenant(ctx, stale, "target"); err != nil {
		t.Fatal(err)
	}
	var stalePID int32
	if err := stale.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&stalePID); err != nil {
		t.Fatal(err)
	}
	if _, err := stale.Exec(ctx, `INSERT INTO bound_rows VALUES('target','must not resume')`); err != nil {
		t.Fatal(err)
	}
	if err := admissionTransition(ctx, h, job, "close"); err != nil {
		t.Fatal(err)
	}
	reopening, err := h.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopening.Rollback(ctx) }()
	if err := pg.BindScopeAdmissionSnapshot(ctx, reopening); err != nil {
		t.Fatal(err)
	}
	var reopenPID int32
	if err := reopening.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&reopenPID); err != nil {
		t.Fatal(err)
	}
	if err := pg.TransitionScopeAdmission(ctx, reopening, job, "source", "target", "abort"); err != nil {
		t.Fatal(err)
	}
	// A second TX begins after the reopening SQL but before its COMMIT. Even
	// this commit-gap case must use the snapshot, not a wall-clock comparison.
	gap, err := h.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gap.Rollback(ctx) }()
	if err := pg.BindScopeAdmissionSnapshot(ctx, gap); err != nil {
		t.Fatal(err)
	}
	if err := pg.SetTenant(ctx, gap, "target"); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- stale.Commit(ctx) }()
	pgtest.WaitForBlockingPID(ctx, t, h.Pool, stalePID, reopenPID)
	if err := reopening.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	staleErr := <-finished
	var staleWrites int
	if err := h.PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM bound_rows WHERE pin='must not resume'`).Scan(&staleWrites); err != nil || staleWrites != 0 {
		t.Fatal("stale transaction published after reopening", staleWrites, err)
	}
	admissionSQLState(t, staleErr, "40001")
	_, err = gap.Exec(ctx, `INSERT INTO bound_rows VALUES('target','commit gap must fail')`)
	admissionSQLState(t, err, "40001")
	if err := admissionInsert(ctx, h, "target"); err != nil {
		t.Fatal("fresh retry could not publish", err)
	}
	var n int
	if err := h.PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM bound_rows WHERE pin IN('must not resume','commit gap must fail')`).Scan(&n); err != nil || n != 0 {
		t.Fatal("stale authorization resumed", n, err)
	}
}

func TestScopeAdmissionClosingWaitsForPreviouslyAdmittedScopeWrite(t *testing.T) {
	h := admissionFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	admitted, err := h.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admitted.Rollback(ctx) }()
	if err := pg.BindScopeAdmissionSnapshot(ctx, admitted); err != nil {
		t.Fatal(err)
	}
	if err := pg.SetTenant(ctx, admitted, "source"); err != nil {
		t.Fatal(err)
	}
	var writerPID int32
	if err := admitted.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&writerPID); err != nil {
		t.Fatal(err)
	}
	if _, err := admitted.Exec(ctx, `INSERT INTO bound_rows VALUES('source','admitted before close')`); err != nil {
		t.Fatal(err)
	}
	// Execute the real deferred guard while retaining the transaction, so the
	// test can observe the commit decision's lock before allowing COMMIT.
	if _, err := admitted.Exec(ctx, `SET CONSTRAINTS scope_bound_admission_commit IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	closing, err := h.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closing.Rollback(ctx) }()
	if err := pg.BindScopeAdmissionSnapshot(ctx, closing); err != nil {
		t.Fatal(err)
	}
	var closingPID int32
	if err := closing.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&closingPID); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	job := uuid.New()
	go func() { finished <- pg.TransitionScopeAdmission(ctx, closing, job, "source", "target", "close") }()
	pgtest.WaitForBlockingPID(ctx, t, h.Pool, closingPID, writerPID)
	if err := admitted.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if err := closing.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	err = h.Pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		inventory, err := pg.ConsultaInventarioDeFusion(ctx, tx)
		if err != nil {
			return err
		}
		return pg.CheckScopeBoundMerge(ctx, tx, inventory, "source", "target")
	})
	if !errors.Is(err, pg.ErrScopeBoundMerge) {
		t.Fatal("a committed earlier admission escaped the data preflight", err)
	}
}

func TestScopeAdmissionCloseAndRetirementRejectPendingWriteAtCommit(t *testing.T) {
	for _, phase := range []string{"close", "complete"} {
		t.Run(phase, func(t *testing.T) {
			h := admissionFixture(t)
			ctx := t.Context()
			job := uuid.New()
			if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `SELECT sytue_scope_admission_check('source')`)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			pending, err := h.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = pending.Rollback(ctx) }()
			if err := pg.BindScopeAdmissionSnapshot(ctx, pending); err != nil {
				t.Fatal(err)
			}
			if err := pg.SetTenant(ctx, pending, "source"); err != nil {
				t.Fatal(err)
			}
			if _, err := pending.Exec(ctx, `INSERT INTO bound_rows VALUES('source','pending before close')`); err != nil {
				t.Fatal(err)
			}
			if err := admissionTransition(ctx, h, job, "close"); err != nil {
				t.Fatal(err)
			}
			if phase == "complete" {
				if err := admissionTransition(ctx, h, job, "complete"); err != nil {
					t.Fatal(err)
				}
			}
			pendingErr := pending.Commit(ctx)
			var committedWrites int
			if err := h.PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM bound_rows WHERE pin='pending before close'`).Scan(&committedWrites); err != nil || committedWrites != 0 {
				t.Fatal("pending write resurrected source after admission closed", committedWrites, err)
			}
			admissionSQLState(t, pendingErr, "PST03")
			var n int
			if err := h.PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM bound_rows`).Scan(&n); err != nil || n != 0 {
				t.Fatal("source scope resurrected after admission closed", n, err)
			}
		})
	}
}

func TestScopeAdmissionCoverageRequiresBothEnabledGuards(t *testing.T) {
	for _, guard := range []string{"scope_bound_admission", "scope_bound_admission_commit"} {
		t.Run(guard, func(t *testing.T) {
			h := admissionFixture(t)
			inventory, err := pg.ConsultaInventarioDeFusion(t.Context(), h.Pool)
			if err != nil {
				t.Fatal(err)
			}
			if err := pg.RequireScopeAdmissionCoverage(t.Context(), h.Pool, inventory); err != nil {
				t.Fatal("complete guard rejected", err)
			}
			if _, err := h.PoolSinRLS(t).Exec(t.Context(), `ALTER TABLE bound_rows DISABLE TRIGGER `+guard); err != nil {
				t.Fatal(err)
			}
			if err := pg.RequireScopeAdmissionCoverage(t.Context(), h.Pool, inventory); err == nil {
				t.Fatal("disabled admission guard accepted", guard)
			}
		})
	}
}

func TestScopeAdmissionRollbackWaitsForWriterThenRetainsItsProof(t *testing.T) {
	h := admissionFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	writer, err := h.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Rollback(ctx) }()
	if err := pg.BindScopeAdmissionSnapshot(ctx, writer); err != nil {
		t.Fatal(err)
	}
	if err := pg.SetTenant(ctx, writer, "source"); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(ctx, `INSERT INTO bound_rows VALUES('source','retained through rollback')`); err != nil {
		t.Fatal(err)
	}
	var writerPID int32
	if err := writer.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&writerPID); err != nil {
		t.Fatal(err)
	}
	down, err := h.PoolSinRLS(t).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = down.Rollback(ctx) }()
	var downPID int32
	if err := down.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&downPID); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { _, err := down.Exec(ctx, `SELECT sytue_scope_admission_remove()`); finished <- err }()
	pgtest.WaitForBlockingPID(ctx, t, h.Pool, downPID, writerPID)
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	admissionSQLState(t, <-finished, "PST03")
	if err := down.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := h.PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM bound_rows WHERE tenant='source' AND pin='retained through rollback'`).Scan(&n); err != nil || n != 1 {
		t.Fatal("rollback lost committed bound data", n, err)
	}
	inventory, err := pg.ConsultaInventarioDeFusion(ctx, h.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := pg.RequireScopeAdmissionCoverage(ctx, h.Pool, inventory); err != nil {
		t.Fatal("rollback removed admission protection", err)
	}
}

func TestScopeAdmissionCommitLockDoesNotBlockUnrelatedMergeScopes(t *testing.T) {
	h := admissionFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	writer, err := h.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Rollback(ctx) }()
	if err := pg.BindScopeAdmissionSnapshot(ctx, writer); err != nil {
		t.Fatal(err)
	}
	if err := pg.SetTenant(ctx, writer, "source"); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(ctx, `INSERT INTO bound_rows VALUES('source','independent scope'); SET CONSTRAINTS scope_bound_admission_commit IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	unrelated, cancelUnrelated := context.WithTimeout(ctx, 2*time.Second)
	defer cancelUnrelated()
	if err := h.Pool.InTx(unrelated, func(tx pgx.Tx) error {
		return pg.TransitionScopeAdmission(unrelated, tx, uuid.New(), "unrelated-source", "unrelated-target", "close")
	}); err != nil {
		t.Fatal("unrelated merge waited on a writer in another scope", err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestScopeAdmissionRollbackRefusesHistoryEvenWithoutApplicationRows(t *testing.T) {
	h := admissionFixture(t)
	ctx := t.Context()
	job := uuid.New()
	if err := admissionTransition(ctx, h, job, "close"); err != nil {
		t.Fatal(err)
	}
	if err := admissionTransition(ctx, h, job, "abort"); err != nil {
		t.Fatal(err)
	}
	_, err := h.PoolSinRLS(t).Exec(ctx, `SELECT sytue_scope_admission_remove()`)
	admissionSQLState(t, err, "PST03")
}

func TestScopeAdmissionRollbackCanRemoveUnusedGenerationZeroInfrastructure(t *testing.T) {
	h := admissionFixture(t)
	ctx := t.Context()
	if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `SELECT sytue_scope_admission_check('unused')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.PoolSinRLS(t).Exec(ctx, `SELECT sytue_scope_admission_remove(); DROP TABLE private_scope_admission_control`); err != nil {
		t.Fatal("empty untransitioned admission could not be rolled back", err)
	}
}

func TestScopeAdmissionOldCompletionCannotReopenTargetOfLaterMerge(t *testing.T) {
	h := admissionFixture(t)
	ctx := t.Context()
	original, later := uuid.New(), uuid.New()
	if err := admissionTransition(ctx, h, original, "close"); err != nil {
		t.Fatal(err)
	}
	if err := admissionTransition(ctx, h, original, "complete"); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		return pg.TransitionScopeAdmission(ctx, tx, later, "target", "later-target", "close")
	}); err != nil {
		t.Fatal(err)
	}
	if err := admissionTransition(ctx, h, original, "complete"); err != nil {
		t.Fatal("old completion was not idempotent", err)
	}
	admissionSQLState(t, admissionInsert(ctx, h, "target"), "PST03")
	if err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		return pg.TransitionScopeAdmission(ctx, tx, later, "target", "later-target", "abort")
	}); err != nil {
		t.Fatal(err)
	}
	if err := admissionInsert(ctx, h, "target"); err != nil {
		t.Fatal("later job could not reopen its own target generation", err)
	}
}
