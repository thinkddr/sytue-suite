// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// ListNotesPage is a capability route: older backends return 404, so readers can
// fall back to one legacy note per request without risking an unbounded page.
func (h *Handlers) ListNotesPage(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.listNotesPage(w, r, id)
}

func (h *Handlers) listNotesPage(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	w.Header().Set("Cache-Control", "no-store")
	query := r.URL.Query()
	rawLimit := query.Get("limit")
	if rawLimit == "" {
		rawLimit = "100"
	}
	limit, err := strconv.Atoi(rawLimit)
	if err != nil || limit < 1 || limit > 100 || (query.Get("raw_data") != "" && query.Get("raw_data") != "0" && query.Get("raw_data") != "1") {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_note_page"))
		return
	}
	var after *uuid.UUID
	if raw := query.Get("cursor"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil || parsed == uuid.Nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_note_page"))
			return
		}
		after = &parsed
	}
	page, err := h.store.ListNotesPage(r.Context(), id, sub(r), limit, after)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if query.Get("raw_data") == "1" {
		for i := range page.Notes {
			page.Notes[i].DataJSON = string(page.Notes[i].Data)
			page.Notes[i].Data = nil
		}
	}
	h.writeNotePage(w, r, page, true)
}

func (h *Handlers) writeNotePage(w http.ResponseWriter, r *http.Request, page store.NotePage, paged bool) {
	w.Header().Set("Cache-Control", "no-store")
	body, err := encodeNotePage(page, paged, domain.MaxNotePageBytes)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func encodeNotePage(page store.NotePage, paged bool, maxBytes int) ([]byte, error) {
	body, err := httpx.EncodeJSONLimit(page, maxBytes)
	if err == nil || !errors.Is(err, httpx.ErrJSONResponseTooLarge) {
		return body, err
	}
	if !paged || len(page.Notes) < 2 {
		return nil, domain.ErrNotePageTooLarge
	}
	// JSON size is monotonic in the number of notes. Find the largest prefix
	// without repeatedly re-encoding every possible page length.
	var best []byte
	for low, high := 1, len(page.Notes)-1; low <= high; {
		mid := low + (high-low)/2
		candidate := store.NotePage{Notes: page.Notes[:mid], NextCursor: page.Notes[mid-1].ID.String()}
		encoded, err := httpx.EncodeJSONLimit(candidate, maxBytes)
		switch {
		case err == nil:
			best = encoded
			low = mid + 1
		case errors.Is(err, httpx.ErrJSONResponseTooLarge):
			high = mid - 1
		default:
			return nil, err
		}
	}
	if best == nil {
		return nil, domain.ErrNotePageTooLarge
	}
	return best, nil
}
