// SPDX-License-Identifier: Apache-2.0
package ical

import (
	"errors"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
)

func TestRecurringEventRoundTripUsesRecurValueAndPreservesClauses(t *testing.T) {
	start := time.Date(2027, 3, 29, 7, 0, 0, 0, time.UTC)
	input := event.Event{UID: "recur-roundtrip", DTStart: start, DTEnd: start.Add(time.Hour), TZID: "Europe/Madrid", RRule: "RRULE:FREQ=WEEKLY;COUNT=10;BYDAY=MO,WE;INTERVAL=2"}
	raw, err := EncodeEvent(input, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "RRULE;VALUE=TEXT") || strings.Contains(raw, `WEEKLY\;`) {
		t.Fatal("recurrence serialized as escaped text")
	}
	parsed, err := DecodeCalendarObject(raw, "")
	if err != nil || parsed.Master.RRule != input.RRule || !parsed.Master.DTStart.Equal(start) {
		t.Fatal("recurrence clauses failed to roundtrip", err)
	}
}

func TestHistoricalEscapedTextRecurrenceReadsWithoutRewritingTheSource(t *testing.T) {
	raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:legacy-recur\r\nDTSTART:20270329T070000Z\r\nDTEND:20270329T080000Z\r\nRRULE;VALUE=TEXT:FREQ=WEEKLY\\;COUNT=10\\;BYDAY=MO\\,WE\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	parsed, err := DecodeCalendarObject(raw, "")
	if err != nil || parsed.Master.RRule != "RRULE:FREQ=WEEKLY;COUNT=10;BYDAY=MO,WE" || parsed.Master.ICalData != raw {
		t.Fatal("historical recurrence cannot be retained exactly", err)
	}
}

func TestRecurrenceEncoderRejectsMalformedOrInjectedRules(t *testing.T) {
	start := time.Date(2027, 3, 29, 7, 0, 0, 0, time.UTC)
	for _, rule := range []string{"FREQ=BAD;COUNT=10", "FREQ=WEEKLY\r\nATTENDEE:injected@example.invalid"} {
		if _, err := EncodeEvent(event.Event{UID: "invalid-recur", DTStart: start, DTEnd: start.Add(time.Hour), RRule: rule}, ""); !errors.Is(err, event.ErrInvalidRRULE) {
			t.Fatal("invalid recurrence accepted", err)
		}
	}
}
