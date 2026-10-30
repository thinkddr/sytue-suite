// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package storage

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"
)

// Reference is public routing metadata. Object names, relationships and contents
// do not belong here; those live inside the SDK-encrypted application record.
type Reference struct {
	RoomID  string `json:"room_id"`
	EventID string `json:"event_id"`
}

func matrixID(value string, sigil byte) bool {
	return len(value) > 1 && len(value) <= 255 && value[0] == sigil && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n\t ")
}

func (r Reference) Valid() bool { return matrixID(r.RoomID, '!') && matrixID(r.EventID, '$') }

const ObjectKind = "org.sytue.object.v1"

var ErrIncompleteCatalog = errors.New("storage: object history is incomplete")

// ContentPointer binds a revision to authenticated content in the same object.
// Reusing a content revision permits a rename/trash operation without reuploading
// the media. File keys stay inside its separate encrypted manifest.
type ContentPointer struct {
	Kind       string    `json:"kind"`
	RevisionID string    `json:"revision_id"`
	Reference  Reference `json:"reference"`
}

func (p ContentPointer) Valid() bool {
	return (p.Kind == "record" || p.Kind == "file") && Identifier(p.RevisionID) && p.Reference.Valid()
}

// ObjectCommit is PRIVATE catalog data, carried only in Record role "index".
// Deleted is a logical tombstone, not physical erasure of retained revisions.
// Metadata is opaque JSON bytes so domain numbers survive every SDK roundtrip.
type ObjectCommit struct {
	Kind     string          `json:"kind"`
	State    string          `json:"state"`
	Metadata []byte          `json:"metadata"`
	Content  *ContentPointer `json:"content,omitempty"`
	Parents  []Reference     `json:"parents,omitempty"`
}

func (o ObjectCommit) Valid() bool {
	if o.Kind != ObjectKind || len(o.Metadata) > 8<<10 || !utf8.Valid(o.Metadata) || !json.Valid(o.Metadata) || len(o.Parents) > 16 {
		return false
	}
	switch o.State {
	case "live", "trash":
		if o.Content == nil || !o.Content.Valid() {
			return false
		}
	case "deleted":
		if o.Content != nil {
			return false
		}
	default:
		return false
	}
	seen := map[string]bool{}
	for _, parent := range o.Parents {
		if !parent.Valid() || seen[parent.EventID] {
			return false
		}
		seen[parent.EventID] = true
	}
	return true
}

type RecordEntry struct {
	Reference Reference `json:"reference"`
	Record    Record    `json:"record"`
}

type ObjectVersion struct {
	Reference  Reference    `json:"reference"`
	ObjectID   string       `json:"object_id"`
	RevisionID string       `json:"revision_id"`
	Commit     ObjectCommit `json:"commit"`
}

func DecodeObject(entry RecordEntry, scope Scope) (ObjectVersion, error) {
	rec := entry.Record
	var commit ObjectCommit
	if !entry.Reference.Valid() || rec.Validate(scope) != nil || rec.Role != "index" || json.Unmarshal(rec.Payload, &commit) != nil || !commit.Valid() || len(commit.Parents) != len(rec.Previous) {
		return ObjectVersion{}, ErrRecord
	}
	for i, parent := range commit.Parents {
		if parent == entry.Reference || parent.EventID != rec.Previous[i] {
			return ObjectVersion{}, ErrRecord
		}
	}
	return ObjectVersion{Reference: entry.Reference, ObjectID: rec.ObjectID, RevisionID: rec.RevisionID, Commit: commit}, nil
}

// CatalogHeads reduces a COMPLETE collection of authenticated index records.
// It implements no crypto or trust policy: callers obtain entries from the
// maintained SDK adapter. Missing history, foreign parents and cycles fail; an
// incomplete scan must never become a successful empty catalog. Concurrent heads
// remain explicit, including a deletion concurrent with an edit. Device clocks
// and server arrival order never choose a winning branch.
func CatalogHeads(scope Scope, entries []RecordEntry) ([]ObjectVersion, error) {
	if !scope.Valid() {
		return nil, ErrRecord
	}
	versions := make(map[Reference]ObjectVersion, len(entries))
	events := make(map[string]Reference, len(entries))
	for _, entry := range entries {
		version, err := DecodeObject(entry, scope)
		if err != nil {
			return nil, err
		}
		if _, exists := events[version.Reference.EventID]; exists {
			return nil, ErrRecord
		}
		events[version.Reference.EventID] = version.Reference
		versions[version.Reference] = version
	}
	children := make(map[Reference][]Reference, len(entries))
	degree := make(map[Reference]int, len(entries))
	queue := make([]Reference, 0, len(entries))
	for ref, version := range versions {
		degree[ref] = len(version.Commit.Parents)
		if degree[ref] == 0 {
			queue = append(queue, ref)
		}
		for _, parent := range version.Commit.Parents {
			prior, exists := versions[parent]
			if !exists {
				return nil, ErrIncompleteCatalog
			}
			if prior.ObjectID != version.ObjectID || prior.RevisionID == version.RevisionID {
				return nil, ErrRecord
			}
			children[parent] = append(children[parent], ref)
		}
	}
	// Iterative traversal keeps a long revision chain off the Go call stack.
	for cursor := 0; cursor < len(queue); cursor++ {
		for _, child := range children[queue[cursor]] {
			degree[child]--
			if degree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}
	if len(queue) != len(versions) {
		return nil, ErrRecord
	}
	heads := make([]ObjectVersion, 0)
	for ref, version := range versions {
		if len(children[ref]) == 0 {
			heads = append(heads, version)
		}
	}
	slices.SortFunc(heads, func(a, b ObjectVersion) int {
		if order := strings.Compare(a.ObjectID, b.ObjectID); order != 0 {
			return order
		}
		return strings.Compare(a.Reference.EventID, b.Reference.EventID)
	})
	return heads, nil
}
