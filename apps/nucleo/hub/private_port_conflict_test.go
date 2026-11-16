// SPDX-License-Identifier: Apache-2.0
package hub

import (
	"bytes"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// The in-memory test port must model transaction idempotency: a reused
// transaction cannot acknowledge a different record or index as the original.
func TestMemoryStorageRejectsReusedTransactionWithDifferentPayload(t *testing.T) {
	binding := storage.Binding{
		Scope:  noticeFixture().Scope,
		Target: storage.PublicationTarget{RoomID: "!hub:test", DeviceID: "DEVICE"},
		Rooms:  []string{"!hub:test"},
	}
	newPort := func() *memoryStorage {
		return &memoryStorage{binding: binding, records: map[storage.Reference]storage.Record{}, objects: map[storage.Reference]storage.ObjectVersion{}, tx: map[string]storage.Reference{}}
	}
	t.Run("record", func(t *testing.T) {
		port := newPort()
		original := storage.Record{Version: 1, Scope: binding.Scope, ObjectID: "notice", RevisionID: "rev1", Role: "content", Payload: []byte(`{"subject":"first"}`)}
		ref, err := port.PutStorageRecordAt(t.Context(), binding.Target, "same", original)
		if err != nil {
			t.Fatal(err)
		}
		again, err := port.PutStorageRecordAt(t.Context(), binding.Target, "same", original)
		if err != nil || again != ref {
			t.Fatalf("identical retry: %v %v", again, err)
		}
		changed := original
		changed.Payload = []byte(`{"subject":"other"}`)
		if _, err := port.PutStorageRecordAt(t.Context(), binding.Target, "same", changed); !errors.Is(err, records.ErrConflict) {
			t.Fatalf("changed retry: %v", err)
		}
		got, err := port.ReadStorageRecord(t.Context(), ref, original.ObjectID, original.Role)
		if err != nil || !bytes.Equal(got.Payload, original.Payload) || len(port.records) != 1 {
			t.Fatalf("original record changed: %+v %v", got, err)
		}
	})
	t.Run("index", func(t *testing.T) {
		port := newPort()
		original := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: []byte(`{"subject":"first"}`), Content: &storage.ContentPointer{Kind: "record", RevisionID: "rev1", Reference: storage.Reference{RoomID: "!hub:test", EventID: "$content"}}}
		version, err := port.PutStorageObjectAt(t.Context(), binding.Target, "same", "notice", "rev1_index", original)
		if err != nil {
			t.Fatal(err)
		}
		again, err := port.PutStorageObjectAt(t.Context(), binding.Target, "same", "notice", "rev1_index", original)
		if err != nil || again.Reference != version.Reference {
			t.Fatalf("identical retry: %+v %v", again, err)
		}
		changed := original
		changed.Metadata = []byte(`{"subject":"other"}`)
		if _, err := port.PutStorageObjectAt(t.Context(), binding.Target, "same", "notice", "rev1_index", changed); !errors.Is(err, records.ErrConflict) {
			t.Fatalf("changed retry: %v", err)
		}
		got, err := port.StorageObject(t.Context(), version.Reference, "notice")
		if err != nil || !bytes.Equal(got.Commit.Metadata, original.Metadata) || len(port.objects) != 1 {
			t.Fatalf("original index changed: %+v %v", got, err)
		}
	})
}
