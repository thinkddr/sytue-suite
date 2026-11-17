// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"net/http"
	"slices"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// Ventana de deshacer el envío del dueño (GET/PUT /api/v1/undo-send).
//
// Era una constante del cliente web (10 s). Ahora la elige quien escribe y se
// guarda en la CUENTA, así que es la misma en cualquier equipo. El servidor no la
// aplica por su cuenta al enviar: el cliente la lee y la manda en `undo_seconds`
// de /send, como hasta ahora, para que un cliente que no ofrece «Deshacer» (SMTP
// desde Thunderbird o el móvil) no retenga correos que nadie podrá sacar de la cola.

// undoSendOpciones son las ventanas que se ofrecen (las de Gmail). Una lista
// cerrada y no un rango: la interfaz pinta un selector con estas cuatro, y un 17
// guardado por otro cliente no tendría opción que marcar.
var undoSendOpciones = []int{5, 10, 20, 30}

type undoSendReq struct {
	Seconds int `json:"seconds"`
}

func (a *API) getUndoSend(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	secs, err := a.store.GetUndoSendSeconds(r.Context(), acc.ID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, undoSendReq{Seconds: secs})
}

func (a *API) setUndoSend(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body undoSendReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Se rechaza en vez de redondear a la opción más cercana: guardar algo distinto
	// de lo que se pidió y contestar 204 dejaría al cliente enseñando un valor falso.
	if !slices.Contains(undoSendOpciones, body.Seconds) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("La ventana de deshacer debe ser de 5, 10, 20 o 30 segundos."))
		return
	}
	if err := a.store.SetUndoSendSeconds(r.Context(), acc.ID, body.Seconds); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}
