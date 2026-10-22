// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package alarm

import (
	"testing"
	"time"
)

func vcal(valarms ...string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:e1@sytue\r\n" +
		"DTSTART:20260710T100000Z\r\nDTEND:20260710T110000Z\r\nSUMMARY:Reunión\r\n" +
		joinAlarms(valarms) + "END:VEVENT\r\nEND:VCALENDAR\r\n"
}

func joinAlarms(vs []string) string {
	out := ""
	for _, v := range vs {
		out += v + "\r\n"
	}
	return out
}

var (
	start = time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	end   = time.Date(2026, 7, 10, 11, 0, 0, 0, time.UTC)
)

func TestRelativeToStart(t *testing.T) {
	got, err := Triggers(vcal("BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nEND:VALARM"), start, end)
	if err != nil {
		t.Fatalf("Triggers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("esperaba 1 disparo, hay %d", len(got))
	}
	want := start.Add(-15 * time.Minute)
	if !got[0].At.Equal(want) {
		t.Errorf("At = %s, quería %s", got[0].At, want)
	}
	if got[0].Action != "DISPLAY" {
		t.Errorf("Action = %q, quería DISPLAY", got[0].Action)
	}
}

func TestRelatedEnd(t *testing.T) {
	got, err := Triggers(vcal("BEGIN:VALARM\r\nACTION:EMAIL\r\nTRIGGER;RELATED=END:PT5M\r\nEND:VALARM"), start, end)
	if err != nil {
		t.Fatalf("Triggers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("esperaba 1 disparo, hay %d", len(got))
	}
	want := end.Add(5 * time.Minute) // 5 min DESPUÉS del fin
	if !got[0].At.Equal(want) {
		t.Errorf("At = %s, quería %s (relativo a DTEND)", got[0].At, want)
	}
}

func TestAbsoluteTrigger(t *testing.T) {
	got, err := Triggers(vcal("BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER;VALUE=DATE-TIME:20260710T093000Z\r\nEND:VALARM"), start, end)
	if err != nil {
		t.Fatalf("Triggers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("esperaba 1 disparo, hay %d", len(got))
	}
	want := time.Date(2026, 7, 10, 9, 30, 0, 0, time.UTC)
	if !got[0].At.Equal(want) {
		t.Errorf("At = %s, quería %s (absoluto)", got[0].At, want)
	}
}

func TestMultipleAlarms(t *testing.T) {
	got, err := Triggers(vcal(
		"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT10M\r\nEND:VALARM",
		"BEGIN:VALARM\r\nACTION:EMAIL\r\nTRIGGER:-PT1H\r\nEND:VALARM",
	), start, end)
	if err != nil {
		t.Fatalf("Triggers: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("esperaba 2 disparos, hay %d", len(got))
	}
}

func TestNoAlarms(t *testing.T) {
	got, err := Triggers(vcal(), start, end)
	if err != nil {
		t.Fatalf("Triggers: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("esperaba 0 disparos, hay %d", len(got))
	}
}

func TestUnreadableTriggerIgnored(t *testing.T) {
	// Un VALARM sin TRIGGER no debe romper el parseo del resto.
	got, err := Triggers(vcal(
		"BEGIN:VALARM\r\nACTION:AUDIO\r\nEND:VALARM",
		"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT30M\r\nEND:VALARM",
	), start, end)
	if err != nil {
		t.Fatalf("Triggers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("esperaba 1 disparo legible, hay %d", len(got))
	}
}
