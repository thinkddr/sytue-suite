// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de la traducción PURA a los dialectos de Google Calendar v3 y Microsoft
// Graph: evento con hora vs de día completo, con y sin asistentes, series con y
// sin expandir, paginación en los dos formatos, ventana temporal de $filter y
// parámetros no soportados.
package compat

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

var (
	evID     = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	calID    = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	baseTime = time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC)
)

// timedEvent es un evento con hora en Europe/Madrid, con asistentes.
func timedEvent() domain.Event {
	return domain.Event{
		ID: evID, CalendarID: calID, UID: "uid-1@sytue",
		Summary: "Repaso semanal", Description: "Notas", Location: "Sala 2",
		Organizer: "ada@ex.com",
		DTStart:   baseTime, DTEnd: baseTime.Add(time.Hour),
		TZID: "Europe/Madrid", Status: domain.StatusConfirmed,
		Classification: domain.ClassPublic, Transparency: domain.TranspOpaque,
		SequenceNo: 2, CreatedAt: baseTime, UpdatedAt: baseTime,
		Attendees: []domain.Attendee{
			{Email: "ada@ex.com", CommonName: "Ada", Role: domain.RoleChair, PartStat: domain.PartAccepted},
			{Email: "bob@ex.com", Role: domain.RoleOptional, PartStat: domain.PartNeedsAction},
		},
	}
}

// allDayEvent es un evento de día completo (fin EXCLUSIVO) sin asistentes.
func allDayEvent() domain.Event {
	start := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	return domain.Event{
		ID: evID, CalendarID: calID, UID: "uid-2@sytue", Summary: "Festivo",
		DTStart: start, DTEnd: start.Add(24 * time.Hour), AllDay: true, TZID: "UTC",
		Status: domain.StatusConfirmed, Transparency: domain.TranspTransparent,
		CreatedAt: start, UpdatedAt: start,
	}
}

func TestGoogleFromEvent(t *testing.T) {
	tests := []struct {
		name                     string
		ev                       domain.Event
		wantStartDate            string
		wantStartDateTime        string
		wantTimeZone             string
		wantAttendees            int
		wantRecurrence           bool
		wantTransparency, wantID string
	}{
		{
			name: "con hora en su zona", ev: timedEvent(),
			wantStartDateTime: "2026-07-28T11:00:00+02:00", wantTimeZone: "Europe/Madrid",
			wantAttendees: 2, wantTransparency: "opaque", wantID: evID.String(),
		},
		{
			name: "de día completo", ev: allDayEvent(),
			wantStartDate: "2026-07-28", wantTransparency: "transparent", wantID: evID.String(),
		},
		{
			name: "serie sin expandir", ev: func() domain.Event {
				e := timedEvent()
				e.RRule = "FREQ=WEEKLY;COUNT=3"
				return e
			}(),
			wantStartDateTime: "2026-07-28T11:00:00+02:00", wantTimeZone: "Europe/Madrid",
			wantAttendees: 2, wantRecurrence: true, wantTransparency: "opaque", wantID: evID.String(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := GoogleFromEvent(tc.ev)
			if g.Kind != "calendar#event" || g.ID != tc.wantID {
				t.Fatalf("kind/id: %+v", g)
			}
			if g.Start.Date != tc.wantStartDate {
				t.Errorf("start.date = %q, quiero %q", g.Start.Date, tc.wantStartDate)
			}
			if g.Start.DateTime != tc.wantStartDateTime {
				t.Errorf("start.dateTime = %q, quiero %q", g.Start.DateTime, tc.wantStartDateTime)
			}
			if g.Start.TimeZone != tc.wantTimeZone && tc.wantStartDate == "" {
				t.Errorf("start.timeZone = %q, quiero %q", g.Start.TimeZone, tc.wantTimeZone)
			}
			if len(g.Attendees) != tc.wantAttendees {
				t.Errorf("asistentes = %d, quiero %d", len(g.Attendees), tc.wantAttendees)
			}
			if got := len(g.Recurrence) > 0; got != tc.wantRecurrence {
				t.Errorf("recurrence presente = %v, quiero %v", got, tc.wantRecurrence)
			}
			if g.Transparency != tc.wantTransparency {
				t.Errorf("transparency = %q, quiero %q", g.Transparency, tc.wantTransparency)
			}
		})
	}
}

