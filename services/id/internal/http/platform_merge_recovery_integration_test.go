//go:build integration

// SPDX-License-Identifier: Apache-2.0
package http

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/id/internal/merge"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

type runningMergeEnvironment struct {
	server              *Server
	identity            *pgtest.Harness
	services            map[string]*pgtest.Harness
	source, target, job string
}

func runningMergeFixture(t *testing.T) runningMergeEnvironment {
	t.Helper()
	identity, services := mergeScopeSuiteFixture(t)
	source, target, job := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := identity.Pool.Exec(t.Context(), `INSERT INTO organizations(id,slug) VALUES($1,'source'),($2,'target')`, source, target); err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Pool.Exec(t.Context(), `INSERT INTO org_merges(id,source_org_id,target_org_id,status,error,started_at) VALUES($1,$2,$3,'running','retained progress',now()+interval '1 year')`, job, source, target); err != nil {
		t.Fatal(err)
	}
	server := &Server{mergeRunner: newMergeRunnerState(), cfg: Config{DatabaseDSN: identity.AppDSN}, store: store.New(identity.Pool), log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return runningMergeEnvironment{server, identity, services, source, target, job}
}

func TestRunningMergeRecoveryKeepsLiveOwnerAndReconcilesSameCommittedIntent(t *testing.T) {
	for _, completed := range []bool{false, true} {
		name := "before admission completion"
		if completed {
			name = "after admission completion"
		}
		t.Run(name, func(t *testing.T) {
			fixture := runningMergeFixture(t)
			server, identity, services, source, target, job := fixture.server, fixture.identity, fixture.services, fixture.source, fixture.target, fixture.job
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			// ID's direct URL must not route any sibling session back to ID.
			t.Setenv(pg.DirectDSNEnv, identity.AppDSN)
			owner, err := merge.AcquireOwnership(ctx, identity.AppDSN, job)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = owner.Close(ctx) }()
			for _, service := range append([]string{"id"}, merge.Services...) {
				var actual string
				if err := owner.Service(service).QueryRow(ctx, `SELECT current_database()`).Scan(&actual); err != nil {
					t.Fatal(err)
				}
				var expected string
				h := identity
				if service != "id" {
					h = services[service]
				}
				if err := h.Pool.QueryRow(ctx, `SELECT current_database()`).Scan(&expected); err != nil || actual != expected {
					t.Fatal("owned session routed to another service", service, actual, expected, err)
				}
			}
			admission, err := merge.PrepareOwnedAdmission(ctx, owner, "source", "target")
			if err != nil {
				t.Fatal(err)
			}
			var generationBefore int64
			if err := services["rooms"].PoolSinRLS(t).QueryRow(ctx, `SELECT generation FROM private_scope_admission_control WHERE scope_key='target'`).Scan(&generationBefore); err != nil {
				t.Fatal(err)
			}
			if err := server.RecoverOrgMerges(ctx); err != nil {
				t.Fatal(err)
			}
			var liveSourceRows int
			if err := services["mail"].PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM untouched_domain_rows WHERE tenant='source'`).Scan(&liveSourceRows); err != nil || liveSourceRows != 1 {
				t.Fatal("competing recovery moved a live owner's source rows", liveSourceRows, err)
			}
			var status, detail string
			if err := identity.Pool.QueryRow(ctx, `SELECT status,error FROM org_merges WHERE id=$1`, job).Scan(&status, &detail); err != nil || status != "running" || detail != "retained progress" {
				t.Fatal("discovery stole live owner or inferred expiry from a clock", status, detail, err)
			}
			// Commit identity before its final job/progress acknowledgement.
			if _, err := owner.Identity().Exec(ctx, `UPDATE organizations SET status='merged',merged_into_org_id=$2 WHERE id=$1`, source, target); err != nil {
				t.Fatal(err)
			}
			expectedGeneration := generationBefore + 1
			if completed {
				if err := admission.Complete(ctx); err != nil {
					t.Fatal(err)
				}
				if err := services["rooms"].Pool.InTx(ctx, func(tx pgx.Tx) error {
					if err := pg.SetTenant(ctx, tx, "target"); err != nil {
						return err
					}
					_, err := tx.Exec(ctx, `INSERT INTO matrix_storage_rooms(tenant,subject,product,room_id) VALUES('target','new-device','photos','!new:fixture.invalid')`)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			if err := owner.Close(ctx); err != nil {
				t.Fatal(err)
			}
			recoverMergeToCompletion(ctx, t, server, identity, job)
			var sourceState, targetState string
			var generation int64
			if err := services["rooms"].PoolSinRLS(t).QueryRow(ctx, `SELECT state FROM private_scope_admission_control WHERE scope_key='source'`).Scan(&sourceState); err != nil || sourceState != "retired" {
				t.Fatal("recovery reopened source", sourceState, err)
			}
			if err := services["rooms"].PoolSinRLS(t).QueryRow(ctx, `SELECT state,generation FROM private_scope_admission_control WHERE scope_key='target'`).Scan(&targetState, &generation); err != nil || targetState != "open" || generation != expectedGeneration {
				t.Fatal("recovery flipped target generation", targetState, generation, expectedGeneration, err)
			}
			if completed {
				var count int
				if err := services["rooms"].PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM matrix_storage_rooms WHERE tenant='target' AND room_id='!new:fixture.invalid'`).Scan(&count); err != nil || count != 1 {
					t.Fatal("recovery lost new target ciphertext routing", count, err)
				}
			}
		})
	}
}

