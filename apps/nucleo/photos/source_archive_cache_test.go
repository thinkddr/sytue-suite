// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type archiveCryptoFixture struct {
	SourcePlanCrypto
	scope                storage.Scope
	ref                  SourcePlanReference
	root                 storage.Reference
	raw                  []byte
	sha                  string
	infoErr, downloadErr error
	downloads            int
}

func (c *archiveCryptoFixture) StorageIdentityVersion() storage.IdentityVersion {
	return storage.IsolatedIdentity
}

func (c *archiveCryptoFixture) ReadStorageRecord(_ context.Context, ref storage.Reference, object, role string) (storage.Record, error) {
	if c.infoErr != nil {
		return storage.Record{}, c.infoErr
	}
	if ref != c.ref.Link || object != sourcePlanObject(c.ref.Epoch) || role != "content" {
		return storage.Record{}, ErrSourcePlan
	}
	payload, _ := json.Marshal(storage.FileLink{Kind: storage.FileLinkKind, Content: storage.ContentPointer{Kind: "file", RevisionID: c.ref.Epoch, Reference: c.root}})
	return storage.Record{Version: 1, Scope: c.scope, ObjectID: object, RevisionID: c.ref.Epoch, Role: role, Payload: payload}, nil
}

func (c *archiveCryptoFixture) StorageFileInfo(context.Context, storage.Reference, string, string) (storage.FileRequest, error) {
	return storage.FileRequest{ObjectID: sourcePlanObject(c.ref.Epoch), RevisionID: c.ref.Epoch, SizeBytes: int64(len(c.raw)), Metadata: sourcePlanMetadata(sourceArchiveKind)}, c.infoErr
}

func (c *archiveCryptoFixture) StorageFileCommitment(ctx context.Context, root storage.Reference, object, revision string) (storage.FileRequest, string, error) {
	request, err := c.StorageFileInfo(ctx, root, object, revision)
	sha := c.sha
	if sha == "" {
		sha = planBytesHash(c.raw)
	}
	return request, sha, err
}

func (c *archiveCryptoFixture) DownloadStorageFile(ctx context.Context, _ *api.Drive, root storage.Reference, object, revision string, out io.Writer) (storage.FileRequest, error) {
	c.downloads++
	if c.downloadErr != nil {
		return storage.FileRequest{}, c.downloadErr
	}
	request, err := c.StorageFileInfo(ctx, root, object, revision)
	if err != nil {
		return request, err
	}
	_, err = io.Copy(out, bytes.NewReader(c.raw))
	return request, err
}

func sourceArchiveCacheFixture(t *testing.T) (*SourcePlanStore, *archiveCryptoFixture, sourceArchiveHeader) {
	t.Helper()
	header := archiveFixtureHeader(t, 137)
	var data bytes.Buffer
	if err := encodeSourceArchive(t.Context(), &data, header, archiveFixtureNext(137)); err != nil {
		t.Fatal(err)
	}
	journal, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	crypto := &archiveCryptoFixture{scope: header.Info.Scope, ref: SourcePlanReference{Epoch: header.Info.Epoch, Link: storage.Reference{RoomID: "!photos:test", EventID: "$archive"}}, root: storage.Reference{RoomID: "!photos:test", EventID: "$archive_file"}, raw: bytes.Clone(data.Bytes())}
	store, err := NewSourcePlanStore(SourcePlanConfig{Context: t.Context(), Scope: header.Info.Scope, Crypto: crypto, Gateway: api.NewDrive("https://fixture.invalid", nil, nil), Journal: journal})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return store, crypto, header
}

func TestSourceArchiveCacheIndexesBoundedPagesAndReauthorizesEveryHandle(t *testing.T) {
	store, crypto, header := sourceArchiveCacheFixture(t)
	archive, err := store.openArchive(t.Context(), crypto.ref, false)
	if err != nil {
		t.Fatal(err)
	}
	if actual, err := archive.header(t.Context()); err != nil || actual != header {
		t.Fatal("archive header changed", err)
	}
	for _, n := range []int64{0, 63, 64, 128, 136} {
		index, err := archive.index(t.Context(), n)
		if err != nil {
			t.Fatal(err)
		}
		entry, err := archive.entry(t.Context(), index)
		if err != nil || entry.Backend != archiveFixtureEntry(int(n)).Backend {
			t.Fatal("archive random lookup changed", n, err)
		}
	}
	if _, err := store.openArchive(t.Context(), crypto.ref, false); err != nil || crypto.downloads != 1 {
		t.Fatal("authenticated cache unnecessarily redownloaded whole plan", crypto.downloads, err)
	}
	row := archiveFixtureEntry(66).Backend
	n, found, err := archive.findIndex(t.Context(), storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID})
	if err != nil || !found || n != 67 {
		t.Fatal("cursor lookup escaped its exact archived inventory", n, found, err)
	}
	crypto.infoErr = errors.New("synthetic SDK authorization failure")
	if _, err := archive.header(t.Context()); !errors.Is(err, crypto.infoErr) {
		t.Fatal("cached handle bypassed current SDK authorization", err)
	}
	if _, err := store.openArchive(t.Context(), crypto.ref, false); !errors.Is(err, crypto.infoErr) {
		t.Fatal("cached plan masked SDK failure", err)
	}
}

func TestSourceArchiveMetadataCacheNeverBecomesAvailabilityProof(t *testing.T) {
	store, crypto, _ := sourceArchiveCacheFixture(t)
	if _, err := store.openArchive(t.Context(), crypto.ref, false); err != nil {
		t.Fatal(err)
	}
	crypto.downloadErr = errors.New("synthetic provider unavailable")
	if _, err := store.openArchive(t.Context(), crypto.ref, false); err != nil {
		t.Fatal("metadata cache unnecessarily required provider bytes", err)
	}
	if _, err := store.openArchive(t.Context(), crypto.ref, true); !errors.Is(err, crypto.downloadErr) {
		t.Fatal("metadata cache hid unavailable plan bytes", err)
	}
	crypto.downloadErr = nil
	crypto.sha = strings.Repeat("1", 64)
	if _, err := store.openArchive(t.Context(), crypto.ref, false); !errors.Is(err, ErrSourcePlan) {
		t.Fatal("changed SDK commitment reused old cache", err)
	}
}

func TestSourceArchiveCacheRejectsCorruptIndexAndCancelledLease(t *testing.T) {
	store, crypto, _ := sourceArchiveCacheFixture(t)
	archive, err := store.openArchive(t.Context(), crypto.ref, false)
	if err != nil {
		t.Fatal(err)
	}
	var page sourceArchiveIndexPage
	key := archiveIndexKey(archive.key, 0)
	if found, err := store.cfg.Journal.Read(store.owner(), sourceArchiveNamespace, key, &page); err != nil || !found {
		t.Fatal(err)
	}
	page.Entries[0].Offset = archive.cache.File.SizeBytes
	if err := store.cfg.Journal.Write(store.owner(), sourceArchiveNamespace, key, page); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.index(t.Context(), 0); !errors.Is(err, ErrSourcePlan) {
		t.Fatal("corrupt index offset read arbitrary staged bytes", err)
	}
	store.Close()
	if _, err := archive.header(t.Context()); err == nil {
		t.Fatal("closed owner lease retained archive access")
	}
}
