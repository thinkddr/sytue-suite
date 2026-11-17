// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Query tests replace only authenticated SDK record reads. The real-Synapse
// integration below covers the SDK boundary; no query may read binary media.
type queryRecords struct {
	Crypto
	records  map[storage.Reference]storage.Record
	repo     *Repository
	extra    map[storage.Reference]storage.ObjectVersion
	retained map[storage.Reference]storage.ObjectVersion
	rawExtra []storage.RecordEntry
}

func (q *queryRecords) StorageBindings(context.Context) (storage.Binding, error) {
	return storage.Binding{Scope: q.repo.cfg.Scope, Target: storage.PublicationTarget{RoomID: "!private:test", DeviceID: "DEVICE"}, Rooms: []string{"!private:test"}}, nil
}

func (q *queryRecords) StorageObjectHistory(_ context.Context, _, _ string, _ int) (storage.ObjectPage, error) {
	q.repo.mu.Lock()
	defer q.repo.mu.Unlock()
	page := storage.ObjectPage{}
	seen := map[storage.Reference]bool{}
	add := func(version storage.ObjectVersion) error {
		ref := version.Reference
		for seen[ref] {
			ref.EventID += "_duplicate"
		}
		seen[ref] = true
		payload, err := json.Marshal(version.Commit)
		if err != nil {
			return err
		}
		previous := make([]string, len(version.Commit.Parents))
		for i, parent := range version.Commit.Parents {
			previous[i] = parent.EventID
		}
		page.Entries = append(page.Entries, storage.RecordEntry{Reference: ref, Record: storage.Record{Version: 1, Scope: q.repo.cfg.Scope, ObjectID: version.ObjectID, RevisionID: version.RevisionID, Role: "index", Previous: previous, Payload: payload}})
		return nil
	}
	for _, heads := range q.repo.heads {
		for _, version := range heads {
			if _, exists := q.extra[version.Reference]; exists {
				continue
			}
			if _, exists := q.retained[version.Reference]; exists {
				continue
			}
			if err := add(version); err != nil {
				return storage.ObjectPage{}, err
			}
		}
	}
	for _, version := range q.extra {
		if err := add(version); err != nil {
			return storage.ObjectPage{}, err
		}
	}
	for _, version := range q.retained {
		if err := add(version); err != nil {
			return storage.ObjectPage{}, err
		}
	}
	page.Entries = append(page.Entries, q.rawExtra...)
	return page, nil
}

func (q *queryRecords) ReadStorageRecord(_ context.Context, ref storage.Reference, object, role string) (storage.Record, error) {
	record, ok := q.records[ref]
	if !ok || record.ObjectID != object || record.Role != role {
		return storage.Record{}, ErrSource
	}
	return record, nil
}

func queryFixture(t *testing.T, count int) *Repository {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	r := &Repository{ctx: ctx, cancel: cancel, gate: offline.NewWorkGate(ctx), ready: true, heads: map[string][]storage.ObjectVersion{}, cfg: Config{Scope: storage.Scope{Tenant: "tenant", Subject: "owner", Product: "drive"}, Crypto: &queryRecords{records: map[storage.Reference]storage.Record{}}}}
	q := r.cfg.Crypto.(*queryRecords)
	q.repo = r
	r.cfg.BindPublicationContext = func(ctx context.Context) (context.Context, error) {
		snapshot, err := storage.LoadAtomicCatalog(ctx, q, r.cfg.Scope)
		if err != nil {
			return nil, err
		}
		heads := map[string][]storage.ObjectVersion{}
		for _, version := range snapshot.Heads {
			heads[version.ObjectID] = append(heads[version.ObjectID], version)
		}
		if err := r.installObservation(ctx, snapshot, heads); err != nil {
			return nil, err
		}
		return ctx, nil
	}
	t.Cleanup(r.Close)
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	addQueryRow(t, r, "folder", "root", map[string]any{"id": "root", "name": "My drive", "is_root": true, "updated_at": now})
	for i := 1; i <= count; i++ {
		id := fmt.Sprintf("file_%03d", i)
		addQueryRow(t, r, "file", id, api.File{ID: id, FolderID: "root", Name: fmt.Sprintf("Reunión %03d.txt", i), Version: 1, UpdatedAt: now})
	}
	return r
}

func addQueryRow(t *testing.T, r *Repository, kind, id string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	item, err := summarize(storage.LegacySourceRow{Kind: kind, ID: id, Payload: raw}, "")
	if err != nil {
		t.Fatal(err)
	}
	payload := entry{Kind: entryKind, SourceKind: kind, SourceID: id, Raw: raw}
	if kind == "file" {
		payload.Binary = &bodyPointer{ObjectID: bodyID(kind, id, "binary"), Reference: storage.Reference{RoomID: "!private:test", EventID: "$binary_" + id}}
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	ref := storage.Reference{RoomID: "!private:test", EventID: "$content_" + id}
	object := objectID(kind, id)
	r.cfg.Crypto.(*queryRecords).records[ref] = storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: object, RevisionID: "r_" + id, Role: "content", Payload: bytes}
	metadata, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	state := "live"
	if item.DeletedAt != nil {
		state = "trash"
	}
	r.heads[object] = []storage.ObjectVersion{{ObjectID: object, RevisionID: "index_" + id, Reference: storage.Reference{RoomID: "!private:test", EventID: "$index_" + id}, Commit: storage.ObjectCommit{Kind: storage.ObjectKind, State: state, Metadata: metadata, Content: &storage.ContentPointer{Kind: "record", RevisionID: "r_" + id, Reference: ref}}}}
}

