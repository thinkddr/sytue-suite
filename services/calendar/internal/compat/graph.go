// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package compat

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// Los tags JSON de este fichero son el contrato LITERAL de Microsoft Graph v1.0
// (event, dateTimeTimeZone, @odata.nextLink…): el cliente que apunta a Sytue cree
// hablar con Microsoft, así que renombrarlos a snake_case rompería la emulación.

// graphTimeLayout es el formato de fecha-hora de Graph: sin offset, con la zona en
// el campo `timeZone` contiguo (7 decimales, como emite el servicio real).
const graphTimeLayout = "2006-01-02T15:04:05.0000000"

// GraphDateTimeTimeZone es el `dateTimeTimeZone` de Graph: el instante SIN offset
// más su zona en un campo aparte.
//
//nolint:tagliatelle // Contrato literal de Microsoft Graph v1.0.
type GraphDateTimeTimeZone struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

// GraphItemBody es el cuerpo de un evento (aquí siempre texto plano: Sytue guarda
// la DESCRIPTION de iCalendar, que no es HTML).
//
//nolint:tagliatelle // Contrato literal de Microsoft Graph v1.0.
type GraphItemBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

// GraphLocation es la ubicación de un evento.
//
//nolint:tagliatelle // Contrato literal de Microsoft Graph v1.0.
type GraphLocation struct {
	DisplayName string `json:"displayName"`
}

// GraphEmailAddress es el par nombre/dirección que Graph usa en organizador y
// asistentes.
//
//nolint:tagliatelle // Contrato literal de Microsoft Graph v1.0.
type GraphEmailAddress struct {
	Name    string `json:"name,omitempty"`
	Address string `json:"address"`
}

// GraphRecipient envuelve una dirección (organizer, attendee).
//
//nolint:tagliatelle // Contrato literal de Microsoft Graph v1.0.
type GraphRecipient struct {
	EmailAddress GraphEmailAddress `json:"emailAddress"`
}

// GraphResponseStatus es la respuesta de un asistente.
//
//nolint:tagliatelle // Contrato literal de Microsoft Graph v1.0.
type GraphResponseStatus struct {
	Response string `json:"response"`
}

// GraphAttendee es un asistente en el dialecto de Graph.
//
//nolint:tagliatelle // Contrato literal de Microsoft Graph v1.0.
type GraphAttendee struct {
	Type         string               `json:"type"`
	Status       *GraphResponseStatus `json:"status,omitempty"`
	EmailAddress GraphEmailAddress    `json:"emailAddress"`
}

// GraphEvent es el recurso `event` de Graph con el subconjunto de campos que
// consumen las librerías. `recurrence` NO se emite: traducir un RRULE a
// `patternedRecurrence` con fidelidad no está implementado y se prefiere omitirlo
// (con `type: seriesMaster`, que sí dice la verdad) antes que inventar un patrón.
//
//nolint:tagliatelle // Contrato literal de Microsoft Graph v1.0.
type GraphEvent struct {
	ID                   string                 `json:"id"`
	Subject              string                 `json:"subject"`
	BodyPreview          string                 `json:"bodyPreview,omitempty"`
	Body                 *GraphItemBody         `json:"body,omitempty"`
	Start                *GraphDateTimeTimeZone `json:"start,omitempty"`
	End                  *GraphDateTimeTimeZone `json:"end,omitempty"`
	Location             *GraphLocation         `json:"location,omitempty"`
	Organizer            *GraphRecipient        `json:"organizer,omitempty"`
	Attendees            []GraphAttendee        `json:"attendees,omitempty"`
	IsAllDay             bool                   `json:"isAllDay"`
	IsCancelled          bool                   `json:"isCancelled"`
	ShowAs               string                 `json:"showAs,omitempty"`
	Sensitivity          string                 `json:"sensitivity,omitempty"`
	Type                 string                 `json:"type"`
	SeriesMasterID       string                 `json:"seriesMasterId,omitempty"`
	ICalUID              string                 `json:"iCalUId,omitempty"`
	CreatedDateTime      string                 `json:"createdDateTime,omitempty"`
	LastModifiedDateTime string                 `json:"lastModifiedDateTime,omitempty"`
}

