// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// calendarService es la superficie del servicio que consume la capa HTTP. Se
// declara como interfaz (satisfecha por *service.Service) para poder inyectar
// dobles en los tests sin depender de una base de datos.
type calendarService interface {
	EnsureDefaultCalendar(ctx context.Context, ownerSub string) (domain.Calendar, error)
	ListCalendars(ctx context.Context, ownerSub string) ([]domain.Calendar, error)
	GetCalendar(ctx context.Context, id uuid.UUID) (domain.Calendar, error)
	CreateCalendar(ctx context.Context, nc store.NewCalendar) (domain.Calendar, error)
	UpdateCalendar(ctx context.Context, id uuid.UUID, p store.CalendarPatch) (domain.Calendar, error)
	DeleteCalendar(ctx context.Context, id uuid.UUID) error

	SubscribeICS(ctx context.Context, ownerSub, url, displayName, color string) (domain.Calendar, error)
	RefreshSubscription(ctx context.Context, calendarID uuid.UUID) (domain.Calendar, error)

	CreateEvent(ctx context.Context, in service.EventInput) (domain.Event, error)
	GetEvent(ctx context.Context, id uuid.UUID) (domain.Event, error)
	EventReminders(ctx context.Context, ev domain.Event) ([]int, error)
	UpdateEvent(ctx context.Context, in service.EventInput) (domain.Event, error)
	DeleteEvent(ctx context.Context, id uuid.UUID) error
	DeleteEventNotifying(ctx context.Context, id uuid.UUID, send bool) error
	ListInstances(ctx context.Context, calendarID uuid.UUID, from, to time.Time) ([]domain.Instance, error)
	ListEventsInRange(ctx context.Context, calendarID uuid.UUID, from, to time.Time) ([]domain.Event, error)
	ListObjects(ctx context.Context, calendarID uuid.UUID) ([]domain.CalendarObject, error)
	SearchEvents(ctx context.Context, q store.EventSearchQuery) ([]domain.Event, error)
	EsInvitadoDeEvento(ctx context.Context, uid string) (bool, error)

	SetAttendeePartStat(ctx context.Context, eventID uuid.UUID, email string, partstat domain.ParticipationStatus) (domain.Attendee, error)
	FreeBusy(ctx context.Context, ownerSub string, from, to time.Time) ([]domain.FreeBusyPeriod, error)
	SuggestSlots(ctx context.Context, users []string, from, to time.Time, dur time.Duration, loc *time.Location, wh domain.WorkingHours, limit int) ([]domain.FreeSlot, error)

	DueReminders(ctx context.Context, ownerSub string, now time.Time, before *store.ReminderCursor, limit int) ([]store.DueReminder, error)
	DismissReminder(ctx context.Context, ownerSub string, id uuid.UUID) error
	AmendRecurrence(ctx context.Context, calendarID uuid.UUID, uid string, recurrenceID time.Time, mode string, expectETag uuid.UUID) (domain.CalendarObject, error)

	PedirEdicion(ctx context.Context, calendarID uuid.UUID, p peticiones.Peticion, token string) (bool, error)
	PeticionesEdicion(ctx context.Context, calendarID uuid.UUID, ownerSub string) ([]peticiones.Peticion, error)
	ResolverPeticion(ctx context.Context, calendarID uuid.UUID, ownerSub, actor, sujeto string, concede bool) error

	ListInbox(ctx context.Context, recipient string, onlyPending bool, limit int) ([]domain.Invitation, error)
	RespondInvitation(ctx context.Context, id uuid.UUID, status domain.InvitationStatus) (domain.Invitation, error)
	// RecibirRespuesta aplica el REPLY de una invitación contestada por correo.
	RecibirRespuesta(ctx context.Context, remitente, ics string) error

	// Compartir calendarios con ACL (solo el dueño gestiona; el acceso a lo
	// compartido va por la verja CalendarAccess dentro del servicio).
	ShareCalendar(ctx context.Context, calendarID uuid.UUID, ownerSub, ownerEmail, granteeEmail string, perm domain.SharePermission) (domain.CalendarShare, error)
	ListCalendarShares(ctx context.Context, calendarID uuid.UUID, ownerSub string) ([]domain.CalendarShare, error)
	RevokeCalendarShare(ctx context.Context, calendarID, shareID uuid.UUID, ownerSub string) error
	SharedWithMe(ctx context.Context, sub, email string) ([]store.SharedCalendar, error)

	// Recursos reservables (salas, equipos). El anti doble-reserva atómico y el
	// aislamiento por tenant viven en el store; la verja de rol/gestor la fija la
	// capa HTTP (isAdmin) o el store (IsManager).
	ListResources(ctx context.Context, from, to time.Time, withAvailability bool) ([]store.ResourceAvailability, error)
	GetResource(ctx context.Context, id uuid.UUID) (domain.Resource, error)
	CreateResource(ctx context.Context, nr store.NewResource) (domain.Resource, error)
	UpdateResource(ctx context.Context, id uuid.UUID, sub string, isAdmin bool, p store.ResourcePatch) (domain.Resource, error)
	DeleteResource(ctx context.Context, id uuid.UUID, sub string, isAdmin bool) error
	BookResource(ctx context.Context, resourceID uuid.UUID, spec service.BookingSpec) (store.BookingResult, error)
	ReleaseResource(ctx context.Context, sub, email string, isAdmin bool, resourceID uuid.UUID, sourceUID string) error
}

