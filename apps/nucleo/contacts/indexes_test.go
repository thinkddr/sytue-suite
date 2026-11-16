// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/contacts/model"
)

func TestPrivateIndexesApplyBatchFavoriteTrashRestoreAndPurgeAsOneView(t *testing.T) {
	snapshot, err := NewSnapshot(testScope(), activeRows(t))
	if err != nil {
		t.Fatal(err)
	}
	favorites := FavoriteIndex{Version: 1, Scope: testScope(), ID: model.PrivateIndexID, Values: map[string]bool{"person": false, "unresolved": true}}
	now := time.Now().UTC()
	trash := TrashIndex{Version: 1, Scope: testScope(), ID: model.PrivateIndexID, Values: map[string]Removal{contactID: {DeletedAt: &now}}}
	current, err := snapshot.WithIndexes(&favorites, &trash)
	if err != nil {
		t.Fatal(err)
	}
	people, err := current.Contacts()
	if err != nil || len(people) != 1 || people[0].Favorite || people[0].DeletedAt == nil {
		t.Fatal("batch marks lost", err)
	}
	trash.Values[contactID] = Removal{}
	current, err = snapshot.WithIndexes(&favorites, &trash)
	if err != nil {
		t.Fatal(err)
	}
	people, err = current.Contacts()
	if err != nil || people[0].DeletedAt != nil {
		t.Fatal("restore not applied", err)
	}
	trash.Values[contactID] = Removal{DeletedAt: &now, Purged: true}
	current, err = snapshot.WithIndexes(&favorites, &trash)
	if err != nil {
		t.Fatal(err)
	}
	people, err = current.Contacts()
	if err != nil || len(people) != 0 {
		t.Fatal("purge not applied", err)
	}
	groups, err := current.Groups()
	if err != nil || len(groups) != 1 || len(groups[0].Members) != 2 {
		t.Fatal("purge lost group membership", err)
	}
	// The immutable baseline survives, with its own independent preferences.
	people, err = snapshot.Contacts()
	if err != nil || len(people) != 1 || !people[0].Favorite || people[0].DeletedAt != nil {
		t.Fatal("index changed baseline", err)
	}
}

func TestPrivateIndexesRejectForeignScopesMalformedIDsAndInvalidPurge(t *testing.T) {
	snapshot, err := NewSnapshot(testScope(), activeRows(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, favorite := range []FavoriteIndex{
		{Version: 1, Scope: testScope(), ID: model.PrivateIndexID},
		{Version: 1, Scope: testScope(), ID: "other", Values: map[string]bool{}},
		{Version: 2, Scope: testScope(), ID: model.PrivateIndexID, Values: map[string]bool{}},
		{Version: 1, Scope: testScope(), ID: model.PrivateIndexID, Values: map[string]bool{"": true}},
	} {
		if _, err := snapshot.WithIndexes(&favorite, nil); !errors.Is(err, ErrSnapshot) {
			t.Fatal("accepted invalid favorite index", err)
		}
	}
	other := testScope()
	other.Subject = "bob"
	favorite := FavoriteIndex{Version: 1, Scope: other, ID: model.PrivateIndexID, Values: map[string]bool{}}
	if _, err := snapshot.WithIndexes(&favorite, nil); !errors.Is(err, ErrSnapshot) {
		t.Fatal("accepted foreign favorite index")
	}
	for _, removal := range []map[string]Removal{nil, {"not-uuid": {}}, {contactID: {Purged: true}}} {
		trash := TrashIndex{Version: 1, Scope: testScope(), ID: model.PrivateIndexID, Values: removal}
		if _, err := snapshot.WithIndexes(nil, &trash); !errors.Is(err, ErrSnapshot) {
			t.Fatal("accepted invalid removal index", err)
		}
	}
}
