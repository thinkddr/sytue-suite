// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	bookID      = "10000000-0000-4000-8000-000000000001"
	cardID      = "10000000-0000-4000-8000-000000000002"
	contactID   = "10000000-0000-4000-8000-000000000003"
	groupCardID = "10000000-0000-4000-8000-000000000004"
	groupID     = "10000000-0000-4000-8000-000000000005"
	emailID     = "10000000-0000-4000-8000-000000000006"
	phoneID     = "10000000-0000-4000-8000-000000000007"
	memberID    = "10000000-0000-4000-8000-000000000008"
	pendingID   = "10000000-0000-4000-8000-000000000009"
)

func rowKey(kind, id string) string { return kind + "\x00" + id }

func testScope() storage.Scope {
	return storage.Scope{Tenant: "acme", Subject: "alice", Product: "contacts"}
}

func fixtureRow(kind, id string, fields map[string]any) Row {
	fields["id"], fields["tenant"] = id, "acme"
	raw, err := json.Marshal(fields)
	if err != nil {
		panic(err)
	}
	return Row{Kind: kind, ID: id, JSON: raw}
}

func completeRows() []Row {
	photo := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:person\r\nFN:Ada\r\nPHOTO:data:image/png;base64,aW1hZ2U=\r\nX-ARCHIVE:keep\r\nEND:VCARD\r\n"
	group := "BEGIN:VCARD\r\nVERSION:4.0\r\nKIND:group\r\nUID:group\r\nFN:Friends\r\nMEMBER:urn:uuid:person\r\nMEMBER:urn:uuid:remote\r\nEND:VCARD\r\n"
	return []Row{
		fixtureRow("book", bookID, map[string]any{"owner_sub": "alice", "name": "personal", "deleted_at": "2020-01-01T00:00:00Z", "sync_token": json.RawMessage("900719925474099512345"), "future": map[string]any{"keep": "yes"}}),
		fixtureRow("card", cardID, map[string]any{"address_book_id": bookID, "uid": "person", "etag": cardID, "resource_name": "original.vcf", "data": photo, "size_bytes": len(photo), "deleted_at": "2020-01-01T00:00:00Z"}),
		fixtureRow("contact", contactID, map[string]any{"address_book_id": bookID, "object_id": cardID, "uid": "person", "formatted_name": "Ada", "is_favorite": true, "deleted_at": "2020-01-01T00:00:00Z", "birthday": "1815-12-10"}),
		fixtureRow("card", groupCardID, map[string]any{"address_book_id": bookID, "uid": "group", "etag": groupCardID, "resource_name": "friends.vcf", "data": group, "size_bytes": len(group)}),
		fixtureRow("group", groupID, map[string]any{"address_book_id": bookID, "object_id": groupCardID, "uid": "group", "name": "Friends"}),
		fixtureRow("email", emailID, map[string]any{"contact_id": contactID, "email": "ada@example.invalid", "position": 7, "is_primary": true}),
		fixtureRow("phone", phoneID, map[string]any{"contact_id": contactID, "phone": "+34000000000", "position": 5}),
		fixtureRow("member", memberID, map[string]any{"group_id": groupID, "contact_id": contactID, "contact_uid": "person"}),
		fixtureRow("member", pendingID, map[string]any{"group_id": groupID, "contact_id": nil, "contact_uid": "remote"}),
	}
}

