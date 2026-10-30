// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var catalogScope = Scope{Tenant: "acme", Subject: "alice", Product: "drive"}

func catalogEntry(id, revision, state string, parents ...Reference) RecordEntry {
	commit := ObjectCommit{Kind: ObjectKind, State: state, Metadata: []byte(`{"name":"private.pdf","integer":9007199254740993}`), Parents: parents}
	if state != "deleted" {
		commit.Content = &ContentPointer{Kind: "record", RevisionID: "content-revision", Reference: Reference{RoomID: "!private:test", EventID: "$content"}}
	}
	payload, _ := json.Marshal(commit)
	previous := make([]string, len(parents))
	for i := range parents {
		previous[i] = parents[i].EventID
	}
	return RecordEntry{Reference: Reference{RoomID: "!private:test", EventID: "$" + id}, Record: Record{Version: 1, Scope: catalogScope, ObjectID: "object", RevisionID: revision, Role: "index", Previous: previous, Payload: payload}}
}

func TestCatalogKeepsConcurrentEditAndDeleteUntilExplicitMerge(t *testing.T) {
	root := catalogEntry("root", "r0", "live")
	left := catalogEntry("left", "r1", "live", root.Reference)
	right := catalogEntry("right", "r2", "deleted", root.Reference)
	entries := make([]RecordEntry, 0, 5)
	entries = append(entries, root, right, left)
	heads, err := CatalogHeads(catalogScope, entries)
	if err != nil || len(heads) != 2 || heads[0].Reference != left.Reference || heads[1].Reference != right.Reference {
		t.Fatal("concurrent deletion silently won over an edit", heads, err)
	}
	slices.Reverse(entries)
	reordered, err := CatalogHeads(catalogScope, entries)
	if err != nil || !reflect.DeepEqual(heads, reordered) {
		t.Fatal("server arrival order chose a winning branch", err)
	}
	merged := catalogEntry("merged", "r3", "live", left.Reference, right.Reference)
	entries = append(entries, merged)
	trash := catalogEntry("trash", "r4", "trash", merged.Reference)
	entries = append(entries, trash)
	heads, err = CatalogHeads(catalogScope, entries)
	if err != nil || len(heads) != 1 || heads[0].Reference != trash.Reference || heads[0].Commit.State != "trash" || !bytes.Equal(heads[0].Commit.Metadata, []byte(`{"name":"private.pdf","integer":9007199254740993}`)) {
		t.Fatal("explicit merge/trash lost its exact private metadata", err)
	}
}

func TestCatalogIncompleteAndHostileHistoryNeverBecomesAnEmptySuccess(t *testing.T) {
	root := catalogEntry("root", "r0", "live")
	child := catalogEntry("child", "r1", "live", root.Reference)
	if heads, err := CatalogHeads(catalogScope, []RecordEntry{child}); !errors.Is(err, ErrIncompleteCatalog) || heads != nil {
		t.Fatal("missing page became a successful catalog", err)
	}
	foreign := root
	foreign.Record.ObjectID = "another-object"
	other := root
	other.Record.Scope.Product = "mail"
	cycleA := catalogEntry("a", "ra", "live", Reference{RoomID: "!private:test", EventID: "$b"})
	cycleB := catalogEntry("b", "rb", "live", cycleA.Reference)
	mismatch := child
	mismatch.Record.Previous = []string{"$different"}
	for i, history := range [][]RecordEntry{
		{foreign, child},
		{other, child},
		{root, root},
		{cycleA, cycleB},
		{root, mismatch},
		{root, catalogEntry("self", "r1", "live", Reference{RoomID: "!private:test", EventID: "$self"})},
		{root, catalogEntry("duplicate", "r1", "live", root.Reference, root.Reference)},
		{root, catalogEntry("same-revision", "r0", "live", root.Reference)},
	} {
		if heads, err := CatalogHeads(catalogScope, history); err == nil || heads != nil {
			t.Fatal("hostile catalog history was accepted", "case", i)
		}
	}
	if _, err := CatalogHeads(Scope{}, nil); err == nil {
		t.Fatal("invalid empty scope became an empty success")
	}
	for _, ref := range []Reference{{RoomID: "!bad room", EventID: "$ok"}, {RoomID: "!ok", EventID: "$bad\x00"}, {RoomID: "!ok", EventID: "$" + string([]byte{0xff})}} {
		if ref.Valid() {
			t.Fatal("ambiguous routing reference accepted")
		}
	}
}

func TestCatalogLongRevisionChainUsesBoundedCallStack(t *testing.T) {
	entries := make([]RecordEntry, 1, 10000)
	entries[0] = catalogEntry("0", "r0", "live")
	for i := 1; i < 10000; i++ {
		entries = append(entries, catalogEntry(strconv.Itoa(i), "r"+strconv.Itoa(i), "live", entries[i-1].Reference))
	}
	heads, err := CatalogHeads(catalogScope, entries)
	if err != nil || len(heads) != 1 || heads[0].Reference != entries[len(entries)-1].Reference {
		t.Fatal("long revision chain lost its head", err)
	}
}

func TestCatalogMaximumFieldsFitPrivateRecordWithoutTruncation(t *testing.T) {
	scope := Scope{Tenant: strings.Repeat("t", 63), Subject: strings.Repeat("s", 255), Product: "calendar"}
	entry := catalogEntry("head", strings.Repeat("r", 128), "live")
	entry.Record.Scope = scope
	entry.Record.ObjectID = strings.Repeat("o", 128)
	var commit ObjectCommit
	if err := json.Unmarshal(entry.Record.Payload, &commit); err != nil {
		t.Fatal(err)
	}
	commit.Metadata = []byte(`"` + strings.Repeat("m", (8<<10)-2) + `"`)
	commit.Content.RevisionID = strings.Repeat("c", 128)
	commit.Content.Reference = Reference{RoomID: "!" + strings.Repeat("r", 254), EventID: "$" + strings.Repeat("c", 254)}
	for i := range 16 {
		parent := Reference{RoomID: "!" + strings.Repeat("p", 254), EventID: "$" + string(rune('a'+i)) + strings.Repeat("p", 253)}
		commit.Parents = append(commit.Parents, parent)
		entry.Record.Previous = append(entry.Record.Previous, parent.EventID)
	}
	entry.Record.Payload, _ = json.Marshal(commit)
	if _, err := DecodeObject(entry, scope); err != nil {
		t.Fatal("legitimate maximum fields exceeded the encrypted record bound", err)
	}
	commit.Metadata = []byte{'"', 0xff, '"'}
	if commit.Valid() {
		t.Fatal("private metadata would be silently normalized by a JSON parser")
	}
}
