// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/calendar/internal/compat"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
)

// Capa de compatibilidad de calendario: Sytue Calendar respondiendo un subconjunto
// PRAGMÁTICO de la API de Google Calendar v3 (/compat/google) y de Microsoft Graph
// calendar (/compat/graph), para que una integración escrita contra Google o
// Microsoft —con base URL configurable— funcione sin cambios. La traducción de
// modelos vive en internal/compat (pura y testeable); aquí solo hay handlers finos.
//
// AISLAMIENTO (crítico): estos handlers usan EXACTAMENTE el mismo servicio y el
// mismo contexto de petición que la API nativa (GetCalendar/GetEvent/ListInstances
// aplican la verja CalendarAccess). Nunca tocan el store en crudo, así que el
// tenant (RLS), el dueño y los shares se aplican igual, y un calendario ajeno o
// inexistente responden lo mismo: 404. La autenticación es el Bearer de Sytue ID
// (mismo middleware): no se emula ningún flujo OAuth de Google/Microsoft.
//
// VENTANA TEMPORAL OBLIGATORIA en los listados de eventos (timeMin/timeMax de
// Google, $filter de Graph): el servicio lista por rango, así que sin ventana no se
// puede devolver «todo» — y devolver un trozo por defecto sería una agenda
// incompleta que el cliente daría por buena. Sin ventana: 400 explicando. La
// ventana se acota con el MISMO máximo que /api/v1 (MAX_RANGE_WINDOW) y el listado
// hereda el mismo techo de eventos por consulta que la API nativa: por la vía
// compat nunca se ve ni más ni menos que por la nativa.
//
// Lo que NO está implementado responde 501 con mensaje claro (nunca un 200 vacío):
// el resto de Google Calendar v3 (events.patch/update/delete/move/instances,
// calendars.*, acl, freebusy, colors, settings, channels/watch, sincronización por
// syncToken) y el resto de Graph (calendarView, eventos con recurrencia
// estructurada, escritura, delta, subscriptions, /users/{id}/…).

// Tamaños de página de la emulación: por defecto y techo de cada dialecto.
const (
	googleDefaultPage = 250
	googleMaxPage     = 1000
	graphDefaultPage  = 100
	graphMaxPage      = 500
	graphMaxSkip      = 100000
)

// compatHandlers sirve ambos dialectos sobre el mismo servicio que la API nativa.
type compatHandlers struct {
	svc calendarService
	cfg config.Config
}

// Scopes de Google Calendar que abren /compat/google a una app de terceros registrada con
// permisos de Google (cuenta de Google emulada, ver authn.RequireGoogleAPIs).
var (
	googleScopesLectura   = []string{googleScope + "calendar.readonly", googleScope + "calendar.events.readonly"}
	googleScopesEscritura = []string{googleScope + "calendar", googleScope + "calendar.events"}
)

const googleScope = "https://www.googleapis.com/auth/"

// mountCompat monta las dos capas. /compat/google lleva su propia verja
// (googleAuth), que admite además los tokens de apps de terceros con scopes de
// Google; /compat/graph sigue solo con la de la suite.
func mountCompat(r chi.Router, h *compatHandlers, googleAuth, auth func(http.Handler) http.Handler) {
	r.Route("/compat/google", func(g chi.Router) {
		g.Use(googleAuth)
		g.Get("/calendar/v3/users/me/calendarList", h.googleCalendarList)
		g.Get("/calendar/v3/calendars/{calendarId}/events", h.googleEvents)
		g.Get("/calendar/v3/calendars/{calendarId}/events/{eventId}", h.googleEvent)
		g.Post("/calendar/v3/calendars/{calendarId}/events", h.googleCreateEvent)
		g.Put("/calendar/v3/calendars/{calendarId}/events/{eventId}", h.googleUpdateEvent)
		g.Patch("/calendar/v3/calendars/{calendarId}/events/{eventId}", h.googlePatchEvent)
		g.Delete("/calendar/v3/calendars/{calendarId}/events/{eventId}", h.googleDeleteEvent)
		g.NotFound(h.googleNotImplemented)
		g.MethodNotAllowed(h.googleNotImplemented)
	})

	r.Route("/compat/graph", func(g chi.Router) {
		g.Use(auth)
		g.Get("/v1.0/me/calendars", h.graphCalendars)
		g.Get("/v1.0/me/events", h.graphEvents)
		g.Get("/v1.0/me/events/{eventId}", h.graphEvent)
		g.Get("/v1.0/me/calendars/{calendarId}/events", h.graphEvents)
		g.Get("/v1.0/me/calendars/{calendarId}/events/{eventId}", h.graphEvent)
		g.NotFound(h.graphNotImplemented)
		g.MethodNotAllowed(h.graphNotImplemented)
	})
}

