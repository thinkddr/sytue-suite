// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"bytes"
	"reflect"
	"slices"
	"testing"
)

func TestCatalogHistoryRetainsEveryBranchTombstoneAndExactMetadata(t *testing.T) {
	base := catalogEntry("base", "base", "live")
	left := catalogEntry("left", "left", "live", base.Reference)
	right := catalogEntry("right", "right", "trash", base.Reference)
	deleted := catalogEntry("deleted", "deleted", "deleted", left.Reference)
	entries := []RecordEntry{deleted, right, base, left}
	history, err := CatalogHistory(catalogScope, entries)
	if err != nil || len(history) != len(entries) {
		t.Fatal("complete history lost revisions", err)
	}
	position := map[Reference]int{}
	for i, version := range history {
		position[version.Reference] = i
		if !bytes.Equal(version.Commit.Metadata, []byte(`{"name":"private.pdf","integer":9007199254740993}`)) {
			t.Fatal("history normalized exact private metadata")
		}
	}
	for _, version := range history {
		for _, parent := range version.Commit.Parents {
			if position[parent] >= position[version.Reference] {
				t.Fatal("child preceded its retained parent")
			}
		}
	}
	if history[len(history)-1].Commit.State != "deleted" {
		t.Fatal("tombstone dropped or replaced by a live branch")
	}
	slices.Reverse(entries)
	reordered, err := CatalogHistory(catalogScope, entries)
	if err != nil || !reflect.DeepEqual(history, reordered) {
		t.Fatal("server arrival order changed migration traversal", err)
	}
	heads, err := CatalogHeads(catalogScope, entries)
	if err != nil || len(heads) != 2 {
		t.Fatal("history traversal changed conflict reduction", err)
	}
}

func TestCatalogHistoryRejectsMissingParentsWithoutReturningPartialExport(t *testing.T) {
	base := catalogEntry("base", "base", "live")
	child := catalogEntry("child", "child", "trash", base.Reference)
	history, err := CatalogHistory(catalogScope, []RecordEntry{child})
	if err == nil || history != nil {
		t.Fatal("incomplete catalog yielded an exportable prefix")
	}
}
