// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdf

import (
	"errors"
	"fmt"
)

// ReadFailure preserves actionable PDF error codes across device and HTTP views.
type ReadFailure struct{ Code, Message string }

// DescribeReadError distinguishes passwords, unsupported protection and corrupt inputs.
func DescribeReadError(err error, name string) ReadFailure {
	switch {
	case errors.Is(err, ErrPDFClaveIncorrecta):
		return ReadFailure{Code: "pdf_password_required", Message: fmt.Sprintf("“%s” needs a password to be opened.", name)}
	case errors.Is(err, ErrPDFCifrado):
		return ReadFailure{Code: "pdf_encryption_unsupported", Message: fmt.Sprintf("“%s” uses a protection scheme we cannot open (a third-party security handler).", name)}
	case errors.Is(err, ErrPDFNoEsPDF):
		return ReadFailure{Code: "pdf_not_pdf", Message: fmt.Sprintf("“%s” is not a PDF.", name)}
	case errors.Is(err, ErrPDFNoSoportado):
		return ReadFailure{Code: "pdf_unsupported", Message: fmt.Sprintf("“%s” uses a PDF feature we do not support yet.", name)}
	default:
		return ReadFailure{Code: "pdf_unreadable", Message: fmt.Sprintf("“%s” could not be read.", name)}
	}
}
