// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// changes es el endpoint de refresco en vivo de la bandeja abierta: devuelve el
// DELTA de mensajes desde un cursor de estado (?since=), no la bandeja entera. El
// cliente lo sondea con backoff (pausado en pestaña oculta) y aplica solo lo
// cambiado. Modelo JMAP Email/changes (RFC 8621 §4.3) sobre REST de conveniencia.
//
// Contrato:
//   - ?since vacío  → arranque: state=estado actual del servidor, sin cambios.
//   - ?since=<token> → { changed:[sobres vivos], destroyed:[ids], state, has_more }.
//
// El token de estado reusa el formato del cursor keyset ("received_at,id" aquí
// aplicado a (updated_at,id)): parseCursor/formatCursor. Owner-scoped: a.account
// resuelve la cuenta del token; el store filtra por account_id (+ RLS por tenant),
// así que el canal solo entrega cambios del propio usuario.
func (a *API) changes(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	since, sinceID, err := parseCursor(r.URL.Query().Get("since"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid since."))
		return
	}
	limit := queryInt(r, 100)
	changed, destroyed, lastTS, lastID, hasMore, err := a.store.Changes(r.Context(), acc.ID, since, sinceID, limit)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if changed == nil {
		changed = []domain.Message{}
	}
	destroyedStr := make([]string, len(destroyed))
	for i, id := range destroyed {
		destroyedStr[i] = id.String()
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"changed":   changed,
		"destroyed": destroyedStr,
		"state":     formatCursor(lastTS, lastID),
		"has_more":  hasMore,
	})
}
