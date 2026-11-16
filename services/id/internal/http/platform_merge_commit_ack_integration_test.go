//go:build integration

// SPDX-License-Identifier: Apache-2.0
package http

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/id/internal/merge"
)

func TestRunningMergeRecoveryAfterRealLostIdentityCommitAcknowledgement(t *testing.T) {
	fixture := runningMergeFixture(t)
	server, identity, services, source, target, job := fixture.server, fixture.identity, fixture.services, fixture.source, fixture.target, fixture.job
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	t.Setenv(pg.DirectDSNEnv, "")
	proxy := pgtest.NewCommitAckProxy(t, identity.AppDSN)
	owner, err := merge.AcquireOwnership(ctx, proxy.DSN(), job)
	if err != nil {
		t.Fatal("proxy ownership acquisition failed")
	}
	defer func() { _ = owner.Close(ctx) }()
	if _, err := merge.PrepareOwnedAdmission(ctx, owner, "source", "target"); err != nil {
		t.Fatal(err)
	}
	// Complete the domain phase on the physical sessions that own it.
	for _, service := range merge.Services {
		if err := owner.Service(service).InMergeTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE untouched_domain_rows SET tenant='target' WHERE tenant='source'`)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	proxy.Arm()
	err = owner.Identity().InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE organizations SET status='merged',merged_into_org_id=$2 WHERE id=$1`, source, target)
		return err
	})
	if err == nil {
		t.Fatal("fixture did not lose the real COMMIT acknowledgement")
	}
	if err := proxy.WaitDropped(ctx); err != nil {
		t.Fatal("proxy never observed PostgreSQL's successful COMMIT", err)
	}
	var status string
	if err := identity.Pool.QueryRow(ctx, `SELECT status FROM organizations WHERE id=$1`, source).Scan(&status); err != nil || status != "merged" {
		t.Fatal("lost acknowledgement was confused with a rolled-back transaction", status, err)
	}
	// Identity's connection died, but a competitor cannot run ahead of the old
	// worker's remaining owned sessions or turn the uncertain result into abort.
	candidate, err := merge.AcquireOwnership(ctx, identity.AppDSN, job)
	if candidate != nil {
		defer func() { _ = candidate.Close(ctx) }()
	}
	if !errors.Is(err, pg.ErrSessionLockBusy) {
		t.Fatal("competitor ignored retained domain ownership", err)
	}
	if err := server.RecoverOrgMerges(ctx); err != nil {
		t.Fatal(err)
	}
	if err := identity.Pool.QueryRow(ctx, `SELECT status FROM org_merges WHERE id=$1`, job).Scan(&status); err != nil || status != "running" {
		t.Fatal("uncertain owner was overwritten", status, err)
	}
	var state string
	if err := services["rooms"].PoolSinRLS(t).QueryRow(ctx, `SELECT state FROM private_scope_admission_control WHERE scope_key='source'`).Scan(&state); err != nil || state != "closed" {
		t.Fatal("uncertain COMMIT reopened source", state, err)
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	recoverMergeToCompletion(ctx, t, server, identity, job)
	if err := services["rooms"].PoolSinRLS(t).QueryRow(ctx, `SELECT state FROM private_scope_admission_control WHERE scope_key='source'`).Scan(&state); err != nil || state != "retired" {
		t.Fatal("fresh owner reopened committed source", state, err)
	}
	for service, h := range services {
		var count int
		if err := h.PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM untouched_domain_rows WHERE tenant='target'`).Scan(&count); err != nil || count != 2 {
			t.Fatal("fresh owner moved or lost committed domain data", service, count, err)
		}
	}
}
