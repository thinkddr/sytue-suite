// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// --- Autorespuesta / vacaciones — REST de conveniencia ---
//
// Config de fuera-de-oficina por cuenta: la aplica la ENTREGA ENTRANTE (encola UNA
// autorespuesta por remitente, respetando RFC 3834 y el dedupe por ventana). Aquí
// solo el get/set del dueño. GET devuelve la config; PUT la reemplaza.

// vacationReq es el cuerpo de PUT /vacation. starts_at/ends_at son RFC3339
// opcionales (vacío = sin límite por ese extremo).
type vacationReq struct {
	Enabled  bool   `json:"enabled"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	StartsAt string `json:"starts_at"`
	EndsAt   string `json:"ends_at"`
}

func (a *API) getVacation(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := a.store.GetVacation(r.Context(), acc.ID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, v)
}

func (a *API) setVacation(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body vacationReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	starts, err := parseOptTime(body.StartsAt)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("starts_at must be an RFC3339 date."))
		return
	}
	ends, err := parseOptTime(body.EndsAt)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("ends_at must be an RFC3339 date."))
		return
	}
	v := domain.Vacation{
		Enabled:  body.Enabled,
		Subject:  body.Subject,
		Body:     body.Body,
		StartsAt: starts,
		EndsAt:   ends,
	}
	if err := a.store.SetVacation(r.Context(), acc.ID, v); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}

// --- Firma de correo — REST de conveniencia ---
//
// La firma vivía en localStorage del navegador, así que no existía fuera del
// equipo donde se escribió. Aquí es un valor por CUENTA (columna
// accounts.signature): la misma en el portátil, en el móvil y en el navegador
// de otra persona. GET la lee; PUT la reemplaza.

// maxSignatureBytes acota la firma. Es texto plano (el editor es un textarea);
// 8 KiB da de sobra para nombre, cargo, teléfono y un aviso legal largo, y evita
// que una imagen pegada como data: URI acabe en cada fila de la tabla de cuentas.
const maxSignatureBytes = 8 << 10

type signatureReq struct {
	Signature string `json:"signature"`
}

func (a *API) getSignature(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sig, err := a.store.GetSignature(r.Context(), acc.ID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, signatureReq{Signature: sig})
}

func (a *API) setSignature(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body signatureReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// El tope se comprueba aquí y no se trunca: truncar una firma por la mitad es
	// peor que rechazarla, porque el usuario no se entera hasta que alguien
	// recibe el correo cortado.
	if len(body.Signature) > maxSignatureBytes {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("La firma es demasiado larga."))
		return
	}
	if err := a.store.SetSignature(r.Context(), acc.ID, body.Signature); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}

// parseOptTime parsea una fecha RFC3339 opcional: vacía ⇒ nil (sin límite).
func parseOptTime(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil //nolint:nilnil // contrato: vacío = sin límite (nil), no un error.
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, err
	}
	u := t.UTC()
	return &u, nil
}
