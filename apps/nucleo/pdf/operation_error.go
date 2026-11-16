// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdf

import "errors"

// OperationFailure preserves actionable edit failures across device and HTTP views.
type OperationFailure struct {
	Status        int
	Code, Message string
}

func (e *OperationFailure) Error() string { return e.Message }

// DescribeOperationError retains the explicit confirmation required to invalidate signatures.
func DescribeOperationError(err error) OperationFailure {
	var known *OperationFailure
	if errors.As(err, &known) {
		return *known
	}
	failure := OperationFailure{Status: 400, Code: "bad_request", Message: err.Error()}
	switch {
	case errors.Is(err, ErrCertificadoNoUsable):
		failure.Code = "pdf_bad_certificate"
		failure.Message = "The certificate or the private key cannot be used to sign."
	case errors.Is(err, ErrCertificarFirmado):
		failure.Code = "pdf_certify_signed"
		failure.Message = "Only the first signature can certify the document."
	case errors.Is(err, ErrFirmaNoCabe):
		failure.Code = "pdf_signature_too_large"
		failure.Message = "The certificate chain is too long for the reserved signature space."
	case errors.Is(err, ErrInvalidaFirmas):
		failure.Status = 409
		failure.Code = "pdf_invalidates_signatures"
		failure.Message = "Saving this change would invalidate the signatures of the document: " + err.Error()
	case errors.Is(err, ErrLetrasDesconocidas):
		failure.Code = "pdf_unknown_letters"
		failure.Message = "The font of this text does not say which letters it uses, so it cannot be rewritten."
	case errors.Is(err, ErrPDFClaveIncorrecta):
		failure.Code = "pdf_password_required"
		failure.Message = "The password is not correct."
	}
	return failure
}
