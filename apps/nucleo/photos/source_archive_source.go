// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"io"
	"slices"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func composePhotoCounts(base SnapshotState, scope storage.Scope, epoch string, counts SourcePlanCounts) (SnapshotState, error) {
	if base.Version != 1 || base.Validate(scope) != nil || base.Shared || base.Mode == "legacy" || base.Epoch != epoch || base.Rows != counts.BackendRows || base.Blobs != counts.BackendBlobs {
		return SnapshotState{}, ErrSourcePlan
	}
	rows, blobs, err := counts.Totals()
	if err != nil {
		return SnapshotState{}, err
	}
	base.Rows, base.Blobs = rows, blobs
	return base, base.Validate(scope)
}

func (a *sourceArchive) validateBackend(ctx context.Context, backend Source) (SnapshotState, error) {
	state, err := backend.State(ctx, a.cache.Scope)
	if err != nil {
		return state, err
	}
	return composePhotoCounts(state, a.cache.Scope, a.cache.Ref.Epoch, a.cache.Header.Counts)
}

func (a *sourceArchive) validateOrigins(parent context.Context) error {
	ctx, done, err := a.store.gate.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	if err := a.reauthorize(ctx); err != nil {
		return err
	}
	if a.cache.Header.Info.Originals > 0 && a.store.cfg.Originals == nil {
		return ErrNotReady
	}
	reader, err := a.store.cfg.Journal.OpenStagedFile(ctx, a.store.owner(), sourceArchiveNamespace, a.cache.File)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	_, err = walkSourceArchive(ctx, io.NewSectionReader(reader, 0, a.cache.File.SizeBytes), a.cache.Scope, a.cache.Ref.Epoch, func(_ sourceArchiveHeader, entry sourceArchiveEntry, _ int64, _ int) error {
		if entry.Original != nil {
			return a.store.cfg.Originals.Validate(ctx, a.cache.Scope, *entry.Original)
		}
		return nil
	})
	return err
}

func (a *sourceArchive) backendPage(ctx context.Context, backend Source, after storage.LegacySourceCursor) (SnapshotPage, int64, error) {
	position, _, err := a.findIndex(ctx, after)
	if err != nil {
		return SnapshotPage{}, 0, err
	}
	cursor := storage.LegacySourceCursor{}
	if position > 0 {
		previous, err := a.index(ctx, position-1)
		if err != nil {
			return SnapshotPage{}, 0, err
		}
		cursor = storage.LegacySourceCursor{Kind: previous.Backend.Kind, ID: previous.Backend.ID}
	}
	page, err := backend.Page(ctx, a.cache.Scope, a.cache.Ref.Epoch, cursor)
	if err != nil {
		return page, 0, err
	}
	if page.Validate(a.cache.Scope, a.cache.Ref.Epoch, cursor) != nil {
		return SnapshotPage{}, 0, ErrSourcePlan
	}
	remaining := a.cache.Header.Info.Rows - position
	if int64(len(page.Rows)) > remaining || len(page.Rows) == 0 && remaining != 0 || (page.Next == nil) != (int64(len(page.Rows)) == remaining) {
		return SnapshotPage{}, 0, ErrSourcePlan
	}
	for i, row := range page.Rows {
		expected, err := a.index(ctx, position+int64(i))
		if err != nil {
			return SnapshotPage{}, 0, err
		}
		if row != expected.Backend {
			return SnapshotPage{}, 0, ErrSourcePlan
		}
	}
	return page, position, nil
}

func (a *sourceArchive) knownCursor(ctx context.Context, after storage.LegacySourceCursor) bool {
	if after == (storage.LegacySourceCursor{}) {
		return true
	}
	_, known, err := a.findIndex(ctx, after)
	if err != nil {
		return false
	}
	if known {
		return true
	}
	if after.Kind != "media" && after.Kind != "original" {
		return false
	}
	_, known, err = a.findIndex(ctx, storage.LegacySourceCursor{Kind: "asset", ID: after.ID})
	return err == nil && known
}

func (a *sourceArchive) generatedAfter(ctx context.Context, after storage.LegacySourceCursor) ([]storage.StreamSourceRow, bool, error) {
	rows := make([]storage.StreamSourceRow, 0, storage.MaxLegacySourceRows)
	for _, kind := range []string{"media", "original"} {
		if after.Kind > kind {
			continue
		}
		start := int64(0)
		if after.Kind == kind {
			position, _, err := a.findIndex(ctx, storage.LegacySourceCursor{Kind: "asset", ID: after.ID})
			if err != nil {
				return nil, false, err
			}
			start = max(int64(0), position-a.cache.AssetStart)
		}
		for n := start; n < a.cache.Header.Info.Originals; n++ {
			if len(rows) == storage.MaxLegacySourceRows {
				return rows, true, nil
			}
			index, err := a.index(ctx, a.cache.AssetStart+n)
			if err != nil {
				return nil, false, err
			}
			entry, err := a.entry(ctx, index)
			if err != nil {
				return nil, false, err
			}
			if entry.Original == nil || entry.Backend.Kind != "asset" {
				return nil, false, ErrSourcePlan
			}
			row := entry.Original.Descriptor
			if kind == "media" {
				metadata, raw, err := generatedOriginalMetadata(a.cache.Scope, *entry.Original)
				clear(raw)
				if err != nil {
					clear(entry.Original.Pin)
					return nil, false, err
				}
				row = metadata
			}
			clear(entry.Original.Pin)
			rows = append(rows, row)
		}
	}
	return rows, false, nil
}

