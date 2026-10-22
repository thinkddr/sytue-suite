// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// MigrateEncryptedContent never receives a key or plaintext. The store checks
// authorization and both revisions atomically before replacing a legacy copy.
func (h *Handlers) MigrateEncryptedContent(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in domain.EncryptedContentInput
	if err := httpx.DecodeJSONLimit(w, r, &in, domain.MaxNoteRequestBody); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	note, err := h.store.MigrateEncryptedContent(r.Context(), id, sub(r), in)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, note)
}
