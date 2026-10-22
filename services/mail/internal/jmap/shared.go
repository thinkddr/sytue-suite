// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// --- Buzones compartidos + delegación — REST de conveniencia que usa el SDK web ---
//
// Un buzón compartido (soporte@) es una cuenta de la ORG (kind=shared); la
// delegación es un ACL sobre la cuenta personal de otro. El acceso lo concede
// mailbox_acl (grantee = sub, nivel read|send|manage). Aquí:
//   - listar los buzones a los que tengo acceso (sidebar / selector «De»),
//   - crear un buzón compartido,
//   - leer sus carpetas/mensajes (gated read),
//   - gestionar su ACL (gated manage) y delegar el propio.
//
// SEGURIDAD — deny-by-default: cada acceso a una cuenta que no es la propia del
// usuario pasa por requireGrant (comprobación de ACL), ADEMÁS del aislamiento por
// tenant de RLS. El envío verifica el permiso en sendMessage (ver handlers.go).

// callerSubject devuelve el sub de Sytue ID del usuario autenticado.
func (a *API) callerSubject(r *http.Request) (string, error) {
	claims, ok := authn.FromContext(r.Context())
	if !ok {
		return "", httpx.ErrUnauthorized
	}
	return claims.Subject, nil
}

// requireGrant comprueba que el usuario autenticado tiene al menos `need` sobre
// `accountID` (ACL vigente). deny-by-default: sin ACL o con permiso insuficiente
// devuelve 403, sin revelar el contenido del buzón.
func (a *API) requireGrant(r *http.Request, accountID uuid.UUID, need domain.Permission) error {
	subject, err := a.callerSubject(r)
	if err != nil {
		return err
	}
	perm, err := a.store.GranteePermission(r.Context(), subject, accountID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return httpx.ErrForbidden.WithMessage("You do not have access to this mailbox.")
		}
		return mapErr(err)
	}
	if !perm.AtLeast(need) {
		return httpx.ErrForbidden.WithMessage("Insufficient permission for this action.")
	}
	return nil
}

// requireMessageRead autoriza la LECTURA de un mensaje ya cargado: el dueño
// siempre; una cuenta ajena solo con ACL de al menos `read` (buzón compartido /
// delegación), para que el grantee siga leyendo cuerpo y adjuntos. Cualquier
// otro caso devuelve ErrNotFound —404, no 403— para no revelar la existencia de
// ids ajenos (anti-enumeración). Gate único de getMessage/getMessageRaw/Email/get.
func (a *API) requireMessageRead(r *http.Request, callerAccount uuid.UUID, m domain.Message) error {
	if m.AccountID == callerAccount {
		return nil
	}
	subject, err := a.callerSubject(r)
	if err != nil {
		return err
	}
	perm, err := a.store.GranteePermission(r.Context(), subject, m.AccountID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return err
	}
	if !perm.AtLeast(domain.PermRead) {
		return domain.ErrNotFound
	}
	return nil
}

// listSharedMailboxes lista los buzones (compartidos/delegados) accesibles al
// usuario, con su permiso. Sin N+1 (índice por grantee).
func (a *API) listSharedMailboxes(w http.ResponseWriter, r *http.Request) {
	subject, err := a.callerSubject(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	boxes, err := a.store.ListAccessibleMailboxes(r.Context(), subject)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if boxes == nil {
		boxes = []domain.SharedMailbox{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"shared_mailboxes": boxes})
}

type sharedMailboxReq struct {
	Address     string `json:"address"`
	DisplayName string `json:"display_name"`
}

// Tope de buzones compartidos. Cada uno es una dirección más en un dominio que
// comparten todos: sin techo, la ruta era una fábrica de direcciones.
// ponytail: constantes. Si una organización necesita más, que el tope venga del plan
// en el token, como la cuota del buzón.
const (
	maxBuzonesCompartidos         = 25
	maxBuzonesCompartidosPersonal = 1
)

// createSharedMailbox crea un buzón compartido de la organización. El creador
// queda como `manage`.
//
// Tres verjas, las mismas que un alias más el rol: lo crea quien ADMINISTRA la
// organización (owner o admin en su token), en una dirección admitida —dominio
// servido y no reservada, ver direccionAdmitida— y hasta el tope. Sin ellas,
// cualquier cuenta, también una personal gratuita, creaba `seguridad@<dominio raíz>`
// y recibía y enviaba en nombre de la plataforma, u ocupaba la dirección de otra
// persona antes de que existiera.
func (a *API) createSharedMailbox(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	claims, _ := authn.FromContext(r.Context())
	if !claims.HasAnyRole("owner", "admin") {
		httpx.Fail(w, r, httpx.ErrForbidden.WithCode("shared_mailbox_role").
			WithMessage("Only an owner or administrator of the organisation can create a shared mailbox."))
		return
	}
	var body sharedMailboxReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := a.direccionAdmitida(r, acc, body.Address); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	tope := maxBuzonesCompartidos
	if claims.OrgKind == "personal" {
		tope = maxBuzonesCompartidosPersonal
	}
	hay, err := a.store.ContarBuzonesCompartidos(r.Context())
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if hay >= tope {
		httpx.Fail(w, r, mapErr(domain.ErrBuzonesLimite))
		return
	}
	created, err := a.store.EnsureSharedMailbox(r.Context(), body.Address, body.DisplayName, a.cfg.DefaultMailboxQuotaBytes, acc.Subject)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"account_id": created.ID, "address": created.Address, "display_name": created.DisplayName,
		"kind": domain.KindShared, "permission": domain.PermManage,
	})
}

