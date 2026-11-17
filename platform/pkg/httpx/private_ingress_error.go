// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// PrivateIngressError maps the shared queue contract. Unknown domain failures
// remain the caller's responsibility, preserving the app's existing error API.
func PrivateIngressError(err error) *Error {
	switch {
	case errors.Is(err, ingress.ErrEnrollmentEpoch):
		return ErrConflict.WithCode("private_ingress_epoch_changed")
	case errors.Is(err, ingress.ErrRevoked), errors.Is(err, matrixproof.ErrUnverified):
		return ErrForbidden.WithCode("private_ingress_authorization_changed")
	case errors.Is(err, ingress.ErrClaim):
		return ErrConflict.WithCode("private_ingress_claim_changed")
	case errors.Is(err, ingress.ErrReceipt):
		return ErrConflict.WithCode("private_ingress_receipt_required")
	case errors.Is(err, ingress.ErrContract):
		return ErrBadRequest
	default:
		return nil
	}
}
