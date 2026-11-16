// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

type storagePublicationService interface {
	CancelMatrixStoragePublication(context.Context, string, string, publication.SignedRequest) (publication.Receipt, error)
	ReserveMatrixStoragePublication(context.Context, string, string, publication.SignedRequest) (publication.Receipt, error)
	ReadMatrixStoragePublication(context.Context, string, string, string, string) (publication.Receipt, error)
}
type storagePublicationHandlers struct{ svc storagePublicationService }

func (h storagePublicationHandlers) scope(w http.ResponseWriter, r *http.Request) (storage.Scope, bool) {
	scope, ok := httpx.AuthenticatedStorageScope(w, r, chi.URLParam(r, "product"), authn.PrivateStorageIdentity)
	if !ok {
		return storage.Scope{}, false
	}
	if h.svc == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return storage.Scope{}, false
	}
	return scope, true
}

func (h storagePublicationHandlers) reserve(w http.ResponseWriter, r *http.Request) {
	h.reservation(w, r, false)
}

func (h storagePublicationHandlers) cancel(w http.ResponseWriter, r *http.Request) {
	h.reservation(w, r, true)
}

func (h storagePublicationHandlers) reservation(w http.ResponseWriter, r *http.Request, cancel bool) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	var signed publication.SignedRequest
	if httpx.DecodeJSONLimit(w, r, &signed, 16<<10) != nil || !signed.Valid() || len(signed.Signatures) == 0 || signed.Scope != scope || signed.Namespace != "storage:"+scope.Product {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	var result publication.Receipt
	var err error
	if cancel {
		result, err = h.svc.CancelMatrixStoragePublication(r.Context(), scope.Subject, scope.Product, signed)
	} else {
		result, err = h.svc.ReserveMatrixStoragePublication(r.Context(), scope.Subject, scope.Product, signed)
	}
	if err != nil {
		httpx.Fail(w, r, authorityHTTPError(err))
		return
	}
	if !result.Valid() {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h storagePublicationHandlers) get(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	result, err := h.svc.ReadMatrixStoragePublication(r.Context(), scope.Subject, scope.Product, r.URL.Query().Get("room_id"), r.URL.Query().Get("transaction_id"))
	if err != nil {
		httpx.Fail(w, r, authorityHTTPError(err))
		return
	}
	if !result.Valid() {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	httpx.JSON(w, r, http.StatusOK, result)
}

type storagePublicationRecoveryService interface {
	ReadMatrixStoragePublicationRecovery(context.Context, string, string, string, string) (publication.RecoveryState, error)
}

func (h storagePublicationHandlers) getRecovery(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	service, ok := h.svc.(storagePublicationRecoveryService)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	query := r.URL.Query()
	selection := publication.RecoverySelection{Scope: scope, RoomID: query.Get("room_id"), TransactionID: query.Get("transaction_id")}
	if len(query) != 2 || len(query["room_id"]) != 1 || len(query["transaction_id"]) != 1 || !selection.Valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	result, err := service.ReadMatrixStoragePublicationRecovery(r.Context(), scope.Subject, scope.Product, selection.RoomID, selection.TransactionID)
	if err != nil {
		httpx.Fail(w, r, authorityHTTPError(err))
		return
	}
	if !result.Valid(selection) {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	httpx.JSON(w, r, http.StatusOK, result)
}
