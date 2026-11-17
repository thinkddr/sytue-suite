// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const queryPageSize = 50

type recentCursor struct {
	V       int       `json:"v"`
	Account string    `json:"account"`
	Kind    string    `json:"kind"`
	Before  time.Time `json:"before"`
	ID      string    `json:"id"`
}

func (r *Repository) recentCursor(value string) (recentCursor, error) {
	if value == "" {
		return recentCursor{}, nil
	}
	if len(value) > 4096 {
		return recentCursor{}, ErrSource
	}
	bytes, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil {
		return recentCursor{}, ErrSource
	}
	var cursor recentCursor
	if json.Unmarshal(bytes, &cursor) != nil || cursor.V != 1 || cursor.Account != digest([]byte(r.owner())) || cursor.Kind != "drive_recent" || cursor.Before.IsZero() || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: "file", ID: cursor.ID}) {
		return recentCursor{}, ErrSource
	}
	return cursor, nil
}

func (r *Repository) encodeRecentCursor(item summary) string {
	raw, _ := json.Marshal(recentCursor{V: 1, Account: digest([]byte(r.owner())), Kind: "drive_recent", Before: item.UpdatedAt, ID: item.SourceID})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func compareRecent(a, b summary) int {
	if compared := b.UpdatedAt.Compare(a.UpdatedAt); compared != 0 {
		return compared
	}
	return strings.Compare(b.SourceID, a.SourceID)
}

func afterRecent(item summary, cursor recentCursor) bool {
	return cursor.V == 0 || compareRecent(item, summary{UpdatedAt: cursor.Before, SourceID: cursor.ID}) > 0
}

func (r *Repository) liveQueryCandidates(ctx context.Context, tree privateTree, accept func(summary) bool) ([]resource, error) {
	result := []resource{}
	for _, node := range tree.nodes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if node.item.SourceKind != "file" || node.item.DeletedAt != nil || node.index.Commit.State != "live" || !accept(node.item) {
			continue
		}
		_, visible, err := tree.parentPath(ctx, node)
		if err != nil {
			return nil, err
		}
		if visible {
			result = append(result, node)
		}
	}
	slices.SortFunc(result, func(a, b resource) int { return compareRecent(a.item, b.item) })
	return result, nil
}

func (r *Repository) queryView(ctx context.Context, tree privateTree, node resource) (api.Compartido, error) {
	path, visible, err := tree.parentPath(ctx, node)
	if err != nil {
		return api.Compartido{}, err
	}
	if !visible {
		return api.Compartido{}, ErrNotFound
	}
	_, _, raw, err := r.readEntry(ctx, node.index)
	defer clear(raw)
	if err != nil {
		return api.Compartido{}, err
	}
	file, err := fileView(raw, node.item.SourceID)
	if err != nil {
		return api.Compartido{}, err
	}
	file.Path = path
	return api.Compartido{File: file, IsDir: node.item.SourceKind == "folder"}, nil
}

// Search uses the suite's accent/case folding on private names. Like the legacy
// API it returns the fifty most recently updated matching live files.
func (r *Repository) Search(parent context.Context, text string) (api.Children, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return api.Children{}, err
	}
	defer done()
	tree, err := r.resourceTree(ctx)
	if err != nil {
		return api.Children{}, err
	}
	query := buscar.Plegar(strings.TrimSpace(text))
	nodes, err := r.liveQueryCandidates(ctx, tree, func(item summary) bool { return strings.Contains(buscar.Plegar(item.Name), query) })
	if err != nil {
		return api.Children{}, err
	}
	result := api.Compartidos{}
	for _, node := range nodes[:min(len(nodes), queryPageSize)] {
		view, err := r.queryView(ctx, tree, node)
		if err != nil {
			return api.Children{}, err
		}
		result = append(result, view)
	}
	return result.Hijos(), nil
}

// Recent paginates the private tree with the same timestamp/id ordering as the
// service. Cursors retain their original key and are bound to this account and
// product, even if the item is moved, renamed or deleted between pages.
func (r *Repository) Recent(parent context.Context, value string) (api.Compartidos, string, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, "", err
	}
	defer done()
	cursor, err := r.recentCursor(value)
	if err != nil {
		return nil, "", err
	}
	tree, err := r.resourceTree(ctx)
	if err != nil {
		return nil, "", err
	}
	nodes, err := r.liveQueryCandidates(ctx, tree, func(item summary) bool { return afterRecent(item, cursor) })
	if err != nil {
		return nil, "", err
	}
	result := api.Compartidos{}
	next := ""
	for _, node := range nodes[:min(len(nodes), queryPageSize)] {
		view, err := r.queryView(ctx, tree, node)
		if err != nil {
			return nil, "", err
		}
		result = append(result, view)
	}
	if len(nodes) > queryPageSize {
		next = r.encodeRecentCursor(nodes[queryPageSize-1].item)
	}
	return result, next, nil
}

// Trash returns top-level deleted resources. Descendants hidden by a trashed
// ancestor are represented by that ancestor, without rewriting every child.
// Retention stays zero until the authorized private cleanup executor is active.
func (r *Repository) Trash(parent context.Context) (api.DriveTrash, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return api.DriveTrash{}, err
	}
	defer done()
	tree, err := r.resourceTree(ctx)
	if err != nil {
		return api.DriveTrash{}, err
	}
	nodes := []resource{}
	for _, node := range tree.nodes {
		if node.item.DeletedAt == nil || node.item.IsRoot || node.index.Commit.State != "trash" {
			continue
		}
		_, visible, err := tree.parentPath(ctx, node)
		if err != nil {
			return api.DriveTrash{}, err
		}
		if visible {
			nodes = append(nodes, node)
		}
	}
	slices.SortFunc(nodes, func(a, b resource) int {
		if order := b.item.DeletedAt.Compare(*a.item.DeletedAt); order != 0 {
			return order
		}
		return strings.Compare(childKey(a.item), childKey(b.item))
	})
	result := api.DriveTrash{Children: api.Children{Folders: []api.Folder{}, Files: []api.File{}}}
	for _, node := range nodes {
		if err := r.appendTrash(ctx, tree, &result, node); err != nil {
			return api.DriveTrash{}, err
		}
	}
	return result, nil
}

func (r *Repository) appendTrash(ctx context.Context, tree privateTree, result *api.DriveTrash, node resource) error {
	view, err := r.queryView(ctx, tree, node)
	if err != nil {
		return err
	}
	if !view.IsDir {
		result.Files = append(result.Files, view.File)
		return nil
	}
	parent := node.item.ParentID
	result.Folders = append(result.Folders, api.Folder{ID: view.ID, ParentID: &parent, Name: view.Name, UpdatedAt: view.UpdatedAt, DeletedAt: view.DeletedAt, Path: view.Path})
	return nil
}
