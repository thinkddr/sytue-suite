// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// maxNombresOrgs es lo que pide el Hub de una vez: las organizaciones de fuera de una
// página de avisos, que es de 50 como mucho.
const maxNombresOrgs = 50

// POST /internal/orgs/names {slugs:[…≤50]} → {orgs:[{slug, name}]}
//
// El nombre visible («Acme Ingeniería S.L.») de las organizaciones que mandan un aviso
// a una persona de otra, para que la campana no la enseñe por su identificador
// (decisión del dueño, 17/10/2026). No es un directorio: lo piden el Hub, Rooms y
// Calendar, con su secreto PROPIO (Config.NombresOrgsSecret), y solo por los slugs de
// lo que va dirigido a esa persona —sus avisos y sus invitaciones—; nadie más que su
// destinatario ve el nombre. Un slug que no es de una organización activa no sale: ni dice si existe.
func (s *Server) handleInternalOrgsNames(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slugs []string `json:"slugs"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(req.Slugs) > maxNombresOrgs {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Too many slugs (max 50)."))
		return
	}
	nombres, err := s.store.NombresDeOrgs(r.Context(), req.Slugs)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	type orgNombre struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	out := make([]orgNombre, 0, len(nombres))
	for _, slug := range req.Slugs {
		if n, ok := nombres[slug]; ok {
			out = append(out, orgNombre{Slug: slug, Name: n})
			delete(nombres, slug) // un slug repetido sale una vez
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"orgs": out})
}