// TestGoogleAllDayEndIsExclusive: el fin de un evento de día completo viaja como
// el día SIGUIENTE (exclusivo), que es lo que esperan Google y iCalendar.
func TestGoogleAllDayEndIsExclusive(t *testing.T) {
	g := GoogleFromEvent(allDayEvent())
	if g.Start.Date != "2026-07-28" || g.End.Date != "2026-07-29" {
		t.Fatalf("día completo: start=%q end=%q", g.Start.Date, g.End.Date)
	}
	if g.Start.DateTime != "" || g.End.DateTime != "" {
		t.Fatal("un evento de día completo no debe llevar dateTime")
	}
}

func TestGoogleAttendeeMapping(t *testing.T) {
	g := GoogleFromEvent(timedEvent())
	if g.Attendees[0].ResponseStatus != "accepted" || !g.Attendees[0].Organizer {
		t.Fatalf("organizador: %+v", g.Attendees[0])
	}
	if !g.Attendees[1].Optional || g.Attendees[1].ResponseStatus != "needsAction" {
		t.Fatalf("opcional: %+v", g.Attendees[1])
	}
}

// TestGoogleFromInstances: singleEvents=true devuelve OCURRENCIAS con el id
// compuesto de Google y el enlace a la serie; el override conserva su propio id.
func TestGoogleFromInstances(t *testing.T) {
	master := timedEvent()
	master.RRule = "FREQ=DAILY;COUNT=2"
	overrideID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	rid := baseTime.Add(24 * time.Hour)
	override := timedEvent()
	override.ID = overrideID
	override.RecurrenceID = &rid
	override.Summary = "Repaso movido"

	insts := []domain.Instance{
		{Event: master, Start: baseTime, End: baseTime.Add(time.Hour)},
		{Event: override, Start: rid.Add(2 * time.Hour), End: rid.Add(3 * time.Hour), IsOverride: true},
	}

	out := GoogleFromInstances(insts)
	if len(out) != 2 {
		t.Fatalf("ocurrencias = %d", len(out))
	}
	wantID := evID.String() + "_20260728T090000Z"
	if out[0].ID != wantID {
		t.Errorf("id de ocurrencia = %q, quiero %q", out[0].ID, wantID)
	}
	if out[0].RecurringEventID != evID.String() {
		t.Errorf("recurringEventId = %q", out[0].RecurringEventID)
	}
	if len(out[0].Recurrence) != 0 {
		t.Error("una ocurrencia expandida no debe llevar recurrence")
	}
	if out[1].ID != overrideID.String() || out[1].RecurringEventID != evID.String() {
		t.Errorf("override: id=%q recurringEventId=%q", out[1].ID, out[1].RecurringEventID)
	}
	if out[1].OriginalStartTime == nil || out[1].OriginalStartTime.DateTime != "2026-07-29T11:00:00+02:00" {
		t.Errorf("originalStartTime del override: %+v", out[1].OriginalStartTime)
	}
}

