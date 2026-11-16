// SPDX-License-Identifier: Apache-2.0
package calendar

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPrivateEventRevisionKeepsOriginalUnknownFieldsAndAlarmHistory(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}
	r := &Repository{scope: scope}
	now := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	start := now.Add(48 * time.Hour)
	minutes := []int{15}
	in := api.EventInput{UID: "private", Summary: "Original", Start: start, End: start.Add(time.Hour), TZID: "Europe/Madrid", RRule: "FREQ=WEEKLY;COUNT=5", Reminders: &minutes, Attendees: []api.Attendee{{Email: "bob@example.invalid"}}}
	data, err := editedData(in, in.UID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	data = strings.Replace(data, "END:VEVENT", "X-PRIVATE-FUTURE:9007199254740993\r\nEND:VEVENT", 1)
	calendar, object := uuid.NewString(), uuid.NewString()
	cal, err := newRow(scope, "calendar", calendar, map[string]any{"owner_sub": scope.Subject, "name": "private", "ctag": uuid.NewString(), "sync_token": 1})
	if err != nil {
		t.Fatal(err)
	}
	obj, err := newRow(scope, "object", object, map[string]any{"calendar_id": calendar, "uid": in.UID, "resource_name": "private.ics", "component": "VEVENT", "data": data, "size_bytes": len(data), "etag": uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	a, err := r.reproject(model.Aggregate{Version: 1, Scope: scope, Kind: "object_state", ID: object, Rows: []model.Row{obj}}, data, now)
	if err != nil {
		t.Fatal(err)
	}
	eventID, reminderID := "", ""
	for i, row := range a.Rows {
		if row.Kind == "event" || row.Kind == "attendee" {
			a.Rows[i], err = patch(row, map[string]any{"future_integer": json.RawMessage("9007199254740993")})
			if err != nil {
				t.Fatal(err)
			}
			if row.Kind == "event" {
				eventID = row.ID
			}
		}
		if row.Kind == "reminder" {
			reminderID = row.ID
			a.Rows[i], err = patch(row, map[string]any{"fired_at": now, "dismissed_at": now})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if eventID == "" || reminderID == "" {
		t.Fatal("projections omitted")
	}
	baseline, err := model.NewSnapshot(scope, append([]model.Row{cal}, a.Rows...))
	if err != nil {
		t.Fatal(err)
	}
	originalReminder, _ := baseline.Row("reminder", reminderID)
	originalObject, _ := baseline.Row("object", object)
	in.Summary = "Edited"
	in.Reminders = nil
	nextData, err := editedData(in, in.UID, data, 1)
	if err != nil {
		t.Fatal(err)
	}
	change, err := baseline.Edit("object_state", object)
	if err != nil {
		t.Fatal(err)
	}
	change, err = r.reproject(change, nextData, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	next, err := baseline.WithAggregates([]model.Aggregate{change})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := findEvent(next, eventID)
	if err != nil || updated.Summary != "Edited" || len(updated.Attendees) != 1 {
		t.Fatal("stable projection changed", err)
	}
	for _, row := range next.Rows() {
		if (row.Kind == "event" || row.Kind == "attendee") && !rowDeleted(row) && !bytes.Contains(row.JSON, []byte("9007199254740993")) {
			t.Fatal("unknown integer lost", row.Kind)
		}
	}
	retained, _ := next.Row("reminder", reminderID)
	if !bytes.Equal(retained.JSON, originalReminder.JSON) {
		t.Fatal("fired/dismissed alarm history changed")
	}
	unchanged, _ := baseline.Row("object", object)
	if !bytes.Equal(unchanged.JSON, originalObject.JSON) {
		t.Fatal("immutable original ICS mutated")
	}
	current, _ := next.Row("object", object)
	currentData, err := objectData(current)
	if err != nil || !strings.Contains(currentData, "X-PRIVATE-FUTURE:9007199254740993") || !strings.Contains(currentData, "VALARM") {
		t.Fatal("edited resource lost extension/alarm", err)
	}
	if _, err := r.CreateEvent(t.Context(), calendar, api.EventInput{Send: true}); !errors.Is(err, ErrPrivateIngress) {
		t.Fatal("send bypassed ingress capability", err)
	}
}
