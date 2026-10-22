// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// TestColorRoundTrip: un color hex de la paleta sobrevive el ciclo
// EncodeEvent -> DecodeCalendarObject sin perdida (round-trip CalDAV). Se emite
// como X-APPLE-CALENDAR-COLOR y se vuelve a leer en la columna consultable.
func TestColorRoundTrip(t *testing.T) {
	start := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	ev := domain.Event{
		UID:     "color-1@calendar.sytue.eu",
		Summary: "Revision de diseño",
		DTStart: start,
		DTEnd:   start.Add(time.Hour),
		Color:   "#6B3E9E", // Ciruela (paleta de la suite)
	}
	data, err := EncodeEvent(ev, "")
	if err != nil {
		t.Fatalf("EncodeEvent: %v", err)
	}
	// go-ical puede anadir «;VALUE=TEXT» a la propiedad de extension: basta con que
	// la linea X-APPLE-CALENDAR-COLOR lleve el hex.
	if !strings.Contains(data, "X-APPLE-CALENDAR-COLOR") || !strings.Contains(data, "#6B3E9E") {
		t.Fatalf("el ICS no emitio el color:\n%s", data)
	}

	po, err := DecodeCalendarObject(data, "cal-1")
	if err != nil {
		t.Fatalf("DecodeCalendarObject: %v", err)
	}
	if po.Master.Color != "#6B3E9E" {
		t.Errorf("color tras round-trip = %q, want %q", po.Master.Color, "#6B3E9E")
	}
}

// TestColorRFC7986Name: un COLOR con nombre CSS3 (RFC 7986) de un cliente externo
// se lee y se re-emite sin perdida.
func TestColorRFC7986Name(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
		"UID:name-1@ex\r\nDTSTART:20260701T090000Z\r\nDTEND:20260701T100000Z\r\n" +
		"COLOR:cornflowerblue\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	po, err := DecodeCalendarObject(ics, "cal-1")
	if err != nil {
		t.Fatalf("DecodeCalendarObject: %v", err)
	}
	if po.Master.Color != "cornflowerblue" {
		t.Fatalf("color CSS3 = %q, want cornflowerblue", po.Master.Color)
	}
	// Re-encode debe emitir COLOR (nombre), no X-APPLE-CALENDAR-COLOR (hex).
	ev := domain.Event{UID: po.Master.UID, DTStart: po.Master.DTStart, DTEnd: po.Master.DTEnd, Color: po.Master.Color}
	data, err := EncodeEvent(ev, "")
	if err != nil {
		t.Fatalf("EncodeEvent: %v", err)
	}
	if !strings.Contains(data, "cornflowerblue") || strings.Contains(data, "APPLE") {
		t.Errorf("re-encode de nombre CSS3 incorrecto (deberia ser COLOR, no X-APPLE):\n%s", data)
	}
}

// TestColorSanitizeRejectsInjection: un valor con caracteres peligrosos (posible
// inyeccion de lineas ICS) se descarta al decodificar.
func TestColorSanitizeRejectsInjection(t *testing.T) {
	if got := domain.SanitizeColor("#fff\r\nSUMMARY:evil"); got != "" {
		t.Errorf("SanitizeColor no descarto la inyeccion: %q", got)
	}
	if got := domain.SanitizeColor("red;X-EVIL=1"); got != "" {
		t.Errorf("SanitizeColor no descarto el parametro: %q", got)
	}
	if got := domain.SanitizeColor("#6b3e9e"); got != "#6B3E9E" {
		t.Errorf("SanitizeColor(hex) = %q, want #6B3E9E", got)
	}
}
