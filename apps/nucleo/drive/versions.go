// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"encoding/json"
	"io"
	"slices"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privateVersion struct {
	item        summary
	index       storage.ObjectVersion
	binary      bodyPointer
	observation *observation
	view        api.DriveVersion
	sum         string
}

// Versions combines retained legacy revisions with the authenticated ancestors
// of the current file. Metadata-only edits do not manufacture content versions.
// Device clocks order neither branches nor winners; divergent same-number
// content is an explicit conflict, even when a legacy checksum was absent.
func (r *Repository) Versions(parent context.Context, id, cursor string) ([]api.DriveVersion, string, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, "", err
	}
	defer done()
	before := 0
	if cursor != "" {
		before, err = strconv.Atoi(cursor)
		if err != nil || before < 1 {
			return nil, "", ErrSource
		}
	}
	versions, err := r.fileVersions(ctx, id)
	if err != nil {
		return nil, "", err
	}
	result := []api.DriveVersion{}
	next := ""
	for _, version := range versions {
		if before != 0 && version.view.Version >= before {
			continue
		}
		if len(result) == 500 {
			next = strconv.Itoa(result[len(result)-1].Version)
			break
		}
		result = append(result, version.view)
	}
	return result, next, nil
}

func (r *Repository) fileVersions(ctx context.Context, id string) ([]privateVersion, error) {
	current, err := r.version(ctx, "file", id)
	if err != nil {
		return nil, err
	}
	item, err := decodeSummary(current)
	if err != nil {
		return nil, err
	}
	if err := r.requireLive(ctx, item); err != nil {
		return nil, err
	}
	indexes, err := r.versionIndexes(ctx, current, id)
	if err != nil {
		return nil, err
	}
	byNumber := map[int]privateVersion{}
	for _, index := range indexes {
		candidate, err := r.privateVersion(ctx, index)
		if err != nil {
			return nil, err
		}
		n := candidate.view.Version
		if n < 1 || n > item.Version {
			return nil, ErrSource
		}
		if prior, exists := byNumber[n]; exists {
			if prior.sum != candidate.sum || prior.item.SizeBytes != candidate.item.SizeBytes || prior.item.ContentType != candidate.item.ContentType {
				return nil, ErrConflict
			}
			if !candidate.view.CreatedAt.Before(prior.view.CreatedAt) {
				continue
			}
		}
		candidate.view.IsCurrent = n == item.Version
		byNumber[n] = candidate
	}
	result := make([]privateVersion, 0, len(byNumber))
	for _, candidate := range byNumber {
		result = append(result, candidate)
	}
	slices.SortFunc(result, func(a, b privateVersion) int {
		if a.view.Version > b.view.Version {
			return -1
		}
		if a.view.Version < b.view.Version {
			return 1
		}
		return 0
	})
	return result, nil
}

func (r *Repository) versionIndexes(ctx context.Context, current storage.ObjectVersion, id string) ([]storage.ObjectVersion, error) {
	view, err := r.currentObservation(ctx)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, ErrClosed
	}
	indexes := []storage.ObjectVersion{}
	queue := []storage.ObjectVersion{current}
	seen := map[storage.Reference]bool{}
	for len(queue) > 0 {
		index := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if seen[index.Reference] {
			continue
		}
		seen[index.Reference] = true
		if index.ObjectID != current.ObjectID {
			return nil, ErrSource
		}
		indexes = append(indexes, index)
		for _, ref := range index.Commit.Parents {
			previous, found, err := storage.AcceptedAtomicObject(view.snapshot, r.cfg.Scope, current.ObjectID, "", &ref)
			if err != nil || !found {
				return nil, ErrSource
			}
			queue = append(queue, previous)
		}
	}
	byObject := map[string][]storage.ObjectVersion{}
	for _, version := range view.snapshot.Heads {
		byObject[version.ObjectID] = append(byObject[version.ObjectID], version)
	}
	for _, heads := range byObject {
		for _, index := range heads {
			item, err := decodeSummary(index)
			if err != nil || item.SourceKind != "version" || item.ParentID != id {
				continue
			}
			if len(heads) != 1 {
				return nil, ErrConflict
			}
			if index.Commit.State != "live" {
				return nil, ErrSource
			}
			indexes = append(indexes, index)
		}
	}
	return indexes, nil
}

func (r *Repository) privateVersion(ctx context.Context, index storage.ObjectVersion) (privateVersion, error) {
	item, data, raw, err := r.readEntry(ctx, index)
	defer clear(raw)
	if err != nil {
		return privateVersion{}, err
	}
	if !binaryKind(item.SourceKind) || data.Binary == nil {
		return privateVersion{}, ErrSource
	}
	_, sum, err := r.contentReference(ctx, data.observation, item, *data.Binary)
	if err != nil {
		return privateVersion{}, err
	}
	var attribution struct {
		Owner  string `json:"owner_sub"`
		Author string `json:"author_sub"`
	}
	if json.Unmarshal(raw, &attribution) != nil {
		return privateVersion{}, ErrSource
	}
	if attribution.Author == "" {
		attribution.Author = attribution.Owner
	}
	view := api.DriveVersion{Version: item.Version, SizeBytes: item.SizeBytes, AuthorSub: attribution.Author, CreatedAt: item.UpdatedAt}
	return privateVersion{item: item, index: index, binary: *data.Binary, observation: data.observation, view: view, sum: sum}, nil
}

// DownloadVersion opens an archived version only after checking that the file
// and its parent tree remain live. It uses exactly the same verified stream and
// cancellation path as the current version, never a legacy plaintext endpoint.
func (r *Repository) DownloadVersion(parent context.Context, id string, n int) (io.ReadCloser, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	versions, err := r.fileVersions(ctx, id)
	if err != nil {
		done()
		return nil, err
	}
	for _, version := range versions {
		if version.view.Version == n && !version.view.IsCurrent {
			return r.streamBody(ctx, done, version.observation, version.item, version.binary), nil
		}
	}
	done()
	return nil, ErrNotFound
}
