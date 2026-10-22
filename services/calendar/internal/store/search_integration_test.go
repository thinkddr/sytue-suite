// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integracion de la BÚSQUEDA server-side de eventos (search.go) contra un
// PostgreSQL real (pgtest/testcontainers). Verifican: casar por título/descripción/
// lugar/asistente; filtro por rango y por calendario; que una serie recurrente
// aparece UNA vez (su maestro); aislamiento por DUEÑO (no ve eventos de otro
// owner_sub del mismo tenant); y que el keyset pagina TODOS los resultados sin
// duplicar ni bucle. Reutiliza tenantCtx del harness del paquete.
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// searchEvent construye un EventInput mínimo para poblar la búsqueda.
func searchEvent(calID uuid.UUID, uid, summary, description, location string, start time.Time) domain.EventInput {
	return domain.EventInput{
		CalendarID:  calID,
		UID:         uid,
		Summary:     summary,
		Description: description,
		Location:    location,
		DTStart:     start,
		DTEnd:       start.Add(time.Hour),
		ICalData:    "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:" + uid + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
	}
}

func mustCal(t *testing.T, st *store.Store, ctx context.Context, owner, name string) domain.Calendar {
	t.Helper()
	c, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: owner, Name: name, DisplayName: name})
	if err != nil {
		t.Fatalf("CreateCalendar(%s/%s): %v", owner, name, err)
	}
	return c
}

// TestSearchEventsByFields casa por título, descripción, lugar y asistente.
func TestSearchEventsByFields(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	cal := mustCal(t, st, ctx, "u-1", "trabajo")

	base := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	if _, _, err := st.CreateEvent(ctx, searchEvent(cal.ID, "e-title", "Planificación trimestral", "", "", base)); err != nil {
		t.Fatalf("crear e-title: %v", err)
	}
	if _, _, err := st.CreateEvent(ctx, searchEvent(cal.ID, "e-desc", "Café", "hablar del presupuesto anual", "", base.Add(time.Hour))); err != nil {
		t.Fatalf("crear e-desc: %v", err)
	}
	if _, _, err := st.CreateEvent(ctx, searchEvent(cal.ID, "e-loc", "Comida", "", "Restaurante Girasol", base.Add(2*time.Hour))); err != nil {
		t.Fatalf("crear e-loc: %v", err)
	}
	att := searchEvent(cal.ID, "e-att", "1:1", "", "", base.Add(3*time.Hour))
	att.Attendees = []domain.Attendee{{Email: "ariadna@ejemplo.com", CommonName: "Ariadna Ruiz"}}
	if _, _, err := st.CreateEvent(ctx, att); err != nil {
		t.Fatalf("crear e-att: %v", err)
	}

	cases := []struct {
		term string
		uid  string
	}{
		{"trimestral", "e-title"}, // título
		{"presupuesto", "e-desc"}, // descripción
		{"girasol", "e-loc"},      // lugar (case-insensitive)
		{"ariadna", "e-att"},      // asistente (nombre)
		{"ejemplo.com", "e-att"},  // asistente (email)
	}
	for _, c := range cases {
		got, err := st.SearchEvents(ctx, store.EventSearchQuery{OwnerSub: "u-1", Text: c.term, Limit: 10})
		if err != nil {
			t.Fatalf("SearchEvents(%q): %v", c.term, err)
		}
		if len(got) != 1 || got[0].UID != c.uid {
			t.Errorf("SearchEvents(%q) = %d resultados (%v), want 1 (%s)", c.term, len(got), uids(got), c.uid)
		}
	}

	// Término vacío/corto no vuelca el calendario.
	if got, _ := st.SearchEvents(ctx, store.EventSearchQuery{OwnerSub: "u-1", Text: "", Limit: 10}); len(got) != 0 {
		t.Errorf("término vacío devolvió %d, want 0", len(got))
	}
	if got, _ := st.SearchEvents(ctx, store.EventSearchQuery{OwnerSub: "u-1", Text: "a", Limit: 10}); len(got) != 0 {
		t.Errorf("término de 1 char devolvió %d, want 0", len(got))
	}
}

// TestSearchEventsRangeAndCalendar filtra por rango y por calendario.
func TestSearchEventsRangeAndCalendar(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	work := mustCal(t, st, ctx, "u-1", "trabajo")
	home := mustCal(t, st, ctx, "u-1", "personal")

	jul := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)
	sep := time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)
	if _, _, err := st.CreateEvent(ctx, searchEvent(work.ID, "w-jul", "Repaso julio", "", "", jul)); err != nil {
		t.Fatalf("w-jul: %v", err)
	}
	if _, _, err := st.CreateEvent(ctx, searchEvent(work.ID, "w-sep", "Repaso septiembre", "", "", sep)); err != nil {
		t.Fatalf("w-sep: %v", err)
	}
	if _, _, err := st.CreateEvent(ctx, searchEvent(home.ID, "h-jul", "Repaso casa", "", "", jul.Add(time.Hour))); err != nil {
		t.Fatalf("h-jul: %v", err)
	}

	// Rango [ago, oct): solo el de septiembre.
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	got, err := st.SearchEvents(ctx, store.EventSearchQuery{OwnerSub: "u-1", Text: "repaso", From: &from, To: &to, Limit: 10})
	if err != nil {
		t.Fatalf("rango: %v", err)
	}
	if len(got) != 1 || got[0].UID != "w-sep" {
		t.Errorf("rango [ago,oct) = %v, want [w-sep]", uids(got))
	}

	// Filtro por calendario (trabajo) sin rango: 2 de trabajo, ninguno de casa.
	got, err = st.SearchEvents(ctx, store.EventSearchQuery{OwnerSub: "u-1", Text: "repaso", CalendarID: work.ID, Limit: 10})
	if err != nil {
		t.Fatalf("por calendario: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("por calendario trabajo = %v, want 2 (w-jul,w-sep)", uids(got))
	}
	for _, e := range got {
		if e.CalendarID != work.ID {
			t.Errorf("resultado de otro calendario: %v", e.UID)
		}
	}
}

