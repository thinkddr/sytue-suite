// SPDX-License-Identifier: Apache-2.0
package http

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

type photoPrivateSource interface {
	httpx.PrivateSourcePort
	Preflight(context.Context, storage.Scope, string) (model.SourcePreflight, error)
	PlanRouting(context.Context, storage.Scope) (model.SourcePlanRouting, error)
	AcceptPlan(context.Context, storage.Scope, string, string, storage.Reference) (model.SourcePlanReference, error)
	StartPreflight(context.Context, storage.Scope, string) (model.SourcePreflightInfo, error)
	PreflightPage(context.Context, storage.Scope, string, string, storage.LegacySourceCursor, int) (storage.StreamSourcePage, error)
	OpenPreflightJSON(context.Context, storage.Scope, string, string, string, string) (storage.StreamSourceRow, io.ReadCloser, error)
}

type photoSourceHandlers struct {
	guard *httpx.PrivateSourceHandlers
	port  photoPrivateSource
}

func photoSourceError(err error) *httpx.Error {
	switch {
	case errors.Is(err, pg.ErrPrivateStorageScope):
		return httpx.ErrConflict.WithCode("storage_identity_changed")
	case errors.Is(err, store.ErrPrivateSource), errors.Is(err, pg.ErrPrivateSnapshotEpoch):
		return httpx.ErrConflict.WithCode("private_storage_snapshot_changed")
	case errors.Is(err, model.ErrSnapshot), errors.Is(err, model.ErrShared):
		return httpx.ErrConflict.WithCode("private_photos_source_incompatible")
	default:
		return httpx.ErrInternal
	}
}

func mountPhotosPrivateSource(api chi.Router, source photoPrivateSource) {
	var port httpx.PrivateSourcePort
	if source != nil {
		port = source
	}
	cfg := httpx.PrivateSourceHTTPConfig{
		Product: "photos", Port: port, MaxBodyBytes: storage.MaxFileBytes,
		Kind:     func(kind string) bool { return kind == "variant" || model.SourceKind(kind) },
		MapError: photoSourceError,
		Identity: func(ctx context.Context) (httpx.PrivateSourceIdentity, bool) {
			claims, ok := authn.FromContext(ctx)
			return httpx.PrivateSourceIdentity{Tenant: claims.Tenant, Subject: claims.Subject, Delegado: claims.Delegado}, ok
		},
	}
	guard, err := httpx.NewPrivateSourceHandlers(cfg)
	if err != nil {
		panic(err)
	}
	h := photoSourceHandlers{guard: guard, port: source}
	api.Get("/private-storage/migration", guard.State)
	api.Post("/private-storage/migration", guard.Begin)
	api.Get("/private-storage/migration/{epoch}/rows", guard.Page)
	api.Get("/private-storage/migration/{epoch}/source/{kind}/{source}", guard.Body)
	api.Post("/private-storage/preflight", h.preflight)
	api.Get("/private-storage/plan", h.routing)
	api.Post("/private-storage/plan", h.accept)
	api.Post("/private-storage/preflight/paged", h.startPreflight)
	if source != nil {
		cfg.Port = photoPreflightPort{photoPrivateSource: source}
	} else {
		cfg.Port = nil
	}
	paged, err := httpx.NewPrivateSourceHandlers(cfg)
	if err != nil {
		panic(err)
	}
	api.With(photoPreflightTicket).Get("/private-storage/preflight/{epoch}/{ticket}/rows", paged.Page)
	api.With(photoPreflightTicket).Get("/private-storage/preflight/{epoch}/{ticket}/source/{kind}/{source}", paged.Body)
}

type (
	photoPreflightContextKey struct{}
	photoPreflightPort       struct{ photoPrivateSource }
)

func (p photoPreflightPort) Page(ctx context.Context, scope storage.Scope, epoch uuid.UUID, after storage.LegacySourceCursor, limit int) (storage.StreamSourcePage, error) {
	ticket, _ := ctx.Value(photoPreflightContextKey{}).(string)
	return p.PreflightPage(ctx, scope, epoch.String(), ticket, after, limit)
}

func (p photoPreflightPort) Open(ctx context.Context, scope storage.Scope, epoch uuid.UUID, kind, id string) (storage.StreamSourceRow, io.ReadCloser, error) {
	ticket, _ := ctx.Value(photoPreflightContextKey{}).(string)
	return p.OpenPreflightJSON(ctx, scope, epoch.String(), ticket, kind, id)
}

func photoPreflightTicket(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ticket := chi.URLParam(r, "ticket")
		if !storage.MigrationEpoch(ticket) {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), photoPreflightContextKey{}, ticket)))
	})
}

func (h photoSourceHandlers) startPreflight(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.guard.AuthenticatedScope(w, r)
	if !ok {
		return
	}
	var input struct {
		Epoch string `json:"epoch"`
	}
	if httpx.DecodeJSONLimit(w, r, &input, 128) != nil || !storage.MigrationEpoch(input.Epoch) {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	value, err := h.port.StartPreflight(r.Context(), scope, input.Epoch)
	if err != nil {
		photoControlFailure(w, r, err)
		return
	}
	if value.Validate(scope, input.Epoch) != nil {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	httpx.JSON(w, r, http.StatusOK, value)
}

func photoControlFailure(w http.ResponseWriter, r *http.Request, err error) {
	failure := photoSourceError(err)
	// Keep legacy data, object keys and provider error text out of API failures.
	httpx.Fail(w, r, httpx.NewError(failure.Status, failure.Code, http.StatusText(failure.Status)))
}

func (h photoSourceHandlers) preflight(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.guard.AuthenticatedScope(w, r)
	if !ok {
		return
	}
	var input struct {
		Epoch string `json:"epoch"`
	}
	if httpx.DecodeJSONLimit(w, r, &input, 128) != nil || !storage.MigrationEpoch(input.Epoch) {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	value, err := h.port.Preflight(r.Context(), scope, input.Epoch)
	if err != nil {
		photoControlFailure(w, r, err)
		return
	}
	if value.Validate(scope, input.Epoch) != nil {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	httpx.JSON(w, r, http.StatusOK, value)
}

func (h photoSourceHandlers) routing(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.guard.AuthenticatedScope(w, r)
	if !ok {
		return
	}
	value, err := h.port.PlanRouting(r.Context(), scope)
	if err != nil {
		photoControlFailure(w, r, err)
		return
	}
	if value.Validate(scope) != nil {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	httpx.JSON(w, r, http.StatusOK, value)
}

func (h photoSourceHandlers) accept(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.guard.AuthenticatedScope(w, r)
	if !ok {
		return
	}
	var input struct {
		Epoch string            `json:"epoch"`
		CAS   string            `json:"cas"`
		Link  storage.Reference `json:"link"`
	}
	if httpx.DecodeJSONLimit(w, r, &input, 8192) != nil || !storage.MigrationEpoch(input.Epoch) || !storage.MigrationEpoch(input.CAS) || !input.Link.Valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	value, err := h.port.AcceptPlan(r.Context(), scope, input.Epoch, input.CAS, input.Link)
	if err != nil {
		photoControlFailure(w, r, err)
		return
	}
	if !value.Valid() || value.Epoch != input.Epoch || value.Link != input.Link {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	httpx.JSON(w, r, http.StatusOK, value)
}
