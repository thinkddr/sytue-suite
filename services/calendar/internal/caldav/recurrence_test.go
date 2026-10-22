// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import (
	"strings"
	"testing"
	"time"
)

// seriesWithOverride es una serie semanal (COUNT=10) con VALARM, ATTENDEE, un
// EXDATE previo y un VEVENT override (RECURRENCE-ID): el caso que estresa la
// preservación al recortar el maestro.
const seriesWithOverride = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//Sytue//Test//ES\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:series-1@sytue.eu\r\n" +
	"DTSTAMP:20260630T120000Z\r\n" +
	"DTSTART:20260701T090000Z\r\n" +
	"DTEND:20260701T100000Z\r\n" +
	"SUMMARY:Reunion semanal\r\n" +
	"RRULE:FREQ=WEEKLY;COUNT=10;BYDAY=WE\r\n" +
	"EXDATE:20260708T090000Z\r\n" +
	"ATTENDEE;ROLE=REQ-PARTICIPANT;RSVP=TRUE:mailto:ana@sytue.eu\r\n" +
	"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:series-1@sytue.eu\r\n" +
	"DTSTAMP:20260630T120000Z\r\n" +
	"RECURRENCE-ID:20260715T090000Z\r\n" +
	"DTSTART:20260715T110000Z\r\n" +
	"DTEND:20260715T120000Z\r\n" +
	"SUMMARY:Reunion semanal (movida)\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

// target es el inicio original de la instancia que se edita/borra.
var target = time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)

func TestAmendExcludePreservesEverything(t *testing.T) {
	out, err := AmendMasterRecurrence(seriesWithOverride, target, AmendExclude)
	if err != nil {
		t.Fatalf("AmendMasterRecurrence: %v", err)
	}
	// EXDATE nuevo AÑADIDO, el previo conservado.
	if !strings.Contains(out, "20260722T090000Z") {
		t.Errorf("falta el EXDATE nuevo (instancia excluida) en:\n%s", out)
	}
	if !strings.Contains(out, "20260708T090000Z") {
		t.Errorf("se perdió el EXDATE previo en:\n%s", out)
	}
	// El RRULE del maestro NO se toca en «solo este».
	if !strings.Contains(out, "COUNT=10") || strings.Contains(out, "UNTIL=") {
		t.Errorf("el RRULE no debe cambiar al excluir; got:\n%s", out)
	}
	// VALARM, ATTENDEE y el override (RECURRENCE-ID) sobreviven.
	assertPreserved(t, out)
}

func TestAmendUntilCapsMasterOnly(t *testing.T) {
	out, err := AmendMasterRecurrence(seriesWithOverride, target, AmendUntil)
	if err != nil {
		t.Fatalf("AmendMasterRecurrence: %v", err)
	}
	// UNTIL un segundo antes de la instancia; COUNT descartado; BYDAY conservado.
	if !strings.Contains(out, "UNTIL=20260722T085959Z") {
		t.Errorf("falta UNTIL recortado en:\n%s", out)
	}
	if strings.Contains(out, "COUNT=") {
		t.Errorf("COUNT debe descartarse al fijar UNTIL; got:\n%s", out)
	}
	if !strings.Contains(out, "BYDAY=WE") {
		t.Errorf("se perdió BYDAY (componente no interpretado) en:\n%s", out)
	}
	// El EXDATE previo y el override no se tocan al recortar el maestro.
	if !strings.Contains(out, "20260708T090000Z") {
		t.Errorf("se perdió el EXDATE previo en:\n%s", out)
	}
	assertPreserved(t, out)
}

func TestAmendAllDay(t *testing.T) {
	allDay := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Sytue//Test//ES\r\nBEGIN:VEVENT\r\n" +
		"UID:ad-1@sytue.eu\r\n" +
		"DTSTAMP:20260630T120000Z\r\n" +
		"DTSTART;VALUE=DATE:20260701\r\n" +
		"DTEND;VALUE=DATE:20260702\r\n" +
		"SUMMARY:Diario\r\n" +
		"RRULE:FREQ=DAILY;COUNT=30\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	day := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)

	ex, err := AmendMasterRecurrence(allDay, day, AmendExclude)
	if err != nil {
		t.Fatalf("exclude: %v", err)
	}
	if !strings.Contains(ex, "EXDATE;VALUE=DATE:20260715") {
		t.Errorf("EXDATE de día completo mal formado en:\n%s", ex)
	}

	un, err := AmendMasterRecurrence(allDay, day, AmendUntil)
	if err != nil {
		t.Fatalf("until: %v", err)
	}
	// Día completo: el último día conservado es el anterior (DATE, sin hora).
	if !strings.Contains(un, "UNTIL=20260714") || strings.Contains(un, "UNTIL=20260714T") {
		t.Errorf("UNTIL de día completo debe ser DATE del día anterior; got:\n%s", un)
	}
}

func TestAmendUntilNoRRuleErrors(t *testing.T) {
	single := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Sytue//Test//ES\r\nBEGIN:VEVENT\r\n" +
		"UID:x@sytue.eu\r\nDTSTAMP:20260630T120000Z\r\nDTSTART:20260701T090000Z\r\nDTEND:20260701T100000Z\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	if _, err := AmendMasterRecurrence(single, target, AmendUntil); err == nil {
		t.Error("AmendUntil sobre un evento sin RRULE debe fallar")
	}
}

func TestAmendUnknownMode(t *testing.T) {
	if _, err := AmendMasterRecurrence(seriesWithOverride, target, AmendMode("nope")); err == nil {
		t.Error("un modo desconocido debe fallar")
	}
}

func assertPreserved(t *testing.T, out string) {
	t.Helper()
	if !strings.Contains(out, "BEGIN:VALARM") || !strings.Contains(out, "TRIGGER:-PT15M") {
		t.Errorf("se perdió el VALARM en:\n%s", out)
	}
	if !strings.Contains(out, "ana@sytue.eu") {
		t.Errorf("se perdió el ATTENDEE en:\n%s", out)
	}
	if !strings.Contains(out, "RECURRENCE-ID:20260715T090000Z") {
		t.Errorf("se perdió el override RECURRENCE-ID en:\n%s", out)
	}
}
