// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// StorageRootConsentRequest reads public trust data from this verified SDK
// session, never from a server challenge. The caller must obtain explicit user
// consent before first enrollment, resume or rotation, and retain this exact
// request until MatrixStorageRootChallenge and SignIngressAction finish.
func (c *Client) StorageRootConsentRequest(ctx context.Context, lease offline.WorkLease, scope storage.Scope, target storage.PublicationTarget, kind string, previous *matrixproof.Authority) (matrixproof.RootEnrollment, error) {
	if lease == nil {
		return matrixproof.RootEnrollment{}, offline.ErrWorkLease
	}
	release, err := lease(ctx)
	if err != nil {
		return matrixproof.RootEnrollment{}, err
	}
	if release == nil {
		return matrixproof.RootEnrollment{}, offline.ErrWorkLease
	}
	defer release()
	if _, err := c.PrepareStorage(ctx, scope); err != nil {
		return matrixproof.RootEnrollment{}, err
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.cfg.Bootstrap.StorageIdentityVersion != storage.IsolatedIdentity || c.storageScope() != scope {
		return matrixproof.RootEnrollment{}, ErrIdentity
	}
	if err := c.checkStoragePublicationTarget(target); err != nil {
		return matrixproof.RootEnrollment{}, err
	}
	status, err := c.verificationStatus(ctx)
	if err != nil {
		return matrixproof.RootEnrollment{}, err
	}
	if !status.Verified {
		return matrixproof.RootEnrollment{}, ErrUnverified
	}
	keys, err := c.crypto.Machine().GetOwnCrossSigningPublicKeys(ctx)
	if err != nil {
		return matrixproof.RootEnrollment{}, err
	}
	if keys == nil {
		return matrixproof.RootEnrollment{}, ErrRecoveryRequired
	}
	request := matrixproof.RootEnrollment{Version: 1, Scope: scope, UserID: c.cli.UserID.String(), Target: target, RootKey: keys.MasterKey.String(), Generation: 1}
	if err := ctx.Err(); err != nil {
		return matrixproof.RootEnrollment{}, err
	}
	return completeStorageRootConsentRequest(request, kind, previous)
}

func completeStorageRootConsentRequest(request matrixproof.RootEnrollment, kind string, previous *matrixproof.Authority) (matrixproof.RootEnrollment, error) {
	if kind == "root.enroll" {
		if previous != nil {
			return matrixproof.RootEnrollment{}, matrixproof.ErrUnverified
		}
	} else {
		if previous == nil || previous.Validate(request.Scope) != nil || previous.UserID != request.UserID {
			return matrixproof.RootEnrollment{}, matrixproof.ErrUnverified
		}
		switch kind {
		case "root.resume":
			if previous.Status != "revoked" || previous.RootKey != request.RootKey {
				return matrixproof.RootEnrollment{}, matrixproof.ErrUnverified
			}
		case "root.rotate":
			if previous.Status != "active" || previous.RootKey == request.RootKey {
				return matrixproof.RootEnrollment{}, matrixproof.ErrUnverified
			}
		default:
			return matrixproof.RootEnrollment{}, matrixproof.ErrUnverified
		}
		request.PreviousGeneration = previous.Generation
		request.PreviousRootKey = previous.RootKey
		request.Generation = previous.Generation + 1
	}
	return request, nil
}
