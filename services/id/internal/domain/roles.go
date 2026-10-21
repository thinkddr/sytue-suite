// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "time"

// Permission es una capacidad de administración GRANULAR dentro de una organización.
// El catálogo es FIJO (estas constantes): un rol personalizado concede un subconjunto
// de ellas y el enforcement es deny-by-default (sin el permiso, no se autoriza).
//
// owner/admin son roles BUILT-IN y superusuarios: conservan acceso TOTAL y NO dependen
// de estos permisos (ver HasPermission) — retrocompatibilidad y anti-lockout.
type Permission string

const (
	PermUsersManage          Permission = "users.manage"           // alta/baja, rol, reset de contraseña/MFA, sesiones
	PermUsersRead            Permission = "users.read"             // ver el directorio y las sesiones (solo lectura)
	PermSSOManage            Permission = "sso.manage"             // proveedores SSO / federación
	PermAppsManage           Permission = "apps.manage"            // apps de terceros (SAML SP, tokens DCR)
	PermAccessPoliciesManage Permission = "access_policies.manage" // acceso condicional
	PermGroupsManage         Permission = "groups.manage"          // grupos y su membresía
	PermAuditView            Permission = "audit.view"             // log de auditoría
	PermBillingManage        Permission = "billing.manage"         // facturación (aún sin endpoints en id; lo consumen otros servicios)
	PermRolesManage          Permission = "roles.manage"           // definir/asignar roles — RESERVADO a owner/admin (no delegable)
)

// AllPermissions es el catálogo FIJO completo (orden estable para UI/serialización).
var AllPermissions = []Permission{
	PermUsersManage, PermUsersRead, PermSSOManage, PermAppsManage,
	PermAccessPoliciesManage, PermGroupsManage, PermAuditView,
	PermBillingManage, PermRolesManage,
}

// permGrantable indica los permisos que un rol PERSONALIZADO puede conceder. Es el
// catálogo MENOS roles.manage: la gestión de roles queda reservada a owner/admin, así
// un rol delegado nunca puede otorgarse la capacidad de crear/asignar roles y escalar
// más allá de sus permisos (anti auto-escalada; ver mig 0019).
var permGrantable = func() map[Permission]struct{} {
	m := make(map[Permission]struct{}, len(AllPermissions))
	for _, p := range AllPermissions {
		if p == PermRolesManage {
			continue
		}
		m[p] = struct{}{}
	}
	return m
}()

// CustomRole es un rol de administración delegada de una organización: concede un
// subconjunto de permisos granulares a los usuarios a los que se asigna.
type CustomRole struct {
	ID          string
	OrgID       string
	Name        string
	Permissions []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NormalizePermissions valida y normaliza el conjunto de permisos de un rol
// personalizado: recorta, deduplica y exige que CADA permiso sea concedible
// (permGrantable). Devuelve (perms, false) si alguno es desconocido o no delegable
// (p. ej. roles.manage) — el handler lo traduce a 400. Un conjunto vacío es válido
// (rol inerte). El orden de AllPermissions se preserva para una salida estable.
func NormalizePermissions(in []string) ([]string, bool) {
	want := make(map[Permission]struct{}, len(in))
	for _, s := range in {
		p := Permission(s)
		if _, ok := permGrantable[p]; !ok {
			return nil, false
		}
		want[p] = struct{}{}
	}
	out := make([]string, 0, len(want))
	for _, p := range AllPermissions {
		if _, ok := want[p]; ok {
			out = append(out, string(p))
		}
	}
	return out, true
}

// HasPermission es la DECISIÓN de autorización pura (sin BD): ¿el sujeto con estos
// roles built-in y estos permisos efectivos puede ejercer `want`?
//
//   - owner/admin ⇒ SIEMPRE sí (superusuarios built-in, como hoy): retrocompat y
//     anti-lockout — nunca se quedan fuera por el modelo de permisos.
//   - resto ⇒ sí solo si `want` está en sus permisos efectivos (deny-by-default).
func HasPermission(roles, perms []string, want Permission) bool {
	for _, r := range roles {
		if r == string(RoleOwner) || r == string(RoleAdmin) {
			return true
		}
	}
	for _, p := range perms {
		if p == string(want) {
			return true
		}
	}
	return false
}