// TestFromEventsLinksExceptionsToMaster: en el listado SIN expandir, la excepción
// de una serie apunta a su maestro (Google recurringEventId / Graph seriesMasterId),
// resuelto por UID entre los eventos de la propia página.
func TestFromEventsLinksExceptionsToMaster(t *testing.T) {
	master := timedEvent()
	master.RRule = "FREQ=DAILY;COUNT=3"
	rid := baseTime.Add(24 * time.Hour)
	exception := timedEvent()
	exception.ID = uuid.MustParse("33333333-3333-4333-8333-333333333333")
	exception.RecurrenceID = &rid

	google := GoogleFromEvents([]domain.Event{master, exception})
	if google[0].RecurringEventID != "" {
		t.Errorf("el maestro no apunta a sí mismo: %q", google[0].RecurringEventID)
	}
	if google[1].RecurringEventID != evID.String() {
		t.Errorf("recurringEventId de la excepción = %q", google[1].RecurringEventID)
	}

	graph := GraphFromEvents([]domain.Event{master, exception})
	if graph[0].Type != "seriesMaster" || graph[0].SeriesMasterID != "" {
		t.Errorf("maestro en Graph: %+v", graph[0])
	}
	if graph[1].Type != "exception" || graph[1].SeriesMasterID != evID.String() {
		t.Errorf("excepción en Graph: %+v", graph[1])
	}

	// Sin el maestro en la página no se inventa el enlace: se omite.
	if got := GoogleFromEvents([]domain.Event{exception}); got[0].RecurringEventID != "" {
		t.Errorf("sin maestro conocido no debe inventarse recurringEventId: %q", got[0].RecurringEventID)
	}
}

// TestGoogleFromInstanceSingleEvent: un evento puntual expandido mantiene su id
// simple (no se le inventa sufijo de ocurrencia).
func TestGoogleFromInstanceSingleEvent(t *testing.T) {
	ev := timedEvent()
	g := GoogleFromInstance(domain.Instance{Event: ev, Start: ev.DTStart, End: ev.DTEnd}, "")
	if g.ID != evID.String() || g.RecurringEventID != "" {
		t.Fatalf("evento puntual: id=%q recurringEventId=%q", g.ID, g.RecurringEventID)
	}
}

func TestInstanceIDRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		allDay bool
		start  time.Time
	}{
		{name: "con hora", start: baseTime},
		{name: "día completo", allDay: true, start: time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id := InstanceID(evID.String(), tc.start, tc.allDay)
			master, start, ok := SplitInstanceID(id)
			if !ok || master != evID.String() || !start.Equal(tc.start) {
				t.Fatalf("SplitInstanceID(%q) = (%q, %v, %v)", id, master, start, ok)
			}
		})
	}
	if _, _, ok := SplitInstanceID(evID.String()); ok {
		t.Fatal("un UUID suelto no es un id de ocurrencia")
	}
}

func TestGraphFromEvent(t *testing.T) {
	tests := []struct {
		name          string
		ev            domain.Event
		wantType      string
		wantAllDay    bool
		wantStart     string
		wantZone      string
		wantShowAs    string
		wantAttendees int
	}{
		{
			name: "con hora", ev: timedEvent(), wantType: "singleInstance",
			wantStart: "2026-07-28T11:00:00.0000000", wantZone: "Europe/Madrid",
			wantShowAs: "busy", wantAttendees: 2,
		},
		{
			name: "de día completo", ev: allDayEvent(), wantType: "singleInstance", wantAllDay: true,
			wantStart: "2026-07-28T00:00:00.0000000", wantZone: "UTC", wantShowAs: "free",
		},
		{
			name: "serie", ev: func() domain.Event {
				e := timedEvent()
				e.RRule = "FREQ=DAILY"
				return e
			}(), wantType: "seriesMaster",
			wantStart: "2026-07-28T11:00:00.0000000", wantZone: "Europe/Madrid",
			wantShowAs: "busy", wantAttendees: 2,
		},
		{
			name: "excepción de serie", ev: func() domain.Event {
				e := timedEvent()
				rid := baseTime
				e.RecurrenceID = &rid
				return e
			}(), wantType: "exception",
			wantStart: "2026-07-28T11:00:00.0000000", wantZone: "Europe/Madrid",
			wantShowAs: "busy", wantAttendees: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := GraphFromEvent(tc.ev)
			if g.Type != tc.wantType {
				t.Errorf("type = %q, quiero %q", g.Type, tc.wantType)
			}
			if g.IsAllDay != tc.wantAllDay {
				t.Errorf("isAllDay = %v", g.IsAllDay)
			}
			if g.Start.DateTime != tc.wantStart || g.Start.TimeZone != tc.wantZone {
				t.Errorf("start = %+v, quiero %s/%s", g.Start, tc.wantStart, tc.wantZone)
			}
			if g.ShowAs != tc.wantShowAs {
				t.Errorf("showAs = %q, quiero %q", g.ShowAs, tc.wantShowAs)
			}
			if len(g.Attendees) != tc.wantAttendees {
				t.Errorf("asistentes = %d, quiero %d", len(g.Attendees), tc.wantAttendees)
			}
		})
	}
}

