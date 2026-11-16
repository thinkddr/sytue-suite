// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func archiveFixtureEntry(n int) sourceArchiveEntry {
	return sourceArchiveEntry{Backend: storage.StreamSourceRow{Kind: "album", ID: fmt.Sprintf("%08x-0000-0000-0000-000000000001", n+1), Size: 1024, SHA256: strings.Repeat("a", 64)}}
}

func archiveFixtureHeader(t *testing.T, n int) sourceArchiveHeader {
	t.Helper()
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
	epoch := "12345678-1234-1234-1234-123456789abc"
	sum, err := model.SourceInventoryHash(scope, epoch)
	if err != nil {
		t.Fatal(err)
	}
	for i := range n {
		if err := model.AppendSourceInventoryHash(sum, archiveFixtureEntry(i).Backend); err != nil {
			t.Fatal(err)
		}
	}
	return sourceArchiveHeader{Kind: sourceArchiveKind, Info: model.SourcePreflightInfo{Scope: scope, Epoch: epoch, CAS: epoch, SHA256: hex.EncodeToString(sum.Sum(nil)), Rows: int64(n)}, Counts: SourcePlanCounts{BackendRows: int64(n)}}
}

func archiveFixtureNext(n int) func() (sourceArchiveEntry, bool, error) {
	i := 0
	return func() (sourceArchiveEntry, bool, error) {
		if i == n {
			return sourceArchiveEntry{}, false, nil
		}
		entry := archiveFixtureEntry(i)
		i++
		return entry, true, nil
	}
}

func TestSourceArchiveStreamsAbove32MiBIntoExistingEncryptedStaging(t *testing.T) {
	const entries = 220000
	header := archiveFixtureHeader(t, entries)
	journal, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close() }()
	encoded := make(chan error, 1)
	go func() {
		err := encodeSourceArchive(t.Context(), writer, header, archiveFixtureNext(entries))
		_ = writer.CloseWithError(err)
		encoded <- err
	}()
	file, err := journal.StageStream(t.Context(), "acme\x00alice\x00photos", "photos-private-stage-v1", reader, storage.MaxFileBytes)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := <-encoded; err != nil {
		t.Fatal(err)
	}
	if file.SizeBytes <= 32<<20 {
		t.Fatal("regression archive did not exceed old aggregate cap", file.SizeBytes)
	}
	staged, err := journal.OpenStagedFile(t.Context(), "acme\x00alice\x00photos", "photos-private-stage-v1", file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = staged.Close() }()
	count, maxLine := 0, 0
	var previous int64
	got, err := walkSourceArchive(t.Context(), io.NewSectionReader(staged, 0, file.SizeBytes), header.Info.Scope, header.Info.Epoch, func(actual sourceArchiveHeader, entry sourceArchiveEntry, offset int64, size int) error {
		if actual != header || entry.Backend != archiveFixtureEntry(count).Backend || offset <= previous {
			return ErrSourcePlan
		}
		previous = offset
		count++
		maxLine = max(maxLine, size)
		return nil
	})
	if err != nil || got != header || count != entries || maxLine > maxSourceArchiveLine {
		t.Fatal("streamed archive changed or accumulated rows", count, maxLine, err)
	}
	// Exercise the same encrypted paged index used by the production view;
	// neither the encoder nor this index builder accumulates the full inventory.
	store := &SourcePlanStore{cfg: SourcePlanConfig{Scope: header.Info.Scope, Journal: journal}}
	cache := sourceArchiveCache{Scope: header.Info.Scope, Ref: SourcePlanReference{Epoch: header.Info.Epoch}, File: file, AssetStart: -1}
	if err := store.indexArchive(t.Context(), "large_archive_index", &cache); err != nil {
		t.Fatal(err)
	}
	view := &sourceArchive{store: store, key: "large_archive_index", cache: cache}
	for _, ordinal := range []int64{0, 63, 64, entries - 1} {
		index, err := view.index(t.Context(), ordinal)
		if err != nil || index.Backend != archiveFixtureEntry(int(ordinal)).Backend {
			t.Fatal("large index changed a descriptor", ordinal, err)
		}
	}
	var first sourceArchiveIndexPage
	if found, err := journal.Read(store.owner(), sourceArchiveNamespace, archiveIndexKey(view.key, 0), &first); err != nil || !found || len(first.Entries) != sourceArchiveIndexRows {
		t.Fatal("large archive did not keep a bounded index page", len(first.Entries), err)
	}
	t.Log("PHOTOS_ARCHIVE_STREAM_PASS", file.SizeBytes, "bytes", count, "entries; maximum decoded line", maxLine)
}

func TestSourceArchiveRejectsIncompleteOrChangedInventory(t *testing.T) {
	header := archiveFixtureHeader(t, 3)
	var data bytes.Buffer
	if err := encodeSourceArchive(t.Context(), &data, header, archiveFixtureNext(3)); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"truncated", "missing row", "wrong inventory", "wrong scope", "wrong epoch"} {
		t.Run(change, func(t *testing.T) {
			raw := bytes.Clone(data.Bytes())
			scope, epoch := header.Info.Scope, header.Info.Epoch
			switch change {
			case "truncated":
				raw = raw[:len(raw)-1]
			case "missing row":
				position := bytes.LastIndexByte(raw[:len(raw)-1], '\n')
				raw = raw[:position+1]
			case "wrong inventory":
				raw = bytes.Replace(raw, []byte(header.Info.SHA256), []byte(strings.Repeat("1", 64)), 1)
			case "wrong scope":
				scope.Subject = "bob"
			case "wrong epoch":
				epoch = "22345678-1234-1234-1234-123456789abc"
			}
			if _, err := walkSourceArchive(t.Context(), bytes.NewReader(raw), scope, epoch, nil); err == nil {
				t.Fatal("changed archive accepted")
			}
		})
	}
	if err := encodeSourceArchive(t.Context(), io.Discard, header, archiveFixtureNext(2)); err == nil {
		t.Fatal("incomplete archive was declared complete")
	}
	if err := encodeSourceArchive(t.Context(), shortArchiveWriter{}, header, archiveFixtureNext(3)); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("short writer accepted", err)
	}
}

type shortArchiveWriter struct{}

func (shortArchiveWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
