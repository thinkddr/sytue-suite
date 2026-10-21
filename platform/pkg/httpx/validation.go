// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"errors"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/validation"
)

// FailValidation traduce un error a la respuesta canónica. Si es un error de
// validación (validation.Errors) responde 422 con los detalles por campo; en
// otro caso delega en Fail (que oculta la causa a 5xx).
func FailValidation(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}
	var verrs *validation.Errors
	if errors.As(err, &verrs) {
		Fail(w, r, ErrUnprocessable.WithDetails(verrs).Wrap(err))
		return
	}
	Fail(w, r, err)
}

// DecodeAndValidate decodifica el cuerpo JSON en dst y, si dst implementa
// Validatable, lo valida. Devuelve el error listo para FailValidation.
func DecodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) error {
	if err := DecodeJSON(w, r, dst); err != nil {
		return err
	}
	if v, ok := dst.(Validatable); ok {
		if err := v.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validatable lo implementan los DTO que saben autovalidarse.
type Validatable interface {
	Validate() error
}
