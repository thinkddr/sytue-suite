// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import "testing"

// La administración de plataforma exige SIEMPRE segundo factor, sea cual sea el
// rol de organización de quien llama. Un token nacido de solo contraseña no
// abre esta API aunque su dueño esté en platform_admins.
func TestHasMFAReconoceLosFactoresRealesYSoloEsos(t *testing.T) {
	conFactor := [][]string{
		{"mfa"},
		{"pwd", "mfa"},
		{"otp"},
		{"webauthn"},
		{"pwd", "webauthn"},
	}
	for _, amr := range conFactor {
		if !hasMFA(amr) {
			t.Errorf("amr=%v debería contar como segundo factor", amr)
		}
	}
	sinFactor := [][]string{
		nil,
		{},
		{"pwd"},
		// `sso` es autenticación federada, no un segundo factor: el IdP upstream
		// puede haber pedido solo contraseña y aquí no hay forma de saberlo.
		{"sso"},
		{"pwd", "sso"},
		// Ni prefijos ni variantes: la comparación es exacta a propósito.
		{"mfa_pending"},
		{"MFA"},
	}
	for _, amr := range sinFactor {
		if hasMFA(amr) {
			t.Errorf("amr=%v NO debería contar como segundo factor", amr)
		}
	}
}
