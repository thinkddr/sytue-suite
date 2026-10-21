// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de zona horaria del puente iCalendar: DTSTART;TZID se resuelve al
// instante UTC correcto (DST), el TZID se persiste, el recordatorio (VALARM) se
// calcula desde ese instante y EncodeEvent hace round-trip del TZID sin pérdida.
package caldav

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/calendar/internal/alarm"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// madridEventCEST: DTSTART;TZID=Europe/Madrid a las 09:00 del 2025-03-31, ya en
// horario de verano (CEST, +2) tras el spring-forward del 2025-03-30 => 07:00Z.
// VALARM -PT15M => el recordatorio dispara a las 06:45Z.
const madridEventCEST = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//Sytue//Test//ES\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:tz-1@sytue.eu\r\n" +
	"DTSTAMP:20250301T120000Z\r\n" +
	"DTSTART;TZID=Europe/Madrid:20250331T090000\r\n" +
	"DTEND;TZID=Europe/Madrid:20250331T100000\r\n" +
	"SUMMARY:Reunion Madrid\r\n" +
	"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

// TestDecodeTZIDResolvesInstantAndPersistsZone: el parseo respeta DTSTART;TZID,
// resuelve el instante UTC con DST correcto y conserva el TZID en el modelo.
func TestDecodeTZIDResolvesInstantAndPersistsZone(t *testing.T) {
	po, err := DecodeCalendarObject(madridEventCEST, "cal-1")
	if err != nil {
		t.Fatalf("DecodeCalendarObject: %v", err)
	}
	if want := time.Date(2025, 3, 31, 7, 0, 0, 0, time.UTC); !po.Master.DTStart.Equal(want) {
		t.Errorf("DTStart = %s, want %s (09:00 CEST = 07:00Z)", po.Master.DTStart.UTC(), want)
	}
	if po.Master.TZID != "Europe/Madrid" {
		t.Errorf("TZID = %q, want Europe/Madrid", po.Master.TZID)
	}
}

// TestReminderFromCorrectInstant: el recordatorio se calcula desde el instante
// UTC correcto según TZID, no desde una hora naíf. 09:00 CEST = 07:00Z; -15 min
// => 06:45Z.
func TestReminderFromCorrectInstant(t *testing.T) {
	po, err := DecodeCalendarObject(madridEventCEST, "cal-1")
	if err != nil {
		t.Fatalf("DecodeCalendarObject: %v", err)
	}
	triggers, err := alarm.Triggers(madridEventCEST, po.Master.DTStart, po.Master.DTEnd)
	if err != nil {
		t.Fatalf("alarm.Triggers: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("got %d triggers, want 1", len(triggers))
	}
	if want := time.Date(2025, 3, 31, 6, 45, 0, 0, time.UTC); !triggers[0].At.Equal(want) {
		t.Errorf("trigger_at = %s, want %s (07:00Z - 15m)", triggers[0].At.UTC(), want)
	}
}

// TestEncodeEventRoundTripsTZID: EncodeEvent emite DTSTART;TZID=<IANA> para un
// evento con zona (no UTC), y al re-decodificarlo se recupera el MISMO instante
// UTC y el MISMO TZID: round-trip CalDAV sin pérdida.
func TestEncodeEventRoundTripsTZID(t *testing.T) {
	ev := domain.Event{
		UID:     "rt-1@sytue.eu",
		Summary: "Round-trip",
		DTStart: time.Date(2025, 3, 31, 7, 0, 0, 0, time.UTC), // 09:00 CEST
		DTEnd:   time.Date(2025, 3, 31, 8, 0, 0, 0, time.UTC),
		TZID:    "Europe/Madrid",
		Status:  domain.StatusConfirmed,
	}
	data, err := EncodeEvent(ev, "")
	if err != nil {
		t.Fatalf("EncodeEvent: %v", err)
	}
	if !strings.Contains(data, "TZID=Europe/Madrid") {
		t.Fatalf("el iCal no lleva TZID=Europe/Madrid:\n%s", data)
	}
	po, err := DecodeCalendarObject(data, "cal-1")
	if err != nil {
		t.Fatalf("re-decodificar: %v", err)
	}
	if !po.Master.DTStart.Equal(ev.DTStart) {
		t.Errorf("round-trip DTStart = %s, want %s", po.Master.DTStart.UTC(), ev.DTStart.UTC())
	}
	if po.Master.TZID != "Europe/Madrid" {
		t.Errorf("round-trip TZID = %q, want Europe/Madrid", po.Master.TZID)
	}
}

// TestEncodeEventUTCStaysUTC: un evento en UTC sigue serializándose como ...Z (sin
// TZID), preservando el comportamiento anterior para eventos sin zona.
func TestEncodeEventUTCStaysUTC(t *testing.T) {
	ev := domain.Event{
		UID:     "utc-1@sytue.eu",
		Summary: "UTC",
		DTStart: time.Date(2025, 6, 1, 9, 0, 0, 0, time.UTC),
		DTEnd:   time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC),
		TZID:    "UTC",
		Status:  domain.StatusConfirmed,
	}
	data, err := EncodeEvent(ev, "")
	if err != nil {
		t.Fatalf("EncodeEvent: %v", err)
	}
	if strings.Contains(data, "TZID=") {
		t.Errorf("evento UTC no debe llevar TZID:\n%s", data)
	}
	if !strings.Contains(data, "DTSTART:20250601T090000Z") {
		t.Errorf("falta DTSTART UTC (...Z):\n%s", data)
	}
}
