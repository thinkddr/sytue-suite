// SPDX-License-Identifier: Apache-2.0
package drivehttp

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

func (h *Handlers) CipherUsage(w http.ResponseWriter, r *http.Request) {
	if !cipherRequestScope(w, r) {
		return
	}
	reader, ok := h.cipher.(interface {
		CipherUsage(context.Context, string) (storage.CipherUsage, error)
	})
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	value, err := reader.CipherUsage(r.Context(), chi.URLParam(r, "product"))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	expected, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader))
	if err != nil || !value.Valid(expected) {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, value)
}