// Handlers agrupa los manejadores HTTP de la API REST de Calendar.
type Handlers struct {
	svc calendarService
	cfg config.Config
}

// fail traduce un error de dominio/servicio a la respuesta HTTP canonica.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	httpx.Fail(w, r, toAPIError(err))
}

// toAPIError normaliza un error de dominio/servicio al error HTTP canonico. Lo
// comparten la API nativa y la capa de compatibilidad, para que el MISMO error de
// dominio produzca el MISMO estado por las dos vias (solo cambia el sobre).
func toAPIError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrNotFound
	case errors.Is(err, domain.ErrConflict):
		return httpx.ErrConflict
	case errors.Is(err, domain.ErrPreconditionFailed):
		return httpx.NewError(http.StatusPreconditionFailed, "precondition_failed", "The ETag precondition failed.")
	case errors.Is(err, domain.ErrDefaultImmutable):
		return httpx.ErrForbidden.WithMessage("The default calendar cannot be deleted.")
	case errors.Is(err, domain.ErrReadOnly):
		return httpx.ErrForbidden.WithMessage("This calendar is read-only (a subscription).")
	case errors.Is(err, domain.ErrNotManager):
		return httpx.ErrForbidden.WithMessage("You do not have permission to manage this resource.")
	case errors.Is(err, domain.ErrResourceInactive):
		return httpx.ErrConflict.WithMessage("This resource is deactivated.")
	case errors.Is(err, domain.ErrInvalidURL):
		return httpx.ErrBadRequest.WithMessage("The subscription URL is not valid (it must be a public http/https URL).")
	case errors.Is(err, domain.ErrCupoInvitaciones):
		return httpx.NewError(http.StatusTooManyRequests, "invitation_rate",
			"Too many invitations sent by email in the last hour. Try again later.")
	case service.IsValidationError(err):
		return httpx.ErrBadRequest.WithMessage(err.Error())
	default:
		return err
	}
}

// subject extrae el sub del usuario autenticado del contexto.
func subject(r *http.Request) (string, bool) {
	claims, ok := authn.FromContext(r.Context())
	if !ok || claims.Subject == "" {
		return "", false
	}
	return claims.Subject, true
}

// userEmail devuelve el email del usuario autenticado (para inbox/free-busy).
func userEmail(r *http.Request) string {
	claims, _ := authn.FromContext(r.Context())
	return claims.Email
}

// parseUUID extrae y valida el UUID del parámetro de ruta "id".
func parseUUID(r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// --- Calendarios ------------------------------------------------------------

func (h *Handlers) ListCalendars(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	// Garantiza el calendario por defecto en el primer acceso.
	if _, err := h.svc.EnsureDefaultCalendar(r.Context(), sub); err != nil {
		fail(w, r, err)
		return
	}
	cals, err := h.svc.ListCalendars(r.Context(), sub)
	if err != nil {
		fail(w, r, err)
		return
	}
	if cals == nil {
		cals = []domain.Calendar{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"calendars": cals})
}

type createCalendarReq struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Color       string   `json:"color"`
	Timezone    string   `json:"timezone"`
	Components  []string `json:"components"`
}

