// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// PIM (Privileged Identity Management, mig 0020) — ADMIN. Plano de administración del
// acceso privilegiado just-in-time: definir ELEGIBILIDADES (quién puede elevar a qué
// rol, con o sin aprobación y con qué ventana máxima), APROBAR/DENEGAR las solicitudes
// pendientes y ver las activaciones vigentes.
//
// RESERVADO A owner/admin (como la gestión de roles, mig 0019): estas rutas van gateadas
// por requireRole("owner","admin") — gobernar el acceso privilegiado no se delega. La org
// se deriva SIEMPRE del claim (activeOrg → org.ID), nunca del path; todos los métodos de
// store cruzan org_id (anti-IDOR). Cada mutación se AUDITA.
package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// handleAdminListEligibilities — GET /api/v1/admin/pim/eligibilities — elegibilidades de
// la org activa (quién es elegible para qué rol).
func (s *Server) handleAdminListEligibilities(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	list, err := s.store.ListEligibilities(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, e := range list {
		out = append(out, eligibilityJSON(e))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"eligibilities": out})
}

type createEligibilityRequest struct {
	UserID             string `json:"user_id"`
	RoleID             string `json:"role_id"`
	BuiltinRole        string `json:"builtin_role"`
	RequiresApproval   bool   `json:"requires_approval"`
	MaxDurationMinutes int    `json:"max_duration_minutes"`
}

// handleAdminCreateEligibility — POST /api/v1/admin/pim/eligibilities {user_id,
// role_id?|builtin_role?, requires_approval, max_duration_minutes}. Valida que el usuario
// es miembro de la org y que el rol personalizado (si aplica) es de la org (aislamiento).
// Un built-in debe ser elevable (owner/admin). Duplicado ⇒ 409.
func (s *Server) handleAdminCreateEligibility(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req createEligibilityRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !uuidRe.MatchString(req.UserID) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid user_id."))
		return
	}
	roleID, builtinRole, apiErr := resolveRoleRef(req.RoleID, req.BuiltinRole)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	if req.MaxDurationMinutes < 1 || req.MaxDurationMinutes > 1440 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("max_duration_minutes must be between 1 and 1440."))
		return
	}
	// El usuario debe ser miembro de ESTA org (aislamiento + integridad).
	if _, err := s.store.GetMembership(r.Context(), org.ID, req.UserID); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("That user does not belong to the organisation."))
		return
	}
	// Un rol personalizado debe ser de ESTA org (un id de otro tenant ⇒ 404 anti-IDOR).
	if roleID != "" {
		if _, err := s.store.GetCustomRole(r.Context(), org.ID, roleID); err != nil {
			httpx.Fail(w, r, mapDomainErr(err))
			return
		}
	}
	elig, err := s.store.CreateEligibility(r.Context(), org.ID, req.UserID, roleID, builtinRole,
		req.RequiresApproval, req.MaxDurationMinutes)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	detail := roleRefDetail(elig.RoleID, elig.BuiltinRole, elig.RoleName)
	detail["target_user"] = elig.UserID
	detail["requires_approval"] = elig.RequiresApproval
	detail["max_duration_minutes"] = elig.MaxDurationMinutes
	s.pimAudit(r.Context(), c.Subject, org.ID, "pim.eligibility.created", elig.ID, r, detail)
	httpx.JSON(w, r, http.StatusCreated, eligibilityJSON(elig))
}

// handleAdminDeleteEligibility — DELETE /api/v1/admin/pim/eligibilities/{id} — retira una
// elegibilidad (no afecta a activaciones ya en curso). 404 si no existe en la org.
func (s *Server) handleAdminDeleteEligibility(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid id."))
		return
	}
	ok, err := s.store.DeleteEligibility(r.Context(), org.ID, id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if !ok {
		httpx.Fail(w, r, httpx.ErrNotFound.WithMessage("Eligibility not found."))
		return
	}
	s.pimAudit(r.Context(), c.Subject, org.ID, "pim.eligibility.deleted", id, r, nil)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "deleted": true})
}

// handleAdminListActivations — GET /api/v1/admin/pim/activations?status= — activaciones
// de la org. `status` (opcional) filtra: pending (para aprobar) o active (vigentes). El
// estado efectivo (expiración perezosa) va en cada fila (effective_status).
func (s *Server) handleAdminListActivations(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	status := r.URL.Query().Get("status")
	switch status {
	case "", domain.ActivationPending, domain.ActivationActive,
		domain.ActivationRevoked, domain.ActivationDenied, domain.ActivationExpired:
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid status."))
		return
	}
	list, err := s.store.ListActivations(r.Context(), org.ID, "", status)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	now := time.Now()
	out := make([]map[string]any, 0, len(list))
	for _, a := range list {
		out = append(out, activationJSON(a, now))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"activations": out})
}

// handleAdminApproveActivation — POST /api/v1/admin/pim/activations/{id}/approve —
// concede una activación PENDIENTE: pasa a active y fija la ventana [ahora, ahora +
// duración]. 404 si no está pendiente en la org.
func (s *Server) handleAdminApproveActivation(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid id."))
		return
	}
	act, err := s.store.ApproveActivation(r.Context(), org.ID, id, c.Subject, time.Now())
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err)) // ErrNotFound ⇒ 404 (no pendiente / otro tenant)
		return
	}
	detail := roleRefDetail(act.RoleID, act.BuiltinRole, act.RoleName)
	detail["target_user"] = act.UserID
	if act.ExpiresAt != nil {
		detail["expires_at"] = act.ExpiresAt.UTC().Format(time.RFC3339)
	}
	s.pimAudit(r.Context(), c.Subject, org.ID, "pim.activation.approved", act.ID, r, detail)
	httpx.JSON(w, r, http.StatusOK, activationJSON(act, time.Now()))
}

// handleAdminDenyActivation — POST /api/v1/admin/pim/activations/{id}/deny — rechaza una
// activación PENDIENTE. 404 si no está pendiente en la org.
func (s *Server) handleAdminDenyActivation(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid id."))
		return
	}
	ok, err := s.store.DenyActivation(r.Context(), org.ID, id, c.Subject)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if !ok {
		httpx.Fail(w, r, httpx.ErrNotFound.WithMessage("There is no pending request with that id."))
		return
	}
	s.pimAudit(r.Context(), c.Subject, org.ID, "pim.activation.denied", id, r, nil)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "status": domain.ActivationDenied})
}