// ---------------------------------------------------------------------------
// Respuestas de error en cada dialecto
// ---------------------------------------------------------------------------

// failGoogle responde el sobre de error de Google con el estado que la API nativa
// habría devuelto para ese mismo error de dominio.
func failGoogle(w http.ResponseWriter, r *http.Request, err error) {
	e := compatError(r, err)
	httpx.JSON(w, r, e.Status, compat.NewGoogleError(e.Status, e.Message))
}

// failGraph responde el sobre de error de Microsoft Graph.
func failGraph(w http.ResponseWriter, r *http.Request, err error) {
	e := compatError(r, err)
	httpx.JSON(w, r, e.Status, compat.NewGraphError(e.Status, e.Message))
}

// compatError normaliza el error a *httpx.Error (los de dominio, con el MISMO
// mapeo de códigos que la API nativa; los de traducción, a 400 con el término
// exacto) y registra los 5xx sin filtrar la causa al cliente.
func compatError(r *http.Request, err error) *httpx.Error {
	switch {
	case errors.Is(err, compat.ErrUnsupportedParam),
		errors.Is(err, compat.ErrUnsupportedFilter),
		errors.Is(err, compat.ErrInvalidPageToken),
		errors.Is(err, compat.ErrInvalidEvent):
		return httpx.ErrBadRequest.WithMessage(err.Error())
	}
	e := httpx.AsError(toAPIError(err))
	if e.Status >= http.StatusInternalServerError {
		log.From(r.Context()).Error("compat: error de servidor", "code", e.Code, "error", e.Error())
	}
	return e
}

// errNotImplemented describe un endpoint del dialecto que no se emula.
func errNotImplemented(dialect string) error {
	return httpx.NewError(http.StatusNotImplemented, "not_implemented",
		"Sytue Calendar emulates a subset of "+dialect+"; this endpoint is not implemented.")
}

func (h *compatHandlers) googleNotImplemented(w http.ResponseWriter, r *http.Request) {
	failGoogle(w, r, errNotImplemented("the Google Calendar v3 API"))
}

func (h *compatHandlers) graphNotImplemented(w http.ResponseWriter, r *http.Request) {
	failGraph(w, r, errNotImplemented("Microsoft Graph (calendar)"))
}

// ---------------------------------------------------------------------------
// Resolución de calendario y ventana temporal (compartidas por los dialectos)
// ---------------------------------------------------------------------------

// resolveCalendar resuelve el calendario de la ruta a través del SERVICIO: la
// alias «primary» (y la ruta /me de Graph) es el calendario por defecto del
// usuario; un UUID pasa por la verja de lectura, y cualquier otra cosa —id ajeno,
// inexistente o no-UUID— responde 404, sin distinguir unos de otros.
func (h *compatHandlers) resolveCalendar(r *http.Request, id string) (domain.Calendar, error) {
	if id == "" || id == compat.PrimaryAlias {
		sub, ok := subject(r)
		if !ok {
			return domain.Calendar{}, httpx.ErrUnauthorized
		}
		return h.svc.EnsureDefaultCalendar(r.Context(), sub)
	}
	cid, err := uuid.Parse(id)
	if err != nil {
		return domain.Calendar{}, domain.ErrNotFound
	}
	return h.svc.GetCalendar(r.Context(), cid)
}

