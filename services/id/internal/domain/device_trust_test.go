// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// TestEvaluarConfianzaDelDispositivo: `standard` pide las DOS cosas —dispositivo de
// confianza y passkey—; con una sola no hay garantía que dar, y `managed` no lo da
// nunca porque no hay inscripción de dispositivos gestionados.
func TestEvaluarConfianzaDelDispositivo(t *testing.T) {
	casos := []struct {
		amr         []string
		deConfianza bool
		quiere      string
	}{
		{[]string{"webauthn"}, true, DeviceTrustStandard},
		{[]string{"pwd", "webauthn"}, true, DeviceTrustStandard},
		{[]string{"webauthn"}, false, DeviceTrustNone},
		{[]string{"pwd", "mfa"}, true, DeviceTrustNone},
		{[]string{"pwd"}, true, DeviceTrustNone},
		{[]string{"federated"}, true, DeviceTrustNone},
		{nil, true, DeviceTrustNone},
	}
	for _, c := range casos {
		if got := EvaluarConfianzaDelDispositivo(c.amr, c.deConfianza); got != c.quiere {
			t.Errorf("amr=%v confianza=%v: %q, quiero %q", c.amr, c.deConfianza, got, c.quiere)
		}
		if got := EvaluarConfianzaDelDispositivo(c.amr, c.deConfianza); got == DeviceTrustManaged {
			t.Error("managed está reservado y no se emite")
		}
	}
}