func (h *Handlers) CreateCalendar(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req createCalendarReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := h.svc.CreateCalendar(r.Context(), store.NewCalendar{
		OwnerSub:    sub,
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Color:       req.Color,
		Timezone:    req.Timezone,
		Components:  req.Components,
	})
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, c)
}

func (h *Handlers) GetCalendar(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid calendar id."))
		return
	}
	c, err := h.svc.GetCalendar(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, c)
}

type updateCalendarReq struct {
	DisplayName *string `json:"display_name"`
	Description *string `json:"description"`
	Color       *string `json:"color"`
	Timezone    *string `json:"timezone"`
}

func (h *Handlers) UpdateCalendar(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid calendar id."))
		return
	}
	var req updateCalendarReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := h.svc.UpdateCalendar(r.Context(), id, store.CalendarPatch{
		DisplayName: req.DisplayName,
		Description: req.Description,
		Color:       req.Color,
		Timezone:    req.Timezone,
	})
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, c)
}

func (h *Handlers) DeleteCalendar(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid calendar id."))
		return
	}
	if err := h.svc.DeleteCalendar(r.Context(), id); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// --- Suscripciones ICS externas ---------------------------------------------

type subscribeReq struct {
	URL         string `json:"url"`
	DisplayName string `json:"display_name"`
	Color       string `json:"color"`
}

// Subscribe da de alta una suscripción a un feed ICS externo (el «Añadir
// calendario por URL»): crea un calendario de solo lectura respaldado por la URL.
// Mutación por POST (CSRF-safe). La barrera SSRF completa se aplica en el fetch.
func (h *Handlers) Subscribe(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req subscribeReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := h.svc.SubscribeICS(r.Context(), sub, req.URL, req.DisplayName, req.Color)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, c)
}

// RefreshSubscription fuerza un refresco inmediato de la suscripción de un
// calendario («actualizar ahora»). Mutación por POST (CSRF-safe).
func (h *Handlers) RefreshSubscription(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid calendar id."))
		return
	}
	c, err := h.svc.RefreshSubscription(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, c)
}

// --- Eventos ----------------------------------------------------------------

type eventReq struct {
	UID            string        `json:"uid"`
	Summary        string        `json:"summary"`
	Description    string        `json:"description"`
	Location       string        `json:"location"`
	Organizer      string        `json:"organizer"`
	Start          time.Time     `json:"start"`
	End            time.Time     `json:"end"`
	TZID           string        `json:"tzid"`
	AllDay         bool          `json:"all_day"`
	Status         string        `json:"status"`
	Classification string        `json:"classification"`
	Transparency   string        `json:"transp"`
	Color          string        `json:"color"`
	ConferenceURL  string        `json:"conference_url"`
	RRule          string        `json:"rrule"`
	RDate          []time.Time   `json:"rdate"`
	ExDate         []time.Time   `json:"exdate"`
	Attendees      []attendeeReq `json:"attendees"`
	Send           bool          `json:"send"`
	// Reminders: minutos antes del inicio. Ausente (o null), no se tocan los que haya.
	Reminders *[]int `json:"reminders"`
}

type attendeeReq struct {
	Email      string `json:"email"`
	CommonName string `json:"common_name"`
	Role       string `json:"role"`
	PartStat   string `json:"partstat"`
	CUType     string `json:"cutype"`
	RSVP       bool   `json:"rsvp"`
}

func (req eventReq) toInput(calendarID uuid.UUID) service.EventInput {
	atts := make([]domain.Attendee, 0, len(req.Attendees))
	for _, a := range req.Attendees {
		atts = append(atts, domain.Attendee{
			Email:      a.Email,
			CommonName: a.CommonName,
			Role:       domain.AttendeeRole(a.Role),
			PartStat:   domain.ParticipationStatus(a.PartStat),
			CUType:     a.CUType,
			RSVP:       a.RSVP,
		})
	}
	return service.EventInput{
		CalendarID:     calendarID,
		UID:            req.UID,
		Summary:        req.Summary,
		Description:    req.Description,
		Location:       req.Location,
		Organizer:      req.Organizer,
		DTStart:        req.Start,
		DTEnd:          req.End,
		TZID:           req.TZID,
		AllDay:         req.AllDay,
		Status:         domain.EventStatus(req.Status),
		Classification: domain.Classification(req.Classification),
		Transparency:   domain.Transparency(req.Transparency),
		Color:          req.Color,
		ConferenceURL:  req.ConferenceURL,
		RRule:          req.RRule,
		RDate:          req.RDate,
		ExDate:         req.ExDate,
		Attendees:      atts,
		Send:           req.Send,
		Reminders:      req.Reminders,
	}
}

