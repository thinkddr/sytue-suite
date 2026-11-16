// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type auditFile struct {
	request storage.FileRequest
	sum     string
	media   []byte
}

type auditCrypto struct {
	*catalogTransferCrypto
	files     map[storage.Reference]*auditFile
	reads     map[storage.Reference]int
	onRead    func()
	onHistory func(int)
	histories int
	failAt    int
	failure   error
	waitRead  bool
}

func auditTestBudget() AuditBudget {
	return AuditBudget{MaxDuration: time.Minute, MaxRevisions: 1000, MaxBytes: 1 << 20}
}

func (c *auditCrypto) StorageObjectHistory(ctx context.Context, room, cursor string, n int) (storage.ObjectPage, error) {
	c.histories++
	if c.onHistory != nil {
		c.onHistory(c.histories)
	}
	if c.histories == c.failAt {
		return storage.ObjectPage{}, c.failure
	}
	if err := ctx.Err(); err != nil {
		return storage.ObjectPage{}, err
	}
	return c.catalogTransferCrypto.StorageObjectHistory(ctx, room, cursor, n)
}

func (c *auditCrypto) StorageFileCommitment(ctx context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, string, error) {
	if err := ctx.Err(); err != nil {
		return storage.FileRequest{}, "", err
	}
	f := c.files[ref]
	if f == nil || f.request.ObjectID != object || f.request.RevisionID != revision {
		return storage.FileRequest{}, "", ErrSource
	}
	return f.request, f.sum, nil
}

func (c *auditCrypto) StorageFileInfo(ctx context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, error) {
	request, _, err := c.StorageFileCommitment(ctx, ref, object, revision)
	return request, err
}

func (c *auditCrypto) DownloadStorageFile(ctx context.Context, _ *api.Drive, ref storage.Reference, object, revision string, out io.Writer) (storage.FileRequest, error) {
	request, _, err := c.StorageFileCommitment(ctx, ref, object, revision)
	if err != nil {
		return storage.FileRequest{}, err
	}
	c.reads[ref]++
	if c.onRead != nil {
		c.onRead()
	}
	if c.waitRead {
		<-ctx.Done()
		return storage.FileRequest{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return storage.FileRequest{}, err
	}
	if c.files[ref].media == nil {
		return storage.FileRequest{}, errors.New("provider object missing")
	}
	_, err = out.Write(c.files[ref].media)
	return request, err
}

func auditFixture(t *testing.T) (*catalogTransferFixture, *auditCrypto, []storage.Reference) {
	t.Helper()
	f := newCatalogTransferFixture(t)
	var branches []storage.Reference
	for _, v := range f.versions {
		if v.RevisionID == "child-trash-index" || v.RevisionID == "child-delete-index" {
			branches = append(branches, v.Reference)
		}
	}
	if len(branches) != 2 {
		t.Fatal("fixture lacks both branches")
	}
	f.addFolder(t, f.child, "child-merge", "merged", false, "", branches)
	c := &auditCrypto{catalogTransferCrypto: f.old, files: map[storage.Reference]*auditFile{}, reads: map[storage.Reference]int{}}
	f.source.cfg.Crypto = c
	oldBody, oldMedia := addAuditBody(t, f, c, "file", "one", "binary", "old", []byte("old bytes"), nil)
	newBody, newMedia := addAuditBody(t, f, c, "file", "one", "binary", "new", []byte("new bytes"), []storage.Reference{oldBody.Reference})
	addAuditItem(t, f, "file", "one", "file-old", oldBody, []byte("old bytes"), false, nil)
	addAuditItem(t, f, "file", "one", "file-trash", newBody, []byte("new bytes"), true, []storage.Reference{lastAuditItem(t, f, "file", "one")})
	// A distinct version entry retains the old bytes by an authenticated origin.
	addAuditItem(t, f, "version", "snapshot", "version", oldBody, []byte("old bytes"), false, nil)
	metadata := []byte(`{"id":"metadata-folder","name":"external","is_root":false,"updated_at":"2026-10-29T21:00:00Z"}`)
	metaBody, metaMedia := addAuditBody(t, f, c, "folder", "metadata-folder", "metadata", "meta", metadata, nil)
	item, err := summarize(storage.LegacySourceRow{Kind: "folder", ID: "metadata-folder", Payload: metadata}, "")
	if err != nil {
		t.Fatal(err)
	}
	entryBytes, _ := json.Marshal(entry{Kind: entryKind, SourceKind: "folder", SourceID: "metadata-folder", Metadata: &bodyPointer{ObjectID: metaBody.ObjectID, Reference: metaBody.Reference}})
	itemBytes, _ := json.Marshal(item)
	f.addRecordVersion(t, objectID("folder", "metadata-folder"), "external-folder", itemBytes, entryBytes, "live", nil)
	// Reads validate against the repository's immutable observation, which a
	// stale view refuses (as in the transfer scan); observe the complete fixture.
	if err := f.source.refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	return f, c, []storage.Reference{oldMedia, newMedia, metaMedia}
}

func addAuditBody(t *testing.T, f *catalogTransferFixture, c *auditCrypto, kind, id, purpose, revision string, media []byte, parents []storage.Reference) (storage.ObjectVersion, storage.Reference) {
	t.Helper()
	object := bodyID(kind, id, purpose)
	ref := storage.Reference{RoomID: f.old.binding.Target.RoomID, EventID: "$media-" + revision}
	request := storage.FileRequest{ObjectID: object, RevisionID: "media-" + revision, SizeBytes: int64(len(media)), Metadata: []byte(`{}`)}
	c.files[ref] = &auditFile{request: request, sum: digest(media), media: bytes.Clone(media)}
	value, _ := json.Marshal(bodySummary{Kind: bodyKind, SourceKind: kind, SourceID: id, Purpose: purpose, SHA256: digest(media), SizeBytes: int64(len(media))})
	commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: value, Content: &storage.ContentPointer{Kind: "file", Reference: ref, RevisionID: request.RevisionID}, Parents: parents}
	version, err := f.old.PutStorageObjectAt(t.Context(), f.old.binding.Target, "audit-"+revision, object, "index-"+revision, commit)
	if err != nil {
		t.Fatal(err)
	}
	return version, ref
}

