// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Búsqueda en los logs de runtime de un proyecto (WS4/O1). El tail en vivo por
// SSE (streams.go) ya existía; lo que faltaba era poder MIRAR ATRÁS: buscar un
// texto acotando por tiempo sobre lo que el recolector fue guardando.
//
// Misma verja que el tail en vivo (ver server.go): solo humanos con member+, no
// tokens de API — un log de producción contiene secretos y PII.
package cloudhttp

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// SearchLogs — GET /projects/{id}/logs — página de logs de runtime del proyecto
// (?q=&since=&until=&cursor=&limit=), del más reciente al más antiguo. Responde
// {lines, next_cursor}; un cursor vacío o ausente pide la primera página.
func (h *Handlers) SearchLogs(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, err := parseLogFilter(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// ?format=txt descarga el filtro ENTERO como fichero, sin cursor: la página es
	// de la pantalla, no del fichero. Mismo criterio que el CSV de auditoría.
	if r.URL.Query().Get("format") == "txt" {
		h.exportLogsTxt(w, r, id, f)
		return
	}
	page, err := h.svc.SearchLogs(r.Context(), id, f, queryCursor(r), queryLimit(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	lines := page.Lines
	if lines == nil {
		lines = []domain.LogLine{}
	}
	resp := map[string]any{"lines": lines}
	if page.NextCursor != "" {
		resp["next_cursor"] = page.NextCursor
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

// exportLogsTxt descarga el registro filtrado como texto plano.
//
// Las cabeceras van ANTES del primer byte: en cuanto el store empieza a escribir
// ya no se puede cambiar el status, así que un fallo a media descarga solo se
// registra —el fichero queda corto, y eso es visible— en vez de intentar un 500
// que el cliente ya no vería.
func (h *Handlers) exportLogsTxt(w http.ResponseWriter, r *http.Request, projectID uuid.UUID, f store.RuntimeLogFilter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="logs-`+projectID.String()+`-`+time.Now().UTC().Format("20060102")+`.log"`)
	if err := h.svc.ExportLogs(r.Context(), projectID, f, w); err != nil {
		log.From(r.Context()).Error("export de logs falló", "error", err)
	}
}

// parseLogFilter extrae y valida los filtros de la query. Un rango inválido es
// 400 y no "todo", por el mismo motivo que en la auditoría: devolver el log
// entero a quien pidió una hora concreta le oculta que se equivocó al escribirla.
// Se reutiliza parseAuditBound (RFC3339 o AAAA-MM-DD, superior inclusivo del día).
func parseLogFilter(r *http.Request) (store.RuntimeLogFilter, error) {
	q := r.URL.Query()
	var f store.RuntimeLogFilter
	f.Query = strings.TrimSpace(q.Get("q"))
	if len(f.Query) > maxAuditFilterLen {
		return f, httpx.ErrBadRequest.WithMessage("The search is too long.")
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
