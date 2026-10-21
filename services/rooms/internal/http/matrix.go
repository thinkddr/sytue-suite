// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

func (h *Handlers) MatrixBootstrap(w http.ResponseWriter, r *http.Request) {
	actor, name := caller(r)
	result, err := h.svc.BootstrapMatrix(r.Context(), actor, name)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h *Handlers) MatrixAccountAccess(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject string `json:"subject"`
		Status  string `json:"status"`
		Version int64  `json:"version"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.MatrixAccountAccess(r.Context(), req.Subject, req.Status, req.Version); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) MatrixTenantAccess(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject string `json:"subject"`
		Tenant  string `json:"tenant"`
		Status  string `json:"status"`
		Version int64  `json:"version"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.MatrixTenantAccess(r.Context(), req.Subject, req.Tenant, req.Status, req.Version); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
