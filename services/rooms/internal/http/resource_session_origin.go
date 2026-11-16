// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
)

type resourceSessionOriginService interface {
	ResourceSessionOriginForDocs(context.Context, resourcepg.SessionOriginRequest) (resourcepg.SessionOriginResponse, error)
}
type resourceSessionOriginHandlers struct {
	svc    resourceSessionOriginService
	secret string
}

func (h resourceSessionOriginHandlers) current(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var request resourcepg.SessionOriginRequest
	if r.URL.RawQuery != "" || httpx.DecodeJSONLimit(w, r, &request, 4096) != nil || !request.ValidForService("docs") {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if h.svc == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	result, err := h.svc.ResourceSessionOriginForDocs(r.Context(), request)
	if err != nil {
		httpx.Fail(w, r, authorityHTTPError(err))
		return
	}
	result, err = resourcepg.SignSessionOriginResponse(h.secret, request, result)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	raw, err := json.Marshal(result)
	if err != nil || len(raw) > 128<<10 {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	httpx.JSON(w, r, http.StatusOK, result)
}

func mountResourceSessionOrigin(r chi.Router, secret string, svc resourceSessionOriginService) {
	if !resourcepg.ValidSessionServiceSecret(secret) {
		return
	}
	h := resourceSessionOriginHandlers{svc: svc, secret: secret}
	r.Route("/internal/resources/sessions/docs", func(internal chi.Router) {
		internal.Use(httpx.SoloDesdeDentro(secret))
		internal.Post("/current", h.current)
	})
}