func (h *Handlers) CreateEvent(w http.ResponseWriter, r *http.Request) {
	calID, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid calendar id."))
		return
	}
	var req eventReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in := req.toInput(calID)
	if in.Organizer == "" {
		in.Organizer = userEmail(r)
	}
	ev, err := h.svc.CreateEvent(r.Context(), in)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, ev)
}

// EventInvited — GET /api/v1/events/invited?uid=…[&org=…] — 204 si quien llama organiza el
// evento de ese UID o está invitado a él, 404 si no. Lo pregunta Rooms: la reunión de un
// evento la abren sus invitados (decidido por el dueño el 2026-10-16).
//
// `org` es la organización ANFITRIONA cuando quien llama es de otra («Que entren como
// invitados», decidido por el dueño el 2026-10-16): la respuesta sigue siendo solo sobre
// SU invitación —sus direcciones y su sub, del token verificado—, así que preguntar por
// otra organización no enseña nada de nadie más.
func (h *Handlers) EventInvited(w http.ResponseWriter, r *http.Request) {
	uid := r.URL.Query().Get("uid")
	if uid == "" || len(uid) > 1024 {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	ctx := r.Context()
	if org := r.URL.Query().Get("org"); org != "" {
		t, err := tenancy.New(org)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		ctx = tenancy.Into(ctx, t)
	}
	si, err := h.svc.EsInvitadoDeEvento(ctx, uid)
	if err != nil {
		fail(w, r, err)
		return
	}
	if !si {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	httpx.NoContent(w)
}

func (h *Handlers) GetEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid event id."))
		return
	}
	ev, err := h.svc.GetEvent(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	// Solo el evento suelto trae sus recordatorios: para eso hay que leer su recurso, y
	// en un listado sería una lectura más por evento. Es lo que pide el formulario.
	rec, err := h.svc.EventReminders(r.Context(), ev)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, eventConRecordatorios{Event: ev, Reminders: rec})
}

// eventConRecordatorios es el evento con sus recordatorios editables (minutos antes).
type eventConRecordatorios struct {
	domain.Event
	Reminders []int `json:"reminders"`
}

func (h *Handlers) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid event id."))
		return
	}
	// Resuelve el evento para conocer su calendario y UID (la edicion es por UID).
	cur, err := h.svc.GetEvent(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	var req eventReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in := req.toInput(cur.CalendarID)
	in.UID = cur.UID // el UID es inmutable; ignora cualquier UID del cuerpo
	if in.Organizer == "" {
		in.Organizer = cur.Organizer
	}
	ev, err := h.svc.UpdateEvent(r.Context(), in)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, ev)
}

func (h *Handlers) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid event id."))
		return
	}
	if err := h.svc.DeleteEvent(r.Context(), id); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// ListInstances expande las recurrencias de un calendario en [start, end).
func (h *Handlers) ListInstances(w http.ResponseWriter, r *http.Request) {
	calID, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid calendar id."))
		return
	}
	from, to, err := parseRange(r, h.cfg.MaxRangeWindow)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	insts, err := h.svc.ListInstances(r.Context(), calID, from, to)
	if err != nil {
		fail(w, r, err)
		return
	}
	if insts == nil {
		insts = []domain.Instance{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"start":     from,
		"end":       to,
		"instances": insts,
	})
}

