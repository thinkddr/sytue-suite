// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Administración del Acceso Condicional (paridad Entra Conditional Access) desde
// Sytue Admin. CRUD de las access_policies de la organización ACTIVA del solicitante
// (claim tenant; requireRole ya exige owner/admin para esa org). Todo se acota por org
// (aislamiento tenant, fail-closed): una policy de otra org ⇒ 404, nunca se toca. La
// evaluación real (enforcement) vive en el login (access.go/login.go/session.go); esto
// es su plano de administración.
package http

import (
	"net/http"
	"net/netip"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// orEmpty garantiza un slice no-nil para que el JSON serialice `[]` y no `null`.
func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// accessPolicyWire es la forma JSON de una política para la Console.
func accessPolicyWire(p domain.AccessPolicy) map[string]any {
	return map[string]any{
		"id":                     p.ID,
		"name":                   p.Name,
		"enabled":                p.Enabled,
		"priority":               p.Priority,
		"action":                 string(p.Action),
		"include_users":          orEmpty(p.IncludeUsers),
		"include_groups":         orEmpty(p.IncludeGroups),
		"exclude_users":          orEmpty(p.ExcludeUsers),
		"exclude_groups":         orEmpty(p.ExcludeGroups),
		"ip_ranges":              orEmpty(p.IPRanges),
		"ip_negate":              p.IPNegate,
		"client_ids":             orEmpty(p.ClientIDs),
		"require_trusted_device": p.RequireTrustedDevice,
		"days_mask":              p.DaysMask,
		"start_minute":           p.StartMinute,
		"end_minute":             p.EndMinute,
		"created_at":             p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":             p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

// accessPolicyRequest es el cuerpo de create/update. Punteros/omitidos = "no tocar"
// (PATCH); un array presente (aunque vacío) REEMPLAZA el conjunto.
type accessPolicyRequest struct {
	Name                 *string  `json:"name"`
	Enabled              *bool    `json:"enabled"`
	Priority             *int     `json:"priority"`
	Action               *string  `json:"action"`
	IncludeUsers         []string `json:"include_users"`
	IncludeGroups        []string `json:"include_groups"`
	ExcludeUsers         []string `json:"exclude_users"`
	ExcludeGroups        []string `json:"exclude_groups"`
	IPRanges             []string `json:"ip_ranges"`
	IPNegate             *bool    `json:"ip_negate"`
	ClientIDs            []string `json:"client_ids"`
	RequireTrustedDevice *bool    `json:"require_trusted_device"`
	DaysMask             *int     `json:"days_mask"`
	StartMinute          *int     `json:"start_minute"`
	EndMinute            *int     `json:"end_minute"`
}

// normalizeStrs recorta, descarta vacíos y deduplica (para CIDRs y client_ids).
func normalizeStrs(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// validateUUIDs normaliza y valida una lista de UUID (users/groups). Un id ajeno a la
// org no puede afectar a otro tenant (el motor solo casa contra los grupos/id del
// usuario de ESTA org), así que basta validar el formato — no revela pertenencia.
func validateUUIDs(in []string) ([]string, bool) {
	out := normalizeStrs(in)
	for _, v := range out {
		if _, err := uuid.Parse(v); err != nil {
			return nil, false
		}
	}
	return out, true
}

// validateCIDRs normaliza y valida rangos CIDR (net/netip, sin red).
func validateCIDRs(in []string) ([]string, bool) {
	out := normalizeStrs(in)
	for _, v := range out {
		if _, err := netip.ParsePrefix(v); err != nil {
			return nil, false
		}
	}
	return out, true
}

// clampMinute acota un minuto-del-día: -1 (sin ventana) o 0..1439.
func clampMinute(m int) int {
	if m < 0 {
		return -1
	}
	if m > 1439 {
		return 1439
	}
	return m
}

// applyPolicyRequest vuelca los campos presentes de req sobre p, validando. Devuelve
// un error HTTP 400 (mensaje sobrio) si algún valor es inválido. En create, name y
// action son obligatorios (el llamante los exige).
func applyPolicyRequest(p *domain.AccessPolicy, req accessPolicyRequest) error {
	if req.Name != nil {
		p.Name = strings.TrimSpace(*req.Name)
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	if req.Priority != nil {
		p.Priority = *req.Priority
	}
	if req.Action != nil {
		act := strings.TrimSpace(*req.Action)
		if !domain.ValidAccessAction(act) {
			return httpx.ErrBadRequest.WithMessage("action must be allow, require_mfa or block.")
		}
		p.Action = domain.AccessAction(act)
	}
	if ok := applyIDList(&p.IncludeUsers, req.IncludeUsers); !ok {
		return httpx.ErrBadRequest.WithMessage("include_users: invalid UUIDs.")
	}
	if ok := applyIDList(&p.IncludeGroups, req.IncludeGroups); !ok {
		return httpx.ErrBadRequest.WithMessage("include_groups: invalid UUIDs.")
	}
	if ok := applyIDList(&p.ExcludeUsers, req.ExcludeUsers); !ok {
		return httpx.ErrBadRequest.WithMessage("exclude_users: invalid UUIDs.")
	}
	if ok := applyIDList(&p.ExcludeGroups, req.ExcludeGroups); !ok {
		return httpx.ErrBadRequest.WithMessage("exclude_groups: invalid UUIDs.")
	}
	if req.IPRanges != nil {
		cidrs, ok := validateCIDRs(req.IPRanges)
		if !ok {
			return httpx.ErrBadRequest.WithMessage("ip_ranges: one of the CIDRs is not valid (e.g. 203.0.113.0/24).")
		}
		p.IPRanges = cidrs
	}
	if req.IPNegate != nil {
		p.IPNegate = *req.IPNegate
	}
	if req.ClientIDs != nil {
		p.ClientIDs = normalizeStrs(req.ClientIDs)
	}
	if req.RequireTrustedDevice != nil {
		p.RequireTrustedDevice = *req.RequireTrustedDevice
	}
	if req.DaysMask != nil {
		p.DaysMask = *req.DaysMask & 0x7f // 7 bits (Domingo..Sábado); ignora bits sobrantes.
	}
	if req.StartMinute != nil {
		p.StartMinute = clampMinute(*req.StartMinute)
	}
	if req.EndMinute != nil {
		p.EndMinute = clampMinute(*req.EndMinute)
	}
	if p.Name == "" {
		return httpx.ErrBadRequest.WithMessage("name is required.")
	}
	return nil
}

// applyIDList valida y reemplaza *dst con la lista de UUID si viene presente (no nil).
func applyIDList(dst *[]string, in []string) bool {
	if in == nil {
		return true
	}
	ids, ok := validateUUIDs(in)
	if !ok {
		return false
	}
	*dst = ids
	return true
}

// handleAdminListAccessPolicies — GET /api/v1/admin/access-policies — políticas de la
// org activa, por prioridad (incluye deshabilitadas).
func (s *Server) handleAdminListAccessPolicies(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	list, err := s.store.ListAccessPolicies(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	items := make([]map[string]any, 0, len(list))
	for _, p := range list {
		items = append(items, accessPolicyWire(p))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// handleAdminCreateAccessPolicy — POST /api/v1/admin/access-policies — crea una
// política en la org activa. name y action son obligatorios.
func (s *Server) handleAdminCreateAccessPolicy(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	var req accessPolicyRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.Action == nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("action is required (allow, require_mfa or block)."))
		return
	}
	// Defaults antes de aplicar: prioridad 100, sin ventana horaria.
	p := domain.AccessPolicy{
		OrgID: org.ID, Priority: 100, StartMinute: -1, EndMinute: -1, CreatedBy: c.Subject,
	}
	if err := applyPolicyRequest(&p, req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	created, err := s.store.CreateAccessPolicy(r.Context(), nil, p)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "access_policy.created", created.ID, r))
	httpx.JSON(w, r, http.StatusCreated, accessPolicyWire(created))
}

// handleAdminGetAccessPolicy — GET /api/v1/admin/access-policies/{id} — una política
// de la org activa. Cross-org ⇒ 404.
func (s *Server) handleAdminGetAccessPolicy(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := s.loadOrgPolicy(r, org.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, accessPolicyWire(p))
}

// handleAdminUpdateAccessPolicy — PATCH /api/v1/admin/access-policies/{id} — edita una
// política de la org activa (solo los campos presentes).
func (s *Server) handleAdminUpdateAccessPolicy(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	p, err := s.loadOrgPolicy(r, org.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req accessPolicyRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := applyPolicyRequest(&p, req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	updated, err := s.store.UpdateAccessPolicy(r.Context(), nil, p)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "access_policy.updated", updated.ID, r))
	httpx.JSON(w, r, http.StatusOK, accessPolicyWire(updated))
}

// handleAdminDeleteAccessPolicy — DELETE /api/v1/admin/access-policies/{id} — borra una
// política de la org activa. Cross-org ⇒ 404.
func (s *Server) handleAdminDeleteAccessPolicy(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	id := chi.URLParam(r, "id")
	if _, perr := uuid.Parse(id); perr != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	ok, err := s.store.DeleteAccessPolicy(r.Context(), org.ID, id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if !ok {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "access_policy.deleted", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "deleted": true})
}

// loadOrgPolicy carga la política {id} acotada a la org (cross-org/inexistente ⇒
// ErrNotFound HTTP, sin revelar su existencia).
func (s *Server) loadOrgPolicy(r *http.Request, orgID string) (domain.AccessPolicy, error) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		return domain.AccessPolicy{}, httpx.ErrNotFound
	}
	p, err := s.store.GetAccessPolicy(r.Context(), orgID, id)
	if err != nil {
		return domain.AccessPolicy{}, mapDomainErr(err)
	}
	return p, nil
}
