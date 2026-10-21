// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"fmt"

	"git.sytue.eu/sytue/one/platform/pkg/clave"
)

// HashPassword devuelve un hash Argon2id en formato PHC para proteger un enlace
// público. Nunca se almacena la contraseña en claro (CONVENTIONS §0.7). El esquema es
// el de `platform/pkg/clave`, el mismo que usa Docs para los suyos.
func HashPassword(password string) (string, error) {
	h, err := clave.Hash(password)
	if errors.Is(err, clave.ErrVacia) {
		return "", fmt.Errorf("%w: contraseña vacía", ErrInvalidName)
	}
	return h, err
}

// VerifyPassword compara una contraseña con un hash PHC en tiempo constante.
func VerifyPassword(password, encoded string) (bool, error) {
	return clave.Verificar(password, encoded)
}