// SearchEvents busca los eventos del usuario por TEXTO (título/descripción/lugar/
// organizador/asistentes) con rango de fechas (?from, ?to RFC3339) y calendario
// (?calendar UUID) opcionales, paginado por keyset (?cursor, ?limit). Es una
// LECTURA (GET) de solo lectura: sin CSRF. Solo eventos de los calendarios del
// usuario autenticado (aislamiento por dueño, además del tenant por RLS). Con ?q
// vacío o demasiado corto devuelve una lista vacía (no vuelca el calendario). Las
// series recurrentes aparecen UNA vez (el maestro, marcado por su rrule/rdate).
func (h *Handlers) SearchEvents(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	qp := r.URL.Query()
	limit := parsePositiveInt(qp.Get("limit"), 50)
	if limit > 100 {
		limit = 100
	}
	q := store.EventSearchQuery{
		OwnerSub: sub,
		Text:     qp.Get("q"),
		Limit:    limit,
	}
	if cal := strings.TrimSpace(qp.Get("calendar")); cal != "" {
		id, err := uuid.Parse(cal)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The 'calendar' filter must be a UUID."))
			return
		}
		q.CalendarID = id
	}
	from, hasFrom, err := parseOptTime(qp.Get("from"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("'from' must be an RFC3339 date."))
		return
	}
	if hasFrom {
		q.From = &from
	}
	to, hasTo, err := parseOptTime(qp.Get("to"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("'to' must be an RFC3339 date."))
		return
	}
	if hasTo {
		q.To = &to
	}
	if cur := qp.Get("cursor"); cur != "" {
		c, err := parseEventCursor(cur)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid cursor."))
			return
		}
		q.After = c
	}

	events, err := h.svc.SearchEvents(r.Context(), q)
	if err != nil {
		fail(w, r, err)
		return
	}
	if events == nil {
		events = []domain.Event{}
	}
	// next_cursor: si la página vino llena, el keyset continúa desde su último
	// (dtstart, id); si no, no hay más resultados.
	next := ""
	if len(events) == limit && limit > 0 {
		last := events[len(events)-1]
		next = encodeEventCursor(last.DTStart, last.ID)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"events": events, "next_cursor": next})
}

// --- Asistentes / RSVP ------------------------------------------------------

type partStatReq struct {
	// Email se acepta por compatibilidad de wire pero se IGNORA: el RSVP es
	// siempre del usuario autenticado (email del JWT), nunca del cuerpo.
	Email    string `json:"email"`
	PartStat string `json:"partstat"`
}

// SetPartStat fija el PARTSTAT (RSVP) del PROPIO usuario autenticado en el
// evento {id}. El asistente se identifica SIEMPRE por el email del JWT
// verificado, nunca por el cuerpo (nadie responde por otro). Si el usuario no
// es asistente del evento → 404. El organizador/delegado edita PARTSTAT de
// terceros via PUT /events/{id} bajo la verja CalendarAccess.
func (h *Handlers) SetPartStat(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid event id."))
		return
	}
	var req partStatReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	email := userEmail(r)
	if email == "" {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("The token carries no email address, so invitations cannot be answered."))
		return
	}
	a, err := h.svc.SetAttendeePartStat(r.Context(), id, email, domain.ParticipationStatus(req.PartStat))
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, a)
}

// --- Free/Busy --------------------------------------------------------------

func (h *Handlers) FreeBusy(w http.ResponseWriter, r *http.Request) {
	from, to, err := parseRange(r, h.cfg.MaxFreeBusyWindow)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Por defecto, free/busy del propio usuario; "user" (sub) opcional para
	// consultar la disponibilidad de un companero del tenant.
	target := r.URL.Query().Get("user")
	if target == "" {
		sub, ok := subject(r)
		if !ok {
			httpx.Fail(w, r, httpx.ErrUnauthorized)
			return
		}
		target = sub
	}
	periods, err := h.svc.FreeBusy(r.Context(), target, from, to)
	if err != nil {
		fail(w, r, err)
		return
	}
	if periods == nil {
		periods = []domain.FreeBusyPeriod{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"user":    target,
		"start":   from,
		"end":     to,
		"periods": periods,
	})
}