func replaceField(t *testing.T, rows []Row, index int, field string, value any) {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rows[index].JSON, &raw); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	raw[field] = data
	rows[index].JSON, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCompleteSnapshotPreservesOriginalBytesAndOwnsItsBuffers(t *testing.T) {
	rows := completeRows()
	original := make(map[string][]byte, len(rows))
	for _, row := range rows {
		original[rowKey(row.Kind, row.ID)] = bytes.Clone(row.JSON)
	}
	snapshot, err := NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Scope() != testScope() {
		t.Fatal("scope changed")
	}
	for _, row := range rows {
		clear(row.JSON)
	}
	output := snapshot.Rows()
	if len(output) != len(original) {
		t.Fatal("lost rows")
	}
	for i, row := range output {
		if !bytes.Equal(row.JSON, original[rowKey(row.Kind, row.ID)]) {
			t.Fatalf("lossy %s", row.Kind)
		}
		if i > 0 && rowKey(output[i-1].Kind, output[i-1].ID) >= rowKey(row.Kind, row.ID) {
			t.Fatal("unstable inventory")
		}
		clear(row.JSON)
	}
	for _, row := range snapshot.Rows() {
		if !bytes.Equal(row.JSON, original[rowKey(row.Kind, row.ID)]) {
			t.Fatal("caller changed snapshot")
		}
	}
	if !bytes.Contains(original[rowKey("book", bookID)], []byte("900719925474099512345")) {
		t.Fatal("fixture lost integer")
	}
}

func TestSnapshotRejectsForeignMissingAndConflictingRelationships(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*testing.T, []Row) []Row
	}{
		{"foreign-tenant", func(t *testing.T, r []Row) []Row { replaceField(t, r, 0, "tenant", "other"); return r }},
		{"foreign-owner", func(t *testing.T, r []Row) []Row { replaceField(t, r, 0, "owner_sub", "bob"); return r }},
		{"wrong-row-id", func(t *testing.T, r []Row) []Row { replaceField(t, r, 0, "id", cardID); return r }},
		{"missing-book", func(_ *testing.T, r []Row) []Row { return r[1:] }},
		{"missing-card", func(_ *testing.T, r []Row) []Row { return append(r[:1], r[2:]...) }},
		{"missing-contact", func(_ *testing.T, r []Row) []Row { return append(r[:2], r[3:]...) }},
		{"missing-group", func(_ *testing.T, r []Row) []Row { return append(r[:4], r[5:]...) }},
		{"mismatched-object-book", func(t *testing.T, r []Row) []Row { replaceField(t, r, 2, "address_book_id", emailID); return r }},
		{"mismatched-object-uid", func(t *testing.T, r []Row) []Row { replaceField(t, r, 2, "uid", "other"); return r }},
		{"mismatched-member-uid", func(t *testing.T, r []Row) []Row { replaceField(t, r, 7, "contact_uid", "other"); return r }},
		{"missing-resolved-member", func(t *testing.T, r []Row) []Row { replaceField(t, r, 7, "contact_id", emailID); return r }},
		{"missing-channel-contact", func(t *testing.T, r []Row) []Row { replaceField(t, r, 5, "contact_id", emailID); return r }},
		{"duplicate", func(_ *testing.T, r []Row) []Row { return append(r, r[0]) }},
		{"unknown-kind", func(_ *testing.T, r []Row) []Row { r[0].Kind = "unknown"; return r }},
		{"wrong-size", func(t *testing.T, r []Row) []Row { replaceField(t, r, 1, "size_bytes", 0); return r }},
		{"invalid-etag", func(t *testing.T, r []Row) []Row { replaceField(t, r, 1, "etag", "not-uuid"); return r }},
		{"malformed", func(_ *testing.T, r []Row) []Row { r[0].JSON = []byte("{"); return r }},
		{"null", func(_ *testing.T, r []Row) []Row { r[0].JSON = []byte("null"); return r }},
		{"oversized", func(_ *testing.T, r []Row) []Row { r[0].JSON = []byte(strings.Repeat(" ", MaxRowBytes+1)); return r }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewSnapshot(testScope(), test.change(t, completeRows())); !errors.Is(err, ErrSnapshot) {
				t.Fatalf("accepted bad graph: %v", err)
			}
		})
	}
	scope := testScope()
	scope.Product = "notes"
	if _, err := NewSnapshot(scope, completeRows()); !errors.Is(err, ErrSnapshot) {
		t.Fatal("accepted foreign product")
	}
}
