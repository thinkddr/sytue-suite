// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// ListVersions devuelve el historial de una nota (sin el texto de cada versión).
func (h *Handlers) ListVersions(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	vs, err := h.store.Versiones(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"versions": vs})
}

// GetVersion devuelve una versión entera. En un cuaderno cifrado, con su
// criptograma tal cual: la descifra el navegador.
func (h *Handlers) GetVersion(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	vid, err := uuid.Parse(chi.URLParam(r, "versionID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_id").Wrap(err))
		return
	}
	v, err := h.store.Version(r.Context(), id, vid, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, v)
}
