// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pg_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

type ingressAuthorityFixture struct {
	keys              *matrixprooftest.Fixture
	authority         matrixproof.Authority
	failOnCall, calls int
}

func (f *ingressAuthorityFixture) ReadStorageAuthority(_ context.Context, scope storage.Scope) (matrixproof.Authority, error) {
	if f.authority.Validate(scope) != nil {
		return matrixproof.Authority{}, matrixproof.ErrUnverified
	}
	return f.authority, nil
}

func (f *ingressAuthorityFixture) VerifyStorageAction(ctx context.Context, expected matrixproof.Action, proof matrixproof.SignedAction) (matrixproof.Authorization, error) {
	f.calls++
	if f.failOnCall > 0 && f.calls >= f.failOnCall {
		return matrixproof.Authorization{}, matrixproof.ErrUnverified
	}
	if f.authority.RequireActive(expected.Scope, expected.Generation) != nil {
		return matrixproof.Authorization{}, matrixproof.ErrUnverified
	}
	device, err := matrixproof.Verify(ctx, f.keys.Origin, f.keys.Master.PublicKey())
	if err != nil {
		return matrixproof.Authorization{}, err
	}
	if err := device.VerifyAction(ctx, proof, expected, time.Now()); err != nil {
		return matrixproof.Authorization{}, err
	}
	result := matrixproof.Authorization{Version: 1, Authority: f.authority, Target: expected.Target, SigningKey: f.keys.Device.PublicKey().String(), Nonce: expected.Nonce, RequestSHA256: expected.RequestSHA256}
	return result, result.Validate(expected)
}

func newIngressControlFixture(t *testing.T) (*ingressHarness, pg.AuthorizedPrivateIngressConfig, *ingressAuthorityFixture, ingress.ControlRequest) {
	t.Helper()
	h := newIngressHarness(t)
	keys := matrixprooftest.New(t, h.cfg.Scope, "test", "DEVICE_B")
	reader := &ingressAuthorityFixture{keys: keys, authority: matrixproof.Authority{Version: 1, Scope: h.cfg.Scope, UserID: keys.Origin.UserID.String(), RootKey: keys.Master.PublicKey().String(), Generation: 1, Status: "active"}}
	cfg := pg.AuthorizedPrivateIngressConfig{PrivateIngressConfig: h.cfg, Authority: reader}
	request := ingress.ControlRequest{Version: 1, Scope: h.cfg.Scope, Epoch: h.cfg.Epoch.String(), Kind: "key.enroll", Target: storage.PublicationTarget{RoomID: h.target.RoomID, DeviceID: "DEVICE_B"}, Enrollment: &h.plan}
	return h, cfg, reader, request
}

func ingressControlChallenge(t *testing.T, h *ingressHarness, cfg pg.AuthorizedPrivateIngressConfig, request ingress.ControlRequest, keys *matrixprooftest.Fixture) matrixproof.SignedAction {
	t.Helper()
	var action matrixproof.Action
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		action, err = pg.ChallengePrivateIngress(ctx, tx, cfg, request)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	proof := matrixproof.SignedAction{Action: action}
	proof.Signatures = matrixprooftest.Sign(t, keys.Device, action, keys.Origin.UserID, keys.Origin.DeviceID.String())
	return proof
}

func ingressApplyControl(t *testing.T, h *ingressHarness, cfg pg.AuthorizedPrivateIngressConfig, request ingress.ControlRequest, proof matrixproof.SignedAction) (ingress.ControlResult, error) {
	t.Helper()
	var result ingress.ControlResult
	err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		result, err = pg.ApplyPrivateIngressAction(ctx, tx, cfg, request, proof)
		return err
	})
	return result, err
}

func TestPrivateIngressNonceCommitsWithEnrollmentClaimAndAcknowledgment(t *testing.T) {
	h, cfg, reader, enrollment := newIngressControlFixture(t)
	proof := ingressControlChallenge(t, h, cfg, enrollment, reader.keys)
	for range 2 {
		if _, err := ingressApplyControl(t, h, cfg, enrollment, proof); err != nil {
			t.Fatal("same signed enrollment retry", err)
		}
	}
	changed := enrollment
	changed.Target.DeviceID = "OTHER"
	if _, err := ingressApplyControl(t, h, cfg, changed, proof); err == nil {
		t.Fatal("consumed nonce accepted another device")
	}
	h.seal(t)
	request := ingress.ControlRequest{Version: 1, Scope: h.cfg.Scope, Epoch: h.cfg.Epoch.String(), Kind: "task.claim", Target: enrollment.Target, Claim: &ingress.ClaimRequest{TaskID: h.task.ID, Token: "retained_claim", LeaseSeconds: 60}}
	proof = ingressControlChallenge(t, h, cfg, request, reader.keys)
	result, err := ingressApplyControl(t, h, cfg, request, proof)
	if err != nil || result.Claim == nil {
		t.Fatal(err)
	}
	claim := *result.Claim
	retry, err := ingressApplyControl(t, h, cfg, request, proof)
	if err != nil || retry.Claim == nil || retry.Claim.Token != claim.Token || retry.Claim.Generation != claim.Generation {
		t.Fatal("claim retry changed original transaction", err)
	}
	receipt := ingress.Receipt{Version: 1, Scope: h.cfg.Scope, Epoch: h.cfg.Epoch.String(), TaskID: h.task.ID, CipherSHA256: h.task.Cipher.SHA256, ClaimGeneration: claim.Generation, Commit: storage.Reference{RoomID: claim.Target.RoomID, EventID: "$encrypted_receipt"}}
	ack := ingress.ControlRequest{Version: 1, Scope: h.cfg.Scope, Epoch: h.cfg.Epoch.String(), Kind: "task.ack", Target: claim.Target, Acknowledgment: &ingress.AcknowledgmentRequest{Claim: claim, Receipt: receipt}}
	ackProof := ingressControlChallenge(t, h, cfg, ack, reader.keys)
	for range 2 {
		got, err := ingressApplyControl(t, h, cfg, ack, ackProof)
		if err != nil || got.Receipt == nil || *got.Receipt != receipt {
			t.Fatal("ACK retry", err)
		}
	}
	reader.authority.Status = "revoked"
	reader.authority.Generation++
	if _, err := ingressApplyControl(t, h, cfg, ack, ackProof); err == nil {
		t.Fatal("old consumed proof remained valid after revocation")
	}
}

func TestPrivateIngressSecondAuthorityCheckRollsBackMutationAndNonce(t *testing.T) {
	h, cfg, reader, request := newIngressControlFixture(t)
	proof := ingressControlChallenge(t, h, cfg, request, reader.keys)
	reader.failOnCall = 2
	if _, err := ingressApplyControl(t, h, cfg, request, proof); err == nil {
		t.Fatal("revocation during mutation committed")
	}
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
			return err
		}
		var count int
		var consumed *time.Time
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM private_ingress_devices WHERE device_id=$1`, request.Target.DeviceID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("failed authorization left enrolled device")
		}
		if err := tx.QueryRow(ctx, `SELECT consumed_at FROM private_ingress_actions WHERE nonce=$1`, proof.Nonce).Scan(&consumed); err != nil {
			return err
		}
		if consumed != nil {
			return errors.New("failed authorization consumed nonce")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reader.failOnCall = 0
	if _, err := ingressApplyControl(t, h, cfg, request, proof); err != nil {
		t.Fatal("rolled back original did not recover", err)
	}
}
