// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sección "Seguridad" de /account (self-service SIN JS): cambiar contraseña y
// activar/desactivar la verificación en dos pasos (TOTP). Igual patrón que las
// sesiones: formularios POST con verificación de Origin (sameOrigin), Post/
// Redirect/Get con un código de estado opaco en ?sec= (NUNCA se refleja input del
// usuario), y no-store. El núcleo (verificar, política, re-hash, revocar, validar
// TOTP) es el MISMO de la API JSON (me_security.go); aquí solo cambia el
// transporte (cookie de sesión + HTML en vez de bearer + JSON). El secreto TOTP
// pendiente se lee de la BD para pintar el paso de confirmación: nunca viaja por
// la URL.
package http

import (
	"encoding/base64"
	"errors"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/qr"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
	"git.sytue.eu/sytue/one/services/id/internal/totp"
)

// securityCSS estiliza la sección "Seguridad" de /account. Contenido puro (sin
// <style>): su sha256 entra en la CSP (ver cspStyleHashes en middleware).
const securityCSS = `.sub-sec{margin:22px 0 0;font-size:13px;font-weight:600;color:var(--muted);text-transform:uppercase;letter-spacing:.04em}` +
	`.sec-form{margin:8px 0 0}.sec-form button{margin-top:16px}` +
	// Los botones de estos formularios van solos en su fila, así que pueden partir
	// la etiqueta: el `nowrap` general de `.s-btn` está pensado para el botón de
	// fila de /account. «Neue Wiederherstellungscodes erzeugen» mide 330px y sacaba
	// el documento 42px fuera de un iPhone SE (barrido móvil del 2026-09-08).
	`.sec-form .s-btn{white-space:normal;overflow-wrap:anywhere;max-width:100%}` +
	`.totp-state{display:flex;align-items:center;gap:8px;margin:10px 0 0}` +
	`.totp-key{margin:12px 0 0;padding:10px 12px;border:1px solid var(--border);border-radius:8px;background:var(--bg);` +
	`font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:16px;letter-spacing:.16em;text-align:center;overflow-wrap:anywhere}` +
	`.totp-uri{margin:6px 0 0;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:11px;color:var(--muted);overflow-wrap:anywhere}` +
	`.qr{margin:12px 0 0;display:flex;justify-content:center}` +
	`.qr img{width:184px;height:184px;border:1px solid var(--border);border-radius:8px;padding:8px;background:#fff}` +
	`.rc-status{margin:10px 0 0;color:var(--muted);font-size:13px;line-height:1.5}` +
	`.rc-intro{margin:8px 0 0;font-size:14px;line-height:1.55}` +
	`.rc-list{margin:16px 0 0;padding:14px 16px;list-style:none;border:1px solid var(--border);border-radius:8px;background:var(--bg);` +
	`display:grid;grid-template-columns:repeat(2,1fr);gap:8px 20px;` +
	`font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:15px;letter-spacing:.08em;text-align:center}` +
	`.rc-list li{padding:2px 0}` +
	`.rc-actions{display:flex;flex-wrap:wrap;gap:8px;margin-top:20px}` +
	`.rc-actions a{flex:1 1 auto;text-align:center;padding:10px 12px;border-radius:var(--radius-control);text-decoration:none;font-weight:600;font-size:14px;white-space:nowrap;transition:background .15s,border-color .15s}` +
	`.rc-dl{color:var(--text);background:transparent;border:1px solid var(--border)}.rc-dl:hover{background:var(--bg);border-color:var(--border-2)}` +
	`.rc-done{color:#fff;background:var(--brand)}.rc-done:hover{background:var(--brand-h)}` +
	`.rc-actions a:focus-visible{outline:none;box-shadow:0 0 0 3px var(--ring)}` +
	`.hint{margin:8px 0 0;color:var(--muted);font-size:13px;line-height:1.5}` +
	`.note{display:flex;gap:8px;align-items:flex-start;margin:12px 0 0;padding:10px 12px;border-radius:8px;font-size:14px;line-height:1.45}` +
	`.note.ok{background:var(--ok-bg);color:var(--ok)}.note.err{background:var(--danger-bg);color:var(--danger)}` +
	`.btn-danger{margin-top:16px;width:100%;padding:10px 12px;font:inherit;font-weight:600;color:var(--danger);` +
	`background:transparent;border:1px solid var(--border);border-radius:var(--radius-control);cursor:pointer;transition:background .15s,border-color .15s}` +
	`.btn-danger:hover{background:var(--danger-bg);border-color:var(--danger)}.btn-danger:focus-visible{outline:none;box-shadow:0 0 0 3px var(--ring)}`

