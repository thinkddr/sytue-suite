// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

type retryBlobStore struct {
	*fakeBlobs
	content  string
	getErr   error
	readErr  error
	closeErr error
	cancel   context.CancelFunc
	gets     int
	bytes    int
	closed   bool
}

func (s *retryBlobStore) Get(context.Context, tenancy.Tenant, string) (io.ReadCloser, error) {
	s.gets++
	if s.getErr != nil {
		return nil, s.getErr
	}
	return &retryBlobReader{reader: strings.NewReader(s.content), store: s}, nil
}

func (s *retryBlobStore) GetRaw(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error) {
	return s.Get(ctx, t, key)
}

type retryBlobReader struct {
	reader *strings.Reader
	store  *retryBlobStore
}

func (r *retryBlobReader) Read(p []byte) (int, error) {
	if r.store.readErr != nil {
		return 0, r.store.readErr
	}
	n, err := r.reader.Read(p)
	r.store.bytes += n
	if r.store.cancel != nil {
		r.store.cancel()
	}
	return n, err
}

func (r *retryBlobReader) Close() error {
	r.store.closed = true
	return r.store.closeErr
}

func committedCipherForRetry() domain.CipherBlob {
	sum := sha256.Sum256([]byte("abc"))
	return domain.CipherBlob{BlobReceipt: storage.BlobReceipt{ID: strings.Repeat("A", 26), Product: "mail", SizeBytes: 3, SHA256: hex.EncodeToString(sum[:])}, StorageKey: "opaque-prior", OwnerSub: "alice"}
}

func TestCipherBlobRetryVerifiesProviderBeforeAcknowledging(t *testing.T) {
	for _, tc := range []struct {
		name     string
		content  string
		getErr   error
		readErr  error
		closeErr error
		want     error
	}{
		{name: "intact", content: "abc"},
		{name: "missing", getErr: objectstore.ErrNotFound, want: ErrCipherBlobMissing},
		{name: "missing on lazy read", readErr: objectstore.ErrNotFound, want: ErrCipherBlobMissing},
		{name: "short", content: "ab", want: ErrCipherBlobCorrupt},
		{name: "altered", content: "xyz", want: ErrCipherBlobCorrupt},
		{name: "long", content: strings.Repeat("a", 1000), want: ErrCipherBlobCorrupt},
		{name: "provider error", getErr: errBoom, want: ErrCipherBlobProvider},
		{name: "read error", readErr: errBoom, want: ErrCipherBlobProvider},
		{name: "close error", content: "abc", closeErr: errBoom, want: ErrCipherBlobProvider},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prior := committedCipherForRetry()
			blobs := &retryBlobStore{fakeBlobs: &fakeBlobs{}, content: tc.content, getErr: tc.getErr, readErr: tc.readErr, closeErr: tc.closeErr}
			s := newSvc(&fakeStore{}, blobs, nil)
			s.cipher = &memoryCipherBlobs{blob: prior}
			got, err := s.PutCipherBlob(store.WithOwner(tenantCtx(t), "alice"), "mail", prior.ID, 3, strings.NewReader("abc"))
			if !errors.Is(err, tc.want) || tc.want == nil && got.StorageKey != prior.StorageKey {
				t.Fatal("incorrect retry outcome", got, err)
			}
			if blobs.gets != 1 || blobs.bytes > 4 || blobs.getErr == nil && !blobs.closed || len(blobs.delKeys) != 0 || len(blobs.putBody) != 0 {
				t.Fatal("retry did not read a bounded block and preserve the provider copy", blobs)
			}
		})
	}
}

func TestCipherBlobRetryCancellationAndConflict(t *testing.T) {
	prior := committedCipherForRetry()
	ctx, cancel := context.WithCancel(store.WithOwner(tenantCtx(t), "alice"))
	blobs := &retryBlobStore{fakeBlobs: &fakeBlobs{}, content: "abc", cancel: cancel}
	s := newSvc(&fakeStore{}, blobs, nil)
	s.cipher = &memoryCipherBlobs{blob: prior}
	if _, err := s.PutCipherBlob(ctx, "mail", prior.ID, 3, strings.NewReader("abc")); !errors.Is(err, context.Canceled) || !blobs.closed {
		t.Fatal("canceled provider read was acknowledged or left open", err)
	}
	blobs = &retryBlobStore{fakeBlobs: &fakeBlobs{}, content: "abc"}
	s = newSvc(&fakeStore{}, blobs, nil)
	s.cipher = &memoryCipherBlobs{blob: prior}
	if _, err := s.PutCipherBlob(store.WithOwner(tenantCtx(t), "alice"), "mail", prior.ID, 3, strings.NewReader("xyz")); !errors.Is(err, domain.ErrConflict) || blobs.gets != 0 {
		t.Fatal("different bytes caused a provider read or were accepted", err)
	}
}

type concurrentCipherCommit struct {
	prior domain.CipherBlob
	reads int
}

func (s *concurrentCipherCommit) CipherBlob(context.Context, string, string) (domain.CipherBlob, error) {
	s.reads++
	if s.reads == 1 {
		return domain.CipherBlob{}, domain.ErrNotFound
	}
	return s.prior, nil
}

func (*concurrentCipherCommit) CreateCipherBlob(context.Context, domain.CipherBlob) (domain.CipherBlob, error) {
	return domain.CipherBlob{}, domain.ErrConflict
}

func (*concurrentCipherCommit) DeleteCipherBlob(context.Context, string, string) (domain.CipherBlob, error) {
	panic("unexpected delete")
}

func (*concurrentCipherCommit) CompleteCipherBlobDelete(context.Context, domain.CipherBlob) error {
	panic("unexpected delete completion")
}

func TestCipherBlobConcurrentCommitVerifiesWinningProviderCopy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    error
	}{{"intact", "abc", nil}, {"corrupt", "xyz", ErrCipherBlobCorrupt}} {
		t.Run(tc.name, func(t *testing.T) {
			prior := committedCipherForRetry()
			blobs := &retryBlobStore{fakeBlobs: &fakeBlobs{}, content: tc.content}
			s := newSvc(&fakeStore{quota: domain.Quota{LimitBytes: 1 << 20}}, blobs, nil)
			s.cipher = &concurrentCipherCommit{prior: prior}
			got, err := s.PutCipherBlob(store.WithOwner(tenantCtx(t), "alice"), "mail", prior.ID, 3, strings.NewReader("abc"))
			if !errors.Is(err, tc.want) || tc.want == nil && got.StorageKey != prior.StorageKey || blobs.gets != 1 || !blobs.closed || len(blobs.delKeys) != 0 {
				t.Fatal("concurrent winner was acknowledged without verification", got, err)
			}
		})
	}
}
