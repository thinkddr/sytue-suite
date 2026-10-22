// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// runPerm ejecuta el middleware requirePermission(perms) con unos claims dados y
// devuelve el status resultante (200 si dejó pasar al next). Sin BD: prueba el
// enforcement puro sobre el contexto, como los tests admin existentes.
func runPerm(claims *tokenClaims, perms ...domain.Permission) int {
	s := &Server{}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := s.requirePermission(perms...)(next)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/x", http.NoBody)
	if claims != nil {
		req = req.WithContext(context.WithValue(req.Context(), ctxKeyClaims{}, *claims))
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr.Code
}

// requirePermission: owner/admin pasan todo (retrocompat); un rol delegado con
// users.manage entra en SU endpoint y recibe 403 en otro (sso); sin claims ⇒ 401.
func TestRequirePermissionEnforcement(t *testing.T) {
	helpdesk := &tokenClaims{Roles: []string{"member"}, Perms: []string{"users.manage"}}
	owner := &tokenClaims{Roles: []string{"owner"}}
	bare := &tokenClaims{Roles: []string{"member"}}

	cases := []struct {
		name   string
		claims *tokenClaims
		perms  []domain.Permission
		want   int
	}{
		{"helpdesk-en-su-endpoint", helpdesk, []domain.Permission{domain.PermUsersManage}, http.StatusOK},
		{"helpdesk-403-en-sso", helpdesk, []domain.Permission{domain.PermSSOManage}, http.StatusForbidden},
		{"helpdesk-403-en-billing", helpdesk, []domain.Permission{domain.PermBillingManage}, http.StatusForbidden},
		{"helpdesk-lectura-any-of", helpdesk, []domain.Permission{domain.PermUsersRead, domain.PermUsersManage}, http.StatusOK},
		{"owner-pasa-sso", owner, []domain.Permission{domain.PermSSOManage}, http.StatusOK},
		{"owner-pasa-billing", owner, []domain.Permission{domain.PermBillingManage}, http.StatusOK},
		{"member-pelado-403", bare, []domain.Permission{domain.PermUsersManage}, http.StatusForbidden},
		{"sin-claims-401", nil, []domain.Permission{domain.PermUsersManage}, http.StatusUnauthorized},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := runPerm(c.claims, c.perms...); got != c.want {
				t.Fatalf("status = %d, quiero %d", got, c.want)
			}
		})
	}
}

// wouldLeaveOrgWithoutOwner: el guard anti-lockout «≥1 owner». Degradar/quitar al
// último owner debe detectarse; con otro owner presente, no.
func TestWouldLeaveOrgWithoutOwner(t *testing.T) {
	cases := []struct {
		name       string
		current    domain.Role
		next       domain.Role // "" = quitar
		ownerCount int
		want       bool
	}{
		{"ultimo-owner-a-member", domain.RoleOwner, domain.RoleMember, 1, true},
		{"ultimo-owner-a-admin", domain.RoleOwner, domain.RoleAdmin, 1, true},
		{"quitar-ultimo-owner", domain.RoleOwner, "", 1, true},
		{"owner-con-otro-owner", domain.RoleOwner, domain.RoleMember, 2, false},
		{"owner-sigue-owner", domain.RoleOwner, domain.RoleOwner, 1, false},
		{"degradar-admin-no-aplica", domain.RoleAdmin, domain.RoleMember, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := wouldLeaveOrgWithoutOwner(c.current, c.next, c.ownerCount); got != c.want {
				t.Fatalf("wouldLeaveOrgWithoutOwner(%q,%q,%d) = %v, quiero %v",
					c.current, c.next, c.ownerCount, got, c.want)
			}
		})
	}
}