// passwordFormHTML es el formulario de cambio de contraseña (sin datos de usuario,
// constante). autocomplete y minlength dan las pistas al navegador; la política y
// la coincidencia las valida el servidor.
func passwordFormHTML(loc string) string {
	esc := html.EscapeString
	return `<form method="post" action="/account/password" class="sec-form">` +
		`<label for="sec-cur">` + esc(i18n.T(loc, "security.current_password")) + `</label>` +
		`<input id="sec-cur" type="password" name="current_password" autocomplete="current-password" required>` +
		`<label for="sec-new">` + esc(i18n.T(loc, "forgot.new_password")) + `</label>` +
		`<input id="sec-new" type="password" name="new_password" autocomplete="new-password" minlength="8" required>` +
		`<label for="sec-cf">` + esc(i18n.T(loc, "security.repeat_new_password")) + `</label>` +
		`<input id="sec-cf" type="password" name="confirm_password" autocomplete="new-password" minlength="8" required>` +
		`<button type="submit">` + esc(i18n.T(loc, "security.change_password")) + `</button></form>`
}

// totpInactiveHTML: sin TOTP configurado, botón para empezar el enrolamiento.
func totpInactiveHTML(loc string) string {
	esc := html.EscapeString
	return `<div class="totp-state"><span class="pill off">` + esc(i18n.T(loc, "security.off")) + `</span></div>` +
		`<p class="hint">` + esc(i18n.T(loc, "security.two_step_hint")) + `</p>` +
		`<form method="post" action="/account/mfa/totp/enroll" class="sec-form"><button type="submit">` + esc(i18n.T(loc, "security.enable_two_step")) + `</button></form>`
}

// securitySectionHTML construye la sección "Seguridad" de /account: cambio de
// contraseña y el bloque TOTP según su estado (activa / enrolamiento pendiente /
// desactivada), con el aviso de resultado de la última acción (?sec=).
func (s *Server) securitySectionHTML(r *http.Request, user domain.User) string {
	loc := i18n.From(r)
	// Sin <style> ni <h2>: desde el rediseño esta sección vive dentro de una
	// tarjeta del panel, que ya pone el título, y su CSS va en el <head> (ver
	// accountPage). La página de códigos de recuperación, que sigue usando el
	// shell del login, sí lo lleva en línea.
	var b strings.Builder
	if note := securityNoteHTML(r.URL.Query().Get("sec")); note != "" {
		b.WriteString(note)
	}
	b.WriteString(`<h3 class="sub-sec">` + html.EscapeString(i18n.T(loc, "security.password")) + `</h3>`)
	b.WriteString(passwordFormHTML(loc))
	b.WriteString(`<h3 class="sub-sec">` + html.EscapeString(i18n.T(loc, "security.two_step")) + `</h3>`)
	switch {
	case s.totpConfirmed(r, user.ID):
		b.WriteString(s.totpActiveHTML(r, user.ID))
	case s.totpPending(r, user.ID):
		b.WriteString(s.totpPendingHTML(r, user))
	default:
		b.WriteString(totpInactiveHTML(loc))
	}
	return b.String()
}

