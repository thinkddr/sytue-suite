// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestRecordatoriosPeganDondeToca: las tres rutas y los nombres de los campos son los
// del servicio, que es quien manda.
func TestRecordatoriosPeganDondeToca(t *testing.T) {
	var visto []string
	var puesto map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visto = append(visto, r.Method+" "+r.URL.Path)
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"reminders":[{"note_id":"n1","notebook_id":"c1","due_at":"2026-10-03T07:00:00Z","all_day":true,"after_days":2,"tzid":"Europe/Madrid"}]}`)
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&puesto)
			_, _ = io.WriteString(w, `{"note_id":"n1","notebook_id":"c1","due_at":"2026-10-03T07:00:00Z","all_day":false,"rrule":"FREQ=DAILY","tzid":"UTC"}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	n, ctx := NewNotes(srv.URL, nil, nil), context.Background()

	rs, err := n.ListReminders(ctx)
	if err != nil || len(rs) != 1 || rs[0].NoteID != "n1" || !rs[0].AllDay || rs[0].AfterDays != 2 || !rs[0].Repite() {
		t.Fatalf("mis recordatorios: %+v %v", rs, err)
	}
	cuando := time.Date(2026, 10, 3, 7, 0, 0, 0, time.UTC)
	r, err := n.SetReminder(ctx, "n1", NoteReminderInput{DueAt: cuando, RRule: "FREQ=DAILY", TZID: "UTC"})
	if err != nil || r.RRule != "FREQ=DAILY" {
		t.Fatalf("poner: %+v %v", r, err)
	}
	for _, campo := range []string{"due_at", "all_day", "rrule", "after_days", "tzid"} {
		if _, ok := puesto[campo]; !ok {
			t.Errorf("poner no manda %q: %v", campo, puesto)
		}
	}
	if err := n.DeleteReminder(ctx, "n1"); err != nil {
		t.Fatal(err)
	}
	quiero := "GET /api/v1/reminders|PUT /api/v1/notes/n1/reminder|DELETE /api/v1/notes/n1/reminder"
	if strings.Join(visto, "|") != quiero {
		t.Errorf("peticiones %v, esperaba %s", visto, quiero)
	}
	rutas := webref.Read(t, "services/notes/internal/http/server.go")
	for _, ruta := range []string{`Get("/reminders"`, `Put("/notes/{id}/reminder"`, `Delete("/notes/{id}/reminder"`} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/notes ya no sirve %s: revisar notes_recordatorios.go", ruta)
		}
	}
}