// GraphCalendar es el recurso `calendar` de Graph.
//
//nolint:tagliatelle // Contrato literal de Microsoft Graph v1.0.
type GraphCalendar struct {
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	HexColor          string             `json:"hexColor,omitempty"`
	IsDefaultCalendar bool               `json:"isDefaultCalendar"`
	CanEdit           bool               `json:"canEdit"`
	CanShare          bool               `json:"canShare"`
	Owner             *GraphEmailAddress `json:"owner,omitempty"`
}

// GraphCollection es una colección OData con su enlace a la página siguiente.
//
//nolint:tagliatelle // Contrato literal de Microsoft Graph v1.0 (OData).
type GraphCollection[T any] struct {
	Value    []T    `json:"value"`
	NextLink string `json:"@odata.nextLink,omitempty"`
}

// NewGraphCollection arma la colección (value nunca nulo: las librerías iteran el
// array sin comprobarlo).
func NewGraphCollection[T any](items []T, nextLink string) GraphCollection[T] {
	if items == nil {
		items = []T{}
	}
	return GraphCollection[T]{Value: items, NextLink: nextLink}
}

// NextLink construye el `@odata.nextLink` ABSOLUTO que Graph exige: el cliente lo
// vuelve a pedir tal cual. skip<=0 ⇒ "" (no hay más páginas). Conserva el resto de
// la query (el $filter con la ventana temporal) para que la página siguiente
// consulte lo mismo.
func NextLink(baseURL, path string, query url.Values, top, skip int) string {
	if skip <= 0 {
		return ""
	}
	v := url.Values{}
	for k, vals := range query {
		if k == "$top" || k == "$skip" {
			continue
		}
		v[k] = vals
	}
	if top > 0 {
		v.Set("$top", strconv.Itoa(top))
	}
	v.Set("$skip", strconv.Itoa(skip))
	return strings.TrimRight(baseURL, "/") + path + "?" + v.Encode()
}

// GraphFromCalendar traduce un calendario de Sytue al recurso calendar de Graph.
// canEdit lo decide quien llama (dueño o permiso del share), no el traductor.
func GraphFromCalendar(c domain.Calendar, ownerEmail string, canEdit bool) GraphCalendar {
	g := GraphCalendar{
		ID:                c.ID.String(),
		Name:              calendarTitle(c),
		HexColor:          c.Color,
		IsDefaultCalendar: c.IsDefault,
		CanEdit:           canEdit && !c.ReadOnly,
	}
	if ownerEmail != "" {
		g.Owner = &GraphEmailAddress{Address: ownerEmail}
	}
	return g
}

// GraphFromEvents traduce una lista de eventos almacenados, resolviendo por UID el
// `seriesMasterId` de las excepciones (el único dato de la serie que la fila de la
// excepción no lleva).
func GraphFromEvents(evts []domain.Event) []GraphEvent {
	masters := make(map[string]string, len(evts))
	for _, ev := range evts {
		if ev.RecurrenceID == nil && ev.IsRecurring() {
			masters[ev.UID] = ev.ID.String()
		}
	}
	out := make([]GraphEvent, 0, len(evts))
	for _, ev := range evts {
		g := GraphFromEvent(ev)
		if ev.RecurrenceID != nil {
			g.SeriesMasterID = masters[ev.UID]
		}
		out = append(out, g)
	}
	return out
}

