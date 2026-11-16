// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"context"
	"io"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type privateIngressHandoffPort interface {
	Inspect(context.Context, storage.Scope, string, string) (ingress.TaskState, error)
	OpenHandoff(context.Context, storage.Scope, ingress.ControlRequest, matrixproof.SignedAction) (storage.StreamSourceRow, io.ReadCloser, error)
}

func (h *PrivateIngressHandlers) Inspect(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	port, ok := h.cfg.Port.(privateIngressHandoffPort)
	if !ok {
		Fail(w, r, ErrUnavailable)
		return
	}
	var request ingress.InspectRequest
	if DecodeJSONLimit(w, r, &request, 4096) != nil || !storage.MigrationEpoch(request.Epoch) || !storage.Identifier(request.ID) {
		Fail(w, r, ErrBadRequest)
		return
	}
	result, err := port.Inspect(r.Context(), scope, request.Epoch, request.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if result.Validate(scope, request.Epoch, request.ID) != nil || result.Task.Cipher.Size > h.cfg.MaxBodyBytes {
		privateIngressInvalidResponse(w, r)
		return
	}
	JSON(w, r, http.StatusOK, result)
}

func (h *PrivateIngressHandlers) HandoffBody(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	port, ok := h.cfg.Port.(privateIngressHandoffPort)
	if !ok {
		Fail(w, r, ErrUnavailable)
		return
	}
	var request ingress.HandoffBodyRequest
	if DecodeJSONLimit(w, r, &request, 64<<10) != nil || request.Validate(scope) != nil {
		Fail(w, r, ErrBadRequest)
		return
	}
	row, body, err := port.OpenHandoff(r.Context(), scope, request.Control.Request, request.Control.Proof)
	servePrivateStorageBody(w, r, row, body, err, row == request.Task.Cipher, h.cfg.MaxBodyBytes, h.cfg.MapError)
}
