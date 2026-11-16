// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type pinnedCrypto struct {
	*queryRecords
	indexes            map[storage.Reference]storage.ObjectVersion
	requests           map[storage.Reference]storage.FileRequest
	media              map[storage.Reference][]byte
	lookups, downloads int
	lookup             func(int)
	reading            func()
}

func TestPinnedIndexRejectsUncommittedPreparedRevision(t *testing.T) {
	r, _, _ := pinFixture(t, "file")
	current := r.heads[objectID("file", "original")][0]
	commit := current.Commit
	commit.Parents = []storage.Reference{current.Reference}
	raw, err := json.Marshal(storage.AtomicPrepared{Kind: storage.ObjectPreparedKind, BatchID: "pending", Commit: commit})
	if err != nil {
		t.Fatal(err)
	}
	ref := storage.Reference{RoomID: current.Reference.RoomID, EventID: "$orphan"}
	orphan := storage.RecordEntry{Reference: ref, Record: storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: current.ObjectID, RevisionID: "prepared", Role: "index", Previous: []string{current.Reference.EventID}, Payload: raw}}
	q := r.cfg.Crypto.(*pinnedCrypto).queryRecords
	q.rawExtra = []storage.RecordEntry{orphan}
	ctx, done, err := r.start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if _, err := r.pinnedIndex(ctx, FilePin{SourceKind: "file", SourceID: "original", IndexRef: ref}); !errors.Is(err, ErrSource) {
		t.Fatalf("uncommitted prepared index was accepted: %v", err)
	}
}

func (c *pinnedCrypto) StorageObject(_ context.Context, ref storage.Reference, object string) (storage.ObjectVersion, error) {
	index, found := c.indexes[ref]
	if !found || index.ObjectID != object {
		return storage.ObjectVersion{}, ErrSource
	}
	return index, nil
}

func (c *pinnedCrypto) StorageFileInfo(_ context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, error) {
	c.lookups++
	if c.lookup != nil {
		c.lookup(c.lookups)
	}
	request, found := c.requests[ref]
	if !found || request.ObjectID != object || request.RevisionID != revision {
		return storage.FileRequest{}, ErrSource
	}
	return request, nil
}

func (c *pinnedCrypto) StorageFileCommitment(ctx context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, string, error) {
	request, err := c.StorageFileInfo(ctx, ref, object, revision)
	return request, digest(c.media[ref]), err
}

func (c *pinnedCrypto) DownloadStorageFile(ctx context.Context, _ *api.Drive, ref storage.Reference, object, revision string, output io.Writer) (storage.FileRequest, error) {
	c.downloads++
	if c.reading != nil {
		c.reading()
	}
	request, err := c.StorageFileInfo(ctx, ref, object, revision)
	if err != nil {
		return storage.FileRequest{}, err
	}
	_, err = output.Write(c.media[ref])
	return request, err
}

func pinFixture(t *testing.T, kind string) (*Repository, *pinnedCrypto, []byte) {
	t.Helper()
	r := queryFixture(t, 0)
	original := []byte("retained private original")
	value := map[string]any{"id": "original", "name": "Original", "folder_id": "root", "file_id": "parent", "version": 1, "size_bytes": len(original), "updated_at": time.Now().UTC()}
	addQueryRow(t, r, kind, "original", value)
	index := r.heads[objectID(kind, "original")][0]
	q := r.cfg.Crypto.(*queryRecords)
	dataRef := index.Commit.Content.Reference
	var data entry
	record := q.records[dataRef]
	if err := json.Unmarshal(record.Payload, &data); err != nil {
		t.Fatal(err)
	}
	bodyRef := storage.Reference{RoomID: "!private:test", EventID: "$retained_body"}
	data.Binary = &bodyPointer{ObjectID: bodyID(kind, "original", "binary"), Reference: bodyRef}
	record.Payload, _ = json.Marshal(data)
	q.records[dataRef] = record
	mediaRef := storage.Reference{RoomID: "!private:test", EventID: "$retained_media"}
	metadata, _ := json.Marshal(bodySummary{Kind: bodyKind, SourceKind: kind, SourceID: "original", Purpose: "binary", SizeBytes: int64(len(original)), SHA256: digest(original)})
	body := storage.ObjectVersion{ObjectID: data.Binary.ObjectID, Reference: bodyRef, RevisionID: "body-index", Commit: storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: metadata, Content: &storage.ContentPointer{Kind: "file", Reference: mediaRef, RevisionID: "media-original"}}}
	crypto := &pinnedCrypto{queryRecords: q, indexes: map[storage.Reference]storage.ObjectVersion{bodyRef: body}, requests: map[storage.Reference]storage.FileRequest{mediaRef: {ObjectID: body.ObjectID, RevisionID: "media-original", SizeBytes: int64(len(original)), Metadata: []byte(`{}`)}}, media: map[storage.Reference][]byte{mediaRef: original}}
	q.extra = crypto.indexes
	r.cfg.Crypto = crypto
	return r, crypto, original
}

