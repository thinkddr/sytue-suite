// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// HasPermission es la decisión de autorización de la admin delegada. Matriz:
//   - owner/admin (built-in) autorizan CUALQUIER permiso, aunque no tengan `perms`.
//   - un rol con users.manage puede lo suyo y NO billing.manage.
//   - sin rol built-in ni el permiso ⇒ deny-by-default.
func TestHasPermission(t *testing.T) {
	cases := []struct {
		name  string
		roles []string
		perms []string
		want  Permission
		allow bool
	}{
		{"owner-todo-users", []string{"owner"}, nil, PermUsersManage, true},
		{"owner-todo-billing", []string{"owner"}, nil, PermBillingManage, true},
		{"owner-todo-roles", []string{"owner"}, nil, PermRolesManage, true},
		{"admin-todo-billing", []string{"admin"}, nil, PermBillingManage, true},
		{"helpdesk-puede-users", []string{"member"}, []string{"users.manage"}, PermUsersManage, true},
		{"helpdesk-no-billing", []string{"member"}, []string{"users.manage"}, PermBillingManage, false},
		{"helpdesk-no-sso", []string{"member"}, []string{"users.manage"}, PermSSOManage, false},
		{"sin-rol-ni-perm-deny", []string{"member"}, nil, PermUsersManage, false},
		{"sin-nada-deny", nil, nil, PermUsersManage, false},
		{"perm-no-concede-roles", []string{"member"}, []string{"users.manage", "billing.manage"}, PermRolesManage, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasPermission(c.roles, c.perms, c.want); got != c.allow {
				t.Fatalf("HasPermission(%v,%v,%q) = %v, quiero %v", c.roles, c.perms, c.want, got, c.allow)
			}
		})
	}
}

// NormalizePermissions valida contra el catálogo y RECHAZA lo no delegable
// (roles.manage) y lo desconocido; deduplica y ordena por AllPermissions.
func TestNormalizePermissions(t *testing.T) {
	// Subconjunto válido con duplicado y desorden → normalizado, ordenado, sin dupes.
	got, ok := NormalizePermissions([]string{"billing.manage", "users.manage", "users.manage"})
	if !ok {
		t.Fatal("subconjunto válido rechazado")
	}
	want := []string{"users.manage", "billing.manage"} // orden de AllPermissions
	if len(got) != len(want) {
		t.Fatalf("normalizado = %v, quiero %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("orden/dedupe: %v, quiero %v", got, want)
		}
	}

	// roles.manage NO es delegable → rechazado (anti auto-escalada).
	if _, ok := NormalizePermissions([]string{"users.manage", "roles.manage"}); ok {
		t.Fatal("roles.manage debería rechazarse (no delegable)")
	}
	// Permiso desconocido → rechazado.
	if _, ok := NormalizePermissions([]string{"users.destroy"}); ok {
		t.Fatal("permiso desconocido debería rechazarse")
	}
	// Vacío → válido (rol inerte).
	if p, ok := NormalizePermissions(nil); !ok || len(p) != 0 {
		t.Fatalf("vacío = (%v,%v), quiero ([],true)", p, ok)
	}
}
