// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// TestSearchEventsUnauthorized: sin claims, 401.
func TestSearchEventsUnauthorized(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).SearchEvents(rec, req(http.MethodGet, "/?q=reunion", "", nil, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

// TestSearchEventsForwardsFilters: q, calendar, from, to, limit llegan al servicio
// tal cual (scope por dueño = sub autenticado).
func TestSearchEventsForwardsFilters(t *testing.T) {
	calID := uuid.New()
	f := &fakeService{searchEvents: []domain.Event{}}
	rec := httptest.NewRecorder()
	target := "/?q=reunion&calendar=" + calID.String() +
		"&from=2026-07-01T00:00:00Z&to=2026-08-01T00:00:00Z&limit=25"
	newHandlers(f).SearchEvents(rec, req(http.MethodGet, target, "", authClaims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if f.gotSearch.OwnerSub != "user-1" {
		t.Errorf("owner = %q, want user-1 (scope por dueño)", f.gotSearch.OwnerSub)
	}
	if f.gotSearch.Text != "reunion" {
		t.Errorf("text = %q, want reunion", f.gotSearch.Text)
	}
	if f.gotSearch.CalendarID != calID {
		t.Errorf("calendar = %v, want %v", f.gotSearch.CalendarID, calID)
	}
	if f.gotSearch.From == nil || !f.gotSearch.From.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("from = %v, want 2026-07-01Z", f.gotSearch.From)
	}
	if f.gotSearch.To == nil || !f.gotSearch.To.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("to = %v, want 2026-08-01Z", f.gotSearch.To)
	}
	if f.gotSearch.Limit != 25 {
		t.Errorf("limit = %d, want 25", f.gotSearch.Limit)
	}
}

// TestSearchEventsNextCursorRoundTrip: página llena => next_cursor no vacío, y ese
// cursor decodifica al (dtstart, id) del último resultado en la siguiente llamada.
func TestSearchEventsNextCursorRoundTrip(t *testing.T) {
	last := domain.Event{ID: uuid.New(), DTStart: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	f := &fakeService{searchEvents: []domain.Event{{ID: uuid.New()}, last}}
	rec := httptest.NewRecorder()
	// limit=2 y devuelve 2 => página llena => hay next_cursor.
	newHandlers(f).SearchEvents(rec, req(http.MethodGet, "/?q=reunion&limit=2", "", authClaims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	m := decodeEnvelope(t, rec)
	next, _ := m["next_cursor"].(string)
	if next == "" {
		t.Fatal("next_cursor vacío en página llena")
	}
	// La segunda llamada con ?cursor= decodifica al keyset del último resultado.
	rec2 := httptest.NewRecorder()
	newHandlers(f).SearchEvents(rec2, req(http.MethodGet, "/?q=reunion&limit=2&cursor="+next, "", authClaims(), nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("segunda página code = %d, want 200", rec2.Code)
	}
	if f.gotSearch.After == nil {
		t.Fatal("After nil: el cursor no se decodificó")
	}
	if f.gotSearch.After.ID != last.ID || !f.gotSearch.After.DTStart.Equal(last.DTStart) {
		t.Errorf("cursor decodificado = %+v, want id=%v dtstart=%v", f.gotSearch.After, last.ID, last.DTStart)
	}
}

// TestSearchEventsPartialPageNoCursor: página no llena => sin next_cursor.
func TestSearchEventsPartialPageNoCursor(t *testing.T) {
	f := &fakeService{searchEvents: []domain.Event{{ID: uuid.New()}}}
	rec := httptest.NewRecorder()
	newHandlers(f).SearchEvents(rec, req(http.MethodGet, "/?q=reunion&limit=50", "", authClaims(), nil))
	m := decodeEnvelope(t, rec)
	if next, _ := m["next_cursor"].(string); next != "" {
		t.Errorf("next_cursor = %q, want vacío (página parcial)", next)
	}
	if _, ok := m["events"]; !ok {
		t.Errorf("falta clave 'events': %v", m)
	}
}

// TestSearchEventsBadParams: filtros mal formados => 400 (no 500/200).
func TestSearchEventsBadParams(t *testing.T) {
	cases := []string{
		"/?q=x&calendar=not-a-uuid",
		"/?q=x&from=ayer",
		"/?q=x&to=manana",
		"/?q=x&cursor=abcd", // base64 válido pero sin separador "|": cursor inválido
	}
	for _, target := range cases {
		rec := httptest.NewRecorder()
		newHandlers(&fakeService{}).SearchEvents(rec, req(http.MethodGet, target, "", authClaims(), nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d, want 400 (%s)", target, rec.Code, rec.Body.String())
		}
	}
}
