// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// CRUD admin de GRUPOS de la organización activa desde Sytue Admin, y la base
// del claim `groups` del JWT que desbloquea el share por-grupo de Sytue Drive.
//
// Aislamiento por tenant (docs/CONVENTIONS.md §0.7), mismo anclaje que
// admin_users.go: requireRole ya exige owner/admin (deny-by-default); estas rutas
// derivan SIEMPRE la org del claim del token (activeOrg → org.ID), NUNCA del path,
// y todos los métodos de store cruzan groups.org_id — un group_id o user_id de
// otro tenant da 404, no una acción cross-tenant (anti-IDOR). Cada mutación se
// audita. AddGroupMember valida que el usuario pertenece a la org (400 si no).
//
// El aprovisionamiento externo de grupos por SCIM 2.0 (RFC 7643 §4.2, Entra/Okta)
// vive en internal/scim (scim_groups.go) y reutiliza el mismo modelo/store y
// aislamiento por org que este CRUD admin; ambos escriben en groups/group_members
// y alimentan el claim `groups` del JWT.
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

type createGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// MembershipRule, si viene, crea el grupo como DINÁMICO (paridad Entra/Google):
	// su membresía la calcula la regla y no admite asignación manual.
	MembershipRule *domain.Rule `json:"membership_rule"`
}

type patchGroupRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	// MembershipRule presente ⇒ fija/actualiza la regla y recomputa la membresía
	// (convierte el grupo en dinámico si no lo era). Omitido ⇒ no toca la regla.
	MembershipRule *domain.Rule `json:"membership_rule"`
}

type addGroupMemberRequest struct {
	UserID string `json:"user_id"`
}

// groupJSON es la forma de un grupo en las respuestas admin (contrato Console). Incluye
// `dynamic` y, si lo es, su `membership_rule` (para el constructor de reglas de Console).
func groupJSON(g domain.Group, memberCount int) map[string]any {
	out := map[string]any{
		"id":           g.ID,
		"name":         g.Name,
		"description":  g.Description,
		"member_count": memberCount,
		"dynamic":      g.Dynamic(),
	}
	if g.MembershipRule != nil {
		out["membership_rule"] = g.MembershipRule
	}
	return out
}

// handleAdminListGroups — GET /api/v1/admin/groups — grupos de la org activa con
// member_count, orden estable por nombre.
func (s *Server) handleAdminListGroups(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	list, err := s.store.ListGroups(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	groups := make([]map[string]any, 0, len(list))
	for _, g := range list {
		groups = append(groups, groupJSON(g.Group, g.MemberCount))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"groups": groups})
}

// handleAdminCreateGroup — POST /api/v1/admin/groups {name, description?} — crea un
// grupo en la org activa. name obligatorio y único por-tenant (409 en duplicado).
func (s *Server) handleAdminCreateGroup(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req createGroupRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name := domain.SanitizeDisplayName(req.Name)
	if name == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("A group name is required."))
		return
	}
	if req.MembershipRule != nil {
		if err := req.MembershipRule.Validate(); err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(ruleErrMessage(err)))
			return
		}
	}
	// Alta + (si es dinámico) fijar regla y recomputar la membresía en UNA tx: el grupo
	// nunca queda a medias (creado sin su membresía materializada).
	var (
		g     domain.Group
		count int
	)
	ctx := r.Context()
	err = s.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		if g, e = s.store.CreateGroup(ctx, q, org.ID, name, domain.SanitizeDisplayName(req.Description)); e != nil {
			return e
		}
		if req.MembershipRule != nil {
			if g, e = s.store.SetGroupRule(ctx, q, org.ID, g.ID, *req.MembershipRule); e != nil {
				return e
			}
			var res store.RecomputeResult
			res, e = s.store.RecomputeDynamicGroup(ctx, q, org.ID, g.ID, *req.MembershipRule)
			count = res.Total
		}
		return e
	})
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.group.created", g.ID, r))
	httpx.JSON(w, r, http.StatusCreated, groupJSON(g, count))
}

// ruleErrMessage traduce un error de validación de regla (domain.ErrInvalidInput
// envuelto con contexto) a un mensaje de 400 legible para la Console.
func ruleErrMessage(err error) string {
	return "Regla de membresía inválida: " + err.Error()
}

// adminGroupInOrg resuelve la org activa y verifica que {id} sea un uuid y un
// grupo de ELLA, devolviendo el grupo. Verja anti-IDOR compartida por las rutas
// de un grupo concreto (un id de otro tenant ⇒ 404).
func (s *Server) adminGroupInOrg(r *http.Request) (domain.Organization, domain.Group, error) {
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		return domain.Organization{}, domain.Group{}, httpx.ErrBadRequest.WithMessage("Invalid group id.")
	}
	org, err := s.activeOrg(r)
	if err != nil {
		return domain.Organization{}, domain.Group{}, err
	}
	g, err := s.store.GetGroup(r.Context(), org.ID, id)
	if err != nil {
		return org, domain.Group{}, mapDomainErr(err)
	}
	return org, g, nil
}

