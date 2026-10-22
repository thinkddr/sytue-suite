// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
)

// «Pedir acceso de edición» de algo compartido en lectura, con el contrato común de
// platform/pkg/peticiones. `{id}` es el fichero o la carpeta compartida:
//
//	POST   /shared/{id}/edit-requests              pedir (quien lo tiene en lectura)
//	GET    /shared/{id}/edit-requests              las pendientes (quien lo compartió o el dueño)
//	POST   /shared/{id}/edit-requests/{sub}/grant  conceder
//	DELETE /shared/{id}/edit-requests/{sub}        rechazar

func idDeRuta(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid id."))
		return uuid.Nil, false
	}
	return id, true
}

// PedirEdicion — POST /shared/{id}/edit-requests.
func (h *Handlers) PedirEdicion(w http.ResponseWriter, r *http.Request) {
	c, ok := authn.FromContext(r.Context())
	if !ok || c.Subject == "" {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id, ok := idDeRuta(w, r)
	if !ok {
		return
	}
	yaPuede, err := h.svc.PedirEdicion(r.Context(), id, jwtGroups(r), peticiones.De(c, id.String()), authn.Token(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"requested": !yaPuede})
}

// ListarPeticiones — GET /shared/{id}/edit-requests.
func (h *Handlers) ListarPeticiones(w http.ResponseWriter, r *http.Request) {
	id, ok := idDeRuta(w, r)
	if !ok {
		return
	}
	ps, err := h.svc.PeticionesEdicion(r.Context(), id, subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if ps == nil {
		ps = []peticiones.Peticion{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"requests": ps})
}

// ConcederPeticion — POST /shared/{id}/edit-requests/{sub}/grant.
func (h *Handlers) ConcederPeticion(w http.ResponseWriter, r *http.Request) {
	h.resolverPeticion(w, r, true)
}

// RechazarPeticion — DELETE /shared/{id}/edit-requests/{sub}.
func (h *Handlers) RechazarPeticion(w http.ResponseWriter, r *http.Request) {
	h.resolverPeticion(w, r, false)
}

func (h *Handlers) resolverPeticion(w http.ResponseWriter, r *http.Request, concede bool) {
	id, ok := idDeRuta(w, r)
	if !ok {
		return
	}
	if err := h.svc.ResolverPeticion(r.Context(), id, subFromCtx(r), chi.URLParam(r, "sub"), concede); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}
