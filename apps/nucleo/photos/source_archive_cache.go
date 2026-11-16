// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	sourceArchiveIndexRows = 64
	sourceArchiveNamespace = "photos-private-stage-v1"
)

type sourceArchiveCache struct {
	Version    int                    `json:"v"`
	Scope      storage.Scope          `json:"scope"`
	Ref        SourcePlanReference    `json:"ref"`
	Pointer    storage.ContentPointer `json:"pointer"`
	Request    storage.FileRequest    `json:"request"`
	SHA256     string                 `json:"sha256"`
	File       offline.StagedFile     `json:"file"`
	Header     sourceArchiveHeader    `json:"header"`
	AssetStart int64                  `json:"asset_start"`
	Indexed    bool                   `json:"indexed"`
}
type sourceArchiveIndex struct {
	Ordinal int64                   `json:"ordinal"`
	Offset  int64                   `json:"offset"`
	Bytes   int                     `json:"bytes"`
	Backend storage.StreamSourceRow `json:"backend"`
}
type sourceArchiveIndexPage struct {
	First   int64                `json:"first"`
	Entries []sourceArchiveIndex `json:"entries"`
}

// sourceArchive is an authenticated private-file view with bounded index pages.
// It never holds the full descriptor/pin catalog in RAM. Each operation renews
// the same work lease and reauthorizes the exact SDK file commitment.
type sourceArchive struct {
	store *SourcePlanStore
	key   string
	cache sourceArchiveCache
}

func archiveCacheKey(scope storage.Scope, ref SourcePlanReference, pointer storage.ContentPointer, request storage.FileRequest, sha string) string {
	if len(request.Previous) == 0 {
		request.Previous = nil
	}
	raw, _ := json.Marshal(struct {
		Scope   storage.Scope
		Ref     SourcePlanReference
		Pointer storage.ContentPointer
		Request storage.FileRequest
		SHA256  string
	}{scope, ref, pointer, request, sha})
	defer clear(raw)
	return "source_archive_" + planBytesHash(raw)
}

func archiveIndexKey(key string, page int64) string {
	return key + "_page_" + strconv.FormatInt(page, 10)
}

func (s *SourcePlanStore) archiveCommitment(ctx context.Context, ref SourcePlanReference) (storage.ContentPointer, storage.FileRequest, string, error) {
	pointer, request, err := s.planFile(ctx, ref)
	if err != nil {
		return pointer, request, "", err
	}
	if !bytes.Equal(request.Metadata, sourcePlanMetadata(sourceArchiveKind)) {
		return pointer, request, "", ErrSourcePlan
	}
	proof, ok := s.cfg.Crypto.(planCommitments)
	if !ok {
		return pointer, request, "", ErrNotReady
	}
	actual, sha, err := proof.StorageFileCommitment(ctx, pointer.Reference, request.ObjectID, request.RevisionID)
	if err != nil {
		return pointer, request, "", err
	}
	if !samePlanRequest(actual, request) || actual.Validate(s.cfg.Scope) != nil {
		return pointer, request, "", ErrSourcePlan
	}
	return pointer, request, sha, ctx.Err()
}

func (c sourceArchiveCache) matches(scope storage.Scope, ref SourcePlanReference, pointer storage.ContentPointer, request storage.FileRequest, sha string) bool {
	return c.Version == 2 && c.Scope == scope && c.Ref == ref && c.Pointer == pointer && samePlanRequest(c.Request, request) && c.SHA256 == sha && c.File.SHA256 == sha && c.File.SizeBytes == request.SizeBytes
}

// openArchive(forceAvailability=true) verifies all SDK-backed file bytes even
// when a complete local cache exists. Acceptance of a newly published plan must
// use it. Ordinary metadata reads prove the authenticated commitment and cache,
// not current provider availability of this plan or any original image.
func (s *SourcePlanStore) openArchive(parent context.Context, ref SourcePlanReference, forceAvailability bool) (*sourceArchive, error) {
	ctx, done, err := s.gate.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	pointer, request, sha, err := s.archiveCommitment(ctx, ref)
	if err != nil {
		return nil, err
	}
	key := archiveCacheKey(s.cfg.Scope, ref, pointer, request, sha)
	cache := sourceArchiveCache{Version: 2, Scope: s.cfg.Scope, Ref: ref, Pointer: pointer, Request: request, SHA256: sha, AssetStart: -1}
	err = s.cfg.Journal.Exclusive(ctx, s.owner(), sourceArchiveNamespace, key, func() error { return s.loadArchiveCache(ctx, key, &cache, forceAvailability) })
	if err != nil {
		return nil, err
	}
	return &sourceArchive{store: s, key: key, cache: cache}, nil
}

