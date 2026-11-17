// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"time"

	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// SignIngressAction signs the server-retained public control transcript with
// the maintained SDK account. A token-only device cannot authorize ingress;
// the product identity, own master and this device must already be verified.
// The mandatory lease validates the persistent device-session witness before
// any SDK/network access and remains held through signing.
func (c *Client) SignIngressAction(ctx context.Context, lease offline.WorkLease, expected matrixproof.Intent, action matrixproof.Action) (matrixproof.SignedAction, error) {
	if !expected.Valid() || action.Intent() != expected || !action.Valid(time.Now()) {
		return matrixproof.SignedAction{}, matrixproof.ErrUnverified
	}
	if lease == nil {
		return matrixproof.SignedAction{}, offline.ErrWorkLease
	}
	release, err := lease(ctx)
	if err != nil {
		return matrixproof.SignedAction{}, err
	}
	if release == nil {
		return matrixproof.SignedAction{}, offline.ErrWorkLease
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return matrixproof.SignedAction{}, err
	}
	if _, err := c.PrepareStorage(ctx, action.Scope); err != nil {
		return matrixproof.SignedAction{}, err
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.cfg.Bootstrap.StorageIdentityVersion != storage.IsolatedIdentity || c.storageScope() != action.Scope {
		return matrixproof.SignedAction{}, ErrIdentity
	}
	if err := c.checkStoragePublicationTarget(action.Target); err != nil {
		return matrixproof.SignedAction{}, err
	}
	status, err := c.verificationStatus(ctx)
	if err != nil {
		return matrixproof.SignedAction{}, err
	}
	if !status.Verified {
		return matrixproof.SignedAction{}, ErrUnverified
	}
	machine := c.crypto.Machine()
	keys, err := machine.GetOwnCrossSigningPublicKeys(ctx)
	if err != nil {
		return matrixproof.SignedAction{}, err
	}
	if keys == nil || keys.MasterKey.String() != action.RootKey {
		return matrixproof.SignedAction{}, ErrIdentity
	}
	signature, err := machine.GetAccount().SignJSON(action)
	if err != nil {
		return matrixproof.SignedAction{}, err
	}
	if err := ctx.Err(); err != nil {
		return matrixproof.SignedAction{}, err
	}
	return matrixproof.SignedAction{Action: action, Signatures: signatures.NewSingleSignature(c.cli.UserID, id.KeyAlgorithmEd25519, c.session.DeviceID.String(), signature)}, nil
}
