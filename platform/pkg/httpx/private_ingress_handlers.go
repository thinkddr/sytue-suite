// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type PrivateIngressPort interface {
	Challenge(context.Context, storage.Scope, ingress.ControlRequest) (matrixproof.Action, error)
	Apply(context.Context, storage.Scope, ingress.ControlRequest, matrixproof.SignedAction) (ingress.ControlResult, error)
	Pending(context.Context, storage.Scope, string, string, int) (ingress.PendingPage, error)
	Revalidate(context.Context, storage.Scope, ingress.ControlRequest, matrixproof.SignedAction, ingress.Claim) (matrixproof.Authorization, error)
	Open(context.Context, storage.Scope, ingress.ControlRequest, matrixproof.SignedAction, ingress.Claim) (storage.StreamSourceRow, io.ReadCloser, error)
}
type PrivateIngressHTTPConfig struct {
	Product      string
	Port         PrivateIngressPort
	Identity     func(context.Context) (PrivateSourceIdentity, bool)
	MapError     func(error) *Error
	MaxBodyBytes int64
}
type PrivateIngressHandlers struct{ cfg PrivateIngressHTTPConfig }

func NewPrivateIngressHandlers(cfg PrivateIngressHTTPConfig) *PrivateIngressHandlers {
	return &PrivateIngressHandlers{cfg: cfg}
}

func (h *PrivateIngressHandlers) scope(w http.ResponseWriter, r *http.Request) (storage.Scope, bool) {
	scope, ok := AuthenticatedStorageScope(w, r, h.cfg.Product, h.cfg.Identity)
	if !ok {
		return storage.Scope{}, false
	}
	if h.cfg.Port == nil || h.cfg.MaxBodyBytes < 1 || h.cfg.MaxBodyBytes > storage.MaxFileBytes {
		Fail(w, r, ErrUnavailable)
		return storage.Scope{}, false
	}
	return scope, true
}

func (h *PrivateIngressHandlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	privateStorageHTTPFailure(w, r, err, h.cfg.MapError)
}

func privateIngressInvalidResponse(w http.ResponseWriter, r *http.Request) {
	Fail(w, r, ErrInternal.WithCode("private_ingress_invalid_response"))
}

func (h *PrivateIngressHandlers) Challenge(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	var request ingress.ControlRequest
	if DecodeJSONLimit(w, r, &request, 32<<10) != nil || request.Validate(scope, request.Epoch) != nil {
		Fail(w, r, ErrBadRequest)
		return
	}
	result, err := h.cfg.Port.Challenge(r.Context(), scope, request)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	intent, err := matrixproof.IntentFor(scope, result.Generation, request.Epoch, request.Kind, request.Target, request)
	if err != nil || result.Intent() != intent || !result.Valid(time.Now()) {
		privateIngressInvalidResponse(w, r)
		return
	}
	JSON(w, r, http.StatusCreated, result)
}

func (h *PrivateIngressHandlers) Apply(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	var request ingress.ControlProof
	if DecodeJSONLimit(w, r, &request, 64<<10) != nil || request.Validate(scope, false) != nil {
		Fail(w, r, ErrBadRequest)
		return
	}
	result, err := h.cfg.Port.Apply(r.Context(), scope, request.Request, request.Proof)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if result.Validate(request.Request, request.Proof.Action) != nil {
		privateIngressInvalidResponse(w, r)
		return
	}
	JSON(w, r, http.StatusOK, result)
}

func (h *PrivateIngressHandlers) Pending(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	var request ingress.PendingRequest
	if DecodeJSONLimit(w, r, &request, 4096) != nil || !storage.MigrationEpoch(request.Epoch) || (request.After != "" && !storage.Identifier(request.After)) || request.Limit < 1 || request.Limit > ingress.MaxPendingTasks {
		Fail(w, r, ErrBadRequest)
		return
	}
	result, err := h.cfg.Port.Pending(r.Context(), scope, request.Epoch, request.After, request.Limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if result.Validate(scope, request.Epoch, request.After) != nil || len(result.Tasks) > request.Limit {
		privateIngressInvalidResponse(w, r)
		return
	}
	for _, task := range result.Tasks {
		if task.Cipher.Size > h.cfg.MaxBodyBytes {
			privateIngressInvalidResponse(w, r)
			return
		}
	}
	JSON(w, r, http.StatusOK, result)
}

func (h *PrivateIngressHandlers) Revalidate(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	var request ingress.ControlProof
	if DecodeJSONLimit(w, r, &request, 64<<10) != nil || request.Validate(scope, true) != nil {
		Fail(w, r, ErrBadRequest)
		return
	}
	result, err := h.cfg.Port.Revalidate(r.Context(), scope, request.Request, request.Proof, *request.Claim)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if result.Validate(request.Proof.Action) != nil {
		privateIngressInvalidResponse(w, r)
		return
	}
	JSON(w, r, http.StatusOK, result)
}

func (h *PrivateIngressHandlers) Body(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	var request ingress.ControlProof
	if DecodeJSONLimit(w, r, &request, 64<<10) != nil || request.Validate(scope, true) != nil {
		Fail(w, r, ErrBadRequest)
		return
	}
	row, body, err := h.cfg.Port.Open(r.Context(), scope, request.Request, request.Proof, *request.Claim)
	servePrivateStorageBody(w, r, row, body, err, row == request.Claim.Task.Cipher, h.cfg.MaxBodyBytes, h.cfg.MapError)
}

func (h *PrivateIngressHandlers) Mount(api chi.Router) {
	api.Post("/private-storage/ingress/enrollment", h.Enrollment)
	api.Post("/private-storage/ingress/challenge", h.Challenge)
	api.Post("/private-storage/ingress/apply", h.Apply)
	api.Post("/private-storage/ingress/pending", h.Pending)
	api.Post("/private-storage/ingress/revalidate", h.Revalidate)
	api.Post("/private-storage/ingress/body", h.Body)
	api.Post("/private-storage/ingress/inspect", h.Inspect)
	api.Post("/private-storage/ingress/handoff/body", h.HandoffBody)
}