func (s *SourcePlanStore) loadArchiveCache(ctx context.Context, key string, cache *sourceArchiveCache, forceAvailability bool) error {
	var existing sourceArchiveCache
	found, err := s.cfg.Journal.Read(s.owner(), sourceArchiveNamespace, key, &existing)
	if err != nil {
		return err
	}
	if found {
		if !existing.matches(cache.Scope, cache.Ref, cache.Pointer, cache.Request, cache.SHA256) {
			return ErrSourcePlan
		}
		*cache = existing
		reader, err := s.cfg.Journal.OpenStagedFile(ctx, s.owner(), sourceArchiveNamespace, cache.File)
		if err != nil {
			return err
		}
		if err := reader.Close(); err != nil {
			return err
		}
		if forceAvailability {
			if err := s.verifyArchiveAvailability(ctx, cache.Pointer, cache.Request); err != nil {
				return err
			}
		}
	} else {
		file, err := s.captureArchiveDownload(ctx, cache.Pointer, cache.Request)
		if err != nil {
			return err
		}
		cache.File = file
		if file.SHA256 != cache.SHA256 || file.SizeBytes != cache.Request.SizeBytes {
			return ErrSourcePlan
		}
		if err := s.cfg.Journal.Write(s.owner(), sourceArchiveNamespace, key, *cache); err != nil {
			return err
		}
	}
	if !cache.Indexed {
		if err := s.indexArchive(ctx, key, cache); err != nil {
			return err
		}
	}
	return validateArchiveCache(*cache)
}

func (s *SourcePlanStore) verifyArchiveAvailability(ctx context.Context, pointer storage.ContentPointer, request storage.FileRequest) error {
	actual, err := s.cfg.Crypto.DownloadStorageFile(ctx, s.cfg.Gateway, pointer.Reference, request.ObjectID, request.RevisionID, io.Discard)
	if err != nil {
		return err
	}
	if !samePlanRequest(actual, request) {
		return ErrSourcePlan
	}
	return ctx.Err()
}

func (s *SourcePlanStore) captureArchiveDownload(ctx context.Context, pointer storage.ContentPointer, request storage.FileRequest) (offline.StagedFile, error) {
	reader, writer := io.Pipe()
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(work, func() { _ = reader.CloseWithError(work.Err()) })
	defer stop()
	finished := make(chan error, 1)
	go func() {
		actual, err := s.cfg.Crypto.DownloadStorageFile(work, s.cfg.Gateway, pointer.Reference, request.ObjectID, request.RevisionID, writer)
		if err == nil && !samePlanRequest(actual, request) {
			err = ErrSourcePlan
		}
		_ = writer.CloseWithError(err)
		finished <- err
	}()
	file, err := s.cfg.Journal.StageFile(work, s.owner(), sourceArchiveNamespace, reader, request.SizeBytes)
	_ = reader.Close()
	cancel()
	producerErr := <-finished
	if err != nil {
		return offline.StagedFile{}, err
	}
	if producerErr != nil {
		return offline.StagedFile{}, producerErr
	}
	return file, nil
}

