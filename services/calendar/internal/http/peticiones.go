// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"cmp"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
)

// «Pedir acceso de edición» de un calendario compartido, con el contrato común de
// platform/pkg/peticiones:
//
//	POST   /calendars/{id}/edit-requests              pedir (quien lo tiene de lector)
//	GET    /calendars/{id}/edit-requests              las pendientes (el dueño)
//	POST   /calendars/{id}/edit-requests/{sub}/grant  conceder (el dueño)
//	DELETE /calendars/{id}/edit-requests/{sub}        rechazar (el dueño)

// PedirEdicion — POST /calendars/{id}/edit-requests.
func (h *Handlers) PedirEdicion(w http.ResponseWriter, r *http.Request) {
	c, ok := authn.FromContext(r.Context())
	calID, okID := parseUUID(r)
	if !ok || c.Subject == "" {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if !okID {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid calendar id."))
		return
	}
	yaPuede, err := h.svc.PedirEdicion(r.Context(), calID, peticiones.De(c, calID.String()), authn.Token(r))
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"requested": !yaPuede})
}

// ListarPeticiones — GET /calendars/{id}/edit-requests.
func (h *Handlers) ListarPeticiones(w http.ResponseWriter, r *http.Request) {
	sub, ok := subject(r)
	calID, okID := parseUUID(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if !okID {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid calendar id."))
		return
	}
	ps, err := h.svc.PeticionesEdicion(r.Context(), calID, sub)
	if err != nil {
		fail(w, r, err)
		return
	}
	if ps == nil {
		ps = []peticiones.Peticion{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"requests": ps})
}

// ConcederPeticion — POST /calendars/{id}/edit-requests/{sub}/grant.
func (h *Handlers) ConcederPeticion(w http.ResponseWriter, r *http.Request) {
	h.resolverPeticion(w, r, true)
}

// RechazarPeticion — DELETE /calendars/{id}/edit-requests/{sub}.
func (h *Handlers) RechazarPeticion(w http.ResponseWriter, r *http.Request) {
	h.resolverPeticion(w, r, false)
}

func (h *Handlers) resolverPeticion(w http.ResponseWriter, r *http.Request, concede bool) {
	c, ok := authn.FromContext(r.Context())
	calID, okID := parseUUID(r)
	if !ok || c.Subject == "" {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if !okID {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid calendar id."))
		return
	}
	actor := cmp.Or(strings.TrimSpace(c.Name), c.Email)
	if err := h.svc.ResolverPeticion(r.Context(), calID, c.Subject, actor, chi.URLParam(r, "sub"), concede); err != nil {
		fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}
