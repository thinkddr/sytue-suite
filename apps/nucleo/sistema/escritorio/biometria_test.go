// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"errors"
	"testing"
)

// TestSoloElCeroDeBiometricPromptAbre: el bloqueo de la sesión deja pasar con
// ErrNoDisponible, así que solo un fallo ANTES del diálogo puede darlo; un código que no
// es 0 (cancelar, -1 si Java falló con el diálogo pedido) no abre nada.
func TestSoloElCeroDeBiometricPromptAbre(t *testing.T) {
	for _, c := range []struct {
		antes  error
		codigo int
		quiero error
	}{
		{nil, 0, nil},
		{nil, -1, ErrNoVerificado},
		{nil, 10, ErrNoVerificado}, // BIOMETRIC_ERROR_USER_CANCELED
		{errors.New("sin la clase"), 0, ErrNoDisponible},
	} {
		if got := respuestaBiometricPrompt(c.antes, c.codigo); !errors.Is(got, c.quiero) || (got == nil) != (c.quiero == nil) {
			t.Errorf("respuestaBiometricPrompt(%v, %d) = %v, quiero %v", c.antes, c.codigo, got, c.quiero)
		}
	}
}
