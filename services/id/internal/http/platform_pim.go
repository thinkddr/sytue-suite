// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// PIM del alcance de PLATAFORMA (mig 0043): la elevación global deja de ser
// permanente. Estar en `platform_admins` pasa a ser ELEGIBILIDAD —quién puede
// elevarse— y la autorización real la da una ACTIVACIÓN viva, con motivo y
// ventana, igual que lleva haciendo el PIM de organización desde la 0020.
//
// ESTAS RUTAS NO VAN DETRÁS DE `requirePlatformAdmin`, y es lo primero que hay
// que entender de este fichero: ese gate exige una activación viva, así que
// pedirla desde detrás de él sería imposible por construcción —nadie podría
// elevarse nunca—. Van bajo `requireAccessToken` y comprueban aquí mismo las dos
// condiciones que sí se pueden comprobar sin estar elevado: estar en la
// allowlist y haber entrado con segundo factor.
package http

import (
	"net/http"
	"strconv"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// Ventana por defecto y tope. El tope lo repite el CHECK de la tabla: aquí para
// poder decir por qué se rechaza, allí para que nadie lo salte por otra vía.
const (
	platformActivationDefaultMin = 60
	platformActivationMaxMin     = 480
	platformActivationMinMin     = 5
)

// activationJSON es la forma de una activación de plataforma en el cable.
// `effective_status` aplica la caducidad perezosa y `remaining_seconds` evita
// que el cliente tenga que restar fechas para saber cuánto le queda.
func platformActivationJSON(a store.PlatformActivation, now time.Time) map[string]any {
	out := map[string]any{
		"id":               a.ID,
		"user_id":          a.UserID,
		"status":           a.Status,
		"effective_status": a.EffectiveStatus(now),
		"justification":    a.Justification,
		"duration_minutes": a.DurationMinutes,
		"activated_at":     a.ActivatedAt.UTC().Format(time.RFC3339),
		"expires_at":       a.ExpiresAt.UTC().Format(time.RFC3339),
	}
	if a.UserEmail != "" {
		out["user_email"] = a.UserEmail
		out["user_name"] = a.UserName
	}
	if a.RevokedAt != nil {
		out["revoked_at"] = a.RevokedAt.UTC().Format(time.RFC3339)
	}
	if resto := time.Until(a.ExpiresAt); a.EffectiveStatus(now) == "active" && resto > 0 {
		out["remaining_seconds"] = int(resto.Seconds())
	}
	return out
}

// elegibleParaPlataforma comprueba lo que se puede comprobar SIN estar elevado:
// que el usuario está en la allowlist y que su sesión llevó segundo factor.
// Devuelve el sujeto si pasa; si no, ya ha respondido.
func (s *Server) elegibleParaPlataforma(w http.ResponseWriter, r *http.Request) (string, bool) {
	c, ok := claimsFrom(r.Context())
	if !ok || c.Subject == "" {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return "", false
	}
	esAdmin, err := s.store.IsPlatformAdmin(r.Context(), c.Subject)
	if err != nil {
		// Fail-closed explícito: un fallo de la base no eleva a nadie.
		s.log.Error("platform pim: no se pudo comprobar el alcance", "user_id", c.Subject, "error", err)
		httpx.Fail(w, r, httpx.ErrForbidden)
		return "", false
	}
	if !esAdmin {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return "", false
	}
	if !hasMFA(c.AMR) {
		httpx.Fail(w, r, errPlatformMFA)
		return "", false
	}
	return c.Subject, true
}

// handlePlatformActivationGet — GET /api/v1/platform/activation — la activación
// viva del que pregunta, o `null`. Es lo que la consola necesita para saber si
// enseñar «elevarse» o «te quedan 34 minutos».
func (s *Server) handlePlatformActivationGet(w http.ResponseWriter, r *http.Request) {
	sub, ok := s.elegibleParaPlataforma(w, r)
	if !ok {
		return
	}
	now := time.Now()
	a, existe, err := s.store.CurrentPlatformActivation(r.Context(), sub, now)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if !existe {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"activation": nil})
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"activation": platformActivationJSON(a, now)})
}

type platformActivateRequest struct {
	Justification   string `json:"justification"`
	DurationMinutes int    `json:"duration_minutes"`
}

// handlePlatformActivate — POST /api/v1/platform/activate — eleva al que llama
// por una ventana acotada.
//
// La justificación es OBLIGATORIA y ese es medio valor de todo esto: sin ella el
// registro diría que alguien se elevó, que es justo lo que ya se sabía. El otro
// medio es que caduque sola.
func (s *Server) handlePlatformActivate(w http.ResponseWriter, r *http.Request) {
	sub, ok := s.elegibleParaPlataforma(w, r)
	if !ok {
		return
	}
	var req platformActivateRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	motivo := trim(req.Justification)
	if motivo == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"State what you need the platform scope for: it goes on the record.",
		))
		return
	}
	dur := req.DurationMinutes
	if dur == 0 {
		dur = platformActivationDefaultMin
	}
	if dur < platformActivationMinMin || dur > platformActivationMaxMin {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"The window ranges from 5 minutes to 8 hours.",
		))
		return
	}

	now := time.Now()
	a, err := s.store.CreatePlatformActivation(r.Context(), nil, sub, motivo, dur, now)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// El motivo entra en la auditoría: es la pregunta que se hace después.
	s.auditPlatform(r, "platform.pim.activated", "", motivo)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"activation": platformActivationJSON(a, now)})
}

// handlePlatformDeactivate — POST /api/v1/platform/deactivate — baja la propia
// elevación antes de que caduque. Bajarla cuando ya no había ninguna NO es un
// error: el resultado que se pedía —no estar elevado— es el que hay.
func (s *Server) handlePlatformDeactivate(w http.ResponseWriter, r *http.Request) {
	sub, ok := s.elegibleParaPlataforma(w, r)
	if !ok {
		return
	}
	habia, err := s.store.RevokePlatformActivation(r.Context(), nil, sub, time.Now())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if habia {
		s.auditPlatform(r, "platform.pim.deactivated", "", "")
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"deactivated": habia})
}

// handlePlatformActivations — GET /platform/activations — el histórico de
// elevaciones. Este SÍ va detrás del gate: es información de plataforma, y para
// leerla hay que estar elevado como para todo lo demás.
func (s *Server) handlePlatformActivations(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	lista, err := s.store.ListPlatformActivations(r.Context(), limit)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	items := make([]map[string]any, 0, len(lista))
	for _, a := range lista {
		items = append(items, platformActivationJSON(a, now))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// handlePlatformAudit — GET /platform/audit — lo que han hecho los admin
// globales, de todas las organizaciones a la vez. Mismos filtros y mismo cursor
// que el registro de una organización (action, actor, since/until, cursor,
// limit); lo que cambia es el alcance y que cada fila dice sobre QUÉ
// organización se actuó.
//
// El registro se escribía desde el principio y no se podía leer desde ninguna
// parte: un registro que nadie mira es un registro que no existe hasta el día
// en que hace falta.
func (s *Server) handlePlatformAudit(w http.ResponseWriter, r *http.Request) {
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
	page, more, err := s.store.ListPlatformAuditPage(r.Context(), filter, after, limit)
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
		item := auditItemJSON(a)
		item["org_slug"] = a.OrgSlug
		items = append(items, item)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}