// GraphFromEvent traduce un evento almacenado al recurso event de Graph. Los
// listados de Graph (/me/events) NO expanden series: un maestro sale con
// `type: "seriesMaster"` y un override con `type: "exception"`.
func GraphFromEvent(ev domain.Event) GraphEvent {
	g := GraphEvent{
		ID:                   ev.ID.String(),
		Subject:              ev.Summary,
		Start:                graphDateTime(ev.DTStart, ev.TZID, ev.AllDay),
		End:                  graphDateTime(ev.DTEnd, ev.TZID, ev.AllDay),
		IsAllDay:             ev.AllDay,
		IsCancelled:          ev.Status == domain.StatusCancelled,
		ShowAs:               graphShowAs(ev),
		Sensitivity:          graphSensitivity(ev.Classification),
		Type:                 graphEventType(ev),
		ICalUID:              ev.UID,
		CreatedDateTime:      rfc3339(ev.CreatedAt),
		LastModifiedDateTime: rfc3339(ev.UpdatedAt),
	}
	if ev.Description != "" {
		g.Body = &GraphItemBody{ContentType: "text", Content: ev.Description}
		g.BodyPreview = ev.Description
	}
	if ev.Location != "" {
		g.Location = &GraphLocation{DisplayName: ev.Location}
	}
	if ev.Organizer != "" {
		g.Organizer = &GraphRecipient{EmailAddress: GraphEmailAddress{Address: ev.Organizer}}
	}
	g.Attendees = graphAttendees(ev.Attendees)
	return g
}

// graphAttendees traduce los ATTENDEE al dialecto de Graph.
func graphAttendees(atts []domain.Attendee) []GraphAttendee {
	if len(atts) == 0 {
		return nil
	}
	out := make([]GraphAttendee, 0, len(atts))
	for _, a := range atts {
		t := "required"
		if a.Role == domain.RoleOptional {
			t = "optional"
		}
		out = append(out, GraphAttendee{
			Type:         t,
			Status:       &GraphResponseStatus{Response: graphResponse(a.PartStat)},
			EmailAddress: GraphEmailAddress{Name: a.CommonName, Address: a.Email},
		})
	}
	return out
}

// graphResponse traduce el PARTSTAT al `response` de Graph. DELEGATED no existe en
// Graph: se emite como notResponded, que es lo cierto para el organizador.
func graphResponse(p domain.ParticipationStatus) string {
	switch p {
	case domain.PartAccepted:
		return "accepted"
	case domain.PartDeclined:
		return "declined"
	case domain.PartTentative:
		return "tentativelyAccepted"
	case domain.PartNeedsAction, domain.PartDelegated:
		return "notResponded"
	default:
		return "notResponded"
	}
}

// graphShowAs traduce el TRANSP a la disponibilidad de Graph.
func graphShowAs(ev domain.Event) string {
	if ev.Transparency == domain.TranspTransparent {
		return "free"
	}
	if ev.Status == domain.StatusTentative {
		return "tentative"
	}
	return "busy"
}

// graphSensitivity traduce el CLASS de iCalendar a la sensitivity de Graph.
func graphSensitivity(c domain.Classification) string {
	switch c {
	case domain.ClassPrivate:
		return "private"
	case domain.ClassConfidential:
		return "confidential"
	case domain.ClassPublic:
		return "normal"
	default:
		return ""
	}
}

// graphEventType clasifica el evento en el vocabulario de Graph.
func graphEventType(ev domain.Event) string {
	switch {
	case ev.RecurrenceID != nil:
		return "exception"
	case ev.IsRecurring():
		return "seriesMaster"
	default:
		return "singleInstance"
	}
}

// graphDateTime traduce un instante al dateTimeTimeZone de Graph. Los eventos de
// día completo viajan a medianoche UTC con fin EXCLUSIVO, como exige Graph.
func graphDateTime(t time.Time, tzid string, allDay bool) *GraphDateTimeTimeZone {
	if t.IsZero() {
		return nil
	}
	if allDay {
		return &GraphDateTimeTimeZone{DateTime: t.UTC().Format(graphTimeLayout), TimeZone: "UTC"}
	}
	loc := domain.EventLocation(tzid, false)
	return &GraphDateTimeTimeZone{DateTime: t.In(loc).Format(graphTimeLayout), TimeZone: orUTC(tzid)}
}

// ---------------------------------------------------------------------------
// $filter (subconjunto: la ventana temporal)
// ---------------------------------------------------------------------------

