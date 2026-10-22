// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Superficie admin de GOBERNANZA L5 del Sytue Integration Gateway (owner/admin de
// org): visor + export CSV del registro de egress, CRUD de reglas de política y
// visor de decisiones (block/warn). Todo acotado a la org activa del token por RLS.
// Tras un cambio de regla se invalida la caché del motor para que surta efecto ya.
package integrationshttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

const maxConditionBytes = 8 << 10 // una condición es un objeto pequeño de metadatos.

// --- Registro de egress (egress_audit) ----------------------------------------

// ListEgressAudit — GET /api/v1/admin/egress-audit — página del registro de egress
// de la org activa (keyset), o export CSV con ?format=csv. Filtros: host (prefijo),
// status (exacto), since/until (RFC3339 o AAAA-MM-DD).
func (h *Handlers) ListEgressAudit(w http.ResponseWriter, r *http.Request) {
	filter, err := parseEgressFilter(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if strings.EqualFold(r.URL.Query().Get("format"), "csv") {
		h.exportEgressCSV(w, r, filter)
		return
	}
	after, err := store.DecodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid cursor."))
		return
	}
	rows, more, err := h.store.ListEgressPage(r.Context(), filter, after, parseLimit(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, e := range rows {
		items = append(items, map[string]any{
			"id":          strconv.FormatInt(e.ID, 10),
			"occurred_at": e.OccurredAt.UTC().Format(time.RFC3339Nano),
			"instance_id": e.InstanceID,
			"host":        e.Host,
			"method":      e.Method,
			"status":      e.Status,
			"req_bytes":   e.ReqBytes,
			"resp_bytes":  e.RespBytes,
			"latency_ms":  e.LatencyMS,
			"auth":        e.Auth,
			"error":       e.Err,
		})
	}
	next := ""
	if more && len(rows) > 0 {
		last := rows[len(rows)-1]
		next = store.EncodeCursor(last.OccurredAt, last.ID)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}

// exportEgressCSV descarga el registro de egress filtrado como CSV en streaming.
func (h *Handlers) exportEgressCSV(w http.ResponseWriter, r *http.Request, filter store.EgressFilter) {
	slug := ""
	if t, err := tenancy.From(r.Context()); err == nil {
		slug = t.Slug
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="egress-`+slug+`-`+time.Now().UTC().Format("20060102")+`.csv"`)
	if err := h.store.ExportEgress(r.Context(), filter, w); err != nil {
		// El cuerpo puede ir a medias; ya no se puede reescribir el status.
		log.From(r.Context()).Error("export de egress falló", "error", err)
	}
}

// parseEgressFilter extrae y valida los filtros del registro de egress.
func parseEgressFilter(r *http.Request) (store.EgressFilter, error) {
	q := r.URL.Query()
	var f store.EgressFilter
	f.Host = strings.TrimSpace(q.Get("host"))
	if len(f.Host) > 255 {
		return f, httpx.ErrBadRequest.WithMessage("The host is too long.")
	}
	if v := strings.TrimSpace(q.Get("status")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 599 {
			return f, httpx.ErrBadRequest.WithMessage("Invalid status (0..599).")
		}
		f.Status = &n
	}
	since, err := parseTimeBound(q.Get("since"), false)
	if err != nil {
		return f, httpx.ErrBadRequest.WithMessage("Invalid since (RFC3339 or YYYY-MM-DD).")
	}
	until, err := parseTimeBound(q.Get("until"), true)
	if err != nil {
		return f, httpx.ErrBadRequest.WithMessage("Invalid until (RFC3339 or YYYY-MM-DD).")
	}
	if since != nil && until != nil && !since.Before(*until) {
		return f, httpx.ErrBadRequest.WithMessage("The date range is empty (since >= until).")
	}
	f.Since, f.Until = since, until
	return f, nil
}

// --- Decisiones de política (policy_decisions) --------------------------------

// ListPolicyDecisions — GET /api/v1/admin/policy-decisions — página de decisiones
// (block/warn) de la org activa. Filtro opcional action=warn|block.
func (h *Handlers) ListPolicyDecisions(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	if action != "" && action != string(domain.PolicyWarn) && action != string(domain.PolicyBlock) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid action (warn|block)."))
		return
	}
	after, err := store.DecodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid cursor."))
		return
	}
	rows, more, err := h.store.ListDecisionsPage(r.Context(), action, after, parseLimit(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, d := range rows {
		items = append(items, map[string]any{
			"id":          strconv.FormatInt(d.ID, 10),
			"occurred_at": d.OccurredAt.UTC().Format(time.RFC3339Nano),
			"instance_id": d.InstanceID,
			"policy_id":   d.PolicyID,
			"policy_name": d.PolicyName,
			"action":      d.Action,
			"request_id":  d.RequestID,
			"host":        d.Host,
			"method":      d.Method,
		})
	}
	next := ""
	if more && len(rows) > 0 {
		last := rows[len(rows)-1]
		next = store.EncodeCursor(last.OccurredAt, last.ID)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}

// --- CRUD de políticas ---------------------------------------------------------

// ListPolicies — GET /api/v1/admin/policies — reglas de la org activa.
func (h *Handlers) ListPolicies(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListPolicies(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"policies": list})
}

type policyReq struct {
	Name      string          `json:"name"`
	Condition json.RawMessage `json:"condition"`
	Action    string          `json:"action"`
	Enabled   *bool           `json:"enabled"`
}

// CreatePolicy — POST /api/v1/admin/policies — crea una regla e invalida la caché.
func (h *Handlers) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	var req policyReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name, cond, action, enabled, err := validPolicy(req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := h.store.CreatePolicy(r.Context(), name, cond, action, enabled)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	h.invalidatePolicy(r)
	httpx.JSON(w, r, http.StatusCreated, p)
}

// UpdatePolicy — PATCH /api/v1/admin/policies/{id} — REEMPLAZA la regla con el
// estado deseado completo (name/condition/action/enabled) e invalida la caché. El
// cliente siempre envía la regla entera (la tiene en el listado), incluido el
// toggle de activación — evita una lectura previa.
func (h *Handlers) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req policyReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name, cond, action, enabled, err := validPolicy(req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := h.store.UpdatePolicy(r.Context(), id, name, cond, action, enabled)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	h.invalidatePolicy(r)
	httpx.JSON(w, r, http.StatusOK, p)
}

// DeletePolicy — DELETE /api/v1/admin/policies/{id} — borra la regla e invalida la caché.
func (h *Handlers) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.store.DeletePolicy(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	h.invalidatePolicy(r)
	httpx.NoContent(w)
}

// invalidatePolicy descarta la caché de reglas de la org activa en ESTA réplica
// tras un cambio (las demás réplicas convergen al vencer el TTL del motor).
func (h *Handlers) invalidatePolicy(r *http.Request) {
	if h.engine == nil {
		return
	}
	if t, err := tenancy.From(r.Context()); err == nil {
		h.engine.Invalidate(t.Slug)
	}
}

// validPolicy valida y normaliza el cuerpo de una regla. enabled ausente ⇒ true.
func validPolicy(req policyReq) (string, json.RawMessage, domain.PolicyAction, bool, error) {
	name, err := validName(req.Name)
	if err != nil {
		return "", nil, "", false, err
	}
	action := domain.PolicyAction(strings.TrimSpace(req.Action))
	if !action.Valid() {
		return "", nil, "", false, httpx.ErrBadRequest.WithMessage("Invalid action (allow|warn|block).")
	}
	cond, err := validCondition(req.Condition)
	if err != nil {
		return "", nil, "", false, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return name, cond, action, enabled, nil
}

// validCondition exige que la condición sea un objeto JSON PLANO de strings (los
// campos de match son todos textuales) y la devuelve en forma canónica. Vacía ⇒ "{}"
// (comodín: casa todo). Los campos no reconocidos se conservan (el motor los ignora).
func validCondition(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage("{}"), nil
	}
	if len(raw) > maxConditionBytes {
		return nil, httpx.ErrBadRequest.WithMessage("The condition is too large.")
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, httpx.ErrBadRequest.WithMessage("condition must be a JSON object of strings.")
	}
	out, err := json.Marshal(m)
	if err != nil {
		return nil, httpx.ErrBadRequest.WithMessage("Invalid condition.")
	}
	return out, nil
}

// --- helpers compartidos -------------------------------------------------------

// parseLimit lee ?limit= (el store lo acota a 1..500, 100 por defecto).
func parseLimit(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return n
}

// parseTimeBound acepta RFC3339 o una fecha AAAA-MM-DD (UTC). Para el límite
// superior (endOfDay) una fecha se interpreta inclusiva: avanza al día siguiente.
func parseTimeBound(v string, endOfDay bool) (*time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil //nolint:nilnil // contrato: sin valor ⇒ límite ausente, no error.
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
	return nil, errBadTime
}

// errBadTime marca una cota temporal no parseable (se traduce a 400 en el llamante).
var errBadTime = errors.New("cota temporal no parseable")
