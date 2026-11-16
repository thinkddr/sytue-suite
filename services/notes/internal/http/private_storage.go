// SPDX-License-Identifier: Apache-2.0
package noteshttp

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

type privateNotesSource interface {
	PrivateStorageState(context.Context, string) (storage.AppMigrationState, error)
	BeginPrivateStorageMigration(context.Context, string, uuid.UUID) (storage.AppMigrationState, error)
	PrivateStorageSourcePage(context.Context, string, uuid.UUID, storage.LegacySourceCursor, int) (storage.StreamSourcePage, error)
	PrivateStorageSourceBody(context.Context, string, uuid.UUID, string, string) (store.PrivateSourceBody, error)
}

func (h *Handlers) privateNotesScope(w http.ResponseWriter, r *http.Request) (storage.Scope, bool) {
	tenant, err := tenancy.From(r.Context())
	scope := storage.Scope{Tenant: tenant.Slug, Subject: sub(r), Product: "notes"}
	if err != nil || !scope.Valid() {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return scope, false
	}
	if err := storage.ValidateRequestScope(r.Header, scope); err != nil {
		failure := httpx.ErrBadRequest
		if errors.Is(err, storage.ErrRequestScopeMismatch) {
			failure = httpx.ErrConflict.WithCode("storage_identity_changed")
		}
		httpx.Fail(w, r, failure)
		return scope, false
	}
	if h.migration == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return scope, false
	}
	return scope, true
}

func privateNotesEpoch(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := chi.URLParam(r, "epoch")
	if !storage.MigrationEpoch(raw) {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return uuid.Nil, false
	}
	epoch, err := uuid.Parse(raw)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return uuid.Nil, false
	}
	return epoch, true
}

func (h *Handlers) PrivateStorageState(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.privateNotesScope(w, r)
	if !ok {
		return
	}
	state, err := h.migration.PrivateStorageState(r.Context(), scope.Subject)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, state)
}

func (h *Handlers) BeginPrivateStorage(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.privateNotesScope(w, r)
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
	state, err := h.migration.BeginPrivateStorageMigration(r.Context(), scope.Subject, epoch)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, state)
}

func (h *Handlers) PrivateStoragePage(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.privateNotesScope(w, r)
	if !ok {
		return
	}
	epoch, ok := privateNotesEpoch(w, r)
	if !ok {
		return
	}
	after, err := storage.DecodeSourceCursor(r.URL.Query().Get("after"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	page, err := h.migration.PrivateStorageSourcePage(r.Context(), scope.Subject, epoch, after, storage.MaxLegacySourceRows)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, page)
}

func (h *Handlers) PrivateStorageBody(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.privateNotesScope(w, r)
	if !ok {
		return
	}
	epoch, ok := privateNotesEpoch(w, r)
	if !ok {
		return
	}
	body, err := h.migration.PrivateStorageSourceBody(r.Context(), scope.Subject, epoch, chi.URLParam(r, "kind"), chi.URLParam(r, "source"))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	defer clear(body.Data)
	w.Header().Set("Cache-Control", "no-store, no-transform")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(body.Size, 10))
	w.Header().Set("X-Sytue-Source-SHA256", body.SHA256)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body.Data)
}

func privateNotesAPIError(err error) error {
	switch {
	case errors.Is(err, store.ErrPrivateStorageFrozen):
		return httpx.ErrConflict.WithCode("private_storage_migration_required").WithMessage("Use a verified private client to change these notes.")
	case errors.Is(err, store.ErrPrivateStorageEpoch):
		return httpx.ErrConflict.WithCode("private_storage_snapshot_changed")
	case errors.Is(err, store.ErrPrivateStorageShared):
		return httpx.ErrConflict.WithCode("private_notes_shared_migration_required").WithMessage("Shared notebooks need encrypted recipient access before migration. Existing notes remain available.")
	case errors.Is(err, store.ErrPrivateStorageAttachment):
		return httpx.ErrConflict.WithCode("private_notes_attachment_import_required").WithMessage("Attachment content must be privately imported before migration. Existing notes remain available.")
	default:
		return nil
	}
}
