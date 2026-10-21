// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de la capa de compatibilidad de calendario (Google Calendar v3 y
// Microsoft Graph) con el fake del servicio: sin PostgreSQL. Cubren la ventana
// temporal obligatoria, la expansión de series, la paginación en los dos formatos,
// los parámetros no soportados, los 501 honestos y el AISLAMIENTO (un usuario no
// ve por la vía compat lo que no vería por la nativa).
package calendarhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// --- fixtures y helpers ------------------------------------------------------

var (
	compatCalID = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	compatEvID  = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	compatStart = time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC)
	compatWin   = "timeMin=" + url.QueryEscape("2026-07-01T00:00:00Z") + "&timeMax=" + url.QueryEscape("2026-08-01T00:00:00Z")
	graphWin    = "$filter=" + url.QueryEscape("start/dateTime ge '2026-07-01T00:00:00Z' and end/dateTime le '2026-08-01T00:00:00Z'")
)

func compatCalendar(owner string) domain.Calendar {
	return domain.Calendar{
		ID: compatCalID, OwnerSub: owner, Name: "trabajo", DisplayName: "Trabajo",
		Timezone: "Europe/Madrid", Color: "#0B6E4F", IsDefault: true,
	}
}

func compatEvent() domain.Event {
	return domain.Event{
		ID: compatEvID, CalendarID: compatCalID, UID: "uid-1@sytue", Summary: "Repaso",
		DTStart: compatStart, DTEnd: compatStart.Add(time.Hour), TZID: "Europe/Madrid",
		Status: domain.StatusConfirmed, Transparency: domain.TranspOpaque,
		CreatedAt: compatStart, UpdatedAt: compatStart,
	}
}

// compatRouter monta los dos dialectos con un middleware de auth de mentira que
// inyecta los claims del usuario dado (en producción es Verifier.RequireAuth).
func compatRouter(svc calendarService, sub string) http.Handler {
	r := chi.NewRouter()
	auth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			claims := authn.Claims{Subject: sub, Tenant: "acme", Email: sub + "@ex.com"}
			next.ServeHTTP(w, req.WithContext(authn.Into(req.Context(), claims)))
		})
	}
	mountCompat(r, &compatHandlers{svc: svc, cfg: config.Config{
		PublicURL:      "https://calendar.sytue.eu",
		MaxRangeWindow: 366 * 24 * time.Hour,
	}}, auth, auth)
	return r
}

// compatDo lanza una petición contra el router de compat.
func compatDo(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, target, http.NoBody)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// compatBody decodifica el cuerpo JSON en un mapa genérico.
func compatBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("cuerpo no es JSON: %v (%q)", err, rec.Body.String())
	}
	return m
}

// googleReasonOf extrae error.errors[0].reason del sobre de Google.
func googleReasonOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	body := compatBody(t, rec)
	e, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("sin sobre de error de Google: %s", rec.Body.String())
	}
	list, _ := e["errors"].([]any)
	if len(list) == 0 {
		t.Fatalf("sobre de Google sin errors[]: %s", rec.Body.String())
	}
	item, _ := list[0].(map[string]any)
	reason, _ := item["reason"].(string)
	return reason
}

// graphCodeOf extrae error.code del sobre de Graph.
func graphCodeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	body := compatBody(t, rec)
	e, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("sin sobre de error de Graph: %s", rec.Body.String())
	}
	code, _ := e["code"].(string)
	return code
}

// items extrae la lista de una respuesta de Google (items) o de Graph (value).
func items(t *testing.T, rec *httptest.ResponseRecorder, key string) []map[string]any {
	t.Helper()
	list, _ := compatBody(t, rec)[key].([]any)
	out := make([]map[string]any, 0, len(list))
	for _, it := range list {
		m, _ := it.(map[string]any)
		out = append(out, m)
	}
	return out
}

// --- Google Calendar v3 ------------------------------------------------------

