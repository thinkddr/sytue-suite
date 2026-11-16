// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

type resourceNativeService interface {
	StartMatrixResourceNative(context.Context, string, string, string, string) (service.MatrixNativeStart, error)
	ApproveMatrixResourceNative(context.Context, string, string) error
	FinishMatrixResourceNative(context.Context, string, string, string, string) (store.MatrixResourceNativeGrant, error)
}

func (h *Handlers) MatrixResourceNativeStart(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.svc.(resourceNativeService)
	if !ok {
		httpx.Fail(w, r, toAPIError(domain.ErrMatrixUnavailable))
		return
	}
	var request struct {
		Challenge string `json:"code_challenge"`
		Method    string `json:"code_challenge_method"`
		Product   string `json:"product"`
	}
	if err := httpx.DecodeJSON(w, r, &request); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	value, err := svc.StartMatrixResourceNative(r.Context(), actor, request.Challenge, request.Method, request.Product)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusCreated, value)
}

func (h *Handlers) MatrixResourceNativeApprove(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.svc.(resourceNativeService)
	if !ok {
		httpx.Fail(w, r, toAPIError(domain.ErrMatrixUnavailable))
		return
	}
	var request struct {
		State string `json:"state"`
	}
	if err := httpx.DecodeJSON(w, r, &request); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	if err := svc.ApproveMatrixResourceNative(r.Context(), actor, request.State); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]bool{"approved": true})
}

func (h *Handlers) MatrixResourceNativeFinish(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.svc.(resourceNativeService)
	if !ok {
		httpx.Fail(w, r, toAPIError(domain.ErrMatrixUnavailable))
		return
	}
	var request struct {
		State    string `json:"state"`
		Ticket   string `json:"ticket"`
		Verifier string `json:"code_verifier"`
	}
	if err := httpx.DecodeJSON(w, r, &request); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	value, err := svc.FinishMatrixResourceNative(r.Context(), actor, request.State, request.Ticket, request.Verifier)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, value)
}
