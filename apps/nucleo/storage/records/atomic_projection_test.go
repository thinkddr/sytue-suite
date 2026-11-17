// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// The port supplies already authenticated records; SDK provenance is exercised
// separately in the remote integration test, not inferred from this fixture.
type atomicProjectionPort struct {
	Crypto
	binding storage.Binding
	entries []storage.RecordEntry
	pages   map[storage.Reference]storage.Record
}

func (p *atomicProjectionPort) StorageBindings(context.Context) (storage.Binding, error) {
	return p.binding, nil
}

func (p *atomicProjectionPort) StorageObjectHistory(context.Context, string, string, int) (storage.ObjectPage, error) {
	return storage.ObjectPage{Entries: p.entries}, nil
}

func (p *atomicProjectionPort) ReadStorageRecord(_ context.Context, ref storage.Reference, _, _ string) (storage.Record, error) {
	record, ok := p.pages[ref]
	if !ok {
		return storage.Record{}, storage.ErrIncompleteCatalog
	}
	return record, nil
}

func TestRefreshProjectsOnlyCompleteAtomicBatchAndRetainsRawHistory(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	r := &Repository{cfg: Config{Scope: scope, Product: "notes"}}
	object := r.object("note", "id")
	work := mutation{Entity: "note", ID: "id", Acknowledge: true}
	room := "!private:test"
	old := storage.Reference{RoomID: room, EventID: "$old"}
	prepared := storage.Reference{RoomID: room, EventID: "$prepared"}
	pageRef := storage.Reference{RoomID: room, EventID: "$page"}
	rootRef := storage.Reference{RoomID: room, EventID: "$root"}
	encode := func(value any) []byte {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	oldCommit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "deleted", Metadata: []byte(`{"v":1}`)}
	newCommit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "deleted", Metadata: []byte(`{"v":2}`), Parents: []storage.Reference{old}}
	oldEntry := storage.RecordEntry{Reference: old, Record: storage.Record{Version: 1, Scope: scope, ObjectID: object, RevisionID: "old", Role: "index", Payload: encode(oldCommit)}}
	preparedEntry := storage.RecordEntry{Reference: prepared, Record: storage.Record{Version: 1, Scope: scope, ObjectID: object, RevisionID: "new", Role: "index", Previous: []string{old.EventID}, Payload: encode(storage.AtomicPrepared{Kind: storage.ObjectPreparedKind, BatchID: "batch", Ordinal: 0, Commit: newCommit})}}
	memberSHA, err := storage.AtomicRecordSHA256(preparedEntry.Record)
	if err != nil {
		t.Fatal(err)
	}
	page := storage.Record{Version: 1, Scope: scope, ObjectID: storage.AtomicBatchObjectID("batch"), RevisionID: "page", Role: "manifest", Payload: encode(storage.AtomicBatchPage{Kind: storage.ObjectBatchPageKind, BatchID: "batch", Members: []storage.AtomicMember{{Reference: prepared, ObjectID: object, RevisionID: "new", SHA256: memberSHA}}})}
	pageSHA, err := storage.AtomicRecordSHA256(page)
	if err != nil {
		t.Fatal(err)
	}
	root := storage.RecordEntry{Reference: rootRef, Record: storage.Record{Version: 1, Scope: scope, ObjectID: storage.AtomicBatchObjectID("batch"), RevisionID: "root", Role: "index", Payload: encode(storage.AtomicBatchCommit{Kind: storage.ObjectBatchKind, BatchID: "batch", Count: 1, Last: storage.AtomicPagePointer{Reference: pageRef, RevisionID: "page", SHA256: pageSHA}})}}
	port := &atomicProjectionPort{binding: storage.Binding{Scope: scope, Target: storage.PublicationTarget{RoomID: room, DeviceID: "DEVICE"}, Rooms: []string{room}}, entries: []storage.RecordEntry{oldEntry, preparedEntry}, pages: map[storage.Reference]storage.Record{pageRef: page}}
	r.cfg.Crypto = port
	defer r.Close()
	if err := r.refresh(t.Context()); err != nil {
		t.Fatal("orphan prevented an existing catalog from loading", err)
	}
	if len(r.catalog.Entries) != 2 || len(r.catalog.Pages) != 0 || len(r.catalog.History) != 1 || len(r.heads[object]) != 1 || r.heads[object][0].Reference != old {
		t.Fatal("uncommitted work became visible or raw history was lost")
	}
	r.ready = true
	if _, err := r.acceptedMutationVersion(work, prepared); err == nil {
		t.Fatal("orphan was accepted for local acknowledgment")
	}
	port.entries = append(port.entries, root)
	if err := r.refresh(t.Context()); err != nil {
		t.Fatal("complete batch did not load", err)
	}
	if len(r.catalog.Entries) != 3 || len(r.catalog.Pages) != 1 || len(r.catalog.History) != 2 || len(r.byObject[object]) != 2 || len(r.heads[object]) != 1 || r.heads[object][0].Reference != prepared {
		t.Fatal("complete batch did not become visible atomically")
	}
	if !bytes.Equal(r.catalog.Entries[prepared].Record.Payload, preparedEntry.Record.Payload) {
		t.Fatal("prepared envelope was rewritten")
	}
	if _, err := r.acceptedMutationVersion(work, prepared); err != nil {
		t.Fatal("committed member not available for acknowledgment", err)
	}
	before := r.catalog
	delete(port.pages, pageRef)
	if err := r.refresh(t.Context()); err == nil {
		t.Fatal("missing manifest was accepted")
	}
	if !storage.SameAtomicCatalog(before, r.catalog) || r.heads[object][0].Reference != prepared {
		t.Fatal("failed refresh replaced the previous projection")
	}
}

