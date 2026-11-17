// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

type publicationOriginService interface {
	BeforeMatrixPublicationOrigin(context.Context, publication.OriginEvent) (publication.OriginDecision, error)
	AcceptedMatrixPublicationOrigin(context.Context, publication.OriginEvent) (publication.OriginDecision, error)
}
type publicationOriginHandlers struct{ svc publicationOriginService }

func (h publicationOriginHandlers) before(w http.ResponseWriter, r *http.Request) {
	h.apply(w, r, false)
}

func (h publicationOriginHandlers) accepted(w http.ResponseWriter, r *http.Request) {
	h.apply(w, r, true)
}

func (h publicationOriginHandlers) apply(w http.ResponseWriter, r *http.Request, accepted bool) {
	w.Header().Set("Cache-Control", "no-store")
	var event publication.OriginEvent
	if httpx.DecodeJSONLimit(w, r, &event, 128<<10) != nil || !event.Valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if h.svc == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	var result publication.OriginDecision
	var err error
	if accepted {
		result, err = h.svc.AcceptedMatrixPublicationOrigin(r.Context(), event)
	} else {
		result, err = h.svc.BeforeMatrixPublicationOrigin(r.Context(), event)
	}
	if err != nil {
		httpx.Fail(w, r, authorityHTTPError(err))
		return
	}
	if result.Version != 1 {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	httpx.JSON(w, r, http.StatusOK, result)
}

func mountPublicationOrigin(r chi.Router, secret string, svc publicationOriginService) {
	if secret == "" {
		return
	}
	h := publicationOriginHandlers{svc: svc}
	r.Route("/internal/matrix/publications", func(internal chi.Router) {
		internal.Use(httpx.SecretoInterno(secret))
		internal.Post("/before", h.before)
		internal.Post("/accepted", h.accepted)
	})
}
