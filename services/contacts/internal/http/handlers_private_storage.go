// SPDX-License-Identifier: Apache-2.0
package contactshttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

type privateSourceService interface {
	PrivateStorageState(context.Context, string) (store.PrivateStorageState, error)
	BeginPrivateStorageMigration(context.Context, string, uuid.UUID) (store.PrivateStorageState, error)
	PrivateStorageSourcePage(context.Context, string, uuid.UUID, storage.LegacySourceCursor) (storage.StreamSourcePage, error)
	OpenPrivateStorageSource(context.Context, string, uuid.UUID, string, string) (storage.StreamSourceRow, io.ReadCloser, error)
}

func (h *Handlers) privateSourceScope(w http.ResponseWriter, r *http.Request) (storage.Scope, bool) {
	tenant, err := tenancy.From(r.Context())
	scope := storage.Scope{Tenant: tenant.Slug, Subject: subFromCtx(r), Product: "contacts"}
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return storage.Scope{}, false
	}
	if scopeErr := storage.ValidateRequestScope(r.Header, scope); scopeErr != nil {
		failure := httpx.ErrBadRequest
		if errors.Is(scopeErr, storage.ErrRequestScopeMismatch) {
			failure = httpx.ErrConflict
		}
		httpx.Fail(w, r, failure)
		return storage.Scope{}, false
	}
	if h.source == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return storage.Scope{}, false
	}
	return scope, true
}

func validPrivateContactsState(state store.PrivateStorageState, scope storage.Scope) bool {
	return state.Version == 1 && !state.Shared && state.Validate(scope) == nil
}

func (h *Handlers) PrivateStorageState(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.privateSourceScope(w, r)
	if !ok {
		return
	}
	state, err := h.source.PrivateStorageState(r.Context(), scope.Subject)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if !validPrivateContactsState(state, scope) {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, state)
}

func (h *Handlers) BeginPrivateStorageMigration(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.privateSourceScope(w, r)
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
	epoch, err := uuid.Parse(input.Epoch)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	state, err := h.source.BeginPrivateStorageMigration(r.Context(), scope.Subject, epoch)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if !validPrivateContactsState(state, scope) || state.Epoch != epoch.String() || (state.Mode != "frozen" && state.Mode != "private") {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, state)
}

func privateSourceEpoch(r *http.Request) (uuid.UUID, error) {
	value := chi.URLParam(r, "epoch")
	if !storage.MigrationEpoch(value) {
		return uuid.Nil, httpx.ErrBadRequest
	}
	return uuid.Parse(value)
}

func (h *Handlers) PrivateStorageSourcePage(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.privateSourceScope(w, r)
	if !ok {
		return
	}
	epoch, err := privateSourceEpoch(r)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	after, err := storage.DecodeSourceCursor(r.URL.Query().Get("after"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	page, err := h.source.PrivateStorageSourcePage(r.Context(), scope.Subject, epoch, after)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if page.Validate(scope, epoch.String(), after) != nil {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, page)
}

func (h *Handlers) OpenPrivateStorageSource(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.privateSourceScope(w, r)
	if !ok {
		return
	}
	epoch, err := privateSourceEpoch(r)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	kind, id := chi.URLParam(r, "kind"), chi.URLParam(r, "id")
	if !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: kind, ID: id}) {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	descriptor, body, err := h.source.OpenPrivateStorageSource(r.Context(), scope.Subject, epoch, kind, id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if body == nil {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	defer func() { _ = body.Close() }()
	if descriptor.Validate() != nil || descriptor.Kind != kind || descriptor.ID != id {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(descriptor.Size, 10))
	w.Header().Set("X-Sytue-Source-SHA256", descriptor.SHA256)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, io.LimitReader(body, descriptor.Size))
}

func privateSourceAPIError(err error) error {
	switch {
	case errors.Is(store.PrivateStorageError(err), store.ErrPrivateStorageFrozen):
		return httpx.ErrConflict.WithCode("private_storage_migration_required")
	case errors.Is(err, store.ErrPrivateStorageEpoch):
		return httpx.ErrConflict.WithCode("private_storage_snapshot_changed")
	case errors.Is(err, store.ErrPrivateStorageScope):
		return httpx.ErrBadRequest
	default:
		return nil
	}
}