// totpActiveHTML: TOTP activo. Muestra cuántos códigos de recuperación quedan sin
// usar, el botón para regenerarlos y el formulario de desactivación (que exige un
// código vigente o la contraseña).
func (s *Server) totpActiveHTML(r *http.Request, userID string) string {
	loc := i18n.From(r)
	esc := html.EscapeString
	n, _ := s.store.CountRecoveryCodes(r.Context(), userID)
	return `<div class="totp-state"><span class="pill ok">` + esc(i18n.T(loc, "security.on")) + `</span></div>` +
		`<p class="rc-status">` + esc(i18n.T(loc, "security.recovery_left")) + ` <strong>` + strconv.Itoa(n) + `</strong>. ` +
		esc(i18n.T(loc, "security.recovery_hint")) + `</p>` +
		`<form method="post" action="/account/mfa/totp/recovery/regenerate" class="sec-form">` +
		`<button type="submit" class="s-btn">` + esc(i18n.T(loc, "security.regenerate_recovery")) + `</button></form>` +
		`<p class="hint">` + esc(i18n.T(loc, "security.disable_hint")) + `</p>` +
		`<form method="post" action="/account/mfa/totp/disable" class="sec-form">` +
		`<label for="dis-code">` + esc(i18n.T(loc, "security.app_code")) + `</label>` +
		`<input id="dis-code" class="code" type="text" name="code" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]*" maxlength="6" spellcheck="false">` +
		`<label for="dis-pw">` + esc(i18n.T(loc, "security.or_password")) + `</label>` +
		`<input id="dis-pw" type="password" name="password" autocomplete="current-password">` +
		`<button type="submit" class="btn-danger">` + esc(i18n.T(loc, "security.disable_two_step")) + `</button></form>`
}

// totpPendingHTML pinta el paso de enrolamiento pendiente: el QR (generado
// server-side, SVG inline: la CSP no exige hash para <svg> en línea) más la clave
// en texto (para escanear O teclear en la app) y el formulario para confirmar con
// el primer código. El secreto se lee de la BD (no viaja por la URL).
func (s *Server) totpPendingHTML(r *http.Request, user domain.User) string {
	esc := html.EscapeString
	loc := i18n.From(r)
	secret, err := s.store.GetPendingTOTPSecret(r.Context(), user.ID)
	if err != nil {
		// Estado inconsistente entre la comprobación y el render: cae a inactivo.
		return totpInactiveHTML(loc)
	}
	b32 := totp.EncodeSecret(secret)
	uri := totp.ProvisioningURI(s.mfaIssuerLabel(), user.Email, secret)
	// PNG incrustado y no SVG en línea: varios gestores de TOTP ofrecen «subir una
	// imagen del código» y filtran el selector por PNG/JPEG, así que con un SVG ni
	// se podía elegir el fichero —y «guardar imagen» daba uno que tampoco abrían—.
	// Si por lo que sea no cabe en un QR, se cae con elegancia a la clave.
	codigo := ""
	if b, e := qr.PNG(uri); e == nil {
		codigo = `<div class="qr"><img width="200" height="200" alt="` +
			esc(i18n.T(loc, "security.qr_alt")) + `" src="data:image/png;base64,` +
			base64.StdEncoding.EncodeToString(b) + `"></div>`
	}
	return `<div class="totp-state"><span class="pill off">` + esc(i18n.T(loc, "security.pending")) + `</span></div>` +
		`<p class="hint">` + esc(i18n.T(loc, "security.scan_hint")) + `</p>` +
		codigo +
		`<div class="totp-key" aria-label="Clave de configuración">` + esc(groupSecret(b32)) + `</div>` +
		`<p class="totp-uri">` + esc(uri) + `</p>` +
		`<form method="post" action="/account/mfa/totp/confirm" class="sec-form">` +
		`<label for="conf-code">` + esc(i18n.T(loc, "security.six_digits")) + `</label>` +
		`<input id="conf-code" class="code" type="text" name="code" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]*" maxlength="6" spellcheck="false" autofocus required>` +
		`<button type="submit">` + esc(i18n.T(loc, "security.confirm_enable")) + `</button></form>` +
		`<form method="post" action="/account/mfa/totp/disable" class="sec-form">` +
		`<button type="submit" class="btn-danger">` + esc(i18n.T(loc, "common.cancel")) + `</button></form>`
}

