// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"slices"
	"testing"
)

func TestAcceptedObjectUsesRetainedRevisionAcrossNewHeadAndDevice(t *testing.T) {
	snapshot, err := LoadCatalog(t.Context(), catalogPortFixture(), catalogScope)
	if err != nil {
		t.Fatal(err)
	}
	original := Reference{RoomID: "!private:test", EventID: "$root"}
	entry := snapshot.Entries[original]
	snapshot.Binding.Target.DeviceID = "FRESH"
	for _, reference := range []*Reference{nil, &original} {
		version, found, err := AcceptedObject(snapshot, catalogScope, entry.Record.ObjectID, entry.Record.RevisionID, reference)
		if err != nil || !found || version.Reference != original {
			t.Fatal("accepted ancestor was lost behind current head", err)
		}
	}
}

func TestAcceptedObjectRejectsForgedReferenceWrongScopeAndDuplicateRevision(t *testing.T) {
	for _, fault := range []string{"scope", "missing-reference", "foreign-room", "wrong-object", "wrong-revision", "duplicate-revision"} {
		t.Run(fault, func(t *testing.T) {
			snapshot, err := LoadCatalog(t.Context(), catalogPortFixture(), catalogScope)
			if err != nil {
				t.Fatal(err)
			}
			scope := catalogScope
			ref := Reference{RoomID: "!private:test", EventID: "$root"}
			entry := snapshot.Entries[ref]
			object, revision := entry.Record.ObjectID, entry.Record.RevisionID
			reference := &ref
			switch fault {
			case "scope":
				scope.Product = "mail"
			case "missing-reference":
				ref.EventID = "$missing"
			case "foreign-room":
				snapshot.Binding.Rooms = []string{"!retained:test"}
				snapshot.Binding.Target.RoomID = "!retained:test"
			case "wrong-object":
				object = "different"
			case "wrong-revision":
				revision = "different"
			case "duplicate-revision":
				reference = nil
				cloned := entry
				cloned.Reference.EventID = "$duplicate"
				snapshot.Entries[cloned.Reference] = cloned
			}
			if _, found, err := AcceptedObject(snapshot, scope, object, revision, reference); err == nil || found {
				t.Fatal("forged accepted revision allowed cleanup", fault)
			}
		})
	}
}

func TestAcceptedAtomicObjectUsesOnlyCommittedHistory(t *testing.T) {
	orphan, err := LoadAtomicCatalog(t.Context(), atomicTestPort(t, 1, false), catalogScope)
	if err != nil {
		t.Fatal(err)
	}
	prepared := Reference{RoomID: "!private:test", EventID: "$prepared0"}
	if _, found, err := AcceptedAtomicObject(orphan, catalogScope, "object0", "r1", &prepared); err == nil || found {
		t.Fatal("orphan prepared index was treated as accepted")
	}
	if _, found, err := AcceptedAtomicObject(orphan, catalogScope, "object0", "r1", nil); err != nil || found {
		t.Fatal("orphan revision was visible", found, err)
	}
	committed, err := LoadAtomicCatalog(t.Context(), atomicTestPort(t, 1, true), catalogScope)
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range []*Reference{nil, &prepared} {
		version, found, err := AcceptedAtomicObject(committed, catalogScope, "object0", "r1", reference)
		if err != nil || !found || version.Reference != prepared || version.Commit.State != "deleted" {
			t.Fatal("committed member was not found", found, err)
		}
	}
	old := Reference{RoomID: "!private:test", EventID: "$old0"}
	if version, found, err := AcceptedAtomicObject(committed, catalogScope, "object0", "r0", &old); err != nil || !found || version.Reference != old {
		t.Fatal("historical revision was lost behind batch head", found, err)
	}
	root := Reference{RoomID: "!private:test", EventID: "$commit"}
	if _, found, err := AcceptedAtomicObject(committed, catalogScope, AtomicBatchObjectID("batch"), "commit", &root); err == nil || found {
		t.Fatal("batch root was treated as a domain revision")
	}
}

func TestAcceptedAtomicObjectRejectsMismatchAndReturnsOwnedData(t *testing.T) {
	snapshot, err := LoadAtomicCatalog(t.Context(), atomicTestPort(t, 1, true), catalogScope)
	if err != nil {
		t.Fatal(err)
	}
	ref := Reference{RoomID: "!private:test", EventID: "$prepared0"}
	for _, fault := range []string{"scope", "missing", "wrong-object", "wrong-revision", "duplicate"} {
		t.Run(fault, func(t *testing.T) {
			cloned := snapshot
			cloned.History = slices.Clone(snapshot.History)
			scope, object, revision, reference := catalogScope, "object0", "r1", &ref
			switch fault {
			case "scope":
				scope.Product = "mail"
			case "missing":
				missing := Reference{RoomID: ref.RoomID, EventID: "$missing"}
				reference = &missing
			case "wrong-object":
				object = "other"
			case "wrong-revision":
				revision = "other"
			case "duplicate":
				reference = nil
				for _, version := range snapshot.History {
					if version.Reference == ref {
						cloned.History = append(cloned.History, version)
					}
				}
			}
			if _, found, err := AcceptedAtomicObject(cloned, scope, object, revision, reference); err == nil || found {
				t.Fatal("invalid accepted revision was returned", fault)
			}
		})
	}
	version, found, err := AcceptedAtomicObject(snapshot, catalogScope, "object0", "r1", &ref)
	if err != nil || !found || len(version.Commit.Metadata) == 0 || len(version.Commit.Parents) == 0 {
		t.Fatal("missing test revision", err)
	}
	metadata, parent := version.Commit.Metadata[0], version.Commit.Parents[0]
	version.Commit.Metadata[0] ^= 1
	version.Commit.Parents[0].EventID = "$changed"
	again, found, err := AcceptedAtomicObject(snapshot, catalogScope, "object0", "r1", &ref)
	if err != nil || !found || again.Commit.Metadata[0] != metadata || again.Commit.Parents[0] != parent {
		t.Fatal("returned revision mutated retained snapshot", err)
	}
}

func TestSameAtomicCatalogIncludesRawPagesAndCommittedHistory(t *testing.T) {
	for _, fault := range []string{"none", "binding", "entry", "page", "history", "heads", "batches"} {
		t.Run(fault, func(t *testing.T) {
			before, err := LoadAtomicCatalog(t.Context(), atomicTestPort(t, 1, true), catalogScope)
			if err != nil {
				t.Fatal(err)
			}
			after, err := LoadAtomicCatalog(t.Context(), atomicTestPort(t, 1, true), catalogScope)
			if err != nil {
				t.Fatal(err)
			}
			switch fault {
			case "binding":
				after.Binding.Target.DeviceID = "OTHER"
			case "entry":
				ref := Reference{RoomID: "!private:test", EventID: "$prepared0"}
				entry := after.Entries[ref]
				entry.Record.Previous = []string{"$changed"}
				after.Entries[ref] = entry
			case "page":
				for ref, entry := range after.Pages {
					entry.Record.Payload[0] ^= 1
					after.Pages[ref] = entry
					break
				}
			case "history":
				after.History[0].RevisionID = "changed"
			case "heads":
				after.Heads = nil
			case "batches":
				delete(after.Batches, "batch")
			}
			if got := SameAtomicCatalog(before, after); got != (fault == "none") {
				t.Fatal("incorrect atomic observation equality", fault, got)
			}
		})
	}
}
