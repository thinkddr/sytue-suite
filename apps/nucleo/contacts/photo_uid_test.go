// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"net/http"
	"reflect"
	"testing"
	"time"
)

const (
	otherPhotoBook    = "20000000-0000-4000-8000-000000000001"
	otherPhotoCard    = "20000000-0000-4000-8000-000000000002"
	otherPhotoContact = "20000000-0000-4000-8000-000000000003"
)

func duplicatePhotoRows(t *testing.T, secondUID string) ([]Row, []byte, string) {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	photo := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
	rows := activeRows(t)
	first := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:person\r\nFN:Ada\r\nPHOTO:" + photo + "\r\nEND:VCARD\r\n"
	replaceField(t, rows, 1, "data", first)
	replaceField(t, rows, 1, "size_bytes", len(first))
	second := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:" + secondUID + "\r\nFN:Bea\r\nEND:VCARD\r\n"
	rows = append(rows,
		fixtureRow("book", otherPhotoBook, map[string]any{"owner_sub": "alice", "name": "work", "display_name": "Work"}),
		fixtureRow("card", otherPhotoCard, map[string]any{"address_book_id": otherPhotoBook, "uid": secondUID, "etag": otherPhotoCard, "resource_name": "person.vcf", "data": second, "size_bytes": len(second)}),
		fixtureRow("contact", otherPhotoContact, map[string]any{"address_book_id": otherPhotoBook, "object_id": otherPhotoCard, "uid": secondUID, "formatted_name": "Bea"}),
	)
	return rows, encoded.Bytes(), photo
}

func TestPrivatePhotoRejectsAmbiguousUIDRegardlessOfOrderOrPhoto(t *testing.T) {
	for _, secondUID := range []string{"person", "urn:uuid:person"} {
		for _, secondFirst := range []bool{false, true} {
			rows, _, photo := duplicatePhotoRows(t, secondUID)
			if secondFirst {
				replaceField(t, rows, 2, "formatted_name", "Zulu")
				replaceField(t, rows, len(rows)-1, "formatted_name", "Aardvark")
			}
			snapshot, err := NewSnapshot(testScope(), rows)
			if err != nil {
				t.Fatal(err)
			}
			before := snapshot.Rows()
			for _, uid := range []string{"person", " urn:uuid:person "} {
				got, err := snapshot.QueryPhoto(uid)
				statusIs(t, err, http.StatusConflict)
				if got != nil {
					t.Fatal("ambiguous UID returned a photo")
				}
				for _, remove := range []bool{false, true} {
					change, err := snapshot.ChangePhoto(uid, photo, remove, time.Now().UTC())
					statusIs(t, err, http.StatusConflict)
					if change.ID != "" || len(change.Rows) != 0 {
						t.Fatal("ambiguous UID produced a mutation")
					}
				}
			}
			if !reflect.DeepEqual(before, snapshot.Rows()) {
				t.Fatal("ambiguous photo operation changed the snapshot")
			}
		}
	}
}

func TestPrivatePhotoIgnoresDeletedDuplicatesAndPreservesNotFound(t *testing.T) {
	for _, hidden := range []string{"book", "card", "contact"} {
		rows, photoBytes, photo := duplicatePhotoRows(t, "person")
		index := len(rows) - 1
		switch hidden {
		case "book":
			index = len(rows) - 3
		case "card":
			index = len(rows) - 2
		}
		replaceField(t, rows, index, "deleted_at", "2020-01-01T00:00:00Z")
		snapshot, err := NewSnapshot(testScope(), rows)
		if err != nil {
			t.Fatal(err)
		}
		got, err := snapshot.QueryPhoto("urn:uuid:person")
		if err != nil || !bytes.Equal(got, photoBytes) {
			t.Fatal("hidden duplicate prevented reading the visible photo", err)
		}
		change, err := snapshot.ChangePhoto("person", photo, false, time.Now().UTC())
		if err != nil || change.ID != contactID {
			t.Fatal("hidden duplicate prevented updating the visible contact", err)
		}
		_, err = snapshot.QueryPhoto("missing")
		statusIs(t, err, http.StatusNotFound)
		_, err = snapshot.ChangePhoto("missing", "", true, time.Now().UTC())
		statusIs(t, err, http.StatusNotFound)
	}
	rows, _, _ := duplicatePhotoRows(t, "person")
	replaceField(t, rows, 2, "deleted_at", "2020-01-01T00:00:00Z")
	snapshot, err := NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	_, err = snapshot.QueryPhoto("person")
	statusIs(t, err, http.StatusNotFound)
}
