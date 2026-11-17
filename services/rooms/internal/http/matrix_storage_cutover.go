// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func cutoverRequestScope(w http.ResponseWriter, r *http.Request) (storage.Scope, bool) {
	actor, _ := caller(r)
	tenant, err := tenancy.From(r.Context())
	scope := storage.Scope{Tenant: tenant.Slug, Subject: actor, Product: chi.URLParam(r, "product")}
	if err != nil || !scope.Valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return scope, false
	}
	err = storage.ValidateRequestScope(r.Header, scope)
	if err != nil {
		failure := httpx.ErrBadRequest
		if errors.Is(err, storage.ErrRequestScopeMismatch) {
			failure = httpx.ErrConflict
		}
		httpx.Fail(w, r, failure)
		return scope, false
	}
	return scope, true
}

func (h *Handlers) MatrixStorageMigration(w http.ResponseWriter, r *http.Request) {
	scope, ok := cutoverRequestScope(w, r)
	if !ok {
		return
	}
	state, err := h.svc.MatrixStorageMigration(r.Context(), scope.Subject, scope.Product)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, state)
}

func (h *Handlers) BeginMatrixStorageMigration(w http.ResponseWriter, r *http.Request) {
	scope, ok := cutoverRequestScope(w, r)
	if !ok {
		return
	}
	var request struct {
		Epoch string `json:"epoch"`
	}
	if err := httpx.DecodeJSONLimit(w, r, &request, 4096); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !storage.MigrationEpoch(request.Epoch) {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	state, err := h.svc.BeginMatrixStorageMigration(r.Context(), scope.Subject, scope.Product, request.Epoch)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, state)
}

func (h *Handlers) CompleteMatrixStorageMigration(w http.ResponseWriter, r *http.Request) {
	scope, ok := cutoverRequestScope(w, r)
	if !ok {
		return
	}
	var request struct {
		Epoch   string            `json:"epoch"`
		Receipt storage.Reference `json:"receipt"`
	}
	if err := httpx.DecodeJSONLimit(w, r, &request, 4096); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !storage.MigrationEpoch(request.Epoch) || !request.Receipt.Valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	state, err := h.svc.CompleteMatrixStorageMigration(r.Context(), scope.Subject, scope.Product, request.Epoch, request.Receipt)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, state)
}
