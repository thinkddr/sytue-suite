//go:build integration

// SPDX-License-Identifier: Apache-2.0
package http

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/id/internal/merge"
)

type mergeOwnershipProcessRequest struct{ DSN, Job, SourceID, TargetID string }

// Credentials go through an anonymous pipe to the isolated test process. Neither
// its environment nor its output contains provider/production credentials.
func TestMergeOwnershipProcessFixture(t *testing.T) {
	if os.Getenv("SYTUE_MERGE_OWNER_FIXTURE") != "1" {
		t.Skip("subprocess fixture")
	}
	var request mergeOwnershipProcessRequest
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		t.Fatal("invalid private fixture input")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	owner, err := merge.AcquireOwnership(ctx, request.DSN, request.Job)
	if err != nil {
		t.Fatal("fixture could not own operation")
	}
	defer func() { _ = owner.Close(ctx) }()
	if _, err := merge.PrepareOwnedAdmission(ctx, owner, "source", "target"); err != nil {
		t.Fatal("fixture could not prepare operation")
	}
	for _, service := range merge.Services {
		if err := owner.Service(service).InMergeTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE untouched_domain_rows SET tenant='target' WHERE tenant='source'`)
			return err
		}); err != nil {
			t.Fatal("fixture domain phase failed")
		}
	}
	if err := owner.Identity().InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE organizations SET status='merged',merged_into_org_id=$2 WHERE id=$1`, request.SourceID, request.TargetID)
		return err
	}); err != nil {
		t.Fatal("fixture identity phase failed")
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]string{"phase": "committed-owner-held"}); err != nil {
		t.Fatal("fixture signal failed")
	}
	var stop string
	if err := json.NewDecoder(os.Stdin).Decode(&stop); err != nil {
		t.Fatal("fixture stopped without command")
	}
}

func TestRunningMergeRecoveryAfterRealOwnerProcessDeath(t *testing.T) {
	fixture := runningMergeFixture(t)
	server, identity, services, source, target, job := fixture.server, fixture.identity, fixture.services, fixture.source, fixture.target, fixture.job
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	process := exec.CommandContext(ctx, executable, "-test.run=^TestMergeOwnershipProcessFixture$", "-test.v=false") //nolint:gosec // The executable is this exact test binary; no user input is evaluated.
	process.Env = []string{"SYTUE_MERGE_OWNER_FIXTURE=1", "GOMAXPROCS=2", "GOMEMLIMIT=1GiB"}
	input, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := process.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	process.Stderr = io.Discard
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close(); _ = output.Close(); _ = process.Process.Kill(); _ = process.Wait() })
	if err := json.NewEncoder(input).Encode(mergeOwnershipProcessRequest{identity.AppDSN, job, source, target}); err != nil {
		t.Fatal("send fixture routing", err)
	}
	var signal map[string]string
	if err := json.NewDecoder(output).Decode(&signal); err != nil || signal["phase"] != "committed-owner-held" {
		t.Fatal("process did not reach committed ownership barrier")
	}
	if err := server.RecoverOrgMerges(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := identity.Pool.QueryRow(ctx, `SELECT status FROM org_merges WHERE id=$1`, job).Scan(&status); err != nil || status != "running" {
		t.Fatal("competing process stole live ownership", status, err)
	}
	if err := process.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = process.Wait()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := server.RecoverOrgMerges(ctx); err != nil {
			t.Fatal(err)
		}
		if err := identity.Pool.QueryRow(ctx, `SELECT status FROM org_merges WHERE id=$1`, job).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "completed" {
			break
		}
		if status != "running" {
			t.Fatal("crash recovery failed", status)
		}
		select {
		case <-ctx.Done():
			t.Fatal("dead process ownership not recovered", ctx.Err())
		case <-ticker.C:
		}
	}
	var state string
	if err := services["rooms"].PoolSinRLS(t).QueryRow(ctx, `SELECT state FROM private_scope_admission_control WHERE scope_key='source'`).Scan(&state); err != nil || state != "retired" {
		t.Fatal("process recovery reopened retired source", state, err)
	}
	for service, h := range services {
		var count int
		if err := h.PoolSinRLS(t).QueryRow(ctx, `SELECT count(*) FROM untouched_domain_rows WHERE tenant='target'`).Scan(&count); err != nil || count != 2 {
			t.Fatal("recovery repeated or lost committed domain phase", service, count, err)
		}
	}
}
