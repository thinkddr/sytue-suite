// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pg_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// This fixture exercises real SQL/transcripts, not a Matrix deletion proof.
// The disposable Synapse joint test owns that independent guarantee.
type ingressFenceFixture struct {
	*ingressAuthorityFixture
	fence                   *ingress.DeviceFence
	failFence, failRead     bool
	fences, reads, receipts int
}

func (f *ingressFenceFixture) FenceStorageDevice(ctx context.Context, proof ingress.ControlProof) (ingress.DeviceFence, error) {
	f.fences++
	if proof.Request.Handoff.Phase != "begin" {
		return ingress.DeviceFence{}, ingress.ErrContract
	}
	if _, err := f.VerifyStorageAction(ctx, proof.Proof.Action, proof.Proof); err != nil {
		return ingress.DeviceFence{}, err
	}
	if f.failFence {
		return ingress.DeviceFence{}, errors.New("uncertain remote fence")
	}
	if f.fence == nil {
		digest, err := matrixproof.RequestDigest(proof.Request)
		if err != nil {
			return ingress.DeviceFence{}, err
		}
		now := time.Now().UTC()
		h := proof.Request.Handoff
		f.fence = &ingress.DeviceFence{Version: 1, Scope: proof.Request.Scope, Epoch: proof.Request.Epoch, ID: h.ID, TaskID: h.TaskID, CipherSHA256: h.CipherSHA256, Previous: h.Previous, Target: proof.Request.Target, Authority: f.authority, RequestSHA256: digest, Status: "confirmed", ConfirmedAt: &now}
	}
	return *f.fence, nil
}

func (f *ingressFenceFixture) ReadStorageDeviceFence(ctx context.Context, proof ingress.ControlProof) (ingress.DeviceFence, error) {
	f.reads++
	if f.failRead || f.fence == nil {
		return ingress.DeviceFence{}, ingress.ErrRevoked
	}
	if _, err := f.VerifyStorageAction(ctx, proof.Proof.Action, proof.Proof); err != nil {
		return ingress.DeviceFence{}, err
	}
	return *f.fence, f.fence.Validate(proof.Request, f.authority)
}

func (f *ingressFenceFixture) VerifyStorageReceipt(_ context.Context, _ storage.Scope, _ storage.PublicationTarget, _ storage.Reference) error {
	f.receipts++
	return nil
}

type handoffFixture struct {
	h       *ingressHarness
	cfg     pg.AuthorizedPrivateIngressConfig
	fencer  *ingressFenceFixture
	queue   pg.PrivateIngressQueue
	request ingress.ControlRequest
	ctx     context.Context
}