type atomicBinaryAuditPort struct {
	*binaryAuditProbe
	pages map[storage.Reference]storage.Record
}

func (p *atomicBinaryAuditPort) ReadStorageRecord(_ context.Context, ref storage.Reference, _, _ string) (storage.Record, error) {
	record, ok := p.pages[ref]
	if !ok {
		return storage.Record{}, storage.ErrIncompleteCatalog
	}
	return record, nil
}

func TestBinaryAuditIncludesCommittedAtomicMembersButNotOrphans(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "orphan", true: "committed"}[committed], func(t *testing.T) {
			r, base, entry := binaryCatalogFixture(t)
			defer r.Close()
			entry.Version.Commit.Kind = storage.ObjectKind
			row := binaryRow(mustBinaryDescription(t, r, entry.Version))
			encode := func(value any) []byte {
				t.Helper()
				payload, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				return payload
			}
			ref := entry.Version.Reference
			pageRef := storage.Reference{RoomID: ref.RoomID, EventID: "$page"}
			prepared := storage.RecordEntry{Reference: ref, Record: storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: entry.Version.ObjectID, RevisionID: entry.Version.RevisionID, Role: "index", Payload: encode(storage.AtomicPrepared{Kind: storage.ObjectPreparedKind, BatchID: "batch", Commit: entry.Version.Commit})}}
			memberSHA, err := storage.AtomicRecordSHA256(prepared.Record)
			if err != nil {
				t.Fatal(err)
			}
			page := storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: storage.AtomicBatchObjectID("batch"), RevisionID: "page", Role: "manifest", Payload: encode(storage.AtomicBatchPage{Kind: storage.ObjectBatchPageKind, BatchID: "batch", Members: []storage.AtomicMember{{Reference: ref, ObjectID: entry.Version.ObjectID, RevisionID: entry.Version.RevisionID, SHA256: memberSHA}}})}
			pageSHA, err := storage.AtomicRecordSHA256(page)
			if err != nil {
				t.Fatal(err)
			}
			binding := storage.Binding{Scope: r.cfg.Scope, Target: storage.PublicationTarget{RoomID: ref.RoomID, DeviceID: "TEST"}, Rooms: []string{ref.RoomID}}
			probe := &atomicBinaryAuditPort{binaryAuditProbe: &binaryAuditProbe{commitmentReadProbe: &commitmentReadProbe{binaryReadProbe: base, digest: row.SHA256}, binding: binding, entries: []storage.RecordEntry{prepared}}, pages: map[storage.Reference]storage.Record{pageRef: page}}
			if committed {
				probe.entries = append(probe.entries, storage.RecordEntry{Reference: storage.Reference{RoomID: ref.RoomID, EventID: "$root"}, Record: storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: storage.AtomicBatchObjectID("batch"), RevisionID: "root", Role: "index", Payload: encode(storage.AtomicBatchCommit{Kind: storage.ObjectBatchKind, BatchID: "batch", Count: 1, Last: storage.AtomicPagePointer{Reference: pageRef, RevisionID: "page", SHA256: pageSHA}})}})
			}
			r.cfg.Crypto, r.binding = probe, binding
			snapshot, err := storage.LoadAtomicCatalog(t.Context(), probe, r.cfg.Scope)
			if err != nil {
				t.Fatal(err)
			}
			result, err := r.auditBinaryEntries(t.Context(), snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if committed && (result.Objects != 1 || result.Bytes != row.Size || probe.downloads != 1) {
				t.Fatal("committed binary was not fully audited", result, probe.downloads)
			}
			if !committed && (result.Objects != 0 || probe.downloads != 0) {
				t.Fatal("orphan binary was treated as retained", result, probe.downloads)
			}
		})
	}
}
