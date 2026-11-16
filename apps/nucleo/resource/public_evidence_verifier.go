// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// publicEvidenceVerifier is only a signature/transcript reader. Epoch,
// capability and recipient metadata may outlive their signer's device. Current
// enrollment, producers and public action APIs retain concrete live Devices;
// this private interface cannot sign, enroll, acquire or publish as that device.
type publicEvidenceVerifier interface {
	Identity() matrixproof.Identity
	VerifyJSON(context.Context, storage.Scope, storage.PublicationTarget, any) error
	VerifyAccountJSON(context.Context, storage.Scope, any) error
	PublicEvidence() (matrixproof.DeviceEvidence, error)
}
