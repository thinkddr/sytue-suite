// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Gestión de organizaciones (tenants) desde Sytue Admin, sobre el modelo de
// orgs/membresías YA existente (mismo que usa el motor OIDC/SCIM). Modelo de
// autorización, fail-closed y sin super-admin de plataforma:
//
//   - LISTAR: cada admin ve SOLO las orgs que gestiona — aquellas en las que es
//     owner o admin (ListManagedOrgsPage lo aísla por membresía). La lista ES su
//     alcance: no existe un rol global que vea todas las orgs de la plataforma.
//   - CREAR: cualquiera que ya sea owner/admin (requireRole, el mismo gate que da
//     acceso a la Console) puede crear una org; queda como owner de la nueva.
//   - EDITAR AJUSTES: solo un owner/admin DE ESA org concreta. La pertenencia se
//     re-verifica contra la BD por-org (RolesForUserInOrg), no basta el claim de
//     rol de la org activa: así un admin de la org A NO puede tocar la org B
//     (anti-IDOR cross-tenant, §0.7). Una org ajena/desconocida ⇒ 404 (no revela
//     su existencia), coherente con el aislamiento de admin_users.
//
// El slug es la identidad inmutable del tenant (namespace/almacenamiento/grupo
// derivan de él en toda la suite); los ajustes editables son nombre y estado.
package http

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// tenantWire es la forma JSON de una organización para la Console. namespace se
// deriva del slug con la MISMA convención de tenancy que el resto de la suite.
func tenantWire(id, slug, name string, status domain.Status, userCount int, createdAt time.Time, role domain.Role) map[string]any {
	return map[string]any{
		"id":         id,
		"slug":       slug,
		"name":       name,
		"status":     string(status),
		"user_count": userCount,
		"namespace":  tenancy.Tenant{Slug: slug}.Namespace(),
		"created_at": createdAt.UTC().Format(time.RFC3339),
		// Rol del SOLICITANTE en esta org, no el de otro: el claim `roles` del token
		// habla solo de la org activa, así que sin esto Console no puede saber si en
		// una segunda organización es owner o admin — y esa diferencia decide si
		// puede ofrecer el rol owner al añadir a alguien (POST /orgs/{slug}/members).
		"role": string(role),
	}
}

// handleAdminListTenants — GET /api/v1/admin/tenants — página keyset de las orgs
// que el solicitante gestiona (owner/admin), (created_at,id) DESC. Query: cursor
// (token opaco de la página anterior) y limit (1..200, 50 por defecto). Responde
// {items, nextCursor} con nextCursor="" al agotar.
func (s *Server) handleAdminListTenants(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid limit (1..200)."))
			return
		}
		limit = n
	}
	after, err := parseOrgCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid cursor."))
		return
	}
	page, more, err := s.store.ListManagedOrgsPage(r.Context(), c.Subject, after, limit)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	next := ""
	if more && len(page) > 0 {
		last := page[len(page)-1]
		next = formatOrgCursor(last.CreatedAt, last.ID)
	}
	items := make([]map[string]any, 0, len(page))
	for _, o := range page {
		items = append(items, tenantWire(o.ID, o.Slug, o.Name, o.Status, o.UserCount, o.CreatedAt, o.Role))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}