// accessRole resuelve el rol del llamante sobre un calendario ya autorizado: dueño
// o el permiso del share por el que lo ve. No concede nada: solo describe.
func (h *compatHandlers) accessRole(r *http.Request, cal domain.Calendar) string {
	sub, _ := subject(r)
	if cal.OwnerSub == sub {
		return compat.RoleOwner
	}
	shared, err := h.svc.SharedWithMe(r.Context(), sub, userEmail(r))
	if err != nil {
		return compat.RoleReader
	}
	for _, s := range shared {
		if s.Calendar.ID == cal.ID {
			return compat.GoogleAccessRole(s.Permission)
		}
	}
	return compat.RoleReader
}

// compatWindow valida la ventana temporal pedida: obligatoria, ordenada y dentro
// del máximo configurado (el mismo que la API nativa aplica en /instances).
func (h *compatHandlers) compatWindow(from, to time.Time, hint string) error {
	if from.IsZero() || to.IsZero() {
		return httpx.ErrBadRequest.WithMessage("The emulation needs an explicit time window: " + hint + ".")
	}
	if !to.After(from) {
		return httpx.ErrBadRequest.WithMessage("The end of the time window must be later than its start.")
	}
	if h.cfg.MaxRangeWindow > 0 && to.Sub(from) > h.cfg.MaxRangeWindow {
		return httpx.ErrBadRequest.WithMessage("The requested time window is too wide.")
	}
	return nil
}

// listCalendars devuelve los calendarios visibles para el usuario (los suyos, con
// el por defecto garantizado, más los compartidos con él) junto a su rol. Es la
// misma lectura que sirve la API nativa: nada de consultas paralelas al store.
func (h *compatHandlers) listCalendars(r *http.Request) ([]domain.Calendar, []string, error) {
	sub, ok := subject(r)
	if !ok {
		return nil, nil, httpx.ErrUnauthorized
	}
	ctx := r.Context()
	if _, err := h.svc.EnsureDefaultCalendar(ctx, sub); err != nil {
		return nil, nil, err
	}
	own, err := h.svc.ListCalendars(ctx, sub)
	if err != nil {
		return nil, nil, err
	}
	shared, err := h.svc.SharedWithMe(ctx, sub, userEmail(r))
	if err != nil {
		return nil, nil, err
	}
	cals := make([]domain.Calendar, 0, len(own)+len(shared))
	roles := make([]string, 0, len(own)+len(shared))
	for _, c := range own {
		cals = append(cals, c)
		roles = append(roles, compat.RoleOwner)
	}
	for _, s := range shared {
		cals = append(cals, s.Calendar)
		roles = append(roles, compat.GoogleAccessRole(s.Permission))
	}
	return cals, roles, nil
}

// ---------------------------------------------------------------------------
// Google Calendar v3 — /compat/google
// ---------------------------------------------------------------------------

// googleCalendarList responde calendarList.list: los calendarios del usuario y los
// compartidos con él, con su accessRole real. `minAccessRole` se aplica de verdad;
// `showHidden`/`showDeleted` se aceptan como no-op documentado (Sytue no expone
// calendarios ocultos ni borrados por esta vía, así que el resultado es el mismo).
func (h *compatHandlers) googleCalendarList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := compat.CheckParams(q, "maxResults", "pageToken", "minAccessRole", "showHidden", "showDeleted"); err != nil {
		failGoogle(w, r, err)
		return
	}
	offset, err := compat.DecodePageToken(q.Get("pageToken"))
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	minRole := strings.TrimSpace(q.Get("minAccessRole"))
	if minRole != "" && minRole != compat.RoleOwner && minRole != compat.RoleWriter && minRole != compat.RoleReader {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("minAccessRole must be owner, writer or reader."))
		return
	}

	cals, roles, err := h.listCalendars(r)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	entries := make([]compat.GoogleCalendarListEntry, 0, len(cals))
	for i, c := range cals {
		if !roleAtLeast(roles[i], minRole) {
			continue
		}
		entries = append(entries, compat.GoogleFromCalendar(c, roles[i]))
	}

	page, next := compat.Slice(entries, offset, compat.PageSize(q.Get("maxResults"), googleDefaultPage, googleMaxPage))
	token := ""
	if next > 0 {
		token = compat.EncodePageToken(next)
	}
	httpx.JSON(w, r, http.StatusOK, compat.NewGoogleCalendarList(page, token))
}

