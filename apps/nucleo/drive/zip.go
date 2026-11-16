// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"path"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var ErrSelectionTooLarge = errors.New("drive: private selection exceeds limit")

type archiveEntry struct {
	node resource
	name string
}

// Zip captures the selected private revisions before streaming. Plaintext is
// written only to the caller, never to a server or a temporary archive on disk.
// The display name is owned by the OS save dialog, not an HTTP response header.
func (r *Repository) Zip(parent context.Context, files, folders []string, _ string) (io.ReadCloser, int64, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, 0, err
	}
	tree, err := r.resourceTree(ctx)
	if err != nil {
		done()
		return nil, 0, err
	}
	entries, size, err := tree.archivePlan(ctx, files, folders)
	if err != nil {
		done()
		return nil, 0, err
	}
	body := r.stream(ctx, done, func(ctx context.Context, out io.Writer) error { return r.writeArchive(ctx, out, entries) })
	return body, size, nil
}

func archiveSelection(files, folders []string) (map[string]bool, map[string]bool, error) {
	if len(files)+len(folders) == 0 {
		return nil, nil, ErrSource
	}
	if len(files)+len(folders) > storage.MaxDriveSelectionItems {
		return nil, nil, ErrSelectionTooLarge
	}
	selected := []map[string]bool{{}, {}}
	for i, ids := range [][]string{files, folders} {
		kind := "file"
		if i == 1 {
			kind = "folder"
		}
		for _, id := range ids {
			if !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: kind, ID: id}) {
				return nil, nil, ErrSource
			}
			selected[i][id] = true
		}
	}
	return selected[0], selected[1], nil
}

func (tree privateTree) archiveFolders(ctx context.Context, selected map[string]bool) ([]string, error) {
	paths := []string{}
	for _, node := range tree.nodes {
		if node.item.SourceKind != "folder" || !selected[node.item.SourceID] || node.item.IsRoot || node.item.DeletedAt != nil || node.index.Commit.State != "live" {
			continue
		}
		name, visible, err := tree.parentPath(ctx, node)
		if err != nil {
			return nil, err
		}
		if visible {
			paths = append(paths, name)
		}
	}
	// The outer selected ancestor wins over an overlapping child selection.
	slices.Sort(paths)
	return paths, nil
}

func (tree privateTree) archivePlan(ctx context.Context, files, folders []string) ([]archiveEntry, int64, error) {
	selected, selectedFolders, err := archiveSelection(files, folders)
	if err != nil {
		return nil, 0, err
	}
	paths, err := tree.archiveFolders(ctx, selectedFolders)
	if err != nil {
		return nil, 0, err
	}
	entries := []archiveEntry{}
	var total int64
	for _, node := range tree.nodes {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		if node.item.SourceKind != "file" || node.item.DeletedAt != nil || node.index.Commit.State != "live" {
			continue
		}
		full, visible, err := tree.parentPath(ctx, node)
		if err != nil {
			return nil, 0, err
		}
		if !visible {
			continue
		}
		name := ""
		for _, folder := range paths {
			if strings.HasPrefix(full, folder+"/") {
				name = strings.TrimPrefix(full, path.Dir(folder)+"/")
				break
			}
		}
		if name == "" && selected[node.item.SourceID] {
			name = node.item.Name
		}
		if name == "" {
			continue
		}
		name = storage.DriveZipPath(name)
		if name == "" || len(name) > 65500 {
			return nil, 0, ErrSource
		}
		entries = append(entries, archiveEntry{node: node, name: name})
		if len(entries) > storage.MaxDriveZipFiles {
			return nil, 0, ErrSelectionTooLarge
		}
		total += node.item.SizeBytes
	}
	slices.SortFunc(entries, func(a, b archiveEntry) int {
		if order := strings.Compare(a.name, b.name); order != 0 {
			return order
		}
		return strings.Compare(a.node.item.SourceID, b.node.item.SourceID)
	})
	if err := uniqueArchiveNames(entries); err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func uniqueArchiveNames(entries []archiveEntry) error {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.name
	}
	if err := storage.UniqueDriveZipNames(names); err != nil {
		return ErrSource
	}
	for i := range entries {
		entries[i].name = names[i]
	}
	return nil
}

func (r *Repository) writeArchive(ctx context.Context, out io.Writer, entries []archiveEntry) error {
	writer := zip.NewWriter(out)
	defer func() { _ = writer.Close() }()
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, data, raw, err := r.readEntry(ctx, entry.node.index)
		clear(raw)
		if err != nil {
			return err
		}
		if item.SourceKind != "file" || data.Binary == nil {
			return ErrSource
		}
		file, err := writer.CreateHeader(&zip.FileHeader{Name: entry.name, Method: zip.Deflate, Modified: item.UpdatedAt})
		if err != nil {
			return err
		}
		if err := r.readBody(ctx, data.observation, *data.Binary, item, "binary", file); err != nil {
			return err
		}
	}
	return writer.Close()
}
