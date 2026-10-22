// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

func TestUnirICS(t *testing.T) {
	zona := "BEGIN:VTIMEZONE\r\nTZID:Europe/Madrid\r\nBEGIN:STANDARD\r\nTZOFFSETFROM:+0200\r\nEND:STANDARD\r\nEND:VTIMEZONE\r\n"
	a := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Otro//EN\r\nMETHOD:PUBLISH\r\n" + zona +
		"BEGIN:VEVENT\r\nUID:a\r\nSUMMARY:Reunión con una descripción muy larga que viene\r\n  plegada\r\nBEGIN:VALARM\r\nACTION:DISPLAY\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	b := "BEGIN:VCALENDAR\nVERSION:2.0\n" + strings.ReplaceAll(zona, "\r\n", "\n") + "BEGIN:VEVENT\nUID:b\nEND:VEVENT\nEND:VCALENDAR"

	got := UnirICS("Trabajo, casa", []string{a, b})

	for _, debe := range []string{
		"X-WR-CALNAME:Trabajo\\, casa\r\n",
		"UID:a\r\n", "UID:b\r\n", "  plegada\r\n", "BEGIN:VALARM\r\n",
	} {
		if !strings.Contains(got, debe) {
			t.Errorf("falta %q en:\n%s", debe, got)
		}
	}
	if strings.Count(got, "BEGIN:VCALENDAR") != 1 || strings.Count(got, "END:VCALENDAR") != 1 {
		t.Errorf("debe quedar un solo VCALENDAR:\n%s", got)
	}
	if strings.Count(got, "BEGIN:VTIMEZONE") != 1 || strings.Contains(got, "METHOD:") || strings.Contains(got, "Otro") {
		t.Errorf("zona repetida o propiedades del envoltorio colándose:\n%s", got)
	}
}