// groupSecret separa la clave base32 en bloques de 4 para leerla más fácil.
func groupSecret(b32 string) string {
	var b strings.Builder
	for i, c := range b32 {
		if i > 0 && i%4 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// securityNoteHTML mapea el código de estado opaco de ?sec= a un aviso fijo (ok o
// error). Al ser un conjunto cerrado de códigos, NO se refleja entrada del usuario.
func securityNoteHTML(code string) string {
	note := func(ok bool, msg string) string {
		cls := "err"
		if ok {
			cls = "ok"
		}
		return `<div class="note ` + cls + `" role="status">` + html.EscapeString(msg) + `</div>`
	}
	switch code {
	case "pw_ok":
		return note(true, "Tu contraseña se ha actualizado. Se cerraron las demás sesiones.")
	case "pw_bad":
		return note(false, "La contraseña actual no es correcta.")
	case "pw_weak":
		return note(false, "La nueva contraseña debe tener al menos 8 caracteres y ser distinta de la actual.")
	case "pw_mismatch":
		return note(false, "Las contraseñas nuevas no coinciden.")
	case "totp_on":
		return note(true, "Verificación en dos pasos activada.")
	case "totp_off":
		return note(true, "Verificación en dos pasos desactivada.")
	case "totp_bad":
		return note(false, "El código no es correcto. Revisa la hora de tu dispositivo e inténtalo de nuevo.")
	case "totp_verify_bad":
		return note(false, "No se pudo verificar tu identidad. Prueba con un código nuevo o tu contraseña.")
	default:
		return ""
	}
}

// totpConfirmed indica si el usuario tiene un TOTP confirmado (activo).
func (s *Server) totpConfirmed(r *http.Request, userID string) bool {
	_, err := s.store.GetTOTPSecret(r.Context(), userID)
	return err == nil
}

// totpPending indica si el usuario tiene un enrolamiento TOTP sin confirmar.
func (s *Server) totpPending(r *http.Request, userID string) bool {
	_, err := s.store.GetPendingTOTPSecret(r.Context(), userID)
	return err == nil
}

// accountRedirect es el PRG a /account con un código de estado opaco y anclado a
// la sección de seguridad. See Other (303): el reenvío del navegador es un GET.
func accountRedirect(w http.ResponseWriter, r *http.Request, sec string) {
	loc := "/account#seguridad"
	if sec != "" {
		loc = "/account?sec=" + sec + "#seguridad"
	}
	http.Redirect(w, r, loc, http.StatusSeeOther)
}

// handleAccountPassword — POST /account/password — cambio de contraseña
// self-service. Conserva la sesión actual (sess.ID) y revoca las demás.
func (s *Server) handleAccountPassword(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login/?return_to=/account", http.StatusFound)
		return
	}
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, i18n.T(i18n.From(r), "common.bad_origin"))
		return
	}
	if err := r.ParseForm(); err != nil {
		httpFailHTML(w, r, http.StatusBadRequest, i18n.T(i18n.From(r), "common.bad_form"))
		return
	}
	cur := r.Form.Get("current_password")
	nw := r.Form.Get("new_password")
	if nw != r.Form.Get("confirm_password") {
		accountRedirect(w, r, "pw_mismatch")
		return
	}
	if _, err := s.changeUserPassword(r, sess.UserID, cur, nw, sess.ID); err != nil {
		switch {
		case errors.Is(err, errWrongCurrentPassword):
			_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "user.password.change_failed", "", r))
			accountRedirect(w, r, "pw_bad")
		case errors.Is(err, domain.ErrWeakPassword), errors.Is(err, domain.ErrPasswordComplexity):
			accountRedirect(w, r, "pw_weak")
		default:
			httpFailHTML(w, r, http.StatusInternalServerError, "No se pudo cambiar la contraseña.")
		}
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "user.password.changed", "", r))
	s.avisarSeguridad(r.Context(), sess.OrgID, sess.UserID, "security.password.changed", nil)
	accountRedirect(w, r, "pw_ok")
}

