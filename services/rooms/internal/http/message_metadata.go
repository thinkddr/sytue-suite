// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"net/http"
	"strconv"

	roomsearch "git.sytue.eu/sytue/one/apps/nucleo/rooms/search"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

func (h *Handlers) MessageMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	params := r.URL.Query()
	for key := range params {
		if key != "cursor" && key != "limit" {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Only an opaque cursor and page size are accepted."))
			return
		}
	}
	limit, _ := strconv.Atoi(params.Get("limit"))
	actor, _ := caller(r)
	page, err := h.svc.MessageMetadata(r.Context(), actor, params.Get("cursor"), limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, page)
}

func (h *Handlers) MessageMetadataVisible(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var input struct {
		Messages []roomsearch.Identity `json:"messages"`
	}
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	rows, err := h.svc.MessageMetadataVisible(r.Context(), actor, input.Messages)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if rows == nil {
		rows = []roomsearch.Metadata{}
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"messages": rows})
}
