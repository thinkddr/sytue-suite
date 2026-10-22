// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"cmp"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// «Pedir acceso de edición» de un cuaderno ajeno que se lee (uno público de otra
// persona), con el contrato común de platform/pkg/peticiones:
//
//	POST   /notebooks/{id}/edit-requests              pedir
//	GET    /notebooks/{id}/edit-requests              las pendientes (el dueño)
//	POST   /notebooks/{id}/edit-requests/{sub}/grant  conceder: editar sus notas
//	DELETE /notebooks/{id}/edit-requests/{sub}        rechazar

func (h *Handlers) avisador() peticiones.Avisador {
	return peticiones.Avisador{Hub: h.hub, Correo: h.cfg.Correo, IDBaseURL: h.cfg.IDBaseURL}
}

// PedirEdicion — POST /notebooks/{id}/edit-requests.
func (h *Handlers) PedirEdicion(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := authn.FromContext(r.Context())
	p := peticiones.De(c, id.String())
	nueva, yaPuede, dueno, err := h.store.PedirEdicion(r.Context(), id, p)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if nueva {
		quien := p.Quien()
		h.avisador().Avisar(r.Context(), peticiones.Aviso{
			Tenant: tenancy.MustFrom(r.Context()).Slug, Para: dueno, Source: "notes", Type: "notes.access.requested",
			Titulo: quien + " asks to edit a notebook", URL: hubnotify.EnlaceCuaderno(h.cfg.PublicURL, tenancy.MustFrom(r.Context()).Slug, id.String()),
			Recurso: id.String(), Params: map[string]string{"actor": quien}, Token: authn.Token(r),
			Asunto: quien + " asks to edit one of your notebooks",
			Cuerpo: quien + " is asking for permission to edit one of your notebooks in Sytue Notepads. Open it to grant it with one click:",
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"requested": !yaPuede})
}

// ListarPeticiones — GET /notebooks/{id}/edit-requests (el dueño).
func (h *Handlers) ListarPeticiones(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ps, err := h.store.PeticionesEdicion(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if ps == nil {
		ps = []peticiones.Peticion{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"requests": ps})
}

// ConcederPeticion — POST /notebooks/{id}/edit-requests/{sub}/grant.
func (h *Handlers) ConcederPeticion(w http.ResponseWriter, r *http.Request) {
	h.resolverPeticion(w, r, true)
}

// RechazarPeticion — DELETE /notebooks/{id}/edit-requests/{sub}.
func (h *Handlers) RechazarPeticion(w http.ResponseWriter, r *http.Request) {
	h.resolverPeticion(w, r, false)
}

func (h *Handlers) resolverPeticion(w http.ResponseWriter, r *http.Request, concede bool) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sujeto := chi.URLParam(r, "sub")
	if err := h.store.ResolverPeticionEdicion(r.Context(), id, sub(r), sujeto, concede); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if concede {
		c, _ := authn.FromContext(r.Context())
		h.avisador().Avisar(r.Context(), peticiones.Aviso{
			Tenant: tenancy.MustFrom(r.Context()).Slug, Para: sujeto, Source: "notes", Type: "notes.access.granted",
			Titulo: "You can now edit a notebook", URL: hubnotify.EnlaceCuaderno(h.cfg.PublicURL, tenancy.MustFrom(r.Context()).Slug, id.String()),
			Recurso: id.String(), Params: map[string]string{"actor": cmp.Or(strings.TrimSpace(c.Name), c.Email)},
		})
	}
	httpx.NoContent(w)
}