// handleAccountTOTPEnroll — POST /account/mfa/totp/enroll — genera el secreto
// pendiente y vuelve a /account, que ya mostrará el paso de confirmación. Si el
// TOTP ya está activo, no hace nada (evita pisarlo).
func (s *Server) handleAccountTOTPEnroll(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login/?return_to=/account", http.StatusFound)
		return
	}
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, i18n.T(i18n.From(r), "common.bad_origin"))
		return
	}
	if s.totpConfirmed(r, sess.UserID) {
		accountRedirect(w, r, "")
		return
	}
	user, err := s.store.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, "No se pudo cargar tu cuenta.")
		return
	}
	secret, err := totp.GenerateSecret()
	if err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, "No se pudo generar el secreto.")
		return
	}
	if err := s.store.UpsertTOTPSecret(r.Context(), nil, user.ID, secret, false); err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, "No se pudo iniciar la configuración.")
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(user.ID, sess.OrgID, "mfa.totp.enroll_begin", "", r))
	accountRedirect(w, r, "")
}

// handleAccountTOTPConfirm — POST /account/mfa/totp/confirm — valida el primer
// código contra el secreto pendiente y activa el TOTP.
func (s *Server) handleAccountTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login/?return_to=/account", http.StatusFound)
		return
	}
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, i18n.T(i18n.From(r), "common.bad_origin"))
		return
	}
	if err := r.ParseForm(); err != nil {
		httpFailHTML(w, r, http.StatusBadRequest, i18n.T(i18n.From(r), "common.bad_form"))
		return
	}
	secret, err := s.store.GetPendingTOTPSecret(r.Context(), sess.UserID)
	if err != nil {
		accountRedirect(w, r, "") // nada pendiente: vuelve al estado real
		return
	}
	if !totp.Validate(secret, r.Form.Get("code"), time.Now()) {
		_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "mfa.totp.confirm_failed", "", r))
		accountRedirect(w, r, "totp_bad")
		return
	}
	codes, e := s.activateTOTP(r, sess.UserID)
	if e != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, "No se pudo activar la verificación.")
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "mfa.totp.enabled", "", r))
	s.avisarSeguridad(r.Context(), sess.OrgID, sess.UserID, "security.mfa.enabled", nil)
	// Los códigos de recuperación se muestran UNA vez (no hay PRG: tras el redirect
	// ya no podrían recuperarse — solo se guarda su hash). Página terminal con salida.
	s.writeRecoveryCodesPage(w, r, "Verificación en dos pasos activada",
		"Guarda estos códigos de recuperación en un lugar seguro. Te permitirán entrar si pierdes el acceso a tu app de autenticación. Cada uno funciona una sola vez y no volverás a verlos.", codes)
}

// handleAccountRecoveryRegenerate — POST /account/mfa/totp/recovery/regenerate —
// regenera los códigos de recuperación (exige TOTP activo) y los muestra una vez;
// los anteriores dejan de servir.
func (s *Server) handleAccountRecoveryRegenerate(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login/?return_to=/account", http.StatusFound)
		return
	}
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, i18n.T(i18n.From(r), "common.bad_origin"))
		return
	}
	if !s.totpConfirmed(r, sess.UserID) {
		// Sin TOTP activo no hay códigos que regenerar: vuelve al estado real.
		accountRedirect(w, r, "")
		return
	}
	codes, err := s.regenerateRecoveryCodes(r, sess.UserID)
	if err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, "No se pudieron generar los códigos.")
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "mfa.totp.recovery_regenerated", "", r))
	s.writeRecoveryCodesPage(w, r, "Códigos de recuperación nuevos",
		"Se han generado códigos nuevos. Los anteriores han dejado de servir. Guárdalos: cada uno funciona una sola vez.", codes)
}

