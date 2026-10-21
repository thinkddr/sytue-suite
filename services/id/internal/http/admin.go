// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

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
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// handleMe devuelve el perfil del usuario autenticado y sus membresías.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	user, err := s.store.GetUserByID(r.Context(), c.Subject)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	// Con el nombre y el slug de cada organización, no solo su id: es lo que
	// necesita el selector de organización para pintar una lista donde elegir, y
	// resolverlos uno a uno desde el cliente sería un N+1 por pantalla.
	mems, _ := s.store.ListOrgMembershipsForUser(r.Context(), user.ID)
	orgs := make([]map[string]any, 0, len(mems))
	for _, m := range mems {
		kind := m.Kind
		if kind == "" {
			kind = domain.OrgKindOrg
		}
		orgs = append(orgs, map[string]any{
			"org_id": m.OrgID, "role": m.Role,
			"slug": m.Slug, "name": m.Name, "status": m.Status, "kind": kind,
		})
	}
	out := meProfileWire(user)
	out["memberships"] = orgs
	// El slug de la organización ACTIVA (el claim `tenant` de este token), para
	// que el selector sepa cuál marcar sin adivinarlo por el orden.
	out["active_org"] = c.Tenant
	setAvatarFields(r.Context(), s, user.ID, out)
	httpx.JSON(w, r, http.StatusOK, out)
}

type addMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// handleAddMember añade un usuario existente a una organización con un rol.
func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	var req addMemberRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	role := domain.Role(req.Role)
	if !role.Valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid role (owner|admin|member|billing)."))
		return
	}
	org, err := s.store.GetOrgBySlug(r.Context(), slug)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	// Autorización POR-ORG (no basta el claim global de rol): el solicitante debe
	// ser owner/admin DE ESTA org concreta. El claim `roles` del token solo refleja
	// la org activa, así que sin esta comprobación un owner de otra org podría
	// mutar la membresía de una org ajena (IDOR / escalada cross-tenant, §0.7).
	callerRoles, err := s.store.RolesForUserInOrg(r.Context(), org.ID, c.Subject)
	if err != nil || !hasAnyRole(callerRoles, domain.RoleOwner, domain.RoleAdmin) {
		// Fail-closed: si no se puede determinar la pertenencia, denegar.
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Not authorised in this organisation."))
		return
	}
	// Un admin no puede conceder un rol igual o superior al de owner (solo un owner
	// puede crear/promover owners): evita la auto-escalada de privilegios.
	if role == domain.RoleOwner && !hasAnyRole(callerRoles, domain.RoleOwner) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Only an owner can assign the owner role."))
		return
	}
	// requireOrgKind solo mira la organización ACTIVA. Quien administra una empresa es
	// también owner de su cuenta personal, y sin esto podía meter a otros en ella: una
	// cuenta personal es de una persona (ADR 0038).
	if org.Personal() {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Not available for personal accounts."))
		return
	}
	user, err := s.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if err := s.store.UpsertMembership(r.Context(), nil, domain.Membership{
		OrgID: org.ID, UserID: user.ID, Role: role,
	}); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.member.added", user.ID, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"org": org.Slug, "user_id": user.ID, "role": role})
}

// handleAdminListUsers — GET /api/v1/admin/users — lista los usuarios de la
// organización activa del solicitante (claim tenant; los roles del token están
// emitidos para ESA org, así que requireRole ya autoriza el ámbito) con
// paginación keyset (created_at, id) DESC: sin OFFSET, estable a cientos de
// miles de usuarios. Query: cursor (token opaco de la página anterior; vacío =
// primera página), limit (1..200, 50 por defecto) y q (subcadena de correo o
// nombre). Responde {items, nextCursor} con nextCursor="" al agotar.
//
// `q` es del SERVIDOR: buscar sobre lo ya paginado responde «no hay nadie» a
// quien está en la página doce, y el buscador queda inútil justo en las
// organizaciones donde hace falta.
//
// El listado SCIM (/scim/v2/Users, startIndex/count RFC 7644) es una superficie
// distinta e intocable; este endpoint no comparte su método de store.
// Acepta GET y QUERY. Aquí `q` son nombres y correos de personas concretas que
// alguien escribe buscándolas; en la URL van a parar al log de accesos de cada
// salto y al `Referer`, y en el cuerpo no. GET se queda como respaldo.
func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	if c.Tenant == "" {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("The token has no active organisation."))
		return
	}
	org, err := s.store.GetOrgBySlug(r.Context(), c.Tenant)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	s.responderUsuariosDeOrg(w, r, org.ID)
}

