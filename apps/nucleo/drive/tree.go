// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"encoding/json"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type resource struct {
	item  summary
	index storage.ObjectVersion
}

// A query captures the authenticated projection once. Resolving paths never
// fetches one folder per ancestor, and a concurrent refresh cannot mix trees.
type privateTree struct {
	nodes     []resource
	marks     []resource
	folders   map[string][]resource
	conflicts map[string]bool
}

func (r *Repository) resourceTree(ctx context.Context) (privateTree, error) {
	view, err := r.currentObservation(ctx)
	if err != nil {
		return privateTree{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return privateTree{}, ErrClosed
	}
	if !r.ready {
		return privateTree{}, ErrNotReady
	}
	tree := privateTree{folders: map[string][]resource{}, conflicts: map[string]bool{}}
	roots := 0
	byObject := map[string][]storage.ObjectVersion{}
	for _, version := range view.snapshot.Heads {
		byObject[version.ObjectID] = append(byObject[version.ObjectID], version)
	}
	for _, heads := range byObject {
		for _, index := range heads {
			root, err := tree.addResource(index, len(heads) != 1)
			if err != nil {
				return privateTree{}, err
			}
			if root {
				roots++
			}
		}
	}
	if roots > 1 {
		return privateTree{}, ErrConflict
	}
	if roots != 1 {
		return privateTree{}, ErrSource
	}
	return tree, nil
}

func (tree *privateTree) addResource(index storage.ObjectVersion, conflict bool) (bool, error) {
	var header struct {
		Kind       string `json:"kind"`
		SourceKind string `json:"source_kind"`
	}
	if json.Unmarshal(index.Commit.Metadata, &header) != nil {
		return false, ErrSource
	}
	if header.Kind != itemKind || header.SourceKind != "file" && header.SourceKind != "folder" && header.SourceKind != "favorite" {
		return false, nil
	}
	item, err := decodeSummary(index)
	if err != nil {
		return false, err
	}
	node := resource{item: item, index: index}
	if conflict {
		tree.conflicts[index.ObjectID] = true
	}
	if item.SourceKind == "favorite" {
		tree.marks = append(tree.marks, node)
		return false, nil
	}
	tree.nodes = append(tree.nodes, node)
	if item.SourceKind == "folder" {
		tree.folders[item.SourceID] = append(tree.folders[item.SourceID], node)
	}
	return item.IsRoot && index.Commit.State == "live", nil
}

func (tree privateTree) unique(node resource) error {
	if tree.conflicts[node.index.ObjectID] {
		return ErrConflict
	}
	return nil
}

// parentPath checks the whole ancestry. Hidden descendants remain hidden even
// when only the top folder has a trash/deletion commit. Names belong only to
// this private in-process tree; no plaintext path request leaves the device.
func (tree privateTree) parentPath(ctx context.Context, node resource) (string, bool, error) {
	if err := tree.unique(node); err != nil {
		return "", false, err
	}
	if node.item.IsRoot {
		if node.item.SourceKind != "folder" || node.item.ParentID != "" || node.index.Commit.State != "live" {
			return "", false, ErrSource
		}
		return "/", true, nil
	}
	names := []string{}
	seen := map[string]bool{}
	if node.item.SourceKind == "folder" {
		seen[node.item.SourceID] = true
	}
	id := node.item.ParentID
	for {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		if id == "" {
			return "", false, ErrSource
		}
		if seen[id] {
			return "", false, ErrCycle
		}
		seen[id] = true
		parents := tree.folders[id]
		if len(parents) == 0 {
			return "", false, ErrSource
		}
		if len(parents) != 1 {
			return "", false, ErrConflict
		}
		parent := parents[0]
		if parent.index.Commit.State != "live" || parent.item.DeletedAt != nil {
			return "", false, nil
		}
		if parent.item.IsRoot {
			if parent.item.ParentID != "" {
				return "", false, ErrSource
			}
			break
		}
		names = append(names, parent.item.Name)
		id = parent.item.ParentID
	}
	for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
		names[i], names[j] = names[j], names[i]
	}
	names = append(names, node.item.Name)
	return "/" + strings.Join(names, "/"), true, nil
}