func TestPrivateQueriesFoldNamesAndPageTiedTimestampsWithoutDependingOnCurrentCursorItem(t *testing.T) {
	r := queryFixture(t, 101)
	found, err := r.Search(t.Context(), " REUNION ")
	if err != nil || len(found.Files) != queryPageSize || found.Files[0].ID != "file_101" || found.Files[0].Path != "/Reunión 101.txt" {
		t.Fatal("private search lost folding, path or ordering", found, err)
	}
	page, cursor, err := r.Recent(t.Context(), "")
	if err != nil || len(page) != 50 || cursor == "" || page[0].ID != "file_101" || page[49].ID != "file_052" {
		t.Fatal("first private page changed", len(page), cursor, err)
	}
	seen := map[string]bool{}
	for _, file := range page {
		seen[file.ID] = true
	}
	delete(r.heads, objectID("file", "file_052"))
	for _, wanted := range []int{50, 1} {
		page, cursor, err = r.Recent(t.Context(), cursor)
		if err != nil || len(page) != wanted {
			t.Fatal("private pagination skipped a timestamp tie", len(page), err)
		}
		for _, file := range page {
			if seen[file.ID] {
				t.Fatal("duplicate paged file", file.ID)
			}
			seen[file.ID] = true
		}
	}
	if cursor != "" || len(seen) != 101 {
		t.Fatal("private page did not terminate", cursor, len(seen))
	}
	if _, _, err := r.Recent(t.Context(), strings.Repeat("x", 4097)); !errors.Is(err, ErrSource) {
		t.Fatal("unbounded private cursor accepted", err)
	}
	value := r.encodeRecentCursor(summary{SourceID: "file_001", UpdatedAt: time.Now().UTC()})
	r.cfg.Scope.Subject = "other"
	if _, err := r.recentCursor(value); !errors.Is(err, ErrSource) {
		t.Fatal("cross-account cursor reached the private tree", err)
	}
	if _, _, err := r.Recent(t.Context(), value); !errors.Is(err, ErrSource) {
		t.Fatal("cross-account cursor accepted", err)
	}
}

func TestPrivateQueryTreeHidesInheritedTrashAndRejectsMissingCyclesAndConflicts(t *testing.T) {
	r := queryFixture(t, 0)
	now := time.Now().UTC()
	root := "root"
	parent := "parent"
	child := "child"
	addQueryRow(t, r, "folder", parent, api.Folder{ID: parent, ParentID: &root, Name: "Parent", UpdatedAt: now})
	addQueryRow(t, r, "folder", child, api.Folder{ID: child, ParentID: &parent, Name: "Child", UpdatedAt: now})
	addQueryRow(t, r, "file", "file", api.File{ID: "file", FolderID: child, Name: "Résumé.txt", Version: 1, UpdatedAt: now})
	found, err := r.Search(t.Context(), "resume")
	if err != nil || len(found.Files) != 1 || found.Files[0].Path != "/Parent/Child/Résumé.txt" {
		t.Fatal("nested private path changed", found, err)
	}
	addQueryRow(t, r, "folder", parent, api.Folder{ID: parent, ParentID: &root, Name: "Parent", UpdatedAt: now, DeletedAt: &now})
	found, err = r.Search(t.Context(), "resume")
	if err != nil || len(found.Files) != 0 {
		t.Fatal("search exposed inherited trash", found, err)
	}
	trash, err := r.Trash(t.Context())
	if err != nil || len(trash.Folders) != 1 || trash.Folders[0].ID != parent || len(trash.Files) != 0 || trash.RetentionSeconds != 0 {
		t.Fatal("trash lost top-level subtree", trash, err)
	}
	addQueryRow(t, r, "folder", parent, api.Folder{ID: parent, ParentID: &root, Name: "Parent", UpdatedAt: now})
	addQueryRow(t, r, "file", "file", api.File{ID: "file", FolderID: child, Name: "Résumé.txt", Version: 1, UpdatedAt: now, DeletedAt: &now})
	trash, err = r.Trash(t.Context())
	if err != nil || len(trash.Files) != 1 || trash.Files[0].Path != "/Parent/Child/Résumé.txt" {
		t.Fatal("independent trash did not reappear after parent restore", trash, err)
	}
	addQueryRow(t, r, "file", "file", api.File{ID: "file", FolderID: child, Name: "Résumé.txt", Version: 1, UpdatedAt: now})
	addQueryRow(t, r, "folder", parent, api.Folder{ID: parent, ParentID: &child, Name: "Parent", UpdatedAt: now})
	if _, err := r.Search(t.Context(), "resume"); !errors.Is(err, ErrCycle) {
		t.Fatal("private query traversed a folder cycle", err)
	}
	delete(r.heads, objectID("folder", parent))
	if _, err := r.Search(t.Context(), "resume"); !errors.Is(err, ErrSource) {
		t.Fatal("incomplete ancestry looked like no results", err)
	}
	addQueryRow(t, r, "folder", parent, api.Folder{ID: parent, ParentID: &root, Name: "Parent", UpdatedAt: now})
	index := r.heads[objectID("folder", child)][0]
	index.Reference.EventID += "_branch"
	r.heads[index.ObjectID] = append(r.heads[index.ObjectID], index)
	if _, err := r.Search(t.Context(), "resume"); !errors.Is(err, ErrConflict) {
		t.Fatal("private query chose a conflicting parent", err)
	}
}