func TestGraphAttendeeAndBody(t *testing.T) {
	g := GraphFromEvent(timedEvent())
	if g.Body == nil || g.Body.ContentType != "text" || g.Body.Content != "Notas" {
		t.Fatalf("body: %+v", g.Body)
	}
	if g.Location == nil || g.Location.DisplayName != "Sala 2" {
		t.Fatalf("location: %+v", g.Location)
	}
	if g.Attendees[0].Status.Response != "accepted" || g.Attendees[0].Type != "required" {
		t.Fatalf("asistente 0: %+v", g.Attendees[0])
	}
	if g.Attendees[1].Type != "optional" || g.Attendees[1].Status.Response != "notResponded" {
		t.Fatalf("asistente 1: %+v", g.Attendees[1])
	}
	// Un evento sin descripción ni lugar no inventa cuerpo ni ubicación.
	plain := GraphFromEvent(allDayEvent())
	if plain.Body != nil || plain.Location != nil {
		t.Fatalf("evento sin cuerpo: body=%+v location=%+v", plain.Body, plain.Location)
	}
}

func TestParseGraphDateFilter(t *testing.T) {
	tests := []struct {
		name             string
		filter           string
		wantFrom, wantTo string
		wantErr          bool
	}{
		{
			name:     "rango completo",
			filter:   "start/dateTime ge '2026-07-01T00:00:00Z' and end/dateTime le '2026-08-01T00:00:00Z'",
			wantFrom: "2026-07-01T00:00:00Z", wantTo: "2026-08-01T00:00:00Z",
		},
		{
			name:     "sin comillas ni zona",
			filter:   "start/dateTime gt 2026-07-01T00:00:00 and end/dateTime lt 2026-08-01T00:00:00",
			wantFrom: "2026-07-01T00:00:00Z", wantTo: "2026-08-01T00:00:00Z",
		},
		{name: "vacío", filter: ""},
		{name: "campo no soportado", filter: "subject eq 'Repaso'", wantErr: true},
		{name: "operador invertido", filter: "start/dateTime le '2026-07-01T00:00:00Z'", wantErr: true},
		{name: "fecha ilegible", filter: "start/dateTime ge 'ayer'", wantErr: true},
		{
			name:    "término extra no soportado",
			filter:  "start/dateTime ge '2026-07-01T00:00:00Z' and isAllDay eq true",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			from, to, err := ParseGraphDateFilter(tc.filter)
			if tc.wantErr {
				if !errors.Is(err, ErrUnsupportedFilter) {
					t.Fatalf("quiero ErrUnsupportedFilter, tengo %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if got := rfc3339(from); got != tc.wantFrom {
				t.Errorf("from = %q, quiero %q", got, tc.wantFrom)
			}
			if got := rfc3339(to); got != tc.wantTo {
				t.Errorf("to = %q, quiero %q", got, tc.wantTo)
			}
		})
	}
}

func TestCheckParams(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{name: "soportados", query: "timeMin=x&timeMax=y&maxResults=10"},
		{name: "de sistema y proyección", query: "alt=json&prettyPrint=true&fields=items(id)&key=abc"},
		{name: "filtro no emulado", query: "timeMin=x&q=reunión", wantErr: true},
		{name: "sincronización incremental", query: "syncToken=abc", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := url.ParseQuery(tc.query)
			if err != nil {
				t.Fatalf("query de prueba inválida: %v", err)
			}
			err = CheckParams(q, "timeMin", "timeMax", "maxResults", "pageToken", "singleEvents")
			if tc.wantErr != (err != nil) {
				t.Fatalf("CheckParams = %v (quiero error = %v)", err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, ErrUnsupportedParam) {
				t.Fatalf("quiero ErrUnsupportedParam, tengo %v", err)
			}
		})
	}
}