// roleAtLeast indica si el rol cubre el mínimo pedido (owner > writer > reader).
func roleAtLeast(role, wanted string) bool {
	if wanted == "" {
		return true
	}
	rank := map[string]int{compat.RoleReader: 1, compat.RoleWriter: 2, compat.RoleOwner: 3}
	return rank[role] >= rank[wanted]
}

// googleEvents responde events.list. Aplica de verdad `timeMin`/`timeMax`
// (obligatorios), `maxResults`, `pageToken`, `singleEvents` y `orderBy=startTime`;
// cualquier otro parámetro es 400 nombrándolo (nunca una agenda filtrada a medias).
//
// singleEvents=true expande las series con el MISMO motor que CalDAV y free/busy;
// sin él se devuelve la serie sin expandir (el maestro con su RRULE), como Google.
func (h *compatHandlers) googleEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := compat.CheckParams(q, "timeMin", "timeMax", "maxResults", "pageToken", "singleEvents", "orderBy"); err != nil {
		failGoogle(w, r, err)
		return
	}
	if o := strings.TrimSpace(q.Get("orderBy")); o != "" && o != "startTime" {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("orderBy only accepts startTime (events are always returned by start time)."))
		return
	}
	offset, err := compat.DecodePageToken(q.Get("pageToken"))
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	from, okFrom := parseCompatTime(q.Get("timeMin"))
	to, okTo := parseCompatTime(q.Get("timeMax"))
	if !okFrom || !okTo {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("timeMin and timeMax must be RFC 3339 dates."))
		return
	}
	if err := h.compatWindow(from, to, "timeMin and timeMax (RFC 3339)"); err != nil {
		failGoogle(w, r, err)
		return
	}

	cal, err := h.resolveCalendar(r, chi.URLParam(r, "calendarId"))
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	expand, err := compatBool(q.Get("singleEvents"))
	if err != nil {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("singleEvents must be true or false."))
		return
	}
	items, err := h.googleItems(r.Context(), cal.ID, from, to, expand)
	if err != nil {
		failGoogle(w, r, err)
		return
	}

	page, next := compat.Slice(items, offset, compat.PageSize(q.Get("maxResults"), googleDefaultPage, googleMaxPage))
	token := ""
	if next > 0 {
		token = compat.EncodePageToken(next)
	}
	httpx.JSON(w, r, http.StatusOK, compat.NewGoogleEvents(cal, h.accessRole(r, cal), page, token))
}

// googleItems lee los eventos del calendario en la ventana: ocurrencias expandidas
// si el cliente pidió singleEvents, o las series sin expandir si no.
func (h *compatHandlers) googleItems(ctx context.Context, calID uuid.UUID, from, to time.Time, expand bool) ([]compat.GoogleEvent, error) {
	if expand {
		insts, err := h.svc.ListInstances(ctx, calID, from, to)
		if err != nil {
			return nil, err
		}
		return compat.GoogleFromInstances(insts), nil
	}
	evts, err := h.svc.ListEventsInRange(ctx, calID, from, to)
	if err != nil {
		return nil, err
	}
	return compat.GoogleFromEvents(evts), nil
}

// googleEvent responde events.get. Acepta el id de un evento almacenado (UUID) y
// el id compuesto de una OCURRENCIA (`{eventId}_{YYYYMMDDThhmmssZ}`), que se
// resuelve expandiendo la serie en ese instante: si la ocurrencia no existe (o fue
// excluida por EXDATE), 404 — nunca una ocurrencia inventada.
func (h *compatHandlers) googleEvent(w http.ResponseWriter, r *http.Request) {
	cal, err := h.resolveCalendar(r, chi.URLParam(r, "calendarId"))
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	raw := chi.URLParam(r, "eventId")
	if masterID, start, ok := compat.SplitInstanceID(raw); ok {
		inst, master, err := h.occurrence(r.Context(), cal.ID, masterID, start)
		if err != nil {
			failGoogle(w, r, err)
			return
		}
		httpx.JSON(w, r, http.StatusOK, compat.GoogleFromInstance(inst, master))
		return
	}
	ev, err := h.compatEvent(r.Context(), cal.ID, raw)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, compat.GoogleFromEvent(ev))
}

