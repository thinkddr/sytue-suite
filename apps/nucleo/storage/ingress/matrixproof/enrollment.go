// SPDX-License-Identifier: Apache-2.0
package matrixproof

import (
	"context"
	"time"

	"maunium.net/go/mautrix/id"
)

// EnrollmentRoot is a candidate from the configured Matrix origin, not an
// enrolled trust anchor. It authorizes no queue operation or existing pin reset.
func EnrollmentRoot(origin Origin) (id.Ed25519, error) {
	if !expectedOrigin(origin) {
		return "", ErrUnverified
	}
	master, ok := origin.Keys.MasterKeys[origin.UserID]
	if !ok {
		return "", ErrUnverified
	}
	return crossKey(master, origin.UserID, id.XSUsageMaster)
}

// VerifyRootConsent is deliberately separate from ordinary authorization. It
// admits only an explicit root.enroll transcript and a complete signed device
// chain. The caller must atomically consume Rooms' retained challenge and
// insert generation 1 only when that scope has no existing root.
func VerifyRootConsent(ctx context.Context, origin Origin, proof SignedAction, expected RootChallenge, now time.Time) error {
	request, action := expected.Request, expected.Action
	root, err := EnrollmentRoot(origin)
	if err != nil {
		return err
	}
	if request.PreviousGeneration != 0 || request.PreviousRootKey != "" || action.Generation != 1 || request.Version != 1 || request.Generation != 1 || request.Scope != origin.Scope || request.UserID != origin.UserID.String() || request.RootKey != root.String() || request.Target != action.Target || request.Target.DeviceID != origin.DeviceID.String() || action.Kind != "root.enroll" || action.Scope != origin.Scope || action.RootKey != root.String() {
		return ErrUnverified
	}
	digest, err := RequestDigest(request)
	if err != nil || action.RequestSHA256 != digest {
		return ErrUnverified
	}
	device, err := Verify(ctx, origin, root)
	if err != nil {
		return err
	}
	return device.VerifyAction(ctx, proof, action, now)
}

// EnrollmentUserRoot is only a candidate for an explicit canonical-user root
// consent. It verifies the same live SDK chain, returns no enrollment or Device,
// and cannot replace a previously pinned root. expectedUser comes from the
// independently resolved SytueID actor, never the enrollment body.
func EnrollmentUserRoot(ctx context.Context, origin Origin, expectedUser id.UserID) (id.Ed25519, error) {
	master, ok := origin.Keys.MasterKeys[expectedUser]
	if !ok {
		return "", ErrUnverified
	}
	root, err := crossKey(master, expectedUser, id.XSUsageMaster)
	if err != nil {
		return "", err
	}
	if _, err := VerifyUser(ctx, origin, expectedUser, root); err != nil {
		return "", err
	}
	return root, nil
}
