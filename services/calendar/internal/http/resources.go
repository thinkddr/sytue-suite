// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// Recursos reservables (salas, equipos) — patrón resource-calendar. Reservar/liberar
// y consultar disponibilidad los puede hacer cualquier usuario autenticado del tenant
// (los recursos son de la organización). Dar de ALTA un recurso exige rol admin/owner;
// EDITAR/BORRAR, ser gestor del recurso (o admin). El anti doble-reserva atómico y el
// aislamiento por tenant viven en el store. Mutaciones por POST/DELETE (el BFF de Astro
// valida el origen → CSRF-safe).

// isTenantAdmin indica si el JWT trae rol owner|admin (provisiona/gestiona recursos).
func isTenantAdmin(r *http.Request) bool {
	claims, _ := authn.FromContext(r.Context())
	return claims.HasAnyRole("owner", "admin")
}

// resourceDTO aplana un recurso con sus periodos ocupados en la ventana consultada
// (libre = busy vacío) para «¿está la sala libre en esta franja?».
type resourceDTO struct {
	domain.Resource
	Busy []domain.FreeBusyPeriod `json:"busy"`
}

// ListResources lista los recursos activos del tenant. Con ?start&end (RFC3339)
// adjunta la disponibilidad (periodos ocupados) de cada uno; sin rango, solo el
// catálogo. LECTURA (GET), sin CSRF.
func (h *Handlers) ListResources(w http.ResponseWriter, r *http.Request) {
	if _, ok := subject(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	q := r.URL.Query()
	var from, to time.Time
	withAvail := q.Get("start") != "" && q.Get("end") != ""
	if withAvail {
		f, t2, err := parseRange(r, h.cfg.MaxRangeWindow)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		from, to = f, t2
	}
	list, err := h.svc.ListResources(r.Context(), from, to, withAvail)
	if err != nil {
		fail(w, r, err)
		return
	}
	out := make([]resourceDTO, 0, len(list))
	for _, ra := range list {
		busy := ra.Busy
		if busy == nil {
			busy = []domain.FreeBusyPeriod{}
		}
		out = append(out, resourceDTO{Resource: ra.Resource, Busy: busy})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"resources": out})
}

// GetResource devuelve un recurso por ID.
func (h *Handlers) GetResource(w http.ResponseWriter, r *http.Request) {
	if _, ok := subject(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid resource id."))
		return
	}
	res, err := h.svc.GetResource(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, res)
}

type resourceReq struct {
	Kind       string   `json:"kind"`
	Name       string   `json:"name"`
	Capacity   int      `json:"capacity"`
	Location   string   `json:"location"`
	Email      string   `json:"email"`
	AutoAccept *bool    `json:"auto_accept"`
	Managers   []string `json:"managers"`
}

// CreateResource da de alta un recurso. Solo rol admin/owner del tenant (else 403);
// el creador queda como gestor. Mutación por POST (CSRF-safe).
func (h *Handlers) CreateResource(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if !isTenantAdmin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Only an administrator can add rooms and equipment."))
		return
	}
	var req resourceReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	autoAccept := true
	if req.AutoAccept != nil {
		autoAccept = *req.AutoAccept
	}
	res, err := h.svc.CreateResource(r.Context(), store.NewResource{
		Kind:       domain.ResourceKind(req.Kind),
		Name:       req.Name,
		Capacity:   req.Capacity,
		Location:   req.Location,
		Email:      req.Email,
		AutoAccept: autoAccept,
		Managers:   req.Managers,
		CreatedBy:  sub,
	})
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, res)
}

type resourcePatchReq struct {
	Name       *string   `json:"name"`
	Capacity   *int      `json:"capacity"`
	Location   *string   `json:"location"`
	Email      *string   `json:"email"`
	AutoAccept *bool     `json:"auto_accept"`
	Active     *bool     `json:"active"`
	Managers   *[]string `json:"managers"`
}

