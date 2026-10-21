// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Flujo "olvidé mi contraseña" (server-rendered, sin JS): solicitud del enlace
// (respuesta UNIFORME, no enumera cuentas) y fijado de la nueva contraseña con la
// política de dominio. El token es firmado (HMAC), de un solo uso, con caducidad
// corta y solo su hash vive en BD (ver domain.NewResetToken + store.reset). Igual
// que /account, los POST verifican Origin (CSRF) y el envío de email va rate-
// limitado como el login.
package http

import (
	"context"
	"errors"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// handleForgotPasswordForm — GET /forgot-password — pinta el formulario de
// solicitud, o el aviso uniforme de "enlace enviado" tras el POST (?sent=1).
func (s *Server) handleForgotPasswordForm(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// El viaje que traía el login (la app que pidió entrar) se conserva de punta a
	// punta: enlace de vuelta, campo oculto del POST y redirección a ?sent=1. Sin
	// esto, pasar por «olvidé mi contraseña» aterrizaba en la cuenta de Sytue ID y
	// obligaba a rehacer el OAuth. Solo se transporta: quien valida es
	// redirectAfterLogin.
	returnTo := r.URL.Query().Get("return_to")
	backToLogin := `<p class="alt"><a class="btn-link" href="` +
		html.EscapeString(withReturnTo("/login/", returnTo)) + `">` +
		html.EscapeString(i18n.T(loc, "login.back")) + `</a></p>`
	if r.URL.Query().Get("sent") == "1" {
		_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "forgot.sent_title"),
			`<p class="sub">`+html.EscapeString(i18n.T(loc, "forgot.sent_body"))+`</p>`+
				backToLogin)))
		return
	}
	_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "forgot.reset_title"),
		`<p class="sub">`+html.EscapeString(i18n.T(loc, "forgot.subtitle"))+`</p>`+
			`<form method="post" action="/forgot-password">`+
			`<input type="hidden" name="return_to" value="`+html.EscapeString(returnTo)+`">`+
			`<label for="email">`+html.EscapeString(i18n.T(loc, "login.identifier"))+`</label>`+
			`<input id="email"`+emailFieldAttrs+` autofocus>`+
			`<button type="submit">`+html.EscapeString(i18n.T(loc, "forgot.send"))+`</button></form>`+
			`<script>`+emailFieldScript+`</script>`+
			backToLogin)))
}

// handleForgotPasswordSubmit — POST /forgot-password — SIEMPRE responde igual
// (PRG a ?sent=1), exista la cuenta o no (no enumera). El trabajo (lookup + token
// + email) se hace en segundo plano para que la latencia de red del envío no
// filtre la existencia de la cuenta por temporización.
func (s *Server) handleForgotPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, "Petición rechazada: origen no permitido.")
		return
	}
	if err := r.ParseForm(); err != nil {
		httpFailHTML(w, r, http.StatusBadRequest, "Formulario inválido.")
		return
	}
	email := domain.NormalizeLogin(r.Form.Get("email"))
	if email != "" {
		// Contexto desligado de la petición (el handler responde ya) y datos del
		// cliente capturados por valor: r no es seguro de usar tras el return.
		ctx := context.WithoutCancel(r.Context())
		go s.dispatchResetEmail(ctx, email, clientIP(r), r.UserAgent())
	}
	// El PRG conserva el return_to del formulario: la página de «enlace enviado»
	// necesita ofrecer la vuelta al login del viaje original, no al genérico.
	sent := "/forgot-password?sent=1"
	if rt := r.Form.Get("return_to"); rt != "" {
		sent += "&" + url.Values{"return_to": {rt}}.Encode()
	}
	http.Redirect(w, r, sent, http.StatusSeeOther)
}

