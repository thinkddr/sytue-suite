// SPDX-License-Identifier: Apache-2.0
package calendarhttp

import (
	"context"
	"errors"
	"io"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

type calendarPrivateStore interface {
	PrivateStorageState(context.Context, string) (storage.AppMigrationState, error)
	BeginPrivateStorageMigration(context.Context, string, uuid.UUID) (storage.AppMigrationState, error)
	PrivateStorageSourcePage(context.Context, string, uuid.UUID, storage.LegacySourceCursor, int) (storage.StreamSourcePage, error)
	OpenPrivateStorageSource(context.Context, string, uuid.UUID, string, string) (storage.StreamSourceRow, io.ReadCloser, error)
}

type calendarPrivateSource struct{ store calendarPrivateStore }

func calendarSourceOwner(ctx context.Context, scope storage.Scope) (string, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil || !scope.Valid() || scope.Product != "calendar" || scope.Tenant != tenant.Slug {
		return "", pg.ErrPrivateStorageScope
	}
	return scope.Subject, nil
}

func (s calendarPrivateSource) State(ctx context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	owner, err := calendarSourceOwner(ctx, scope)
	if err != nil {
		return storage.AppMigrationState{}, err
	}
	return s.store.PrivateStorageState(ctx, owner)
}

func (s calendarPrivateSource) Begin(ctx context.Context, scope storage.Scope, epoch uuid.UUID) (storage.AppMigrationState, error) {
	owner, err := calendarSourceOwner(ctx, scope)
	if err != nil {
		return storage.AppMigrationState{}, err
	}
	return s.store.BeginPrivateStorageMigration(ctx, owner, epoch)
}

func (s calendarPrivateSource) Page(ctx context.Context, scope storage.Scope, epoch uuid.UUID, after storage.LegacySourceCursor, limit int) (storage.StreamSourcePage, error) {
	owner, err := calendarSourceOwner(ctx, scope)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	return s.store.PrivateStorageSourcePage(ctx, owner, epoch, after, limit)
}

func (s calendarPrivateSource) Open(ctx context.Context, scope storage.Scope, epoch uuid.UUID, kind, id string) (storage.StreamSourceRow, io.ReadCloser, error) {
	owner, err := calendarSourceOwner(ctx, scope)
	if err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	return s.store.OpenPrivateStorageSource(ctx, owner, epoch, kind, id)
}

func calendarSourceError(err error) *httpx.Error {
	switch {
	case errors.Is(err, store.ErrPrivateStorageIngress):
		return httpx.ErrUnavailable.WithCode("private_calendar_ingress_required")
	case errors.Is(err, store.ErrPrivateStorageEpoch):
		return httpx.ErrConflict.WithCode("private_storage_snapshot_changed")
	case errors.Is(err, pg.ErrPrivateStorageScope):
		return httpx.ErrConflict.WithCode("storage_identity_changed")
	case errors.Is(err, store.ErrPrivateStorageCollaboration):
		return httpx.ErrConflict.WithCode("private_calendar_collaboration_required")
	case errors.Is(err, store.ErrPrivateStorageResources):
		return httpx.ErrConflict.WithCode("private_calendar_resources_required")
	case errors.Is(err, store.ErrPrivateStorageSubscriptions):
		return httpx.ErrConflict.WithCode("private_calendar_subscriptions_required")
	case errors.Is(err, store.ErrPrivateStorageReminders):
		return httpx.ErrConflict.WithCode("private_calendar_reminders_required")
	case errors.Is(err, store.ErrPrivateStorageOutbox):
		return httpx.ErrConflict.WithCode("private_calendar_outbox_required")
	case errors.Is(err, store.ErrPrivateStorageAmbiguous):
		return httpx.ErrConflict.WithCode("private_calendar_source_ambiguous")
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrNotFound
	default:
		return httpx.ErrInternal
	}
}

func mountCalendarPrivateSource(api chi.Router, source calendarPrivateStore) {
	var port httpx.PrivateSourcePort
	if source != nil {
		port = calendarPrivateSource{store: source}
	}
	handlers, err := httpx.NewPrivateSourceHandlers(httpx.PrivateSourceHTTPConfig{Product: "calendar", Port: port, MaxBodyBytes: model.MaxRowBytes, Kind: model.SourceKind, MapError: calendarSourceError, Identity: func(ctx context.Context) (httpx.PrivateSourceIdentity, bool) {
		claims, ok := authn.FromContext(ctx)
		return httpx.PrivateSourceIdentity{Tenant: claims.Tenant, Subject: claims.Subject, Delegado: claims.Delegado}, ok
	}})
	if err != nil {
		panic(err)
	}
	api.Get("/private-storage/migration", handlers.State)
	api.Post("/private-storage/migration", handlers.Begin)
	api.Get("/private-storage/migration/{epoch}/rows", handlers.Page)
	api.Get("/private-storage/migration/{epoch}/source/{kind}/{source}", handlers.Body)
}