func (a *sourceArchive) page(parent context.Context, backend Source, after storage.LegacySourceCursor) (SnapshotPage, error) {
	if after != (storage.LegacySourceCursor{}) && !storage.SourceCursorValid(after) {
		return SnapshotPage{}, ErrSourcePlan
	}
	ctx, done, err := a.store.gate.Start(parent)
	if err != nil {
		return SnapshotPage{}, err
	}
	defer done()
	if err := a.reauthorize(ctx); err != nil {
		return SnapshotPage{}, err
	}
	if _, err := a.validateBackend(ctx, backend); err != nil {
		return SnapshotPage{}, err
	}
	if !a.knownCursor(ctx, after) {
		return SnapshotPage{}, ErrSourcePlan
	}
	page, position, err := a.backendPage(ctx, backend, after)
	if err != nil {
		return SnapshotPage{}, err
	}
	generated, moreGenerated, err := a.generatedAfter(ctx, after)
	if err != nil {
		return SnapshotPage{}, err
	}
	rows := slices.Clone(page.Rows)
	for _, row := range generated {
		if page.Next != nil {
			boundary, err := a.index(ctx, position+int64(len(page.Rows)))
			if err != nil {
				return SnapshotPage{}, err
			}
			if sourceRowCompare(row, boundary.Backend) >= 0 {
				continue
			}
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, sourceRowCompare)
	more := page.Next != nil || moreGenerated || len(rows) > storage.MaxLegacySourceRows
	if len(rows) > storage.MaxLegacySourceRows {
		rows = rows[:storage.MaxLegacySourceRows]
	}
	result := SnapshotPage{Scope: a.cache.Scope, Epoch: a.cache.Ref.Epoch, Rows: rows}
	if more && len(rows) > 0 {
		last := rows[len(rows)-1]
		result.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
	}
	return result, result.Validate(a.cache.Scope, a.cache.Ref.Epoch, after)
}

func (a *sourceArchive) original(ctx context.Context, id string) (PlannedOriginal, error) {
	position, found, err := a.findIndex(ctx, storage.LegacySourceCursor{Kind: "asset", ID: id})
	if err != nil {
		return PlannedOriginal{}, err
	}
	if !found {
		return PlannedOriginal{}, ErrSourcePlan
	}
	index, err := a.index(ctx, position-1)
	if err != nil {
		return PlannedOriginal{}, err
	}
	entry, err := a.entry(ctx, index)
	if err != nil {
		return PlannedOriginal{}, err
	}
	if entry.Original == nil {
		return PlannedOriginal{}, ErrSourcePlan
	}
	return *entry.Original, nil
}

func (a *sourceArchive) open(parent context.Context, backend Source, row SnapshotRow) (io.ReadCloser, error) {
	if row.Validate() != nil {
		return nil, ErrSourcePlan
	}
	ctx, done, err := a.store.gate.Start(parent)
	if err != nil {
		return nil, err
	}
	transferred := false
	defer func() {
		if !transferred {
			done()
		}
	}()
	if err := a.reauthorize(ctx); err != nil {
		return nil, err
	}
	if _, err := a.validateBackend(ctx, backend); err != nil {
		return nil, err
	}
	position, found, err := a.findIndex(ctx, storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID})
	if err != nil {
		return nil, err
	}
	if found {
		index, err := a.index(ctx, position-1)
		if err != nil {
			return nil, err
		}
		if index.Backend != row {
			return nil, ErrSourcePlan
		}
		body, err := backend.Open(ctx, a.cache.Scope, a.cache.Ref.Epoch, row)
		if err != nil {
			return nil, err
		}
		if body == nil {
			return nil, ErrSourcePlan
		}
		var closeErr error
		finish := sync.OnceFunc(func() { closeErr = body.Close(); done() })
		stop := context.AfterFunc(ctx, finish)
		transferred = true
		return archiveLeasedReader{ReadCloser: body, close: func() error { stop(); finish(); return closeErr }}, nil
	}
	if row.Kind != "media" && row.Kind != "original" {
		return nil, ErrSourcePlan
	}
	pin, err := a.original(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	if row.Kind == "media" {
		defer clear(pin.Pin)
		metadata, raw, err := generatedOriginalMetadata(a.cache.Scope, pin)
		if err != nil {
			return nil, err
		}
		if metadata != row {
			clear(raw)
			return nil, ErrSourcePlan
		}
		return io.NopCloser(bytes.NewReader(raw)), nil
	}
	if row != pin.Descriptor {
		clear(pin.Pin)
		return nil, ErrSourcePlan
	}
	if a.store.cfg.Originals == nil {
		clear(pin.Pin)
		return nil, ErrNotReady
	}
	work, cancel := context.WithCancel(ctx)
	reader, writer := io.Pipe()
	release := sync.OnceFunc(done)
	stop := context.AfterFunc(work, func() { _ = reader.CloseWithError(work.Err()); release() })
	transferred = true
	go func() {
		defer clear(pin.Pin)
		err := a.store.cfg.Originals.Write(work, a.cache.Scope, pin, writer)
		_ = writer.CloseWithError(err)
	}()
	finish := sync.OnceFunc(func() { stop(); cancel(); _ = reader.Close(); release() })
	return archiveLeasedReader{ReadCloser: reader, close: func() error { finish(); return nil }}, nil
}

type archiveLeasedReader struct {
	io.ReadCloser
	close func() error
}

func (r archiveLeasedReader) Close() error { return r.close() }