func recoverMergeToCompletion(ctx context.Context, t *testing.T, server *Server, identity *pgtest.Harness, job string) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := server.RecoverOrgMerges(ctx); err != nil {
			t.Fatal(err)
		}
		var status string
		if err := identity.Pool.QueryRow(ctx, `SELECT status FROM org_merges WHERE id=$1`, job).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "completed" {
			return
		}
		if status != "running" {
			t.Fatal("crash recovery failed", status)
		}
		select {
		case <-ctx.Done():
			t.Fatal("actual owner sessions not recovered", ctx.Err())
		case <-ticker.C:
		}
	}
}

func TestOwnedAdmissionCannotReopenSourceUsingAReplacementConnection(t *testing.T) {
	fixture := runningMergeFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	owner, err := merge.AcquireOwnership(ctx, fixture.identity.AppDSN, fixture.job)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close(ctx) }()
	admission, err := merge.PrepareOwnedAdmission(ctx, owner, "source", "target")
	if err != nil {
		t.Fatal(err)
	}
	var pid int32
	if err := owner.Service("rooms").QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	admin := fixture.services["rooms"].PoolSinRLS(t)
	if _, err := admin.Exec(ctx, `SELECT pg_terminate_backend($1)`, pid); err != nil {
		t.Fatal(err)
	}
	abortErr := admission.Abort(ctx)
	writeErr := admin.InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO matrix_storage_rooms(tenant,subject,product,room_id) VALUES('source','zombie-owner','photos','!replacement:fixture.invalid')`)
		return err
	})
	var state string
	var count int
	if err := admin.QueryRow(ctx, `SELECT state FROM private_scope_admission_control WHERE scope_key='source'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_rooms WHERE subject='zombie-owner'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if state != "closed" || count != 0 {
		t.Fatal("dead owned admission reopened source and admitted replacement write", state, count)
	}
	if abortErr == nil || writeErr == nil {
		t.Fatal("dead session was treated as authority for scope transition", abortErr, writeErr)
	}
}

