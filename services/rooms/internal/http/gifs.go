// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/miniatura"
	"git.sytue.eu/sytue/one/services/rooms/internal/gifcatalog"
)

// gifHandlers sirve búsquedas y bytes a los dos clientes. El token de Sytue nunca
// se envía a Commons y ninguna imagen requiere contactar con terceros desde el dispositivo.
type gifHandlers struct {
	catalog *gifcatalog.Catalog
	slots   chan struct{}
}

func newGifHandlers() *gifHandlers {
	return &gifHandlers{catalog: gifcatalog.New(), slots: make(chan struct{}, 4)}
}

func (h *gifHandlers) search(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Query  string `json:"query"`
		Cursor string `json:"cursor"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	page, err := h.catalog.Search(r.Context(), in.Query, in.Cursor)
	if err != nil {
		gifError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, page)
}

func gifError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, gifcatalog.ErrUnavailable) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	httpx.Fail(w, r, &httpx.Error{Status: http.StatusBadGateway, Code: "gif_catalog_unavailable", Message: "The GIF catalogue could not be loaded."})
}

func (h *gifHandlers) image(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "gifID"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	case <-r.Context().Done():
		return
	}
	data, err := h.catalog.Content(r.Context(), id)
	if err != nil {
		gifError(w, r, err)
		return
	}
	kind := "image/gif"
	if r.URL.Query().Get("preview") == "1" {
		data, err = miniatura.Render(data, 180, "image/png")
		if err != nil {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		kind = "image/png"
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
