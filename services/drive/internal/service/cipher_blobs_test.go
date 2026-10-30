// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

type memoryCipherBlobs struct {
	blob      domain.CipherBlob
	tombstone bool
	createErr error
	completed bool
}

func (m *memoryCipherBlobs) CipherBlob(context.Context, string, string) (domain.CipherBlob, error) {
	if m.blob.ID == "" || m.tombstone {
		return domain.CipherBlob{}, domain.ErrNotFound
	}
	return m.blob, nil
}

func (m *memoryCipherBlobs) CreateCipherBlob(_ context.Context, b domain.CipherBlob) (domain.CipherBlob, error) {
	if m.createErr != nil {
		return domain.CipherBlob{}, m.createErr
	}
	m.blob = b
	return b, nil
}

func (m *memoryCipherBlobs) DeleteCipherBlob(context.Context, string, string) (domain.CipherBlob, error) {
	m.tombstone = true
	return m.blob, nil
}

func (m *memoryCipherBlobs) CompleteCipherBlobDelete(context.Context, domain.CipherBlob) error {
	m.completed = true
	return nil
}

func TestCipherBlobRetriesCompensationsAndIndependentBlockLimit(t *testing.T) {
	ctx := store.WithOwner(tenantCtx(t), "alice")
	id := strings.Repeat("A", 26)
	blobs := &fakeBlobs{}
	records := &memoryCipherBlobs{}
	s := newSvc(&fakeStore{quota: domain.Quota{LimitBytes: 1 << 20}}, blobs, nil)
	s.cipher = records
	// Ordinary Drive uploads are capped at 100 in this fixture. Suite ciphertext
	// blocks have their own fixed bound, regardless of that UI setting.
	ciphertext := strings.Repeat("opaque", 100)
	saved, err := s.PutCipherBlob(ctx, "mail", id, int64(len(ciphertext)), strings.NewReader(ciphertext))
	if err != nil || saved.ID != id || string(blobs.putBody) != ciphertext {
		t.Fatal("cipher block did not reach storage", err)
	}
	if _, err = s.PutCipherBlob(ctx, "mail", id, int64(len(ciphertext)), strings.NewReader(ciphertext)); err != nil {
		t.Fatal("lost receipt could not be retried", err)
	}
	if _, err = s.PutCipherBlob(ctx, "mail", id, int64(len(ciphertext)), strings.NewReader(strings.Repeat("x", len(ciphertext)))); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("same ID accepted different ciphertext", err)
	}
	records.blob = domain.CipherBlob{}
	records.createErr = domain.ErrQuotaExceeded
	if _, err = s.PutCipherBlob(ctx, "mail", id, 3, strings.NewReader("abc")); !errors.Is(err, domain.ErrQuotaExceeded) || len(blobs.delKeys) != 1 {
		t.Fatal("failed commit leaked upload", err)
	}
	records.createErr = nil
	if _, err = s.PutCipherBlob(ctx, "mail", id, 2, strings.NewReader("abc")); !errors.Is(err, ErrUploadSizeMismatch) || len(blobs.delKeys) != 2 {
		t.Fatal("declared length silently truncated ciphertext", err)
	}
	if _, err = s.PutCipherBlob(tenantCtx(t), "mail", id, 3, strings.NewReader("abc")); !errors.Is(err, domain.ErrPermission) {
		t.Fatal("unidentified upload", err)
	}
}

func TestCipherBlobDeletionRetainsRoutingUntilProviderConfirms(t *testing.T) {
	ctx := store.WithOwner(tenantCtx(t), "alice")
	records := &memoryCipherBlobs{blob: domain.CipherBlob{StorageKey: "ext/old-object"}}
	blobs := &fakeBlobs{delErr: errBoom}
	s := newSvc(&fakeStore{}, blobs, nil)
	s.cipher = records
	if err := s.DeleteCipherBlob(ctx, "mail", strings.Repeat("A", 26)); !errors.Is(err, errBoom) || !records.tombstone || records.completed {
		t.Fatal("provider failure lost deletion state", err)
	}
	blobs.delErr = objectstore.ErrNotFound
	if err := s.DeleteCipherBlob(ctx, "mail", strings.Repeat("A", 26)); err != nil || !records.completed {
		t.Fatal("idempotent provider deletion failed", err)
	}
}
