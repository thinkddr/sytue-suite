// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"cmp"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/correo"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
)

// «Pedir acceso de edición» de un módulo, con el contrato común de platform/pkg/peticiones:
//
//	POST   /modulos/{modulo}/edit-requests              pedir (quien tiene `lectura`)
//	GET    /modulos/{modulo}/edit-requests              las pendientes (owner o admin)
//	POST   /modulos/{modulo}/edit-requests/{sub}/grant  conceder: `usuario` en el módulo
//	DELETE /modulos/{modulo}/edit-requests/{sub}        rechazar
//
// Concede la administración de la organización, la misma que pone los niveles en
// Sytue Admin → ERP; al conceder, `erp.access.granted` le quita la franja a quien lo
// pidió por el flujo del Hub.

var errModuloDesconocido = httpx.ErrNotFound.WithMessage("Unknown module.")

// moduloDeRuta es `{modulo}` si es un módulo de este ERP.
func (h *Handlers) moduloDeRuta(w http.ResponseWriter, r *http.Request) (string, bool) {
	m := chi.URLParam(r, "modulo")
	if m == permisos.Obligaciones {
		return m, true
	}
	for _, x := range h.cat.Modulos() {
		if x.Clave == m {
			return m, true
		}
	}
	httpx.Fail(w, r, errModuloDesconocido)
	return "", false
}

func (h *Handlers) avisador() peticiones.Avisador {
	c := h.cfg
	return peticiones.Avisador{Hub: h.hub, IDBaseURL: c.IDBaseURL, Correo: correo.Config{
		Addr: c.SMTPAddr, From: c.SMTPFrom, User: c.SMTPUser, Password: c.SMTPPassword, TLSServerName: c.SMTPTLSServerName,
	}}
}

// PedirEdicion — POST /modulos/{modulo}/edit-requests.
func (h *Handlers) PedirEdicion(w http.ResponseWriter, r *http.Request) {
	modulo, ok := h.moduloDeRuta(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	nivel := h.nivelEn(ctx, modulo)
	if permisos.Alcanza(nivel, permisos.Usuario) {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"requested": false})
		return
	}
	if !permisos.Alcanza(nivel, permisos.Lectura) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	c, _ := authn.FromContext(ctx)
	p := peticiones.De(c, modulo)
	nueva, admins, err := h.store.PedirEdicion(ctx, modulo, p)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if nueva {
		quien, t := p.Quien(), tenancy.MustFrom(ctx)
		for _, a := range admins {
			h.avisador().Avisar(ctx, peticiones.Aviso{
				Tenant: t.Slug, Para: a, Source: "erp", Type: "erp.access.requested",
				Titulo: quien + " asks to edit “" + modulo + "”", URL: hubnotify.EnlacePeticionesERP(h.cfg.PublicURL, t.Slug, modulo),
				Recurso: modulo, Params: map[string]string{"actor": quien, "name": modulo}, Token: authn.Token(r),
				Asunto: quien + " asks to edit “" + modulo + "” in Sytue ERP",
				Cuerpo: quien + " is asking for permission to edit the “" + modulo + "” module. Open Sytue ERP to grant it with one click:",
			})
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"requested": true})
}

// ListarPeticiones — GET /modulos/{modulo}/edit-requests (owner o admin).
func (h *Handlers) ListarPeticiones(w http.ResponseWriter, r *http.Request) {
	modulo, ok := h.moduloDeRuta(w, r)
	if !ok {
		return
	}
	ps, err := h.store.PeticionesEdicion(r.Context(), modulo)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if ps == nil {
		ps = []peticiones.Peticion{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"requests": ps})
}

// ConcederPeticion — POST /modulos/{modulo}/edit-requests/{sub}/grant (owner o admin).
func (h *Handlers) ConcederPeticion(w http.ResponseWriter, r *http.Request) {
	h.resolverPeticion(w, r, true)
}

// RechazarPeticion — DELETE /modulos/{modulo}/edit-requests/{sub} (owner o admin).
func (h *Handlers) RechazarPeticion(w http.ResponseWriter, r *http.Request) {
	h.resolverPeticion(w, r, false)
}

func (h *Handlers) resolverPeticion(w http.ResponseWriter, r *http.Request, concede bool) {
	modulo, ok := h.moduloDeRuta(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	c, _ := authn.FromContext(ctx)
	sujeto := chi.URLParam(r, "sub")
	if err := h.store.ResolverPeticionEdicion(ctx, modulo, sujeto, concede, c.Subject); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		httpx.Fail(w, r, err)
		return
	}
	if concede {
		h.avisador().Avisar(ctx, peticiones.Aviso{
			Tenant: tenancy.MustFrom(ctx).Slug, Para: sujeto, Source: "erp", Type: "erp.access.granted",
			Titulo: "You can now edit “" + modulo + "”", URL: hubnotify.EnlacePeticionesERP(h.cfg.PublicURL, tenancy.MustFrom(ctx).Slug, modulo),
			Recurso: modulo, Params: map[string]string{"actor": cmp.Or(strings.TrimSpace(c.Name), c.Email), "name": modulo},
		})
	}
	httpx.NoContent(w)
}
