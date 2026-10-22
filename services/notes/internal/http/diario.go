// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package noteshttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// DailyNotebook abre o crea el diario del día civil elegido en el cliente.
// Su reloj y zona horaria, no los del servidor, deciden qué significa «hoy».
func (h *Handlers) DailyNotebook(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Date  string `json:"date"`
		Title string `json:"title"`
		Carry bool   `json:"carry"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	titulo, err := diaValido(in.Date, in.Title)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	nb, err := h.store.DailyNotebook(r.Context(), sub(r), in.Date, titulo, in.Carry)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, nb)
}

// diaValido comprueba la fecha civil `YYYY-MM-DD` y el título del diario, y devuelve
// el título normalizado.
func diaValido(date, title string) (string, error) {
	nbIn := domain.NotebookInput{Title: title}
	if !domain.FechaDiaria(date) || nbIn.Normalize() != nil {
		return "", traducir(domain.ErrInvalidInput)
	}
	return nbIn.Title, nil
}

// DailyRange son los días con notas de `?from=&to=` (incluidos): cuentas para los
// puntos de la semana y, con `notes=1`, las notas de las vistas Semana y Días.
func (h *Handlers) DailyRange(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	notas := q.Get("notes") == "1"
	maximo := domain.MaxDiasCuentas
	if notas {
		maximo = domain.MaxDiasNotas
	}
	if !domain.RangoDiario(q.Get("from"), q.Get("to"), maximo) {
		httpx.Fail(w, r, traducir(domain.ErrInvalidInput))
		return
	}
	dias, err := h.store.DailyRange(r.Context(), sub(r), q.Get("from"), q.Get("to"), notas)
	responderDias(w, r, dias, err)
}

// ThisDay es «Este día»: `?date=` en años anteriores.
func (h *Handlers) ThisDay(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if !domain.FechaDiaria(date) {
		httpx.Fail(w, r, traducir(domain.ErrInvalidInput))
		return
	}
	dias, err := h.store.ThisDay(r.Context(), sub(r), date)
	responderDias(w, r, dias, err)
}

func responderDias(w http.ResponseWriter, r *http.Request, dias []domain.DailyDay, err error) {
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if dias == nil {
		dias = []domain.DailyDay{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"days": dias})
}
