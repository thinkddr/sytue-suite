// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"context"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

type privateIngressEnrollmentPort interface {
	Enrollment(context.Context, storage.Scope, string) (ingress.EnrollmentState, error)
}

func (h *PrivateIngressHandlers) Enrollment(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	port, ok := h.cfg.Port.(privateIngressEnrollmentPort)
	if !ok {
		Fail(w, r, ErrUnavailable)
		return
	}
	var request ingress.EnrollmentRequest
	if DecodeJSONLimit(w, r, &request, 4096) != nil || !storage.MigrationEpoch(request.Epoch) {
		Fail(w, r, ErrBadRequest)
		return
	}
	result, err := port.Enrollment(r.Context(), scope, request.Epoch)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if result.Validate(scope, request.Epoch) != nil {
		privateIngressInvalidResponse(w, r)
		return
	}
	JSON(w, r, http.StatusOK, result)
}