func TestCompatGoogleCalendarList(t *testing.T) {
	own := compatCalendar("user-1")
	sharedCal := compatCalendar("otro")
	sharedCal.ID = uuid.New()
	sharedCal.IsDefault = false
	f := &fakeService{
		ensureCal: own,
		calendars: []domain.Calendar{own},
		sharedCals: []store.SharedCalendar{
			{Calendar: sharedCal, Permission: domain.PermReader, SharedBy: "otro@ex.com"},
		},
	}
	h := compatRouter(f, "user-1")

	rec := compatDo(h, http.MethodGet, "/compat/google/calendar/v3/users/me/calendarList", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
	}
	body := compatBody(t, rec)
	if body["kind"] != "calendar#calendarList" {
		t.Fatalf("kind = %v", body["kind"])
	}
	got := items(t, rec, "items")
	if len(got) != 2 || got[0]["accessRole"] != "owner" || got[1]["accessRole"] != "reader" {
		t.Fatalf("calendarios/roles: %+v", got)
	}
	if got[0]["summary"] != "Trabajo" || got[0]["primary"] != true {
		t.Fatalf("entrada propia: %+v", got[0])
	}

	// minAccessRole=writer descarta el compartido de solo lectura.
	rec = compatDo(h, http.MethodGet, "/compat/google/calendar/v3/users/me/calendarList?minAccessRole=writer", "")
	if got := items(t, rec, "items"); len(got) != 1 || got[0]["accessRole"] != "owner" {
		t.Fatalf("minAccessRole=writer: %+v", got)
	}
}

func TestCompatGoogleCalendarListPagination(t *testing.T) {
	own := compatCalendar("user-1")
	second := compatCalendar("user-1")
	second.ID = uuid.New()
	second.DisplayName = "Personal"
	second.IsDefault = false
	f := &fakeService{ensureCal: own, calendars: []domain.Calendar{own, second}}
	h := compatRouter(f, "user-1")

	rec := compatDo(h, http.MethodGet, "/compat/google/calendar/v3/users/me/calendarList?maxResults=1", "")
	body := compatBody(t, rec)
	token, _ := body["nextPageToken"].(string)
	if token == "" {
		t.Fatalf("falta nextPageToken: %s", rec.Body.String())
	}
	if got := items(t, rec, "items"); len(got) != 1 || got[0]["summary"] != "Trabajo" {
		t.Fatalf("primera página: %+v", got)
	}

	rec = compatDo(h, http.MethodGet, "/compat/google/calendar/v3/users/me/calendarList?maxResults=1&pageToken="+url.QueryEscape(token), "")
	body = compatBody(t, rec)
	if body["nextPageToken"] != nil {
		t.Fatalf("la última página no debe traer token: %s", rec.Body.String())
	}
	if got := items(t, rec, "items"); len(got) != 1 || got[0]["summary"] != "Personal" {
		t.Fatalf("segunda página: %+v", got)
	}

	rec = compatDo(h, http.MethodGet, "/compat/google/calendar/v3/users/me/calendarList?pageToken=basura", "")
	if rec.Code != http.StatusBadRequest || googleReasonOf(t, rec) != "badRequest" {
		t.Fatalf("token ajeno: %d %s", rec.Code, rec.Body.String())
	}
}

// TestCompatGoogleEventsWindow: sin ventana temporal la emulación NO devuelve una
// agenda parcial: responde 400 explicando qué falta.
func TestCompatGoogleEventsWindow(t *testing.T) {
	f := &fakeService{cal: compatCalendar("user-1")}
	h := compatRouter(f, "user-1")

	tests := []struct {
		name, query string
	}{
		{name: "sin ventana", query: ""},
		{name: "solo timeMin", query: "?timeMin=" + url.QueryEscape("2026-07-01T00:00:00Z")},
		{name: "fin antes que inicio", query: "?timeMin=" + url.QueryEscape("2026-08-01T00:00:00Z") + "&timeMax=" + url.QueryEscape("2026-07-01T00:00:00Z")},
		{name: "fecha ilegible", query: "?timeMin=ayer&timeMax=hoy"},
		{name: "ventana desmesurada", query: "?timeMin=" + url.QueryEscape("2000-01-01T00:00:00Z") + "&timeMax=" + url.QueryEscape("2030-01-01T00:00:00Z")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := compatDo(h, http.MethodGet, "/compat/google/calendar/v3/calendars/"+compatCalID.String()+"/events"+tc.query, "")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
			}
			if r := googleReasonOf(t, rec); r != "badRequest" {
				t.Fatalf("reason = %q", r)
			}
		})
	}
}

