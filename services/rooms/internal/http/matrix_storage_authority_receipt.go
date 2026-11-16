// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type storageReceiptAuthority interface {
	VerifyMatrixStorageReceipt(context.Context, string, string, storage.PublicationTarget, storage.Reference) error
}

func (h storageAuthorityHandlers) receiptInternal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var request struct {
		Scope   storage.Scope             `json:"scope"`
		Target  storage.PublicationTarget `json:"target"`
		Receipt storage.Reference         `json:"receipt"`
	}
	if httpx.DecodeJSONLimit(w, r, &request, 4096) != nil || !request.Scope.Valid() || !request.Target.Valid() || !request.Receipt.Valid() || request.Target.RoomID != request.Receipt.RoomID {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	svc, ok := h.svc.(storageReceiptAuthority)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	tenant, err := tenancy.New(request.Scope.Tenant)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if err := svc.VerifyMatrixStorageReceipt(tenancy.Into(r.Context(), tenant), request.Scope.Subject, request.Scope.Product, request.Target, request.Receipt); err != nil {
		httpx.Fail(w, r, authorityHTTPError(err))
		return
	}
	httpx.NoContent(w)
}
