// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// isAdminRole delimita el conjunto de administración (owner|admin) que el guard
// del "último admin" protege: solo esos roles cuentan, member/billing no.
func TestIsAdminRole(t *testing.T) {
	cases := map[domain.Role]bool{
		domain.RoleOwner:   true,
		domain.RoleAdmin:   true,
		domain.RoleMember:  false,
		domain.RoleBilling: false,
	}
	for role, want := range cases {
		if got := isAdminRole(role); got != want {
			t.Errorf("isAdminRole(%q) = %v, quiero %v", role, got, want)
		}
	}
}

// roleRank ordena por privilegio (owner>admin>member=billing) para que una
// degradación (bajada de rango) se detecte y revoque sesiones. Se comprueba el
// orden estricto y que member/billing empaten (ninguno es degradación del otro).
func TestRoleRankOrdersPrivilege(t *testing.T) {
	if roleRank(domain.RoleOwner) <= roleRank(domain.RoleAdmin) ||
		roleRank(domain.RoleAdmin) <= roleRank(domain.RoleMember) {
		t.Fatalf("orden de privilegio incorrecto: owner=%d admin=%d member=%d",
			roleRank(domain.RoleOwner), roleRank(domain.RoleAdmin), roleRank(domain.RoleMember))
	}
	if roleRank(domain.RoleMember) != roleRank(domain.RoleBilling) {
		t.Fatal("member y billing deben empatar en privilegio (ninguno degrada al otro)")
	}
	// Un cambio owner→member es degradación; member→admin es promoción.
	if roleRank(domain.RoleMember) >= roleRank(domain.RoleOwner) {
		t.Fatal("member debe rankear por debajo de owner (degradación)")
	}
}
