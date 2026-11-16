// SPDX-License-Identifier: Apache-2.0
package matrixproof

import (
	"context"
	"time"

	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/id"
)

// VerifyRootChangeConsent anchors every transition in the already retained
// authority. Reactivation keeps that root; rotation additionally requires the
// old root's SDK signature on the newly published master. A JWT, directory key
// replacement or caller-selected previous master cannot authorize this change.
func VerifyRootChangeConsent(ctx context.Context, origin Origin, previous Authority, proof SignedAction, expected RootChallenge, now time.Time) error {
	request, action := expected.Request, expected.Action
	if !rootChangeMatchesOrigin(origin, previous, request, action) {
		return ErrUnverified
	}
	root, err := EnrollmentRoot(origin)
	if err != nil {
		return err
	}
	if request.RootKey != root.String() || action.RootKey != root.String() {
		return ErrUnverified
	}
	switch action.Kind {
	case "root.resume":
		if previous.Status != "revoked" || request.RootKey != previous.RootKey {
			return ErrUnverified
		}
	case "root.rotate":
		if previous.Status != "active" || request.RootKey == previous.RootKey {
			return ErrUnverified
		}
		master := origin.Keys.MasterKeys[origin.UserID]
		if ok, err := signatures.VerifySignatureJSON(master, origin.UserID, previous.RootKey, id.Ed25519(previous.RootKey)); err != nil || !ok {
			return ErrUnverified
		}
	default:
		return ErrUnverified
	}
	digest, err := RequestDigest(request)
	if err != nil || digest != action.RequestSHA256 {
		return ErrUnverified
	}
	device, err := Verify(ctx, origin, root)
	if err != nil {
		return err
	}
	return device.VerifyAction(ctx, proof, action, now)
}

func rootChangeMatchesOrigin(origin Origin, previous Authority, request RootEnrollment, action Action) bool {
	return previous.Validate(origin.Scope) == nil && previous.UserID == origin.UserID.String() && request.Version == 1 && request.Scope == previous.Scope && request.UserID == previous.UserID && request.PreviousGeneration == previous.Generation && request.PreviousRootKey == previous.RootKey && request.Generation == previous.Generation+1 && action.Generation == previous.Generation && action.Scope == previous.Scope && request.Target == action.Target && request.Target.DeviceID == origin.DeviceID.String()
}