// TestCompatGoogleEventsSingleEvents: con singleEvents=true se sirven OCURRENCIAS
// expandidas (mismo motor que CalDAV); sin él, la serie sin expandir con su RRULE.
func TestCompatGoogleEventsSingleEvents(t *testing.T) {
	master := compatEvent()
	master.RRule = "FREQ=DAILY;COUNT=2"
	f := &fakeService{
		cal: compatCalendar("user-1"),
		instances: []domain.Instance{
			{Event: master, Start: compatStart, End: compatStart.Add(time.Hour)},
			{Event: master, Start: compatStart.Add(24 * time.Hour), End: compatStart.Add(25 * time.Hour)},
		},
		rangeEvents: []domain.Event{master},
	}
	h := compatRouter(f, "user-1")

	rec := compatDo(h, http.MethodGet, "/compat/google/calendar/v3/calendars/"+compatCalID.String()+"/events?"+compatWin+"&singleEvents=true", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
	}
	got := items(t, rec, "items")
	if len(got) != 2 {
		t.Fatalf("ocurrencias = %d", len(got))
	}
	if got[0]["id"] != compatEvID.String()+"_20260728T090000Z" {
		t.Fatalf("id de ocurrencia: %v", got[0]["id"])
	}
	if got[0]["recurringEventId"] != compatEvID.String() {
		t.Fatalf("recurringEventId: %v", got[0]["recurringEventId"])
	}

	rec = compatDo(h, http.MethodGet, "/compat/google/calendar/v3/calendars/"+compatCalID.String()+"/events?"+compatWin, "")
	got = items(t, rec, "items")
	if len(got) != 1 || got[0]["id"] != compatEvID.String() {
		t.Fatalf("serie sin expandir: %+v", got)
	}
	rec2 := got[0]["recurrence"]
	if list, _ := rec2.([]any); len(list) != 1 || list[0] != "RRULE:FREQ=DAILY;COUNT=2" {
		t.Fatalf("recurrence = %v", rec2)
	}
	if f.gotRangeCal != compatCalID {
		t.Fatalf("el listado sin expandir debe ir al calendario resuelto, fue a %v", f.gotRangeCal)
	}
}

