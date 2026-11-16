// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

type resourceSessionService interface {
	MatrixResourceWebConsent(context.Context, string, string) (store.MatrixResourceNativeGrant, error)
	MatrixResourceSessionChallenge(context.Context, string, string, string, resource.SignedRecipient) (resource.SessionChallenge, error)
	ConfirmMatrixResourceSession(context.Context, string, string, resource.SignedSessionEnrollment) (resource.SessionRecord, error)
	MatrixResourceSession(context.Context, string, string, string) (resource.SessionRecord, error)
}

func (h *Handlers) resourceSessionService(w http.ResponseWriter, r *http.Request) (resourceSessionService, bool) {
	svc, ok := h.svc.(resourceSessionService)
	if !ok {
		httpx.Fail(w, r, toAPIError(domain.ErrMatrixUnavailable))
	}
	return svc, ok
}

func (h *Handlers) MatrixResourceWebConsent(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.resourceSessionService(w, r)
	if !ok {
		return
	}
	actor, _ := caller(r)
	value, err := svc.MatrixResourceWebConsent(r.Context(), actor, chi.URLParam(r, "product"))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusCreated, value)
}

func (h *Handlers) MatrixResourceSessionChallenge(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.resourceSessionService(w, r)
	if !ok {
		return
	}
	var request struct {
		Grant     string                   `json:"grant"`
		Recipient resource.SignedRecipient `json:"recipient"`
	}
	if err := httpx.DecodeJSON(w, r, &request); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	value, err := svc.MatrixResourceSessionChallenge(r.Context(), actor, chi.URLParam(r, "product"), request.Grant, request.Recipient)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusCreated, value)
}

func (h *Handlers) ConfirmMatrixResourceSession(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.resourceSessionService(w, r)
	if !ok {
		return
	}
	var proof resource.SignedSessionEnrollment
	if err := httpx.DecodeJSON(w, r, &proof); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	value, err := svc.ConfirmMatrixResourceSession(r.Context(), actor, chi.URLParam(r, "product"), proof)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, value)
}

func (h *Handlers) MatrixResourceSession(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.resourceSessionService(w, r)
	if !ok {
		return
	}
	actor, _ := caller(r)
	value, err := svc.MatrixResourceSession(r.Context(), actor, chi.URLParam(r, "product"), r.URL.Query().Get("device"))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, value)
}

func (h *Handlers) MatrixResourceBootstrap(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.svc.(interface {
		MatrixResourceBootstrap(context.Context, string, string) (service.MatrixBootstrap, error)
	})
	if !ok {
		httpx.Fail(w, r, toAPIError(domain.ErrMatrixUnavailable))
		return
	}
	actor, _ := caller(r)
	value, err := svc.MatrixResourceBootstrap(r.Context(), actor, chi.URLParam(r, "product"))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, value)
}
