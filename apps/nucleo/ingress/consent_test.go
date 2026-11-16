// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type consentFixture struct {
	request                                                  matrixproof.RootEnrollment
	authority                                                matrixproof.Authority
	readErr                                                  error
	challengeErr                                             error
	changedChallenge                                         bool
	valid                                                    bool
	leases, approvals, challenges, signatures, confirmations int
}

func (f *consentFixture) StorageRootConsentRequest(ctx context.Context, lease offline.WorkLease, _ storage.Scope, _ storage.PublicationTarget, kind string, previous *matrixproof.Authority) (matrixproof.RootEnrollment, error) {
	release, err := lease(ctx)
	if err != nil {
		return matrixproof.RootEnrollment{}, err
	}
	defer release()
	request := f.request
	if kind == "root.resume" {
		request.PreviousGeneration = previous.Generation
		request.PreviousRootKey = previous.RootKey
		request.Generation = previous.Generation + 1
	}
	return request, nil
}

func (f *consentFixture) SignIngressAction(_ context.Context, _ offline.WorkLease, expected matrixproof.Intent, action matrixproof.Action) (matrixproof.SignedAction, error) {
	f.signatures++
	if action.Intent() != expected {
		return matrixproof.SignedAction{}, matrixproof.ErrUnverified
	}
	return matrixproof.SignedAction{Action: action}, nil
}

func (f *consentFixture) MatrixStorageAuthority(context.Context, storage.Scope) (matrixproof.Authority, error) {
	return f.authority, f.readErr
}

func (f *consentFixture) MatrixStorageRootChallenge(_ context.Context, scope storage.Scope, epoch, kind string, request matrixproof.RootEnrollment) (matrixproof.RootChallenge, error) {
	f.challenges++
	if f.challengeErr != nil {
		return matrixproof.RootChallenge{}, f.challengeErr
	}
	generation := request.Generation
	if kind == "root.resume" {
		generation = request.PreviousGeneration
	}
	digest, err := matrixproof.RequestDigest(request)
	if err != nil {
		return matrixproof.RootChallenge{}, err
	}
	now := time.Now()
	action := matrixproof.Action{Version: 1, Generation: generation, Scope: scope, Epoch: epoch, Kind: kind, Target: request.Target, RootKey: request.RootKey, RequestSHA256: digest, Nonce: "consent_fixture", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()}
	if f.changedChallenge {
		action.Generation++
	}
	return matrixproof.RootChallenge{Action: action, Request: request}, nil
}

func (f *consentFixture) ConfirmMatrixStorageRoot(_ context.Context, _ storage.Scope, proof matrixproof.SignedAction) (matrixproof.Authority, error) {
	f.confirmations++
	generation := proof.Generation
	if proof.Kind == "root.resume" {
		generation++
	}
	return matrixproof.Authority{Version: 1, Scope: proof.Scope, UserID: f.request.UserID, RootKey: proof.RootKey, Generation: generation, Status: "active"}, nil
}

func newConsentFixture(t *testing.T) (*consentFixture, RootConsent) {
	t.Helper()
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	request := matrixproof.RootEnrollment{Version: 1, Scope: scope, UserID: storage.IsolatedMatrixUserPrefix(scope) + "matrix.test", Target: storage.PublicationTarget{RoomID: "!inbox:matrix.test", DeviceID: "DEVICE"}, RootKey: strings.Repeat("A", 43), Generation: 1}
	f := &consentFixture{request: request, valid: true, readErr: &api.Error{Status: http.StatusConflict, Body: `{"error":{"code":"storage_root_enrollment_required"}}`}}
	c := RootConsent{
		Scope: scope, Target: request.Target, Device: f, Service: f,
		Lease: func(ctx context.Context) (func(), error) { f.leases++; return func() { f.leases-- }, ctx.Err() },
		Authorize: func(ctx context.Context) error {
			if !f.valid {
				return matrixproof.ErrUnverified
			}
			return ctx.Err()
		},
		Approve: func(context.Context, matrixproof.RootEnrollment) error {
			f.approvals++
			if f.leases != 0 {
				t.Fatal("device lease held across human prompt")
			}
			return nil
		},
	}
	return f, c
}

func TestRootConsentBindsHumanApprovalAndOriginalGeneration(t *testing.T) {
	for _, kind := range []string{"root.enroll", "root.resume"} {
		t.Run(kind, func(t *testing.T) {
			f, c := newConsentFixture(t)
			generation := int64(1)
			if kind == "root.resume" {
				f.readErr = nil
				f.authority = matrixproof.Authority{Version: 1, Scope: c.Scope, UserID: f.request.UserID, RootKey: f.request.RootKey, Generation: 2, Status: "revoked"}
				generation = 3
			}
			result, err := c.Ensure(t.Context())
			if err != nil || result.Generation != generation || f.approvals != 1 || f.challenges != 1 || f.signatures != 1 || f.confirmations != 1 {
				t.Fatal("consent flow changed", result, err, f)
			}
		})
	}
}

func TestRootConsentNeverTreatsUnavailableAuthorityAsFirstEnrollment(t *testing.T) {
	for _, failure := range []error{errors.New("offline"), &api.Error{Status: 404}, &api.Error{Status: 403}, &api.Error{Status: 500}, &api.Error{Status: 409, Body: `{"error":{"code":"other_conflict"}}`}} {
		f, c := newConsentFixture(t)
		f.readErr = failure
		if _, err := c.Ensure(t.Context()); err == nil || f.approvals != 0 || f.challenges != 0 || f.signatures != 0 {
			t.Fatal("unavailable authority authorized enrollment", failure, err)
		}
	}
}

func TestRootConsentStopsBeforeChallengeWhenSessionChangesInPrompt(t *testing.T) {
	f, c := newConsentFixture(t)
	c.Approve = func(context.Context, matrixproof.RootEnrollment) error { f.valid = false; return nil }
	if _, err := c.Ensure(t.Context()); err == nil || f.challenges != 0 || f.signatures != 0 {
		t.Fatal("stale consent reached network signing", err, f)
	}
}

func TestRootConsentRejectsChangedChallengeAndDifferentRoot(t *testing.T) {
	f, c := newConsentFixture(t)
	f.changedChallenge = true
	if _, err := c.Ensure(t.Context()); err == nil || f.signatures != 0 {
		t.Fatal("signed substituted authority generation", err)
	}
	f, c = newConsentFixture(t)
	f.readErr = nil
	f.authority = matrixproof.Authority{Version: 1, Scope: c.Scope, UserID: f.request.UserID, RootKey: strings.Repeat("B", 43), Generation: 1, Status: "active"}
	if _, err := c.Ensure(t.Context()); err == nil || f.approvals != 0 || f.signatures != 0 {
		t.Fatal("different root became an implicit rotation", err)
	}
}

func TestRootConsentReusesMatchingActiveRootWithoutAnotherPrompt(t *testing.T) {
	f, c := newConsentFixture(t)
	f.readErr = nil
	f.authority = matrixproof.Authority{Version: 1, Scope: c.Scope, UserID: f.request.UserID, RootKey: f.request.RootKey, Generation: 3, Status: "active"}
	result, err := c.Ensure(t.Context())
	if err != nil || result != f.authority || f.approvals != 0 || f.challenges != 0 {
		t.Fatal("active root prompted again", result, err)
	}
}