// writeRecoveryCodesPage renderiza la página que muestra los códigos de recuperación
// UNA sola vez, con el mismo shell de marca. no-store: no deben quedar en caché.
func (s *Server) writeRecoveryCodesPage(w http.ResponseWriter, r *http.Request, title, intro string, codes []string) {
	loc := i18n.From(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(loginPage(loc, title, recoveryCodesBody(loc, intro, codes))))
}

// recoveryCodesBody construye el cuerpo de la página de códigos: la lista en rejilla
// monoespaciada, un enlace de descarga (.txt vía data URI, sin JS) y la salida a la
// cuenta. Los códigos se escapan por defensa en profundidad (son alfanuméricos).
func recoveryCodesBody(loc, intro string, codes []string) string {
	esc := html.EscapeString
	var list, plain strings.Builder
	list.WriteString(`<ul class="rc-list">`)
	for _, c := range codes {
		list.WriteString(`<li>` + esc(c) + `</li>`)
		plain.WriteString(c)
		plain.WriteString("\r\n")
	}
	list.WriteString(`</ul>`)
	// Descarga sin JS: data URI con el texto en base64. El atributo download fuerza
	// la descarga; la CSP (directivas de fetch) no gobierna las descargas de anclas.
	href := "data:text/plain;charset=utf-8;base64," + base64.StdEncoding.EncodeToString([]byte(plain.String()))
	return `<style>` + securityCSS + `</style>` +
		`<p class="rc-intro">` + esc(intro) + `</p>` +
		list.String() +
		`<div class="rc-actions">` +
		`<a class="rc-dl" href="` + href + `" download="sytue-codigos-recuperacion.txt">` + esc(i18n.T(loc, "common.download")) + `</a>` +
		`<a class="rc-done" href="/account#seguridad">` + esc(i18n.T(loc, "common.done")) + `</a>` +
		`</div>`
}

// handleAccountTOTPDisable — POST /account/mfa/totp/disable — desactiva un TOTP
// activo (exige código o contraseña) o cancela un enrolamiento pendiente.
func (s *Server) handleAccountTOTPDisable(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login/?return_to=/account", http.StatusFound)
		return
	}
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, i18n.T(i18n.From(r), "common.bad_origin"))
		return
	}
	if err := r.ParseForm(); err != nil {
		httpFailHTML(w, r, http.StatusBadRequest, i18n.T(i18n.From(r), "common.bad_form"))
		return
	}
	user, err := s.store.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, "No se pudo cargar tu cuenta.")
		return
	}
	secret, err := s.store.GetTOTPSecret(r.Context(), user.ID)
	if err == nil {
		// TOTP activo: exige prueba de identidad.
		req := totpDisableRequest{Code: r.Form.Get("code"), Password: r.Form.Get("password")}
		if !s.verifyTOTPDisable(user, secret, req) {
			_ = s.store.Audit(r.Context(), nil, auditEntry(user.ID, sess.OrgID, "mfa.totp.disable_failed", "", r))
			accountRedirect(w, r, "totp_verify_bad")
			return
		}
		if e := s.disableTOTP(r, user.ID); e != nil {
			httpFailHTML(w, r, http.StatusInternalServerError, "No se pudo desactivar la verificación.")
			return
		}
		_ = s.store.Audit(r.Context(), nil, auditEntry(user.ID, sess.OrgID, "mfa.totp.disabled", "", r))
		s.avisarSeguridad(r.Context(), sess.OrgID, user.ID, "security.mfa.disabled", nil)
		accountRedirect(w, r, "totp_off")
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		// Sin TOTP activo: si hay uno pendiente, cancelarlo (no baja MFA).
		if _, perr := s.store.GetPendingTOTPSecret(r.Context(), user.ID); perr == nil {
			_ = s.store.DeleteTOTPSecret(r.Context(), nil, user.ID)
		}
		accountRedirect(w, r, "")
		return
	}
	httpFailHTML(w, r, http.StatusInternalServerError, "No se pudo desactivar la verificación.")
}
