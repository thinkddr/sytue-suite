// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"io"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

type privateStorageStore interface {
	PrivateStorageState(context.Context) (store.PrivateStorageState, error)
	BeginPrivateStorageMigration(context.Context, uuid.UUID) (store.PrivateStorageState, error)
	PrivateStorageSourcePage(context.Context, uuid.UUID, store.PrivateSourceCursor, int) (store.PrivateSourcePage, error)
	PrivateStorageSourceBlob(context.Context, uuid.UUID, string, string) (store.PrivateSourceBlob, error)
}

func (s *Service) PrivateStorageAvailable() bool { _, ok := s.store.(privateStorageStore); return ok }

type privateStorageReleaser interface {
	ReleasePrivateStorage(context.Context, storage.Scope, pg.PrivateStorageRelease) error
}

// ReleasePrivateStorage es «soltar la fuente» (ADR 0086): vuelve a legacy la fuente
// congelada del Drive de esta persona. Lo monta platform/pkg/soltarfuente, que ya
// exige la confirmación y la reautenticación reciente.
func (s *Service) ReleasePrivateStorage(ctx context.Context, scope storage.Scope, release pg.PrivateStorageRelease) error {
	source, ok := s.store.(privateStorageReleaser)
	if !ok {
		return domain.ErrNotFound
	}
	return source.ReleasePrivateStorage(ctx, scope, release)
}

func (s *Service) PrivateStorageState(ctx context.Context) (store.PrivateStorageState, error) {
	source, ok := s.store.(privateStorageStore)
	if !ok {
		return store.PrivateStorageState{}, domain.ErrNotFound
	}
	return source.PrivateStorageState(ctx)
}

func (s *Service) BeginPrivateStorageMigration(ctx context.Context, epoch uuid.UUID) (store.PrivateStorageState, error) {
	source, ok := s.store.(privateStorageStore)
	if !ok {
		return store.PrivateStorageState{}, domain.ErrNotFound
	}
	return source.BeginPrivateStorageMigration(ctx, epoch)
}

func (s *Service) PrivateStorageSourcePage(ctx context.Context, epoch uuid.UUID, cursor store.PrivateSourceCursor, limit int) (store.PrivateSourcePage, error) {
	source, ok := s.store.(privateStorageStore)
	if !ok {
		return store.PrivateSourcePage{}, domain.ErrNotFound
	}
	return source.PrivateStorageSourcePage(ctx, epoch, cursor, limit)
}

func (s *Service) OpenPrivateStorageSource(ctx context.Context, epoch uuid.UUID, kind, id string) (store.PrivateSourceBlob, io.ReadCloser, error) {
	source, ok := s.store.(privateStorageStore)
	if !ok {
		return store.PrivateSourceBlob{}, nil, domain.ErrNotFound
	}
	blob, err := source.PrivateStorageSourceBlob(ctx, epoch, kind, id)
	if err != nil {
		return blob, nil, err
	}
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return blob, nil, err
	}
	size, err := s.blobs.Stat(ctx, tenant, blob.StorageKey)
	if err != nil {
		return blob, nil, err
	}
	if size != blob.SizeBytes {
		return blob, nil, domain.ErrPrivateStorageEpoch
	}
	body, err := s.blobs.Get(ctx, tenant, blob.StorageKey)
	return blob, body, err
}
