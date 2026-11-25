// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import "errors"

// ImportError dice qué comprobación paró la importación (o la apertura) del Drive
// privado: un código corto y fijo, el mismo que usa la web
// (`DriveImportError.check`), para que la puerta lo enseñe y en producción se sepa
// qué pasó sin datos de la persona. Envuelve el error de siempre: errors.Is con
// ErrSource, ErrConflict o ErrNotReady sigue valiendo.
type ImportError struct {
	Check string
	Err   error
}

func (e *ImportError) Error() string { return e.Err.Error() }
func (e *ImportError) Unwrap() error { return e.Err }

func refuse(check string, err error) error { return &ImportError{Check: check, Err: err} }

// ImportCheck es el código de la comprobación que paró la importación, o "".
func ImportCheck(err error) string {
	var failure *ImportError
	if errors.As(err, &failure) {
		return failure.Check
	}
	return ""
}