type createTenantRequest struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// handleAdminCreateTenant — POST /api/v1/admin/tenants — crea una org y hace owner
// al solicitante (creador ⇒ owner). requireRole ya exige owner/admin. slug único
// (constraint) ⇒ 409 si colisiona.
func (s *Server) handleAdminCreateTenant(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	var req createTenantRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	req.Name = strings.TrimSpace(req.Name)
	if !tenancy.ValidSlug(req.Slug) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid slug (lowercase letters, digits and hyphens)."))
		return
	}
	// La reserva no la burla quien la crea desde Console: hosts de la suite y
	// nombres de página de las apps (ADR 0056), igual que en el alta guiada.
	if !domain.SlugAllowedForOrg(req.Slug) {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("That identifier is reserved. Choose another one."))
		return
	}
	if req.Name == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("A name is required."))
		return
	}
	var org domain.Organization
	ctx := r.Context()
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		created, e := s.store.CreateOrg(ctx, q, domain.Organization{Slug: req.Slug, Name: req.Name})
		if e != nil {
			return e
		}
		org = created
		return s.store.UpsertMembership(ctx, q, domain.Membership{
			OrgID: org.ID, UserID: c.Subject, Role: domain.RoleOwner,
		})
	})
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.created", org.Slug, r))
	// El creador es el único miembro ⇒ user_count = 1.
	// El creador queda como owner (lo hace el propio alta), así que ese es su rol.
	httpx.JSON(w, r, http.StatusCreated, tenantWire(org.ID, org.Slug, org.Name, org.Status, 1, org.CreatedAt, domain.RoleOwner))
}

type updateTenantRequest struct {
	Name   *string `json:"name"`
	Status *string `json:"status"`
}

// handleAdminUpdateTenant — PATCH /api/v1/admin/tenants/{slug} — edita los ajustes
// (nombre, estado) de una org que el solicitante gestiona. Autorización POR-ORG
// (fail-closed, anti-IDOR): el solicitante DEBE ser owner/admin de ESTA org; una
// org ajena o inexistente ⇒ 404 (no revela su existencia). El slug es inmutable.
func (s *Server) handleAdminUpdateTenant(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	org, err := s.store.GetOrgBySlug(r.Context(), slug)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err)) // ErrNotFound ⇒ 404
		return
	}
	// La org SIEMPRE se resuelve por slug del path, pero la AUTORIZACIÓN se decide
	// contra la BD por-org: el claim de rol solo vale para la org activa, no para una
	// arbitraria. Sin membresía owner/admin en ESTA org ⇒ 404 (aislamiento tenant).
	// Una cuenta personal tampoco se gestiona desde aquí, aunque sea suya: suspenderla
	// sería dejarse fuera de su propio correo (ADR 0038). Fuera de la lista, mismo 404.
	callerRoles, rerr := s.store.RolesForUserInOrg(r.Context(), org.ID, c.Subject)
	if rerr != nil || !hasAnyRole(callerRoles, domain.RoleOwner, domain.RoleAdmin) || org.Personal() {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var req updateTenantRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name := org.Name
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The name cannot be left empty."))
			return
		}
		name = *req.Name
	}
	status := org.Status
	if req.Status != nil {
		st := domain.Status(strings.TrimSpace(*req.Status))
		// Solo el ciclo activo⇄suspendido por esta vía (deleted es otro flujo).
		if st != domain.StatusActive && st != domain.StatusSuspended {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid status (active|suspended)."))
			return
		}
		status = st
	}
	updated, err := s.store.UpdateOrgSettings(r.Context(), nil, org.ID, name, status)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.settings_updated", org.Slug, r))
	// user_count no cambia con un ajuste; se recalcularía al recargar la lista.
	// callerRoles no puede estar vacío: hasAnyRole acaba de encontrar owner o admin.
	httpx.JSON(w, r, http.StatusOK, tenantWire(updated.ID, updated.Slug, updated.Name, updated.Status, 0, updated.CreatedAt, domain.Role(callerRoles[0])))
}

// formatOrgCursor / parseOrgCursor codifican la posición keyset (created_at, id)
// de la última org vista como token opaco URL-safe. Espeja formatUserCursor.
func formatOrgCursor(ts time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(ts.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func parseOrgCursor(token string) (*store.OrgCursor, error) {
	if token == "" {
		return nil, nil //nolint:nilnil // (nil, nil) es el contrato: token vacío ⇒ primera página.
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	tsRaw, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, errors.New("cursor sin separador")
	}
	ts, err := time.Parse(time.RFC3339Nano, tsRaw)
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, err
	}
	return &store.OrgCursor{CreatedAt: ts, ID: id}, nil
}
