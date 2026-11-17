// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const sourceGraphKind = "org.sytue.photos.source-graph.v1"

func sourceGraphFileKey(info model.SourcePreflightInfo, file offline.StagedFile) string {
	return "source_graph_" + planBytesHash([]byte(info.Epoch+"\x00"+info.CAS+"\x00"+file.SHA256))
}

type (
	sourceGraphHeader struct {
		Kind string                    `json:"kind"`
		Info model.SourcePreflightInfo `json:"source"`
	}
	sourceGraphEntry struct {
		Backend storage.StreamSourceRow `json:"backend"`
		Graph   *model.Row              `json:"graph,omitempty"`
	}
	sourceGraphFile struct {
		source *sourceArchive
		info   model.SourcePreflightInfo
	}
)

func writeSourceGraph(ctx context.Context, out io.Writer, info model.SourcePreflightInfo, control PagedSourcePlanControl) error {
	if info.Validate(info.Scope, info.Epoch) != nil || control == nil {
		return ErrSourcePlan
	}
	if err := writeSourceArchiveLine(out, sourceGraphHeader{Kind: sourceGraphKind, Info: info}); err != nil {
		return err
	}
	sum, err := model.SourceInventoryHash(info.Scope, info.Epoch)
	if err != nil {
		return err
	}
	cursor := storage.LegacySourceCursor{}
	var rows, blobs, originals int64
	for {
		page, err := control.PrivateSourcePreflightPage(ctx, info, cursor)
		if err != nil {
			return err
		}
		if page.Validate(info.Scope, info.Epoch, cursor) != nil {
			return ErrSourcePlan
		}
		for _, descriptor := range page.Rows {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := model.AppendSourceInventoryHash(sum, descriptor); err != nil {
				return err
			}
			if err := writeSourceGraphEntry(ctx, out, info, control, descriptor); err != nil {
				return err
			}
			if descriptor.Kind == "variant" {
				blobs++
			}
			if descriptor.Kind == "asset" {
				originals++
			}
			rows++
		}
		if page.Next == nil {
			break
		}
		cursor = *page.Next
	}
	if rows != info.Rows || blobs != info.Blobs || originals != info.Originals || hex.EncodeToString(sum.Sum(nil)) != info.SHA256 {
		return ErrSourcePlan
	}
	return ctx.Err()
}

