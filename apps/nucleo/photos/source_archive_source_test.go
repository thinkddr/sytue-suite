// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type archiveBackendFixture struct {
	Source
	state  SnapshotState
	rows   []storage.StreamSourceRow
	limit  int
	tamper bool
	closed chan struct{}
}

func (b *archiveBackendFixture) State(context.Context, storage.Scope) (SnapshotState, error) {
	return b.state, nil
}

func (b *archiveBackendFixture) Page(_ context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (SnapshotPage, error) {
	page := SnapshotPage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{}}
	for _, row := range b.rows {
		if !afterSourceRow(row, after) {
			continue
		}
		if len(page.Rows) == b.limit {
			last := page.Rows[len(page.Rows)-1]
			page.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
			break
		}
		page.Rows = append(page.Rows, row)
	}
	if b.tamper && len(page.Rows) > 0 {
		page.Rows[0].SHA256 = planBytesHash([]byte("changed"))
	}
	return page, nil
}

func (b *archiveBackendFixture) Open(ctx context.Context, _ storage.Scope, _ string, _ SnapshotRow) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	go func() { <-ctx.Done(); _ = writer.CloseWithError(ctx.Err()); close(b.closed) }()
	return reader, nil
}

func archiveBackendFor(t *testing.T, header sourceArchiveHeader) *archiveBackendFixture {
	t.Helper()
	now := time.Now().UTC()
	backend := &archiveBackendFixture{limit: 7, closed: make(chan struct{}), state: SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: header.Info.Scope, Mode: "frozen", Epoch: header.Info.Epoch, StartedAt: &now, Rows: header.Info.Rows, Blobs: header.Info.Blobs}}}
	for n := 0; n < int(header.Info.Rows); n++ {
		backend.rows = append(backend.rows, archiveFixtureEntry(n).Backend)
	}
	return backend
}

func TestSourceArchiveSourcePagesExactBackendAcrossIndexBoundaries(t *testing.T) {
	store, crypto, header := sourceArchiveCacheFixture(t)
	archive, err := store.openArchive(t.Context(), crypto.ref, false)
	if err != nil {
		t.Fatal(err)
	}
	backend := archiveBackendFor(t, header)
	cursor := storage.LegacySourceCursor{}
	got := []storage.StreamSourceRow{}
	for {
		page, err := archive.page(t.Context(), backend, cursor)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, page.Rows...)
		if page.Next == nil {
			break
		}
		cursor = *page.Next
	}
	if !slices.Equal(got, backend.rows) || crypto.downloads != 1 {
		t.Fatal("archive skipped or changed a backend page", len(got), crypto.downloads)
	}
	backend.tamper = true
	if _, err := archive.page(t.Context(), backend, storage.LegacySourceCursor{}); err == nil {
		t.Fatal("changed backend row substituted into retained archive")
	}
	if _, err := archive.page(t.Context(), backend, storage.LegacySourceCursor{Kind: "album", ID: "unknown"}); err == nil {
		t.Fatal("unknown cursor skipped source records")
	}
}

func TestSourceArchiveBackendReaderKeepsLeaseUntilClose(t *testing.T) {
	store, crypto, header := sourceArchiveCacheFixture(t)
	archive, err := store.openArchive(t.Context(), crypto.ref, false)
	if err != nil {
		t.Fatal(err)
	}
	backend := archiveBackendFor(t, header)
	reader, err := archive.open(t.Context(), backend, backend.rows[0])
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-backend.closed:
		t.Fatal("reader context was cancelled before consumer closed it")
	default:
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-backend.closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel backend read and release lease")
	}
}

// This authority is deliberately a domain fixture: SDK/Synapse integration
// separately proves authenticated Drive pins and actual binary recovery.
type archivePinAuthority struct {
	captures int
	denied   bool
}

func (a *archivePinAuthority) Capture(_ context.Context, scope storage.Scope, original model.Original) (PlannedOriginal, error) {
	a.captures++
	driveScope := scope
	driveScope.Product = "drive"
	pin := drive.FilePin{Scope: driveScope, SourceKind: "file", SourceID: original.DriveID, IndexRef: storage.Reference{RoomID: "!drive:test", EventID: "$original"}, ObjectID: "original", ContentRevision: "retained", SizeBytes: original.Size, SHA256: planBytesHash([]byte{0, 1, 2, 3})}
	raw, err := json.Marshal(pin)
	return PlannedOriginal{Original: original, Descriptor: storage.StreamSourceRow{Kind: "original", ID: original.AssetID, Size: original.Size, SHA256: pin.SHA256}, Pin: raw}, err
}

