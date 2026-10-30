// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

type cipherBlobStore interface {
	CipherBlob(context.Context, string, string) (domain.CipherBlob, error)
	CreateCipherBlob(context.Context, domain.CipherBlob) (domain.CipherBlob, error)
	DeleteCipherBlob(context.Context, string, string) (domain.CipherBlob, error)
	CompleteCipherBlobDelete(context.Context, domain.CipherBlob) error
}

// PutCipherBlob never receives a key or private manifest. The existing router
// chooses the connected provider and pins the stored key exactly as for Drive.
// A repeated ID acknowledges only identical ciphertext, never a replacement.
func (s *Service) PutCipherBlob(ctx context.Context, product, id string, size int64, body io.Reader) (domain.CipherBlob, error) {
	if s.cipher == nil || store.OwnerFrom(ctx) == "" {
		return domain.CipherBlob{}, domain.ErrPermission
	}
	if !storage.Product(product) || !storage.BlobID(id) || size < 0 || size > storage.MaxCipherBlockBytes || body == nil {
		return domain.CipherBlob{}, domain.ErrInvalidName
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return domain.CipherBlob{}, err
	}
	limited := io.LimitReader(body, size+1)
	prior, err := s.cipher.CipherBlob(ctx, product, id)
	if err == nil {
		return verifyCipherRetry(prior, size, limited)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.CipherBlob{}, err
	}
	if _, err := s.store.EnsureQuota(ctx, s.cfg.DefaultQuotaBytes); err != nil {
		return domain.CipherBlob{}, err
	}
	key, err := s.claveDeBlob(ctx, t, uuid.New(), id)
	if err != nil {
		return domain.CipherBlob{}, err
	}
	// Reuse the suite's durable upload reservations. A crash or cancelled request
	// leaves an opaque key for the existing collector, rather than an untracked
	// object. The request ends before the reservation can be collected.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := s.store.ReservarSubida(ctx, key, size, time.Now().Add(5*time.Minute), !store.EsExterno(key)); err != nil {
		return domain.CipherBlob{}, err
	}
	// The name is opaque and the provider does not need the person's identity.
	key, actual, digest, err := s.putBlobLimited(ctx, t, key, id, "application/octet-stream", "", limited, storage.MaxCipherBlockBytes)
	if err != nil {
		return domain.CipherBlob{}, err
	}
	if actual != size {
		s.soltarBlob(ctx, t, key)
		return domain.CipherBlob{}, ErrUploadSizeMismatch
	}
	blob := domain.CipherBlob{BlobReceipt: storage.BlobReceipt{ID: id, Product: product, SizeBytes: actual, SHA256: digest}, StorageKey: key, OwnerSub: store.OwnerFrom(ctx)}
	return s.commitCipherBlob(ctx, t, blob)
}

func (s *Service) commitCipherBlob(ctx context.Context, t tenancy.Tenant, blob domain.CipherBlob) (domain.CipherBlob, error) {
	saved, err := s.cipher.CreateCipherBlob(ctx, blob)
	if err != nil {
		s.soltarBlob(ctx, t, blob.StorageKey)
	}
	if errors.Is(err, domain.ErrConflict) {
		prior, readErr := s.cipher.CipherBlob(ctx, blob.Product, blob.ID)
		if readErr == nil && prior.SizeBytes == blob.SizeBytes && prior.SHA256 == blob.SHA256 {
			return prior, nil
		}
	}
	return saved, err
}

func verifyCipherRetry(prior domain.CipherBlob, size int64, body io.Reader) (domain.CipherBlob, error) {
	h := sha256.New()
	n, err := io.Copy(h, body)
	if err != nil {
		return domain.CipherBlob{}, err
	}
	if n != size || prior.SizeBytes != size || prior.SHA256 != hex.EncodeToString(h.Sum(nil)) {
		return domain.CipherBlob{}, domain.ErrConflict
	}
	return prior, nil
}

func (s *Service) CipherBlob(ctx context.Context, product, id string) (domain.CipherBlob, error) {
	if s.cipher == nil {
		return domain.CipherBlob{}, domain.ErrNotFound
	}
	return s.cipher.CipherBlob(ctx, product, id)
}

func (s *Service) OpenCipherBlob(ctx context.Context, product, id string) (domain.CipherBlob, io.ReadCloser, error) {
	blob, err := s.CipherBlob(ctx, product, id)
	if err != nil {
		return blob, nil, err
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return blob, nil, err
	}
	stream, err := s.blobs.Get(ctx, t, blob.StorageKey)
	if errors.Is(err, objectstore.ErrNotFound) {
		return blob, nil, domain.ErrNotFound
	}
	return blob, stream, err
}

func (s *Service) DeleteCipherBlob(ctx context.Context, product, id string) error {
	if s.cipher == nil {
		return domain.ErrNotFound
	}
	blob, err := s.cipher.DeleteCipherBlob(ctx, product, id)
	if err != nil {
		return err
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	if err := s.blobs.Delete(ctx, t, blob.StorageKey); err != nil && !errors.Is(err, objectstore.ErrNotFound) {
		return err
	}
	return s.cipher.CompleteCipherBlobDelete(ctx, blob)
}
