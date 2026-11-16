// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"testing"
)

const (
	photoIDBook    = "30000000-0000-4000-8000-000000000001"
	photoIDCard    = "30000000-0000-4000-8000-000000000002"
	photoIDContact = "30000000-0000-4000-8000-000000000003"
	photoNewETag   = "30000000-0000-4000-8000-000000000004"
)

func photoByIDImage(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, c)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func photoByIDCard(name string, photo []byte) string {
	return "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:person\r\nFN:" + name + "\r\nPHOTO:data:image/png;base64," + base64.StdEncoding.EncodeToString(photo) + "\r\nEND:VCARD\r\n"
}

func photoByIDRows(t *testing.T) ([]Row, []byte, []byte) {
	t.Helper()
	first := photoByIDImage(t, color.RGBA{R: 255, A: 255})
	second := photoByIDImage(t, color.RGBA{B: 255, A: 255})
	rows := activeRows(t)
	firstCard := photoByIDCard("Ada", first)
	replaceField(t, rows, 1, "data", firstCard)
	replaceField(t, rows, 1, "size_bytes", len(firstCard))
	secondCard := photoByIDCard("Bea", second)
	rows = append(rows,
		fixtureRow("book", photoIDBook, map[string]any{"owner_sub": "alice", "name": "work", "display_name": "Work"}),
		fixtureRow("card", photoIDCard, map[string]any{"address_book_id": photoIDBook, "uid": "person", "etag": photoIDCard, "resource_name": "person.vcf", "data": secondCard, "size_bytes": len(secondCard)}),
		fixtureRow("contact", photoIDContact, map[string]any{"address_book_id": photoIDBook, "object_id": photoIDCard, "uid": "person", "formatted_name": "Bea"}),
	)
	return rows, first, second
}

func TestPhotoByIDUsesOneSnapshotAndDistinguishesDuplicateUIDs(t *testing.T) {
	rows, first, second := photoByIDRows(t)
	catalog := catalogRows(rows)
	snapshot, err := readCatalog(t.Context(), testScope(), catalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		id, etag string
		want     []byte
	}{
		{contactID, cardID, first},
		{photoIDContact, photoIDCard, second},
	} {
		got, err := snapshot.queryPhotoByID(test.id, test.etag)
		if err != nil || !bytes.Equal(got, test.want) {
			t.Fatal("photo did not match contact and card revision", test.id, err)
		}
	}
	if len(catalog.returned) != len(rows) {
		t.Fatal("photo read consulted another catalog snapshot")
	}
	for _, test := range []struct {
		id, etag string
		status   int
	}{
		{"missing", cardID, http.StatusNotFound},
		{contactID, "", http.StatusBadRequest},
		{contactID, " ", http.StatusBadRequest},
		{contactID, photoIDCard, http.StatusPreconditionFailed},
		{photoIDContact, cardID, http.StatusPreconditionFailed},
	} {
		got, err := snapshot.queryPhotoByID(test.id, test.etag)
		statusIs(t, err, test.status)
		if got != nil {
			t.Fatal("failed photo request exposed bytes")
		}
	}
}

func TestPhotoByIDRejectsChangedOrHiddenCard(t *testing.T) {
	for _, hidden := range []string{"book", "card", "contact"} {
		rows, _, _ := photoByIDRows(t)
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
		got, err := snapshot.queryPhotoByID(photoIDContact, photoIDCard)
		statusIs(t, err, http.StatusNotFound)
		if got != nil {
			t.Fatal("hidden contact exposed photo bytes")
		}
	}
	rows, first, second := photoByIDRows(t)
	old, err := NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	changedCard := photoByIDCard("Ada", second)
	replaceField(t, rows, 1, "data", changedCard)
	replaceField(t, rows, 1, "size_bytes", len(changedCard))
	replaceField(t, rows, 1, "etag", photoNewETag)
	current, err := NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	got, err := old.queryPhotoByID(contactID, cardID)
	if err != nil || !bytes.Equal(got, first) {
		t.Fatal("previous snapshot changed while reading it", err)
	}
	got, err = current.queryPhotoByID(contactID, cardID)
	statusIs(t, err, http.StatusPreconditionFailed)
	if got != nil {
		t.Fatal("stale revision exposed current photo")
	}
	got, err = current.queryPhotoByID(contactID, photoNewETag)
	if err != nil || !bytes.Equal(got, second) {
		t.Fatal("current revision returned the wrong photo", err)
	}
	without := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:person\r\nFN:Ada\r\nEND:VCARD\r\n"
	replaceField(t, rows, 1, "data", without)
	replaceField(t, rows, 1, "size_bytes", len(without))
	current, err = NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	got, err = current.queryPhotoByID(contactID, photoNewETag)
	statusIs(t, err, http.StatusNotFound)
	if got != nil {
		t.Fatal("contact without a photo returned bytes")
	}
}
