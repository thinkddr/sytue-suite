// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	sourceArchiveKind = "org.sytue.photos.source-plan.v2"
	// A line contains one descriptor and, for an asset, its existing Drive pin
	// (max16KiB). The complete archive has no aggregate JSON cap: the maintained
	// SDK already pages private files, and offline staging handles bounded blocks.
	maxSourceArchiveLine = 64 << 10
)

type sourceArchiveHeader struct {
	Kind   string                    `json:"kind"`
	Info   model.SourcePreflightInfo `json:"source"`
	Counts SourcePlanCounts          `json:"counts"`
}

func (h sourceArchiveHeader) validate(scope storage.Scope, epoch string) error {
	if h.Kind != sourceArchiveKind || h.Info.Validate(scope, epoch) != nil || h.Counts.BackendRows != h.Info.Rows || h.Counts.BackendBlobs != h.Info.Blobs || h.Counts.OriginalBlobs != h.Info.Originals || h.Counts.OriginalRows != 2*h.Info.Originals {
		return ErrSourcePlan
	}
	_, _, err := h.Counts.Totals()
	return err
}

type sourceArchiveEntry struct {
	Backend  storage.StreamSourceRow `json:"backend"`
	Original *PlannedOriginal        `json:"original,omitempty"`
}

func (e sourceArchiveEntry) validate(scope storage.Scope) error {
	if !backendPlanKind(e.Backend.Kind) || e.Backend.Validate() != nil {
		return ErrSourcePlan
	}
	if e.Backend.Kind != "asset" {
		if e.Original != nil {
			return ErrSourcePlan
		}
		return nil
	}
	if e.Original == nil || e.Original.Original.AssetID != e.Backend.ID {
		return ErrSourcePlan
	}
	_, err := originalDrivePin(scope, *e.Original)
	return err
}

type sourceArchiveWalk struct {
	header                 sourceArchiveHeader
	sum                    hash.Hash
	last                   storage.LegacySourceCursor
	rows, blobs, originals int64
}

func newSourceArchiveWalk(header sourceArchiveHeader) (*sourceArchiveWalk, error) {
	if err := header.validate(header.Info.Scope, header.Info.Epoch); err != nil {
		return nil, err
	}
	sum, err := model.SourceInventoryHash(header.Info.Scope, header.Info.Epoch)
	if err != nil {
		return nil, err
	}
	return &sourceArchiveWalk{header: header, sum: sum}, nil
}

func (w *sourceArchiveWalk) append(entry sourceArchiveEntry) error {
	if entry.validate(w.header.Info.Scope) != nil || !afterSourceRow(entry.Backend, w.last) || w.rows >= w.header.Info.Rows {
		return ErrSourcePlan
	}
	if err := model.AppendSourceInventoryHash(w.sum, entry.Backend); err != nil {
		return err
	}
	w.rows++
	if entry.Backend.Kind == "variant" {
		w.blobs++
	}
	if entry.Original != nil {
		w.originals++
	}
	w.last = storage.LegacySourceCursor{Kind: entry.Backend.Kind, ID: entry.Backend.ID}
	return nil
}

func (w *sourceArchiveWalk) finish() error {
	if w.rows != w.header.Info.Rows || w.blobs != w.header.Info.Blobs || w.originals != w.header.Info.Originals || hex.EncodeToString(w.sum.Sum(nil)) != w.header.Info.SHA256 {
		return ErrSourcePlan
	}
	return nil
}

func encodeSourceArchive(ctx context.Context, out io.Writer, header sourceArchiveHeader, next func() (sourceArchiveEntry, bool, error)) error {
	if out == nil || next == nil {
		return ErrSourcePlan
	}
	walk, err := newSourceArchiveWalk(header)
	if err != nil {
		return err
	}
	if err := writeSourceArchiveLine(out, header); err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry, found, err := next()
		if err != nil {
			return err
		}
		if !found {
			break
		}
		if err := walk.append(entry); err != nil {
			return err
		}
		if err := writeSourceArchiveLine(out, entry); err != nil {
			return err
		}
	}
	return walk.finish()
}

func writeSourceArchiveLine(out io.Writer, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	defer clear(raw)
	if len(raw)+1 > maxSourceArchiveLine {
		return ErrSourcePlan
	}
	if n, err := out.Write(raw); err != nil {
		return err
	} else if n != len(raw) {
		return io.ErrShortWrite
	}
	n, err := out.Write([]byte{'\n'})
	if err != nil {
		return err
	}
	if n != 1 {
		return io.ErrShortWrite
	}
	return nil
}

// walkSourceArchive authenticates structure, order, counts and the complete
// preflight inventory. The caller must first authenticate the complete SDK
// file commitment and keep its work lease. This does not prove original media
// availability. Visitor pins alias this iteration only; clone if retained.
func walkSourceArchive(ctx context.Context, input io.Reader, scope storage.Scope, epoch string, visit func(sourceArchiveHeader, sourceArchiveEntry, int64, int) error) (sourceArchiveHeader, error) {
	if input == nil {
		return sourceArchiveHeader{}, ErrSourcePlan
	}
	reader := bufio.NewReaderSize(input, maxSourceArchiveLine)
	raw, err := reader.ReadSlice('\n')
	if err != nil {
		return sourceArchiveHeader{}, ErrSourcePlan
	}
	var header sourceArchiveHeader
	if json.Unmarshal(raw, &header) != nil || header.validate(scope, epoch) != nil {
		return sourceArchiveHeader{}, ErrSourcePlan
	}
	offset := int64(len(raw))
	clear(raw)
	walk, err := newSourceArchiveWalk(header)
	if err != nil {
		return sourceArchiveHeader{}, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return sourceArchiveHeader{}, err
		}
		raw, err := reader.ReadSlice('\n')
		if errors.Is(err, io.EOF) && len(raw) == 0 {
			break
		}
		if err != nil {
			return sourceArchiveHeader{}, ErrSourcePlan
		}
		if err := visitSourceArchiveLine(raw, walk, offset, visit); err != nil {
			return sourceArchiveHeader{}, err
		}
		offset += int64(len(raw))
	}
	if err := walk.finish(); err != nil {
		return sourceArchiveHeader{}, err
	}
	return header, nil
}

func visitSourceArchiveLine(raw []byte, walk *sourceArchiveWalk, offset int64, visit func(sourceArchiveHeader, sourceArchiveEntry, int64, int) error) error {
	defer clear(raw)
	var entry sourceArchiveEntry
	if json.Unmarshal(raw, &entry) != nil {
		return ErrSourcePlan
	}
	if entry.Original != nil {
		defer clear(entry.Original.Pin)
	}
	if err := walk.append(entry); err != nil {
		return err
	}
	if visit != nil {
		return visit(walk.header, entry, offset, len(raw))
	}
	return nil
}
