// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Revocación de sesiones por API. La lectura (`GET /me/sessions`) ya existía;
// faltaba el otro lado, que hasta ahora solo estaba como formulario HTML en
// `/account/sessions/revoke*`.
//
// No comparte handler con aquellos a propósito: el de HTML resuelve la identidad
// por la cookie del IdP y responde con redirecciones, y este por el JWT y con
// JSON. Fundirlos obligaría a ramificar en cada paso —quién eres, qué devuelvo,
// qué hago ante un error— por un ahorro de veinte líneas.
package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// handleMeSessionRevoke cierra UNA sesión del propio usuario.
//
// La verja es la pertenencia: `RevokeSessionByID` acota por `userID`, así que un
// identificador de otra persona no revoca nada y devuelve `revoked=false`. Se
// responde 204 igual —no 404— para no confirmar si esa sesión existe.
func (s *Server) handleMeSessionRevoke(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("session id must be a UUID."))
		return
	}
	var revoked bool
	ctx := r.Context()
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		revoked, e = s.store.RevokeSessionByID(ctx, q, c.Subject, id)
		return e
	})
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if revoked {
		_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, c.Tenant, "session.revoked", id, r))
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleMeSessionRevokeOthers cierra todas las del usuario menos la que está en
// uso. La actual se identifica por la cookie de sesión del IdP si viaja; si no
// —una app nativa, por ejemplo—, se cierran TODAS.
//
// Cerrar también la propia es la opción SEGURA de las dos, y es deliberada: quien
// pulsa «cerrar las demás» quiere que ninguna sesión que no controla siga abierta.
// Un bearer no lleva `sid`, así que sin la cookie el servicio no puede saber cuál
// es la suya; dejar viva «alguna» sería dejar viva precisamente la que no se puede
// identificar. Lo peor que pasa entonces es que hay que volver a entrar.
//
// Lo que pasaba HASTA AHORA era otra cosa, y peor: aquí se le pasaba a
// `RevokeOtherSessions` el HASH de la cookie —64 caracteres hex— o la cadena vacía,
// y la SQL lo compara con `sessions.id`, que es `uuid`. Postgres respondía 22P02
// («invalid input syntax for type uuid»), el manejador devolvía 500 y NO se cerraba
// ni una sesión. Desde ningún cliente, nunca, ni con cookie ni sin ella. En la web
// el fallo era mudo porque `SesionesCard.cerrar()` ignora la respuesta y relee la
// lista. La causa raíz es de tipos y se arregla en los dos sitios: aquí se resuelve
// la sesión por su hash y se pasa su `ID`, y el store deja de aceptar cualquier
// texto donde espera un uuid (ver `RevokeOtherSessions`).
func (s *Server) handleMeSessionRevokeOthers(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	ctx := r.Context()
	actual := ""
	if ck, err := s.requestCookie(r, sessionCookieName); err == nil && ck.Value != "" {
		// Y la sesión tiene que ser de QUIEN LLAMA: una cookie de otra persona
		// —o de una sesión ya revocada— no sirve para salvarse de la limpieza.
		sess, err := s.store.GetSessionByToken(ctx, domain.HashToken(ck.Value))
		if err == nil && sess.UserID == c.Subject && sess.Valid(time.Now()) {
			actual = sess.ID
		}
	}
	cerradas := 0
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		cerradas, e = s.store.RevokeOtherSessions(ctx, q, c.Subject, actual)
		return e
	})
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, c.Tenant, "session.revoked_others", strconv.Itoa(cerradas), r))
	w.WriteHeader(http.StatusNoContent)
}
