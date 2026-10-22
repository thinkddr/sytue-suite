// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"net/http"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// Acciones en LOTE sobre una multiselección de la lista (archivar/papelera/spam,
// mover-a, marcar leído/no leído). Cada una aplica el cambio a N mensajes en UNA
// operación set-based del store (id = ANY), no N round-trips. La autorización por
// dueño (mensajes y buzón destino de la cuenta; ids ajenos omitidos) la impone el
// store; aquí solo se parsean los ids. Etiquetar en lote reusa /labels/{id}/apply.

type batchMoveReq struct {
	IDs       []string `json:"ids"`
	MailboxID string   `json:"mailbox_id"`
}

// batchMove mueve la selección a un buzón (archivar/papelera/spam/mover-a).
func (a *API) batchMove(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body batchMoveReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	dest, err := uuid.Parse(body.MailboxID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid mailbox_id."))
		return
	}
	ids, err := parseUUIDs(body.IDs)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	moved, err := a.store.MoveMessages(r.Context(), acc.ID, ids, dest)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"moved": moved})
}

type batchFlagsReq struct {
	IDs []string `json:"ids"`
	// Flag es el keyword JMAP ($seen/$flagged) o el flag IMAP crudo; se traduce con
	// keywordToFlag (los desconocidos pasan tal cual), como en /flags por mensaje.
	Flag string `json:"flag"`
	Add  bool   `json:"add"`
}

// batchFlags marca leído/no leído (o destaca) la selección en un solo UPDATE.
func (a *API) batchFlags(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body batchFlagsReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.Flag == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("flag requerido."))
		return
	}
	ids, err := parseUUIDs(body.IDs)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	n, err := a.store.SetFlagsBatch(r.Context(), acc.ID, ids, keywordToFlag(body.Flag), body.Add)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"updated": n})
}

// parseUUIDs convierte los ids de texto a uuid, rechazando los MAL FORMADOS (error
// de cliente, 400). Los ids bien formados pero ajenos a la cuenta NO se rechazan
// aquí: el store los omite por su filtro owner (account_id = cuenta). Lista vacía
// es válida (no-op): el store devuelve 0 afectados.
func parseUUIDs(raw []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, httpx.ErrBadRequest.WithMessage("Invalid ids.")
		}
		ids = append(ids, id)
	}
	return ids, nil
}
