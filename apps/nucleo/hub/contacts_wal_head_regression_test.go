// SPDX-License-Identifier: Apache-2.0
package hub

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	privatecontacts "git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

func contactsWALDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func contactsWALJournal(t *testing.T) *offline.Store {
	t.Helper()
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	return journal
}

func contactsWALRepository(t *testing.T, port *memoryStorage, source *emptySource, journal *offline.Store) *privatecontacts.Repository {
	t.Helper()
	repository, err := privatecontacts.New(privatecontacts.Config{Context: t.Context(), Scope: port.binding.Scope, Crypto: port, Gateway: &api.Drive{}, Journal: journal, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repository.Close)
	return repository
}

func contactsWALHeads(t *testing.T, port *memoryStorage, object string) []storage.ObjectVersion {
	t.Helper()
	catalog, err := storage.LoadCatalog(t.Context(), port, port.binding.Scope)
	if err != nil {
		t.Fatal("read complete catalog", err)
	}
	var result []storage.ObjectVersion
	for _, head := range catalog.Heads {
		if head.ObjectID == object {
			result = append(result, head)
		}
	}
	return result
}

func contactsWALContent(t *testing.T, port *memoryStorage, head storage.ObjectVersion) []byte {
	t.Helper()
	if head.Commit.Content == nil || head.Commit.Content.Kind != "record" {
		t.Fatal("expected small encrypted-record fixture")
	}
	record, err := port.ReadStorageRecord(t.Context(), head.Commit.Content.Reference, head.ObjectID, "content")
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Kind string `json:"kind"`
		JSON []byte `json:"json"`
	}
	if json.Unmarshal(record.Payload, &value) != nil || value.Kind != "org.sytue.contacts.entity.v1" {
		t.Fatal("invalid Contacts content fixture")
	}
	return value.JSON
}

