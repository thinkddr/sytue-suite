// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// «Pedir acceso» de un buzón compartido o delegado, con el contrato común de
// platform/pkg/peticiones:
//
//	POST   /shared-mailboxes/{accountID}/edit-requests              pedir (quien lo tiene en read)
//	GET    /shared-mailboxes/{accountID}/edit-requests              las pendientes (quien lo gestiona)
//	POST   /shared-mailboxes/{accountID}/edit-requests/{sub}/grant  conceder: pasa a `send`
//	DELETE /shared-mailboxes/{accountID}/edit-requests/{sub}        rechazar
//
// En un buzón compartido, «editar» es responder y escribir desde él: organizar sus
// mensajes no lo hace nadie todavía, tampoco quien tiene `manage`.

// peticionesStore es lo que esto necesita del store.
type peticionesStore interface {
	PedirEdicion(ctx context.Context, cuenta uuid.UUID, p peticiones.Peticion) (bool, []string, error)
	PeticionesEdicion(ctx context.Context, cuenta uuid.UUID, sub string) ([]peticiones.Peticion, error)
	ResolverPeticionEdicion(ctx context.Context, cuenta uuid.UUID, sub, sujeto string, concede bool) error
}

// WithHub conecta el Hub para avisar de las peticiones de acceso. Sin él se piden y se
// conceden igual, sin aviso.
func (a *API) WithHub(h *hubnotify.Client) *API {
	a.hub = h
	return a
}

// avisador es el de platform/pkg/peticiones con el Hub, el buzón de avisos y Sytue ID.
func (a *API) avisador() peticiones.Avisador {
	return peticiones.Avisador{Hub: a.hub, Correo: a.cfg.Correo, IDBaseURL: cmp.Or(a.cfg.IDInternalURL, "http://id:8081")}
}

func cuentaDeRuta(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "accountID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid accountID."))
		return uuid.Nil, false
	}
	return id, true
}

// pedirEdicion — POST /shared-mailboxes/{accountID}/edit-requests.
func (a *API) pedirEdicion(w http.ResponseWriter, r *http.Request) {
	cuenta, ok := cuentaDeRuta(w, r)
	if !ok {
		return
	}
	c, _ := authn.FromContext(r.Context())
	perm, err := a.store.GranteePermission(r.Context(), c.Subject, cuenta)
	if errors.Is(err, domain.ErrNotFound) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("You do not have access to this mailbox."))
		return
	}
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if perm.AtLeast(domain.PermSend) {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"requested": false})
		return
	}
	p := peticiones.De(c, cuenta.String())
	nueva, avisar, err := a.peticiones.PedirEdicion(r.Context(), cuenta, p)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if nueva {
		quien, t := p.Quien(), tenancy.MustFrom(r.Context())
		for _, sub := range avisar {
			a.avisador().Avisar(r.Context(), peticiones.Aviso{
				Tenant: t.Slug, Para: sub, Source: "mail", Type: "mail.access.requested",
				Titulo: quien + " asks to write from a shared mailbox", URL: hubnotify.EnlaceBuzon(a.cfg.PublicURL, t.Slug, cuenta.String()),
				Recurso: cuenta.String(), Params: map[string]string{"actor": quien}, Token: authn.Token(r),
				Asunto: quien + " asks to write from a shared mailbox",
				Cuerpo: quien + " is asking for permission to write from a shared mailbox you manage. Open Sytue Mail to grant it with one click:",
			})
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"requested": true})
}

// listarPeticiones — GET /shared-mailboxes/{accountID}/edit-requests.
func (a *API) listarPeticiones(w http.ResponseWriter, r *http.Request) {
	cuenta, ok := cuentaDeRuta(w, r)
	if !ok {
		return
	}
	c, _ := authn.FromContext(r.Context())
	ps, err := a.peticiones.PeticionesEdicion(r.Context(), cuenta, c.Subject)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if ps == nil {
		ps = []peticiones.Peticion{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"requests": ps})
}

func (a *API) concederPeticion(w http.ResponseWriter, r *http.Request) {
	a.resolverPeticion(w, r, true)
}

func (a *API) rechazarPeticion(w http.ResponseWriter, r *http.Request) {
	a.resolverPeticion(w, r, false)
}

func (a *API) resolverPeticion(w http.ResponseWriter, r *http.Request, concede bool) {
	cuenta, ok := cuentaDeRuta(w, r)
	if !ok {
		return
	}
	c, _ := authn.FromContext(r.Context())
	sujeto := chi.URLParam(r, "sub")
	if err := a.peticiones.ResolverPeticionEdicion(r.Context(), cuenta, c.Subject, sujeto, concede); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if concede {
		a.avisador().Avisar(r.Context(), peticiones.Aviso{
			Tenant: tenancy.MustFrom(r.Context()).Slug, Para: sujeto, Source: "mail", Type: "mail.access.granted",
			Titulo: "You can now write from a shared mailbox", URL: hubnotify.EnlaceBuzon(a.cfg.PublicURL, tenancy.MustFrom(r.Context()).Slug, cuenta.String()),
			Recurso: cuenta.String(), Params: map[string]string{"actor": cmp.Or(strings.TrimSpace(c.Name), c.Email)},
		})
	}
	httpx.NoContent(w)
}