// SuggestSlots («buscar hueco») calcula los huecos libres comunes de los
// participantes en una ventana y una franja laboral. Es una LECTURA (GET), sin
// mutacion: no requiere CSRF. Participantes por su sub en ?user (repetible);
// SIEMPRE se incluye al usuario autenticado (el organizador) para que su propia
// disponibilidad cuente. duration en minutos; work_start/work_end en "HH:MM"
// interpretados en ?tz (IANA); ?weekdays=true limita a dias laborables.
func (h *Handlers) SuggestSlots(w http.ResponseWriter, r *http.Request) {
	from, to, err := parseRange(r, h.cfg.MaxFreeBusyWindow)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := r.URL.Query()
	users := append([]string{}, q["user"]...)
	if sub, ok := subject(r); ok {
		users = append(users, sub)
	}
	if len(users) == 0 {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	dur := time.Duration(parsePositiveInt(q.Get("duration"), 30)) * time.Minute
	loc := domain.LoadLocationCached(q.Get("tz"))
	wh := domain.WorkingHours{
		StartMin:     parseHHMM(q.Get("work_start"), 9*60),
		EndMin:       parseHHMM(q.Get("work_end"), 17*60),
		WeekdaysOnly: q.Get("weekdays") == "true",
	}
	limit := parsePositiveInt(q.Get("limit"), 50)
	if limit > 200 {
		limit = 200
	}
	slots, err := h.svc.SuggestSlots(r.Context(), users, from, to, dur, loc, wh, limit)
	if err != nil {
		fail(w, r, err)
		return
	}
	if slots == nil {
		slots = []domain.FreeSlot{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"start":            from,
		"end":              to,
		"duration_minutes": int(dur / time.Minute),
		"tz":               q.Get("tz"),
		"slots":            slots,
	})
}

// parsePositiveInt lee un entero > 0 de la query, o def si vacio/invalido.
func parsePositiveInt(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
		return n
	}
	return def
}

// parseHHMM convierte "HH:MM" a minutos desde medianoche; def si vacio/invalido.
func parseHHMM(s string, def int) int {
	hs, ms, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok {
		return def
	}
	h, err1 := strconv.Atoi(hs)
	m, err2 := strconv.Atoi(ms)
	if err1 != nil || err2 != nil || h < 0 || h > 24 || m < 0 || m > 59 {
		return def
	}
	return h*60 + m
}

// --- Invitaciones -----------------------------------------------------------

func (h *Handlers) ListInbox(w http.ResponseWriter, r *http.Request) {
	email := userEmail(r)
	if email == "" {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("The token carries no email address, so the invitations inbox cannot be opened."))
		return
	}
	onlyPending := r.URL.Query().Get("pending") == "true"
	invs, err := h.svc.ListInbox(r.Context(), email, onlyPending, 100)
	if err != nil {
		fail(w, r, err)
		return
	}
	if invs == nil {
		invs = []domain.Invitation{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"invitations": invs})
}

type respondReq struct {
	Status string `json:"status"` // accepted | declined | tentative
}

func (h *Handlers) RespondInvitation(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid invitation id."))
		return
	}
	var req respondReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	inv, err := h.svc.RespondInvitation(r.Context(), id, domain.InvitationStatus(req.Status))
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, inv)
}

// --- Recordatorios in-app ---------------------------------------------------

// reminderDTO es la vista JSON de un recordatorio vencido (sin fugas de
// tenant/owner). Deep-link: el cliente lleva al usuario al evento por su dtstart.
type reminderDTO struct {
	ID        uuid.UUID `json:"id"`
	EventID   uuid.UUID `json:"event_id"`
	UID       string    `json:"uid"`
	Summary   string    `json:"summary"`
	DTStart   time.Time `json:"dtstart"`
	TriggerAt time.Time `json:"trigger_at"`
	Action    string    `json:"action"`
}

// ListDueReminders lista los recordatorios vencidos y no descartados del usuario
// (entrega in-app), keyset por ?cursor y ?limit (por defecto 50, máx 100).
func (h *Handlers) ListDueReminders(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	before, err := parseReminderCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid cursor."))
		return
	}
	rems, err := h.svc.DueReminders(r.Context(), sub, time.Now(), before, limit)
	if err != nil {
		fail(w, r, err)
		return
	}
	out := make([]reminderDTO, 0, len(rems))
	for _, rem := range rems {
		out = append(out, reminderDTO{
			ID: rem.ID, EventID: rem.EventID, UID: rem.UID, Summary: rem.Summary,
			DTStart: rem.DTStart, TriggerAt: rem.TriggerAt, Action: rem.Action,
		})
	}
	next := ""
	if len(rems) == limit && limit > 0 {
		last := rems[len(rems)-1]
		next = encodeReminderCursor(last.TriggerAt, last.ID)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"reminders": out, "next_cursor": next})
}

// DismissReminder descarta un recordatorio in-app del usuario (mutacion por POST,
// CSRF-safe). Idempotente; 404 si no es del usuario.
func (h *Handlers) DismissReminder(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid reminder id."))
		return
	}
	if err := h.svc.DismissReminder(r.Context(), sub, id); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// encodeReminderCursor serializa el punto keyset (trigger_at, id) en un cursor
