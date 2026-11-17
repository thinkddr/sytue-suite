// SPDX-License-Identifier: Apache-2.0
package baseshttp

import (
	"context"
	"errors"
	"io"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
	"git.sytue.eu/sytue/one/services/bases/internal/store"
)

type basesPrivateStore interface {
	PrivateStorageState(context.Context, string) (storage.AppMigrationState, error)
	BeginPrivateStorageMigration(context.Context, string, uuid.UUID) (storage.AppMigrationState, error)
	PrivateStorageSourcePage(context.Context, string, uuid.UUID, storage.LegacySourceCursor, int) (storage.StreamSourcePage, error)
	OpenPrivateStorageSource(context.Context, string, uuid.UUID, string, string) (storage.StreamSourceRow, io.ReadCloser, error)
}

type basesPrivateSource struct{ store basesPrivateStore }

func basesSourceOwner(ctx context.Context, scope storage.Scope) (string, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil || !scope.Valid() || scope.Product != "bases" || scope.Tenant != tenant.Slug {
		return "", pg.ErrPrivateStorageScope
	}
	return scope.Subject, nil
}

func (s basesPrivateSource) State(ctx context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	owner, err := basesSourceOwner(ctx, scope)
	if err != nil {
		return storage.AppMigrationState{}, err
	}
	return s.store.PrivateStorageState(ctx, owner)
}

func (s basesPrivateSource) Begin(ctx context.Context, scope storage.Scope, epoch uuid.UUID) (storage.AppMigrationState, error) {
	owner, err := basesSourceOwner(ctx, scope)
	if err != nil {
		return storage.AppMigrationState{}, err
	}
	return s.store.BeginPrivateStorageMigration(ctx, owner, epoch)
}

func (s basesPrivateSource) Page(ctx context.Context, scope storage.Scope, epoch uuid.UUID, after storage.LegacySourceCursor, limit int) (storage.StreamSourcePage, error) {
	owner, err := basesSourceOwner(ctx, scope)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	return s.store.PrivateStorageSourcePage(ctx, owner, epoch, after, limit)
}

func (s basesPrivateSource) Open(ctx context.Context, scope storage.Scope, epoch uuid.UUID, kind, id string) (storage.StreamSourceRow, io.ReadCloser, error) {
	owner, err := basesSourceOwner(ctx, scope)
	if err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	return s.store.OpenPrivateStorageSource(ctx, owner, epoch, kind, id)
}

func basesSourceKind(kind string) bool {
	switch kind {
	case "base", "table", "field", "view", "record":
		return true
	default:
		return false
	}
}

func basesSourceError(err error) *httpx.Error {
	switch {
	case errors.Is(err, store.ErrPrivateStorageFrozen):
		return httpx.ErrConflict.WithCode("private_storage_frozen")
	case errors.Is(err, store.ErrPrivateStorageEpoch), errors.Is(err, pg.ErrPrivateSource), errors.Is(err, pg.ErrPrivateSnapshotEpoch):
		return httpx.ErrConflict.WithCode("private_storage_snapshot_changed")
	case errors.Is(err, pg.ErrPrivateStorageScope):
		return httpx.ErrConflict.WithCode("storage_identity_changed")
	case errors.Is(err, domain.ErrNoExiste):
		return httpx.ErrNotFound
	default:
		return httpx.ErrInternal
	}
}

func mountBasesPrivateSource(api chi.Router, source basesPrivateStore) {
	var port httpx.PrivateSourcePort
	if source != nil {
		port = basesPrivateSource{store: source}
	}
	handlers, err := httpx.NewPrivateSourceHandlers(httpx.PrivateSourceHTTPConfig{
		Product:      "bases",
		Port:         port,
		MaxBodyBytes: 32 << 20,
		Kind:         basesSourceKind,
		MapError:     basesSourceError,
		Identity: func(ctx context.Context) (httpx.PrivateSourceIdentity, bool) {
			claims, ok := authn.FromContext(ctx)
			return httpx.PrivateSourceIdentity{Tenant: claims.Tenant, Subject: claims.Subject, Delegado: claims.Delegado}, ok
		},
	})
	if err != nil {
		panic(err)
	}
	api.Get("/private-storage/migration", handlers.State)
	api.Post("/private-storage/migration", handlers.Begin)
	api.Get("/private-storage/migration/{epoch}/rows", handlers.Page)
	api.Get("/private-storage/migration/{epoch}/source/{kind}/{source}", handlers.Body)
}
