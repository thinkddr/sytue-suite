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

// --- Plantillas de respuesta del dueño (REST de conveniencia del BFF web) ---
//
// Un texto con nombre que el redactor inserta donde está el cursor. Se guardan
// en la CUENTA (migración 0021) para que sean las mismas en cualquier equipo,
// como la firma. Solo el dueño: una cuenta compartida no ofrece las suyas a
// quien la tiene delegada, porque el redactor las pide con el token de quien
// escribe.

type templateReq struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

func (a *API) listTemplates(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	tpls, err := a.store.ListTemplates(r.Context(), acc.ID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if tpls == nil {
		tpls = []domain.Template{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"templates": tpls})
}

func (a *API) createTemplate(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body templateReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Se valida aquí también y no solo en el store: un 400 claro antes de abrir
	// una transacción, y el fake de las pruebas no tiene que repetir la regla.
	if _, _, err := domain.NormalizarTemplate(body.Name, body.Body); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	created, err := a.store.CreateTemplate(r.Context(), acc.ID, body.Name, body.Body)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, created)
}

func (a *API) updateTemplate(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "templateID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid templateID."))
		return
	}
	var body templateReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, _, err := domain.NormalizarTemplate(body.Name, body.Body); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	updated, err := a.store.UpdateTemplate(r.Context(), acc.ID, id, body.Name, body.Body)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, updated)
}

func (a *API) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "templateID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid templateID."))
		return
	}
	if err := a.store.DeleteTemplate(r.Context(), acc.ID, id); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}