// compatEvent resuelve un evento por id a través del servicio (verja de lectura) y
// comprueba que pertenece al calendario de la ruta; si no, 404 como cualquier otro
// id que el usuario no puede ver.
func (h *compatHandlers) compatEvent(ctx context.Context, calID uuid.UUID, id string) (domain.Event, error) {
	eid, err := uuid.Parse(id)
	if err != nil {
		return domain.Event{}, domain.ErrNotFound
	}
	ev, err := h.svc.GetEvent(ctx, eid)
	if err != nil {
		return domain.Event{}, err
	}
	if ev.CalendarID != calID {
		return domain.Event{}, domain.ErrNotFound
	}
	return ev, nil
}

// occurrence resuelve UNA ocurrencia concreta de una serie: carga el maestro (con
// su verja) y expande la serie en una ventana mínima alrededor del instante pedido,
// devolviendo la ocurrencia REAL o 404 si no la hay.
func (h *compatHandlers) occurrence(ctx context.Context, calID uuid.UUID, masterID string, start time.Time) (domain.Instance, string, error) {
	master, err := h.compatEvent(ctx, calID, masterID)
	if err != nil {
		return domain.Instance{}, "", err
	}
	insts, err := h.svc.ListInstances(ctx, calID, start, start.Add(time.Second))
	if err != nil {
		return domain.Instance{}, "", err
	}
	for _, in := range insts {
		if in.Event.UID == master.UID && in.Start.Equal(start) {
			return in, master.ID.String(), nil
		}
	}
	return domain.Instance{}, "", domain.ErrNotFound
}

// googleCreateEvent responde events.insert sobre el MISMO caso de uso que la API
// nativa (misma validación, mismo iTIP, misma verja de escritura: un calendario
// ajeno responde 404 antes de mirar el cuerpo). `sendUpdates=all` entrega las
// invitaciones; `externalOnly` no está implementado (Sytue no distingue invitados
// externos en la entrega) y se rechaza en vez de notificar de más.
func (h *compatHandlers) googleCreateEvent(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := compat.CheckParams(q, "sendUpdates", "sendNotifications"); err != nil {
		failGoogle(w, r, err)
		return
	}
	send, err := sendUpdates(q.Get("sendUpdates"), q.Get("sendNotifications"))
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	cal, err := h.resolveCalendar(r, chi.URLParam(r, "calendarId"))
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	var req compat.GoogleEventRequest
	if err := decodeCompatJSON(w, r, &req); err != nil {
		failGoogle(w, r, err)
		return
	}
	f, err := req.Normalize()
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	in := service.EventInput{
		CalendarID:     cal.ID,
		UID:            f.UID,
		Summary:        f.Summary,
		Description:    f.Description,
		Location:       f.Location,
		Organizer:      f.Organizer,
		DTStart:        f.Start,
		DTEnd:          f.End,
		TZID:           f.TZID,
		AllDay:         f.AllDay,
		Status:         f.Status,
		Classification: f.Classification,
		Transparency:   f.Transparency,
		RRule:          f.RRule,
		Attendees:      f.Attendees,
		Send:           send,
	}
	if in.Organizer == "" {
		in.Organizer = userEmail(r)
	}
	ev, err := h.svc.CreateEvent(r.Context(), in)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, compat.GoogleFromEvent(ev))
}

// googleUpdateEvent responde events.update: el cuerpo REEMPLAZA el evento entero.
func (h *compatHandlers) googleUpdateEvent(w http.ResponseWriter, r *http.Request) {
	h.googleWriteEvent(w, r, false)
}

// googlePatchEvent responde events.patch: solo cambia lo que trae el cuerpo; lo demás
// se queda como estaba.
func (h *compatHandlers) googlePatchEvent(w http.ResponseWriter, r *http.Request) {
	h.googleWriteEvent(w, r, true)
}