func addAuditItem(t *testing.T, f *catalogTransferFixture, kind, id, revision string, body storage.ObjectVersion, media []byte, trash bool, parents []storage.Reference) {
	t.Helper()
	fields := map[string]any{"id": id, "name": id, "folder_id": "root", "file_id": "one", "version": 1, "size_bytes": len(media), "checksum": digest(media), "updated_at": "2026-10-29T21:00:00Z"}
	state := "live"
	if trash {
		fields["deleted_at"] = "2026-10-30T21:00:00Z"
		state = "trash"
	}
	raw, _ := json.Marshal(fields)
	item, err := summarize(storage.LegacySourceRow{Kind: kind, ID: id, Payload: raw}, "")
	if err != nil {
		t.Fatal(err)
	}
	pointer := &bodyPointer{ObjectID: body.ObjectID, Reference: body.Reference}
	if kind != "file" || id != "one" {
		pointer.Origin = bodyOrigin{Kind: "file", ID: "one"}
	}
	payload, _ := json.Marshal(entry{Kind: entryKind, SourceKind: kind, SourceID: id, Raw: raw, Binary: pointer})
	metadata, _ := json.Marshal(item)
	f.addRecordVersion(t, objectID(kind, id), revision, metadata, payload, state, parents)
}

func lastAuditItem(t *testing.T, f *catalogTransferFixture, kind, id string) storage.Reference {
	t.Helper()
	for i := len(f.versions) - 1; i >= 0; i-- {
		if f.versions[i].ObjectID == objectID(kind, id) {
			return f.versions[i].Reference
		}
	}
	t.Fatal("missing fixture item")
	return storage.Reference{}
}

func TestAuditBinariesReadsEveryRetainedBodyAndKeepsSource(t *testing.T) {
	f, c, media := auditFixture(t)
	writes := c.writes
	result, err := f.source.AuditBinaries(t.Context(), auditTestBudget())
	if err != nil || result.Scope != readerScope || result.Objects != 3 || result.Bytes != int64(len(c.files[media[0]].media)+len(c.files[media[1]].media)+len(c.files[media[2]].media)) {
		t.Fatal("incomplete retained audit", result, err)
	}
	for _, ref := range media {
		if c.reads[ref] == 0 {
			t.Fatal("retained provider content not read", ref)
		}
	}
	if c.writes != writes || f.metadata.pages != 0 || f.metadata.opens != 0 || f.metadata.begins != 0 {
		t.Fatal("audit modified catalog or used legacy source")
	}
}

func TestAuditBinariesRejectsMissingCorruptAndTruncatedProviderBodies(t *testing.T) {
	for _, which := range []int{0, 1, 2} {
		for _, mode := range []string{"missing", "corrupt", "short"} {
			t.Run(mode+string(rune('0'+which)), func(t *testing.T) {
				f, c, media := auditFixture(t)
				file := c.files[media[which]]
				switch mode {
				case "missing":
					file.media = nil
				case "corrupt":
					file.media[len(file.media)-1] ^= 1
				case "short":
					file.media = file.media[:len(file.media)-1]
				}
				// Commitments remain intact even though provider bytes changed.
				if _, _, err := c.StorageFileCommitment(t.Context(), media[which], file.request.ObjectID, file.request.RevisionID); err != nil {
					t.Fatal(err)
				}
				result, err := f.source.AuditBinaries(t.Context(), auditTestBudget())
				if err == nil || result != (BinaryAudit{}) {
					t.Fatal("missing or damaged body accepted", result, err)
				}
			})
		}
	}
}

