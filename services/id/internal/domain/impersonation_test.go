// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"testing"
)

// delegable es un client de confianza YA marcado por el operador: acotado a una
// org, confidencial, con el grant declarado. Es la línea base de los tests: cada
// caso rompe UNA condición y comprueba que el canje se cierra.
func delegable() Client {
	return Client{
		ID:             "acme-migrator",
		Type:           ClientConfidential,
		GrantTypes:     []string{"client_credentials", GrantTokenExchange},
		Scopes:         []string{"openid", "profile", "email", "offline_access", "admin", "drive.write"},
		OrgID:          "org-acme",
		MayImpersonate: true,
	}
}

func TestAuthorizeImpersonationClient(t *testing.T) {
	noGrant := delegable()
	noGrant.GrantTypes = []string{"client_credentials"}

	notMarked := delegable()
	notMarked.MayImpersonate = false

	public := delegable()
	public.TokenEndpointAuthMethod = "none"

	publicType := delegable()
	publicType.Type = ClientPublic

	global := delegable()
	global.OrgID = ""

	cases := []struct {
		name  string
		c     Client
		allow bool
	}{
		{"client delegable completo", delegable(), true},
		{"sin may_impersonate (defecto)", notMarked, false},
		{"sin el grant en grant_types", noGrant, false},
		{"client público (auth_method=none)", public, false},
		{"client público (type)", publicType, false},
		{"client global, sin organización", global, false},
		{"client vacío", Client{}, false},
	}
	for _, tc := range cases {
		err := AuthorizeImpersonationClient(tc.c)
		if tc.allow && err != nil {
			t.Errorf("%s: se esperaba permitir, err=%v", tc.name, err)
		}
		if !tc.allow && !errors.Is(err, ErrImpersonationForbidden) {
			t.Errorf("%s: se esperaba ErrImpersonationForbidden, err=%v", tc.name, err)
		}
	}
}

func TestAuthorizeImpersonationTarget(t *testing.T) {
	active := User{ID: "u-1", Status: StatusActive}
	suspended := User{ID: "u-2", Status: StatusSuspended}

	admins := delegable()
	admins.MayImpersonateAdmins = true

	cases := []struct {
		name  string
		c     Client
		u     User
		role  Role
		allow bool
	}{
		{"miembro activo de la org", delegable(), active, RoleMember, true},
		{"billing activo de la org", delegable(), active, RoleBilling, true},
		// Rol vacío = el store no encontró membresía: usuario de OTRA org, o
		// inexistente. Los dos casos caen aquí, y con el mismo error.
		{"usuario de otra org o inexistente", delegable(), active, "", false},
		{"usuario no activo", delegable(), suspended, RoleMember, false},
		{"owner sin declaración explícita", delegable(), active, RoleOwner, false},
		{"admin sin declaración explícita", delegable(), active, RoleAdmin, false},
		{"owner con may_impersonate_admins", admins, active, RoleOwner, true},
		{"admin con may_impersonate_admins", admins, active, RoleAdmin, true},
		{"rol desconocido", delegable(), active, Role("superuser"), false},
	}
	for _, tc := range cases {
		err := AuthorizeImpersonationTarget(tc.c, tc.u, tc.role)
		if tc.allow && err != nil {
			t.Errorf("%s: se esperaba permitir, err=%v", tc.name, err)
		}
		if !tc.allow && !errors.Is(err, ErrImpersonationTarget) {
			t.Errorf("%s: se esperaba ErrImpersonationTarget, err=%v", tc.name, err)
		}
	}
}

func TestImpersonationScope(t *testing.T) {
	gatewayScopes := delegable()
	gatewayScopes.Scopes = []string{"openid", "id.directory.read", "id.apps.manage"}

	cases := []struct {
		name      string
		requested string
		c         Client
		role      Role
		want      string
	}{
		{
			name: "intersección con lo concedido al client",
			// drive.read no está en los scopes del client: se cae.
			requested: "openid profile drive.write drive.read",
			c:         delegable(), role: RoleMember,
			want: "openid profile drive.write",
		},
		{
			name:      "escalada de scopes recortada, no rechazada",
			requested: "openid admin offline_access",
			c:         delegable(), role: RoleMember,
			want: "openid",
		},
		{
			name:      "admin se concede si el usuario ES admin",
			requested: "openid admin",
			c:         delegable(), role: RoleAdmin,
			want: "openid admin",
		},
		{
			name:      "admin se concede si el usuario es owner",
			requested: "openid admin",
			c:         delegable(), role: RoleOwner,
			want: "openid admin",
		},
		{
			name: "offline_access NUNCA, ni siendo owner",
			// Un refresh token convertiría la delegación en indefinida y sin traza.
			requested: "openid offline_access",
			c:         delegable(), role: RoleOwner,
			want: "openid",
		},
		{
			name:      "scopes de máquina de la pasarela nunca pasan al usuario",
			requested: "openid id.directory.read id.apps.manage",
			c:         gatewayScopes, role: RoleAdmin,
			want: "openid",
		},
		{
			name:      "sin scope: hereda el del client, ya filtrado",
			requested: "   ",
			c:         delegable(), role: RoleMember,
			want: "openid profile email drive.write",
		},
		{
			name:      "duplicados y espacios sobrantes",
			requested: "openid  openid   profile ",
			c:         delegable(), role: RoleMember,
			want: "openid profile",
		},
		{
			name:      "nada delegable: cadena vacía (el handler lo traduce a invalid_scope)",
			requested: "offline_access admin",
			c:         delegable(), role: RoleMember,
			want: "",
		},
	}
	for _, tc := range cases {
		if got := ImpersonationScope(tc.requested, tc.c, tc.role); got != tc.want {
			t.Errorf("%s: ImpersonationScope(%q)=%q, quiero %q", tc.name, tc.requested, got, tc.want)
		}
	}
}

// TestImpersonationTokenTTLIsShort fija el invariante: un token que actúa como
// otra persona vive minutos. Si alguien lo sube, que sea rompiendo este test.
func TestImpersonationTokenTTLIsShort(t *testing.T) {
	if ImpersonationTokenTTL <= 0 || ImpersonationTokenTTL > 15*60*1e9 {
		t.Fatalf("ImpersonationTokenTTL=%v; la impersonación debe durar minutos", ImpersonationTokenTTL)
	}
}