// handleAdminGetGroup — GET /api/v1/admin/groups/{id} — el grupo y sus miembros.
func (s *Server) handleAdminGetGroup(w http.ResponseWriter, r *http.Request) {
	org, g, apiErr := s.adminGroupInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	members, err := s.store.ListGroupMembers(r.Context(), org.ID, g.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	out := make([]map[string]any, 0, len(members))
	for _, m := range members {
		out = append(out, map[string]any{"user_id": m.UserID, "email": m.Email, "name": m.Name})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"group":   groupJSON(g, len(members)),
		"members": out,
	})
}

// handleAdminUpdateGroup — PATCH /api/v1/admin/groups/{id} {name?, description?}.
// Campos omitidos conservan su valor; name presente debe ser no vacío.
func (s *Server) handleAdminUpdateGroup(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, g, apiErr := s.adminGroupInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	var req patchGroupRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name, description := g.Name, g.Description
	// ponytail: RENOMBRAR un grupo cambia qué shares por-grupo de Drive le aplican —
	// Drive matchea shares.subject (texto libre) contra el claim `groups` (= nombres),
	// no contra un id estable. Renombrar «eng»→«engineering» deja sin efecto los shares
	// a «eng»; recrear otro grupo «eng» heredaría esos shares (over-grant). Acoplamiento
	// inherente al modelo de share por-nombre de Drive. Upgrade: que Drive ligue por
	// group_id inmutable (claim con ids + resolución) — follow-up cross-servicio.
	if req.Name != nil {
		name = domain.SanitizeDisplayName(*req.Name)
		if name == "" {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The group name cannot be left empty."))
			return
		}
	}
	if req.Description != nil {
		description = domain.SanitizeDisplayName(*req.Description)
	}
	if req.MembershipRule != nil {
		if err := req.MembershipRule.Validate(); err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(ruleErrMessage(err)))
			return
		}
	}
	// Nombre/descripción y —si viene regla— fijarla y recomputar la membresía en UNA tx.
	// Poner una regla sobre un grupo antes manual lo convierte en dinámico: el recómputo
	// reemplaza su membresía por la que dicta la regla (los miembros manuales previos
	// quedan sustituidos, semántica esperada de «ahora este grupo es dinámico»).
	updated := g
	ctx := r.Context()
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		if updated, e = s.store.UpdateGroup(ctx, q, org.ID, g.ID, name, description); e != nil {
			return e
		}
		if req.MembershipRule != nil {
			if updated, e = s.store.SetGroupRule(ctx, q, org.ID, g.ID, *req.MembershipRule); e != nil {
				return e
			}
			_, e = s.store.RecomputeDynamicGroup(ctx, q, org.ID, g.ID, *req.MembershipRule)
		}
		return e
	})
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	members, _ := s.store.ListGroupMembers(r.Context(), org.ID, g.ID)
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.group.updated", g.ID, r))
	httpx.JSON(w, r, http.StatusOK, groupJSON(updated, len(members)))
}

// handleAdminDeleteGroup — DELETE /api/v1/admin/groups/{id} — borra el grupo; sus
// miembros caen en cascada (FK). Idempotente-seguro: la verja ya dio 404 si no
// existía en la org.
func (s *Server) handleAdminDeleteGroup(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, g, apiErr := s.adminGroupInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	if _, err := s.store.DeleteGroup(r.Context(), nil, org.ID, g.ID); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.group.deleted", g.ID, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": g.ID, "deleted": true})
}

// handleAdminAddGroupMember — POST /api/v1/admin/groups/{id}/members {user_id}.
// Valida que el usuario pertenece a la org (400 si no): un grupo no puede incluir
// a alguien ajeno al tenant. Duplicado ⇒ 409.
func (s *Server) handleAdminAddGroupMember(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, g, apiErr := s.adminGroupInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	if g.Dynamic() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("This is a dynamic group: its membership is governed by the rule, so members cannot be added by hand."))
		return
	}
	var req addGroupMemberRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !uuidRe.MatchString(req.UserID) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid user_id."))
		return
	}
	// El usuario debe ser miembro de ESTA org (aislamiento + integridad): si no
	// tiene membresía, no existe para este tenant ⇒ 400 sin revelar nada más.
	if _, err := s.store.GetMembership(r.Context(), org.ID, req.UserID); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("That user does not belong to the organisation."))
		return
	}
	if err := s.store.AddGroupMember(r.Context(), nil, org.ID, g.ID, req.UserID); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.group.member_added", g.ID, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"group_id": g.ID, "user_id": req.UserID})
}

// handleAdminRemoveGroupMember — DELETE /api/v1/admin/groups/{id}/members/{userID}.
func (s *Server) handleAdminRemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, g, apiErr := s.adminGroupInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	if g.Dynamic() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("This is a dynamic group: its membership is governed by the rule, so members cannot be removed by hand."))
		return
	}
	userID := chi.URLParam(r, "userID")
	if !uuidRe.MatchString(userID) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid user_id."))
		return
	}
	if err := s.store.RemoveGroupMember(r.Context(), nil, org.ID, g.ID, userID); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.group.member_removed", g.ID, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"group_id": g.ID, "user_id": userID, "removed": true})
}
