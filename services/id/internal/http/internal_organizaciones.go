// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// POST /internal/people/orgs {"user_id": "<sub>"} → {"orgs": ["<slug>", …], "personal": bool}
//
// Las organizaciones ACTIVAS de una persona ACTIVA, para una sola cosa: que Sytue Docs
// sepa si alguna de ellas admite peticiones de firma de fuera (ADR 0059) ANTES de
// avisar a esa persona. Nadie más lo recibe: Docs no se lo cuenta a quien pide
// (`people:resolve`, ADR 0040, sigue sin dar la organización de nadie) y la petición
// se «envía» igual la admitan o no.
//
// Solo desde DENTRO y con su secreto PROPIO (Config.OrgsPersonaSecret, compartido
// solo con Docs). Con techo global. La cuenta PERSONAL no sale por su nombre —se
// deriva de la dirección de la persona—: solo `personal: true`, que es lo único que
// Docs necesita (no tiene política: la persona decide por sí misma con sus bloqueos).
func (s *Server) handleInternalPeopleOrgs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := uuid.Parse(req.UserID); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid user identifier."))
		return
	}
	if !s.permiteDirectorio(w, r, s.organizacionesGlobal, "organizaciones-de-persona") {
		return
	}
	s.log.Debug("directorio interno: organizaciones de una persona")
	orgs, personal := []string{}, false
	u, err := s.store.GetUserByID(r.Context(), req.UserID)
	if errors.Is(err, domain.ErrNotFound) || (err == nil && !u.Active()) {
		// Sin persona, o de baja: ninguna organización. La misma respuesta.
		httpx.JSON(w, r, http.StatusOK, map[string]any{"orgs": orgs, "personal": false})
		return
	}
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	ms, err := s.store.ListOrgMembershipsForUser(r.Context(), req.UserID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	for _, m := range ms {
		switch {
		case m.Status != domain.StatusActive:
		case m.Kind == domain.OrgKindPersonal:
			personal = true
		default:
			orgs = append(orgs, m.Slug)
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"orgs": orgs, "personal": personal})
}