// dispatchResetEmail hace el trabajo real fuera del camino de respuesta: si la
// cuenta existe y puede recibir contraseña, emite el token firmado, guarda su
// hash (invalidando los previos) y envía el enlace. Cualquier fallo se registra;
// nunca se propaga al usuario (la respuesta ya fue uniforme).
func (s *Server) dispatchResetEmail(ctx context.Context, email, ip, ua string) {
	user, err := s.userByLogin(ctx, email)
	if err != nil || !user.Active() || user.PasswordHash == "" {
		// Sin cuenta, suspendida o solo-SSO (sin contraseña que restablecer): nada.
		return
	}
	if err := s.sendResetLink(ctx, user); err != nil {
		s.log.ErrorContext(ctx, "enviar enlace de reseteo falló", "error", err)
		return
	}
	_ = s.store.Audit(ctx, nil, store.AuditEntry{
		ActorID: user.ID, Action: "auth.password.reset_requested", IP: ip,
		Detail: map[string]any{"at": time.Now().UTC().Format(time.RFC3339), "user_agent": ua},
	})
}

// sendResetLink emite un token de reseteo FIRMADO, persiste su hash (invalidando
// los previos del usuario, en tx) y envía el enlace por email. Es el núcleo común
// del "olvidé mi contraseña" (self-service) y del reseteo disparado por un admin
// (admin_members.go): mismo token de un-solo-uso, caducidad y correo. No audita
// ni aplica política de enumeración: eso lo decide cada llamante según su modelo.
//
// Va al correo de acceso Y al de recuperación si lo hay (ADR 0041). En una
// cuenta creada por @usuario el de acceso es su propio buzón de Sytue, al que no
// se llega sin contraseña: el de recuperación es el que la devuelve. Los dos
// llevan el mismo enlace y el primero que se use invalida al otro.
func (s *Server) sendResetLink(ctx context.Context, user domain.User) error {
	link, err := s.enlaceDeReseteo(ctx, user.ID)
	if err != nil {
		return err
	}
	err = s.mailer.SendPasswordReset(ctx, user.Email, link)
	if user.RecoveryEmail != "" && !strings.EqualFold(user.RecoveryEmail, user.Email) {
		err = errors.Join(err, s.mailer.SendPasswordReset(ctx, user.RecoveryEmail, link))
	}
	return err
}

// enlaceDeReseteo emite el token firmado de un solo uso, guarda su hash
// (invalidando los anteriores del usuario) y devuelve el enlace a
// /reset-password. Lo usan el correo de «olvidé mi contraseña» y el login cuando
// la contraseña con la que se entra es temporal: los dos acaban en el mismo
// formulario, con la misma caducidad y la misma política.
func (s *Server) enlaceDeReseteo(ctx context.Context, userID string) (string, error) {
	token, hash, err := domain.NewResetToken(s.resetKey)
	if err != nil {
		return "", err
	}
	exp := time.Now().Add(s.cfg.ResetTokenTTL)
	if err := s.store.InTx(ctx, func(q pg.Querier) error {
		return s.store.InsertResetToken(ctx, q, hash, userID, exp)
	}); err != nil {
		return "", err
	}
	return strings.TrimRight(s.cfg.Issuer, "/") + "/reset-password?token=" + url.QueryEscape(token), nil
}

// handleResetPasswordForm — GET /reset-password?token= — valida la FIRMA del token
// (sin tocar la BD) y pinta el formulario de nueva contraseña; si es inválido o
// manipulado, muestra un aviso para volver a pedir el enlace.
func (s *Server) handleResetPasswordForm(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	token := r.URL.Query().Get("token")
	if token == "" || !domain.VerifyResetTokenSig(s.resetKey, token) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(resetInvalidPage(loc)))
		return
	}
	_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "forgot.new_title"), resetFormHTML(loc, html.EscapeString(token), ""))))
}

