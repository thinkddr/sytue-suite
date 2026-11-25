// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// freezeIDParam lee el id de proyecto de la ruta con el mismo mensaje que el
// resto de verbos de proyecto que no van por parseID.
func freezeIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid project id."))
		return uuid.Nil, false
	}
	return id, true
}

// GetDeployFreeze contesta si la producción del proyecto está congelada:
// {"frozen": false} o {"frozen": true, "reason", "frozen_by", "frozen_at"}.
//
// Lo lee cualquier miembro: quien va a desplegar es quien necesita saberlo
// antes de pulsar, no después de un 409.
func (h *Handlers) GetDeployFreeze(w http.ResponseWriter, r *http.Request) {
	id, ok := freezeIDParam(w, r)
	if !ok {
		return
	}
	f, congelado, err := h.svc.DeployFreeze(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if !congelado {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"frozen": false})
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"frozen": true, "reason": f.Reason, "frozen_by": f.FrozenBy, "frozen_at": f.FrozenAt,
	})
}

// FreezeDeploys congela la producción del proyecto. Cuerpo opcional {"reason"}.
func (h *Handlers) FreezeDeploys(w http.ResponseWriter, r *http.Request) {
	id, ok := freezeIDParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	f, err := h.svc.FreezeDeploys(r.Context(), id, req.Reason)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"frozen": true, "reason": f.Reason, "frozen_by": f.FrozenBy, "frozen_at": f.FrozenAt,
	})
}

// UnfreezeDeploys vuelve a abrir la producción. Idempotente.
func (h *Handlers) UnfreezeDeploys(w http.ResponseWriter, r *http.Request) {
	id, ok := freezeIDParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.UnfreezeDeploys(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}
