// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// QUIÉN PUEDE CONSULTAR CADA CONEXIÓN (NEG-06, decidido por el dueño el 2026-10-16).
//
// Una conexión habla con el tercero con la credencial de la ORGANIZACIÓN, y hasta ahora
// bastaba con ser miembro para usarla: con Databricks, eso era cualquier SQL contra el
// almacén de la empresa. Ahora la consultan los administradores, quien la creó y quien
// esté en su lista (personas o grupos). La lista la gestionan quien la creó y los
// administradores.
//
// Lo que es CONSULTAR: la consulta tabular (Calcus, el ERP), el informe incrustado y la
// búsqueda federada del Hub. NO lo es «traerse lo suyo» (/migrations/self): solo lee los
// datos de quien llama, nunca los de otro, y sigue abierta a cualquiera.

// maxLectores acota la lista: con grupos no hace falta nombrar a la organización entera.
const maxLectores = 200

// errNoEnLaLista es lo que recibe quien no está en la lista. El código es estable: con
// él cada interfaz pone el aviso en el idioma de quien lo lee.
var errNoEnLaLista = httpx.NewError(http.StatusForbidden, "connection_not_allowed",
	"No estás en la lista de quién puede consultar esta conexión. Pide a quien la administra que te añada.")

// esAdmin dice si quien llama administra la organización.
func esAdmin(c authn.Claims) bool { return c.HasAnyRole("owner", "admin") }

// puedeConsultar deja pasar a los administradores, a quien creó la conexión y a quien
// esté en su lista, a su nombre o por uno de sus grupos.
func (h *Handlers) puedeConsultar(ctx context.Context, id string) error {
	c, _ := authn.FromContext(ctx)
	if esAdmin(c) {
		return nil
	}
	ids, err := h.store.Consultables(ctx, c.Subject, c.Groups)
	if err != nil {
		return toAPIError(err)
	}
	if !ids[id] {
		return errNoEnLaLista
	}
	return nil
}

type lectoresWire struct {
	CreatedBy string          `json:"created_by"`
	Readers   []domain.Lector `json:"readers"`
}

// GetReaders — GET /api/v1/instances/{id}/readers — quién creó la conexión y su lista.
// Solo para quien la gestiona.
func (h *Handlers) GetReaders(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	creadaPor, lista, err := h.store.Lectores(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if !gestiona(r.Context(), creadaPor) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	httpx.JSON(w, r, http.StatusOK, lectoresWire{CreatedBy: creadaPor, Readers: lista})
}

// PutReaders — PUT /api/v1/instances/{id}/readers — sustituye la lista entera.
func (h *Handlers) PutReaders(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Readers []domain.Lector `json:"readers"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	lista, err := listaValida(req.Readers)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	creadaPor, _, err := h.store.Lectores(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if !gestiona(r.Context(), creadaPor) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	if err := h.store.PonerLectores(r.Context(), id, lista); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, lectoresWire{CreatedBy: creadaPor, Readers: lista})
}

// gestiona: la lista la cambian los administradores y quien creó la conexión.
func gestiona(ctx context.Context, creadaPor string) bool {
	c, _ := authn.FromContext(ctx)
	return esAdmin(c) || (c.Subject != "" && c.Subject == creadaPor)
}

// listaValida limpia la lista que llega: clases conocidas, identificadores con
// contenido y sin repetir, y un tope.
func listaValida(in []domain.Lector) ([]domain.Lector, error) {
	if len(in) > maxLectores {
		return nil, httpx.ErrBadRequest.WithMessage("The list is too long.")
	}
	vistos := make(map[domain.Lector]bool, len(in))
	out := make([]domain.Lector, 0, len(in))
	for _, l := range in {
		l.ID = strings.TrimSpace(l.ID)
		l.Name = strings.TrimSpace(l.Name)
		if l.Kind != domain.LectorPersona && l.Kind != domain.LectorGrupo {
			return nil, httpx.ErrBadRequest.WithMessage("Each entry is a person (user) or a group (group).")
		}
		if l.ID == "" || len(l.ID) > 255 {
			return nil, httpx.ErrBadRequest.WithMessage("Each entry needs an identifier.")
		}
		if r := []rune(l.Name); len(r) > 200 {
			l.Name = string(r[:200])
		}
		clave := domain.Lector{Kind: l.Kind, ID: l.ID}
		if vistos[clave] {
			continue
		}
		vistos[clave] = true
		out = append(out, l)
	}
	return out, nil
}
