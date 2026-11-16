// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import (
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

// vevent arma un VCALENDAR con un único VEVENT con las líneas dadas.
func vevent(t *testing.T, lineas ...string) *ical.Calendar {
	t.Helper()
	raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Sytue//Test//ES\r\n" +
		"BEGIN:VEVENT\r\nUID:rango@sytue.eu\r\nDTSTAMP:20260801T000000Z\r\n" +
		strings.Join(lineas, "\r\n") + "\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, err := ical.NewDecoder(strings.NewReader(raw)).Decode()
	if err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	return cal
}

// La web y Thunderbird piden el calendario por time-range: una serie que
// empezó antes de la vista tiene que volver (RFC 4791 §9.9), y una que acabó
// antes, no.
func TestElRangoIncluyeLasSeriesQueEmpezaronAntes(t *testing.T) {
	// Vista: noviembre de 2026.
	desde := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	hasta := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)

	casos := []struct {
		nombre string
		lineas []string
		quiere bool
	}{
		{"semanal infinita desde septiembre", []string{
			"DTSTART:20260901T090000Z", "DTEND:20260901T093000Z",
			"RRULE:FREQ=WEEKLY;BYDAY=TU",
		}, true},
		{"semanal con zona y COUNT que llega a noviembre", []string{
			"DTSTART;TZID=Europe/Madrid:20260901T090000",
			"DTEND;TZID=Europe/Madrid:20260901T093000",
			"RRULE:FREQ=WEEKLY;COUNT=20",
		}, true},
		{"UNTIL anterior a la vista", []string{
			"DTSTART:20260901T090000Z", "DTEND:20260901T093000Z",
			"RRULE:FREQ=WEEKLY;UNTIL=20261015T090000Z",
		}, false},
		{"COUNT agotado antes de la vista", []string{
			"DTSTART:20260901T090000Z", "DTEND:20260901T093000Z",
			"RRULE:FREQ=WEEKLY;COUNT=3",
		}, false},
		{"serie que empieza después de la vista", []string{
			"DTSTART:20270105T090000Z", "DTEND:20270105T093000Z",
			"RRULE:FREQ=WEEKLY",
		}, false},
		{"RDATE dentro de la vista sin RRULE", []string{
			"DTSTART:20260901T090000Z", "DTEND:20260901T093000Z",
			"RDATE:20261012T090000Z,20261110T090000Z",
		}, true},
		{"RDATE fuera de la vista", []string{
			"DTSTART:20260901T090000Z", "DTEND:20260901T093000Z",
			"RDATE:20261012T090000Z",
		}, false},
		{"suelto dentro", []string{
			"DTSTART:20261110T090000Z", "DTEND:20261110T100000Z",
		}, true},
		{"suelto antes", []string{
			"DTSTART:20261010T090000Z", "DTEND:20261010T100000Z",
		}, false},
		{"suelto que cruza el inicio de la vista", []string{
			"DTSTART:20261031T230000Z", "DTEND:20261101T010000Z",
		}, true},
		{"suelto con DURATION que entra en la vista", []string{
			"DTSTART:20261031T230000Z", "DURATION:PT2H",
		}, true},
		// Sin DTEND ni DURATION: un día si es de día completo y, si no, un
		// instante que cuenta aunque caiga justo en el inicio (RFC 4791 §9.9).
		{"día completo sin DTEND el primer día de la vista", []string{
			"DTSTART;VALUE=DATE:20261101",
		}, true},
		{"día completo sin DTEND la víspera", []string{
			"DTSTART;VALUE=DATE:20261031",
		}, false},
		{"instante justo en el inicio de la vista", []string{
			"DTSTART:20261101T000000Z",
		}, true},
		{"serie de instantes cuya última cae en el inicio de la vista", []string{
			"DTSTART:20261025T000000Z",
			"RRULE:FREQ=WEEKLY;UNTIL=20261101T000000Z",
		}, true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := objectOverlaps(vevent(t, c.lineas...), desde, hasta); got != c.quiere {
				t.Fatalf("objectOverlaps = %v, quiere %v", got, c.quiere)
			}
		})
	}
}
