// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func (h *Handlers) MatrixResourceBootstrapSource(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.svc.(interface {
		MatrixResourceBootstrapSource(context.Context, string, string, resource.SignedBootstrapSourceQuery) (resource.BootstrapSourceSnapshot, error)
	})
	if !ok {
		httpx.Fail(w, r, toAPIError(domain.ErrMatrixUnavailable))
		return
	}
	var query resource.SignedBootstrapSourceQuery
	if err := httpx.DecodeJSONLimit(w, r, &query, 32768); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	result, err := svc.MatrixResourceBootstrapSource(r.Context(), actor, chi.URLParam(r, "product"), query)
	if err != nil {
		httpx.Fail(w, r, authorityHTTPError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}
