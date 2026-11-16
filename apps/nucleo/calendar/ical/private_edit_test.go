// SPDX-License-Identifier: Apache-2.0
package ical

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
)

func TestPrivateEditKeepsDateTypeRecurrenceDatesAndExtensions(t *testing.T) {
	start := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	for _, allDay := range []bool{false, true} {
		ev := event.Event{UID: "private-edit", Summary: "edited", DTStart: start, DTEnd: start.Add(24 * time.Hour), TZID: "Europe/Madrid", AllDay: allDay, RDate: []time.Time{start.AddDate(0, 0, 2)}, ExDate: []time.Time{start.AddDate(0, 0, 4)}, Status: event.StatusConfirmed, Classification: event.ClassPrivate, Transparency: event.TranspOpaque, Attendees: []event.Attendee{{Email: "person@example.test", CUType: "RESOURCE", PartStat: event.PartAccepted}}}
		data, err := EncodeEvent(ev, "")
		if err != nil {
			t.Fatal(err)
		}
		original := strings.Replace(data, "END:VEVENT", "X-PRIVATE-FUTURE:9007199254740993\r\nBEGIN:VALARM\r\nACTION:EMAIL\r\nTRIGGER:-PT5M\r\nDESCRIPTION:retain\r\nEND:VALARM\r\nEND:VEVENT", 1)
		original = strings.Replace(original, "END:VCALENDAR", "BEGIN:VTIMEZONE\r\nTZID:Custom/Retained\r\nBEGIN:STANDARD\r\nDTSTART:19700101T000000\r\nTZOFFSETFROM:+0000\r\nTZOFFSETTO:+0000\r\nEND:STANDARD\r\nEND:VTIMEZONE\r\nEND:VCALENDAR", 1)
		carried, err := CarryOver(data, original, false)
		if err != nil {
			t.Fatal(err)
		}
		carried, err = PreserveExtensions(carried, original)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := DecodeCalendarObject(carried, "")
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Master.AllDay != allDay || len(parsed.Master.RDate) != 1 || len(parsed.Master.ExDate) != 1 || !parsed.Master.RDate[0].Equal(ev.RDate[0]) || !parsed.Master.ExDate[0].Equal(ev.ExDate[0]) || len(parsed.Master.Attendees) != 1 || parsed.Master.Attendees[0].CUType != "RESOURCE" {
			t.Fatal("date or attendee semantics changed", parsed.Master)
		}
		for _, retained := range []string{"X-PRIVATE-FUTURE:9007199254740993", "ACTION:EMAIL", "TZID:Custom/Retained"} {
			if !strings.Contains(carried, retained) {
				t.Fatal("extension lost", retained)
			}
		}
	}
}

func TestPrivateEditPreservesUnmodeledParametersOnlyForSameAddress(t *testing.T) {
	start := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	ev := event.Event{UID: "private-parameters", Organizer: "organizer@example.test", DTStart: start, DTEnd: start.Add(time.Hour), TZID: "Europe/Madrid", Attendees: []event.Attendee{{Email: "person@example.test", CommonName: "old name", Role: event.RoleRequired, PartStat: event.PartNeedsAction}}}
	data, err := EncodeEvent(ev, "")
	if err != nil {
		t.Fatal(err)
	}
	old, err := ical.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		t.Fatal(err)
	}
	master := masterComponent(old)
	master.Props.Get(ical.PropAttendee).Params.Set("X-PRIVATE-PARAM", "retained")
	master.Props.Get(ical.PropAttendee).Params.Set("MEMBER", "mailto:group@example.test")
	master.Props.Get(ical.PropOrganizer).Params.Set("CN", "Retained organizer")
	master.Props.Get(ical.PropOrganizer).Params.Set("SENT-BY", "mailto:delegate@example.test")
	var original bytes.Buffer
	if err := ical.NewEncoder(&original).Encode(old); err != nil {
		t.Fatal(err)
	}
	ev.TZID = "UTC"
	ev.Attendees[0].CommonName = ""
	ev.Attendees[0].PartStat = event.PartAccepted
	for _, changed := range []bool{false, true} {
		if changed {
			ev.Organizer = "other-organizer@example.test"
			ev.Attendees[0].Email = "other-person@example.test"
		}
		next, err := EncodeEvent(ev, "")
		if err != nil {
			t.Fatal(err)
		}
		next, err = PreserveExtensions(next, original.String())
		if err != nil {
			t.Fatal(err)
		}
		cal, err := ical.NewDecoder(strings.NewReader(next)).Decode()
		if err != nil {
			t.Fatal(err)
		}
		props := masterComponent(cal).Props
		attendee, organizer := props.Get(ical.PropAttendee), props.Get(ical.PropOrganizer)
		if attendee.Params.Get("CN") != "" || attendee.Params.Get("PARTSTAT") != "ACCEPTED" || props.Get(ical.PropDateTimeStart).Params.Get("TZID") != "" {
			t.Fatal("old parameters overrode explicit typed edit")
		}
		if changed {
			if attendee.Params.Get("MEMBER") != "" || organizer.Params.Get("SENT-BY") != "" || organizer.Params.Get("CN") != "" {
				t.Fatal("address-bound parameters reassigned to another person")
			}
		} else if attendee.Params.Get("X-PRIVATE-PARAM") != "retained" || attendee.Params.Get("MEMBER") != "mailto:group@example.test" || organizer.Params.Get("CN") != "Retained organizer" || organizer.Params.Get("SENT-BY") != "mailto:delegate@example.test" {
			t.Fatal("unmodeled scheduling parameters lost")
		}
	}
}