func (a *archivePinAuthority) Validate(_ context.Context, scope storage.Scope, pin PlannedOriginal) error {
	if a.denied {
		return ErrNotReady
	}
	_, err := originalDrivePin(scope, pin)
	return err
}

func (a *archivePinAuthority) Write(ctx context.Context, scope storage.Scope, pin PlannedOriginal, out io.Writer) error {
	if err := a.Validate(ctx, scope, pin); err != nil {
		return err
	}
	_, err := out.Write([]byte{0, 1, 2, 3})
	return err
}

func TestSourceArchiveMergesOriginalsAndBackendAcrossBoundedPages(t *testing.T) {
	store, crypto, header := sourceArchiveCacheFixture(t)
	authority := new(archivePinAuthority)
	store.cfg.Originals = authority
	entries := make([]sourceArchiveEntry, 0, 73)
	for n := range 70 {
		id := fmt.Sprintf("%08x-0000-0000-0000-000000000001", n+1)
		original := model.Original{AssetID: id, DriveID: "file-" + id, Size: 4, ContentType: "image/png"}
		pin, err := authority.Capture(t.Context(), header.Info.Scope, original)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, sourceArchiveEntry{Backend: storage.StreamSourceRow{Kind: "asset", ID: id, Size: 500, SHA256: strings.Repeat("a", 64)}, Original: &pin})
	}
	for _, kind := range []string{"media", "variant"} {
		entries = append(entries, sourceArchiveEntry{Backend: storage.StreamSourceRow{Kind: kind, ID: strings.Repeat("b", 64), Size: 42, SHA256: strings.Repeat("c", 64)}})
	}
	sum, err := model.SourceInventoryHash(header.Info.Scope, header.Info.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if err := model.AppendSourceInventoryHash(sum, entry.Backend); err != nil {
			t.Fatal(err)
		}
	}
	header.Info.Rows = int64(len(entries))
	header.Info.Blobs = 1
	header.Info.Originals = 70
	header.Info.SHA256 = hex.EncodeToString(sum.Sum(nil))
	header.Counts = SourcePlanCounts{BackendRows: header.Info.Rows, BackendBlobs: 1, OriginalRows: 140, OriginalBlobs: 70}
	var raw bytes.Buffer
	next := 0
	if err := encodeSourceArchive(t.Context(), &raw, header, func() (sourceArchiveEntry, bool, error) {
		if next == len(entries) {
			return sourceArchiveEntry{}, false, nil
		}
		entry := entries[next]
		next++
		return entry, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	crypto.raw = raw.Bytes()
	archive, err := store.openArchive(t.Context(), crypto.ref, false)
	if err != nil {
		t.Fatal(err)
	}
	backend := archiveBackendFor(t, header)
	backend.rows = nil
	expected := make([]storage.StreamSourceRow, 0, len(entries)+140)
	for _, entry := range entries {
		backend.rows = append(backend.rows, entry.Backend)
		expected = append(expected, entry.Backend)
		if entry.Original != nil {
			metadata, body, err := generatedOriginalMetadata(header.Info.Scope, *entry.Original)
			clear(body)
			if err != nil {
				t.Fatal(err)
			}
			expected = append(expected, metadata, entry.Original.Descriptor)
		}
	}
	slices.SortFunc(expected, sourceRowCompare)
	got := []storage.StreamSourceRow{}
	cursor := storage.LegacySourceCursor{}
	for {
		page, err := archive.page(t.Context(), backend, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Rows) > storage.MaxLegacySourceRows {
			t.Fatal("unbounded page")
		}
		got = append(got, page.Rows...)
		if page.Next == nil {
			break
		}
		cursor = *page.Next
	}
	if !slices.Equal(got, expected) {
		t.Fatal("merged inventory skipped or duplicated rows", len(got), len(expected))
	}
	for _, row := range expected {
		if row.Kind != "original" {
			continue
		}
		reader, err := archive.open(t.Context(), backend, row)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil || !bytes.Equal(body, []byte{0, 1, 2, 3}) {
			t.Fatal("pinned original changed", readErr, closeErr)
		}
		break
	}
	authority.denied = true
	if err := archive.validateOrigins(t.Context()); err == nil {
		t.Fatal("revoked original pin passed pre-freeze validation")
	}
}