func TestPendingMergeRecoveryRetainsIntentBeforeClaimAndReportsFailureWithoutMovingDomains(t *testing.T) {
	fixture := runningMergeFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	if _, err := fixture.identity.Pool.Exec(ctx, `UPDATE org_merges SET status='pending' WHERE id=$1`, fixture.job); err != nil {
		t.Fatal(err)
	}
	owner, err := merge.AcquireOwnership(ctx, fixture.identity.AppDSN, fixture.job)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close(ctx) }()
	if err := fixture.server.RecoverOrgMerges(ctx); err != nil {
		t.Fatal(err)
	}
	var status, detail string
	if err := fixture.identity.Pool.QueryRow(ctx, `SELECT status,error FROM org_merges WHERE id=$1`, fixture.job).Scan(&status, &detail); err != nil || status != "pending" || detail != "retained progress" {
		t.Fatal("discovery stole a pending intent before real ownership", status, detail, err)
	}
	if _, err := fixture.services["mail"].PoolSinRLS(t).Exec(ctx, `CREATE FUNCTION platform_merge_prepare(text,text) RETURNS text LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic pending delivery failure'; END $$`); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := fixture.server.RecoverOrgMerges(ctx); err != nil {
			t.Fatal(err)
		}
		if err := fixture.identity.Pool.QueryRow(ctx, `SELECT status,error FROM org_merges WHERE id=$1`, fixture.job).Scan(&status, &detail); err != nil {
			t.Fatal(err)
		}
		if status == "failed" {
			break
		}
		if status != "pending" {
			t.Fatal("pending intent changed without a settled execution", status, detail)
		}
		select {
		case <-ctx.Done():
			t.Fatal("pending intent stranded after owner failure", ctx.Err())
		case <-ticker.C:
		}
	}
	if !strings.Contains(detail, "synthetic pending delivery failure") {
		t.Fatal("recovery hid the failed stage", detail)
	}
	for service, h := range fixture.services {
		var count int
		if err := h.PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM untouched_domain_rows WHERE tenant='source' AND payload='exact source'`).Scan(&count); err != nil || count != 1 {
			t.Fatal("failed pending recovery mutated a domain", service, count, err)
		}
	}
}

func TestGlobalOperationOwnershipBoundsDistinctJobsAndSurvivesIdentitySessionLoss(t *testing.T) {
	fixture := runningMergeFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	owner, err := merge.AcquireOwnership(ctx, fixture.identity.AppDSN, fixture.job)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close(ctx) }()
	if _, err := merge.PrepareOwnedAdmission(ctx, owner, "source", "target"); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, 17)
	for _, service := range append([]string{"id"}, merge.Services...) {
		var name string
		if err := owner.Service(service).QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	countLocks := func() int {
		t.Helper()
		var count int
		if err := fixture.identity.Pool.QueryRow(ctx, `SELECT count(*) FROM pg_locks WHERE locktype='advisory' AND granted
   AND database IN(SELECT oid FROM pg_database WHERE datname=ANY($2::text[]))
   AND classid::bigint=((hashtextextended($1,17107030)>>32)&4294967295)
   AND objid::bigint=(hashtextextended($1,17107030)&4294967295)`, pg.OrganizationOperationLockGroup, names).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	if count := countLocks(); count != 17 {
		t.Fatal("operation did not own exactly one global session per database", count)
	}
	jobs := make([]string, 0, 32)
	for i := 0; i < 32; i++ {
		source, target, job := uuid.NewString(), uuid.NewString(), uuid.NewString()
		jobs = append(jobs, job)
		if _, err := fixture.identity.Pool.Exec(ctx, `INSERT INTO organizations(id,slug) VALUES($1,$3),($2,$4)`, source, target, fmt.Sprintf("source-%d", i), fmt.Sprintf("target-%d", i)); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.identity.Pool.Exec(ctx, `INSERT INTO org_merges(id,source_org_id,target_org_id,status) VALUES($1,$2,$3,'pending')`, job, source, target); err != nil {
			t.Fatal(err)
		}
	}
	var calls sync.WaitGroup
	for range 32 {
		calls.Add(1)
		go func() { defer calls.Done(); fixture.server.wakeMergeRunner() }()
	}
	calls.Wait()
	if queued := len(fixture.server.mergeRunner.wake); queued != 1 {
		t.Fatal("requests did not coalesce into one serial wakeup", queued)
	}
	if err := fixture.server.RecoverOrgMerges(ctx); err != nil {
		t.Fatal(err)
	}
	var pending int
	if err := fixture.identity.Pool.QueryRow(ctx, `SELECT count(*) FROM org_merges WHERE id=ANY($1::uuid[]) AND status='pending'`, jobs).Scan(&pending); err != nil || pending != 32 {
		t.Fatal("different jobs passed a live global owner", pending, err)
	}
	if count := countLocks(); count != 17 {
		t.Fatal("candidate jobs multiplied retained ownership sessions", count)
	}
	var idPID int32
	if err := owner.Identity().QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&idPID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.identity.PoolSinRLS(t).Exec(ctx, `SELECT pg_terminate_backend($1)`, idPID); err != nil {
		t.Fatal(err)
	}
	if err := fixture.server.RecoverOrgMerges(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fixture.identity.Pool.QueryRow(ctx, `SELECT count(*) FROM org_merges WHERE id=ANY($1::uuid[]) AND status='pending'`, jobs).Scan(&pending); err != nil || pending != 32 {
		t.Fatal("different job advanced past 16 retained database owners", pending, err)
	}
	if count := countLocks(); count > 17 {
		t.Fatal("partial acquisitions exceeded the global owned-session budget", count)
	}
	var scopes int
	if err := fixture.services["rooms"].PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM private_scope_admission_control`).Scan(&scopes); err != nil || scopes != 2 {
		t.Fatal("distinct job wrote new scope admission while old domains remained owned", scopes, err)
	}
	for service, h := range fixture.services {
		var count int
		if err := h.PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM untouched_domain_rows WHERE tenant='source' AND payload='exact source'`).Scan(&count); err != nil || count != 1 {
			t.Fatal("competing jobs moved an owned domain", service, count, err)
		}
	}
}
