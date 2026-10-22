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

// scheduledItem es la forma de un programado en la vista «Programados». El
// cursor keyset se forma con (release_at, id) — mismo formato que listMessages.
type scheduledItem struct {
	ID         string   `json:"id"`
	From       string   `json:"from"`
	Recipients []string `json:"recipients"`
	Subject    string   `json:"subject"`
	ReleaseAt  string   `json:"release_at"`
	CreatedAt  string   `json:"created_at"`
}

// listScheduled pagina por KEYSET (release_at,id) los programados pendientes del
// usuario. `cursor` vacío = primera página; la respuesta trae `next_cursor`.
// Solo del dueño (el store filtra por account_id además de RLS por tenant).
func (a *API) listScheduled(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	limit := queryInt(r, 50)
	before, beforeID, err := parseCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid cursor."))
		return
	}
	rows, err := a.store.ListScheduled(r.Context(), acc.ID, limit, before, beforeID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	items := make([]scheduledItem, 0, len(rows))
	for _, o := range rows {
		items = append(items, scheduledItem{
			ID: o.ID.String(), From: o.From, Recipients: o.Recipients, Subject: o.Subject,
			ReleaseAt: o.NextRetryAt.UTC().Format(time.RFC3339), CreatedAt: o.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	next := ""
	if len(rows) == cappedLimit(limit) {
		last := rows[len(rows)-1]
		next = formatCursor(last.NextRetryAt, last.ID)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"scheduled": items, "next_cursor": next})
}

type rescheduleReq struct {
	SendAt string `json:"send_at"`
}

// rescheduleScheduled cambia la hora de un programado del usuario. send_at debe
// ser futuro. ErrScheduleGone (409) si ya se liberó / no es del usuario.
func (a *API) rescheduleScheduled(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid id."))
		return
	}
	var body rescheduleReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	releaseAt, hold, err := resolveRelease(body.SendAt, 0, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if hold != domain.HoldScheduled {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("send_at es obligatorio."))
		return
	}
	if err := a.store.RescheduleHeld(r.Context(), id, acc.ID, releaseAt); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "release_at": releaseAt.UTC().Format(time.RFC3339)})
}

// cancelOutbound cancela un diferido (programado o deshacer) del usuario. Es la
// carrera clave con el worker y es idempotente-consciente: si ya se liberó (salió),
// ErrScheduleGone (409, «demasiado tarde») — nunca un doble efecto. El .eml queda
// retenido en el blob de la fila cancelada (no se borra): el correo no se pierde.
//
// El contenido para editar/reenviar lo reabre el CLIENTE: en «Deshacer» desde su
// propio estado (rich-text intacto); en «cancelar programado» se descarta (opción
// permitida por el roadmap). No se recrea un borrador HTML server-side porque el
// lector de borradores extrae solo la parte text/plain (un borrador HTML se
// reabriría vacío) — evitamos ese medio-borrador roto. `kind` distingue el caso.
func (a *API) cancelOutbound(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid id."))
		return
	}
	_, hold, err := a.store.CancelHeld(r.Context(), id, acc.ID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"canceled": true, "kind": hold})
}

// cappedLimit replica el clamp de ListScheduled para detectar la página llena.
func cappedLimit(limit int) int {
	if limit <= 0 || limit > 200 {
		return 50
	}
	return limit
}
