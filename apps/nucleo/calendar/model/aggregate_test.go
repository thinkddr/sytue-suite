// SPDX-License-Identifier: Apache-2.0
package model

import (
	"bytes"
	"slices"
	"testing"
	"time"
)

func TestAggregateRetainsAuxiliaryGraphAndAtomicCalendarTombstone(t *testing.T) {
	rows := fixtureRows(t)
	rows[1] = rewrite(t, rows[1], "deleted_at", nil)
	s, err := NewSnapshot(fixtureScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Edit("object_state", fixtureID(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Rows) != 4 {
		t.Fatal("object revision omitted event, attendee or reminder", len(a.Rows))
	}
	for i, row := range a.Rows {
		if row.Kind == "event" {
			a.Rows[i] = rewrite(t, row, "summary", "edited privately")
		}
	}
	next, err := s.WithAggregates([]Aggregate{a})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Rows()) != len(rows) {
		t.Fatal("auxiliary history lost")
	}
	for _, row := range rows {
		original, _ := s.Row(row.Kind, row.ID)
		if !bytes.Equal(original.JSON, row.JSON) {
			t.Fatal("baseline mutated")
		}
		if row.Kind == "event" {
			continue
		}
		actual, _ := next.Row(row.Kind, row.ID)
		if !bytes.Equal(actual.JSON, row.JSON) {
			t.Fatal("unrelated row changed", row.Kind)
		}
	}
	deleted, err := next.Edit("calendar_state", fixtureID(1))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	deleted.Rows[0] = rewrite(t, deleted.Rows[0], "deleted_at", now)
	if err := deleted.Validate(fixtureScope()); err == nil {
		t.Fatal("row/index deletion mismatch accepted")
	}
	deleted.DeletedAt = &now
	trashed, err := next.WithAggregates([]Aggregate{deleted})
	if err != nil {
		t.Fatal(err)
	}
	if len(trashed.Rows()) != len(rows) {
		t.Fatal("calendar deletion discarded retained history")
	}
	for _, kind := range []string{"calendar", "object", "event", "attendee", "reminder"} {
		id := map[string]int{"calendar": 1, "object": 2, "event": 3, "attendee": 4, "reminder": 6}[kind]
		row, ok := trashed.Row(kind, fixtureID(id))
		if !ok || !trashed.Deleted(row) {
			t.Fatal("partial calendar tombstone", kind)
		}
	}
	copied, _ := trashed.Row("event", fixtureID(3))
	clear(copied.JSON)
	again, _ := trashed.Row("event", fixtureID(3))
	if bytes.Equal(copied.JSON, again.JSON) {
		t.Fatal("row returns aliased content")
	}
}

func TestAggregateRejectsForeignRowsOmissionAndOverlappingOwnership(t *testing.T) {
	s, err := NewSnapshot(fixtureScope(), fixtureRows(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"scope", "attendee-target", "other-object", "drop-event", "drop-reminder", "duplicate-revision", "nonowner-calendar"} {
		t.Run(fault, func(t *testing.T) {
			a, err := s.Edit("object_state", fixtureID(2))
			if err != nil {
				t.Fatal(err)
			}
			switch fault {
			case "scope":
				a.Scope.Subject = "other"
			case "attendee-target":
				for i, row := range a.Rows {
					if row.Kind == "attendee" {
						a.Rows[i] = rewrite(t, row, "event_id", fixtureID(99))
					}
				}
			case "other-object":
				a.Rows = append(a.Rows, fixtureRow(t, "object", 99, map[string]any{}))
			case "drop-event", "drop-reminder":
				kind := "event"
				if fault == "drop-reminder" {
					kind = "reminder"
				}
				a.Rows = slices.DeleteFunc(a.Rows, func(row Row) bool { return row.Kind == kind })
			case "nonowner-calendar":
				_, err = s.Edit("calendar_state", fixtureID(11))
				if err == nil {
					t.Fatal("synthetic resource calendar accepted as personal revision")
				}
				return
			}
			changes := []Aggregate{a}
			if fault == "duplicate-revision" {
				changes = append(changes, a)
			}
			if _, err := s.WithAggregates(changes); err == nil {
				t.Fatal("invalid private aggregate accepted")
			}
		})
	}
}
