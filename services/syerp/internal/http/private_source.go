// SPDX-License-Identifier: Apache-2.0
package syerphttp

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
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

type erpPrivateStore interface {
	PrivateStorageState(context.Context, string) (storage.AppMigrationState, error)
	BeginPrivateStorageMigration(context.Context, string, uuid.UUID) (storage.AppMigrationState, error)
	PrivateStorageSourcePage(context.Context, string, uuid.UUID, storage.LegacySourceCursor, int) (storage.StreamSourcePage, error)
	OpenPrivateStorageSource(context.Context, string, uuid.UUID, string, string) (storage.StreamSourceRow, io.ReadCloser, error)
}

type erpPrivateSource struct{ store erpPrivateStore }

func erpSourceOwner(ctx context.Context, scope storage.Scope) (string, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil || !scope.Valid() || scope.Product != "erp" || scope.Tenant != tenant.Slug {
		return "", pg.ErrPrivateStorageScope
	}
	return scope.Subject, nil
}

func (s erpPrivateSource) State(ctx context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	owner, err := erpSourceOwner(ctx, scope)
	if err != nil {
		return storage.AppMigrationState{}, err
	}
	return s.store.PrivateStorageState(ctx, owner)
}

func (s erpPrivateSource) Begin(ctx context.Context, scope storage.Scope, epoch uuid.UUID) (storage.AppMigrationState, error) {
	owner, err := erpSourceOwner(ctx, scope)
	if err != nil {
		return storage.AppMigrationState{}, err
	}
	return s.store.BeginPrivateStorageMigration(ctx, owner, epoch)
}

func (s erpPrivateSource) Page(ctx context.Context, scope storage.Scope, epoch uuid.UUID, after storage.LegacySourceCursor, limit int) (storage.StreamSourcePage, error) {
	owner, err := erpSourceOwner(ctx, scope)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	return s.store.PrivateStorageSourcePage(ctx, owner, epoch, after, limit)
}

func (s erpPrivateSource) Open(ctx context.Context, scope storage.Scope, epoch uuid.UUID, kind, id string) (storage.StreamSourceRow, io.ReadCloser, error) {
	owner, err := erpSourceOwner(ctx, scope)
	if err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	return s.store.OpenPrivateStorageSource(ctx, owner, epoch, kind, id)
}

func erpSourceKind(kind string) bool {
	switch kind {
	case "record", "activity":
		return true
	default:
		return false
	}
}

// errPrivateStorageFrozen es la respuesta a la valla de la 0105: la fuente
// privada del dueño de la fila ya no es legacy y la base rechaza la escritura.
var errPrivateStorageFrozen = httpx.ErrConflict.WithCode("private_storage_frozen").
	WithMessage("This data is being moved to private storage and can no longer be changed here.")

func erpSourceError(err error) *httpx.Error {
	switch {
	case errors.Is(err, store.ErrPrivateStorageFrozen):
		return errPrivateStorageFrozen
	case errors.Is(err, store.ErrPrivateStorageEpoch), errors.Is(err, pg.ErrPrivateSource), errors.Is(err, pg.ErrPrivateSnapshotEpoch):
		return httpx.ErrConflict.WithCode("private_storage_snapshot_changed")
	case errors.Is(err, pg.ErrPrivateStorageScope):
		return httpx.ErrConflict.WithCode("storage_identity_changed")
	case errors.Is(err, store.ErrNoEncontrado):
		return httpx.ErrNotFound
	default:
		return httpx.ErrInternal
	}
}

func mountERPPrivateSource(api chi.Router, source erpPrivateStore) {
	var port httpx.PrivateSourcePort
	if source != nil {
		port = erpPrivateSource{store: source}
	}
	handlers, err := httpx.NewPrivateSourceHandlers(httpx.PrivateSourceHTTPConfig{
		Product:      "erp",
		Port:         port,
		MaxBodyBytes: 32 << 20,
		Kind:         erpSourceKind,
		MapError:     erpSourceError,
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
