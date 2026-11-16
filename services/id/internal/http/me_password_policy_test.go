// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Lo que /me promete en «Mi cuenta» lleva el suelo duro puesto: una política
// guardada por debajo de 8 (datos antiguos o una migración) no puede anunciar
// menos de lo que domain.ValidatePassword va a exigir.
func TestMePasswordPolicyJSONAplicaElSuelo(t *testing.T) {
	got := mePasswordPolicyJSON(domain.PasswordPolicy{MinLength: 4, RequireComplexity: true})
	if got["min_length"] != domain.PasswordMinFloor || got["require_complexity"] != true {
		t.Fatalf("mePasswordPolicyJSON = %v; quiero min_length=%d y complejidad", got, domain.PasswordMinFloor)
	}
	got = mePasswordPolicyJSON(domain.PasswordPolicy{MinLength: 16})
	if got["min_length"] != 16 || got["require_complexity"] != false {
		t.Fatalf("mePasswordPolicyJSON = %v; quiero 16 sin complejidad", got)
	}
}