// The first writer's WAL is seeded at the exact documented pre-publication
// shape because the concrete offline.Store offers no crash-injection hook.
// Both edits and the resumed call use real Contacts repositories and the real
// records engine; memoryStorage is not a Matrix SDK or a power-loss test.
func TestContactsRetainedUpdateETagDoesNotOverwriteLaterWriter(t *testing.T) {
	for _, sameJournal := range []bool{false, true} {
		name := "independent-journals"
		if sameJournal {
			name = "same-journal"
		}
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "contacts"}
			binding := storage.Binding{Scope: scope, Target: storage.PublicationTarget{RoomID: "!contacts:test", DeviceID: "DEVICE"}, Rooms: []string{"!contacts:test"}}
			port := &memoryStorage{binding: binding, records: map[storage.Reference]storage.Record{}, objects: map[storage.Reference]storage.ObjectVersion{}, tx: map[string]storage.Reference{}}
			source := &emptySource{state: records.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}}
			journalA, journalB := contactsWALJournal(t), contactsWALJournal(t)
			if sameJournal {
				journalB = journalA
			}
			a := contactsWALRepository(t, port, source, journalA)
			if err := a.ImportLegacySnapshot(ctx); err != nil {
				t.Fatal("initialize empty private Contacts", err)
			}
			book, err := a.CreateAddressBook(ctx, "Private")
			if err != nil {
				t.Fatal(err)
			}
			original, err := a.Create(ctx, book.ID, api.ContactInput{FormattedName: "Original", Note: "initial note"})
			if err != nil || original.ID == "" || original.ETag == "" {
				t.Fatal("create initial contact", err)
			}
			object := "contacts_" + contactsWALDigest([]byte("contact_state\x00"+original.ID))
			initialHeads := contactsWALHeads(t, port, object)
			if len(initialHeads) != 1 {
				t.Fatal("expected initial contact head", len(initialHeads))
			}
			initialHead := initialHeads[0].Reference
			editA := original.Input()
			editA.FormattedName = "A changed the name"
			snapshot, err := a.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			changeA, err := snapshot.UpdatePerson(original.ID, editA, original.ETag, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal("prepare original Contacts edit", err)
			}
			bodyA, err := json.Marshal(changeA)
			if err != nil {
				t.Fatal(err)
			}
			input := struct {
				ID, ETag string
				Input    api.ContactInput
			}{original.ID, original.ETag, editA}
			encodedInput, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			fingerprint := contactsWALDigest(encodedInput)
			key := "mutation_" + contactsWALDigest([]byte("update-contact\x00"+fingerprint))
			owner, err := scope.JournalOwner()
			if err != nil {
				t.Fatal(err)
			}
			// Same serialized fields as records' existing version-1 mutation WAL.
			pending := struct {
				Version   int           `json:"v"`
				Scope     storage.Scope `json:"scope"`
				Operation string        `json:"operation"`
				Input     string        `json:"input"`
				Entity    string        `json:"entity"`
				ID        string        `json:"id"`
				State     string        `json:"state"`
				JSON      []byte        `json:"json"`
			}{1, scope, "update-contact", fingerprint, changeA.Kind, changeA.ID, "live", bodyA}
			if err := journalA.Write(owner, "contacts-private-v1", key, pending); err != nil {
				t.Fatal("seed retained operation", err)
			}
			var publication json.RawMessage
			if found, err := journalA.Read(owner, "contacts-private-v1", key+"_publication", &publication); err != nil || found {
				t.Fatal("publication was already planned", err)
			}
			// Prove the public Update really addresses this exact journal key.
			// With an unrecognised version it must fail before publishing; if our
			// fingerprint or serialization were wrong, it would prepare a new edit.
			invalid := pending
			invalid.Version = 999
			if err := journalA.Write(owner, "contacts-private-v1", key, invalid); err != nil {
				t.Fatal(err)
			}
			beforeIDs, err := journalA.RecordIDs(owner, "contacts-private-v1")
			if err != nil {
				t.Fatal(err)
			}
			slices.Sort(beforeIDs)
			beforeJournal, err := journalA.List(owner, "contacts-private-v1")
			if err != nil {
				t.Fatal(err)
			}
			slices.SortFunc(beforeJournal, func(a, b offline.Record) int { return strings.Compare(a.ID, b.ID) })
			beforeObjects := make(map[storage.Reference]storage.ObjectVersion, len(port.objects))
			for ref, object := range port.objects {
				beforeObjects[ref] = cloneVersion(object)
			}
			beforeRecords := make(map[storage.Reference]storage.Record, len(port.records))
			for ref, record := range port.records {
				beforeRecords[ref] = cloneRecord(record)
			}
			beforeTransactions := maps.Clone(port.tx)
			if _, err := a.Update(ctx, original.ID, editA, original.ETag); !errors.Is(err, records.ErrSource) {
				t.Fatal("Update did not reject the deliberately invalid retained WAL", err)
			}
			afterIDs, err := journalA.RecordIDs(owner, "contacts-private-v1")
			if err != nil {
				t.Fatal(err)
			}
			slices.Sort(afterIDs)
			afterJournal, err := journalA.List(owner, "contacts-private-v1")
			if err != nil {
				t.Fatal(err)
			}
			slices.SortFunc(afterJournal, func(a, b offline.Record) int { return strings.Compare(a.ID, b.ID) })
			if !slices.Equal(beforeIDs, afterIDs) || !reflect.DeepEqual(beforeJournal, afterJournal) || !reflect.DeepEqual(beforeObjects, port.objects) || !reflect.DeepEqual(beforeRecords, port.records) || !maps.Equal(beforeTransactions, port.tx) {
				t.Fatal("invalid retained WAL changed journals or published data")
			}
			var invalidSaved struct {
				Version int `json:"v"`
			}
			if found, err := journalA.Read(owner, "contacts-private-v1", key, &invalidSaved); err != nil || !found || invalidSaved.Version != 999 {
				t.Fatal("invalid retained WAL was lost or rewritten", err)
			}
			if found, err := journalA.Read(owner, "contacts-private-v1", key+"_publication", &publication); err != nil || found {
				t.Fatal("invalid retained WAL created a publication plan", err)
			}
			if err := journalA.Write(owner, "contacts-private-v1", key, pending); err != nil {
				t.Fatal("restore exact retained WAL", err)
			}
			b := contactsWALRepository(t, port, source, journalB)
			if err := b.Initialize(ctx); err != nil {
				t.Fatal("initialize second writer", err)
			}
			editB := original.Input()
			editB.Note = "changed by B"
			updatedB, err := b.Update(ctx, original.ID, editB, original.ETag)
			if err != nil || updatedB.Note != editB.Note || updatedB.ETag == original.ETag {
				t.Fatal("B did not publish a new contact revision", err)
			}
			headsB := contactsWALHeads(t, port, object)
			if len(headsB) != 1 || !slices.Equal(headsB[0].Commit.Parents, []storage.Reference{initialHead}) {
				t.Fatal("B did not advance the initial contact")
			}
			bHead, bBody := headsB[0].Reference, contactsWALContent(t, port, headsB[0])
			_, retryErr := a.Update(ctx, original.ID, editA, original.ETag)
			if retryErr != nil {
				var response *api.Error
				if !errors.Is(retryErr, records.ErrConflict) && (!errors.As(retryErr, &response) || response.Status != http.StatusPreconditionFailed) {
					t.Fatal("retry failed for a reason unrelated to the changed revision", retryErr)
				}
			}
			heads := contactsWALHeads(t, port, object)
			if len(heads) != 1 && len(heads) != 2 {
				t.Fatal("unexpected contact heads", len(heads))
			}
			foundB, foundA := false, false
			for _, head := range heads {
				body := contactsWALContent(t, port, head)
				if head.Reference == bHead {
					foundB = bytes.Equal(body, bBody)
				} else {
					foundA = bytes.Equal(body, bodyA) && slices.Equal(head.Commit.Parents, []storage.Reference{initialHead})
				}
			}
			if !foundB || (len(heads) == 2 && !foundA) {
				t.Fatal("retry erased B's exact edit or attached stale A as B's child")
			}
			if len(heads) == 1 {
				if retryErr == nil {
					t.Fatal("stale ETag was accepted without a separate branch")
				}
				var saved struct {
					JSON []byte `json:"json"`
				}
				if found, err := journalA.Read(owner, "contacts-private-v1", key, &saved); err != nil || !found || !bytes.Equal(saved.JSON, bodyA) {
					t.Fatal("failed retry discarded or changed original WAL", err)
				}
			}
		})
	}
}
