// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// adminMayManage: matriz de las dos invariantes (no sobre uno mismo; a un owner
// solo lo toca otro owner). Cualquier bloqueo debe ser 403.
func TestAdminMayManage(t *testing.T) {
	const caller = "11111111-1111-1111-1111-111111111111"
	const other = "22222222-2222-2222-2222-222222222222"

	cases := []struct {
		name        string
		callerID    string
		targetID    string
		callerRoles []string
		targetRole  domain.Role
		wantErr     bool
	}{
		{"sobre-uno-mismo", caller, caller, []string{"owner"}, domain.RoleMember, true},
		{"admin-no-toca-owner", caller, other, []string{"admin"}, domain.RoleOwner, true},
		{"owner-si-toca-owner", caller, other, []string{"owner"}, domain.RoleOwner, false},
		{"admin-toca-admin", caller, other, []string{"admin"}, domain.RoleAdmin, false},
		{"admin-toca-member", caller, other, []string{"admin"}, domain.RoleMember, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := adminMayManage(c.callerID, c.targetID, c.callerRoles, c.targetRole)
			if c.wantErr {
				if e == nil {
					t.Fatal("quiero bloqueo, obtuve nil")
				}
				if e.Status != http.StatusForbidden {
					t.Fatalf("status = %d, quiero 403", e.Status)
				}
				return
			}
			if e != nil {
				t.Fatalf("quiero permitir, obtuve %v", e)
			}
		})
	}
}
