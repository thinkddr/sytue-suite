// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// # Vistas de filtro personales de Calcus
//
// Una vista de filtro es la foto con nombre de los criterios del autofiltro de una hoja
// y es DE UNA PERSONA. Vivían en su navegador; aquí viajan con ella entre dispositivos.
// Nunca entran en el modelo compartido, y la ruta no lleva `sub`: siempre es el de quien
// llama, así que no hay manera de pedir las de otra persona ni siquiera siendo el
// propietario (el store tampoco tiene una consulta que lo permita).

// vistasFiltroStore es lo que esto necesita del store (como peticionesStore).
type vistasFiltroStore interface {
	VistasFiltro(ctx context.Context, docID, sub string) (json.RawMessage, bool, error)
	GuardarVistasFiltro(ctx context.Context, docID, sub string, estado json.RawMessage) error
}

// Cotas del estado: ninguna persona necesita más, y un cliente que las rebasa es un cliente roto.
const (
	maxBytesVistasFiltro = 64 * 1024
	maxHojasVistas       = 256
	maxVistasPorHoja     = 50 // = MAX_VIEWS de editor/sheets/filtro.ts
)

// estadoVistasFiltro es la forma que guarda el cliente (`FilterViewsState`): las vistas
// por hoja y la activa de cada una. Las vistas se guardan opacas: el cliente valida sus
// criterios al leerlas, y un servidor que los conociera sería una tercera copia de la semántica.
type estadoVistasFiltro struct {
	Views  map[string][]json.RawMessage `json:"views"`
	Active map[string]string            `json:"active"`
}

func (e *estadoVistasFiltro) validar() error {
	if len(e.Views) > maxHojasVistas || len(e.Active) > maxHojasVistas {
		return errors.New("too many sheets")
	}
	for hoja, vs := range e.Views {
		if len(vs) > maxVistasPorHoja {
			return errors.New("too many views in sheet " + hoja)
		}
	}
	return nil
}

// vistasFiltroDe autoriza (lector basta: filtrar es ver) y devuelve el documento y quien llama.
func (h *Handler) vistasFiltroDe(w http.ResponseWriter, r *http.Request) (id, sub string, ok bool) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return "", "", false
	}
	doc, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleViewer)
	if err != nil {
		fail(w, r, err)
		return "", "", false
	}
	if doc.Kind != domain.KindSpreadsheet {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Filter views only exist in spreadsheets."))
		return "", "", false
	}
	return doc.ID, subj.Sub, true
}

// getVistasFiltro — GET /documents/{id}/filter-views → {"state": {views, active}|null}.
func (h *Handler) getVistasFiltro(w http.ResponseWriter, r *http.Request) {
	id, sub, ok := h.vistasFiltroDe(w, r)
	if !ok {
		return
	}
	estado, hay, err := h.vistasFiltro.VistasFiltro(r.Context(), id, sub)
	if err != nil {
		fail(w, r, err)
		return
	}
	if !hay {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"state": nil})
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"state": estado})
}

// putVistasFiltro — PUT /documents/{id}/filter-views body {"state": {views, active}}.
// Sustituye las de quien llama (último gana).
func (h *Handler) putVistasFiltro(w http.ResponseWriter, r *http.Request) {
	id, sub, ok := h.vistasFiltroDe(w, r)
	if !ok {
		return
	}
	var body struct {
		State estadoVistasFiltro `json:"state"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytesVistasFiltro)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The filter views are not valid or are too large."))
		return
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unexpected data after the filter views."))
		return
	}
	if err := body.State.validar(); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The filter views exceed the limits: "+err.Error()+"."))
		return
	}
	if body.State.Views == nil {
		body.State.Views = map[string][]json.RawMessage{}
	}
	if body.State.Active == nil {
		body.State.Active = map[string]string{}
	}
	estado, err := json.Marshal(body.State)
	if err != nil {
		fail(w, r, err)
		return
	}
	if err := h.vistasFiltro.GuardarVistasFiltro(r.Context(), id, sub, estado); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}
