// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type RootConsentDevice interface {
	StorageRootConsentRequest(context.Context, offline.WorkLease, storage.Scope, storage.PublicationTarget, string, *matrixproof.Authority) (matrixproof.RootEnrollment, error)
	SignIngressAction(context.Context, offline.WorkLease, matrixproof.Intent, matrixproof.Action) (matrixproof.SignedAction, error)
}

type RootConsentService interface {
	MatrixStorageAuthority(context.Context, storage.Scope) (matrixproof.Authority, error)
	MatrixStorageRootChallenge(context.Context, storage.Scope, string, string, matrixproof.RootEnrollment) (matrixproof.RootChallenge, error)
	ConfirmMatrixStorageRoot(context.Context, storage.Scope, matrixproof.SignedAction) (matrixproof.Authority, error)
}

// RootConsent keeps the original device, session witness and public intent across
// the human prompt. Approve must require an explicit action. No work lease is
// held while waiting for the person, so logout can cancel an abandoned prompt.
type RootConsent struct {
	Scope     storage.Scope
	Target    storage.PublicationTarget
	Device    RootConsentDevice
	Service   RootConsentService
	Lease     offline.WorkLease
	Authorize func(context.Context) error
	Approve   func(context.Context, matrixproof.RootEnrollment) error
}

// Ensure returns public routing metadata, never a publication permit. Workers
// must still revalidate the retained authority generation for every operation.
// A different root requires recovery; this flow never resets or rotates keys.
func (c RootConsent) Ensure(ctx context.Context) (matrixproof.Authority, error) {
	if !c.Scope.Valid() || !c.Target.Valid() || c.Device == nil || c.Service == nil || c.Lease == nil || c.Authorize == nil || c.Approve == nil {
		return matrixproof.Authority{}, matrixproof.ErrUnverified
	}
	if err := c.Authorize(ctx); err != nil {
		return matrixproof.Authority{}, err
	}
	own, err := c.Device.StorageRootConsentRequest(ctx, c.Lease, c.Scope, c.Target, "root.enroll", nil)
	if err != nil {
		return matrixproof.Authority{}, err
	}
	request, kind, active, err := c.plan(ctx, own)
	if err != nil {
		return matrixproof.Authority{}, err
	}
	if active != nil {
		if err := c.Authorize(ctx); err != nil {
			return matrixproof.Authority{}, err
		}
		return *active, nil
	}
	if err := c.Approve(ctx, request); err != nil {
		return matrixproof.Authority{}, err
	}
	if err := c.Authorize(ctx); err != nil {
		return matrixproof.Authority{}, err
	}
	return c.confirm(ctx, request, kind)
}

func (c RootConsent) confirm(ctx context.Context, request matrixproof.RootEnrollment, kind string) (matrixproof.Authority, error) {
	epoch := uuid.NewString()
	generation := request.Generation
	if kind == "root.resume" {
		generation = request.PreviousGeneration
	}
	intent, err := matrixproof.IntentFor(c.Scope, generation, epoch, kind, c.Target, request)
	if err != nil {
		return matrixproof.Authority{}, err
	}
	challenge, err := c.Service.MatrixStorageRootChallenge(ctx, c.Scope, epoch, kind, request)
	if err != nil {
		return matrixproof.Authority{}, err
	}
	if challenge.Request != request || challenge.Action.Intent() != intent || challenge.Action.RootKey != request.RootKey {
		return matrixproof.Authority{}, matrixproof.ErrUnverified
	}
	proof, err := c.Device.SignIngressAction(ctx, c.Lease, intent, challenge.Action)
	if err != nil {
		return matrixproof.Authority{}, err
	}
	if err := c.Authorize(ctx); err != nil {
		return matrixproof.Authority{}, err
	}
	confirmed, err := c.Service.ConfirmMatrixStorageRoot(ctx, c.Scope, proof)
	if err != nil {
		return matrixproof.Authority{}, err
	}
	if confirmed.RequireActive(c.Scope, request.Generation) != nil || confirmed.RootKey != request.RootKey || confirmed.UserID != request.UserID {
		return matrixproof.Authority{}, matrixproof.ErrUnverified
	}
	if err := c.Authorize(ctx); err != nil {
		return matrixproof.Authority{}, err
	}
	return confirmed, nil
}

// plan distinguishes an explicit missing-root response from all unavailable or
// conflicting authority states. Existing roots are never replaced by the UI.
func (c RootConsent) plan(ctx context.Context, own matrixproof.RootEnrollment) (matrixproof.RootEnrollment, string, *matrixproof.Authority, error) {
	authority, err := c.Service.MatrixStorageAuthority(ctx, c.Scope)
	if err != nil {
		var failure *api.Error
		if !errors.As(err, &failure) || failure.Status != http.StatusConflict || failure.Code() != "storage_root_enrollment_required" {
			return matrixproof.RootEnrollment{}, "", nil, err
		}
		return own, "root.enroll", nil, nil
	}
	if authority.Validate(c.Scope) != nil || authority.UserID != own.UserID || authority.RootKey != own.RootKey {
		return matrixproof.RootEnrollment{}, "", nil, matrixproof.ErrUnverified
	}
	if authority.Status == "active" {
		return own, "", &authority, nil
	}
	request, err := c.Device.StorageRootConsentRequest(ctx, c.Lease, c.Scope, c.Target, "root.resume", &authority)
	return request, "root.resume", nil, err
}
