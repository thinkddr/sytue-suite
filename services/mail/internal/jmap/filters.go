// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// --- Reglas/filtros de correo (REST de conveniencia que usa el SDK web) ---
//
// Modelo propio, minimalista (ver domain.Filter): condiciones AND de subcadenas
// sobre remitente/asunto y acciones mover/marcar. La entrega entrante las aplica
// (delivery.Inbound.applyFilters). Aquí solo la gestión CRUD por parte del dueño
// de la cuenta.

// filterReq es el cuerpo JSON de crear/actualizar una regla.
type filterReq struct {
	Name         string  `json:"name"`
	Enabled      *bool   `json:"enabled"` // ausente ⇒ true
	Priority     int     `json:"priority"`
	MatchFrom    string  `json:"match_from"`
	MatchSubject string  `json:"match_subject"`
	MoveTo       *string `json:"move_to"` // id de buzón o null
	MarkRead     bool    `json:"mark_read"`
	MarkFlagged  bool    `json:"mark_flagged"`
}

// toFilter traduce el cuerpo a un domain.Filter (parseando move_to a UUID). No
// valida el modelo: eso lo hace el store (domain.Filter.Validate).
func (b filterReq) toFilter() (domain.Filter, error) {
	f := domain.Filter{
		Name:         b.Name,
		Enabled:      b.Enabled == nil || *b.Enabled,
		Priority:     b.Priority,
		MatchFrom:    b.MatchFrom,
		MatchSubject: b.MatchSubject,
		MarkRead:     b.MarkRead,
		MarkFlagged:  b.MarkFlagged,
	}
	if b.MoveTo != nil && *b.MoveTo != "" {
		id, err := uuid.Parse(*b.MoveTo)
		if err != nil {
			return domain.Filter{}, httpx.ErrBadRequest.WithMessage("Invalid move_to.")
		}
		f.MoveTo = &id
	}
	return f, nil
}

func (a *API) listFilters(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	filters, err := a.store.ListFilters(r.Context(), acc.ID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if filters == nil {
		filters = []domain.Filter{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"filters": filters})
}

func (a *API) createFilter(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body filterReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, err := body.toFilter()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	created, err := a.store.CreateFilter(r.Context(), acc.ID, f)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, created)
}

func (a *API) updateFilter(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "filterID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid filterID."))
		return
	}
	var body filterReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, err := body.toFilter()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := a.store.UpdateFilter(r.Context(), acc.ID, id, f); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}

func (a *API) deleteFilter(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "filterID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid filterID."))
		return
	}
	if err := a.store.DeleteFilter(r.Context(), acc.ID, id); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}