func TestAuditBinariesRejectsExhaustedBudgetsBeforeMedia(t *testing.T) {
	for name, budget := range map[string]AuditBudget{
		"missing duration":  {MaxRevisions: 1000, MaxBytes: 1 << 20},
		"missing revisions": {MaxDuration: time.Minute, MaxBytes: 1 << 20},
		"missing bytes":     {MaxDuration: time.Minute, MaxRevisions: 1000},
		"revision limit":    {MaxDuration: time.Minute, MaxRevisions: 1, MaxBytes: 1 << 20},
		"byte limit":        {MaxDuration: time.Minute, MaxRevisions: 1000, MaxBytes: 1},
	} {
		t.Run(name, func(t *testing.T) {
			f, c, _ := auditFixture(t)
			result, err := f.source.AuditBinaries(t.Context(), budget)
			if !errors.Is(err, ErrAuditBudget) || result != (BinaryAudit{}) || len(c.reads) != 0 {
				t.Fatal("exhausted budget accepted or read media", result, err, c.reads)
			}
		})
	}
	t.Run("deadline", func(t *testing.T) {
		f, c, _ := auditFixture(t)
		c.waitRead = true
		budget := auditTestBudget()
		budget.MaxDuration = 50 * time.Millisecond
		result, err := f.source.AuditBinaries(t.Context(), budget)
		if !errors.Is(err, context.DeadlineExceeded) || result != (BinaryAudit{}) {
			t.Fatal("deadline returned a partial success", result, err)
		}
	})
}

func TestAuditBinariesRejectsChangedCatalogAndCancellation(t *testing.T) {
	t.Run("catalog changed", func(t *testing.T) {
		f, c, _ := auditFixture(t)
		c.onRead = func() {
			c.onRead = nil
			f.addFolder(t, "new-folder", "during-audit", "new", false, "", nil)
		}
		if result, err := f.source.AuditBinaries(t.Context(), auditTestBudget()); !errors.Is(err, ErrConflict) || result != (BinaryAudit{}) {
			t.Fatal("changed history accepted", result, err)
		}
	})
	t.Run("binding changed", func(t *testing.T) {
		f, c, _ := auditFixture(t)
		c.onRead = func() {
			c.onRead = nil
			c.binding.Target.DeviceID = "other-device"
		}
		// Item reads recheck the observed binding, so a change is refused as a
		// changed source before the final catalog comparison when an item
		// follows the first body read; otherwise that comparison reports it.
		if result, err := f.source.AuditBinaries(t.Context(), auditTestBudget()); !errors.Is(err, ErrConflict) && !errors.Is(err, ErrSource) || result != (BinaryAudit{}) {
			t.Fatal("changed binding accepted", result, err)
		}
	})
	t.Run("second catalog interrupted", func(t *testing.T) {
		f, c, _ := auditFixture(t)
		failure := errors.New("history unavailable")
		c.failAt, c.failure = 2, failure
		if result, err := f.source.AuditBinaries(t.Context(), auditTestBudget()); !errors.Is(err, failure) || result != (BinaryAudit{}) {
			t.Fatal("interrupted catalog accepted", result, err)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		f, c, _ := auditFixture(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		c.onRead = cancel
		if result, err := f.source.AuditBinaries(ctx, auditTestBudget()); !errors.Is(err, context.Canceled) || result != (BinaryAudit{}) {
			t.Fatal("cancelled audit accepted", result, err)
		}
	})
}

func TestAuditBinariesRejectsConflictAndAtomicPreparedHistory(t *testing.T) {
	t.Run("conflicting heads", func(t *testing.T) {
		f, _, _ := auditFixture(t)
		f.addFolder(t, "fork", "fork-one", "one", false, "", nil)
		f.addFolder(t, "fork", "fork-two", "two", false, "", nil)
		if result, err := f.source.AuditBinaries(t.Context(), auditTestBudget()); !errors.Is(err, ErrConflict) || result != (BinaryAudit{}) {
			t.Fatal("unresolved heads accepted", result, err)
		}
	})
	t.Run("atomic prepared", func(t *testing.T) {
		f, c, _ := auditFixture(t)
		commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "deleted", Metadata: []byte(`{}`)}
		payload, _ := json.Marshal(storage.AtomicPrepared{Kind: storage.ObjectPreparedKind, BatchID: "batch", Commit: commit})
		ref := storage.Reference{RoomID: c.binding.Target.RoomID, EventID: "$atomic"}
		c.records[ref] = storage.Record{Version: 1, Scope: readerScope, ObjectID: "prepared", RevisionID: "prepared-revision", Role: "index", Payload: payload}
		if err := storage.ValidateCatalogEntry(storage.RecordEntry{Reference: ref, Record: c.records[ref]}, readerScope); err != nil {
			t.Fatal("invalid atomic fixture", err)
		}
		if result, err := f.source.AuditBinaries(t.Context(), auditTestBudget()); err == nil || result != (BinaryAudit{}) {
			t.Fatal("atomic history accepted without atomic projection", result, err)
		}
	})
}
