// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/mailaddr"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// --- Alias de correo — REST de conveniencia que usa el SDK web ---
//
// Un alias es otra dirección que entrega a la misma cuenta. CRUD del dueño (la
// cuenta autenticada). La entrega entrante ya resuelve el destinatario por alias
// (store.LookupAddress); aquí se gestionan, y alimentan el selector «De» del
// redactor (enviar-como). Seguridad: el alias debe estar en un dominio que
// servimos y solo puede pertenecer a la cuenta del llamante (ver createAlias).

type aliasReq struct {
	Address string `json:"address"`
}

func (a *API) listAliases(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	aliases, err := a.store.ListAliases(r.Context(), acc.ID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if aliases == nil {
		aliases = []domain.Alias{}
	}
	// `primary` acompaña a los alias para que el cliente componga el selector «De»
	// (dirección primaria + alias) sin una segunda llamada.
	httpx.JSON(w, r, http.StatusOK, map[string]any{"aliases": aliases, "primary": acc.Address})
}

func (a *API) createAlias(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body aliasReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := a.direccionAdmitida(r, acc, body.Address); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Abuso — una cuenta común (personal, o miembro sin rol de administración)
	// tiene UN alias. El admin de una organización reparte los que quiera desde
	// la consola; aquí es autoservicio y sin techo sería una fábrica de
	// direcciones desechables sobre un dominio compartido.
	if c, _ := authn.FromContext(r.Context()); c.OrgKind == "personal" || !c.HasAnyRole("owner", "admin") {
		los, err := a.store.ListAliases(r.Context(), acc.ID)
		if err != nil {
			httpx.Fail(w, r, mapErr(err))
			return
		}
		if len(los) >= 1 {
			httpx.Fail(w, r, mapErr(domain.ErrAliasLimite))
			return
		}
	}
	// La propiedad la garantiza acc.ID (cuenta autenticada del llamante); el store
	// comprueba la unicidad global de la dirección (sin secuestrar la de otro).
	created, err := a.store.CreateAlias(r.Context(), acc.ID, body.Address)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, created)
}

// direccionAdmitida es la verja de TODA dirección nueva que una cuenta pide para sí o
// para su organización: un alias o un buzón compartido. Una sola, para las dos
// rutas: cuando cada una tenía la suya, el buzón compartido se quedó sin la segunda
// mitad y una cuenta personal podía crear `seguridad@<dominio raíz>`.
//
//   - Dominio servido: solo en un dominio de la organización del llamante (los
//     verificados llegan en el token), en el de su propia dirección primaria, en el
//     subdominio de su organización o en el dominio principal configurado. Impide
//     pedir una dirección en un dominio que no servimos o secuestrar el de otro.
//   - Suplantación: en el dominio raíz (cuentas personales y la propia Sytue) nadie
//     se queda con admin@, soporte@ o sytue-*@. La lista es la misma que aplica la
//     consola de Sytue ID (platform/pkg/mailaddr).
func (a *API) direccionAdmitida(r *http.Request, acc domain.Account, address string) error {
	if !domain.AliasDomainAllowed(address, acc, a.cfg.PrimaryDomain, a.cfg.RootTenant, orgMailDomains(r)) {
		return httpx.ErrBadRequest.WithMessage("The address must belong to a domain of the service.")
	}
	if addr, err := domain.ParseAddress(address); err == nil && mailaddr.Suplanta(addr.Local, addr.Domain, a.cfg.PrimaryDomain) {
		return mapErr(domain.ErrAliasReservado)
	}
	return nil
}

func (a *API) deleteAlias(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "aliasID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid aliasID."))
		return
	}
	if err := a.store.DeleteAlias(r.Context(), acc.ID, id); err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}
