// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package casthttp

import (
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cast/internal/diapositivas"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
)

// «Presentar con Cast» (ADR 0061, ampliación): quien presenta desde Sytue Diapos avisa de
// cada diapositiva con `PUT /lives/{id}/slide` y quien mira la lee con
// `GET /videos/{id}/slides` (y su gemela pública). Solo la posición viaja: número y título.

type diapositivaIn struct {
	Index int    `json:"index"`
	Total int    `json:"total"`
	Title string `json:"title"`
	Ref   string `json:"ref"`
}

type marcaDTO struct {
	AtMS  int64  `json:"at_ms"`
	Index int    `json:"index"`
	Title string `json:"title"`
}

type presentacionDTO struct {
	Ref       string     `json:"ref,omitempty"`
	Index     int        `json:"index"`
	Total     int        `json:"total"`
	Title     string     `json:"title"`
	StartedAt time.Time  `json:"started_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Marks     []marcaDTO `json:"marks"`
}

func presentacionDTOde(e diapositivas.Estado) presentacionDTO {
	marks := make([]marcaDTO, 0, len(e.Marks))
	for _, m := range e.Marks {
		marks = append(marks, marcaDTO{AtMS: m.AtMS, Index: m.Index, Title: m.Title})
	}
	return presentacionDTO{
		Ref: e.Ref, Index: e.Index, Total: e.Total, Title: e.Title,
		StartedAt: e.StartedAt, UpdatedAt: e.UpdatedAt, Marks: marks,
	}
}

// AvanzarDiapositiva — PUT /api/v1/lives/{id}/slide {index, total, title?, ref?} → la presentación.
func (h *Handlers) AvanzarDiapositiva(w http.ResponseWriter, r *http.Request) {
	if h.diapositivas == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("Presenting with Cast is not available on this deployment."))
		return
	}
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in diapositivaIn
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	e, err := h.diapositivas.Avanzar(r.Context(), quienDe(r), id, diapositivas.Posicion{Index: in.Index, Total: in.Total, Title: in.Title, Ref: in.Ref})
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, presentacionDTOde(e))
}

// Diapositivas — GET /api/v1/videos/{id}/slides: {"presentation": null} si no lleva ninguna.
func (h *Handlers) Diapositivas(w http.ResponseWriter, r *http.Request) {
	h.diapositivasDe(w, r, quienDe(r))
}

// DiapositivasPublico — GET /public/videos/{id}/slides, sin sesión.
func (h *Handlers) DiapositivasPublico(w http.ResponseWriter, r *http.Request) {
	h.diapositivasDe(w, r, service.Quien{Anonimo: true, IP: quienDe(r).IP})
}

func (h *Handlers) diapositivasDe(w http.ResponseWriter, r *http.Request, quien service.Quien) {
	if h.diapositivas == nil {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"presentation": nil})
		return
	}
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	e, hay, err := h.diapositivas.Ver(r.Context(), quien, id)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if !hay {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"presentation": nil})
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"presentation": presentacionDTOde(e)})
}
