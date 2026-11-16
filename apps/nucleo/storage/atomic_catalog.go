// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"slices"
)

const (
	ObjectPreparedKind  = "org.sytue.object.prepared.v1"
	ObjectBatchKind     = "org.sytue.object.batch.v1"
	ObjectBatchPageKind = "org.sytue.object.batch-page.v1"
)

// AtomicPrepared is private application data. A batch name grants no authority:
// the surrounding Record must come from the caller's maintained scoped SDK.
// Old ObjectKind readers reject this envelope instead of exposing a partial batch.
type AtomicPrepared struct {
	Kind    string       `json:"kind"`
	BatchID string       `json:"batch_id"`
	Ordinal uint64       `json:"ordinal"`
	Commit  ObjectCommit `json:"commit"`
}

type AtomicPagePointer struct {
	Reference  Reference `json:"reference"`
	RevisionID string    `json:"revision_id"`
	SHA256     string    `json:"sha256"`
}

func validAtomicHash(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size && hex.EncodeToString(raw) == value
}

func (p AtomicPagePointer) valid() bool {
	return p.Reference.Valid() && Identifier(p.RevisionID) && validAtomicHash(p.SHA256)
}

type AtomicMember struct {
	Reference  Reference `json:"reference"`
	ObjectID   string    `json:"object_id"`
	RevisionID string    `json:"revision_id"`
	SHA256     string    `json:"sha256"`
}

// Pages form a backwards hash-pinned chain. Record's existing size bound limits
// each page, rather than imposing a limit on the number of objects in a batch.
type AtomicBatchPage struct {
	Kind     string             `json:"kind"`
	BatchID  string             `json:"batch_id"`
	Start    uint64             `json:"start"`
	Members  []AtomicMember     `json:"members"`
	Previous *AtomicPagePointer `json:"previous,omitempty"`
}

type AtomicBatchCommit struct {
	Kind    string            `json:"kind"`
	BatchID string            `json:"batch_id"`
	Count   uint64            `json:"count"`
	Last    AtomicPagePointer `json:"last"`
}

func AtomicBatchObjectID(batchID string) string {
	sum := sha256.Sum256([]byte(batchID))
	return "storage_batch_" + hex.EncodeToString(sum[:])
}