// googleWriteEvent es update y patch sobre el MISMO UpdateEvent de la API nativa
// (verja de escritura, SEQUENCE y iTIP incluidos). El UID no cambia nunca: es la
// identidad del evento para CalDAV y para los invitados.
//
// ponytail: una ocurrencia suelta de una serie (`{id}_{instante}`) no se edita por
// aquí: exige crear la excepción con RECURRENCE-ID, que la API nativa hace por
// AmendRecurrence. Se rechaza con 400 en vez de reescribir la serie entera.
func (h *compatHandlers) googleWriteEvent(w http.ResponseWriter, r *http.Request, parche bool) {
	q := r.URL.Query()
	if err := compat.CheckParams(q, "sendUpdates", "sendNotifications"); err != nil {
		failGoogle(w, r, err)
		return
	}
	send, err := sendUpdates(q.Get("sendUpdates"), q.Get("sendNotifications"))
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	cal, err := h.resolveCalendar(r, chi.URLParam(r, "calendarId"))
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	actual, err := h.compatWritableEvent(r, cal.ID)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	var req compat.GoogleEventRequest
	if parche {
		req, err = compat.PatchGoogleEvent(actual, r.Body)
	} else {
		err = decodeCompatJSON(w, r, &req)
	}
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	f, err := req.Normalize()
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	if f.UID != "" && f.UID != actual.UID {
		failGoogle(w, r, httpx.ErrBadRequest.WithMessage("iCalUID cannot change: it is the identity of the event for its guests."))
		return
	}
	in := service.EventInput{
		CalendarID: cal.ID, UID: actual.UID, Summary: f.Summary, Description: f.Description,
		Location: f.Location, Organizer: f.Organizer, DTStart: f.Start, DTEnd: f.End, TZID: f.TZID,
		AllDay: f.AllDay, Status: f.Status, Classification: f.Classification, Transparency: f.Transparency,
		RRule: f.RRule, Attendees: f.Attendees, Send: send,
	}
	if in.Organizer == "" {
		in.Organizer = actual.Organizer
	}
	ev, err := h.svc.UpdateEvent(r.Context(), in)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, compat.GoogleFromEvent(ev))
}

// googleDeleteEvent responde events.delete. Como en Google, a los invitados solo se
// les avisa con `sendUpdates=all` (o el antiguo sendNotifications=true).
func (h *compatHandlers) googleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := compat.CheckParams(q, "sendUpdates", "sendNotifications"); err != nil {
		failGoogle(w, r, err)
		return
	}
	send, err := sendUpdates(q.Get("sendUpdates"), q.Get("sendNotifications"))
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	cal, err := h.resolveCalendar(r, chi.URLParam(r, "calendarId"))
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	actual, err := h.compatWritableEvent(r, cal.ID)
	if err != nil {
		failGoogle(w, r, err)
		return
	}
	if err := h.svc.DeleteEventNotifying(r.Context(), actual.ID, send); err != nil {
		failGoogle(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// compatWritableEvent resuelve el evento de la ruta para escribirlo: una ocurrencia
// suelta de una serie es 400 (ver googleWriteEvent), un id ajeno es 404.
func (h *compatHandlers) compatWritableEvent(r *http.Request, calID uuid.UUID) (domain.Event, error) {
	raw := chi.URLParam(r, "eventId")
	if _, _, ok := compat.SplitInstanceID(raw); ok {
		return domain.Event{}, httpx.ErrBadRequest.WithMessage(
			"A single occurrence of a recurring event cannot be modified through this emulation; change the series or use the native /api/v1 API.")
	}
	return h.compatEvent(r.Context(), calID, raw)
}

// sendUpdates traduce sendUpdates/sendNotifications a «entregar invitaciones».
func sendUpdates(updates, notifications string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(updates)) {
	case "", "none":
		return strings.EqualFold(strings.TrimSpace(notifications), "true"), nil
	case "all":
		return true, nil
	default:
		return false, httpx.ErrBadRequest.WithMessage(
			"sendUpdates accepts none or all; externalOnly is not implemented (Sytue does not tell external guests apart when delivering invitations).",
		)
	}
}

// decodeCompatJSON decodifica el cuerpo del dialecto. Si trae un campo que la
// emulación NO interpreta, lo dice por su nombre (el decodificador rechaza los
// campos desconocidos): guardar un evento a medias de lo que el cliente mandó
// —sin sus recordatorios, sin su conferencia— sería peor que fallar.
func decodeCompatJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	err := httpx.DecodeJSON(w, r, dst)
	if err == nil {
		return nil
	}
	if _, field, ok := strings.Cut(err.Error(), "unknown field "); ok {
		return httpx.ErrBadRequest.WithMessage("The body carries a field the emulation does not understand: " +
			field + ". Remove it or use the native /api/v1 API.")
	}
	return err
}

