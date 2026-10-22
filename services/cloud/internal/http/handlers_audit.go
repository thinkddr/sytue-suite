// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Consulta y export de la traza de auditoría de Sytue Cloud (WS4/O5). La escritura
// (RecordAudit) ya existía desde la ola E/F; lo que faltaba era poder MIRARLA: un
// listado paginado por keyset con filtros por actor/acción/fechas y una descarga
// CSV del MISMO conjunto filtrado, apta para ingesta SIEM.
//
// El montaje exige admin/owner (ver server.go) y el aislamiento por organización lo
// da el store (tenant del contexto: RLS + predicado explícito), no un filtro que
// llegue por la query — un admin de A jamás ve la traza de B.
package cloudhttp

import (
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// maxAuditFilterLen acota el texto de los filtros: son un actor y una acción, no
// un documento. Evita construir patrones LIKE absurdos desde la query.
const maxAuditFilterLen = 200

// ListAudit — GET /audit — página de la auditoría de la organización activa
// (?actor=&action=&since=&until=&cursor=&limit=), o su export CSV con ?format=csv.
// Responde {events, next_cursor}; un cursor vacío o ausente pide la primera página.
func (h *Handlers) ListAudit(w http.ResponseWriter, r *http.Request) {
	f, err := parseAuditFilter(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if strings.EqualFold(r.URL.Query().Get("format"), "csv") {
		h.exportAuditCSV(w, r, f)
		return
	}
	page, err := h.svc.ListAudit(r.Context(), f, queryCursor(r), queryLimit(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	events := page.Events
	if events == nil {
		events = []store.AuditRecord{}
	}
	resp := map[string]any{"events": events}
	if page.NextCursor != "" {
		resp["next_cursor"] = page.NextCursor
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

// exportAuditCSV descarga el conjunto filtrado (SIN cursor: el export es del filtro
// entero, no de la página que se está mirando).
func (h *Handlers) exportAuditCSV(w http.ResponseWriter, r *http.Request, f store.AuditFilter) {
	slug := ""
	if t, err := tenancy.From(r.Context()); err == nil {
		slug = t.Slug
	}
	// Cabeceras ANTES del primer byte: en cuanto el store empiece a escribir ya no
	// se puede cambiar el status, así que un fallo a media descarga solo se registra.
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="audit-`+slug+`-`+time.Now().UTC().Format("20060102")+`.csv"`)
	if err := h.svc.ExportAudit(r.Context(), f, w); err != nil {
		log.From(r.Context()).Error("export de auditoría falló", "error", err)
	}
}

// parseAuditFilter extrae y valida los filtros de la query. Una fecha no parseable
// o un rango invertido son 400: servir "todo" cuando el usuario pidió un rango
// concreto es peor que decirle que se equivocó.
func parseAuditFilter(r *http.Request) (store.AuditFilter, error) {
	q := r.URL.Query()
	var f store.AuditFilter
	f.Actor = strings.TrimSpace(q.Get("actor"))
	f.Action = strings.TrimSpace(q.Get("action"))
	if len(f.Actor) > maxAuditFilterLen || len(f.Action) > maxAuditFilterLen {
		return f, httpx.ErrBadRequest.WithMessage("The filter is too long.")
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

// parseAuditBound acepta RFC3339 o una fecha suelta AAAA-MM-DD (UTC). Para el
// límite superior una fecha se interpreta INCLUSIVA del día (avanza al día
// siguiente, ya que el filtro es occurred_at < Until): "hasta el 10" incluye el 10,
// que es lo que espera quien lo escribe.
func parseAuditBound(v string, endOfDay bool) (*time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil //nolint:nilnil // contrato: sin valor ⇒ límite ausente, no error.
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t, nil
	}
	d, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil, err
	}
	if endOfDay {
		d = d.AddDate(0, 0, 1)
	}
	return &d, nil
}