// listSharedFolders lista las carpetas de un buzón compartido/delegado (gated read).
func (a *API) listSharedFolders(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "accountID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid accountID."))
		return
	}
	if err := a.requireGrant(r, accountID, domain.PermRead); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	boxes, err := a.store.ListMailboxes(r.Context(), accountID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"mailboxes": boxes})
}

// listSharedMessages pagina (keyset) los mensajes de una carpeta de un buzón
// compartido/delegado (gated read). Verifica que la carpeta pertenece a la cuenta
// autorizada: un grantee no puede leer una carpeta de OTRA cuenta pasando su id.
func (a *API) listSharedMessages(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "accountID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid accountID."))
		return
	}
	mailboxID, err := uuid.Parse(chi.URLParam(r, "mailboxID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid mailboxID."))
		return
	}
	if err := a.requireGrant(r, accountID, domain.PermRead); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ok, err := a.store.MailboxInAccount(r.Context(), mailboxID, accountID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if !ok {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	limit := queryInt(r, 50)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	before, beforeID, err := parseCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid cursor."))
		return
	}
	msgs, err := a.store.ListMessages(r.Context(), mailboxID, limit, before, beforeID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	next := ""
	if len(msgs) == limit {
		last := msgs[len(msgs)-1]
		next = formatCursor(last.ReceivedAt, last.ID)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"messages": msgs, "limit": limit, "next_cursor": next})
}

type grantReq struct {
	Grantee    string `json:"grantee"`
	Permission string `json:"permission"`
	Delete     bool   `json:"delete"`
}

// listGrants lista el ACL de un buzón compartido (gated manage).
func (a *API) listGrants(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "accountID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid accountID."))
		return
	}
	if err := a.requireGrant(r, accountID, domain.PermManage); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a.respondGrants(w, r, accountID)
}

// postGrant concede/actualiza o revoca (delete:true) un acceso sobre un buzón
// compartido (gated manage). POST multiplexado por la forma del cuerpo (el
// checkOrigin CSRF de Astro rechaza PUT/DELETE del navegador).
func (a *API) postGrant(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "accountID"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid accountID."))
		return
	}
	if err := a.requireGrant(r, accountID, domain.PermManage); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a.applyGrant(w, r, accountID)
}

// listDelegation lista a quién he delegado MI buzón (dueño ⇒ manage implícito).
func (a *API) listDelegation(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a.respondGrants(w, r, acc.ID)
}

// postDelegation concede/revoca acceso delegado a MI buzón (asistente ↔ directivo).
func (a *API) postDelegation(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a.applyGrant(w, r, acc.ID)
}

// respondGrants serializa el ACL de una cuenta.
func (a *API) respondGrants(w http.ResponseWriter, r *http.Request, accountID uuid.UUID) {
	grants, err := a.store.ListGrants(r.Context(), accountID)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	if grants == nil {
		grants = []domain.MailboxGrant{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"grants": grants})
}

// applyGrant aplica un grant/revoke sobre accountID (el llamante ya está
// autorizado a gestionar el ACL: dueño de su cuenta o `manage` sobre la compartida).
func (a *API) applyGrant(w http.ResponseWriter, r *http.Request, accountID uuid.UUID) {
	var body grantReq
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.Grantee == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("grantee requerido."))
		return
	}
	if body.Delete {
		if err := a.store.RevokeAccess(r.Context(), accountID, body.Grantee); err != nil {
			httpx.Fail(w, r, mapErr(err))
			return
		}
		httpx.NoContent(w)
		return
	}
	if !domain.ValidPermission(body.Permission) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("permission must be read, send or manage."))
		return
	}
	if err := a.store.GrantAccess(r.Context(), accountID, body.Grantee, domain.Permission(body.Permission)); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("There is no account with that address in your organisation."))
			return
		}
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.NoContent(w)
}
