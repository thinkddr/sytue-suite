// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
)

// ---------------------------------------------------------------------------
// Tokens de API (credenciales programáticas para CI/CLI)
// ---------------------------------------------------------------------------

type createTokenRequest struct {
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	ExpiresAt string `json:"expires_at,omitempty"` // RFC3339 opcional
	ProjectID string `json:"project_id,omitempty"` // UUID opcional: confina el token a un proyecto
}

// CreateToken emite un token de API del tenant y devuelve el secreto UNA vez (en
// el campo `token`); después solo se persiste su hash. Solo humanos admin/owner.
func (h *Handlers) CreateToken(w http.ResponseWriter, r *http.Request) {
	var req createTokenRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var expires *time.Time
	if s := strings.TrimSpace(req.ExpiresAt); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("expires_at must be an RFC3339 date."))
			return
		}
		expires = &t
	}
	var projectID *uuid.UUID
	if s := strings.TrimSpace(req.ProjectID); s != "" {
		pid, err := uuid.Parse(s)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("project_id must be a UUID."))
			return
		}
		projectID = &pid
	}
	tok, err := h.svc.CreateAPIToken(r.Context(), service.CreateTokenInput{
		Name:      req.Name,
		Scope:     domain.TokenScope(strings.TrimSpace(req.Scope)),
		CreatedBy: subFromCtx(r),
		ExpiresAt: expires,
		ProjectID: projectID,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, tok)
}

// ListTokens lista los tokens del tenant (metadatos; nunca el secreto).
func (h *Handlers) ListTokens(w http.ResponseWriter, r *http.Request) {
	toks, err := h.svc.ListAPITokens(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if toks == nil {
		toks = []domain.APIToken{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"tokens": toks})
}

// RevokeToken revoca (borra) un token del tenant por ID.
func (h *Handlers) RevokeToken(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.RevokeAPIToken(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Miembros / RBAC de la organización
// ---------------------------------------------------------------------------

type addMemberRequest struct {
	Sub   string `json:"sub"`
	Email string `json:"email,omitempty"` // informativo; el identificador es sub
	Role  string `json:"role"`
}

type inviteMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// ListMembers lista los colaboradores de la organización.
func (h *Handlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	ms, err := h.svc.ListMembers(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if ms == nil {
		ms = []domain.Membership{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"members": ms})
}

// AddMember invita/actualiza un colaborador por sub (email opcional, informativo).
func (h *Handlers) AddMember(w http.ResponseWriter, r *http.Request) {
	var req addMemberRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	role := domain.Role(strings.TrimSpace(req.Role))
	// Nombrar owner, o cambiarle el rol a uno, es cosa de un owner: un admin no
	// puede subirse a sí mismo ni dejar fuera al dueño (CLOUD-16). Es la misma
	// regla que ya aplicaba InviteMember.
	if (role == domain.RoleOwner || h.isOwner(r, req.Sub)) && !h.humanHasRole(r, string(domain.RoleOwner)) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Only an owner can grant or change the owner role."))
		return
	}
	m, err := h.svc.AddMember(r.Context(), req.Sub, role)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, m)
}

// RemoveMember elimina un colaborador de la organización por sub.
func (h *Handlers) RemoveMember(w http.ResponseWriter, r *http.Request) {
	sub := strings.TrimSpace(chi.URLParam(r, "sub"))
	if sub == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("A collaborator identifier is required."))
		return
	}
	if h.isOwner(r, sub) && !h.humanHasRole(r, string(domain.RoleOwner)) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Only an owner can remove an owner."))
		return
	}
	if err := h.svc.RemoveMember(r.Context(), sub); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ListMemberInvitations lista el espejo pendiente (sin token). Una invitación
// no es todavía una membresía ni un puesto facturable.
func (h *Handlers) ListMemberInvitations(w http.ResponseWriter, r *http.Request) {
	invitations, err := h.svc.ListMemberInvitations(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if invitations == nil {
		invitations = []domain.MemberInvitation{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"invitations": invitations})
}

// InviteMember invita por correo delegando enlace, cuenta y pertenencia en ID.
func (h *Handlers) InviteMember(w http.ResponseWriter, r *http.Request) {
	var req inviteMemberRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	role := domain.Role(strings.TrimSpace(req.Role))
	if role == domain.RoleOwner && !h.humanHasRole(r, "owner") {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Only an owner can invite someone as owner."))
		return
	}
	result, err := h.svc.InviteMember(r.Context(), bearerValue(r), req.Email, role, subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, result)
}

// RevokeMemberInvitation revoca el token en ID y el espejo de Cloud.
func (h *Handlers) RevokeMemberInvitation(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.RevokeMemberInvitation(r.Context(), bearerValue(r), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// isOwner indica si sub es owner por membresía explícita. Ante un error de BD
// responde que sí: quien llama tendrá que ser owner (falla cerrado).
func (h *Handlers) isOwner(r *http.Request, sub string) bool {
	role, fromMembership, err := h.svc.EffectiveRole(r.Context(), strings.TrimSpace(sub))
	return err != nil || (fromMembership && role == domain.RoleOwner)
}
