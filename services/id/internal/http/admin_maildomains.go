// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// LOS DOMINIOS de correo de la organización, en plural, desde Sytue Admin.
//
// La pantalla anterior gestionaba UNO: conectar el segundo significaba
// desconectar el primero y, con él, las direcciones que ya usaba la plantilla.
// Una empresa con dos marcas, o con el .es y el .com de la misma, tenía que
// elegir. Aquí se listan todos, se añade uno más sin tocar los que funcionan y se
// decide cuál estrena las altas nuevas.
//
// QUÉ CAMBIA AL AÑADIR Y QUÉ NO. Añadir un dominio verificado hace que Sytue
// sirva su correo: sus direcciones entregan y se puede enviar desde ellas. Lo que
// NO cambia es dónde nacen las direcciones nuevas —eso es el principal— ni las
// que ya existen. Cambiar el principal SÍ muda a toda la plantilla, conservando
// la dirección anterior como alias, igual que hacía conectar el primer dominio.
//
// AISLAMIENTO POR TENANT: como el resto de /admin, la organización sale del claim
// del llamante (activeOrg) y nunca del cuerpo ni de la ruta.
package http

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// maxComprobacionesDNS acota cuántos dominios se consultan al DNS en una lectura.
// La pantalla comprueba en cada carga los que están pendientes —para que quien
// acaba de publicar el TXT no tenga que pulsar nada—, y sin tope una organización
// con veinte reclamaciones muertas convertiría cada visita en veinte consultas.
// Los que quedan fuera se enseñan con su estado guardado y se comprueban al
// pulsar «comprobar», que es una llamada por dominio.
const maxComprobacionesDNS = 5

// mailDomainWire es la forma JSON de un dominio de la organización. Comparte
// nombres con `domainWire` —host, record_name, record_value, verified…— porque la
// pantalla enseña lo mismo para cada uno y dos vocabularios para el mismo objeto
// solo se pueden divergir.
func mailDomainWire(d store.OrgMailDomain, chk domain.DNSCheck) map[string]any {
	m := map[string]any{
		"id":           d.ID,
		"host":         d.Domain,
		"record_name":  domain.RecordName(d.Domain),
		"record_value": domain.RecordValue(d.Token),
		"verified":     d.Verified(),
		"primary":      d.Primary,
	}
	if d.VerifiedAt != nil {
		m["verified_at"] = d.VerifiedAt.UTC().Format(time.RFC3339)
	}
	if chk.Status != "" {
		m["status"] = string(chk.Status)
		if len(chk.Found) > 0 {
			m["found"] = chk.Found
		}
	}
	return m
}

// handleAdminListMailDomains — GET /api/v1/admin/mail-domains — todos los
// dominios de la organización, con el subdominio de la suite al lado.
//
// El subdominio va en la respuesta y no como una fila más porque no es un dominio
// conectado: existe siempre, no se verifica y no se puede quitar. La pantalla lo
// enseña arriba para que nadie crea que conectar el suyo lo apaga —la duda es
// siempre «¿y entonces las direcciones de antes dejan de funcionar?»— y la
// respuesta tiene que estar en la pantalla, no en una nota de soporte.
func (s *Server) handleAdminListMailDomains(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	lista, err := s.store.ListOrgMailDomains(ctx, org.ID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	principal, base, err := s.store.MailDomainsForOrg(ctx, nil, org.ID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	items := make([]map[string]any, 0, len(lista))
	consultas := 0
	for _, d := range lista {
		var chk domain.DNSCheck
		if !d.Verified() && consultas < maxComprobacionesDNS {
			consultas++
			chk = s.comprobarTXT(ctx, d)
		}
		items = append(items, mailDomainWire(d, chk))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"items": items,
		// El que se usa hoy para las direcciones nuevas, y el que existe siempre.
		"mail_domain": principal,
		"base_domain": base,
	})
}

// comprobarTXT consulta el DNS de un dominio pendiente, acotado en tiempo.
func (s *Server) comprobarTXT(ctx context.Context, d store.OrgMailDomain) domain.DNSCheck {
	dnsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return domain.CheckDomainTXT(dnsCtx, s.resolver, d.Domain, d.Token)
}

// handleAdminAddMailDomain — POST /api/v1/admin/mail-domains — reclama un dominio
// y lo comprueba.
//
// El MISMO endpoint sirve para «añadir» y para «comprobar ahora», como en el
// asistente: el cuerpo es idéntico y la operación es idempotente —conserva token
// y sello—, así que pulsar «comprobar» es volver a pedir lo mismo. Dos endpoints
// para eso solo podrían divergir.
//
// Añadir NO toca el principal, salvo que sea el primer dominio verificado de la
// organización: verificar el primero es, en la práctica, estrenarlo (lo decide
// MarkOrgMailDomainVerified). Los demás quedan sirviendo correo sin mover a
// nadie, que es justo lo que no se podía hacer antes.
func (s *Server) handleAdminAddMailDomain(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	var req onboardingDomainRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	host, nerr := domain.NormalizeDomain(req.Domain)
	if nerr != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"That does not look like a domain. Write it as acme.com, without 'https://' or '@'.",
		))
		return
	}
	token, terr := domain.NewDomainToken()
	if terr != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(terr))
		return
	}
	d, serr := s.store.AddOrgMailDomain(ctx, nil, org.ID, host, token)
	if serr != nil {
		httpx.Fail(w, r, mapDominioAjeno(serr))
		return
	}
	chk := s.comprobarTXT(ctx, d)
	if chk.Status == domain.DomainVerified && !d.Verified() {
		if merr := s.store.MarkOrgMailDomainVerified(ctx, nil, org.ID, d.Domain); merr != nil {
			httpx.Fail(w, r, mapDominioAjeno(merr))
			return
		}
		_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.domain.verified", d.Domain, r))
		// Releer: MarkOrgMailDomainVerified pudo además hacerlo principal (el
		// primero verificado lo es), y la pantalla tiene que decirlo ya.
		if refrescado, rerr := s.buscarMailDomain(ctx, org.ID, d.ID); rerr == nil {
			d = refrescado
		}
		if d.Primary {
			s.mudarAlDominioPropio(r, org.ID, d.Domain)
		}
	}
	httpx.JSON(w, r, http.StatusOK, mailDomainWire(d, chk))
}