// ParseGraphDateFilter interpreta el ÚNICO subconjunto de `$filter` que la
// emulación aplica, la ventana temporal:
//
//	start/dateTime ge '2026-07-01T00:00:00Z' and end/dateTime le '2026-08-01T00:00:00Z'
//
// Admite ge/gt sobre start/dateTime (inicio de la ventana) y le/lt sobre
// end/dateTime (fin), unidos por `and`, con el valor entre comillas simples o sin
// ellas. Cualquier otro término es ErrUnsupportedFilter con el término exacto: un
// filtro que no se entiende NO se ignora en silencio.
func ParseGraphDateFilter(filter string) (from, to time.Time, err error) {
	if strings.TrimSpace(filter) == "" {
		return time.Time{}, time.Time{}, nil
	}
	for _, term := range splitAnd(filter) {
		field, op, value, ok := splitFilterTerm(term)
		if !ok {
			return time.Time{}, time.Time{}, unsupportedFilter(term)
		}
		ts, ok := parseTime(strings.Trim(value, "'\""))
		if !ok {
			return time.Time{}, time.Time{}, unsupportedFilter(term)
		}
		switch {
		case strings.EqualFold(field, "start/dateTime") && (op == "ge" || op == "gt"):
			from = ts
		case strings.EqualFold(field, "end/dateTime") && (op == "le" || op == "lt"):
			to = ts
		default:
			return time.Time{}, time.Time{}, unsupportedFilter(term)
		}
	}
	return from, to, nil
}

// splitAnd parte la expresión por el operador `and` de nivel superior, respetando
// las cadenas entre comillas simples.
func splitAnd(filter string) []string {
	var terms []string
	start, inQuote := 0, false
	for i := 0; i < len(filter); i++ {
		switch filter[i] {
		case '\'':
			inQuote = !inQuote
		case 'a', 'A':
			if inQuote || !isWordAt(filter, i, "and") {
				continue
			}
			terms = append(terms, strings.TrimSpace(filter[start:i]))
			start = i + len("and")
			i += len("and") - 1
		}
	}
	return append(terms, strings.TrimSpace(filter[start:]))
}

// splitFilterTerm parte `campo op valor` (OData usa operadores en palabras).
func splitFilterTerm(term string) (field, op, value string, ok bool) {
	parts := strings.Fields(strings.TrimSpace(term))
	if len(parts) < 3 {
		return "", "", "", false
	}
	return parts[0], strings.ToLower(parts[1]), strings.Join(parts[2:], " "), true
}

// isWordAt indica si en s[i:] empieza la palabra w delimitada por espacios.
func isWordAt(s string, i int, w string) bool {
	if i+len(w) > len(s) || !strings.EqualFold(s[i:i+len(w)], w) {
		return false
	}
	if i > 0 && !isSpace(s[i-1]) {
		return false
	}
	return i+len(w) < len(s) && isSpace(s[i+len(w)])
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// unsupportedFilter construye el error con el término exacto que el cliente debe
// quitar.
func unsupportedFilter(term string) error {
	return fmt.Errorf("%w: %q (soportado: start/dateTime ge '…' and end/dateTime le '…')",
		ErrUnsupportedFilter, strings.TrimSpace(term))
}

// ---------------------------------------------------------------------------
// Errores en el dialecto de Graph
// ---------------------------------------------------------------------------

// GraphError es el sobre de error de Microsoft Graph.
type GraphError struct {
	Error GraphErrorBody `json:"error"`
}

// GraphErrorBody es el cuerpo del sobre: `code` es un símbolo, no un número.
type GraphErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewGraphError traduce un estado HTTP + mensaje al sobre de Graph.
func NewGraphError(status int, message string) GraphError {
	return GraphError{Error: GraphErrorBody{Code: graphCode(status), Message: message}}
}

// graphCode mapea el estado HTTP al código simbólico de Graph.
func graphCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalidRequest"
	case http.StatusUnauthorized:
		return "unauthenticated"
	case http.StatusForbidden:
		return "accessDenied"
	case http.StatusNotFound:
		return "itemNotFound"
	case http.StatusConflict:
		return "nameAlreadyExists"
	case http.StatusPreconditionFailed:
		return "preconditionFailed"
	case http.StatusTooManyRequests:
		return "activityLimitReached"
	case http.StatusNotImplemented:
		return "notImplemented"
	default:
		return "generalException"
	}
}
