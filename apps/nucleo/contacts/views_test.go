// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSnapshotViewsRetainPhotosTrashGroupsAndNormalizedChannels(t *testing.T) {
	rows := completeRows()
	replaceField(t, rows, 0, "display_name", "All contacts")
	replaceField(t, rows, 0, "is_default", true)
	replaceField(t, rows, 1, "data", "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:person\r\nFN:Old name\r\nBDAY:--1210\r\nPHOTO:data:image/png;base64,aW1hZ2U=\r\nADR;TYPE=work:;;Street;Paris;;;France\r\nX-KEEP:original\r\nEND:VCARD\r\n")
	var original map[string]any
	if err := json.Unmarshal(rows[1].JSON, &original); err != nil {
		t.Fatal(err)
	}
	replaceField(t, rows, 1, "size_bytes", len(original["data"].(string)))
	replaceField(t, rows, 2, "note", "Private note")
	replaceField(t, rows, 2, "updated_at", "2026-10-30T10:11:12Z")
	snapshot, err := NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	books, err := snapshot.Books()
	if err != nil || len(books) != 1 {
		t.Fatalf("books %v %v", books, err)
	}
	if books[0].DisplayName != "All contacts" || !books[0].IsDefault || books[0].DeletedAt == nil {
		t.Fatal("book metadata lost")
	}
	people, err := snapshot.Contacts()
	if err != nil || len(people) != 1 {
		t.Fatalf("contacts %v %v", people, err)
	}
	person := people[0]
	if person.ID != contactID || person.ObjectID != cardID || person.FormattedName != "Ada" || person.Note != "Private note" || !person.Favorite || person.DeletedAt == nil || !person.BookDeleted {
		t.Fatalf("lost contact state: %+v", person)
	}
	if person.Birthday == nil || person.Birthday.Format(time.DateOnly) != "1815-12-10" || person.ETag != cardID || !person.HasPhoto {
		t.Fatal("lost birthday, ETag or embedded photo")
	}
	if len(person.Emails) != 1 || person.Emails[0].Email != "ada@example.invalid" || !person.Emails[0].IsPrimary || len(person.Phones) != 1 || len(person.Addresses) != 1 || person.Addresses[0].Locality != "Paris" {
		t.Fatal("lost channels or vCard address")
	}
	if !strings.Contains(person.VCard, "BDAY:--1210") || !strings.Contains(person.VCard, "X-KEEP:original") {
		t.Fatal("reencoded original card")
	}
	groups, err := snapshot.Groups()
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups %v %v", groups, err)
	}
	if groups[0].ID != groupID || groups[0].ObjectID != groupCardID || !groups[0].BookDeleted || len(groups[0].Members) != 2 || groups[0].Members[0] != "person" || groups[0].Members[1] != "remote" {
		t.Fatal("lost group or unresolved member")
	}
	// Public view buffers must never mutate the retained snapshot.
	people[0].Emails[0].Email = "changed"
	groups[0].Members[0] = "changed"
	again, err := snapshot.Contacts()
	if err != nil || again[0].Emails[0].Email != "ada@example.invalid" {
		t.Fatal("caller changed snapshot emails")
	}
	more, err := snapshot.Groups()
	if err != nil || more[0].Members[0] != "person" {
		t.Fatal("caller changed snapshot members")
	}
}

func TestSnapshotViewsRejectInvalidRepresentationsWithoutPartialSuccess(t *testing.T) {
	for _, test := range []struct {
		name  string
		index int
		field string
		value any
	}{
		{"birthday", 2, "birthday", "not-a-date"},
		{"timestamp", 2, "deleted_at", "not-a-timestamp"},
		{"channel", 5, "position", "not-an-integer"},
		{"malformed-vcard", 1, "data", "invalid card"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := completeRows()
			replaceField(t, rows, test.index, test.field, test.value)
			if test.name == "malformed-vcard" {
				replaceField(t, rows, 1, "size_bytes", len(test.value.(string)))
			}
			snapshot, err := NewSnapshot(testScope(), rows)
			if err != nil {
				t.Fatal(err)
			}
			if people, err := snapshot.Contacts(); err == nil || people != nil {
				t.Fatal("accepted invalid projection or returned partial data")
			}
		})
	}
}
