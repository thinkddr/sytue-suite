// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/model"
)

func applyChange(t *testing.T, snapshot *Snapshot, change Aggregate) *Snapshot {
	t.Helper()
	next, err := snapshot.WithAggregates([]Aggregate{change})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateViews(next); err != nil {
		t.Fatal(err)
	}
	return next
}

func TestPrivateMutationsCreateEditAndKeepUnknownCardMetadata(t *testing.T) {
	rows := activeRows(t)
	data := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:person\r\nFN:Ada\r\nBDAY:--1210\r\nX-KEEP:preserve me\r\nEND:VCARD\r\n"
	replaceField(t, rows, 1, "data", data)
	replaceField(t, rows, 1, "size_bytes", len(data))
	replaceField(t, rows, 2, "birthday", nil)
	replaceField(t, rows, 2, "future", map[string]any{"keep": "yes"})
	snapshot, err := NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	existing, err := snapshot.FindContact(contactID)
	if err != nil {
		t.Fatal(err)
	}
	input := existing.Input()
	input.FormattedName = "Edited Ada"
	input.Note = ""
	input.Emails = []api.ContactEmail{{Email: "edited@example.invalid", Type: "work"}}
	change, err := snapshot.UpdatePerson(contactID, input, existing.ETag, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	snapshot = applyChange(t, snapshot, change)
	edited, err := snapshot.FindContact(contactID)
	if err != nil {
		t.Fatal(err)
	}
	if edited.FormattedName != "Edited Ada" || edited.Note != "" || len(edited.Emails) != 1 || edited.Emails[0].Email != "edited@example.invalid" || edited.ETag == existing.ETag {
		t.Fatal("managed projection did not update atomically")
	}
	if !strings.Contains(edited.VCard, "BDAY:--1210") || !strings.Contains(edited.VCard, "X-KEEP:preserve me") {
		t.Fatal("edit discarded hidden or unknown vCard fields")
	}
	for _, row := range change.Rows {
		if row.Kind == "contact" && !bytes.Contains(row.JSON, []byte(`"future":{"keep":"yes"}`)) {
			t.Fatal("edit discarded unknown JSON metadata")
		}
	}
	_, err = snapshot.UpdatePerson(contactID, input, "wrong-etag", time.Now().UTC())
	statusIs(t, err, http.StatusPreconditionFailed)
	person, err := contactInput(api.ContactInput{FormattedName: "New", Birthday: "2000-02-29", Emails: []api.ContactEmail{{Email: "new@example.invalid", IsPrimary: true}}, Addresses: []api.ContactAddress{{Street: "Private street", Locality: "Paris"}}})
	if err != nil {
		t.Fatal(err)
	}
	person.UID = card.NewUID()
	created, err := snapshot.NewPerson(bookID, person, "", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	snapshot = applyChange(t, snapshot, created)
	newPerson, err := snapshot.FindContact(created.ID)
	if err != nil || newPerson.Birthday == nil || newPerson.Birthday.Format(time.DateOnly) != "2000-02-29" || len(newPerson.Addresses) != 1 {
		t.Fatal("created contact lost fields", err)
	}
	_, err = snapshot.NewPerson(bookID, person, "", time.Now().UTC())
	statusIs(t, err, http.StatusConflict)
}

func TestPrivateBookAndGroupMutationsKeepMembershipAndProtectDefault(t *testing.T) {
	snapshot, err := NewSnapshot(testScope(), activeRows(t))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	created, err := snapshot.NewBook("work", "Work", "#112233", now)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = applyChange(t, snapshot, created)
	renamed, err := snapshot.RenameBook(created.ID, "Renamed", now)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = applyChange(t, snapshot, renamed)
	group, err := snapshot.NewGroup(created.ID, "Friends", now)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = applyChange(t, snapshot, group)
	updated, err := snapshot.UpdateGroup(group.ID, "Colleagues", []string{"remote", "remote"}, now)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = applyChange(t, snapshot, updated)
	visible, err := snapshot.QueryGroups(created.ID)
	if err != nil || len(visible) != 1 || visible[0].Name != "Colleagues" || len(visible[0].Members) != 1 || visible[0].Members[0] != "remote" {
		t.Fatal("group mutation lost/doubled unresolved membership", err)
	}
	removed, err := snapshot.DeleteBook(created.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = applyChange(t, snapshot, removed)
	_, err = snapshot.QueryGroups(created.ID)
	statusIs(t, err, http.StatusNotFound)
	rows := activeRows(t)
	replaceField(t, rows, 0, "is_default", true)
	snapshot, err = NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	_, err = snapshot.DeleteBook(bookID, now)
	statusIs(t, err, http.StatusBadRequest)
}

func TestPrivatePhotoAndAtomicStateMutationsPreserveOtherFields(t *testing.T) {
	snapshot, err := NewSnapshot(testScope(), activeRows(t))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := png.Encode(&output, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	data := "data:image/png;base64," + base64.StdEncoding.EncodeToString(output.Bytes())
	change, err := snapshot.ChangePhoto("person", data, false, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	snapshot = applyChange(t, snapshot, change)
	actual, err := snapshot.QueryPhoto("person")
	if err != nil || !bytes.Equal(actual, output.Bytes()) {
		t.Fatal("set photo lost pixels", err)
	}
	change, err = snapshot.ChangePhoto("person", "", true, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	snapshot = applyChange(t, snapshot, change)
	_, err = snapshot.QueryPhoto("person")
	statusIs(t, err, http.StatusNotFound)
	favorites := FavoriteIndex{Version: 1, Scope: testScope(), ID: model.PrivateIndexID, Values: map[string]bool{}}
	marked, err := snapshot.ChangeFavorites(favorites, []string{"person", "missing"}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = snapshot.WithIndexes(&marked, nil)
	if err != nil {
		t.Fatal(err)
	}
	list, err := snapshot.QueryFavorites()
	if err != nil || len(list) != 0 {
		t.Fatal("batch favorite failed", err)
	}
	trash := TrashIndex{Version: 1, Scope: testScope(), ID: model.PrivateIndexID, Values: map[string]Removal{}}
	deleted, err := snapshot.ChangeTrash(trash, "batch-trash", []string{"person", "missing"}, true, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = snapshot.WithIndexes(nil, &deleted)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := snapshot.ChangeTrash(deleted, "restore", []string{contactID}, false, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = snapshot.WithIndexes(nil, &restored)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snapshot.FindContact(contactID); err != nil {
		t.Fatal("restore lost contact", err)
	}
}

func TestPrivateChannelEditPreservesOriginalIdentityAndUnknownMetadata(t *testing.T) {
	rows := activeRows(t)
	replaceField(t, rows, 5, "future", map[string]any{"value": "keep channel metadata"})
	snapshot, err := NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	person, err := snapshot.FindContact(contactID)
	if err != nil {
		t.Fatal(err)
	}
	input := person.Input()
	input.Note = "Only the note changes"
	change, err := snapshot.UpdatePerson(contactID, input, person.ETag, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range change.Rows {
		if row.Kind == "email" {
			if row.ID != emailID || !bytes.Contains(row.JSON, []byte("keep channel metadata")) {
				t.Fatal("editing another field discarded channel metadata")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("email channel lost")
	}
}

// Una fila anterior a la proyección de URL, RELATED y ANNIVERSARY no los trae; la
// tarjeta sí, y editar otro campo no puede borrarlos.
func TestPrivateEditKeepsCardFieldsMissingFromLegacyProjection(t *testing.T) {
	rows := activeRows(t)
	data := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:person\r\nFN:Ada\r\nURL;TYPE=work:https://example.invalid/ada\r\nRELATED;VALUE=text;TYPE=spouse:Grace\r\nANNIVERSARY:20000101\r\nEND:VCARD\r\n"
	replaceField(t, rows, 1, "data", data)
	replaceField(t, rows, 1, "size_bytes", len(data))
	snapshot, err := NewSnapshot(testScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	existing, err := snapshot.FindContact(contactID)
	if err != nil {
		t.Fatal(err)
	}
	input := existing.Input()
	input.Note = "edited"
	change, err := snapshot.UpdatePerson(contactID, input, existing.ETag, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	edited, err := applyChange(t, snapshot, change).FindContact(contactID)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"https://example.invalid/ada", "Grace", "ANNIVERSARY:2000-01-01"} {
		if !strings.Contains(edited.VCard, want) {
			t.Fatalf("edit discarded %q:\n%s", want, edited.VCard)
		}
	}
}
