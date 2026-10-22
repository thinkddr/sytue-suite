// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// PIM (Privileged Identity Management, mig 0020) — SELF-SERVICE. Elevación de
// privilegios just-in-time: un usuario ELEGIBLE para un rol lo ACTIVA por una ventana
// acotada (con justificación) y, si la elegibilidad lo exige, previa aprobación. Solo
// mientras hay una activación ACTIVA sus permisos entran en el claim `perms` del token
// (buildTokens → EffectivePerms); la elegibilidad por sí sola no concede nada.
//
// Estas rutas van bajo /api/v1 tras requireAccessToken (cualquier usuario autenticado:
// la elegibilidad, no un rol admin, gobierna qué puede activar). La org es SIEMPRE la
// activa del token (activeOrg), nunca del path — aislamiento por tenant. Cada activación
// y desactivación se AUDITA (quién, rol, ventana, justificación).
package http

import (
	"context"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// eligibilityJSON es la forma de una elegibilidad en las respuestas (contrato Console).
func eligibilityJSON(e store.Eligibility) map[string]any {
	return map[string]any{
		"id":                   e.ID,
		"user_id":              e.UserID,
		"user_email":           e.UserEmail,
		"user_name":            e.UserName,
		"role_id":              e.RoleID,
		"builtin_role":         e.BuiltinRole,
		"role_name":            e.RoleName,
		"requires_approval":    e.RequiresApproval,
		"max_duration_minutes": e.MaxDurationMinutes,
	}
}

// activationJSON es la forma de una activación. effective_status y remaining_seconds
// aplican la expiración PEREZOSA contra `now`: una fila 'active' caducada se reporta
// 'expired' (como deja de conceder perms), sin depender de un barrido de estado.
func activationJSON(a store.Activation, now time.Time) map[string]any {
	out := map[string]any{
		"id":               a.ID,
		"user_id":          a.UserID,
		"user_email":       a.UserEmail,
		"user_name":        a.UserName,
		"role_id":          a.RoleID,
		"builtin_role":     a.BuiltinRole,
		"role_name":        a.RoleName,
		"status":           a.Status,
		"justification":    a.Justification,
		"duration_minutes": a.DurationMinutes,
		"requested_at":     a.RequestedAt.UTC().Format(time.RFC3339),
	}
	eff := a.Status
	if a.ExpiresAt != nil {
		if a.ActivatedAt != nil {
			out["activated_at"] = a.ActivatedAt.UTC().Format(time.RFC3339)
		}
		out["expires_at"] = a.ExpiresAt.UTC().Format(time.RFC3339)
		eff = domain.EffectiveStatus(a.Status, *a.ExpiresAt, now)
		if eff == domain.ActivationActive {
			out["remaining_seconds"] = int(a.ExpiresAt.Sub(now).Seconds())
		}
	}
	out["effective_status"] = eff
	if a.ApproverEmail != "" {
		out["approved_by_email"] = a.ApproverEmail
	}
	return out
}

// pimAudit registra una entrada de auditoría de acceso privilegiado con detalle
// (quién/rol/ventana/justificación). Best-effort como el resto de la auditoría admin.
func (s *Server) pimAudit(ctx context.Context, actor, orgID, action, target string, r *http.Request, detail map[string]any) {
	if detail == nil {
		detail = map[string]any{}
	}
	detail["at"] = time.Now().UTC().Format(time.RFC3339)
	_ = s.store.Audit(ctx, nil, store.AuditEntry{
		ActorID: actor, OrgID: orgID, Action: action, Target: target,
		IP: clientIP(r), Detail: detail,
	})
}

// roleRefDetail describe el rol de una activación/elegibilidad para el detalle de
// auditoría, sin exponer más que la referencia y el nombre.
func roleRefDetail(roleID, builtinRole, roleName string) map[string]any {
	d := map[string]any{"role_name": roleName}
	if roleID != "" {
		d["role_id"] = roleID
	} else {
		d["builtin_role"] = builtinRole
	}
	return d
}

// handleMePIM — GET /api/v1/me/pim — mis elegibilidades (para qué roles puedo elevar) y
// mis activaciones (vigentes/pasadas, con cuánto queda) en la org activa.
func (s *Server) handleMePIM(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	elig, err := s.store.ListEligibilitiesForUser(r.Context(), org.ID, c.Subject)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	acts, err := s.store.ListActivations(r.Context(), org.ID, c.Subject, "")
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	now := time.Now()
	eJSON := make([]map[string]any, 0, len(elig))
	for _, e := range elig {
		eJSON = append(eJSON, eligibilityJSON(e))
	}
	aJSON := make([]map[string]any, 0, len(acts))
	for _, a := range acts {
		aJSON = append(aJSON, activationJSON(a, now))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"eligibilities": eJSON, "activations": aJSON})
}