// opaco (base64url de "RFC3339Nano|uuid").
func encodeReminderCursor(triggerAt time.Time, id uuid.UUID) string {
	raw := triggerAt.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// parseReminderCursor decodifica un cursor opaco. "" => primera página (nil).
func parseReminderCursor(s string) (*store.ReminderCursor, error) {
	if s == "" {
		//nolint:nilnil // (nil, nil) ES el contrato documentado: sin cursor ⇒ primera página, no un error.
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	before, after, found := strings.Cut(string(raw), "|")
	if !found {
		return nil, errors.New("cursor sin separador")
	}
	ts, err := time.Parse(time.RFC3339Nano, before)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(after)
	if err != nil {
		return nil, err
	}
	return &store.ReminderCursor{TriggerAt: ts, ID: id}, nil
}

// --- Edición de series recurrentes ------------------------------------------

type amendRecurrenceReq struct {
	UID          string    `json:"uid"`
	RecurrenceID time.Time `json:"recurrence_id"`
	Mode         string    `json:"mode"` // exclude | until
	ETag         string    `json:"etag"`
}

// AmendRecurrence recorta el maestro de una serie (EXDATE o UNTIL) para editar/
// borrar «solo este» / «este y siguientes» sin corromper la serie. El calendario
// va en la ruta ({id}); el objeto, por UID en el cuerpo.
func (h *Handlers) AmendRecurrence(w http.ResponseWriter, r *http.Request) {
	calID, ok := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid calendar id."))
		return
	}
	var req amendRecurrenceReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.UID == "" || req.RecurrenceID.IsZero() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("'uid' and 'recurrence_id' are required."))
		return
	}
	if req.Mode != "exclude" && req.Mode != "until" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("'mode' must be 'exclude' or 'until'."))
		return
	}
	expectETag := uuid.Nil
	if req.ETag != "" {
		if parsed, err := uuid.Parse(strings.Trim(req.ETag, `"`)); err == nil {
			expectETag = parsed
		}
	}
	obj, err := h.svc.AmendRecurrence(r.Context(), calID, req.UID, req.RecurrenceID, req.Mode, expectETag)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, obj)
}

// --- Helpers ----------------------------------------------------------------

// parseOptTime lee una fecha RFC3339 OPCIONAL de la query: (zero, false, nil) si
// está vacía; error solo si viene y no parsea.
func parseOptTime(s string) (time.Time, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false, nil
	}
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false, err
	}
	return ts.UTC(), true, nil
}

// encodeEventCursor serializa el punto keyset (dtstart, id) de la búsqueda en un
// cursor opaco (base64url de "RFC3339Nano|uuid"), como el de recordatorios.
func encodeEventCursor(dtstart time.Time, id uuid.UUID) string {
	raw := dtstart.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// parseEventCursor decodifica un cursor opaco de búsqueda (dtstart, id).
func parseEventCursor(s string) (*store.EventSearchCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	before, after, found := strings.Cut(string(raw), "|")
	if !found {
		return nil, errors.New("cursor sin separador")
	}
	ts, err := time.Parse(time.RFC3339Nano, before)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(after)
	if err != nil {
		return nil, err
	}
	return &store.EventSearchCursor{DTStart: ts, ID: id}, nil
}

// parseRange lee los parametros start/end (RFC3339) de la query y los valida.
func parseRange(r *http.Request, maxWindow time.Duration) (time.Time, time.Time, error) {
	q := r.URL.Query()
	startStr, endStr := q.Get("start"), q.Get("end")
	if startStr == "" || endStr == "" {
		return time.Time{}, time.Time{}, httpx.ErrBadRequest.WithMessage("The 'start' and 'end' parameters are required (RFC3339).")
	}
	from, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return time.Time{}, time.Time{}, httpx.ErrBadRequest.WithMessage("'start' must be an RFC3339 date.")
	}
	to, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return time.Time{}, time.Time{}, httpx.ErrBadRequest.WithMessage("'end' must be an RFC3339 date.")
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, httpx.ErrBadRequest.WithMessage("'end' must be later than 'start'.")
	}
	if maxWindow > 0 && to.Sub(from) > maxWindow {
		return time.Time{}, time.Time{}, httpx.ErrBadRequest.WithMessage("The requested time window is too wide.")
	}
	return from.UTC(), to.UTC(), nil
}
