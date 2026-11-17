// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// --- Posponer y silenciar una conversación (REST de conveniencia del BFF web) ---
//
// Las dos son del HILO del dueño: el hilo se identifica por su id, y el store lo
// busca en la cuenta del token, así que un hilo ajeno es un 404. Cambian lo que
// enseña la Entrada, así que llevan la señal de cambios del widget como mover.

type snoozeReq struct {
	Until time.Time `json:"until"`
}

type muteReq struct {
	Muted *bool `json:"muted"`
}

// threadIDDe lee {threadID} de la ruta; escribe el 400 y devuelve false si no vale.
func threadIDDe(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "threadID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid threadID."))
		return uuid.Nil, false
	}
	return id, true
}

// listSnoozes — GET /api/v1/snoozes — las conversaciones pospuestas de la cuenta y
// hasta cuándo, la que vuelve antes primero.
func (a *API) listSnoozes(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	list, err := a.store.ListSnoozes(r.Context(), acc.ID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if list == nil {
		list = []domain.Snooze{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"snoozes": list})
}

// snoozeThread — PUT /api/v1/threads/{threadID}/snooze {"until": RFC 3339} — saca
// el hilo de Recibidos hasta `until`. Sobre uno ya pospuesto, cambia la fecha.
func (a *API) snoozeThread(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, ok := threadIDDe(w, r)
	if !ok {
		return
	}
	var body snoozeReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Antes de abrir una transacción, y con el mismo error que daría el store.
	if err := domain.ValidarSnooze(body.Until, time.Now()); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	sn, err := a.store.SnoozeThread(r.Context(), acc.ID, id, body.Until)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, sn)
}

// unsnoozeThread — DELETE /api/v1/threads/{threadID}/snooze — lo devuelve ya a
// Recibidos. 404 si no estaba pospuesto.
func (a *API) unsnoozeThread(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, ok := threadIDDe(w, r)
	if !ok {
		return
	}
	if err := a.store.UnsnoozeThread(r.Context(), acc.ID, id); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}

// muteThread — PUT /api/v1/threads/{threadID}/mute {"muted": bool} — silencia el
// hilo (y archiva lo que tenga en Recibidos) o deja de silenciarlo. Devuelve el
// estado y cuántos mensajes archivó, para que el cliente los quite de la Entrada.
func (a *API) muteThread(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, ok := threadIDDe(w, r)
	if !ok {
		return
	}
	var body muteReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Sin el campo no se adivina: un cuerpo vacío no puede querer decir «deja de
	// silenciar» por ser false el valor cero.
	if body.Muted == nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Give muted: true or false."))
		return
	}
	archivados, err := a.store.MuteThread(r.Context(), acc.ID, id, *body.Muted)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"muted": *body.Muted, "archived": archivados})
}