func (s *SourcePlanStore) captureSourceGraph(ctx context.Context, info model.SourcePreflightInfo, control PagedSourcePlanControl) (offline.StagedFile, error) {
	reader, writer := io.Pipe()
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(work, func() { _ = reader.CloseWithError(work.Err()) })
	defer stop()
	finished := make(chan error, 1)
	go func() {
		err := writeSourceGraph(work, writer, info, control)
		_ = writer.CloseWithError(err)
		finished <- err
	}()
	file, err := s.cfg.Journal.StageStream(work, s.owner(), sourceArchiveNamespace, reader, storage.MaxFileBytes)
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

func (s *SourcePlanStore) openSourceGraph(ctx context.Context, info model.SourcePreflightInfo, file offline.StagedFile) (*sourceGraphFile, error) {
	if info.Validate(s.cfg.Scope, info.Epoch) != nil {
		return nil, ErrSourcePlan
	}
	key := sourceGraphFileKey(info, file)
	header := sourceArchiveHeader{Kind: sourceArchiveKind, Info: info, Counts: SourcePlanCounts{BackendRows: info.Rows, BackendBlobs: info.Blobs, OriginalRows: 2 * info.Originals, OriginalBlobs: info.Originals}}
	view := &sourceArchive{store: s, key: key, cache: sourceArchiveCache{Scope: s.cfg.Scope, File: file, Header: header, AssetStart: -1}}
	reader, err := s.cfg.Journal.OpenStagedFile(ctx, s.owner(), sourceArchiveNamespace, file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	input := bufio.NewReaderSize(io.NewSectionReader(reader, 0, file.SizeBytes), maxSourceArchiveLine)
	raw, err := input.ReadSlice('\n')
	if err != nil {
		return nil, ErrSourcePlan
	}
	var actual sourceGraphHeader
	if json.Unmarshal(raw, &actual) != nil || actual.Kind != sourceGraphKind || actual.Info != info {
		return nil, ErrSourcePlan
	}
	offset := int64(len(raw))
	clear(raw)
	return s.indexSourceGraph(ctx, info, view, input, offset)
}

func (s *SourcePlanStore) indexSourceGraph(ctx context.Context, info model.SourcePreflightInfo, view *sourceArchive, input *bufio.Reader, offset int64) (*sourceGraphFile, error) {
	key := view.key
	page := sourceArchiveIndexPage{Entries: make([]sourceArchiveIndex, 0, sourceArchiveIndexRows)}
	var count, blobs, originals int64
	sum, err := model.SourceInventoryHash(info.Scope, info.Epoch)
	if err != nil {
		return nil, err
	}
	flush := func() error {
		if len(page.Entries) == 0 {
			return nil
		}
		if err := s.cfg.Journal.Write(s.owner(), sourceArchiveNamespace, archiveIndexKey(key, page.First/sourceArchiveIndexRows), page); err != nil {
			return err
		}
		page = sourceArchiveIndexPage{First: count, Entries: make([]sourceArchiveIndex, 0, sourceArchiveIndexRows)}
		return nil
	}
	flushFull := func() error {
		if len(page.Entries) == sourceArchiveIndexRows {
			return flush()
		}
		return nil
	}
	last := storage.LegacySourceCursor{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		raw, err := input.ReadSlice('\n')
		if errors.Is(err, io.EOF) && len(raw) == 0 {
			break
		}
		if err != nil {
			return nil, ErrSourcePlan
		}
		entry, err := parseSourceGraphEntry(raw, info.Scope, last)
		if err != nil {
			return nil, err
		}
		if err := model.AppendSourceInventoryHash(sum, entry.Backend); err != nil {
			return nil, err
		}
		page.Entries = append(page.Entries, sourceArchiveIndex{Ordinal: count, Offset: offset, Bytes: len(raw), Backend: entry.Backend})
		if entry.Backend.Kind == "asset" {
			view.cache.AssetStart = firstSourceAsset(view.cache.AssetStart, count)
			originals++
		}
		if entry.Backend.Kind == "variant" {
			blobs++
		}
		count++
		last = storage.LegacySourceCursor{Kind: entry.Backend.Kind, ID: entry.Backend.ID}
		offset += int64(len(raw))
		clear(raw)
		if entry.Graph != nil {
			clear(entry.Graph.JSON)
		}
		if err := flushFull(); err != nil {
			return nil, err
		}
	}
	if !sourceGraphInventoryMatches(info, count, blobs, originals, hex.EncodeToString(sum.Sum(nil))) {
		return nil, ErrSourcePlan
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return &sourceGraphFile{source: view, info: info}, nil
}

func validateSourceGraphFrame(scope storage.Scope, entry sourceGraphEntry) error {
	if entry.Backend.Validate() != nil || !backendPlanKind(entry.Backend.Kind) {
		return ErrSourcePlan
	}
	if entry.Backend.Kind == "variant" {
		if entry.Graph != nil {
			return ErrSourcePlan
		}
		return nil
	}
	if entry.Graph == nil || entry.Graph.Kind != entry.Backend.Kind || entry.Graph.ID != entry.Backend.ID || model.ValidateRow(scope, *entry.Graph) != nil {
		return ErrSourcePlan
	}
	return nil
}

func (g *sourceGraphFile) entry(ctx context.Context, ordinal int64) (sourceGraphEntry, error) {
	index, err := g.source.index(ctx, ordinal)
	if err != nil {
		return sourceGraphEntry{}, err
	}
	entry, err := readSourceIndexedJSON[sourceGraphEntry](ctx, g.source.store, g.source.cache.File, index)
	if err != nil || entry.Backend != index.Backend || validateSourceGraphFrame(g.info.Scope, entry) != nil {
		if entry.Graph != nil {
			clear(entry.Graph.JSON)
		}
		return sourceGraphEntry{}, ErrSourcePlan
	}
	return entry, nil
}

func (g *sourceGraphFile) lookup(ctx context.Context, kind, id string) (storage.StreamSourceRow, *model.Row, error) {
	position, found, err := g.source.findIndex(ctx, storage.LegacySourceCursor{Kind: kind, ID: id})
	if err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	if !found {
		return storage.StreamSourceRow{}, nil, ErrSourcePlan
	}
	entry, err := g.entry(ctx, position-1)
	return entry.Backend, entry.Graph, err
}

func (g *sourceGraphFile) seen(class, key, id string) error {
	store := g.source.store
	name := g.source.key + "_seen_" + planBytesHash([]byte(class+"\x00"+key))
	var existing string
	found, err := store.cfg.Journal.Read(store.owner(), sourceArchiveNamespace, name, &existing)
	if err != nil {
		return err
	}
	if found {
		if existing != id {
			return ErrSourcePlan
		}
		return nil
	}
	return store.cfg.Journal.Write(store.owner(), sourceArchiveNamespace, name, id)
}

func (g *sourceGraphFile) validate(ctx context.Context) error {
	for ordinal := int64(0); ordinal < g.info.Rows; ordinal++ {
		entry, err := g.entry(ctx, ordinal)
		if err != nil {
			return err
		}
		err = validateSourceGraphEntry(ctx, g.info.Scope, entry.Backend, entry.Graph, g.lookup, g.seen)
		if entry.Graph != nil {
			clear(entry.Graph.JSON)
		}
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}

func writeSourceGraphEntry(ctx context.Context, out io.Writer, info model.SourcePreflightInfo, control PagedSourcePlanControl, descriptor storage.StreamSourceRow) error {
	entry := sourceGraphEntry{Backend: descriptor}
	if descriptor.Kind != "variant" {
		row, err := readPreflightValidationRow(ctx, info, control, descriptor)
		if err != nil {
			return err
		}
		entry.Graph = &row
		defer clear(row.JSON)
	}
	return writeSourceArchiveLine(out, entry)
}

func firstSourceAsset(previous, ordinal int64) int64 {
	if previous == -1 {
		return ordinal
	}
	return previous
}

func parseSourceGraphEntry(raw []byte, scope storage.Scope, last storage.LegacySourceCursor) (sourceGraphEntry, error) {
	var entry sourceGraphEntry
	if json.Unmarshal(raw, &entry) != nil || !afterSourceRow(entry.Backend, last) || validateSourceGraphFrame(scope, entry) != nil {
		return sourceGraphEntry{}, ErrSourcePlan
	}
	return entry, nil
}

func sourceGraphInventoryMatches(info model.SourcePreflightInfo, rows, blobs, originals int64, digest string) bool {
	return rows == info.Rows && blobs == info.Blobs && originals == info.Originals && digest == info.SHA256
}