// handleResetPasswordSubmit — POST /reset-password — valida token+política, consume
// el token (atómico, un-solo-uso) y fija la nueva contraseña revocando TODAS las
// sesiones del usuario, en una sola transacción.
func (s *Server) handleResetPasswordSubmit(w http.ResponseWriter, r *http.Request) {
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
	safeToken := html.EscapeString(token)
	if token == "" || !domain.VerifyResetTokenSig(s.resetKey, token) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(resetInvalidPage(loc)))
		return
	}
	pw := r.Form.Get("new_password")
	if pw != r.Form.Get("confirm_password") {
		resetFail(w, r, safeToken, "Las contraseñas no coinciden.")
		return
	}
	ctx := r.Context()
	var userID string
	// Validación DENTRO de la tx contra la política del usuario (conocido solo tras
	// consumir el token). Si falla, la tx revierte y el token NO se consume ⇒ una
	// contraseña débil no quema el enlace de un solo uso.
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		uid, e := s.store.ConsumeResetToken(ctx, q, domain.HashToken(token), time.Now())
		if e != nil {
			return e
		}
		userID = uid
		policy, e := s.store.EffectivePasswordPolicyForUser(ctx, uid)
		if e != nil {
			return e
		}
		if e := domain.ValidatePassword(policy, pw); e != nil {
			return e
		}
		hash, e := domain.HashPassword(pw)
		if e != nil {
			return e
		}
		if e := s.store.SetPassword(ctx, q, uid, hash); e != nil {
			return e
		}
		// Fijar contraseña por recuperación ⇒ revocar TODAS las sesiones y refresh
		// (una credencial pudo quedar comprometida). Mismo criterio que el reset admin.
		_, e = s.store.RevokeAllUserSessions(ctx, q, uid)
		return e
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidInput) {
			resetFail(w, r, safeToken, passwordErrMsg(err))
			return
		}
		if errors.Is(err, domain.ErrNotFound) {
			// Firma válida pero el token caducó, ya se usó o no existe.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(resetInvalidPage(loc)))
			return
		}
		httpFailHTML(w, r, http.StatusInternalServerError, "No se pudo cambiar la contraseña.")
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(userID, "", "auth.password.reset_completed", "", r))
	s.avisarSeguridad(r.Context(), "", userID, "security.password.changed", nil)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "forgot.updated_title"),
		`<p class="sub">`+html.EscapeString(i18n.T(loc, "forgot.updated_body"))+`</p>`+
			`<p class="alt"><a class="btn-link" href="/login/">`+html.EscapeString(i18n.T(loc, "login.title"))+`</a></p>`)))
}

// resetFormHTML es el formulario de nueva contraseña (token en campo oculto), con
// un aviso de error opcional. safeToken llega ya escapado.
func resetFormHTML(loc, safeToken, errMsg string) string {
	b := `<p class="sub">` + html.EscapeString(i18n.T(loc, "forgot.reset_subtitle")) + `</p>`
	if errMsg != "" {
		b += alertHTML(errMsg)
	}
	return b + `<form method="post" action="/reset-password">` +
		`<input type="hidden" name="token" value="` + safeToken + `">` +
		`<label for="new_password">` + html.EscapeString(i18n.T(loc, "forgot.new_password")) + `</label>` +
		`<input id="new_password" type="password" name="new_password" autocomplete="new-password" minlength="8" autofocus required>` +
		`<label for="confirm_password">` + html.EscapeString(i18n.T(loc, "forgot.repeat_password")) + `</label>` +
		`<input id="confirm_password" type="password" name="confirm_password" autocomplete="new-password" minlength="8" required>` +
		`<button type="submit">` + html.EscapeString(i18n.T(loc, "forgot.save")) + `</button></form>`
}

// resetFail re-renderiza el formulario de nueva contraseña con un error (422),
// conservando el token para reintentar.
func resetFail(w http.ResponseWriter, r *http.Request, safeToken, msg string) {
	loc := i18n.From(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "forgot.new_title"), resetFormHTML(loc, safeToken, msg))))
}

// resetInvalidPage es el aviso de enlace inválido/caducado, con salida a pedir uno
// nuevo (sin callejones sin salida).
func resetInvalidPage(loc string) string {
	return loginPage(loc, i18n.T(loc, "forgot.invalid_title"),
		alertHTML(i18n.T(loc, "forgot.invalid_body"))+
			`<p class="alt"><a class="btn-link" href="/forgot-password">`+html.EscapeString(i18n.T(loc, "forgot.request_new"))+`</a></p>`)
}
