// SPDX-License-Identifier: Apache-2.0
package sessionproof

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// VerifyHistoricalCertificate verifies a public SDK signature under an exact
// independently retained pin. It cannot establish current activity, a new
// recipient, or publication authority. A caller must already have a verified
// historical cutover/immutable sending hold for this exact resource/session.
func VerifyHistoricalCertificate(ctx context.Context, raw []byte, expected CertificateExpectation, device matrixproof.HistoricalDevice) (VerifiedCertificate, error) {
	return verifyCertificateSignature(ctx, raw, expected, device.Identity(), device.VerifyJSON)
}