func ingressHandoffFixture(t *testing.T) handoffFixture {
	t.Helper()
	h, cfg, reader, enroll := newIngressControlFixture(t)
	fencer := &ingressFenceFixture{ingressAuthorityFixture: reader}
	cfg.Authority = fencer
	proof := ingressControlChallenge(t, h, cfg, enroll, reader.keys)
	if _, err := ingressApplyControl(t, h, cfg, enroll, proof); err != nil {
		t.Fatal(err)
	}
	h.seal(t)
	_ = h.claim(t)
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE private_ingress_tasks SET lease_until=now()-interval '1 minute' WHERE id=$1`, h.task.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	queue := ingressQueueFixture(h, cfg)
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: cfg.Scope.Tenant})
	state, err := queue.Inspect(ctx, cfg.Scope, cfg.Epoch.String(), h.task.ID)
	if err != nil || state.Assignment == nil || state.Plan != h.plan {
		t.Fatal("original public inspection", err)
	}
	request := ingress.ControlRequest{Version: 1, Scope: cfg.Scope, Epoch: cfg.Epoch.String(), Kind: "task.handoff", Target: enroll.Target, Handoff: &ingress.HandoffRequest{ID: uuid.NewString(), Phase: "begin", TaskID: h.task.ID, CipherSHA256: h.task.Cipher.SHA256, Previous: *state.Assignment, Token: "new_device_secret"}}
	return handoffFixture{h, cfg, fencer, queue, request, ctx}
}

func TestPrivateIngressHandoffRetainsOriginalBeforeRemoteIOAndRequiresFreshFence(t *testing.T) {
	fixture := ingressHandoffFixture(t)
	h, cfg, fencer, queue, request, ctx := fixture.h, fixture.cfg, fixture.fencer, fixture.queue, fixture.request, fixture.ctx
	proof := ingressControlChallenge(t, h, cfg, request, fencer.keys)
	fencer.failFence = true
	if _, err := queue.Apply(ctx, cfg.Scope, request, proof); err == nil {
		t.Fatal("uncertain fence granted publication")
	}
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM private_ingress_claim_history WHERE task_id=$1 AND claim_generation=$2`, h.task.ID, request.Handoff.Previous.Generation).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return errors.New("lost original claim before remote I/O")
		}
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM private_ingress_handoffs WHERE id=$1`, request.Handoff.ID).Scan(&status); err != nil {
			return err
		}
		if status != "fencing" {
			return errors.New("uncertain fence advanced state")
		}
		_, err := pg.ClaimPrivateIngress(ctx, tx, h.cfg, h.task.ID, h.target, "private_claim_01", time.Minute)
		if !errors.Is(err, ingress.ErrRevoked) {
			return errors.New("old device can still renew its claim")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	fencer.failFence = false
	result, err := queue.Apply(ctx, cfg.Scope, request, proof)
	if err != nil || result.Handoff == nil || result.Handoff.Status != "readable" || result.Handoff.Claim != nil {
		t.Fatal("confirmed fence did not grant only read", err)
	}
	if _, err := queue.Revalidate(ctx, cfg.Scope, request, proof, ingress.Claim{}); err == nil {
		t.Fatal("readable handoff granted publication")
	}
	resolve := request
	handoff := *request.Handoff
	handoff.Phase = "resolve"
	handoff.Resolution = "absent"
	handoff.LeaseSeconds = 60
	resolve.Handoff = &handoff
	resolveProof := ingressControlChallenge(t, h, cfg, resolve, fencer.keys)
	fencer.failRead = true
	if _, err := queue.Apply(ctx, cfg.Scope, resolve, resolveProof); err == nil {
		t.Fatal("cached fence bypassed fresh remote absence")
	}
	fencer.failRead = false
	result, err = queue.Apply(ctx, cfg.Scope, resolve, resolveProof)
	if err != nil || result.Handoff == nil || result.Handoff.Claim == nil {
		t.Fatal("fresh fence did not grant reconciled target", err)
	}
	claim := *result.Handoff.Claim
	if claim.Target != request.Target || claim.Token != handoff.Token || claim.Generation != handoff.Previous.Generation+1 {
		t.Fatal("CAS changed original destination or generation")
	}
	altered := resolve
	alteredHandoff := *resolve.Handoff
	alteredHandoff.LeaseSeconds = 30
	altered.Handoff = &alteredHandoff
	alteredProof := ingressControlChallenge(t, h, cfg, altered, fencer.keys)
	if _, err := queue.Apply(ctx, cfg.Scope, altered, alteredProof); err == nil {
		t.Fatal("resolved request changed after its grant")
	}
	for _, table := range []string{"private_ingress_claim_history", "private_ingress_handoffs"} {
		if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
			if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "DELETE FROM "+table)
			return err
		}); err == nil {
			t.Fatalf("%s erased original handoff", table)
		}
	}
	// A changed signed destination cannot reuse a retained handoff ID/token.
	changed := resolve
	changed.Target.RoomID = "!changed:test"
	changedProof := ingressControlChallenge(t, h, cfg, changed, fencer.keys)
	if _, err := queue.Apply(ctx, cfg.Scope, changed, changedProof); err == nil {
		t.Fatal("changed target reused immutable handoff")
	}
}

func TestPrivateIngressHandoffReconcilesOriginalCommitWithoutNewClaim(t *testing.T) {
	fixture := ingressHandoffFixture(t)
	h, cfg, fencer, queue, request, ctx := fixture.h, fixture.cfg, fixture.fencer, fixture.queue, fixture.request, fixture.ctx
	proof := ingressControlChallenge(t, h, cfg, request, fencer.keys)
	if _, err := queue.Apply(ctx, cfg.Scope, request, proof); err != nil {
		t.Fatal(err)
	}
	resolve := request
	handoff := *request.Handoff
	handoff.Phase = "resolve"
	handoff.Resolution = "committed"
	handoff.Receipt = &ingress.Receipt{Version: 1, Scope: cfg.Scope, Epoch: cfg.Epoch.String(), TaskID: h.task.ID, CipherSHA256: h.task.Cipher.SHA256, ClaimGeneration: handoff.Previous.Generation, Commit: storage.Reference{RoomID: handoff.Previous.Target.RoomID, EventID: "$accepted_before_crash"}}
	resolve.Handoff = &handoff
	for range 2 {
		resolveProof := ingressControlChallenge(t, h, cfg, resolve, fencer.keys)
		result, err := queue.Apply(ctx, cfg.Scope, resolve, resolveProof)
		if err != nil || result.Handoff == nil || result.Handoff.Status != "acknowledged" || result.Handoff.Claim != nil || result.Handoff.Receipt == nil || *result.Handoff.Receipt != *handoff.Receipt {
			t.Fatal("original commit did not reconcile idempotently", err)
		}
	}
	if fencer.receipts != 1 {
		t.Fatal("receipt not independently verified before first ACK")
	}
	state, err := queue.Inspect(ctx, cfg.Scope, cfg.Epoch.String(), h.task.ID)
	if err != nil || state.Receipt == nil || *state.Receipt != *handoff.Receipt || state.Assignment == nil || !state.Assignment.Same(handoff.Previous) {
		t.Fatal("ACK replaced original assignment", err)
	}
}
