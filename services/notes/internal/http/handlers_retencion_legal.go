// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Retención legal de Sytue Notepads: preservar contenido frente al recolector de
// papelera mientras dure un litigio, una inspección o un plazo sectorial.
//
// ACCESO PRIVILEGIADO, y de los caros: un hold decide qué NO se destruye de la
// organización entera. Va tras `RequireRole("owner","admin")` en el montaje, y
// las tres operaciones dejan traza —crearlo y liberarlo se anotan en el
// `audit_log` dentro de la misma transacción—.
//
// PRESERVAR NO ES LEER. Esto impide que el recolector destruya; no da a nadie
// acceso al contenido de nadie. Notes no tiene eDiscovery como Drive o Mail, y
// añadirlo es otra decisión que empieza por si un admin puede leer las notas
// ajenas — no se cuela por aquí.
package noteshttp

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// peticionDeHold es el cuerpo de la creación. Los tres campos son opcionales:
// sin `subject` el hold cubre a TODA la organización, que es el caso normal al
// abrir un caso —se preserva todo y luego se acota—.
type peticionDeHold struct {
	Subject string `json:"subject"`
	Reason  string `json:"reason"`
	CaseRef string `json:"case_ref"`
}

// CreateHold abre una retención legal.
func (h *Handlers) CreateHold(w http.ResponseWriter, r *http.Request) {
	var req peticionDeHold
	if r.Body != nil {
		// Cuerpo vacío es válido: es el hold de organización entera sin caso
		// anotado todavía. Solo se rechaza el JSON roto, que sí es un error de
		// quien llama.
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The request body is not valid JSON."))
			return
		}
	}
	hold, err := h.store.CreateHold(r.Context(),
		strings.TrimSpace(req.Subject), strings.TrimSpace(req.Reason),
		strings.TrimSpace(req.CaseRef), sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, hold)
}

// ListHolds devuelve los holds de la organización, activos y liberados.
func (h *Handlers) ListHolds(w http.ResponseWriter, r *http.Request) {
	holds, err := h.store.ListHolds(r.Context(), limiteDeQuery(r, 200))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if holds == nil {
		holds = []domain.LegalHold{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"holds": holds})
}

// ReleaseHold lo levanta: el contenido que cubría vuelve a ser destruible en la
// siguiente pasada del recolector.
//
// Es POST y no DELETE porque no borra nada: la fila se queda con su fecha y su
// autor. Lo que una auditoría pregunta no es qué está preservado ahora, sino qué
// lo estuvo y entre qué fechas.
func (h *Handlers) ReleaseHold(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "holdID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The hold id is not a valid UUID."))
		return
	}
	hold, err := h.store.ReleaseHold(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, hold)
}
