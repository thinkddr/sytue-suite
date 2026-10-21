// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// ArchiveNotebook (POST) archiva un cuaderno con su rama; UnarchiveNotebook
// (DELETE) la devuelve. Los dos sobre /notebooks/{id}/archive.
func (h *Handlers) ArchiveNotebook(w http.ResponseWriter, r *http.Request) { h.archivar(w, r, true) }

// UnarchiveNotebook devuelve al árbol lo que se archivó junto.
func (h *Handlers) UnarchiveNotebook(w http.ResponseWriter, r *http.Request) {
	h.archivar(w, r, false)
}

func (h *Handlers) archivar(w http.ResponseWriter, r *http.Request, archivar bool) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.ArchiveNotebook(r.Context(), id, sub(r), archivar); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}

// PinnedNotes es la vista «Fijadas»: las notas fijadas de todos los cuadernos
// visibles, en la misma forma que un resultado de búsqueda.
func (h *Handlers) PinnedNotes(w http.ResponseWriter, r *http.Request) {
	hits, err := h.store.PinnedNotes(r.Context(), sub(r), limiteDeQuery(r, 100))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if hits == nil {
		hits = []store.Hit{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"notes": hits})
}
