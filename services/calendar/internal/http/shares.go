// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
)

// Compartir calendarios con ACL. La gestión (compartir/listar/revocar) es SOLO del
// dueño: el servicio verifica owner_sub == sub del JWT y, si no, devuelve
// ErrNotFound (404, sin filtrar la existencia). El acceso a lo compartido pasa por
// la verja CalendarAccess (dueño|share; sin acceso → 404). Un reader que intenta
// escribir recibe ErrReadOnly → 403, como una suscripción ICS. Mutaciones por POST/
// DELETE (el BFF de Astro las emite tras validar el origen → CSRF-safe).

// parseUUIDParam extrae y valida un UUID de un parámetro de ruta chi por nombre.
func parseUUIDParam(r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// sharedCalendarDTO aplana un calendario compartido CON el usuario: los campos del
// calendario (incluye owner_sub y name, que el BFF usa para la URL CalDAV) más el
// permiso concedido y quién lo compartió, para «Compartido conmigo».
type sharedCalendarDTO struct {
	domain.Calendar
	Permission domain.SharePermission `json:"permission"`
	SharedBy   string                 `json:"shared_by"`
	// EditRequested: siendo lector, ya ha pedido editarlo (la franja lo dice).
	EditRequested bool `json:"edit_requested,omitempty"`
}

// SharedWithMe lista los calendarios compartidos CON el usuario autenticado (por su
// sub o su correo del JWT verificado), con dueño + permiso. Deny-by-default: sin
// shares, lista vacía (nunca null).
func (h *Handlers) SharedWithMe(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	shared, err := h.svc.SharedWithMe(r.Context(), sub, userEmail(r))
	if err != nil {
		fail(w, r, err)
		return
	}
	out := make([]sharedCalendarDTO, 0, len(shared))
	for _, sc := range shared {
		out = append(out, sharedCalendarDTO{
			Calendar:      sc.Calendar,
			Permission:    sc.Permission,
			SharedBy:      sc.SharedBy,
			EditRequested: sc.EditRequested,
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"calendars": out})
}

type createShareReq struct {
	Email      string `json:"email"`
	Permission string `json:"permission"` // reader | writer
}

// CreateShare comparte el calendario {id} (solo el dueño) con un destinatario por
// correo y permiso. Mutación por POST (CSRF-safe). El grantee se casa luego contra
// el email/sub del JWT del que accede.
func (h *Handlers) CreateShare(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	calID, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid calendar id."))
		return
	}
	var req createShareReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sh, err := h.svc.ShareCalendar(r.Context(), calID, sub, userEmail(r), req.Email, domain.SharePermission(req.Permission))
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, sh)
}

// ListShares lista con quién está compartido el calendario {id} (solo el dueño).
func (h *Handlers) ListShares(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	calID, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid calendar id."))
		return
	}
	shares, err := h.svc.ListCalendarShares(r.Context(), calID, sub)
	if err != nil {
		fail(w, r, err)
		return
	}
	if shares == nil {
		shares = []domain.CalendarShare{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"shares": shares})
}

// RevokeShare revoca el share {shareId} del calendario {id} (solo el dueño): corta
// el acceso del destinatario.
func (h *Handlers) RevokeShare(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	calID, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid calendar id."))
		return
	}
	shareID, ok := parseUUIDParam(r, "shareId")
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid share id."))
		return
	}
	if err := h.svc.RevokeCalendarShare(r.Context(), calID, shareID, sub); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// Asegura en compilación que *service.Service satisface calendarService con los
// métodos de compartición (los handlers se ejercitan con un fake).
var _ calendarService = (*service.Service)(nil)
