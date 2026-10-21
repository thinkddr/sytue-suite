// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// feedSample es un feed ICS EXTERNO de muestra: dos VEVENT con UIDs distintos —
// una festividad puntual y una serie semanal (RRULE) — como los de un calendario
// de festivos/equipo suscrito.
const feedSample = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\nPRODID:-//Ajeno//Feed//EN\r\n" +
	"BEGIN:VEVENT\r\nUID:holiday-2026@ext\r\n" +
	"DTSTART;VALUE=DATE:20260101\r\nDTEND;VALUE=DATE:20260102\r\n" +
	"SUMMARY:Año Nuevo\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:standup@ext\r\n" +
	"DTSTART:20260105T090000Z\r\nDTEND:20260105T091500Z\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=MO;COUNT=6\r\n" +
	"SUMMARY:Standup semanal\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

// TestDecodeICSFeed verifica que un feed multi-VEVENT se parte en un recurso por
// UID reutilizando el parser existente, conservando la RRULE, y que la serie
// expande varias ocurrencias (recurrencia del feed externo respetada).
func TestDecodeICSFeed(t *testing.T) {
	objs, err := DecodeICSFeed(feedSample, 0)
	if err != nil {
		t.Fatalf("DecodeICSFeed: %v", err)
	}
	if len(objs) != 2 {
		t.Fatalf("objetos = %d, want 2", len(objs))
	}

	byUID := map[string]ParsedObject{}
	for _, o := range objs {
		byUID[o.UID] = o
	}

	holiday, ok := byUID["holiday-2026@ext"]
	if !ok || holiday.Master.Summary != "Año Nuevo" || !holiday.Master.AllDay {
		t.Fatalf("festividad mal parseada: %+v", holiday.Master)
	}

	standup, ok := byUID["standup@ext"]
	if !ok {
		t.Fatal("falta el evento recurrente standup@ext")
	}
	if standup.Master.RRule == "" {
		t.Fatalf("RRULE no conservada: %+v", standup.Master)
	}

	// La serie semanal (COUNT=6) expande 6 instancias en una ventana amplia.
	ev := domain.Event{
		UID:     standup.Master.UID,
		DTStart: standup.Master.DTStart,
		DTEnd:   standup.Master.DTEnd,
		TZID:    standup.Master.TZID,
		RRule:   standup.Master.RRule,
		Status:  domain.StatusConfirmed,
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	insts := domain.ExpandEvents([]domain.Event{ev}, from, to)
	if len(insts) != 6 {
		t.Fatalf("instancias expandidas = %d, want 6 (COUNT=6)", len(insts))
	}
}

// TestDecodeICSFeedCap verifica el tope de objetos (defensa frente a feeds
// abusivos).
func TestDecodeICSFeedCap(t *testing.T) {
	if _, err := DecodeICSFeed(feedSample, 1); err == nil {
		t.Error("DecodeICSFeed con maxObjects=1 sobre 2 eventos = nil, want error")
	}
}
