// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate_test

// Tests OFFLINE de la agenda. Lo que se prueba es lo que se pierde en una
// migración de calendario mal hecha: la SERIE («todos los martes») convertida en
// cientos de eventos sueltos, la zona horaria, y quién estaba invitado con qué
// respuesta.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/migrate"
)

func gcalServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	mux.HandleFunc("/calendar/v3/users/me/calendarList", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"items": []map[string]any{
			{"id": "primary", "summary": "Ana Ruiz", "timeZone": "Europe/Madrid", "accessRole": "owner", "primary": true},
			{"id": "equipo@acme", "summary": "Equipo", "timeZone": "Europe/Madrid", "accessRole": "writer"},
			{"id": "festivos@group", "summary": "Festivos", "accessRole": "reader"},
		}})
	})
	mux.HandleFunc("/calendar/v3/calendars/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/events") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		cal := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/calendar/v3/calendars/"), "/events")
		if cal != "primary" {
			writeJSON(w, map[string]any{"items": []any{}})
			return
		}
		writeJSON(w, map[string]any{"items": []map[string]any{
			{
				"id": "e1", "status": "confirmed", "summary": "Semanal de producto",
				"iCalUID":    "e1@google.com",
				"recurrence": []string{"RRULE:FREQ=WEEKLY;BYDAY=TU;UNTIL=20260630T215959Z", "EXDATE;TZID=Europe/Madrid:20260421T100000"},
				"organizer":  map[string]any{"email": "ana@old.example"},
				"start":      map[string]any{"dateTime": "2026-03-03T10:00:00+01:00", "timeZone": "Europe/Madrid"},
				"end":        map[string]any{"dateTime": "2026-03-03T11:00:00+01:00", "timeZone": "Europe/Madrid"},
				"attendees": []map[string]any{
					{"email": "luis@old.example", "displayName": "Luis", "responseStatus": "accepted"},
					{"email": "sala@old.example", "resource": true, "responseStatus": "needsAction"},
					{"email": "eva@old.example", "optional": true, "responseStatus": "declined"},
				},
			},
			{
				"id": "e2", "status": "confirmed", "summary": "Semanal (movido)",
				"recurringEventId": "e1",
				"start":            map[string]any{"dateTime": "2026-04-21T17:00:00+02:00"},
				"end":              map[string]any{"dateTime": "2026-04-21T18:00:00+02:00"},
			},
			{
				"id": "e3", "status": "confirmed", "summary": "Festivo",
				"start": map[string]any{"date": "2026-05-01"},
				"end":   map[string]any{"date": "2026-05-02"},
			},
		}})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

type createdEvent struct {
	Calendar string
	Body     map[string]any
}

type fakeCal struct {
	srv       *httptest.Server
	calendars map[string]string // id → nombre
	events    []createdEvent
	next      int
}

