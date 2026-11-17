// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func queryRows(t *testing.T, total int) []Row {
	t.Helper()
	rows := activeRows(t)
	for i := 1; i < total; i++ {
		objectID, id := uuid.NewString(), uuid.NewString()
		uid := fmt.Sprintf("person-%04d", i)
		name := fmt.Sprintf("Contact %04d", i)
		data := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:" + uid + "\r\nFN:" + name + "\r\nEND:VCARD\r\n"
		rows = append(rows, fixtureRow("card", objectID, map[string]any{"address_book_id": bookID, "uid": uid, "etag": objectID, "resource_name": uid + ".vcf", "data": data, "size_bytes": len(data)}), fixtureRow("contact", id, map[string]any{"address_book_id": bookID, "object_id": objectID, "uid": uid, "formatted_name": name}))
	}
	return rows
}

func statusIs(t *testing.T, err error, status int) {
	t.Helper()
	var failure *api.Error
	if !errors.As(err, &failure) || failure.Status != status {
		t.Fatalf("got %v want HTTP-like %d", err, status)
	}
}

func TestPrivateContactQueriesPageCompleteBooksAndRejectChangedOrForeignCursors(t *testing.T) {
	rows := queryRows(t, 205)
	snapshot, err := NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	cursor := ""
	firstCursor := ""
	for {
		page, err := snapshot.QueryBook(bookID, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, person := range page.Contacts {
			if seen[person.ID] {
				t.Fatal("duplicate page item")
			}
			seen[person.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		if firstCursor == "" {
			firstCursor = page.NextCursor
		}
		cursor = page.NextCursor
	}
	if len(seen) != 205 || firstCursor == "" {
		t.Fatal("incomplete private address book", len(seen))
	}
	replaceField(t, rows, 2, "note", "changed after first page")
	changed, err := NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	page, err := changed.QueryBook(bookID, firstCursor)
	statusIs(t, err, http.StatusConflict)
	if len(page.Contacts) != 0 {
		t.Fatal("returned partial page after mutation")
	}
	scope := testScope()
	scope.Subject = "bob"
	replaceField(t, rows, 0, "owner_sub", "bob")
	foreign, err := NewSnapshot(scope, rows)
	if err != nil {
		t.Fatal(err)
	}
	_, err = foreign.QueryBook(bookID, firstCursor)
	statusIs(t, err, http.StatusConflict)
	_, err = snapshot.QueryBook(uuid.NewString(), "")
	statusIs(t, err, http.StatusNotFound)
	_, err = snapshot.QueryBook(bookID, "malformed")
	statusIs(t, err, http.StatusBadRequest)
	contacts, next, err := snapshot.QueryAll(100, 200)
	if err != nil || len(contacts) != 5 || next != 0 {
		t.Fatal("offset projection changed", next, err)
	}
}

func TestPrivateContactQueriesFoldTextHideTrashAndPreserveExportAndPhotos(t *testing.T) {
	rows := activeRows(t)
	replaceField(t, rows, 2, "formatted_name", "Áda Müller")
	replaceField(t, rows, 2, "note", "=private note")
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	data := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:person\r\nFN:Áda Müller\r\nPHOTO:data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes.Bytes()) + "\r\nX-KEEP:original\r\nEND:VCARD\r\n"
	replaceField(t, rows, 1, "data", data)
	replaceField(t, rows, 1, "size_bytes", len(data))
	snapshot, err := NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	results, err := snapshot.QuerySearch("ada muller", 50)
	if err != nil || len(results) != 1 {
		t.Fatal("search did not fold text", err)
	}
	_, err = snapshot.QuerySearch("", 50)
	statusIs(t, err, http.StatusBadRequest)
	photo, err := snapshot.QueryPhoto("urn:uuid:person")
	if err != nil || !bytes.Equal(photo, imageBytes.Bytes()) {
		t.Fatal("photo changed", err)
	}
	_, err = snapshot.QueryPhoto("other")
	statusIs(t, err, http.StatusNotFound)
	favorite, err := snapshot.QueryFavorites()
	if err != nil || len(favorite) != 1 || favorite[0] != "person" {
		t.Fatal("favorite lost", err)
	}
	name, csv, err := snapshot.QueryCSV()
	if err != nil || name != "contacts.csv" || !strings.Contains(string(csv), "'=private note") {
		t.Fatal("unsafe or incomplete CSV", err)
	}
	group, err := snapshot.QueryGroups(bookID)
	if err != nil || len(group) != 1 || len(group[0].Members) != 2 {
		t.Fatal("group members lost", err)
	}
	now := time.Now().UTC()
	replaceField(t, rows, 2, "deleted_at", now.Format(time.RFC3339Nano))
	snapshot, err = NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	visible, _, err := snapshot.QueryAll(100, 0)
	if err != nil || len(visible) != 0 {
		t.Fatal("trash leaked into visible contacts", err)
	}
	_, err = snapshot.FindContact(contactID)
	statusIs(t, err, http.StatusNotFound)
	trash, err := snapshot.QueryTrash(200)
	if err != nil || len(trash) != 1 || trash[0].ID != contactID || trash[0].PrimaryEmail != "ada@example.invalid" {
		t.Fatal("trash metadata lost", err)
	}
}