// responderUsuariosDeOrg pinta una página del directorio de una organización.
// Quién la pide y de qué organización lo decide cada llamante: la consola de la
// organización la saca del token, la de plataforma de la ruta.
func (s *Server) responderUsuariosDeOrg(w http.ResponseWriter, r *http.Request, orgID string) {
	// GET o QUERY, indistinto: `Consulta` lee de donde toque.
	params, err := httpx.Consulta(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.AnunciarQuery(w)
	limit := 50
	if v := params.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid limit (1..200)."))
			return
		}
		limit = n
	}
	after, err := parseUserCursor(params.Get("cursor"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid cursor."))
		return
	}
	// `q` busca por subcadena de correo o nombre. Se acota la longitud: un
	// término más largo que un correo no encuentra nada que uno corto no
	// encuentre, y sí hace trabajar al índice de trigramas de balde.
	q := strings.TrimSpace(params.Get("q"))
	if len(q) > 128 {
		q = q[:128]
	}
	page, more, err := s.store.ListOrgUsersPage(r.Context(), orgID, q, after, limit)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	next := ""
	if more && len(page) > 0 {
		last := page[len(page)-1]
		next = formatUserCursor(last.CreatedAt, last.ID)
	}
	items := make([]map[string]any, 0, len(page))
	for _, u := range page {
		items = append(items, map[string]any{
			"id":             u.ID,
			"email":          u.Email,
			"email_verified": u.EmailVerified,
			"name":           u.Name,
			"mfa_enabled":    u.MFAEnabled,
			"status":         u.Status,
			"role":           u.Role,
			"created_at":     u.CreatedAt.UTC().Format(time.RFC3339Nano),
			// Los tres atributos que evalúan las reglas de grupo dinámico. Van en
			// el listado y no en un endpoint aparte para que la ficha los pueda
			// enseñar sin una segunda petición por usuario.
			"department": u.Department,
			"title":      u.Title,
			"location":   u.Location,
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}

// formatUserCursor codifica la posición keyset (created_at, id) del último
// usuario visto como token opaco URL-safe: base64url("<RFC3339Nano>|<uuid>").
// El formato es interno; el cliente lo devuelve tal cual en `cursor`.
func formatUserCursor(ts time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(ts.UTC().Format(time.RFC3339Nano) + "|" + id))
}

// parseUserCursor invierte formatUserCursor. Token vacío ⇒ primera página (nil).
// Un token corrupto es entrada inválida del cliente (400), nunca un 500: el id
// se valida como UUID aquí para que el cast ::uuid del store no pueda fallar.
func parseUserCursor(token string) (*store.UserCursor, error) {
	if token == "" {
		return nil, nil //nolint:nilnil // (nil, nil) es el contrato documentado: token vacío ⇒ primera página, no un error.
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
	return &store.UserCursor{CreatedAt: ts, ID: id}, nil
}

// hasAnyRole indica si el conjunto de roles (strings del store) contiene alguno
// de los roles de dominio requeridos.
func hasAnyRole(have []string, want ...domain.Role) bool {
	for _, h := range have {
		for _, w := range want {
			if h == string(w) {
				return true
			}
		}
	}
	return false
}

// mapDomainErr traduce errores de dominio a errores HTTP canónicos.
func mapDomainErr(err error) error {
	// Un error que YA viene resuelto como respuesta HTTP se devuelve tal cual. Sin
	// esto caía en el `default` y salía convertido en 500: `aplicarEstadoOrg`
	// devuelve tres —el 403 de la organización que opera la plataforma, el 409 de
	// una fusionada y su 404—, cada uno con su explicación escrita, y el usuario
	// recibía «Internal server error» en los tres. La verja funcionaba (el estado
	// no cambiaba); lo que se perdía era el motivo, que es justo lo que hace falta
	// para no volver a intentarlo.
	var yaHTTP *httpx.Error
	if errors.As(err, &yaHTTP) {
		return err
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrNotFound
	case errors.Is(err, domain.ErrConflict):
		return httpx.ErrConflict
	case errors.Is(err, domain.ErrInvalidInput):
		return httpx.ErrBadRequest
	case errors.Is(err, domain.ErrAccessDenied):
		return httpx.ErrForbidden
	default:
		return httpx.ErrInternal.Wrap(err)
	}
}