func TestPinnedOriginalRemainsExactAfterHeadMovesOrConflicts(t *testing.T) {
	for _, kind := range []string{"file", "version"} {
		t.Run(kind, func(t *testing.T) {
			r, crypto, original := pinFixture(t, kind)
			pin, err := r.PinRetainedOriginal(t.Context(), kind, "original")
			if err != nil || !pin.Valid() || crypto.downloads != 0 {
				t.Fatal("pin must authenticate metadata without downloading media", pin, err)
			}
			before := r.heads[objectID(kind, "original")][0]
			other := before
			other.Reference.EventID = "$competing_head"
			other.RevisionID = "competing-index"
			r.heads[before.ObjectID] = []storage.ObjectVersion{before, other}
			if err := r.ValidatePinnedOriginal(t.Context(), pin); err != nil || crypto.downloads != 0 {
				t.Fatal("saved pin validation followed head or downloaded media", err)
			}
			if _, err := r.PinRetainedOriginal(t.Context(), kind, "original"); !errors.Is(err, ErrConflict) {
				t.Fatal("ambiguous selection accepted", err)
			}
			var output bytes.Buffer
			if err := r.WritePinnedOriginal(t.Context(), pin, &output); err != nil || !bytes.Equal(output.Bytes(), original) {
				t.Fatal("known pin followed a changed head", err)
			}
		})
	}
}

func TestPinnedOriginalRejectsEveryChangedBindingBeforeMedia(t *testing.T) {
	r, crypto, _ := pinFixture(t, "file")
	pin, err := r.PinRetainedOriginal(t.Context(), "file", "original")
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*FilePin){
		"tenant": func(p *FilePin) { p.Scope.Tenant = "other" }, "subject": func(p *FilePin) { p.Scope.Subject = "other" }, "product": func(p *FilePin) { p.Scope.Product = "photos" },
		"kind": func(p *FilePin) { p.SourceKind = "version" }, "id": func(p *FilePin) { p.SourceID = "other" }, "epoch": func(p *FilePin) { p.Epoch = readerEpoch },
		"index": func(p *FilePin) { p.IndexRef.EventID = "$other" }, "object": func(p *FilePin) { p.ObjectID = "other-object" }, "revision": func(p *FilePin) { p.ContentRevision = "other-revision" },
		"size": func(p *FilePin) { p.SizeBytes++ }, "digest": func(p *FilePin) { p.SHA256 = digest([]byte("other")) },
	} {
		t.Run(name, func(t *testing.T) {
			bad := pin
			change(&bad)
			if err := r.ValidatePinnedOriginal(t.Context(), bad); err == nil || crypto.downloads != 0 {
				t.Fatal("changed pin validated", err)
			}
			var output bytes.Buffer
			if err := r.WritePinnedOriginal(t.Context(), bad, &output); err == nil || output.Len() != 0 || crypto.downloads != 0 {
				t.Fatal("changed pin read media", err)
			}
		})
	}
}

type deniedPinnedCommitment struct {
	*pinnedCrypto
	denied error
}

func (c deniedPinnedCommitment) StorageFileCommitment(context.Context, storage.Reference, string, string) (storage.FileRequest, string, error) {
	return storage.FileRequest{}, "", c.denied
}

