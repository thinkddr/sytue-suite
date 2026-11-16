// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// AuthorizeSession attaches a CURRENT independently enrolled device to the
// same historical user/root/product role. It never rewrites the grant issuer,
// grants a private epoch key, or permits B to publish as historical device A.
// Publishing with B requires a capability in a roster that actually enrolls B;
// managing recovery may instead prepare a signed next epoch and fresh checkpoint.
func AuthorizeSession(ctx context.Context, epoch VerifiedEpoch, capability VerifiedCapability, session VerifiedSession, action Action) (VerifiedAccess, error) {
	access, err := authorizeRole(epoch, capability, session.Scope().Subject, action)
	if err != nil {
		return VerifiedAccess{}, err
	}
	member := capability.member
	binding := session.Binding()
	if !session.valid() || binding.Scope != member.Scope || binding.UserID != member.UserID || binding.RootKey != member.RootKey {
		return VerifiedAccess{}, ErrInvalid
	}
	if err := session.Revalidate(ctx); err != nil {
		return VerifiedAccess{}, err
	}
	if epoch.restored != nil {
		if err := epoch.restored.revalidate(ctx, session); err != nil {
			return VerifiedAccess{}, err
		}
	}
	copied := session
	access.session = &copied
	return access, nil
}

// Actor and Scope distinguish current producer B from historical grant issuer A.
// The latter stays available on Capability().Enrollment() for history proofs.
func (a VerifiedAccess) Actor() Actor {
	if !a.valid() {
		return Actor{}
	}
	if a.session != nil {
		return a.session.Actor()
	}
	return a.capability.enrollment.Issuer
}

func (a VerifiedAccess) Scope() storage.Scope {
	if !a.valid() {
		return storage.Scope{}
	}
	return a.capability.member.Scope
}

func (a VerifiedAccess) CurrentSessionBinding() (SessionBinding, bool) {
	if !a.valid() || a.session == nil || !a.session.valid() {
		return SessionBinding{}, false
	}
	return a.session.Binding(), true
}

func (a VerifiedAccess) RevalidateSession(ctx context.Context) error {
	if !a.valid() || a.session == nil {
		return ErrRevoked
	}
	if err := a.session.Revalidate(ctx); err != nil {
		return err
	}
	if a.epoch.restored != nil {
		return a.epoch.restored.revalidate(ctx, *a.session)
	}
	return ctx.Err()
}

func revalidateAccessSession(ctx context.Context, a VerifiedAccess) error {
	if a.session != nil {
		return a.RevalidateSession(ctx)
	}
	return ctx.Err()
}

// VerifyNextEpochWithSession verifies B's OWN SDK signature while retaining
// A's prior policy/capability as immutable role authority. It requires a fresh
// resource epoch recipient and independently signed checkpoint via the same
// existing baseline-transition checks. No existing strict verifier falls back.
func VerifyNextEpochWithSession(ctx context.Context, value SignedEpoch, roster VerifiedRoster, device matrixproof.Device, previous VerifiedEpoch, manager VerifiedCapability, cutover Position, current VerifiedSession) (VerifiedEpoch, error) {
	if !current.valid() || device.Identity() != current.Device().Identity() {
		return VerifiedEpoch{}, ErrInvalid
	}
	verified, err := verifyNextEpoch(ctx, value, roster, device, previous, manager, cutover, &current)
	if err != nil {
		return VerifiedEpoch{}, err
	}
	if err := current.Revalidate(ctx); err != nil {
		return VerifiedEpoch{}, err
	}
	return verified, nil
}

// Reauthorize preserves the independently verified CURRENT device when a host
// repeats the SAME verified action for its authenticated subject. Rebuilding from
// Epoch/Capability alone would discard B and revive historical issuer A.
func Reauthorize(ctx context.Context, access VerifiedAccess, subject string, action Action) (VerifiedAccess, error) {
	if !access.valid() || access.action != action {
		return VerifiedAccess{}, ErrInvalid
	}
	if access.session != nil {
		if access.session.Scope().Subject != subject {
			return VerifiedAccess{}, ErrInvalid
		}
		return AuthorizeSession(ctx, access.epoch, access.capability, *access.session, action)
	}
	if err := ctx.Err(); err != nil {
		return VerifiedAccess{}, err
	}
	return Authorize(access.epoch, access.capability, subject, action)
}
