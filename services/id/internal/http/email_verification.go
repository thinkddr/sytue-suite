// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Verificación de email (alta sin invitación / cambio de email). Tres mitades:
//
//   - Envío/reenvío (autenticado, API JSON con JWT): POST
//     /api/v1/me/email/verify/send — el usuario dispara un enlace a SU PROPIA
//     dirección (no hay enumeración: solo puedes verificar tu cuenta). Reusa el
//     primitivo de token del reseteo (firmado, un-solo-uso, hash en BD) y el
//     Mailer. Rate-limitado como el login.
//
//   - Confirmación (pública, server-rendered, sin sesión): GET/POST /verify-email
//     — el enlace del correo. El GET solo valida la FIRMA y pinta un botón de
//     confirmar; el POST (verificado Origin, rate-limitado) CONSUME el token y
//     marca email_verified. Se separa GET/POST para que un prefetcher de correo no
//     queme el enlace de un solo uso con un simple GET (mismo criterio que el
//     reseteo y la aceptación de invitación).
//
// En el alta por invitación el email se da por verificado (vino por email); esto
// cubre el resto: altas sin invitación y —ligando el token a la dirección
// emitida— el cambio de email.
package http

import (
	"context"
	"errors"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// handleSendEmailVerification — POST /api/v1/me/email/verify/send — envía (o
// reenvía) el enlace de verificación a la dirección del usuario autenticado. Si ya
// está verificada, responde 200 sin emitir nada (idempotente, no molesta). El
// trabajo real (token + email) lo comparte con el reenvío: emitir uno nuevo purga
// los anteriores (solo el último enlace vale).
func (s *Server) handleSendEmailVerification(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	user, err := s.store.GetUserByID(r.Context(), c.Subject)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if user.EmailVerified {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"verified": true, "sent": false})
		return
	}
	if err := s.sendVerificationLink(r.Context(), user); err != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("The verification email could not be sent."))
		s.log.ErrorContext(r.Context(), "enviar verificación de email falló", "error", err)
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(user.ID, "", "user.email.verification_sent", user.Email, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"verified": false, "sent": true})
}

// sendVerificationLink emite un token de verificación FIRMADO ligado a la
// dirección actual del usuario, persiste su hash (invalidando los previos, en tx)
// y envía el enlace. Núcleo común del envío y el reenvío. Espeja sendResetLink.
func (s *Server) sendVerificationLink(ctx context.Context, user domain.User) error {
	return s.sendEmailLink(ctx, user.ID, user.Email, store.VerifyLogin)
}

// sendEmailLink emite el enlace de verificación de `email` para `purpose` (el
// correo de acceso o el de recuperación) y lo manda a esa dirección.
func (s *Server) sendEmailLink(ctx context.Context, userID, email, purpose string) error {
	token, hash, err := domain.NewResetToken(s.resetKey)
	if err != nil {
		return err
	}
	ttl := s.cfg.EmailVerifyTokenTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	exp := time.Now().Add(ttl)
	if err := s.store.InTx(ctx, func(q pg.Querier) error {
		return s.store.InsertEmailVerification(ctx, q, hash, userID, email, purpose, exp)
	}); err != nil {
		return err
	}
	link := strings.TrimRight(s.cfg.Issuer, "/") + "/verify-email?token=" + url.QueryEscape(token)
	return s.mailer.SendEmailVerification(ctx, email, link)
}

// handleVerifyEmailForm — GET /verify-email?token= — valida la FIRMA del token (sin
// tocar la BD) y pinta un botón de confirmación; si es inválido o manipulado, un
// aviso. El GET no muta estado (evita que un prefetcher queme el enlace).
func (s *Server) handleVerifyEmailForm(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	token := r.URL.Query().Get("token")
	if token == "" || !domain.VerifyResetTokenSig(s.resetKey, token) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(verifyInvalidPage(loc)))
		return
	}
	_, _ = w.Write([]byte(loginPage(loc, "Verificar correo", verifyFormHTML(html.EscapeString(token)))))
}

// handleVerifyEmailSubmit — POST /verify-email — valida token, lo CONSUME (atómico,
// un-solo-uso) y marca email_verified si la dirección sigue coincidiendo, en una
// sola transacción.
func (s *Server) handleVerifyEmailSubmit(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, "Petición rechazada: origen no permitido.")
		return
	}
	if err := r.ParseForm(); err != nil {
		httpFailHTML(w, r, http.StatusBadRequest, "Formulario inválido.")
		return
	}
	token := r.Form.Get("token")
	if token == "" || !domain.VerifyResetTokenSig(s.resetKey, token) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(verifyInvalidPage(loc)))
		return
	}
	ctx := r.Context()
	var userID, purpose string
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		uid, email, p, e := s.store.ConsumeEmailVerification(ctx, q, domain.HashToken(token), time.Now())
		if e != nil {
			return e
		}
		userID, purpose = uid, p
		if purpose == store.VerifyRecovery {
			return s.fijarCorreoDeRecuperacion(ctx, q, uid, email)
		}
		// Ligado a la dirección: si el correo cambió tras emitir el enlace, no marca
		// (RowsAffected=0); el token queda consumido igualmente (un solo uso).
		_, e = s.store.MarkEmailVerified(ctx, q, uid, email)
		return e
	})
	if errors.Is(err, domain.ErrConflict) {
		// La dirección ya da acceso a otra cuenta. Solo lo lee quien acaba de
		// demostrar que la dirección es suya, así que decirlo no delata a nadie.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "recovery.title"),
			alertHTML(i18n.T(loc, "recovery.taken"))+
				`<p class="alt"><a class="btn-link" href="/account">`+html.EscapeString(i18n.T(loc, "recovery.back"))+`</a></p>`)))
		return
	}
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Firma válida pero el token caducó, ya se usó o no existe.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(verifyInvalidPage(loc)))
			return
		}
		httpFailHTML(w, r, http.StatusInternalServerError, "No se pudo verificar el correo.")
		return
	}
	if purpose == store.VerifyRecovery {
		_ = s.store.Audit(ctx, nil, auditEntry(userID, "", "user.recovery_email.verified", userID, r))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "recovery.title"),
			`<p class="sub">`+html.EscapeString(i18n.T(loc, "recovery.verified"))+`</p>`+
				`<p class="alt"><a class="btn-link" href="/account">`+html.EscapeString(i18n.T(loc, "recovery.back"))+`</a></p>`)))
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(userID, "", "user.email.verified", userID, r))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(loginPage(loc, "Correo verificado",
		`<p class="sub">Tu correo está verificado. Ya puedes usar Sytue con normalidad.</p>`+
			`<p class="alt"><a class="btn-link" href="/login/">Ir a Sytue</a></p>`)))
}

// verifyFormHTML es el botón de confirmación (token en campo oculto): un solo POST
// same-origin consume el enlace y marca el correo verificado.
func verifyFormHTML(safeToken string) string {
	return `<p class="sub">Confirma que esta dirección de correo es tuya para completar tu cuenta de Sytue.</p>` +
		`<form method="post" action="/verify-email">` +
		`<input type="hidden" name="token" value="` + safeToken + `">` +
		`<button type="submit">Verificar mi correo</button></form>`
}

// verifyInvalidPage es el aviso de enlace de verificación inválido/caducado, con
// salida a reenviar uno nuevo desde la cuenta (sin callejones sin salida).
func verifyInvalidPage(loc string) string {
	return loginPage(loc, "Enlace no válido",
		alertHTML("Este enlace de verificación no es válido o ha caducado.")+
			`<p class="alt"><a class="btn-link" href="/login/">Inicia sesión y reenvía la verificación</a></p>`)
}
