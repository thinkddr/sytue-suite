// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"bytes"
	"slices"
)

// AcceptedObject locates one exact index in an already complete authenticated
// catalog. Cleanup acknowledges that revision even if a later edit is now head.
// It never selects a branch or authorizes publishing from an old device.
// A nil reference allows recovery of a lost publication ACK by unique revision.
func AcceptedObject(snapshot CatalogSnapshot, scope Scope, object, revision string, reference *Reference) (ObjectVersion, bool, error) {
	if !snapshot.Binding.Valid() || snapshot.Binding.Scope != scope || object == "" || reference == nil && revision == "" {
		return ObjectVersion{}, false, ErrCatalogSource
	}
	var accepted ObjectVersion
	found := false
	for key, entry := range snapshot.Entries {
		if reference != nil && key != *reference {
			continue
		}
		version, err := DecodeObject(entry, scope)
		if err != nil || key != entry.Reference || !slices.Contains(snapshot.Binding.Rooms, key.RoomID) {
			return ObjectVersion{}, false, ErrCatalogSource
		}
		if version.ObjectID != object || revision != "" && version.RevisionID != revision {
			if reference != nil {
				return ObjectVersion{}, false, ErrCatalogSource
			}
			continue
		}
		if found {
			return ObjectVersion{}, false, ErrCatalogSource
		}
		accepted, found = version, true
	}
	if reference != nil && !found {
		return ObjectVersion{}, false, ErrCatalogSource
	}
	return accepted, found, nil
}

// AcceptedAtomicObject locates an exact committed revision in the validated
// History returned by LoadAtomicCatalog. The caller must retain that snapshot
// intact: its exported fields are mutable and this lookup is not an SDK proof.
// Orphan prepared indexes and roots do not occur in its committed History.
// Like AcceptedObject, it grants neither publication authority nor permission
// to delete anything. Repeated lookups are linear in the retained history.
func AcceptedAtomicObject(snapshot AtomicCatalogSnapshot, scope Scope, object, revision string, reference *Reference) (ObjectVersion, bool, error) {
	if !snapshot.Binding.Valid() || snapshot.Binding.Scope != scope || object == "" || reference == nil && revision == "" {
		return ObjectVersion{}, false, ErrCatalogSource
	}
	var accepted ObjectVersion
	found := false
	for _, version := range snapshot.History {
		if reference != nil && version.Reference != *reference {
			continue
		}
		if err := verifyAcceptedAtomicHistoryEntry(snapshot, scope, version); err != nil {
			return ObjectVersion{}, false, err
		}
		if version.ObjectID != object || revision != "" && version.RevisionID != revision {
			if reference != nil {
				return ObjectVersion{}, false, ErrCatalogSource
			}
			continue
		}
		if found {
			return ObjectVersion{}, false, ErrCatalogSource
		}
		accepted, found = version, true
	}
	if reference != nil && !found {
		return ObjectVersion{}, false, ErrCatalogSource
	}
	if found {
		accepted = cloneAcceptedObjectVersion(accepted)
	}
	return accepted, found, nil
}

func verifyAcceptedAtomicHistoryEntry(snapshot AtomicCatalogSnapshot, scope Scope, version ObjectVersion) error {
	if !version.Reference.Valid() || !slices.Contains(snapshot.Binding.Rooms, version.Reference.RoomID) || !Identifier(version.ObjectID) || !Identifier(version.RevisionID) || !version.Commit.Valid() {
		return ErrCatalogSource
	}
	entry, ok := snapshot.Entries[version.Reference]
	if !ok || entry.Reference != version.Reference {
		return ErrCatalogSource
	}
	kind, err := atomicKind(entry, scope)
	if err != nil {
		return ErrCatalogSource
	}
	var decoded ObjectVersion
	switch kind {
	case ObjectKind:
		decoded, err = DecodeObject(entry, scope)
	case ObjectPreparedKind:
		_, decoded, err = DecodeAtomicPrepared(entry, scope)
	default:
		return ErrCatalogSource
	}
	if err != nil || decoded.Reference != version.Reference || decoded.ObjectID != version.ObjectID || decoded.RevisionID != version.RevisionID || !decoded.Commit.MatchesExact(version.Commit) {
		return ErrCatalogSource
	}
	return nil
}

func cloneAcceptedObjectVersion(accepted ObjectVersion) ObjectVersion {
	accepted.Commit.Metadata = slices.Clone(accepted.Commit.Metadata)
	accepted.Commit.Parents = slices.Clone(accepted.Commit.Parents)
	if accepted.Commit.Content != nil {
		content := *accepted.Commit.Content
		accepted.Commit.Content = &content
	}
	return accepted
}

// SameAtomicCatalog compares two complete, unmodified LoadAtomicCatalog
// observations, including the raw index envelopes and manifest pages. It is
// an equality check, not SDK provenance, a distributed fence, or deletion
// authority. Binding.Same deliberately ignores harmless room ordering.
func SameAtomicCatalog(a, b AtomicCatalogSnapshot) bool {
	if !a.Binding.Same(b.Binding) || !sameAtomicEntries(a.Entries, b.Entries) || !sameAtomicEntries(a.Pages, b.Pages) || len(a.Batches) != len(b.Batches) || !sameAtomicVersions(a.History, b.History) || !sameAtomicVersions(a.Heads, b.Heads) {
		return false
	}
	for id, ref := range a.Batches {
		if other, ok := b.Batches[id]; !ok || ref != other {
			return false
		}
	}
	return true
}

func sameAtomicEntries(a, b map[Reference]RecordEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for ref, entry := range a {
		other, ok := b[ref]
		if !ok || entry.Reference != ref || !sameRecordEntry(entry, other) {
			return false
		}
	}
	return true
}

// sameRecordEntry dice si a y b son el mismo registro en la misma referencia.
func sameRecordEntry(a, b RecordEntry) bool {
	x, y := a.Record, b.Record
	return a.Reference == b.Reference && x.Version == y.Version && x.Scope == y.Scope && x.ObjectID == y.ObjectID && x.RevisionID == y.RevisionID && x.Role == y.Role && slices.Equal(x.Previous, y.Previous) && bytes.Equal(x.Payload, y.Payload)
}

func sameAtomicVersions(a, b []ObjectVersion) bool {
	if len(a) != len(b) {
		return false
	}
	for i, version := range a {
		other := b[i]
		if version.Reference != other.Reference || version.ObjectID != other.ObjectID || version.RevisionID != other.RevisionID || !version.Commit.MatchesExact(other.Commit) {
			return false
		}
	}
	return true
}
