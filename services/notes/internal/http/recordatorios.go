// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// ListReminders son mis recordatorios pendientes: el indicador de cada fila.
func (h *Handlers) ListReminders(w http.ResponseWriter, r *http.Request) {
	rs, err := h.store.ListReminders(r.Context(), sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"reminders": rs})
}

// SetReminder pone o cambia mi recordatorio en una nota.
func (h *Handlers) SetReminder(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in domain.ReminderInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := in.Normalize(); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	rem, err := h.store.SetReminder(r.Context(), id, sub(r), in)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, rem)
}

// DeleteReminder quita mi recordatorio de una nota.
func (h *Handlers) DeleteReminder(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.DeleteReminder(r.Context(), id, sub(r)); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}