// AtomicRecordSHA256 is only an exact logical-record checksum, never SDK
// provenance, publication permission, a cipher, or a replacement for readback.
func AtomicRecordSHA256(record Record) (string, error) {
	if record.Validate(record.Scope) != nil {
		return "", ErrRecord
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	defer clear(raw)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func decodeAtomicJSON(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return ErrRecord
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ErrRecord
	}
	return nil
}

func atomicKind(entry RecordEntry, scope Scope) (string, error) {
	if !entry.Reference.Valid() || entry.Record.Validate(scope) != nil || entry.Record.Role != "index" {
		return "", ErrRecord
	}
	var tag struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(entry.Record.Payload, &tag) != nil {
		return "", ErrRecord
	}
	return tag.Kind, nil
}

func DecodeAtomicPrepared(entry RecordEntry, scope Scope) (AtomicPrepared, ObjectVersion, error) {
	var value AtomicPrepared
	if _, err := atomicKind(entry, scope); err != nil {
		return value, ObjectVersion{}, err
	}
	if decodeAtomicJSON(entry.Record.Payload, &value) != nil || value.Kind != ObjectPreparedKind || !Identifier(value.BatchID) || !value.Commit.Valid() {
		return value, ObjectVersion{}, ErrRecord
	}
	canonical := entry
	canonical.Record.Payload, _ = json.Marshal(value.Commit)
	version, err := DecodeObject(canonical, scope)
	return value, version, err
}

func DecodeAtomicBatch(entry RecordEntry, scope Scope) (AtomicBatchCommit, error) {
	var value AtomicBatchCommit
	if _, err := atomicKind(entry, scope); err != nil {
		return value, err
	}
	if decodeAtomicJSON(entry.Record.Payload, &value) != nil || value.Kind != ObjectBatchKind || !Identifier(value.BatchID) || value.Count == 0 || !value.Last.valid() || entry.Record.ObjectID != AtomicBatchObjectID(value.BatchID) || len(entry.Record.Previous) != 0 || value.Last.Reference.RoomID != entry.Reference.RoomID {
		return value, ErrRecord
	}
	return value, nil
}

// ValidateCatalogEntry validates the transport's complete index inventory. It
// does not make prepared entries visible; only LoadAtomicCatalog may do that.
func ValidateCatalogEntry(entry RecordEntry, scope Scope) error {
	kind, err := atomicKind(entry, scope)
	if err != nil {
		return err
	}
	switch kind {
	case ObjectKind:
		_, err = DecodeObject(entry, scope)
	case ObjectPreparedKind:
		_, _, err = DecodeAtomicPrepared(entry, scope)
	case ObjectBatchKind:
		_, err = DecodeAtomicBatch(entry, scope)
	default:
		err = ErrRecord
	}
	return err
}

type AtomicCatalogReader interface {
	CatalogReader
	ReadStorageRecord(context.Context, Reference, string, string) (Record, error)
}

// Entries preserves ALL exact index envelopes, including orphan prepared work
// and commit roots. Pages preserves every authenticated manifest read. History
// and Heads are the all-or-nothing domain projection; no root becomes an object.
// Migration must remap Entries AND Pages, never reserialize only the visible heads.
type AtomicCatalogSnapshot struct {
	Binding Binding
	Entries map[Reference]RecordEntry
	Pages   map[Reference]RecordEntry
	History []ObjectVersion
	Heads   []ObjectVersion
	Batches map[string]Reference
}

func LoadAtomicCatalog(ctx context.Context, reader AtomicCatalogReader, scope Scope) (AtomicCatalogSnapshot, error) {
	binding, entries, err := loadCatalogEntries(ctx, reader, scope)
	if err != nil {
		return AtomicCatalogSnapshot{}, err
	}
	snapshot, err := projectAtomicCatalog(ctx, reader, binding, entries)
	if err != nil {
		return AtomicCatalogSnapshot{}, err
	}
	current, err := reader.StorageBindings(ctx)
	if err != nil {
		return AtomicCatalogSnapshot{}, err
	}
	if !binding.Same(current) {
		return AtomicCatalogSnapshot{}, ErrCatalogSource
	}
	if err := ctx.Err(); err != nil {
		return AtomicCatalogSnapshot{}, err
	}
	return snapshot, nil
}

type atomicPreparedEntry struct {
	value   AtomicPrepared
	version ObjectVersion
	entry   RecordEntry
}

func projectAtomicCatalog(ctx context.Context, reader AtomicCatalogReader, binding Binding, entries map[Reference]RecordEntry) (AtomicCatalogSnapshot, error) {
	snapshot := AtomicCatalogSnapshot{Binding: binding, Entries: entries, Pages: map[Reference]RecordEntry{}, Batches: map[string]Reference{}}
	visible := []RecordEntry{}
	prepared := map[Reference]atomicPreparedEntry{}
	ordinals := map[string]map[uint64]Reference{}
	objects := map[string]map[string]bool{}
	roots := map[Reference]AtomicBatchCommit{}
	events := map[string]bool{}
	for ref, entry := range entries {
		if err := ctx.Err(); err != nil {
			return AtomicCatalogSnapshot{}, err
		}
		if ref != entry.Reference || events[ref.EventID] {
			return AtomicCatalogSnapshot{}, ErrRecord
		}
		events[ref.EventID] = true
		kind, err := atomicKind(entry, binding.Scope)
		if err != nil {
			return AtomicCatalogSnapshot{}, err
		}
		switch kind {
		case ObjectKind:
			if _, err := DecodeObject(entry, binding.Scope); err != nil {
				return AtomicCatalogSnapshot{}, err
			}
			visible = append(visible, entry)
		case ObjectPreparedKind:
			if err := recordPreparedCatalogEntry(entry, binding.Scope, ref, ordinals, objects, prepared); err != nil {
				return AtomicCatalogSnapshot{}, err
			}
		case ObjectBatchKind:
			root, err := DecodeAtomicBatch(entry, binding.Scope)
			if err != nil {
				return AtomicCatalogSnapshot{}, err
			}
			if _, duplicate := snapshot.Batches[root.BatchID]; duplicate {
				return AtomicCatalogSnapshot{}, ErrRecord
			}
			snapshot.Batches[root.BatchID] = ref
			roots[ref] = root
		default:
			return AtomicCatalogSnapshot{}, ErrRecord
		}
	}
	rootRefs := make([]Reference, 0, len(roots))
	for ref := range roots {
		rootRefs = append(rootRefs, ref)
	}
	slices.SortFunc(rootRefs, compareReferences)
	for _, ref := range rootRefs {
		root := roots[ref]
		if root.Count != uint64(len(ordinals[root.BatchID])) {
			return AtomicCatalogSnapshot{}, ErrIncompleteCatalog
		}
		members, err := snapshot.loadAtomicBatch(ctx, reader, ref, root, prepared)
		if err != nil {
			return AtomicCatalogSnapshot{}, err
		}
		visible = append(visible, members...)
	}
	history, err := CatalogHistory(binding.Scope, visible)
	if err != nil {
		return AtomicCatalogSnapshot{}, err
	}
	heads, err := CatalogHeads(binding.Scope, visible)
	if err != nil {
		return AtomicCatalogSnapshot{}, err
	}
	snapshot.History, snapshot.Heads = history, heads
	return snapshot, nil
}

func recordPreparedCatalogEntry(entry RecordEntry, scope Scope, ref Reference, ordinals map[string]map[uint64]Reference, objects map[string]map[string]bool, prepared map[Reference]atomicPreparedEntry) error {
	value, version, err := DecodeAtomicPrepared(entry, scope)
	if err != nil {
		return err
	}
	if ordinals[value.BatchID] == nil {
		ordinals[value.BatchID] = map[uint64]Reference{}
		objects[value.BatchID] = map[string]bool{}
	}
	if _, duplicate := ordinals[value.BatchID][value.Ordinal]; duplicate || objects[value.BatchID][version.ObjectID] {
		return ErrRecord
	}
	ordinals[value.BatchID][value.Ordinal] = ref
	objects[value.BatchID][version.ObjectID] = true
	prepared[ref] = atomicPreparedEntry{value, version, entry}
	return nil
}

//nolint:gocyclo // sequential fail-closed verification steps of security-critical code; splitting them would scatter the check order
func (s *AtomicCatalogSnapshot) loadAtomicBatch(ctx context.Context, reader AtomicCatalogReader, rootRef Reference, root AtomicBatchCommit, prepared map[Reference]atomicPreparedEntry) ([]RecordEntry, error) {
	end := root.Count
	pointer := root.Last
	seen := map[Reference]bool{}
	out := []RecordEntry{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !pointer.valid() || pointer.Reference.RoomID != rootRef.RoomID || seen[pointer.Reference] {
			return nil, ErrRecord
		}
		seen[pointer.Reference] = true
		record, err := reader.ReadStorageRecord(ctx, pointer.Reference, AtomicBatchObjectID(root.BatchID), "manifest")
		if err != nil {
			return nil, err
		}
		actual, err := AtomicRecordSHA256(record)
		if err != nil || record.Validate(s.Binding.Scope) != nil || record.ObjectID != AtomicBatchObjectID(root.BatchID) || record.RevisionID != pointer.RevisionID || record.Role != "manifest" || len(record.Previous) != 0 || actual != pointer.SHA256 {
			return nil, ErrRecord
		}
		var page AtomicBatchPage
		if decodeAtomicJSON(record.Payload, &page) != nil || page.Kind != ObjectBatchPageKind || page.BatchID != root.BatchID || len(page.Members) == 0 || page.Start >= end || uint64(len(page.Members)) != end-page.Start {
			return nil, ErrRecord
		}
		pageEntries, err := decodeAtomicBatchPageMembers(rootRef, root, page, prepared)
		if err != nil {
			return nil, err
		}
		out = append(out, pageEntries...)
		if _, duplicate := s.Pages[pointer.Reference]; duplicate {
			return nil, ErrRecord
		}
		s.Pages[pointer.Reference] = cloneCatalogEntry(RecordEntry{Reference: pointer.Reference, Record: record})
		if page.Start == 0 {
			if page.Previous != nil {
				return nil, ErrRecord
			}
			break
		}
		if page.Previous == nil {
			return nil, ErrIncompleteCatalog
		}
		end = page.Start
		pointer = *page.Previous
	}
	return out, nil
}

func decodeAtomicBatchPageMembers(rootRef Reference, root AtomicBatchCommit, page AtomicBatchPage, prepared map[Reference]atomicPreparedEntry) ([]RecordEntry, error) {
	out := make([]RecordEntry, 0, len(page.Members))
	for i, member := range page.Members {
		candidate, found := prepared[member.Reference]
		if !found {
			return nil, ErrIncompleteCatalog
		}
		sum, err := AtomicRecordSHA256(candidate.entry.Record)
		if err != nil || member.Reference.RoomID != rootRef.RoomID || candidate.value.BatchID != root.BatchID || candidate.value.Ordinal != page.Start+uint64(i) || candidate.version.ObjectID != member.ObjectID || candidate.version.RevisionID != member.RevisionID || sum != member.SHA256 {
			return nil, ErrRecord
		}
		canonical := candidate.entry
		canonical.Record.Payload, _ = json.Marshal(candidate.value.Commit)
		out = append(out, canonical)
	}
	return out, nil
}
