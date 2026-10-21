// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// El dominio de correo de la organización, desde Sytue Admin.
//
// Conectar un dominio propio solo se podía hacer DENTRO del asistente de alta, y
// el asistente se pasa una vez. Quien lo saltaba —o quien compró el dominio tres
// meses después, que es lo normal— se quedaba sin ninguna forma de conectarlo: la
// única puerta exigía tener una fila de onboarding abierta y ser el propietario
// que la abrió. Aquí la misma operación queda disponible siempre.
//
// LOS DOS DOMINIOS. Una organización tiene siempre su subdominio de la suite
// —`<slug>.sytue.eu`— y, opcionalmente, uno propio verificado. El primero no
// desaparece al conectar el segundo: es el cimiento del que cuelga todo lo que en
// la suite necesita una dirección que servimos nosotros (ver
// domain.OrgBaseMailDomain). Esta pantalla enseña los dos por eso mismo, para que
// nadie crea que conectar el suyo apaga el otro.
//
// AISLAMIENTO POR TENANT: como el resto de /admin, la organización sale del claim
// del llamante (activeOrg) y nunca del cuerpo de la petición.
package http

import (
	"context"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// handleAdminGetDomain — GET /api/v1/admin/domain — en qué dominio vive el correo
// de la organización y qué falta para conectar el propio.
//
// Comprueba el DNS en cada lectura cuando hay un dominio reclamado sin verificar.
// Podría devolverse solo lo guardado, pero entonces la pantalla diría «pendiente»
// hasta que alguien pulsara un botón, y quien acaba de crear el registro TXT
// esperaría delante de una pantalla que ya podría estar diciéndole que sí.
func (s *Server) handleAdminGetDomain(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	principal, base, err := s.store.MailDomainsForOrg(ctx, nil, org.ID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	res := map[string]any{
		// El que se usa hoy para las direcciones nuevas, y el que existe siempre.
		"mail_domain": principal,
		"base_domain": base,
	}
	prof, perr := s.store.GetOrgProfile(ctx, org.ID)
	if perr != nil {
		httpx.Fail(w, r, mapDomainErr(perr))
		return
	}
	if prof.Domain == "" {
		// Sin dominio propio reclamado: no hay registro que enseñar ni DNS que mirar.
		res["domain"] = nil
		httpx.JSON(w, r, http.StatusOK, res)
		return
	}
	var chk domain.DNSCheck
	if prof.DomainVerifiedAt == nil {
		dnsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		chk = domain.CheckDomainTXT(dnsCtx, s.resolver, prof.Domain, prof.DomainToken)
	}
	res["domain"] = domainWire(prof, chk)
	httpx.JSON(w, r, http.StatusOK, res)
}

// handleAdminSetDomain — POST /api/v1/admin/domain — reclama un dominio propio y
// lo comprueba. Mismo endpoint para «guardar» y para «comprobar ahora», igual que
// en el asistente: el cuerpo es literalmente el mismo (reclamarDominio), de modo
// que las dos puertas no puedan divergir.
func (s *Server) handleAdminSetDomain(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.reclamarDominio(w, r, org)
}

// handleAdminSetDomainCloudflare — POST /api/v1/admin/domain/cloudflare — el
// camino asistido para quien tiene su dominio en Cloudflare: Sytue crea los
// registros con un token del CLIENTE en vez de dictarle una lista que copiar.
func (s *Server) handleAdminSetDomainCloudflare(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.reclamarDominioCloudflare(w, r, org)
}

// handleAdminDeleteDomain — DELETE /api/v1/admin/domain — desconecta el dominio
// propio y devuelve el correo al subdominio de la suite.
//
// Existe porque sin él la pantalla sería una puerta de una sola dirección: quien
// escribe mal el dominio, o vende el que tenía, se quedaría con un «pendiente de
// verificar» eterno y sin manera de quitarlo. Los buzones que ya existen NO se
// tocan (ver ClearOrgDomain): desconectar es dejar de emitir en ese dominio, no
// borrarle el correo a nadie.
func (s *Server) handleAdminDeleteDomain(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	ctx := r.Context()
	prof, perr := s.store.GetOrgProfile(ctx, org.ID)
	if perr != nil {
		httpx.Fail(w, r, mapDomainErr(perr))
		return
	}
	if prof.Domain == "" {
		// Ya no hay nada conectado. 204 y no 404: el estado que se pedía es el que
		// hay, y hacer fallar a quien pulsa dos veces no ayuda a nadie.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.store.ClearOrgDomain(ctx, org.ID); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.domain.disconnected", prof.Domain, r))
	w.WriteHeader(http.StatusNoContent)
}
