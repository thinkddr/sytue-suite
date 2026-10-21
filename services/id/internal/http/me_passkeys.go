// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Borrado de passkeys por API JSON. El alta (`/webauthn/register/begin|finish`) y
// el listado (`/webauthn/credentials`) ya existían con bearer; quitar una solo se
// podía por el formulario de `/account`, así que una superficie que no renderice
// ese HTML podía dar de alta llaves y no retirarlas nunca.
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// ultimaPasskeyBloquea dice si quitar una credencial dejaría la cuenta con el
// segundo factor exigido y ningún factor con el que cumplirlo.
//
// Es la MISMA regla que aplica el formulario de /account, aquí para los dos
// llamantes: si queda una passkey y no hay TOTP confirmado, quitarla deja
// `mfa_enabled=true` sin nada detrás, y la siguiente aplicación que exija MFA da
// un paso imposible en vez de un acceso. Bloquear es lo contrario de un capricho:
// el estado que evita no se puede deshacer desde una cuenta a la que ya no se
// entra.
func (s *Server) ultimaPasskeyBloquea(r *http.Request, userID string) bool {
	creds, err := s.store.ListCredentialsForUser(r.Context(), userID)
	if err != nil || len(creds) > 1 {
		return false
	}
	if s.totpConfirmed(r, userID) {
		return false
	}
	user, err := s.store.GetUserByID(r.Context(), userID)
	return err == nil && user.MFAEnabled
}

// handleMePasskeyDelete — DELETE /api/v1/webauthn/credentials/{id} — retira una
// passkey propia. El `{id}` es el de `GET /webauthn/credentials`, y el store acota
// por usuario, así que un identificador ajeno no borra nada.
func (s *Server) handleMePasskeyDelete(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.webauthnUser(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if s.ultimaPasskeyBloquea(r, subject) {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage(
			"This is your only way to complete two-step verification. Set up an authenticator app first, or turn two-step verification off."))
		return
	}

	var borrada bool
	ctx := r.Context()
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		borrada, e = s.store.DeleteCredentialForUser(ctx, q, subject, id)
		return e
	})
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if !borrada {
		// 404 y no 204: aquí el identificador lo elige quien llama de una lista que
		// acaba de leer, así que «no existe» es información útil y no una filtración
		// (el store ya acotó por usuario: de otro no se sabe nada).
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	c, _ := claimsFrom(r.Context())
	_ = s.store.Audit(r.Context(), nil, auditEntry(subject, c.Tenant, "webauthn.credential.removed", id, r))
	s.avisarSeguridad(r.Context(), c.Tenant, subject, "security.passkey.removed", nil)
	w.WriteHeader(http.StatusNoContent)
}
