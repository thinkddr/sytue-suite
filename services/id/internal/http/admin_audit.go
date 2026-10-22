// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Lectura admin del log de auditoría para Sytue Admin. Dos superficies, ambas
// acotadas a la organización activa del token (c.Tenant): un listado paginado
// por keyset (occurred_at, id) DESC — la tabla es append-only y solo crece — con
// filtros por acción/actor/rango de fechas, y una exportación CSV en streaming
// del mismo conjunto filtrado. No comparte método de store con SCIM (RFC 7644).
package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// activeOrg resuelve la organización activa del token (claim tenant) a su fila.
// Devuelve un *httpx.Error listo para Fail si no hay tenant activo o no existe.
// Es el mismo anclaje de aislamiento que handleAdminListUsers: todas las
// consultas admin se acotan a org.ID, nunca a un id/slug de la URL.
//
// FAIL-CLOSED: sin tenant, o si el tenant no resuelve, se devuelve error y no se
// consulta nada. `requireRole` ya garantizó que los roles del token son de ESA
// organización, así que resolver aquí y acotar por `org.ID` cierra el círculo.
//
// Hubo un segundo `resolveActiveOrg` idéntico byte a byte en admin_sso.go, con
// 27 llamadas frente a las 44 de este. Se unificaron porque son LA primitiva de
// aislamiento entre organizaciones: con dos copias, quien arregle un fallo de
// aislamiento en una puede dejar la otra intacta y no habrá nada que lo delate.
func (s *Server) activeOrg(r *http.Request) (domain.Organization, error) {
	c, _ := claimsFrom(r.Context())
	if c.Tenant == "" {
		return domain.Organization{}, httpx.ErrForbidden.WithMessage("The token has no active organisation.")
	}
	org, err := s.store.GetOrgBySlug(r.Context(), c.Tenant)
	if err != nil {
		return domain.Organization{}, mapDomainErr(err)
	}
	return org, nil
}

// handleAdminListAudit — GET /api/v1/admin/audit — página del log de la org
// activa. Query: action (prefijo), actor (uuid), since/until (RFC3339 o fecha),
// cursor (opaco), limit (1..500, 100 por defecto). Responde {items, nextCursor}.
func (s *Server) handleAdminListAudit(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	filter, err := parseAuditFilter(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid limit (1..500)."))
			return
		}
		limit = n
	}
	after, err := parseAuditCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid cursor."))
		return
	}
	page, more, err := s.store.ListAuditPage(r.Context(), org.ID, filter, after, limit)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	next := ""
	if more && len(page) > 0 {
		last := page[len(page)-1]
		next = formatAuditCursor(last.OccurredAt, last.ID)
	}
	items := make([]map[string]any, 0, len(page))
	for _, a := range page {
		items = append(items, auditItemJSON(a))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}

// handleAdminExportAudit — GET /api/v1/admin/audit/export.csv — descarga CSV del
// log filtrado de la org activa (mismos filtros que el listado; sin cursor: es
// el conjunto completo, acotado por el tope del store). Streaming fila a fila.
func (s *Server) handleAdminExportAudit(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	filter, err := parseAuditFilter(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Cabeceras antes del primer byte del cuerpo: una vez empieza el stream ya no
	// se puede cambiar el status ni emitir un error JSON limpio.
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="audit-`+org.Slug+`-`+time.Now().UTC().Format("20060102")+`.csv"`)
	if err := s.store.ExportAudit(r.Context(), org.ID, filter, w); err != nil {
		// El cuerpo puede estar a medias; registrar y cerrar. No hay forma limpia
		// de reescribir el status a estas alturas.
		s.log.ErrorContext(r.Context(), "export de auditoría falló", "error", err, "org", org.Slug)
		return
	}
}

// auditItemJSON serializa una fila de auditoría para el listado JSON de Console.
func auditItemJSON(a store.AuditRow) map[string]any {
	detail := any(nil)
	if len(a.Detail) > 0 {
		detail = json.RawMessage(a.Detail) // se emite tal cual, sin re-escapar
	}
	return map[string]any{
		"id":          strconv.FormatInt(a.ID, 10),
		"occurred_at": a.OccurredAt.UTC().Format(time.RFC3339Nano),
		"actor_id":    a.ActorID,
		"actor_email": a.ActorEmail,
		"action":      a.Action,
		"target":      a.Target,
		"ip":          a.IP,
		"detail":      detail,
	}
}

// parseAuditFilter extrae y valida los filtros de la query. Rechaza un actor no
// uuid, fechas no parseables y rangos vacíos/invertidos (400) para no propagar
// basura al store ni devolver una página engañosamente vacía.
func parseAuditFilter(r *http.Request) (store.AuditFilter, error) {
	q := r.URL.Query()
	var f store.AuditFilter
	f.Action = strings.TrimSpace(q.Get("action"))
	if len(f.Action) > 100 {
		return f, httpx.ErrBadRequest.WithMessage("action is too long.")
	}
	if actor := strings.TrimSpace(q.Get("actor")); actor != "" {
		if !uuidRe.MatchString(actor) {
			return f, httpx.ErrBadRequest.WithMessage("Invalid actor (uuid).")
		}
		f.ActorID = actor
	}
	since, err := parseAuditBound(q.Get("since"), false)
	if err != nil {
		return f, httpx.ErrBadRequest.WithMessage("Invalid since (RFC3339 or YYYY-MM-DD).")
	}
	until, err := parseAuditBound(q.Get("until"), true)
	if err != nil {
		return f, httpx.ErrBadRequest.WithMessage("Invalid until (RFC3339 or YYYY-MM-DD).")
	}
	if since != nil && until != nil && !since.Before(*until) {
		return f, httpx.ErrBadRequest.WithMessage("The date range is empty (since >= until).")
	}
	f.Since, f.Until = since, until
	return f, nil
}

// parseAuditBound acepta RFC3339 o una fecha AAAA-MM-DD (UTC). Para el límite
// superior (endOfDay) una fecha se interpreta inclusiva: avanza al día siguiente
// (el store usa occurred_at < Until), de modo que "hasta el 10" incluye el día 10.
func parseAuditBound(v string, endOfDay bool) (*time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil //nolint:nilnil // (nil, nil) es el contrato documentado: sin valor ⇒ límite ausente, no un error.
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t, nil
	}
	if d, err := time.Parse("2006-01-02", v); err == nil {
		if endOfDay {
			d = d.AddDate(0, 0, 1)
		}
		return &d, nil
	}
	return nil, errors.New("fecha no parseable")
}

// formatAuditCursor codifica la posición keyset (occurred_at, id) como token
// opaco URL-safe: base64url("<RFC3339Nano>|<id>"). Mismo esquema que el cursor
// de usuarios, con id bigint en vez de uuid.
func formatAuditCursor(ts time.Time, id int64) string {
	return base64.RawURLEncoding.EncodeToString(
		[]byte(ts.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(id, 10)),
	)
}

// parseAuditCursor invierte formatAuditCursor. Token vacío ⇒ primera página
// (nil). Un token corrupto es entrada inválida del cliente (400), nunca un 500.
func parseAuditCursor(token string) (*store.AuditCursor, error) {
	if token == "" {
		return nil, nil //nolint:nilnil // (nil, nil) es el contrato documentado: token vacío ⇒ primera página, no un error.
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	tsRaw, idRaw, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, errors.New("cursor sin separador")
	}
	ts, err := time.Parse(time.RFC3339Nano, tsRaw)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil {
		return nil, err
	}
	return &store.AuditCursor{OccurredAt: ts, ID: id}, nil
}
