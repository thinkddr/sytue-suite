// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// SignResourceBootstrapCapture binds the owner's plan AND complete member count
// and ordered stream digest with the current maintained SDK device. Signing is
// an intent, not source/roster authority: the backend must verify ORIGINAL freeze
// and every current recipient before finalizing the existing VerifiedPlan.
func (c *Client) SignResourceBootstrapCapture(ctx context.Context, current resource.VerifiedSession, plan resource.BootstrapPlan, manifest resource.BootstrapRosterManifest, lease offline.WorkLease) (resource.SignedBootstrapPlan, resource.SignedBootstrapRosterManifest, error) {
	var zeroPlan resource.SignedBootstrapPlan
	var zeroManifest resource.SignedBootstrapRosterManifest
	binding, err := matrixproof.RequestDigest(current.Binding())
	if err != nil || !manifest.ValidFor(plan) || plan.CreatorScope != current.Scope() || plan.Creator != current.Actor() || plan.SessionBindingSHA256 != binding {
		return zeroPlan, zeroManifest, ErrIdentity
	}
	signed := resource.SignedBootstrapPlan{BootstrapPlan: plan}
	signed.Signatures, err = c.signCurrentResourceMetadata(ctx, current, plan, lease)
	if err != nil {
		return zeroPlan, zeroManifest, err
	}
	signedManifest := resource.SignedBootstrapRosterManifest{BootstrapRosterManifest: manifest}
	signedManifest.Signatures, err = c.signCurrentResourceMetadata(ctx, current, manifest, lease)
	if err != nil {
		return zeroPlan, zeroManifest, err
	}
	return signed, signedManifest, nil
}
