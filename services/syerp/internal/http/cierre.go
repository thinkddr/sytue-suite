// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// CERRAR EL EJERCICIO, y que quede cerrado.
//
// El libro era inmutable apunte a apunte pero no tenía fecha de corte: se podía
// asentar en un ejercicio ya declarado y el balance de ese año cambiaba solo.

var errPeriodoCerrado = httpx.NewError(http.StatusConflict, "periodo_cerrado",
	"That accounting period is closed. Reopen it first, or post with a later date.")

// Cierre — GET /api/v1/contabilidad/cierre — hasta cuándo está cerrado el libro
// y el historial de cierres y reaperturas.
func (h *Handlers) Cierre(w http.ResponseWriter, r *http.Request) {
	hasta, cerrado, err := h.store.CierreVigente(r.Context())
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	historial, err := h.store.HistorialDeCierres(r.Context())
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	var cerradoHasta string
	if cerrado {
		cerradoHasta = hasta.Format("2006-01-02")
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"cerrado_hasta": cerradoHasta,
		"historial":     historial,
	})
}

// Cerrar — POST /api/v1/contabilidad/cierre — mueve la fecha de corte adelante.
func (h *Handlers) Cerrar(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Hasta  string `json:"hasta"`
		Motivo string `json:"motivo"`
	}
	if err := httpx.DecodeJSON(w, r, &c); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	hasta, err := time.Parse("2006-01-02", strings.TrimSpace(c.Hasta))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("hasta: expected a date as YYYY-MM-DD."))
		return
	}
	// Cerrar en el FUTURO bloquearía el trabajo de mañana sin que nadie entienda
	// por qué. Cerrar es declarar terminado lo que ya pasó.
	if hasta.After(time.Now()) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("hasta: you cannot close a period that has not finished yet."))
		return
	}
	motivo := validation.TruncateRunes(strings.TrimSpace(c.Motivo), 1000)
	if err := h.store.Cerrar(r.Context(), hasta, motivo, sub(r)); err != nil {
		if errors.Is(err, store.ErrPeriodoCerrado) {
			// Cerrar hacia atrás sería una reapertura con otro nombre.
			httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "cierre_hacia_atras",
				"The book is already closed up to a later date. Use the reopening route, which asks for a reason."))
			return
		}
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"cerrado_hasta": hasta.Format("2006-01-02")})
}

// Reabrir — POST /api/v1/contabilidad/reapertura — retrocede la fecha de corte,
// o la quita del todo. EXIGE motivo: es lo que deja volver a tocar un ejercicio
// declarado, y su única defensa es que se vea quién y por qué.
func (h *Handlers) Reabrir(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Hasta  string `json:"hasta"` // vacío = abrir el libro entero
		Motivo string `json:"motivo"`
	}
	if err := httpx.DecodeJSON(w, r, &c); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	motivo := validation.TruncateRunes(strings.TrimSpace(c.Motivo), 1000)
	if motivo == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"motivo: reopening a closed period is exceptional; say why."))
		return
	}
	var hasta *time.Time
	if s := strings.TrimSpace(c.Hasta); s != "" {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("hasta: expected a date as YYYY-MM-DD."))
			return
		}
		hasta = &d
	}
	if err := h.store.ReabrirPeriodo(r.Context(), hasta, motivo, sub(r)); err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	abierto := ""
	if hasta != nil {
		abierto = hasta.Format("2006-01-02")
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"cerrado_hasta": abierto})
}
