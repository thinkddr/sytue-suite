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

func (h *Handlers) MatrixStorageBootstrap(w http.ResponseWriter, r *http.Request) {
	actor, _ := caller(r)
	result, err := h.svc.BootstrapMatrixStorage(r.Context(), actor, r.URL.Query().Get("product"))
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

// MatrixRecoveryKey devuelve la clave de recuperación custodiada de quien llama
// (ADR 0081): 404 si todavía no hay ninguna.
func (h *Handlers) MatrixRecoveryKey(w http.ResponseWriter, r *http.Request) {
	actor, _ := caller(r)
	key, err := h.svc.MatrixRecoveryKey(r.Context(), actor)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]string{"recovery_key": key})
}

// StoreMatrixRecoveryKey custodia la clave que el navegador acaba de generar.
func (h *Handlers) StoreMatrixRecoveryKey(w http.ResponseWriter, r *http.Request) {
	actor, _ := caller(r)
	var req struct {
		RecoveryKey string `json:"recovery_key"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.StoreMatrixRecoveryKey(r.Context(), actor, req.RecoveryKey); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