func (s *SourcePlanStore) indexArchive(ctx context.Context, key string, cache *sourceArchiveCache) error {
	reader, err := s.cfg.Journal.OpenStagedFile(ctx, s.owner(), sourceArchiveNamespace, cache.File)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	page := sourceArchiveIndexPage{Entries: make([]sourceArchiveIndex, 0, sourceArchiveIndexRows)}
	var ordinal int64
	flush := func() error {
		if len(page.Entries) == 0 {
			return nil
		}
		if err := s.cfg.Journal.Write(s.owner(), sourceArchiveNamespace, archiveIndexKey(key, page.First/sourceArchiveIndexRows), page); err != nil {
			return err
		}
		page = sourceArchiveIndexPage{First: ordinal, Entries: make([]sourceArchiveIndex, 0, sourceArchiveIndexRows)}
		return nil
	}
	header, err := walkSourceArchive(ctx, io.NewSectionReader(reader, 0, cache.File.SizeBytes), s.cfg.Scope, cache.Ref.Epoch, func(_ sourceArchiveHeader, entry sourceArchiveEntry, offset int64, size int) error {
		if entry.Original != nil && cache.AssetStart == -1 {
			cache.AssetStart = ordinal
		}
		page.Entries = append(page.Entries, sourceArchiveIndex{Ordinal: ordinal, Offset: offset, Bytes: size, Backend: entry.Backend})
		ordinal++
		if len(page.Entries) == sourceArchiveIndexRows {
			return flush()
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	cache.Header = header
	cache.Indexed = true
	if err := validateArchiveCache(*cache); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.cfg.Journal.Write(s.owner(), sourceArchiveNamespace, key, *cache)
}

func validateArchiveCache(cache sourceArchiveCache) error {
	if !cache.Indexed || cache.Header.validate(cache.Scope, cache.Ref.Epoch) != nil {
		return ErrSourcePlan
	}
	if cache.Header.Info.Originals == 0 {
		if cache.AssetStart != -1 {
			return ErrSourcePlan
		}
		return nil
	}
	if cache.AssetStart < 0 || cache.AssetStart > cache.Header.Info.Rows-cache.Header.Info.Originals {
		return ErrSourcePlan
	}
	return nil
}

func (a *sourceArchive) reauthorize(ctx context.Context) error {
	pointer, request, sha, err := a.store.archiveCommitment(ctx, a.cache.Ref)
	if err != nil {
		return err
	}
	if !a.cache.matches(a.store.cfg.Scope, a.cache.Ref, pointer, request, sha) || validateArchiveCache(a.cache) != nil {
		return ErrSourcePlan
	}
	return ctx.Err()
}

func (a *sourceArchive) header(parent context.Context) (sourceArchiveHeader, error) {
	ctx, done, err := a.store.gate.Start(parent)
	if err != nil {
		return sourceArchiveHeader{}, err
	}
	defer done()
	if err := a.reauthorize(ctx); err != nil {
		return sourceArchiveHeader{}, err
	}
	return a.cache.Header, nil
}

func (a *sourceArchive) index(ctx context.Context, ordinal int64) (sourceArchiveIndex, error) {
	if ordinal < 0 || ordinal >= a.cache.Header.Info.Rows {
		return sourceArchiveIndex{}, ErrSourcePlan
	}
	var page sourceArchiveIndexPage
	pageNum := ordinal / sourceArchiveIndexRows
	found, err := a.store.cfg.Journal.Read(a.store.owner(), sourceArchiveNamespace, archiveIndexKey(a.key, pageNum), &page)
	if err != nil {
		return sourceArchiveIndex{}, err
	}
	want := min(int64(sourceArchiveIndexRows), a.cache.Header.Info.Rows-pageNum*sourceArchiveIndexRows)
	if !found || page.First != pageNum*sourceArchiveIndexRows || int64(len(page.Entries)) != want {
		return sourceArchiveIndex{}, ErrSourcePlan
	}
	value := page.Entries[ordinal-page.First]
	if value.Ordinal != ordinal || value.Offset < 0 || value.Bytes < 2 || value.Bytes > maxSourceArchiveLine || value.Offset > a.cache.File.SizeBytes-int64(value.Bytes) || value.Backend.Validate() != nil || !backendPlanKind(value.Backend.Kind) {
		return sourceArchiveIndex{}, ErrSourcePlan
	}
	return value, ctx.Err()
}

func (a *sourceArchive) entry(ctx context.Context, index sourceArchiveIndex) (sourceArchiveEntry, error) {
	entry, err := readSourceIndexedJSON[sourceArchiveEntry](ctx, a.store, a.cache.File, index)
	if err != nil || entry.validate(a.cache.Scope) != nil || entry.Backend != index.Backend {
		if entry.Original != nil {
			clear(entry.Original.Pin)
		}
		return sourceArchiveEntry{}, ErrSourcePlan
	}
	return entry, nil
}

func readSourceIndexedJSON[T any](ctx context.Context, store *SourcePlanStore, file offline.StagedFile, index sourceArchiveIndex) (T, error) {
	var value T
	if index.Bytes < 2 || index.Bytes > maxSourceArchiveLine || index.Offset < 0 || index.Offset > file.SizeBytes-int64(index.Bytes) {
		return value, ErrSourcePlan
	}
	reader, err := store.cfg.Journal.OpenStagedFile(ctx, store.owner(), sourceArchiveNamespace, file)
	if err != nil {
		return value, err
	}
	defer func() { _ = reader.Close() }()
	raw := make([]byte, index.Bytes)
	defer clear(raw)
	if _, err := reader.ReadAt(raw, index.Offset); err != nil {
		return value, err
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, ErrSourcePlan
	}
	return value, ctx.Err()
}

func (a *sourceArchive) findIndex(ctx context.Context, after storage.LegacySourceCursor) (int64, bool, error) {
	left, right := int64(0), a.cache.Header.Info.Rows
	for left < right {
		middle := left + (right-left)/2
		index, err := a.index(ctx, middle)
		if err != nil {
			return 0, false, err
		}
		if afterSourceRow(index.Backend, after) {
			right = middle
		} else {
			left = middle + 1
		}
	}
	if left == 0 {
		return 0, false, nil
	}
	previous, err := a.index(ctx, left-1)
	if err != nil {
		return 0, false, err
	}
	return left, previous.Backend.Kind == after.Kind && previous.Backend.ID == after.ID, nil
}
