// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package compat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// Los tags JSON de este fichero son el contrato LITERAL de la API de Google
// Calendar v3 (calendar#event, dateTime/timeZone, recurringEventId…): el cliente
// que apunta a Sytue cree hablar con Google, así que renombrarlos a snake_case
// rompería la emulación entera.

// dateLayout es el formato de los eventos de día completo en Google (start.date).
const dateLayout = "2006-01-02"

// GoogleEventDateTime es el par start/end de Google: `date` para los eventos de
// DÍA COMPLETO y `dateTime`+`timeZone` para los que tienen hora. Esa distinción es
// la que usan las librerías para decidir cómo pintar el evento.
//
//nolint:tagliatelle // Contrato literal de Google Calendar v3.
type GoogleEventDateTime struct {
	Date     string `json:"date,omitempty"`
	DateTime string `json:"dateTime,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

// GoogleAttendee es un asistente en el dialecto de Google.
//
//nolint:tagliatelle // Contrato literal de Google Calendar v3.
type GoogleAttendee struct {
	Email          string `json:"email"`
	DisplayName    string `json:"displayName,omitempty"`
	Optional       bool   `json:"optional,omitempty"`
	Organizer      bool   `json:"organizer,omitempty"`
	ResponseStatus string `json:"responseStatus,omitempty"`
}

// GooglePerson es el organizador/creador de un evento.
//
//nolint:tagliatelle // Contrato literal de Google Calendar v3.
type GooglePerson struct {
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

// GoogleEvent es el recurso `events` de Google Calendar v3 con el subconjunto de
// campos que las librerías cliente leen de verdad.
//
//nolint:tagliatelle // Contrato literal de Google Calendar v3.
type GoogleEvent struct {
	Kind              string               `json:"kind"`
	ID                string               `json:"id"`
	Status            string               `json:"status,omitempty"`
	Summary           string               `json:"summary,omitempty"`
	Description       string               `json:"description,omitempty"`
	Location          string               `json:"location,omitempty"`
	Created           string               `json:"created,omitempty"`
	Updated           string               `json:"updated,omitempty"`
	Start             *GoogleEventDateTime `json:"start,omitempty"`
	End               *GoogleEventDateTime `json:"end,omitempty"`
	Recurrence        []string             `json:"recurrence,omitempty"`
	RecurringEventID  string               `json:"recurringEventId,omitempty"`
	OriginalStartTime *GoogleEventDateTime `json:"originalStartTime,omitempty"`
	Organizer         *GooglePerson        `json:"organizer,omitempty"`
	Attendees         []GoogleAttendee     `json:"attendees,omitempty"`
	Transparency      string               `json:"transparency,omitempty"`
	Visibility        string               `json:"visibility,omitempty"`
	ICalUID           string               `json:"iCalUID,omitempty"`
	Sequence          int                  `json:"sequence,omitempty"`
}

// GoogleEvents es la respuesta de events.list.
//
//nolint:tagliatelle // Contrato literal de Google Calendar v3.
type GoogleEvents struct {
	Kind          string        `json:"kind"`
	Summary       string        `json:"summary,omitempty"`
	TimeZone      string        `json:"timeZone,omitempty"`
	AccessRole    string        `json:"accessRole,omitempty"`
	NextPageToken string        `json:"nextPageToken,omitempty"`
	Items         []GoogleEvent `json:"items"`
}

// GoogleCalendarListEntry es una entrada de calendarList.
//
//nolint:tagliatelle // Contrato literal de Google Calendar v3.
type GoogleCalendarListEntry struct {
	Kind            string `json:"kind"`
	ID              string `json:"id"`
	Summary         string `json:"summary"`
	Description     string `json:"description,omitempty"`
	TimeZone        string `json:"timeZone,omitempty"`
	BackgroundColor string `json:"backgroundColor,omitempty"`
	AccessRole      string `json:"accessRole"`
	Primary         bool   `json:"primary,omitempty"`
	Selected        bool   `json:"selected,omitempty"`
}

// GoogleCalendarList es la respuesta de calendarList.list.
//
//nolint:tagliatelle // Contrato literal de Google Calendar v3.
type GoogleCalendarList struct {
	Kind          string                    `json:"kind"`
	NextPageToken string                    `json:"nextPageToken,omitempty"`
	Items         []GoogleCalendarListEntry `json:"items"`
}

// Roles de acceso de calendarList, en el vocabulario de Google.
const (
	RoleOwner  = "owner"
	RoleWriter = "writer"
	RoleReader = "reader"
)

// GoogleAccessRole traduce el permiso de un calendario compartido al accessRole de
// Google. `manage` (delegación de agenda) es un writer a ojos del cliente.
func GoogleAccessRole(p domain.SharePermission) string {
	if domain.CanWrite(p) {
		return RoleWriter
	}
	return RoleReader
}

// GoogleFromCalendar traduce un calendario de Sytue a una entrada de calendarList.
// El accessRole viene dado por quien llama (dueño o permiso del share): el
// traductor no decide permisos.
func GoogleFromCalendar(c domain.Calendar, accessRole string) GoogleCalendarListEntry {
	return GoogleCalendarListEntry{
		Kind:            "calendar#calendarListEntry",
		ID:              c.ID.String(),
		Summary:         calendarTitle(c),
		Description:     c.Description,
		TimeZone:        c.Timezone,
		BackgroundColor: c.Color,
		AccessRole:      accessRole,
		Primary:         c.IsDefault,
		Selected:        true,
	}
}

// calendarTitle devuelve el nombre legible del calendario (display_name si lo hay).
func calendarTitle(c domain.Calendar) string {
	if s := strings.TrimSpace(c.DisplayName); s != "" {
		return s
	}
	return c.Name
}

// NewGoogleCalendarList arma la respuesta de calendarList.list (items nunca nulo:
// las librerías iteran el array sin comprobarlo).
func NewGoogleCalendarList(items []GoogleCalendarListEntry, nextPageToken string) GoogleCalendarList {
	if items == nil {
		items = []GoogleCalendarListEntry{}
	}
	return GoogleCalendarList{Kind: "calendar#calendarList", Items: items, NextPageToken: nextPageToken}
}

// NewGoogleEvents arma la respuesta de events.list.
func NewGoogleEvents(cal domain.Calendar, accessRole string, items []GoogleEvent, nextPageToken string) GoogleEvents {
	if items == nil {
		items = []GoogleEvent{}
	}
	return GoogleEvents{
		Kind:          "calendar#events",
		Summary:       calendarTitle(cal),
		TimeZone:      cal.Timezone,
		AccessRole:    accessRole,
		Items:         items,
		NextPageToken: nextPageToken,
	}
}

// GoogleFromEvents traduce una lista de eventos almacenados SIN expandir (Google
// sin singleEvents=true). Resuelve por UID el `recurringEventId` de los overrides,
// que es el único dato de la serie que la fila del override no lleva.
func GoogleFromEvents(evts []domain.Event) []GoogleEvent {
	masters := make(map[string]string, len(evts))
	for _, ev := range evts {
		if ev.RecurrenceID == nil && ev.IsRecurring() {
			masters[ev.UID] = ev.ID.String()
		}
	}
	out := make([]GoogleEvent, 0, len(evts))
	for _, ev := range evts {
		g := GoogleFromEvent(ev)
		if ev.RecurrenceID != nil {
			g.RecurringEventID = masters[ev.UID]
		}
		out = append(out, g)
	}
	return out
}

// GoogleFromEvent traduce un evento almacenado (maestro u override) al recurso
// calendar#event SIN expandir: si es una serie, viaja con su `recurrence`
// ["RRULE:…"], igual que Google sin singleEvents=true.
func GoogleFromEvent(ev domain.Event) GoogleEvent {
	g := baseGoogleEvent(ev)
	g.Start = googleDateTime(ev.DTStart, ev.TZID, ev.AllDay)
	g.End = googleDateTime(ev.DTEnd, ev.TZID, ev.AllDay)
	if ev.RRule != "" {
		g.Recurrence = []string{"RRULE:" + ev.RRule}
	}
	if ev.RecurrenceID != nil {
		g.OriginalStartTime = googleDateTime(*ev.RecurrenceID, ev.TZID, ev.AllDay)
	}
	return g
}

// GoogleFromInstances traduce una lista de OCURRENCIAS expandidas (singleEvents=
// true). El id de cada ocurrencia es el compuesto de Google
// (`{eventId}_{YYYYMMDDThhmmssZ}`, `{eventId}_{YYYYMMDD}` si es de día completo) y
// `recurringEventId` apunta al maestro de la serie, que se resuelve por UID entre
// las propias instancias (un override no lleva el id de su maestro).
func GoogleFromInstances(insts []domain.Instance) []GoogleEvent {
	masters := make(map[string]string, len(insts))
	for _, in := range insts {
		if !in.IsOverride && in.Event.IsRecurring() {
			masters[in.Event.UID] = in.Event.ID.String()
		}
	}
	out := make([]GoogleEvent, 0, len(insts))
	for _, in := range insts {
		out = append(out, GoogleFromInstance(in, masters[in.Event.UID]))
	}
	return out
}

// GoogleFromInstance traduce UNA ocurrencia expandida. masterID es el id del
// evento maestro de la serie ("" si la ocurrencia no pertenece a ninguna, o si no
// se conoce: entonces se omite `recurringEventId` en vez de inventarlo).
func GoogleFromInstance(in domain.Instance, masterID string) GoogleEvent {
	ev := in.Event
	g := baseGoogleEvent(ev)
	g.Start = googleDateTime(in.Start, ev.TZID, ev.AllDay)
	g.End = googleDateTime(in.End, ev.TZID, ev.AllDay)

	switch {
	case in.IsOverride:
		// El override tiene fila propia: su id es el suyo y su arranque original es
		// el RECURRENCE-ID (el hueco de la serie que sustituye).
		g.RecurringEventID = masterID
		if ev.RecurrenceID != nil {
			g.OriginalStartTime = googleDateTime(*ev.RecurrenceID, ev.TZID, ev.AllDay)
		}
	case ev.IsRecurring():
		g.ID = InstanceID(ev.ID.String(), in.Start, ev.AllDay)
		g.RecurringEventID = ev.ID.String()
		g.OriginalStartTime = googleDateTime(in.Start, ev.TZID, ev.AllDay)
	}
	return g
}

// baseGoogleEvent traduce los campos comunes a maestro y ocurrencia.
func baseGoogleEvent(ev domain.Event) GoogleEvent {
	g := GoogleEvent{
		Kind:         "calendar#event",
		ID:           ev.ID.String(),
		Status:       strings.ToLower(string(ev.Status)),
		Summary:      ev.Summary,
		Description:  ev.Description,
		Location:     ev.Location,
		Created:      rfc3339(ev.CreatedAt),
		Updated:      rfc3339(ev.UpdatedAt),
		Transparency: strings.ToLower(string(ev.Transparency)),
		Visibility:   googleVisibility(ev.Classification),
		ICalUID:      ev.UID,
		Sequence:     ev.SequenceNo,
	}
	if ev.Organizer != "" {
		g.Organizer = &GooglePerson{Email: ev.Organizer}
	}
	g.Attendees = googleAttendees(ev.Attendees, ev.Organizer)
	return g
}

// googleAttendees traduce los ATTENDEE al dialecto de Google.
func googleAttendees(atts []domain.Attendee, organizer string) []GoogleAttendee {
	if len(atts) == 0 {
		return nil
	}
	out := make([]GoogleAttendee, 0, len(atts))
	for _, a := range atts {
		out = append(out, GoogleAttendee{
			Email:          a.Email,
			DisplayName:    a.CommonName,
			Optional:       a.Role == domain.RoleOptional,
			Organizer:      organizer != "" && strings.EqualFold(a.Email, organizer),
			ResponseStatus: googleResponseStatus(a.PartStat),
		})
	}
	return out
}

// googleResponseStatus traduce el PARTSTAT (RFC 5545) al responseStatus de Google.
// DELEGATED no existe en Google: se emite como needsAction («aún no ha respondido»),
// que es lo cierto desde el punto de vista del organizador.
func googleResponseStatus(p domain.ParticipationStatus) string {
	switch p {
	case domain.PartAccepted:
		return "accepted"
	case domain.PartDeclined:
		return "declined"
	case domain.PartTentative:
		return "tentative"
	case domain.PartNeedsAction, domain.PartDelegated:
		return "needsAction"
	default:
		return "needsAction"
	}
}

// googleVisibility traduce el CLASS de iCalendar a la visibility de Google.
func googleVisibility(c domain.Classification) string {
	switch c {
	case domain.ClassPrivate:
		return "private"
	case domain.ClassConfidential:
		return "confidential"
	case domain.ClassPublic:
		return "public"
	default:
		return ""
	}
}

// googleDateTime traduce un instante: `date` (día natural) si el evento es de día
// completo, o `dateTime`+`timeZone` en la zona del evento si tiene hora.
func googleDateTime(t time.Time, tzid string, allDay bool) *GoogleEventDateTime {
	if t.IsZero() {
		return nil
	}
	if allDay {
		return &GoogleEventDateTime{Date: t.UTC().Format(dateLayout)}
	}
	loc := domain.EventLocation(tzid, false)
	return &GoogleEventDateTime{
		DateTime: t.In(loc).Format(time.RFC3339),
		TimeZone: orUTC(tzid),
	}
}

// orUTC devuelve el TZID o "UTC" si el evento no lleva zona.
func orUTC(tzid string) string {
	if tzid == "" {
		return "UTC"
	}
	return tzid
}

// InstanceID compone el id de ocurrencia de Google: el id del maestro, un guion
// bajo y el arranque original (`YYYYMMDD` si es de día completo, `YYYYMMDDThhmmssZ`
// si tiene hora). Es el formato que las librerías parten para volver a la serie.
func InstanceID(masterID string, start time.Time, allDay bool) string {
	if allDay {
		return masterID + "_" + start.UTC().Format("20060102")
	}
	return masterID + "_" + start.UTC().Format("20060102T150405Z")
}

// SplitInstanceID parte un id de ocurrencia en (id del maestro, arranque). ok=false
// si el id no es compuesto (es un evento suelto, no una ocurrencia).
func SplitInstanceID(id string) (masterID string, start time.Time, ok bool) {
	i := strings.LastIndexByte(id, '_')
	if i < 0 {
		return "", time.Time{}, false
	}
	suffix := id[i+1:]
	for _, layout := range []string{"20060102T150405Z", "20060102"} {
		if t, err := time.Parse(layout, suffix); err == nil {
			return id[:i], t.UTC(), true
		}
	}
	return "", time.Time{}, false
}

// ---------------------------------------------------------------------------
// Cuerpo de events.insert
// ---------------------------------------------------------------------------

// GoogleEventRequest es el cuerpo de events.insert que la emulación interpreta.
//
//nolint:tagliatelle // Contrato literal de Google Calendar v3.
type GoogleEventRequest struct {
	Summary      string               `json:"summary"`
	Description  string               `json:"description"`
	Location     string               `json:"location"`
	Start        *GoogleEventDateTime `json:"start"`
	End          *GoogleEventDateTime `json:"end"`
	Attendees    []GoogleAttendee     `json:"attendees"`
	Recurrence   []string             `json:"recurrence"`
	Status       string               `json:"status"`
	Transparency string               `json:"transparency"`
	Visibility   string               `json:"visibility"`
	ICalUID      string               `json:"iCalUID"`
	Organizer    *GooglePerson        `json:"organizer"`
}

// PatchGoogleEvent aplica un cuerpo de events.patch sobre el evento actual y devuelve
// la petición completa resultante. Mezcla por campo de primer nivel, como Google: lo
// que trae el cuerpo sustituye al campo entero (`attendees` es la lista nueva, no una
// suma) y `null` lo vacía. Un campo que la emulación no entiende es un error por su
// nombre, igual que en insert.
func PatchGoogleEvent(actual domain.Event, body io.Reader) (GoogleEventRequest, error) {
	raw, err := io.ReadAll(io.LimitReader(body, maxPatchBytes+1))
	if err != nil {
		return GoogleEventRequest{}, fmt.Errorf("%w: cuerpo ilegible", ErrInvalidEvent)
	}
	if len(raw) > maxPatchBytes {
		return GoogleEventRequest{}, fmt.Errorf("%w: cuerpo demasiado grande", ErrInvalidEvent)
	}
	// Primero, lo que diga el cliente y nada más: un campo desconocido se nombra.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var solo GoogleEventRequest
	if err := dec.Decode(&solo); err != nil {
		if _, campo, ok := strings.Cut(err.Error(), "unknown field "); ok {
			return GoogleEventRequest{}, fmt.Errorf("%w: el cuerpo lleva un campo que la emulación no interpreta: %s", ErrInvalidEvent, campo)
		}
		return GoogleEventRequest{}, fmt.Errorf("%w: JSON inválido", ErrInvalidEvent)
	}
	var cambios map[string]json.RawMessage
	if err := json.Unmarshal(raw, &cambios); err != nil {
		return GoogleEventRequest{}, fmt.Errorf("%w: JSON inválido", ErrInvalidEvent)
	}
	base, err := json.Marshal(GoogleFromEvent(actual))
	if err != nil {
		return GoogleEventRequest{}, err
	}
	var mezcla map[string]json.RawMessage
	if err := json.Unmarshal(base, &mezcla); err != nil {
		return GoogleEventRequest{}, err
	}
	for k, v := range cambios {
		mezcla[k] = v
	}
	junto, err := json.Marshal(mezcla)
	if err != nil {
		return GoogleEventRequest{}, err
	}
	// El resultado lleva además campos de solo lectura (id, created…), que la
	// petición no tiene: aquí se ignoran a propósito.
	var out GoogleEventRequest
	if err := json.Unmarshal(junto, &out); err != nil {
		return GoogleEventRequest{}, fmt.Errorf("%w: %w", ErrInvalidEvent, err)
	}
	return out, nil
}

// maxPatchBytes acota el cuerpo de un patch (un evento con cien invitados cabe de sobra).
const maxPatchBytes = 256 << 10

// EventFields son los campos normalizados de un evento entrante, listos para
// construir la entrada del servicio (que es quien valida el resto y persiste).
type EventFields struct {
	UID            string
	Summary        string
	Description    string
	Location       string
	Organizer      string
	TZID           string
	RRule          string
	Start          time.Time
	End            time.Time
	AllDay         bool
	Status         domain.EventStatus
	Classification domain.Classification
	Transparency   domain.Transparency
	Attendees      []domain.Attendee
}

// Normalize traduce el cuerpo de events.insert a campos de dominio. Todo lo que no
// se sepa traducir es ErrInvalidEvent con el campo exacto: nunca se persiste un
// evento «a medias» de lo que el cliente pidió.
func (r GoogleEventRequest) Normalize() (EventFields, error) {
	start, end, allDay, tz, err := normalizeGoogleRange(r.Start, r.End)
	if err != nil {
		return EventFields{}, err
	}
	rrule, err := normalizeRecurrence(r.Recurrence)
	if err != nil {
		return EventFields{}, err
	}
	status, err := googleStatusToDomain(r.Status)
	if err != nil {
		return EventFields{}, err
	}
	transp, err := googleTranspToDomain(r.Transparency)
	if err != nil {
		return EventFields{}, err
	}
	class, err := googleVisibilityToDomain(r.Visibility)
	if err != nil {
		return EventFields{}, err
	}

	f := EventFields{
		UID:            strings.TrimSpace(r.ICalUID),
		Summary:        r.Summary,
		Description:    r.Description,
		Location:       r.Location,
		TZID:           tz,
		RRule:          rrule,
		Start:          start,
		End:            end,
		AllDay:         allDay,
		Status:         status,
		Classification: class,
		Transparency:   transp,
	}
	if r.Organizer != nil {
		f.Organizer = strings.TrimSpace(r.Organizer.Email)
	}
	f.Attendees = attendeesToDomain(r.Attendees)
	return f, nil
}

// normalizeGoogleRange interpreta start/end: `date` (día completo, fin EXCLUSIVO
// como en iCalendar y en Google) o `dateTime`+`timeZone`. Mezclar ambas formas
// entre start y end es un error explícito.
func normalizeGoogleRange(start, end *GoogleEventDateTime) (from, to time.Time, allDay bool, tzid string, err error) {
	if start == nil || end == nil {
		return time.Time{}, time.Time{}, false, "", fmt.Errorf("%w: se requieren `start` y `end`", ErrInvalidEvent)
	}
	switch {
	case start.Date != "" && end.Date != "":
		from, err = time.Parse(dateLayout, start.Date)
		if err != nil {
			return time.Time{}, time.Time{}, false, "", fmt.Errorf("%w: `start.date` debe ser YYYY-MM-DD", ErrInvalidEvent)
		}
		to, err = time.Parse(dateLayout, end.Date)
		if err != nil {
			return time.Time{}, time.Time{}, false, "", fmt.Errorf("%w: `end.date` debe ser YYYY-MM-DD", ErrInvalidEvent)
		}
		return from.UTC(), to.UTC(), true, "UTC", nil
	case start.DateTime != "" && end.DateTime != "":
		from, err = time.Parse(time.RFC3339, start.DateTime)
		if err != nil {
			return time.Time{}, time.Time{}, false, "", fmt.Errorf("%w: `start.dateTime` debe ser RFC 3339", ErrInvalidEvent)
		}
		to, err = time.Parse(time.RFC3339, end.DateTime)
		if err != nil {
			return time.Time{}, time.Time{}, false, "", fmt.Errorf("%w: `end.dateTime` debe ser RFC 3339", ErrInvalidEvent)
		}
		tzid = strings.TrimSpace(start.TimeZone)
		if tzid == "" {
			tzid = strings.TrimSpace(end.TimeZone)
		}
		return from.UTC(), to.UTC(), false, orUTC(tzid), nil
	default:
		return time.Time{}, time.Time{}, false, "", fmt.Errorf(
			"%w: `start` y `end` deben usar los DOS `date` (día completo) o los DOS `dateTime`", ErrInvalidEvent,
		)
	}
}

// normalizeRecurrence acepta UNA línea RRULE. EXDATE/RDATE/EXRULE no están
// implementados: se rechazan nombrando la línea en vez de guardar la serie sin
// sus excepciones (que sería una agenda con eventos que no existen).
func normalizeRecurrence(lines []string) (string, error) {
	var rrule string
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		value, ok := strings.CutPrefix(strings.ToUpper(line), "RRULE:")
		if !ok {
			return "", fmt.Errorf("%w: `recurrence` solo admite una línea RRULE (recibido %q)", ErrInvalidEvent, line)
		}
		if rrule != "" {
			return "", fmt.Errorf("%w: `recurrence` con más de un RRULE no está implementado", ErrInvalidEvent)
		}
		rrule = value
	}
	return rrule, nil
}

// googleStatusToDomain traduce el status entrante al STATUS de iCalendar.
func googleStatusToDomain(s string) (domain.EventStatus, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return "", nil
	case "confirmed":
		return domain.StatusConfirmed, nil
	case "tentative":
		return domain.StatusTentative, nil
	case "cancelled":
		return domain.StatusCancelled, nil
	default:
		return "", fmt.Errorf("%w: `status` debe ser confirmed, tentative o cancelled", ErrInvalidEvent)
	}
}

// googleTranspToDomain traduce transparency al TRANSP de iCalendar.
func googleTranspToDomain(s string) (domain.Transparency, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return "", nil
	case "opaque":
		return domain.TranspOpaque, nil
	case "transparent":
		return domain.TranspTransparent, nil
	default:
		return "", fmt.Errorf("%w: `transparency` debe ser opaque o transparent", ErrInvalidEvent)
	}
}

// googleVisibilityToDomain traduce visibility al CLASS de iCalendar. `default`
// deja que el evento herede la clasificación por defecto del servicio.
func googleVisibilityToDomain(s string) (domain.Classification, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "default":
		return "", nil
	case "public":
		return domain.ClassPublic, nil
	case "private":
		return domain.ClassPrivate, nil
	case "confidential":
		return domain.ClassConfidential, nil
	default:
		return "", fmt.Errorf("%w: `visibility` debe ser default, public, private o confidential", ErrInvalidEvent)
	}
}

// attendeesToDomain traduce los asistentes entrantes a ATTENDEE de dominio.
func attendeesToDomain(atts []GoogleAttendee) []domain.Attendee {
	if len(atts) == 0 {
		return nil
	}
	out := make([]domain.Attendee, 0, len(atts))
	for _, a := range atts {
		email := strings.TrimSpace(a.Email)
		if email == "" {
			continue
		}
		role := domain.RoleRequired
		if a.Optional {
			role = domain.RoleOptional
		}
		out = append(out, domain.Attendee{
			Email:      email,
			CommonName: a.DisplayName,
			Role:       role,
			PartStat:   partStatFromGoogle(a.ResponseStatus),
			RSVP:       true,
		})
	}
	return out
}

// partStatFromGoogle traduce responseStatus al PARTSTAT de iCalendar.
func partStatFromGoogle(s string) domain.ParticipationStatus {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "accepted":
		return domain.PartAccepted
	case "declined":
		return domain.PartDeclined
	case "tentative":
		return domain.PartTentative
	default:
		return domain.PartNeedsAction
	}
}

// ---------------------------------------------------------------------------
// Errores en el dialecto de Google
// ---------------------------------------------------------------------------

// GoogleError es el sobre de error de las APIs de Google.
type GoogleError struct {
	Error GoogleErrorBody `json:"error"`
}

// GoogleErrorBody es el cuerpo del sobre de error.
type GoogleErrorBody struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Errors  []GoogleErrorItem `json:"errors"`
}

// GoogleErrorItem es el detalle por error que leen los clientes para decidir
// reintentos (reason).
type GoogleErrorItem struct {
	Domain  string `json:"domain"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// NewGoogleError traduce un estado HTTP + mensaje al sobre de Google.
func NewGoogleError(status int, message string) GoogleError {
	return GoogleError{Error: GoogleErrorBody{
		Code:    status,
		Message: message,
		Errors: []GoogleErrorItem{{
			Domain:  "global",
			Reason:  googleReason(status),
			Message: message,
		}},
	}}
}

// googleReason mapea el estado HTTP al `reason` que usan las librerías de Google.
func googleReason(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "badRequest"
	case http.StatusUnauthorized:
		return "authError"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "notFound"
	case http.StatusConflict:
		return "duplicate"
	case http.StatusPreconditionFailed:
		return "conditionNotMet"
	case http.StatusTooManyRequests:
		return "rateLimitExceeded"
	case http.StatusNotImplemented:
		return "notImplemented"
	default:
		return "backendError"
	}
}