func newFakeCal(t *testing.T) *fakeCal {
	t.Helper()
	f := &fakeCal{calendars: map[string]string{"cal-primary": "Mi calendario"}}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}

	mux.HandleFunc("/api/v1/calendars", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			list := []map[string]any{{"id": "cal-primary", "name": "Mi calendario", "primary": true}}
			writeJSON(w, http.StatusOK, map[string]any{"calendars": list})
			return
		}
		var body struct{ Name string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.next++
		id := "cal-" + body.Name
		f.calendars[id] = body.Name
		writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": body.Name})
	})
	mux.HandleFunc("/api/v1/calendars/", func(w http.ResponseWriter, r *http.Request) {
		cal := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/calendars/"), "/events")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.next++
		f.events = append(f.events, createdEvent{Calendar: cal, Body: body})
		writeJSON(w, http.StatusCreated, map[string]any{"id": "ev-" + strings.TrimSpace(cal)})
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// TestCalendarioLlegaConSerieYAsistentes: la serie viaja como serie (con su
// regla y su excepción), los asistentes conservan rol y respuesta, y el
// calendario principal se funde con el de destino en vez de duplicarse.
func TestCalendarioLlegaConSerieYAsistentes(t *testing.T) {
	allowLoopback(t)
	gsrv := gcalServer(t)
	cal := newFakeCal(t)

	inst := googleConsentInstance(gsrv.URL)
	inst.Config[migrate.ConfigGCalBase] = gsrv.URL

	j := job("job-cal", domain.SourceGoogle, domain.DomainCalendar)
	j.SourceUser = ""
	st := newMemStore("org-1")
	st.put(j)
	src, err := migrate.NewSource(context.Background(), gmailRuntime(), inst, j)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	deps := migrate.Deps{
		Src:   src,
		Cal:   migrate.NewSytueCalendar(cal.srv.URL, migrate.StaticToken("tok"), nil),
		Store: st,
	}
	if err := migrate.Run(context.Background(), deps, j); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := st.get("job-cal")
	if got.Status != domain.MigrationDone {
		t.Fatalf("estado final = %q (%s)", got.Status, got.LastError)
	}

	// El calendario principal NO se duplica; el de equipo sí se crea.
	if _, ok := cal.calendars["cal-Equipo"]; !ok {
		t.Errorf("no se creó el calendario de equipo: %v", cal.calendars)
	}
	if len(cal.calendars) != 2 {
		t.Errorf("calendarios en destino = %v; el principal debía fundirse con el existente", cal.calendars)
	}

	// La serie, con su regla y su excepción.
	var serie map[string]any
	for _, e := range cal.events {
		if e.Body["summary"] == "Semanal de producto" {
			serie = e.Body
		}
	}
	if serie == nil {
		t.Fatal("no se creó el evento que se repite")
	}
	if rrule, _ := serie["rrule"].(string); !strings.Contains(rrule, "FREQ=WEEKLY") {
		t.Errorf("la serie perdió su regla de repetición: %v", serie["rrule"])
	}
	if ex, _ := serie["exdate"].([]any); len(ex) != 1 {
		t.Errorf("la serie perdió su excepción: %v", serie["exdate"])
	}
	if tz, _ := serie["tzid"].(string); tz != "Europe/Madrid" {
		t.Errorf("se perdió la zona horaria: %q", tz)
	}
	people, _ := serie["attendees"].([]any)
	if len(people) != 3 {
		t.Fatalf("asistentes = %d, quiero 3", len(people))
	}
	var vistos int
	for _, raw := range people {
		a, _ := raw.(map[string]any)
		switch a["email"] {
		case "luis@old.example":
			if a["partstat"] != "ACCEPTED" || a["role"] != "REQ-PARTICIPANT" {
				t.Errorf("se perdió la respuesta de un asistente: %v", a)
			}
			vistos++
		case "sala@old.example":
			if a["cutype"] != "RESOURCE" {
				t.Errorf("una sala llegó como persona: %v", a)
			}
			vistos++
		case "eva@old.example":
			if a["role"] != "OPT-PARTICIPANT" || a["partstat"] != "DECLINED" {
				t.Errorf("se perdió el carácter opcional o el rechazo: %v", a)
			}
			vistos++
		}
	}
	if vistos != 3 {
		t.Errorf("no se reconocieron los tres asistentes")
	}
	// Un evento migrado NO vuelve a invitar a nadie.
	if send, _ := serie["send"].(bool); send {
		t.Error("la migración iba a mandar invitaciones de eventos que ya existían")
	}

	// La excepción movida y el calendario ajeno se omiten CON motivo.
	items, _ := st.LoadMigrationMap(context.Background(), "job-cal")
	var exSkip, ajeno string
	for _, it := range items {
		if it.SourceID == "event:e2" {
			exSkip = it.SkipReason
		}
		if it.SourceID == "cal:festivos@group" {
			ajeno = it.SkipReason
		}
	}
	if exSkip == "" || ajeno == "" {
		t.Errorf("faltan motivos de omisión: excepción=%q ajeno=%q", exSkip, ajeno)
	}

	// Segunda pasada: idempotente.
	before := len(cal.events)
	j2, _ := st.get("job-cal")
	if err := migrate.Run(context.Background(), deps, *j2); err != nil {
		t.Fatalf("Run (2.ª pasada): %v", err)
	}
	if len(cal.events) != before {
		t.Errorf("la segunda pasada duplicó la agenda (%d → %d)", before, len(cal.events))
	}
}
