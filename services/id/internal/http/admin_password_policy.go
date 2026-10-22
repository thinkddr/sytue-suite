// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Política de contraseñas de la organización activa (Sytue Admin). GET/PATCH
// acotados a la org del claim `tenant` (activeOrg), nunca a un id del path —
// mismo aislamiento por tenant que el resto de rutas admin. requireRole ya exige
// owner/admin. La política se APLICA en domain.ValidatePassword (choke point);
// aquí solo se lee/escribe.
package http

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func passwordPolicyJSON(p domain.PasswordPolicy) map[string]any {
	return map[string]any{"min_length": p.MinLength, "require_complexity": p.RequireComplexity}
}

// handleGetPasswordPolicy — GET /api/v1/admin/org/password-policy.
func (s *Server) handleGetPasswordPolicy(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := s.store.GetOrgPasswordPolicy(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, passwordPolicyJSON(p))
}

// patchPasswordPolicyRequest usa punteros para distinguir "no enviado" de un valor
// (PATCH parcial): solo se tocan los campos presentes.
type patchPasswordPolicyRequest struct {
	MinLength         *int  `json:"min_length"`
	RequireComplexity *bool `json:"require_complexity"`
}

// handlePatchPasswordPolicy — PATCH /api/v1/admin/org/password-policy. Valida la
// cota de min_length (8..128); parte de la política actual y aplica los campos
// enviados.
func (s *Server) handlePatchPasswordPolicy(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req patchPasswordPolicyRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := s.store.GetOrgPasswordPolicy(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if req.MinLength != nil {
		if *req.MinLength < domain.PasswordMinFloor || *req.MinLength > domain.PasswordMinCeil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("min_length is out of range (8..128)."))
			return
		}
		p.MinLength = *req.MinLength
	}
	if req.RequireComplexity != nil {
		p.RequireComplexity = *req.RequireComplexity
	}
	if err := s.store.UpdateOrgPasswordPolicy(r.Context(), nil, org.ID, p); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	c, _ := claimsFrom(r.Context())
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.password_policy.updated", org.ID, r))
	httpx.JSON(w, r, http.StatusOK, passwordPolicyJSON(p))
}
