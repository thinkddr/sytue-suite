// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type storageDeviceFenceService interface {
	FenceStorageDevice(context.Context, ingress.ControlProof) (ingress.DeviceFence, error)
	ReadStorageDeviceFence(context.Context, ingress.ControlProof) (ingress.DeviceFence, error)
}

func (h storageAuthorityHandlers) deviceFenceInternal(w http.ResponseWriter, r *http.Request) {
	h.deviceFence(w, r, true)
}

func (h storageAuthorityHandlers) deviceFenceStateInternal(w http.ResponseWriter, r *http.Request) {
	h.deviceFence(w, r, false)
}

// Both endpoints are mounted only beneath the existing internal shared-secret
// gate. State is read-only, including when a fresh resolve signature is used.
func (h storageAuthorityHandlers) deviceFence(w http.ResponseWriter, r *http.Request, beginOnly bool) {
	w.Header().Set("Cache-Control", "no-store")
	var proof ingress.ControlProof
	if httpx.DecodeJSONLimit(w, r, &proof, 64<<10) != nil || proof.Validate(proof.Request.Scope, false) != nil || proof.Request.Kind != "task.handoff" || proof.Request.Handoff == nil || beginOnly && proof.Request.Handoff.Phase != "begin" {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	service, ok := h.svc.(storageDeviceFenceService)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	tenant, err := tenancy.New(proof.Request.Scope.Tenant)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	ctx := tenancy.Into(r.Context(), tenant)
	var out ingress.DeviceFence
	if beginOnly {
		out, err = service.FenceStorageDevice(ctx, proof)
	} else {
		out, err = service.ReadStorageDeviceFence(ctx, proof)
	}
	if err != nil {
		httpx.Fail(w, r, authorityHTTPError(err))
		return
	}
	if out.ValidateProof(proof) != nil {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	httpx.JSON(w, r, http.StatusOK, out)
}
