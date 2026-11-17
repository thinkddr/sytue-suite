// SPDX-License-Identifier: Apache-2.0
package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func fixtureScope() storage.Scope {
	return storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}
}
func fixtureID(n int) string { return fmt.Sprintf("11111111-1111-4111-8111-%012d", n) }

func fixtureRow(t *testing.T, kind string, n int, fields map[string]any) Row {
	t.Helper()
	id := fixtureID(n)
	if kind == "request" {
		var err error
		id, err = RequestID(fields["calendar_id"].(string), fields["sujeto"].(string))
		if err != nil {
			t.Fatal(err)
		}
	} else {
		fields["id"] = id
	}
	fields["tenant"] = "acme"
	fields["future"] = json.RawMessage(`{"integer":9007199254740993,"text":"preserve unknown columns"}`)
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return Row{Kind: kind, ID: id, JSON: raw}
}

func fixtureRows(t *testing.T) []Row {
	t.Helper()
	cal, obj, ev := fixtureID(1), fixtureID(2), fixtureID(3)
	data := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:meeting\r\nDTSTART;TZID=Europe/Madrid:20261030T090000\r\nRRULE:FREQ=WEEKLY;COUNT=10\r\nX-FUTURE:preserve canonical content\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	return []Row{
		fixtureRow(t, "calendar", 1, map[string]any{"owner_sub": "alice", "name": "personal", "ctag": fixtureID(21), "sync_token": 1, "read_only": true}),
		fixtureRow(t, "object", 2, map[string]any{"calendar_id": cal, "uid": "meeting", "resource_name": "meeting.ics", "component": "VEVENT", "data": data, "size_bytes": len(data), "etag": fixtureID(22), "deleted_at": "2026-10-29T09:00:00Z"}),
		fixtureRow(t, "event", 3, map[string]any{"calendar_id": cal, "object_id": obj, "uid": "meeting", "dtstart": "2026-10-30T08:00:00Z", "dtend": "2026-10-30T09:00:00Z", "tzid": "Europe/Madrid", "all_day": false, "status": "CONFIRMED", "classification": "PRIVATE", "transp": "OPAQUE", "rrule": "FREQ=WEEKLY;COUNT=10", "conference_url": "https://rooms.example.invalid/private"}),
		fixtureRow(t, "attendee", 4, map[string]any{"event_id": ev, "email": "bob@example.invalid", "role": "REQ-PARTICIPANT", "partstat": "ACCEPTED", "cutype": "INDIVIDUAL", "rsvp": true}),
		fixtureRow(t, "invitation", 5, map[string]any{"event_id": ev, "uid": "meeting", "recipient": "bob@example.invalid", "recipient_sub": "bob", "method": "REQUEST", "status": "pending", "data": data}),
		fixtureRow(t, "reminder", 6, map[string]any{"owner_sub": "alice", "calendar_id": cal, "object_id": obj, "event_id": ev, "uid": "meeting", "summary": "Private alarm", "dtstart": "2026-10-30T08:00:00Z", "trigger_at": "2026-10-30T07:45:00Z", "action": "DISPLAY", "fired_at": "2026-10-30T07:45:00Z", "dismissed_at": "2026-10-30T07:46:00Z"}),
		fixtureRow(t, "subscription", 7, map[string]any{"calendar_id": cal, "url": "https://feed.example.invalid/private-token", "refresh_interval_seconds": 3600, "etag": "private-etag", "last_modified": "yesterday"}),
		fixtureRow(t, "share", 8, map[string]any{"calendar_id": cal, "grantee_sub": "bob", "grantee_email": "bob@example.invalid", "permission": "reader", "created_by": "alice"}),
		fixtureRow(t, "request", 0, map[string]any{"calendar_id": cal, "sujeto": "bob", "correo": "bob@example.invalid", "nombre": "Bob", "creada": "2026-10-30T08:00:00Z"}),
		fixtureRow(t, "resource", 10, map[string]any{"calendar_id": fixtureID(11), "kind": "room", "name": "Private managed resource", "capacity": 6, "created_by": "alice", "managers": []string{"bob"}, "active": true}),
		fixtureRow(t, "calendar", 11, map[string]any{"owner_sub": "resource:" + fixtureID(10), "name": "resource", "ctag": fixtureID(23), "sync_token": 1}),
		fixtureRow(t, "outbox", 12, map[string]any{"type": "calendar.reminder.due", "payload": map[string]any{"event_id": ev, "summary": "Private alarm"}}),
		fixtureRow(t, "invitation", 13, map[string]any{"event_id": nil, "uid": "external-meeting", "recipient": "alice@example.invalid", "recipient_sub": "alice", "method": "REQUEST", "status": "pending", "data": data}),
	}
}