func TestCompatGoogleEventsUnsupportedParam(t *testing.T) {
	f := &fakeService{cal: compatCalendar("user-1")}
	h := compatRouter(f, "user-1")
	for _, q := range []string{"&q=reunión", "&syncToken=abc", "&orderBy=updated", "&showDeleted=true"} {
		rec := compatDo(h, http.MethodGet, "/compat/google/calendar/v3/calendars/primary/events?"+compatWin+q, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: estado = %d (%s)", q, rec.Code, rec.Body.String())
		}
	}
	// Los parámetros de sistema y de proyección NO rompen la llamada.
	f.ensureCal = compatCalendar("user-1")
	rec := compatDo(h, http.MethodGet, "/compat/google/calendar/v3/calendars/primary/events?"+compatWin+"&alt=json&prettyPrint=true&fields="+url.QueryEscape("items(id,summary)"), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("alt/prettyPrint/fields deben ignorarse: %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCompatGoogleEventGet(t *testing.T) {
	ev := compatEvent()
	f := &fakeService{cal: compatCalendar("user-1"), event: ev}
	h := compatRouter(f, "user-1")

	rec := compatDo(h, http.MethodGet, "/compat/google/calendar/v3/calendars/"+compatCalID.String()+"/events/"+compatEvID.String(), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
	}
	body := compatBody(t, rec)
	if body["kind"] != "calendar#event" || body["id"] != compatEvID.String() {
		t.Fatalf("evento: %+v", body)
	}

	// Un evento de OTRO calendario no se sirve por la ruta de este.
	other := compatCalendar("user-1")
	other.ID = uuid.New()
	f.cal = other
	rec = compatDo(h, http.MethodGet, "/compat/google/calendar/v3/calendars/"+other.ID.String()+"/events/"+compatEvID.String(), "")
	if rec.Code != http.StatusNotFound || googleReasonOf(t, rec) != "notFound" {
		t.Fatalf("evento de otro calendario: %d %s", rec.Code, rec.Body.String())
	}

	// Un id que no es UUID es 404 (indistinguible de «no existe»), no 400.
	f.cal = compatCalendar("user-1")
	rec = compatDo(h, http.MethodGet, "/compat/google/calendar/v3/calendars/"+compatCalID.String()+"/events/no-existe", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("id ilegible: %d %s", rec.Code, rec.Body.String())
	}
}

// TestCompatGoogleEventGetOccurrence: el id compuesto de una ocurrencia se
// resuelve expandiendo la serie; si esa ocurrencia no existe, 404 (nunca una
// ocurrencia inventada).
func TestCompatGoogleEventGetOccurrence(t *testing.T) {
	master := compatEvent()
	master.RRule = "FREQ=DAILY;COUNT=2"
	f := &fakeService{
		cal:   compatCalendar("user-1"),
		event: master,
		instances: []domain.Instance{
			{Event: master, Start: compatStart, End: compatStart.Add(time.Hour)},
		},
	}
	h := compatRouter(f, "user-1")
	base := "/compat/google/calendar/v3/calendars/" + compatCalID.String() + "/events/"

	rec := compatDo(h, http.MethodGet, base+compatEvID.String()+"_20260728T090000Z", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
	}
	body := compatBody(t, rec)
	if body["id"] != compatEvID.String()+"_20260728T090000Z" || body["recurringEventId"] != compatEvID.String() {
		t.Fatalf("ocurrencia: %+v", body)
	}

	// Instante sin ocurrencia (p. ej. excluido por EXDATE): 404.
	f.instances = nil
	rec = compatDo(h, http.MethodGet, base+compatEvID.String()+"_20260730T090000Z", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("ocurrencia inexistente: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCompatGoogleCreateEvent(t *testing.T) {
	f := &fakeService{ensureCal: compatCalendar("user-1"), event: compatEvent()}
	h := compatRouter(f, "user-1")

	body := `{"summary":"Café","location":"Bar","start":{"dateTime":"2026-07-28T11:00:00+02:00","timeZone":"Europe/Madrid"},` +
		`"end":{"dateTime":"2026-07-28T12:00:00+02:00","timeZone":"Europe/Madrid"},` +
		`"attendees":[{"email":"bob@ex.com","optional":true}],"recurrence":["RRULE:FREQ=WEEKLY;COUNT=2"]}`
	rec := compatDo(h, http.MethodPost, "/compat/google/calendar/v3/calendars/primary/events?sendUpdates=all", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
	}
	in := f.gotEventIn
	if in.CalendarID != compatCalID || in.Summary != "Café" || in.Location != "Bar" {
		t.Fatalf("entrada al servicio: %+v", in)
	}
	if !in.DTStart.Equal(compatStart) || in.TZID != "Europe/Madrid" || in.AllDay {
		t.Fatalf("rango/zona: %+v", in)
	}
	if in.RRule != "FREQ=WEEKLY;COUNT=2" || len(in.Attendees) != 1 || in.Attendees[0].Role != domain.RoleOptional {
		t.Fatalf("serie/asistentes: %+v", in)
	}
	if !in.Send {
		t.Fatal("sendUpdates=all debe entregar invitaciones")
	}
	if in.Organizer != "user-1@ex.com" {
		t.Fatalf("organizador por defecto = %q", in.Organizer)
	}

	// Día completo: fin exclusivo, sin zona.
	allDay := `{"summary":"Festivo","start":{"date":"2026-07-28"},"end":{"date":"2026-07-29"}}`
	if rec := compatDo(h, http.MethodPost, "/compat/google/calendar/v3/calendars/primary/events", allDay); rec.Code != http.StatusOK {
		t.Fatalf("día completo: %d (%s)", rec.Code, rec.Body.String())
	}
	if !f.gotEventIn.AllDay || f.gotEventIn.Send {
		t.Fatalf("día completo sin sendUpdates: %+v", f.gotEventIn)
	}
}

func TestCompatGoogleCreateEventRejections(t *testing.T) {
	f := &fakeService{ensureCal: compatCalendar("user-1"), event: compatEvent()}
	h := compatRouter(f, "user-1")
	target := "/compat/google/calendar/v3/calendars/primary/events"

	tests := []struct {
		name, target, body string
	}{
		{
			name: "campo que no interpretamos", target: target,
			body: `{"summary":"x","start":{"date":"2026-07-28"},"end":{"date":"2026-07-29"},"reminders":{"useDefault":true}}`,
		},
		{
			name: "start y end mezclando date y dateTime", target: target,
			body: `{"start":{"date":"2026-07-28"},"end":{"dateTime":"2026-07-28T10:00:00Z"}}`,
		},
		{
			name: "sendUpdates externalOnly", target: target + "?sendUpdates=externalOnly",
			body: `{"start":{"date":"2026-07-28"},"end":{"date":"2026-07-29"}}`,
		},
		{
			name: "recurrence con EXDATE", target: target,
			body: `{"start":{"date":"2026-07-28"},"end":{"date":"2026-07-29"},"recurrence":["RRULE:FREQ=DAILY","EXDATE:20260729"]}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := compatDo(h, http.MethodPost, tc.target, tc.body)
			if rec.Code != http.StatusBadRequest || googleReasonOf(t, rec) != "badRequest" {
				t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCompatGoogleNotImplemented(t *testing.T) {
	h := compatRouter(&fakeService{}, "user-1")
	rec := compatDo(h, http.MethodGet, "/compat/google/calendar/v3/colors", "")
	if rec.Code != http.StatusNotImplemented || googleReasonOf(t, rec) != "notImplemented" {
		t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
	}
	rec = compatDo(h, http.MethodPost, "/compat/google/calendar/v3/calendars/primary/events/"+compatEvID.String()+"/move", "")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("mover no emulado: %d (%s)", rec.Code, rec.Body.String())
	}
}

// update reemplaza con el MISMO UID; patch toca solo lo que trae y deja el resto;
// delete avisa a los invitados solo con sendUpdates=all, como Google.
func TestCompatGoogleUpdatePatchDeleteEvent(t *testing.T) {
	f := &fakeService{ensureCal: compatCalendar("user-1"), event: compatEvent()}
	h := compatRouter(f, "user-1")
	evento := "/compat/google/calendar/v3/calendars/primary/events/" + compatEvID.String()

	put := `{"summary":"Repaso largo","start":{"dateTime":"2026-07-28T11:00:00+02:00","timeZone":"Europe/Madrid"},` +
		`"end":{"dateTime":"2026-07-28T13:00:00+02:00","timeZone":"Europe/Madrid"}}`
	if rec := compatDo(h, http.MethodPut, evento, put); rec.Code != http.StatusOK {
		t.Fatalf("update: %d (%s)", rec.Code, rec.Body.String())
	}
	if in := f.gotEventIn; in.UID != "uid-1@sytue" || in.Summary != "Repaso largo" || !in.DTEnd.Equal(compatStart.Add(2*time.Hour)) {
		t.Fatalf("update al servicio: %+v", in)
	}

	f.gotEventIn = service.EventInput{}
	if rec := compatDo(h, http.MethodPatch, evento+"?sendUpdates=all", `{"summary":"Solo el título"}`); rec.Code != http.StatusOK {
		t.Fatalf("patch: %d (%s)", rec.Code, rec.Body.String())
	}
	if in := f.gotEventIn; in.Summary != "Solo el título" || in.UID != "uid-1@sytue" || !in.DTStart.Equal(compatStart) ||
		!in.DTEnd.Equal(compatStart.Add(time.Hour)) || in.TZID != "Europe/Madrid" || !in.Send {
		t.Fatalf("patch conserva lo demás: %+v", in)
	}

	for nombre, c := range map[string]struct{ method, target, body string }{
		"patch con campo desconocido":  {http.MethodPatch, evento, `{"reminders":{"useDefault":true}}`},
		"update que cambia el iCalUID": {http.MethodPut, evento, `{"iCalUID":"otro","start":{"date":"2026-07-28"},"end":{"date":"2026-07-29"}}`},
		"una ocurrencia suelta":        {http.MethodPatch, evento + "_20260728T090000Z", `{"summary":"x"}`},
	} {
		if rec := compatDo(h, c.method, c.target, c.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d (%s)", nombre, rec.Code, rec.Body.String())
		}
	}

	if rec := compatDo(h, http.MethodDelete, evento, ""); rec.Code != http.StatusNoContent || f.gotDeleteID != compatEvID || f.gotDeleteSend {
		t.Fatalf("delete sin sendUpdates: %d, id=%s, avisa=%v", rec.Code, f.gotDeleteID, f.gotDeleteSend)
	}
	if rec := compatDo(h, http.MethodDelete, evento+"?sendUpdates=all", ""); rec.Code != http.StatusNoContent || !f.gotDeleteSend {
		t.Fatalf("delete con sendUpdates=all: %d, avisa=%v", rec.Code, f.gotDeleteSend)
	}
}

// --- Microsoft Graph ---------------------------------------------------------

func TestCompatGraphCalendars(t *testing.T) {
	own := compatCalendar("user-1")
	sharedCal := compatCalendar("otro")
	sharedCal.ID = uuid.New()
	f := &fakeService{
		ensureCal: own,
		calendars: []domain.Calendar{own},
		sharedCals: []store.SharedCalendar{
			{Calendar: sharedCal, Permission: domain.PermReader},
		},
	}
	h := compatRouter(f, "user-1")

	rec := compatDo(h, http.MethodGet, "/compat/graph/v1.0/me/calendars", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
	}
	got := items(t, rec, "value")
	if len(got) != 2 || got[0]["canEdit"] != true || got[1]["canEdit"] != false {
		t.Fatalf("calendarios de Graph: %+v", got)
	}
	if got[0]["name"] != "Trabajo" || got[0]["isDefaultCalendar"] != true {
		t.Fatalf("calendario propio: %+v", got[0])
	}
}

func TestCompatGraphEvents(t *testing.T) {
	ev := compatEvent()
	second := compatEvent()
	second.ID = uuid.New()
	second.Summary = "Otro"
	f := &fakeService{ensureCal: compatCalendar("user-1"), rangeEvents: []domain.Event{ev, second}}
	h := compatRouter(f, "user-1")

	rec := compatDo(h, http.MethodGet, "/compat/graph/v1.0/me/events?"+graphWin, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
	}
	got := items(t, rec, "value")
	if len(got) != 2 || got[0]["subject"] != "Repaso" {
		t.Fatalf("eventos: %+v", got)
	}
	if start, _ := got[0]["start"].(map[string]any); start["timeZone"] != "Europe/Madrid" {
		t.Fatalf("start: %+v", got[0]["start"])
	}

	// $top pagina y publica @odata.nextLink absoluto, conservando el $filter.
	rec = compatDo(h, http.MethodGet, "/compat/graph/v1.0/me/events?"+graphWin+"&$top=1", "")
	body := compatBody(t, rec)
	next, _ := body["@odata.nextLink"].(string)
	if !strings.HasPrefix(next, "https://calendar.sytue.eu/compat/graph/v1.0/me/events?") ||
		!strings.Contains(next, "%24skip=1") || !strings.Contains(next, "%24filter=") {
		t.Fatalf("@odata.nextLink = %q", next)
	}
	if got := items(t, rec, "value"); len(got) != 1 {
		t.Fatalf("$top=1: %+v", got)
	}
	rec = compatDo(h, http.MethodGet, "/compat/graph/v1.0/me/events?"+graphWin+"&$top=1&$skip=1", "")
	if got := items(t, rec, "value"); len(got) != 1 || got[0]["subject"] != "Otro" {
		t.Fatalf("$skip=1: %+v", got)
	}
	if body := compatBody(t, rec); body["@odata.nextLink"] != nil {
		t.Fatalf("última página con nextLink: %s", rec.Body.String())
	}
}

func TestCompatGraphEventsFilterErrors(t *testing.T) {
	f := &fakeService{ensureCal: compatCalendar("user-1")}
	h := compatRouter(f, "user-1")

	tests := []struct{ name, query string }{
		{name: "sin $filter", query: ""},
		{name: "filtro no soportado", query: "?$filter=" + url.QueryEscape("subject eq 'Repaso'")},
		{name: "solo inicio", query: "?$filter=" + url.QueryEscape("start/dateTime ge '2026-07-01T00:00:00Z'")},
		{name: "parámetro no soportado", query: "?" + graphWin + "&$orderby=start/dateTime"},
		{name: "count no emulado", query: "?" + graphWin + "&$count=true"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := compatDo(h, http.MethodGet, "/compat/graph/v1.0/me/events"+tc.query, "")
			if rec.Code != http.StatusBadRequest || graphCodeOf(t, rec) != "invalidRequest" {
				t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCompatGraphEventGetAndNotImplemented(t *testing.T) {
	f := &fakeService{ensureCal: compatCalendar("user-1"), event: compatEvent()}
	h := compatRouter(f, "user-1")

	rec := compatDo(h, http.MethodGet, "/compat/graph/v1.0/me/events/"+compatEvID.String(), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
	}
	if body := compatBody(t, rec); body["id"] != compatEvID.String() || body["type"] != "singleInstance" {
		t.Fatalf("evento de Graph: %+v", body)
	}

	f.eventErr = domain.ErrNotFound
	rec = compatDo(h, http.MethodGet, "/compat/graph/v1.0/me/events/"+uuid.NewString(), "")
	if rec.Code != http.StatusNotFound || graphCodeOf(t, rec) != "itemNotFound" {
		t.Fatalf("no encontrado: %d (%s)", rec.Code, rec.Body.String())
	}

	rec = compatDo(h, http.MethodGet, "/compat/graph/v1.0/me/calendarView?startDateTime=x", "")
	if rec.Code != http.StatusNotImplemented || graphCodeOf(t, rec) != "notImplemented" {
		t.Fatalf("calendarView no emulado: %d (%s)", rec.Code, rec.Body.String())
	}
}

// --- Aislamiento -------------------------------------------------------------

// isolatedService reproduce la verja REAL del servicio: solo el dueño ve sus
// calendarios y eventos; para cualquier otro, simplemente no existen (404).
type isolatedService struct {
	*fakeService
	owner string
}

func (s *isolatedService) owns(ctx context.Context) bool {
	c, _ := authn.FromContext(ctx)
	return c.Subject == s.owner
}

func (s *isolatedService) GetCalendar(ctx context.Context, id uuid.UUID) (domain.Calendar, error) {
	if !s.owns(ctx) {
		return domain.Calendar{}, domain.ErrNotFound
	}
	return s.fakeService.GetCalendar(ctx, id)
}

func (s *isolatedService) GetEvent(ctx context.Context, id uuid.UUID) (domain.Event, error) {
	if !s.owns(ctx) {
		return domain.Event{}, domain.ErrNotFound
	}
	return s.fakeService.GetEvent(ctx, id)
}

func (s *isolatedService) ListInstances(ctx context.Context, id uuid.UUID, from, to time.Time) ([]domain.Instance, error) {
	if !s.owns(ctx) {
		return nil, domain.ErrNotFound
	}
	return s.fakeService.ListInstances(ctx, id, from, to)
}

func (s *isolatedService) ListEventsInRange(ctx context.Context, id uuid.UUID, from, to time.Time) ([]domain.Event, error) {
	if !s.owns(ctx) {
		return nil, domain.ErrNotFound
	}
	return s.fakeService.ListEventsInRange(ctx, id, from, to)
}

func (s *isolatedService) ListCalendars(ctx context.Context, sub string) ([]domain.Calendar, error) {
	if !s.owns(ctx) {
		return nil, nil
	}
	return s.fakeService.ListCalendars(ctx, sub)
}

func (s *isolatedService) SharedWithMe(ctx context.Context, sub, email string) ([]store.SharedCalendar, error) {
	if !s.owns(ctx) {
		return nil, nil
	}
	return s.fakeService.SharedWithMe(ctx, sub, email)
}

func (s *isolatedService) EnsureDefaultCalendar(ctx context.Context, sub string) (domain.Calendar, error) {
	if !s.owns(ctx) {
		// El intruso tiene SU propio calendario por defecto, no el ajeno.
		return domain.Calendar{ID: uuid.New(), OwnerSub: sub, Name: "personal", IsDefault: true}, nil
	}
	return s.fakeService.EnsureDefaultCalendar(ctx, sub)
}

// TestCompatAislamiento: el usuario B no ve por la vía compat el calendario ni los
// eventos de A — ni listando, ni pidiendo el id directo, en ninguno de los dos
// dialectos. «Ajeno» e «inexistente» responden lo mismo: 404.
func TestCompatAislamiento(t *testing.T) {
	own := compatCalendar("user-a")
	ev := compatEvent()
	base := &fakeService{
		ensureCal:   own,
		cal:         own,
		calendars:   []domain.Calendar{own},
		event:       ev,
		instances:   []domain.Instance{{Event: ev, Start: compatStart, End: compatStart.Add(time.Hour)}},
		rangeEvents: []domain.Event{ev},
	}
	svc := &isolatedService{fakeService: base, owner: "user-a"}

	// Control: el dueño sí ve lo suyo.
	owner := compatRouter(svc, "user-a")
	if got := items(t, compatDo(owner, http.MethodGet, "/compat/google/calendar/v3/users/me/calendarList", ""), "items"); len(got) != 1 {
		t.Fatalf("el dueño debe ver su calendario: %+v", got)
	}
	if rec := compatDo(owner, http.MethodGet, "/compat/google/calendar/v3/calendars/"+compatCalID.String()+"/events?"+compatWin, ""); rec.Code != http.StatusOK {
		t.Fatalf("el dueño debe ver sus eventos: %d", rec.Code)
	}

	intruder := compatRouter(svc, "user-b")
	listados := []struct{ name, target, key string }{
		{name: "google calendarList", target: "/compat/google/calendar/v3/users/me/calendarList", key: "items"},
		{name: "graph calendars", target: "/compat/graph/v1.0/me/calendars", key: "value"},
	}
	for _, tc := range listados {
		t.Run(tc.name, func(t *testing.T) {
			rec := compatDo(intruder, http.MethodGet, tc.target, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
			}
			for _, it := range items(t, rec, tc.key) {
				if it["id"] == compatCalID.String() {
					t.Fatalf("el intruso ve el calendario ajeno: %+v", it)
				}
			}
		})
	}

	directos := []struct{ name, target string }{
		{name: "google eventos del calendario ajeno", target: "/compat/google/calendar/v3/calendars/" + compatCalID.String() + "/events?" + compatWin},
		{name: "google evento ajeno por id", target: "/compat/google/calendar/v3/calendars/" + compatCalID.String() + "/events/" + compatEvID.String()},
		{name: "google ocurrencia ajena", target: "/compat/google/calendar/v3/calendars/" + compatCalID.String() + "/events/" + compatEvID.String() + "_20260728T090000Z"},
		{name: "graph eventos del calendario ajeno", target: "/compat/graph/v1.0/me/calendars/" + compatCalID.String() + "/events?" + graphWin},
		{name: "graph evento ajeno por id", target: "/compat/graph/v1.0/me/calendars/" + compatCalID.String() + "/events/" + compatEvID.String()},
	}
	for _, tc := range directos {
		t.Run(tc.name, func(t *testing.T) {
			rec := compatDo(intruder, http.MethodGet, tc.target, "")
			if rec.Code != http.StatusNotFound {
				t.Fatalf("estado = %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}

	// Y tampoco puede CREAR en el calendario ajeno.
	rec := compatDo(intruder, http.MethodPost, "/compat/google/calendar/v3/calendars/"+compatCalID.String()+"/events",
		`{"summary":"colado","start":{"date":"2026-07-28"},"end":{"date":"2026-07-29"}}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("crear en calendario ajeno: %d (%s)", rec.Code, rec.Body.String())
	}
}
