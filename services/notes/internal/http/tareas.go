// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package noteshttp

import (
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// ListTasks es GET /tasks?state=open|done|cancelled&from=&to=: las tareas de quien
// pregunta en todos sus cuadernos. `from` y `to` son instantes RFC 3339.
func (h *Handlers) ListTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	estado := q.Get("state")
	if estado == "" {
		estado = store.TareaAbierta
	}
	var limites [2]*time.Time
	for i, k := range []string{"from", "to"} {
		if v := q.Get(k); v != "" {
			ts, err := time.Parse(time.RFC3339, v)
			if err != nil {
				httpx.Fail(w, r, traducir(domain.ErrInvalidInput))
				return
			}
			limites[i] = &ts
		}
	}
	if estado != store.TareaAbierta && estado != store.TareaHecha && estado != store.TareaCancelada {
		httpx.Fail(w, r, traducir(domain.ErrInvalidInput))
		return
	}
	ts, err := h.store.ListTasks(r.Context(), sub(r), estado, limites[0], limites[1])
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"tasks": ts})
}

// PostponeTask es POST /notes/{id}/postpone {date,title}: lleva la nota, con lo que
// cuelga de ella, al diario de ese día. Como en /daily, el día lo decide el cliente.
func (h *Handlers) PostponeTask(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Date  string `json:"date"`
		Title string `json:"title"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	titulo, err := diaValido(in.Date, in.Title)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	nb, err := h.store.PostponeTask(r.Context(), id, sub(r), in.Date, titulo)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, nb)
}
