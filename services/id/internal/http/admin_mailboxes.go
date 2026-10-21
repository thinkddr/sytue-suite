// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Gestión de las direcciones de correo de la organización desde Sytue Admin: ver
// qué dirección tiene cada persona, cambiársela y darle alias (varias direcciones
// que entregan en el mismo buzón).
//
// La dirección se asigna sola al dar de alta (ver la migración 0048); esto es para
// las excepciones, que son las que de verdad necesitan una pantalla: el comercial
// que quiere ser `ventas@`, la persona que se casa y cambia de apellido —a la que
// se le añade un alias con el nuevo, sin quitarle el viejo, para que no deje de
// recibir— y los dos homónimos a los que el desempate automático dejó con
// direcciones que nadie acierta a la primera.
//
// AISLAMIENTO POR TENANT: como el resto de /admin, la organización sale SIEMPRE
// del claim del llamante (activeOrg) y nunca del path, y el usuario objetivo tiene
// que ser miembro de ESA organización (adminTargetInOrg) — un id de otra da 404, no
// una acción cruzada.
package http

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

type mailboxAddressRequest struct {
	// Address admite la dirección entera o solo su parte izquierda: en la consola
	// el dominio se enseña al lado del campo y hacerlo teclear otra vez sería pedir
	// que se copie lo que ya está en pantalla.
	Address string `json:"address"`
}

// handleAdminListMailboxes — GET /api/v1/admin/mailboxes — direcciones de la
// organización activa. Con ?user_id= devuelve solo las de esa persona.
//
// Incluye el dominio de correo de la organización porque la consola lo necesita
// para dibujar el campo «parte izquierda + @dominio», y sacarlo de aquí evita que
// tenga que deducirlo repitiendo la regla del dominio propio verificado.
func (s *Server) handleAdminListMailboxes(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	userID := r.URL.Query().Get("user_id")
	if userID != "" && !uuidRe.MatchString(userID) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid user_id."))
		return
	}
	boxes, err := s.store.ListOrgMailboxes(r.Context(), org.ID, userID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if boxes == nil {
		boxes = []domain.OrgMailbox{}
	}
	mailDomain, err := s.store.MailDomainForOrg(r.Context(), nil, org.ID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"mailboxes": boxes, "mail_domain": mailDomain,
	})
}

// handleAdminSetMailbox — POST /api/v1/admin/users/{id}/mailbox — cambia la
// dirección principal de un miembro.
//
// La anterior NO se pierde: pasa a ser alias suyo, para que el correo que ya iba a
// la vieja —remitentes que la tienen guardada, listas de distribución, formularios
// de terceros— siga llegando. Un cambio de dirección que hace desaparecer la
// anterior es una pérdida de correo disfrazada de renombrado, y quien lo pulsa no
// tiene forma de saber quién le escribía a la de antes.
func (s *Server) handleAdminSetMailbox(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, targetID, _, apiErr := s.adminTargetInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	var req mailboxAddressRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	address, err := s.store.SetPrimaryMailbox(r.Context(), org.ID, targetID, req.Address)
	if err != nil {
		httpx.Fail(w, r, mailboxErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.mailbox.changed", targetID, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"address": address})
}

// handleAdminAddMailboxAlias — POST /api/v1/admin/users/{id}/aliases — añade una
// dirección más que entrega en el buzón de ese miembro.
func (s *Server) handleAdminAddMailboxAlias(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, targetID, _, apiErr := s.adminTargetInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	var req mailboxAddressRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	alias, err := s.store.AddMailboxAlias(r.Context(), org.ID, targetID, req.Address)
	if err != nil {
		httpx.Fail(w, r, mailboxErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.mailbox.alias_added", targetID, r))
	httpx.JSON(w, r, http.StatusCreated, alias)
}

// handleAdminDeleteMailboxAlias — DELETE /api/v1/admin/mailboxes/{mailboxID} —
// quita un alias. El store se niega a borrar una dirección principal: dejar a
// alguien sin dirección no puede ser el efecto de pulsar «eliminar» en una lista
// de alias.
func (s *Server) handleAdminDeleteMailboxAlias(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "mailboxID")
	if !uuidRe.MatchString(id) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid address id."))
		return
	}
	if err := s.store.DeleteMailboxAlias(r.Context(), org.ID, id); err != nil {
		httpx.Fail(w, r, mailboxErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.mailbox.alias_removed", id, r))
	httpx.NoContent(w)
}

// mailboxErr traduce los errores de dominio de los buzones a HTTP con un mensaje
// que diga QUÉ hacer. «Conflicto» a secas deja al administrador sin saber si se
// equivocó de dirección o si el sistema falló.
func mailboxErr(err error) error {
	switch {
	case errors.Is(err, domain.ErrConflict):
		return httpx.ErrConflict.WithMessage("That address is already in use. Choose another one.")
	case errors.Is(err, domain.ErrAliasReservado):
		return httpx.ErrBadRequest.WithCode("alias_reserved").WithMessage("That address is reserved: it would look like it comes from the service itself.")
	case errors.Is(err, domain.ErrInvalidInput):
		return httpx.ErrBadRequest.WithMessage("That address is not valid for the organisation domain.")
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrNotFound
	default:
		return httpx.ErrInternal.Wrap(err)
	}
}
