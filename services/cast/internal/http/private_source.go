// SPDX-License-Identifier: Apache-2.0
package casthttp

import (
	"context"
	"errors"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/cast/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

func castPrivateWriteBlocked(err error) bool {
	var failure *pgconn.PgError
	return errors.As(err, &failure) && failure.Code == "PST01"
}

func castSourceError(err error) *httpx.Error {
	switch {
	case castPrivateWriteBlocked(err):
		return httpx.ErrConflict.WithCode("private_storage_frozen")
	case errors.Is(err, pg.ErrPrivateStorageScope):
		return httpx.ErrConflict.WithCode("storage_identity_changed")
	case errors.Is(err, model.ErrShared), errors.Is(err, model.ErrLive), errors.Is(err, model.ErrDeviceWork):
		return httpx.ErrConflict.WithCode("private_cast_source_incompatible")
	case errors.Is(err, store.ErrPrivateSource), errors.Is(err, model.ErrSnapshot), errors.Is(err, pg.ErrPrivateSource), errors.Is(err, pg.ErrPrivateSnapshotEpoch):
		return httpx.ErrConflict.WithCode("private_storage_snapshot_changed")
	default:
		return httpx.ErrInternal
	}
}

func mountCastPrivateSource(api chi.Router, source httpx.PrivateSourcePort) {
	handlers, err := httpx.NewPrivateSourceHandlers(httpx.PrivateSourceHTTPConfig{
		Product: "cast", Port: source, MaxBodyBytes: storage.MaxFileBytes,
		Kind: func(kind string) bool { return kind == model.BinaryKind || model.SourceKind(kind) }, MapError: castSourceError,
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