// compatBool lee un booleano de la query; ausente es false. Un valor que no es un
// booleano es un error (interpretarlo como false cambiaría en silencio el modo del
// listado, y el cliente daría la respuesta por buena).
func compatBool(v string) (bool, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return false, nil
	}
	return strconv.ParseBool(v)
}

// parseCompatTime lee una fecha RFC 3339 de la query.
func parseCompatTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, true // ausente: lo decide quien valida la ventana.
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// ---------------------------------------------------------------------------
// Microsoft Graph (calendar) — /compat/graph
// ---------------------------------------------------------------------------

// graphCalendars responde GET /v1.0/me/calendars con paginación OData ($top/$skip)
// y @odata.nextLink absoluto.
func (h *compatHandlers) graphCalendars(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := compat.CheckParams(q, "$top", "$skip"); err != nil {
		failGraph(w, r, err)
		return
	}
	cals, roles, err := h.listCalendars(r)
	if err != nil {
		failGraph(w, r, err)
		return
	}
	// El owner solo se emite cuando ES el llamante: el correo del dueño de un
	// calendario compartido no viaja en la lectura, y no se inventa.
	out := make([]compat.GraphCalendar, 0, len(cals))
	for i, c := range cals {
		owner := ""
		if roles[i] == compat.RoleOwner {
			owner = userEmail(r)
		}
		out = append(out, compat.GraphFromCalendar(c, owner, roles[i] != compat.RoleReader))
	}

	top := compat.PageSize(q.Get("$top"), graphDefaultPage, graphMaxPage)
	skip := compat.PageSize(q.Get("$skip"), 0, graphMaxSkip)
	page, next := compat.Slice(out, skip, top)
	link := compat.NextLink(h.cfg.PublicURL, r.URL.Path, q, top, next)
	httpx.JSON(w, r, http.StatusOK, compat.NewGraphCollection(page, link))
}

// graphEvents responde GET /v1.0/me/events y /v1.0/me/calendars/{id}/events. La
// ventana temporal viaja en `$filter` (subconjunto documentado en compat) y es
// OBLIGATORIA. Graph NO expande series aquí (eso es calendarView, no emulado): los
// maestros salen con type=seriesMaster.
func (h *compatHandlers) graphEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := compat.CheckParams(q, "$top", "$skip", "$filter"); err != nil {
		failGraph(w, r, err)
		return
	}
	from, to, err := compat.ParseGraphDateFilter(q.Get("$filter"))
	if err != nil {
		failGraph(w, r, err)
		return
	}
	if err := h.compatWindow(from, to, "$filter=start/dateTime ge '…' and end/dateTime le '…'"); err != nil {
		failGraph(w, r, err)
		return
	}
	cal, err := h.resolveCalendar(r, chi.URLParam(r, "calendarId"))
	if err != nil {
		failGraph(w, r, err)
		return
	}
	evts, err := h.svc.ListEventsInRange(r.Context(), cal.ID, from, to)
	if err != nil {
		failGraph(w, r, err)
		return
	}
	out := compat.GraphFromEvents(evts)

	top := compat.PageSize(q.Get("$top"), graphDefaultPage, graphMaxPage)
	skip := compat.PageSize(q.Get("$skip"), 0, graphMaxSkip)
	page, next := compat.Slice(out, skip, top)
	link := compat.NextLink(h.cfg.PublicURL, r.URL.Path, q, top, next)
	httpx.JSON(w, r, http.StatusOK, compat.NewGraphCollection(page, link))
}

// graphEvent responde GET /v1.0/me/events/{id} (y su variante bajo un calendario).
func (h *compatHandlers) graphEvent(w http.ResponseWriter, r *http.Request) {
	cal, err := h.resolveCalendar(r, chi.URLParam(r, "calendarId"))
	if err != nil {
		failGraph(w, r, err)
		return
	}
	ev, err := h.compatEvent(r.Context(), cal.ID, chi.URLParam(r, "eventId"))
	if err != nil {
		failGraph(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, compat.GraphFromEvent(ev))
}