func TestValidatePinnedOriginalKeepsLeaseAndRejectsRevokedSDKWithoutMedia(t *testing.T) {
	r, crypto, _ := pinFixture(t, "file")
	pin, err := r.PinRetainedOriginal(t.Context(), "file", "original")
	if err != nil {
		t.Fatal(err)
	}
	var active atomic.Int32
	var revoked atomic.Bool
	leaseDenied := errors.New("synthetic revoked owner lease")
	r.gate = offline.NewWorkGateWithLease(r.ctx, func(context.Context) (func(), error) {
		if revoked.Load() {
			return nil, leaseDenied
		}
		active.Add(1)
		return func() { active.Add(-1) }, nil
	})
	crypto.lookup = func(int) {
		if active.Load() != 1 {
			t.Error("pin validation escaped private owner lease")
		}
	}
	crypto.reading = func() { t.Error("metadata validation downloaded provider media") }
	if err := r.ValidatePinnedOriginal(t.Context(), pin); err != nil || crypto.downloads != 0 || active.Load() != 0 {
		t.Fatal("validation did not finish scoped lease", err)
	}
	deviceDenied := errors.New("synthetic revoked SDK device")
	r.cfg.Crypto = deniedPinnedCommitment{pinnedCrypto: crypto, denied: deviceDenied}
	if err := r.ValidatePinnedOriginal(t.Context(), pin); !errors.Is(err, deviceDenied) || crypto.downloads != 0 || active.Load() != 0 {
		t.Fatal("revoked SDK device validated", err)
	}
	r.cfg.Crypto = crypto
	revoked.Store(true)
	before := crypto.lookups
	if err := r.ValidatePinnedOriginal(t.Context(), pin); !errors.Is(err, leaseDenied) || crypto.lookups != before || active.Load() != 0 {
		t.Fatal("revoked lease reached SDK", err)
	}
	r.Close()
	if err := r.ValidatePinnedOriginal(t.Context(), pin); !errors.Is(err, ErrClosed) {
		t.Fatal("closed repository validated a pin", err)
	}
}

func TestPinnedOriginalRejectsBodySubstitutionBetweenValidationAndRead(t *testing.T) {
	r, crypto, _ := pinFixture(t, "file")
	pin, err := r.PinRetainedOriginal(t.Context(), "file", "original")
	if err != nil {
		t.Fatal(err)
	}
	crypto.lookups = 0
	crypto.lookup = func(n int) {
		if n != 2 {
			return
		}
		for _, body := range crypto.indexes {
			ref := body.Commit.Content.Reference
			request := crypto.requests[ref]
			request.RevisionID = "different-media"
			crypto.requests[ref] = request
		}
	}
	var output bytes.Buffer
	if err := r.WritePinnedOriginal(t.Context(), pin, &output); err == nil || output.Len() != 0 || crypto.downloads != 0 {
		t.Fatal("pinned body was substituted during read", err)
	}
}

func TestPinnedOriginalKeepsTheSessionLeaseAcrossTheFullRead(t *testing.T) {
	r, crypto, original := pinFixture(t, "file")
	var active atomic.Int32
	r.gate = offline.NewWorkGateWithLease(r.ctx, func(context.Context) (func(), error) { active.Add(1); return func() { active.Add(-1) }, nil })
	pin, err := r.PinRetainedOriginal(t.Context(), "file", "original")
	if err != nil {
		t.Fatal(err)
	}
	crypto.reading = func() {
		if active.Load() != 1 {
			t.Error("private session lease ended before body read")
		}
	}
	var output bytes.Buffer
	if err := r.WritePinnedOriginal(t.Context(), pin, &output); err != nil || !bytes.Equal(output.Bytes(), original) || active.Load() != 0 {
		t.Fatal("pinned read did not finish its originating lease", err)
	}
	r.ready = false
	if _, err := r.PinRetainedOriginal(t.Context(), "file", "original"); !errors.Is(err, ErrNotReady) {
		t.Fatal("uninitialized catalog issued a pin", err)
	}
	if err := r.WritePinnedOriginal(t.Context(), pin, io.Discard); !errors.Is(err, ErrNotReady) {
		t.Fatal("uninitialized catalog reopened a pin", err)
	}
}
