// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

type privateStorageService interface {
	PrivateStorageState(context.Context) (store.PrivateStorageState, error)
	BeginPrivateStorageMigration(context.Context, uuid.UUID) (store.PrivateStorageState, error)
	PrivateStorageSourcePage(context.Context, uuid.UUID, store.PrivateSourceCursor, int) (store.PrivateSourcePage, error)
	OpenPrivateStorageSource(context.Context, uuid.UUID, string, string) (store.PrivateSourceBlob, io.ReadCloser, error)
}

func privateStorageAPIError(err error) error {
	err = store.PrivateStorageError(err)
	switch {
	case errors.Is(err, domain.ErrPrivateStorageFrozen):
		return httpx.ErrConflict.WithCode("private_storage_migration_required").WithMessage("Use an E2E-capable client while private storage migration is running.")
	case errors.Is(err, domain.ErrPrivateStorageEpoch):
		return httpx.ErrConflict.WithCode("private_storage_snapshot_changed").WithMessage("The private storage snapshot does not match this migration.")
	default:
		return nil
	}
}

func (h *Handlers) guardLegacyStorageWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if h.migration == nil || r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions ||
			path == "/api/v1/private-storage/migration" || strings.HasPrefix(path, "/api/v1/cipher/blobs/") ||
			strings.HasPrefix(path, "/api/v1/pq-vault/") ||
			path == "/api/v1/storage" || strings.HasPrefix(path, "/api/v1/storage/") || path == "/api/v1/quota" {
			next.ServeHTTP(w, r)
			return
		}
		state, err := h.migration.PrivateStorageState(r.Context())
		if err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		if state.Mode != "legacy" {
			httpx.Fail(w, r, toAPIError(domain.ErrPrivateStorageFrozen))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handlers) privateSourceRequest(w http.ResponseWriter, r *http.Request) bool {
	if h.migration == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return false
	}
	return storageRequestScope(w, r, "drive")
}

func (h *Handlers) PrivateStorageState(w http.ResponseWriter, r *http.Request) {
	if !h.privateSourceRequest(w, r) {
		return
	}
	state, err := h.migration.PrivateStorageState(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, state)
}

func (h *Handlers) BeginPrivateStorageMigration(w http.ResponseWriter, r *http.Request) {
	if !h.privateSourceRequest(w, r) {
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
	state, err := h.migration.BeginPrivateStorageMigration(r.Context(), epoch)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, state)
}

func privateSourceEpoch(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	value := chi.URLParam(r, "epoch")
	if !storage.MigrationEpoch(value) {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return uuid.Nil, false
	}
	epoch, err := uuid.Parse(value)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return uuid.Nil, false
	}
	return epoch, true
}

func (h *Handlers) PrivateStorageSourcePage(w http.ResponseWriter, r *http.Request) {
	if !h.privateSourceRequest(w, r) {
		return
	}
	epoch, ok := privateSourceEpoch(w, r)
	if !ok {
		return
	}
	cursor := store.PrivateSourceCursor{}
	if after := r.URL.Query().Get("after"); after != "" {
		if len(after) > 4096 {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		data, err := base64.RawURLEncoding.Strict().DecodeString(after)
		if err != nil || json.Unmarshal(data, &cursor) != nil || !storage.SourceCursorValid(cursor) {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
	}
	page, err := h.migration.PrivateStorageSourcePage(r.Context(), epoch, cursor, storage.MaxLegacySourceRows)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, page)
}

func (h *Handlers) PrivateStorageSourceBlob(w http.ResponseWriter, r *http.Request) {
	if !h.privateSourceRequest(w, r) {
		return
	}
	epoch, ok := privateSourceEpoch(w, r)
	if !ok {
		return
	}
	blob, body, err := h.migration.OpenPrivateStorageSource(r.Context(), epoch, chi.URLParam(r, "kind"), chi.URLParam(r, "source"))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	defer func() { _ = body.Close() }()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(blob.SizeBytes, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, io.LimitReader(body, blob.SizeBytes))
}
