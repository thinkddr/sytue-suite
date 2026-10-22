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

// --- Etiquetas (labels) de usuario — REST de conveniencia que usa el SDK web ---
//
// Modelo: KEYWORD JMAP/IMAP (ver domain.Label). El token en messages.flags es el
// id de la etiqueta; aplicar/quitar una etiqueta es escribir/borrar ese keyword,
// coherente con `keywords` de Email/set y con FLAGS de IMAP. Aquí el CRUD del
// dueño de la cuenta, la aplicación en lote a un hilo y el listado filtrado por
// etiqueta (mismo contrato keyset que las carpetas).

type labelReq struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (a *API) listLabels(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	labels, err := a.store.ListLabels(r.Context(), acc.ID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if labels == nil {
		labels = []domain.Label{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"labels": labels})
}

func (a *API) createLabel(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body labelReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	created, err := a.store.CreateLabel(r.Context(), acc.ID, body.Name, body.Color)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, created)
}

func (a *API) updateLabel(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "labelID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid labelID."))
		return
	}
	var body labelReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := a.store.UpdateLabel(r.Context(), acc.ID, id, body.Name, body.Color); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}

func (a *API) deleteLabel(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "labelID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid labelID."))
		return
	}
	if err := a.store.DeleteLabel(r.Context(), acc.ID, id); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}

type applyLabelReq struct {
	MessageIDs []string `json:"message_ids"`
	Add        bool     `json:"add"`
}

// applyLabel aplica/quita una etiqueta a uno o varios mensajes (un hilo entero, o
// una multiselección) en una sola llamada. La autorización por dueño (etiqueta y
// mensajes de la cuenta) la impone el store; aquí solo se parsean los ids.
func (a *API) applyLabel(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "labelID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid labelID."))
		return
	}
	var body applyLabelReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ids := make([]uuid.UUID, 0, len(body.MessageIDs))
	for _, s := range body.MessageIDs {
		mid, err := uuid.Parse(s)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid message_ids."))
			return
		}
		ids = append(ids, mid)
	}
	if err := a.store.ApplyLabel(r.Context(), acc.ID, id, ids, body.Add); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}

// listLabelMessages pagina por KEYSET los mensajes de una etiqueta (mismo contrato
// que listMessages: `cursor` = "received_at,id", `next_cursor` para la siguiente).
func (a *API) listLabelMessages(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "labelID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid labelID."))
		return
	}
	limit := queryInt(r, 50)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	before, beforeID, err := parseCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid cursor."))
		return
	}
	msgs, err := a.store.ListMessagesByLabel(r.Context(), acc.ID, id, limit, before, beforeID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	next := ""
	if len(msgs) == limit {
		last := msgs[len(msgs)-1]
		next = formatCursor(last.ReceivedAt, last.ID)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"messages": msgs, "limit": limit, "next_cursor": next})
}