func rewrite(t *testing.T, row Row, key string, value any) Row {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(row.JSON, &fields); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	fields[key] = raw
	row.JSON, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestSnapshotRetainsCompleteCalendarGraphAndExactUnknownJSON(t *testing.T) {
	rows := fixtureRows(t)
	snapshot, err := NewSnapshot(fixtureScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	actual := snapshot.Rows()
	if len(actual) != len(rows) {
		t.Fatal("lost source rows")
	}
	for _, want := range rows {
		found := false
		for _, row := range actual {
			if row.Kind == want.Kind && row.ID == want.ID {
				if !bytes.Equal(row.JSON, want.JSON) {
					t.Fatal("source JSON changed")
				}
				found = true
			}
		}
		if !found {
			t.Fatal("source row missing", want.Kind)
		}
	}
	clear(rows[0].JSON)
	clear(actual[0].JSON)
	for _, row := range snapshot.Rows() {
		if !json.Valid(row.JSON) {
			t.Fatal("snapshot returned an alias")
		}
	}
	if snapshot.Scope() != fixtureScope() {
		t.Fatal("scope changed")
	}
}

func TestSnapshotRejectsForeignIncompleteAndMalformedGraphs(t *testing.T) {
	cases := []struct {
		name   string
		change func([]Row) []Row
	}{
		{"foreign-tenant", func(r []Row) []Row { r[2] = rewrite(t, r[2], "tenant", "other"); return r }},
		{"foreign-owner-consistent-descendants", func(r []Row) []Row {
			r[0] = rewrite(t, r[0], "owner_sub", "bob")
			r[5] = rewrite(t, r[5], "owner_sub", "bob")
			return r
		}},
		{"foreign-owner", func(r []Row) []Row { r[0] = rewrite(t, r[0], "owner_sub", "bob"); return r }},
		{"missing-calendar", func(r []Row) []Row { return r[1:] }},
		{"missing-object", func(r []Row) []Row { return append(r[:1], r[2:]...) }},
		{"missing-event", func(r []Row) []Row { return append(r[:2], r[3:]...) }},
		{"event-uid-consistent-descendants", func(r []Row) []Row {
			for _, i := range []int{2, 4, 5} {
				r[i] = rewrite(t, r[i], "uid", "other")
			}
			return r
		}},
		{"wrong-uid", func(r []Row) []Row { r[2] = rewrite(t, r[2], "uid", "other"); return r }},
		{"wrong-reminder-owner", func(r []Row) []Row { r[5] = rewrite(t, r[5], "owner_sub", "bob"); return r }},
		{"wrong-reminder-object", func(r []Row) []Row { r[5] = rewrite(t, r[5], "object_id", fixtureID(30)); return r }},
		{"resource-access", func(r []Row) []Row { r[9] = rewrite(t, r[9], "created_by", "carol"); return r }},
		{"resource-owner", func(r []Row) []Row { r[10] = rewrite(t, r[10], "owner_sub", "resource:"+fixtureID(30)); return r }},
		{"foreign-invitation", func(r []Row) []Row { r[12] = rewrite(t, r[12], "recipient_sub", "bob"); return r }},
		{"foreign-outbox", func(r []Row) []Row {
			r[11] = rewrite(t, r[11], "payload", map[string]any{"owner_sub": "bob"})
			return r
		}},
		{"foreign-outbox-owner-with-owned-event", func(r []Row) []Row {
			r[11] = rewrite(t, r[11], "payload", map[string]any{"owner_sub": "bob", "event_id": fixtureID(3)})
			return r
		}},
		{"foreign-outbox-reference", func(r []Row) []Row {
			r[11] = rewrite(t, r[11], "payload", map[string]any{"owner_sub": "alice", "event_id": fixtureID(30)})
			return r
		}},
		{"canonical-uid", func(r []Row) []Row {
			var value struct {
				Data string `json:"data"`
			}
			if err := json.Unmarshal(r[1].JSON, &value); err != nil {
				t.Fatal(err)
			}
			value.Data = strings.Replace(value.Data, "UID:meeting", "UID:changed", 1)
			r[1] = rewrite(t, r[1], "data", value.Data)
			r[1] = rewrite(t, r[1], "size_bytes", len(value.Data))
			return r
		}},
		{"wrong-byte-count", func(r []Row) []Row { r[1] = rewrite(t, r[1], "size_bytes", 1); return r }},
		{"duplicate", func(r []Row) []Row { return append(r, r[2]) }},
		{"request-identity", func(r []Row) []Row { r[8].ID = fixtureID(1) + "/carol"; return r }},
		{"malformed-json", func(r []Row) []Row { r[1].JSON = []byte("{"); return r }},
		{"unknown-kind", func(r []Row) []Row { r[1].Kind = "unknown"; return r }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewSnapshot(fixtureScope(), tc.change(fixtureRows(t))); err == nil {
				t.Fatal("accepted invalid Calendar graph")
			}
		})
	}
}

func TestSnapshotEnforcesActiveSQLUniquenessWithoutDroppingTrash(t *testing.T) {
	for _, kind := range []string{"calendar", "object", "event", "attendee", "subscription", "share"} {
		t.Run(kind, func(t *testing.T) {
			rows := fixtureRows(t)
			for i, row := range rows {
				if row.Kind != kind {
					continue
				}
				rows[i] = rewrite(t, row, "deleted_at", nil)
				duplicate := rewrite(t, rows[i], "id", fixtureID(99))
				duplicate.ID = fixtureID(99)
				if _, err := NewSnapshot(fixtureScope(), append(rows, duplicate)); err == nil {
					t.Fatal("duplicate SQL key accepted", kind)
				}
				return
			}
			t.Fatal("fixture kind absent", kind)
		})
	}
	rows := fixtureRows(t)
	duplicate := rewrite(t, rows[0], "id", fixtureID(99))
	duplicate.ID = fixtureID(99)
	duplicate = rewrite(t, duplicate, "deleted_at", "2026-10-29T09:00:00Z")
	if _, err := NewSnapshot(fixtureScope(), append(rows, duplicate)); err != nil {
		t.Fatal("trash with reused live name rejected", err)
	}
}

func TestRequestKeyPreservesUnicodeSlashAndOriginalJSON(t *testing.T) {
	subjects := []string{"人/é:one", "人/e\u0301:one", "人/é:one/", "人/é:two"}
	seen := map[string]bool{}
	for _, subject := range subjects {
		rows := fixtureRows(t)
		request := fixtureRow(t, "request", 0, map[string]any{"calendar_id": fixtureID(1), "sujeto": subject, "nombre": "Exact Unicode / key"})
		if !storage.Identifier(request.ID) || len(request.ID) != 64 || seen[request.ID] {
			t.Fatal("unsafe or colliding composite key")
		}
		seen[request.ID] = true
		rows[8] = request
		snapshot, err := NewSnapshot(fixtureScope(), rows)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range snapshot.Rows() {
			if row.Kind == "request" {
				found = true
				if row.ID != request.ID || !bytes.Equal(row.JSON, request.JSON) {
					t.Fatal("Unicode request changed during retention")
				}
			}
		}
		if !found {
			t.Fatal("Unicode request lost")
		}
	}
}
