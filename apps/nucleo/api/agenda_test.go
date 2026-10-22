// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// La agenda elige los de día completo por su fecha UTC (flotantes) y el evento que nace
// de una nota va al calendario por defecto, de día completo, y vuelve como enlace.
func TestAgendaYEventoDelDia(t *testing.T) {
	var creado map[string]any
	var creadoEn string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/calendars":
			_, _ = io.WriteString(w, `{"calendars":[{"id":"fest","read_only":true},{"id":"pers","is_default":true}]}`)
		case r.URL.Path == "/api/v1/calendars/pers/instances":
			_, _ = io.WriteString(w, `{"instances":[
				{"event":{"id":"e2","uid":"u2","summary":"Comité"},"start":"2026-10-01T09:00:00Z"},
				{"event":{"id":"e1","uid":"u1","summary":"Víspera","all_day":true},"start":"2026-09-30T00:00:00Z"},
				{"event":{"id":"e3","uid":"u3","summary":"Fiesta","all_day":true},"start":"2026-10-01T00:00:00Z"}]}`)
		case r.URL.Path == "/api/v1/calendars/fest/instances":
			w.WriteHeader(http.StatusInternalServerError) // no se lleva a los demás
		case r.Method == http.MethodPost:
			creadoEn = r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&creado)
			_, _ = io.WriteString(w, `{"id":"e9","uid":"u9","summary":"Llamar"}`)
		default:
			t.Errorf("petición inesperada %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	e := Elegir{Apex: "sytue.eu", Calendar: NewCalendars(srv.URL, nil, nil)}
	ctx := context.Background()
	dia := time.Date(2026, 10, 1, 15, 0, 0, 0, time.FixedZone("CEST", 2*3600))
	evs, err := e.Agenda(ctx, dia)
	if err != nil || len(evs) != 2 || evs[0].ID != "u3" || evs[1].ID != "u2" || !evs[0].DiaCompleto {
		t.Fatalf("Agenda = %+v, %v", evs, err)
	}
	l, err := e.CrearEventoDelDia(ctx, "2026-12-31", " Llamar ")
	if err != nil || l.TargetKind != "calendar_event" || l.TargetID != "u9" || l.Label != "Llamar" || l.URL == "" {
		t.Fatalf("CrearEventoDelDia = %+v, %v", l, err)
	}
	if creadoEn != "/api/v1/calendars/pers/events" || creado["all_day"] != true ||
		creado["start"] != "2026-12-31T00:00:00Z" || creado["end"] != "2027-01-01T00:00:00Z" {
		t.Fatalf("creado en %s: %v", creadoEn, creado)
	}
	if _, err := e.CrearEventoDelDia(ctx, "31-12-2026", "x"); err == nil {
		t.Fatal("una fecha mal escrita crea un evento")
	}
}
