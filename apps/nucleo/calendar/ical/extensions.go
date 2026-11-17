// SPDX-License-Identifier: Apache-2.0
package ical

import (
	"bytes"
	"errors"
	"slices"
	"strings"

	"github.com/emersion/go-ical"
)

// PreserveExtensions carries properties outside the typed REST event fields,
// calendar properties and non-VEVENT components into an edited resource. The
// immutable source continues to retain the exact original bytes. CarryOver owns
// VALARM and recurrence override semantics and must run before this function.
func PreserveExtensions(edited, original string) (string, error) {
	if strings.TrimSpace(original) == "" {
		return edited, nil
	}
	next, err := ical.NewDecoder(strings.NewReader(edited)).Decode()
	if err != nil {
		return "", err
	}
	previous, err := ical.NewDecoder(strings.NewReader(original)).Decode()
	if err != nil {
		return "", err
	}
	master, old := masterComponent(next), masterComponent(previous)
	if master == nil || old == nil {
		return "", errors.New("ical: resource has no master event")
	}
	controlled := map[string]bool{}
	for _, name := range []string{"UID", "DTSTAMP", "DTSTART", "DTEND", "DURATION", "SUMMARY", "DESCRIPTION", "LOCATION", "ORGANIZER", "STATUS", "CLASS", "TRANSP", "COLOR", "X-APPLE-CALENDAR-COLOR", "CONFERENCE", "RRULE", "RDATE", "EXDATE", "RECURRENCE-ID", "SEQUENCE", "ATTENDEE"} {
		controlled[name] = true
	}
	for name, props := range old.Props {
		if !controlled[name] {
			master.Props[name] = props
		}
	}
	preservePropertyParameters(master.Props, old.Props)
	for name, props := range previous.Props {
		if _, set := next.Props[name]; !set {
			next.Props[name] = props
		}
	}
	for _, child := range previous.Children {
		if child.Name != ical.CompEvent {
			next.Children = append(next.Children, child)
		}
	}
	var out bytes.Buffer
	if err := ical.NewEncoder(&out).Encode(next); err != nil {
		return "", err
	}
	return out.String(), nil
}

func preservePropertyParameters(next, previous ical.Props) {
	for name, props := range next {
		for i := range props {
			for _, old := range previous[name] {
				// Address-bound metadata must never move to a different person.
				if (name == ical.PropAttendee || name == ical.PropOrganizer) && !strings.EqualFold(strings.TrimSpace(props[i].Value), strings.TrimSpace(old.Value)) {
					continue
				}
				if props[i].Params == nil {
					props[i].Params = make(ical.Params)
				}
				for parameter, values := range old.Params {
					if controlledParameter(name, strings.ToUpper(parameter)) {
						continue
					}
					if _, set := props[i].Params[parameter]; !set {
						props[i].Params[parameter] = slices.Clone(values)
					}
				}
				break
			}
		}
		next[name] = props
	}
}

func controlledParameter(property, parameter string) bool {
	if parameter == ical.ParamValue {
		return true
	}
	switch property {
	case ical.PropDateTimeStart, ical.PropDateTimeEnd, ical.PropRecurrenceID, ical.PropRecurrenceDates, ical.PropExceptionDates:
		return parameter == ical.ParamTimezoneID
	case ical.PropAttendee:
		return slices.Contains([]string{"CN", "ROLE", "PARTSTAT", "CUTYPE", "RSVP"}, parameter)
	case "CONFERENCE":
		return parameter == "FEATURE" || parameter == "LABEL"
	default:
		return false
	}
}