// UpdateResource edita un recurso (solo gestor o admin del tenant).
func (h *Handlers) UpdateResource(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid resource id."))
		return
	}
	var req resourcePatchReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	res, err := h.svc.UpdateResource(r.Context(), id, sub, isTenantAdmin(r), store.ResourcePatch{
		Name:       req.Name,
		Capacity:   req.Capacity,
		Location:   req.Location,
		Email:      req.Email,
		AutoAccept: req.AutoAccept,
		Active:     req.Active,
		Managers:   req.Managers,
	})
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, res)
}

// DeleteResource desactiva un recurso (solo gestor o admin del tenant).
func (h *Handlers) DeleteResource(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid resource id."))
		return
	}
	if err := h.svc.DeleteResource(r.Context(), id, sub, isTenantAdmin(r)); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

type bookResourceReq struct {
	UID     string `json:"uid"`
	Summary string `json:"summary"`
	// Organizer se ACEPTA en el cuerpo por compatibilidad con clientes ya escritos,
	// pero se IGNORA: el organizador de una reserva es quien la hace. Ver BookResource.
	Organizer string      `json:"organizer"`
	Start     time.Time   `json:"start"`
	End       time.Time   `json:"end"`
	TZID      string      `json:"tzid"`
	AllDay    bool        `json:"all_day"`
	RRule     string      `json:"rrule"`
	RDate     []time.Time `json:"rdate"`
	ExDate    []time.Time `json:"exdate"`
}

// bookingStatusLabel traduce el PARTSTAT resultante a la etiqueta del cliente.
func bookingStatusLabel(p domain.ParticipationStatus) string {
	switch p {
	case domain.PartAccepted:
		return "accepted"
	case domain.PartDeclined:
		return "declined"
	default:
		return "pending"
	}
}

// BookResource reserva un recurso para un evento (por su UID). Devuelve accept/decline
// (o pending si el recurso requiere aprobación manual). Cualquier usuario autenticado
// del tenant puede reservar un recurso activo. Mutación por POST (CSRF-safe).
func (h *Handlers) BookResource(w http.ResponseWriter, r *http.Request) {
	if _, ok := subject(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid resource id."))
		return
	}
	var req bookResourceReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// El organizador es SIEMPRE quien llama, nunca lo que diga el cuerpo. Antes se
	// tomaba `req.Organizer` y solo se caía al usuario real si venía vacío, así que
	// cualquiera podía reservar una sala a nombre de otra persona. Y no era solo
	// cosmético: liberar una reserva se autoriza comparando ese organizador con quien
	// llama (ReleaseResourceBooking), de modo que el campo del cuerpo decidía quién
	// mandaba sobre la reserva.
	res, err := h.svc.BookResource(r.Context(), id, service.BookingSpec{
		SourceUID: req.UID,
		Summary:   req.Summary,
		Organizer: userEmail(r),
		DTStart:   req.Start,
		DTEnd:     req.End,
		TZID:      req.TZID,
		AllDay:    req.AllDay,
		RRule:     req.RRule,
		RDate:     req.RDate,
		ExDate:    req.ExDate,
	})
	if err != nil {
		fail(w, r, err)
		return
	}
	body := map[string]any{"status": bookingStatusLabel(res.Status), "resource": res.Resource}
	if res.Conflict != nil {
		body["conflict"] = res.Conflict
	}
	httpx.JSON(w, r, http.StatusOK, body)
}

// ReleaseResource libera la reserva del (evento, recurso). ?source_uid es el UID del
// evento origen. Autorizado para quien reservó, un gestor del recurso o un admin.
func (h *Handlers) ReleaseResource(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid resource id."))
		return
	}
	sourceUID := strings.TrimSpace(r.URL.Query().Get("source_uid"))
	if sourceUID == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Missing 'source_uid'."))
		return
	}
	if err := h.svc.ReleaseResource(r.Context(), sub, userEmail(r), isTenantAdmin(r), id, sourceUID); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}