// TestSearchEventsRecurringOnce: una serie recurrente aparece UNA vez (su maestro,
// marcado recurrente), no una fila por ocurrencia.
func TestSearchEventsRecurringOnce(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	cal := mustCal(t, st, ctx, "u-1", "trabajo")

	in := searchEvent(cal.ID, "rec-1", "Daily standup", "", "", time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC))
	in.RRule = "FREQ=DAILY;COUNT=30"
	if _, _, err := st.CreateEvent(ctx, in); err != nil {
		t.Fatalf("crear recurrente: %v", err)
	}
	got, err := st.SearchEvents(ctx, store.EventSearchQuery{OwnerSub: "u-1", Text: "standup", Limit: 100})
	if err != nil {
		t.Fatalf("buscar recurrente: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("serie recurrente devolvió %d filas, want 1 (el maestro)", len(got))
	}
	if !got[0].IsRecurring() {
		t.Errorf("el resultado no está marcado como recurrente (rrule=%q)", got[0].RRule)
	}
}

// TestSearchEventsOwnerIsolation: la búsqueda de un dueño NO ve eventos de otro
// owner_sub del mismo tenant (aislamiento por dueño además del tenant por RLS).
func TestSearchEventsOwnerIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	calA := mustCal(t, st, ctx, "u-1", "cal-a")
	calB := mustCal(t, st, ctx, "u-2", "cal-b")

	base := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	if _, _, err := st.CreateEvent(ctx, searchEvent(calA.ID, "a-1", "Secreto de Ana", "", "", base)); err != nil {
		t.Fatalf("a-1: %v", err)
	}
	if _, _, err := st.CreateEvent(ctx, searchEvent(calB.ID, "b-1", "Secreto de Bruno", "", "", base)); err != nil {
		t.Fatalf("b-1: %v", err)
	}

	// u-1 busca "secreto": solo ve el suyo.
	got, err := st.SearchEvents(ctx, store.EventSearchQuery{OwnerSub: "u-1", Text: "secreto", Limit: 10})
	if err != nil {
		t.Fatalf("u-1 buscar: %v", err)
	}
	if len(got) != 1 || got[0].UID != "a-1" {
		t.Fatalf("u-1 ve eventos ajenos: %v, want [a-1]", uids(got))
	}
	// Aunque conozca el ID del calendario de u-2, el filtro por dueño lo tapa.
	got, err = st.SearchEvents(ctx, store.EventSearchQuery{OwnerSub: "u-1", Text: "secreto", CalendarID: calB.ID, Limit: 10})
	if err != nil {
		t.Fatalf("u-1 buscar en cal de u-2: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("u-1 buscó en el calendario de u-2 y vio %v, want 0", uids(got))
	}
}

// TestSearchEventsKeyset: el keyset pagina TODOS los resultados, en orden
// (dtstart,id) ascendente, sin duplicar ni bucle.
func TestSearchEventsKeyset(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	cal := mustCal(t, st, ctx, "u-1", "trabajo")

	const total = 7
	base := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < total; i++ {
		uid := "k-" + string(rune('a'+i))
		if _, _, err := st.CreateEvent(ctx, searchEvent(cal.ID, uid, "Sprint review", "", "", base.Add(time.Duration(i)*time.Hour))); err != nil {
			t.Fatalf("crear %s: %v", uid, err)
		}
	}

	seen := map[uuid.UUID]bool{}
	var cursor *store.EventSearchCursor
	var last time.Time
	for pages := 0; ; pages++ {
		if pages > total+2 {
			t.Fatal("keyset no termina (posible cursor sin avanzar)")
		}
		page, err := st.SearchEvents(ctx, store.EventSearchQuery{
			OwnerSub: "u-1", Text: "sprint", After: cursor, Limit: 3,
		})
		if err != nil {
			t.Fatalf("página %d: %v", pages, err)
		}
		if len(page) == 0 {
			break
		}
		for _, e := range page {
			if seen[e.ID] {
				t.Fatalf("keyset duplicó el evento %v", e.UID)
			}
			seen[e.ID] = true
			if e.DTStart.Before(last) {
				t.Fatalf("orden no ascendente: %v antes de %v", e.DTStart, last)
			}
			last = e.DTStart
		}
		if len(page) < 3 {
			break
		}
		tail := page[len(page)-1]
		cursor = &store.EventSearchCursor{DTStart: tail.DTStart, ID: tail.ID}
	}
	if len(seen) != total {
		t.Fatalf("keyset vio %d eventos, want %d", len(seen), total)
	}
}

func uids(evs []domain.Event) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.UID
	}
	return out
}
