// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Grupos DINÁMICOS (paridad Entra/Google dynamic groups): membresía calculada por una
// REGLA sobre atributos del usuario. El CRUD del grupo y su regla vive en admin_groups.go
// (POST/PATCH /admin/groups con `membership_rule`); aquí van los dos complementos:
//
//   - POST /api/v1/admin/groups/preview  — previsualiza a quién incluiría una regla ANTES
//     de guardarla (groups.manage). No persiste nada.
//   - POST /api/v1/admin/groups/{id}/recompute y POST /api/v1/admin/groups/recompute —
//     RECÁLCULO completo (un grupo / toda la org) contra el estado actual de los usuarios,
//     con el efecto real (cuántos entraron y cuántos salieron). Es la reparación manual de
//     la deriva: la regla creada ANTES que los usuarios, o el atributo que cambió por una
//     vía que no dispara reevaluación. El servicio además barre esto periódicamente
//     (cmd/id/main.go), pero un admin no tiene por qué esperar al tick.
//   - PATCH /api/v1/admin/users/{id}/profile — fija los atributos de perfil que alimentan
//     las reglas (department/title/location) y REEVALÚA los grupos dinámicos del usuario
//     en la org activa (users.manage). Es el disparador «cambió un atributo del usuario»;
//     el SCIM entrante hace lo propio al sincronizar (internal/scim).
//
// Aislamiento por tenant idéntico al resto del admin: org derivada del claim (activeOrg),
// nunca del path; el usuario debe pertenecer a la org (anti cross-tenant). Cada mutación
// se audita.
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

type previewRuleRequest struct {
	MembershipRule domain.Rule `json:"membership_rule"`
}

// handleAdminPreviewDynamicGroup — POST /api/v1/admin/groups/preview {membership_rule} —
// evalúa la regla contra los usuarios de la org activa SIN guardar y devuelve el total de
// coincidencias y una muestra (para el constructor de reglas de Console).
func (s *Server) handleAdminPreviewDynamicGroup(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req previewRuleRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := req.MembershipRule.Validate(); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(ruleErrMessage(err)))
		return
	}
	const sampleLimit = 20
	sample, total, err := s.store.PreviewDynamicMembers(r.Context(), org.ID, req.MembershipRule, sampleLimit)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	members := make([]map[string]any, 0, len(sample))
	for _, m := range sample {
		members = append(members, map[string]any{"user_id": m.UserID, "email": m.Email, "name": m.Name})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"count": total, "members": members})
}

// handleAdminRecomputeGroup — POST /api/v1/admin/groups/{id}/recompute — recomputa la
// membresía de UN grupo dinámico de la org activa contra el estado ACTUAL de los usuarios
// y devuelve el efecto REAL: cuántos entraron, cuántos salieron y cuántos quedan. 400 si
// el grupo es manual (no hay nada que calcular; su membresía es asignada).
func (s *Server) handleAdminRecomputeGroup(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, g, apiErr := s.adminGroupInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	if !g.Dynamic() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("This group is not dynamic: its membership is assigned by hand, so there is nothing to recompute."))
		return
	}
	var res store.RecomputeResult
	ctx := r.Context()
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		res, e = s.store.RecomputeDynamicGroup(ctx, q, org.ID, g.ID, *g.MembershipRule)
		return e
	})
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.group.recomputed", g.ID, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"group_id": g.ID, "added": res.Added, "removed": res.Removed, "member_count": res.Total,
	})
}

// handleAdminRecomputeGroups — POST /api/v1/admin/groups/recompute — recomputa TODOS los
// grupos dinámicos de la org activa (reparación de deriva en un clic). Sin N+1: una carga
// de usuarios por org y las K reglas evaluadas en memoria.
func (s *Server) handleAdminRecomputeGroups(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	results, err := s.store.RecomputeOrgDynamicGroups(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	added, removed := 0, 0
	for _, res := range results {
		added += res.Added
		removed += res.Removed
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.groups.recomputed", org.ID, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"groups": len(results), "added": added, "removed": removed,
	})
}

type setUserProfileRequest struct {
	Department *string `json:"department"`
	Title      *string `json:"title"`
	Location   *string `json:"location"`
}

// handleAdminSetUserProfile — PATCH /api/v1/admin/users/{id}/profile {department?, title?,
// location?} — fija los atributos de perfil de un usuario de la org activa y REEVALÚA sus
// grupos dinámicos (entra/sale según cumpla ahora las reglas). Campos omitidos conservan
// su valor. El usuario debe pertenecer a la org (400 si no).
func (s *Server) handleAdminSetUserProfile(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid user id."))
		return
	}
	// El usuario debe ser miembro de ESTA org (aislamiento + integridad).
	if _, err := s.store.GetMembership(r.Context(), org.ID, id); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("That user does not belong to the organisation."))
		return
	}
	existing, err := s.store.GetUserByID(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	var req setUserProfileRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	dept, title, loc := existing.Department, existing.Title, existing.Location
	if req.Department != nil {
		dept = domain.SanitizeDisplayName(*req.Department)
	}
	if req.Title != nil {
		title = domain.SanitizeDisplayName(*req.Title)
	}
	if req.Location != nil {
		loc = domain.SanitizeDisplayName(*req.Location)
	}
	// Fijar atributos + reevaluar los grupos dinámicos del usuario en UNA tx: la
	// materialización en group_members queda consistente con los atributos nuevos, así
	// la SIGUIENTE emisión de token (claim `groups`) ya refleja la nueva membresía.
	var updated domain.User
	ctx := r.Context()
	err = s.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		if updated, e = s.store.SetUserAttributes(ctx, q, id, dept, title, loc); e != nil {
			return e
		}
		return s.store.ReevaluateUserDynamicGroups(ctx, q, org.ID, id)
	})
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.user.profile_updated", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"id":         updated.ID,
		"email":      updated.Email,
		"department": updated.Department,
		"title":      updated.Title,
		"location":   updated.Location,
	})
}