// handleAdminSetPrimaryMailDomain — POST /api/v1/admin/mail-domains/{id}/primary
// — elige en qué dominio nacen las direcciones nuevas y muda a la plantilla.
//
// Es la operación GRANDE de esta pantalla: cambia la dirección de todo el mundo.
// La anterior de cada persona NO se borra, se conserva como alias (ver
// PromoteOrgMailDomain): mudarse no puede ser perder el correo de antes.
func (s *Server) handleAdminSetPrimaryMailDomain(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	ctx := r.Context()
	d, serr := s.store.SetPrimaryOrgMailDomain(ctx, org.ID, chi.URLParam(r, "id"))
	if serr != nil {
		httpx.Fail(w, r, mapDomainErr(serr))
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.domain.primary_changed", d.Domain, r))
	s.mudarAlDominioPropio(r, org.ID, d.Domain)
	httpx.JSON(w, r, http.StatusOK, mailDomainWire(d, domain.DNSCheck{}))
}

// handleAdminDeleteMailDomain — DELETE /api/v1/admin/mail-domains/{id} —
// desconecta un dominio.
//
// Las direcciones que ya existen en él NO se tocan: quitarle la dirección a quien
// la está usando es perder su correo, no deshacer una configuración. Lo que
// cambia es que Sytue deja de afirmar que sirve ese dominio y que ninguna
// dirección nueva nace ahí.
func (s *Server) handleAdminDeleteMailDomain(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	d, berr := s.buscarMailDomain(ctx, org.ID, id)
	if berr != nil {
		httpx.Fail(w, r, mapDomainErr(berr))
		return
	}
	if err := s.store.DeleteOrgMailDomain(ctx, org.ID, id); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.domain.disconnected", d.Domain, r))
	w.WriteHeader(http.StatusNoContent)
}

// buscarMailDomain localiza un dominio de la organización por su id. Sale de la
// lista y no de una consulta propia: son dos o tres filas y así no hay dos formas
// de leer lo mismo.
func (s *Server) buscarMailDomain(ctx context.Context, orgID, id string) (store.OrgMailDomain, error) {
	lista, err := s.store.ListOrgMailDomains(ctx, orgID)
	if err != nil {
		return store.OrgMailDomain{}, err
	}
	for _, d := range lista {
		if d.ID == id {
			return d, nil
		}
	}
	return store.OrgMailDomain{}, domain.ErrNotFound
}

// mapDominioAjeno traduce el choque contra el índice de dominios verificados. El
// mensaje dice qué pasó y qué hacer: sin él, quien reclama un dominio que otra
// organización ya demostró tener recibe un «conflicto» que no significa nada.
func mapDominioAjeno(err error) error {
	if e := mapDomainErr(err); httpx.AsError(e).Status == http.StatusConflict {
		return httpx.ErrConflict.WithMessage(
			"Another Sytue organisation has already verified this domain. If it is yours, get in touch.",
		)
	}
	return mapDomainErr(err)
}
