// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// CRUD admin de ROLES PERSONALIZADOS / admin delegada (paridad Entra roles / Workspace
// admin roles) de la organización activa desde Sytue Admin. Un rol concede un
// subconjunto de PERMISOS granulares (catálogo fijo en domain/roles.go) a los usuarios a
// los que se asigna; el enforcement vive en requirePermission (mig 0019).
//
// RESERVADO A owner/admin (anti auto-escalada): estas rutas van gateadas por
// requireRole("owner","admin"), NO por un permiso delegable — definir/asignar roles no
// se delega, así un rol personalizado nunca puede concederse la gestión de roles y
// escalar. Además, NormalizePermissions RECHAZA roles.manage en el conjunto de un rol.
//
// Aislamiento por tenant (docs/CONVENTIONS.md §0.7), mismo anclaje que admin_groups.go:
// la org se deriva SIEMPRE del claim (activeOrg → org.ID), NUNCA del path, y todos los
// métodos de store cruzan custom_roles.org_id — un role_id o user_id de otro tenant da
// 404, no una acción cross-tenant (anti-IDOR). Cada mutación se audita.
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

type createRoleRequest struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

type patchRoleRequest struct {
	Name        *string  `json:"name"`
	Permissions []string `json:"permissions"`
}

type assignRoleRequest struct {
	UserID string `json:"user_id"`
}

// roleJSON es la forma de un rol en las respuestas admin (contrato Console).
func roleJSON(r domain.CustomRole, memberCount int) map[string]any {
	return map[string]any{
		"id":           r.ID,
		"name":         r.Name,
		"permissions":  orEmpty(r.Permissions),
		"member_count": memberCount,
	}
}

// handleAdminListRoles — GET /api/v1/admin/roles — roles de la org activa con
// member_count, orden estable por nombre.
func (s *Server) handleAdminListRoles(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	list, err := s.store.ListCustomRoles(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	roles := make([]map[string]any, 0, len(list))
	for _, rc := range list {
		roles = append(roles, roleJSON(rc.CustomRole, rc.MemberCount))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"roles": roles})
}

// handleAdminCreateRole — POST /api/v1/admin/roles {name, permissions[]} — crea un rol en
// la org activa. name obligatorio y único por-tenant (409). Cada permiso debe ser del
// catálogo y delegable (roles.manage se rechaza) ⇒ 400.
func (s *Server) handleAdminCreateRole(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req createRoleRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name := domain.SanitizeDisplayName(req.Name)
	if name == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("A role name is required."))
		return
	}
	perms, ok := domain.NormalizePermissions(req.Permissions)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("permissions contains an unknown or non-delegable permission."))
		return
	}
	role, err := s.store.CreateCustomRole(r.Context(), nil, org.ID, name, perms)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.role.created", role.ID, r))
	httpx.JSON(w, r, http.StatusCreated, roleJSON(role, 0))
}

// adminRoleInOrg resuelve la org activa y verifica que {id} sea un uuid y un rol de
// ELLA, devolviéndolo. Verja anti-IDOR compartida por las rutas de un rol concreto (un
// id de otro tenant ⇒ 404).
func (s *Server) adminRoleInOrg(r *http.Request) (domain.Organization, domain.CustomRole, error) {
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		return domain.Organization{}, domain.CustomRole{}, httpx.ErrBadRequest.WithMessage("Invalid role id.")
	}
	org, err := s.activeOrg(r)
	if err != nil {
		return domain.Organization{}, domain.CustomRole{}, err
	}
	role, err := s.store.GetCustomRole(r.Context(), org.ID, id)
	if err != nil {
		return org, domain.CustomRole{}, mapDomainErr(err)
	}
	return org, role, nil
}

// handleAdminGetRole — GET /api/v1/admin/roles/{id} — el rol y los usuarios asignados.
func (s *Server) handleAdminGetRole(w http.ResponseWriter, r *http.Request) {
	org, role, apiErr := s.adminRoleInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	members, err := s.store.ListCustomRoleMembers(r.Context(), org.ID, role.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	out := make([]map[string]any, 0, len(members))
	for _, m := range members {
		out = append(out, map[string]any{"user_id": m.UserID, "email": m.Email, "name": m.Name})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"role":    roleJSON(role, len(members)),
		"members": out,
	})
}

// handleAdminUpdateRole — PATCH /api/v1/admin/roles/{id} {name?, permissions?}. Campos
// omitidos conservan su valor; un array permissions presente (aunque vacío) REEMPLAZA el
// conjunto. name presente debe ser no vacío.
func (s *Server) handleAdminUpdateRole(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, role, apiErr := s.adminRoleInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	var req patchRoleRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name, perms := role.Name, role.Permissions
	if req.Name != nil {
		name = domain.SanitizeDisplayName(*req.Name)
		if name == "" {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The role name cannot be left empty."))
			return
		}
	}
	if req.Permissions != nil {
		np, ok := domain.NormalizePermissions(req.Permissions)
		if !ok {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("permissions contains an unknown or non-delegable permission."))
			return
		}
		perms = np
	}
	updated, err := s.store.UpdateCustomRole(r.Context(), nil, org.ID, role.ID, name, perms)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	members, _ := s.store.ListCustomRoleMembers(r.Context(), org.ID, role.ID)
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.role.updated", role.ID, r))
	httpx.JSON(w, r, http.StatusOK, roleJSON(updated, len(members)))
}

// handleAdminDeleteRole — DELETE /api/v1/admin/roles/{id} — borra el rol; sus
// asignaciones caen en cascada (FK). Los usuarios afectados pierden esos permisos en su
// próxima emisión de token (deny-by-default).
func (s *Server) handleAdminDeleteRole(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, role, apiErr := s.adminRoleInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	if _, err := s.store.DeleteCustomRole(r.Context(), org.ID, role.ID); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.role.deleted", role.ID, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": role.ID, "deleted": true})
}

// handleAdminAssignRole — POST /api/v1/admin/roles/{id}/members {user_id} — asigna el rol
// a un usuario de la org. Valida que el usuario pertenece a la org (400 si no): un rol no
// puede asignarse a alguien ajeno al tenant. Duplicado ⇒ 409.
func (s *Server) handleAdminAssignRole(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, role, apiErr := s.adminRoleInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	var req assignRoleRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !uuidRe.MatchString(req.UserID) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid user_id."))
		return
	}
	// El usuario debe ser miembro de ESTA org (aislamiento + integridad).
	if _, err := s.store.GetMembership(r.Context(), org.ID, req.UserID); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("That user does not belong to the organisation."))
		return
	}
	if err := s.store.AssignCustomRole(r.Context(), nil, org.ID, role.ID, req.UserID); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.role.assigned", role.ID, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"role_id": role.ID, "user_id": req.UserID})
}

// handleAdminUnassignRole — DELETE /api/v1/admin/roles/{id}/members/{userID}.
func (s *Server) handleAdminUnassignRole(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, role, apiErr := s.adminRoleInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	userID := chi.URLParam(r, "userID")
	if !uuidRe.MatchString(userID) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid user_id."))
		return
	}
	if err := s.store.UnassignCustomRole(r.Context(), nil, org.ID, role.ID, userID); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.role.unassigned", role.ID, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"role_id": role.ID, "user_id": userID, "removed": true})
}
