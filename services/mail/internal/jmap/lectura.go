// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// Preferencias de lectura del dueño (GET/PUT /api/v1/reading-prefs) y sus
// remitentes de confianza (GET/POST/DELETE /api/v1/trusted-senders).
//
// El servidor no aplica nada de esto por su cuenta: el auto-avance y los gestos son
// del cliente, y las imágenes remotas las pide el cliente a su proxy cuando la
// persona (o su lista de confianza) lo dice. Aquí solo se GUARDAN, para que sean
// las mismas en cualquier equipo.

func (a *API) getReadingPrefs(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := a.store.GetReadingPrefs(r.Context(), acc.ID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, p)
}

func (a *API) setReadingPrefs(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body domain.ReadingPrefs
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := body.Validar(); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if err := a.store.SetReadingPrefs(r.Context(), acc.ID, body); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}

type remitentesResp struct {
	Senders []string `json:"senders"`
}

type remitenteReq struct {
	Address string `json:"address"`
}

func (a *API) listTrustedSenders(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	lista, err := a.store.ListTrustedSenders(r.Context(), acc.ID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if lista == nil {
		lista = []string{}
	}
	httpx.JSON(w, r, http.StatusOK, remitentesResp{Senders: lista})
}

// addTrustedSender contesta con la dirección como quedó guardada (normalizada), para
// que el cliente compare contra ella y no contra lo que escribió.
func (a *API) addTrustedSender(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body remitenteReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	addr, err := a.store.AddTrustedSender(r.Context(), acc.ID, body.Address)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, remitenteReq{Address: addr})
}

// removeTrustedSender lleva la dirección en `?address=` y no en la ruta: una
// dirección puede llevar caracteres («/», «%», «+») que en un segmento de ruta
// pasan por demasiados decodificadores.
func (a *API) removeTrustedSender(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := a.store.RemoveTrustedSender(r.Context(), acc.ID, r.URL.Query().Get("address")); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}