type activateRequest struct {
	RoleID          string `json:"role_id"`
	BuiltinRole     string `json:"builtin_role"`
	Justification   string `json:"justification"`
	DurationMinutes int    `json:"duration_minutes"`
}

// resolveRoleRef valida que el cuerpo trae EXACTAMENTE una referencia de rol (role_id
// XOR builtin_role) bien formada, devolviéndola. role_id debe ser uuid; un built-in debe
// ser elevable (owner/admin). Verja compartida por activar y crear elegibilidad.
func resolveRoleRef(roleID, builtinRole string) (string, string, error) {
	hasCustom, hasBuiltin := roleID != "", builtinRole != ""
	if hasCustom == hasBuiltin {
		return "", "", httpx.ErrBadRequest.WithMessage("Give exactly one: role_id or builtin_role.")
	}
	if hasCustom {
		if !uuidRe.MatchString(roleID) {
			return "", "", httpx.ErrBadRequest.WithMessage("Invalid role_id.")
		}
		return roleID, "", nil
	}
	if !domain.ElevatableBuiltin(builtinRole) {
		return "", "", httpx.ErrBadRequest.WithMessage("builtin_role must be owner or admin.")
	}
	return "", builtinRole, nil
}

// handleMePIMActivate — POST /api/v1/me/pim/activate {role_id?|builtin_role?,
// justification, duration_minutes}. Exige elegibilidad (si no, 403), justificación no
// vacía y duración dentro del máximo de la elegibilidad. Si la elegibilidad requiere
// aprobación queda 'pending'; si no, 'active' hasta expires_at. 201 con la activación.
func (s *Server) handleMePIMActivate(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req activateRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	roleID, builtinRole, apiErr := resolveRoleRef(req.RoleID, req.BuiltinRole)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	justification := domain.SanitizeDisplayName(req.Justification)
	if justification == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("A justification is required."))
		return
	}
	// La elegibilidad gobierna: sin ella no se puede elevar (deny-by-default).
	elig, err := s.store.GetEligibilityForUser(r.Context(), org.ID, c.Subject, roleID, builtinRole)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("You are not eligible for that role."))
		return
	}
	if !domain.ValidActivationDuration(req.DurationMinutes, elig.MaxDurationMinutes) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The duration exceeds the maximum allowed for this role."))
		return
	}
	act, err := s.store.CreateActivation(r.Context(), org.ID, c.Subject, roleID, builtinRole,
		justification, req.DurationMinutes, elig.RequiresApproval, time.Now())
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	action := "pim.activation.activated"
	if act.Status == domain.ActivationPending {
		action = "pim.activation.requested"
	}
	detail := roleRefDetail(act.RoleID, act.BuiltinRole, act.RoleName)
	detail["duration_minutes"] = act.DurationMinutes
	detail["justification"] = justification
	detail["status"] = act.Status
	s.pimAudit(r.Context(), c.Subject, org.ID, action, act.ID, r, detail)
	httpx.JSON(w, r, http.StatusCreated, activationJSON(act, time.Now()))
}

type deactivateRequest struct {
	ActivationID string `json:"activation_id"`
}

// handleMePIMDeactivate — POST /api/v1/me/pim/deactivate {activation_id} — revoca una
// activación ACTIVA propia antes de que expire. Los perms desaparecen en la próxima
// emisión de token. 404 si no es una activación activa tuya.
func (s *Server) handleMePIMDeactivate(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req deactivateRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !uuidRe.MatchString(req.ActivationID) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid activation_id."))
		return
	}
	ok, err := s.store.DeactivateActivation(r.Context(), org.ID, c.Subject, req.ActivationID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if !ok {
		httpx.Fail(w, r, httpx.ErrNotFound.WithMessage("There is no active activation with that id."))
		return
	}
	s.pimAudit(r.Context(), c.Subject, org.ID, "pim.activation.deactivated", req.ActivationID, r, nil)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": req.ActivationID, "status": domain.ActivationRevoked})
}