func TestPageTokenAndSlice(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	page, next := Slice(items, 0, 2)
	if len(page) != 2 || next != 2 {
		t.Fatalf("primera página: %v / %d", page, next)
	}
	tok := EncodePageToken(next)
	off, err := DecodePageToken(tok)
	if err != nil || off != 2 {
		t.Fatalf("token %q -> (%d, %v)", tok, off, err)
	}
	page, next = Slice(items, 4, 2)
	if len(page) != 1 || next != 0 {
		t.Fatalf("última página: %v / %d", page, next)
	}
	if page, next = Slice(items, 9, 2); page != nil || next != 0 {
		t.Fatalf("más allá del final: %v / %d", page, next)
	}
	if _, err := DecodePageToken("no-es-un-token"); !errors.Is(err, ErrInvalidPageToken) {
		t.Fatalf("un token ajeno debe ser ErrInvalidPageToken, tengo %v", err)
	}
	if off, err := DecodePageToken(""); err != nil || off != 0 {
		t.Fatalf("token vacío = primera página, tengo (%d, %v)", off, err)
	}
}

func TestPageSize(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{raw: "", want: 250},
		{raw: "0", want: 250},
		{raw: "abc", want: 250},
		{raw: "10", want: 10},
		{raw: "5000", want: 1000},
	}
	for _, tc := range tests {
		if got := PageSize(tc.raw, 250, 1000); got != tc.want {
			t.Errorf("PageSize(%q) = %d, quiero %d", tc.raw, got, tc.want)
		}
	}
}

func TestNextLinkKeepsFilter(t *testing.T) {
	q := url.Values{"$filter": {"start/dateTime ge '2026-07-01T00:00:00Z'"}, "$top": {"2"}, "$skip": {"2"}}
	link := NextLink("https://calendar.sytue.eu/", "/compat/graph/v1.0/me/events", q, 2, 4)
	want := "https://calendar.sytue.eu/compat/graph/v1.0/me/events?" +
		url.Values{"$filter": {"start/dateTime ge '2026-07-01T00:00:00Z'"}, "$top": {"2"}, "$skip": {"4"}}.Encode()
	if link != want {
		t.Fatalf("nextLink = %q, quiero %q", link, want)
	}
	if NextLink("https://x", "/y", q, 2, 0) != "" {
		t.Fatal("sin más páginas no debe haber nextLink")
	}
}

func TestGoogleEventRequestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		req     GoogleEventRequest
		wantErr bool
		check   func(*testing.T, EventFields)
	}{
		{
			name: "con hora y zona",
			req: GoogleEventRequest{
				Summary: "Café",
				Start:   &GoogleEventDateTime{DateTime: "2026-07-28T11:00:00+02:00", TimeZone: "Europe/Madrid"},
				End:     &GoogleEventDateTime{DateTime: "2026-07-28T12:00:00+02:00", TimeZone: "Europe/Madrid"},
				Attendees: []GoogleAttendee{
					{Email: "bob@ex.com", Optional: true, ResponseStatus: "accepted"},
					{Email: "  "},
				},
			},
			check: func(t *testing.T, f EventFields) {
				t.Helper()
				if f.AllDay || f.TZID != "Europe/Madrid" {
					t.Errorf("allDay/tz: %v/%q", f.AllDay, f.TZID)
				}
				if !f.Start.Equal(time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC)) {
					t.Errorf("start = %v", f.Start)
				}
				if len(f.Attendees) != 1 || f.Attendees[0].Role != domain.RoleOptional ||
					f.Attendees[0].PartStat != domain.PartAccepted {
					t.Errorf("asistentes = %+v", f.Attendees)
				}
			},
		},
		{
			name: "de día completo",
			req: GoogleEventRequest{
				Start: &GoogleEventDateTime{Date: "2026-07-28"},
				End:   &GoogleEventDateTime{Date: "2026-07-29"},
			},
			check: func(t *testing.T, f EventFields) {
				t.Helper()
				if !f.AllDay || !f.End.Equal(time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)) {
					t.Errorf("día completo: %+v", f)
				}
			},
		},
		{
			name: "serie con RRULE",
			req: GoogleEventRequest{
				Start:      &GoogleEventDateTime{DateTime: "2026-07-28T09:00:00Z"},
				End:        &GoogleEventDateTime{DateTime: "2026-07-28T10:00:00Z"},
				Recurrence: []string{"RRULE:FREQ=WEEKLY;COUNT=3"},
			},
			check: func(t *testing.T, f EventFields) {
				t.Helper()
				if f.RRule != "FREQ=WEEKLY;COUNT=3" {
					t.Errorf("rrule = %q", f.RRule)
				}
			},
		},
		{name: "sin start ni end", req: GoogleEventRequest{}, wantErr: true},
		{
			name: "date y dateTime mezclados",
			req: GoogleEventRequest{
				Start: &GoogleEventDateTime{Date: "2026-07-28"},
				End:   &GoogleEventDateTime{DateTime: "2026-07-28T10:00:00Z"},
			},
			wantErr: true,
		},
		{
			name: "recurrence con EXDATE",
			req: GoogleEventRequest{
				Start:      &GoogleEventDateTime{DateTime: "2026-07-28T09:00:00Z"},
				End:        &GoogleEventDateTime{DateTime: "2026-07-28T10:00:00Z"},
				Recurrence: []string{"RRULE:FREQ=DAILY", "EXDATE;TZID=UTC:20260729T090000"},
			},
			wantErr: true,
		},
		{
			name: "status desconocido",
			req: GoogleEventRequest{
				Start:  &GoogleEventDateTime{DateTime: "2026-07-28T09:00:00Z"},
				End:    &GoogleEventDateTime{DateTime: "2026-07-28T10:00:00Z"},
				Status: "maybe",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.req.Normalize()
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidEvent) {
					t.Fatalf("quiero ErrInvalidEvent, tengo %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if tc.check != nil {
				tc.check(t, f)
			}
		})
	}
}

func TestGoogleFromCalendarAndAccessRole(t *testing.T) {
	cal := domain.Calendar{
		ID: calID, Name: "trabajo", DisplayName: "Trabajo", Timezone: "Europe/Madrid",
		Color: "#0B6E4F", IsDefault: true,
	}
	e := GoogleFromCalendar(cal, RoleOwner)
	if e.Kind != "calendar#calendarListEntry" || e.Summary != "Trabajo" || !e.Primary || e.AccessRole != RoleOwner {
		t.Fatalf("entrada de calendarList: %+v", e)
	}
	if got := GoogleAccessRole(domain.PermReader); got != RoleReader {
		t.Errorf("reader -> %q", got)
	}
	if got := GoogleAccessRole(domain.PermManage); got != RoleWriter {
		t.Errorf("manage -> %q (la delegación escribe)", got)
	}
	g := GraphFromCalendar(cal, "ada@ex.com", true)
	if g.Name != "Trabajo" || !g.IsDefaultCalendar || !g.CanEdit || g.Owner.Address != "ada@ex.com" {
		t.Fatalf("calendario de Graph: %+v", g)
	}
	ro := GraphFromCalendar(domain.Calendar{ID: calID, Name: "ics", ReadOnly: true}, "", true)
	if ro.CanEdit || ro.Owner != nil {
		t.Fatalf("una suscripción ICS no es editable ni inventa dueño: %+v", ro)
	}
}

func TestNewCollectionsNeverNil(t *testing.T) {
	if items := NewGoogleEvents(domain.Calendar{}, RoleReader, nil, "").Items; items == nil {
		t.Error("items de events.list no puede ser nulo")
	}
	if items := NewGoogleCalendarList(nil, "").Items; items == nil {
		t.Error("items de calendarList no puede ser nulo")
	}
	if v := NewGraphCollection[GraphEvent](nil, "").Value; v == nil {
		t.Error("value de una colección OData no puede ser nulo")
	}
}
