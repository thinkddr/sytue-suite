// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

func (h *Handlers) MatrixNativeStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Challenge string `json:"code_challenge"`
		Method    string `json:"code_challenge_method"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	result, err := h.svc.StartMatrixNative(r.Context(), actor, req.Challenge, req.Method)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusCreated, result)
}

func (h *Handlers) MatrixNativePreview(w http.ResponseWriter, r *http.Request) {
	actor, _ := caller(r)
	result, err := h.svc.PreviewMatrixNative(r.Context(), actor, r.URL.Query().Get("state"))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h *Handlers) MatrixNativeAccept(w http.ResponseWriter, r *http.Request) {
	var req struct {
		State      string `json:"state"`
		LoginToken string `json:"login_token"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	result, err := h.svc.AcceptMatrixNative(r.Context(), actor, req.State, req.LoginToken)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusCreated, result)
}

func (h *Handlers) MatrixNativeFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		State    string `json:"state"`
		Ticket   string `json:"ticket"`
		Verifier string `json:"code_verifier"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	token, err := h.svc.FinishMatrixNative(r.Context(), actor, req.State, req.Ticket, req.Verifier)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]string{"login_token": token})
}
