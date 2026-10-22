// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Alta autoservicio: una persona crea su cuenta personal gratuita.
//
// Hasta aquí NADIE podía registrarse solo: las cuentas nacían por invitación de
// una organización o por un IdP externo. Esto abre la puerta a quien no tiene
// organización, y lo hace con el modelo de datos de siempre —una cuenta personal
// ES una organización de una persona (`kind = personal`), su nombre es su correo
// y su slug se deriva de él (domain.PersonalSlug)—. Lo que la distingue lo
// llevan `organizations.kind` y `org_subscriptions.plan`, y de ahí salen la
// cuota (5 GB gratis, 1 TB pagando) y la verja de los productos que no tiene
// (Sytue Admin, SyERP): ver mig 0063 y el ADR 0038.
//
// SE ELIGE UN @USUARIO, NO SE DA UN CORREO (ADR 0040). @ana es ana@sytue.eu: la
// dirección de la cuenta es la que Sytue le da, en el dominio raíz, y con ese
// nombre la encuentra cualquiera de otra organización. Pedir un correo ajeno
// obligaba a verificarlo —era la única defensa contra abrir cuentas a nombre de
// direcciones de otros— y a no poder decir si ya existía. Con el @usuario la
// dirección es NUESTRA y nace verificada, así que la cuenta entra al momento; y
// decir «ese @usuario no está disponible» no filtra nada, porque un @usuario es
// público por definición.
//
// Y SE DEMUESTRA UN MÓVIL (ADR 0041). El alta pide un móvil y le manda por SMS un
// código de un solo uso; la cuenta no nace hasta que se confirma. Lo que se
// rellenó espera mientras tanto en signup_phone_codes. Ese móvil es después uno
// de los cuatro identificadores con los que se entra. Que un número ya tenga
// cuenta NO se dice al rellenar el formulario —sería decirle a cualquiera de quién
// es un teléfono—: se dice después del código, cuando quien lo lee ya ha
// demostrado que el número es suyo. El captcha y los limitadores por IP y por
// número frenan el abuso, que aquí además cuesta dinero.
//
// Las cuentas que se registraron con un correo ajeno antes de esto siguen
// exigiendo verificarlo (login.go, msgVerifyFirst) y pueden pedir que se reenvíe.
package http

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
)

// altchaScript resuelve el reto criptográfico ALTCHA (Proof of Work) en el navegador
// usando Web Crypto (crypto.subtle) en lotes de 1.500 cálculos por microtarea.
// Su sha256 entra en la CSP (script-src), sin 'unsafe-inline' (ver cspScriptHashes).
const altchaScript = `(function(){` +
	`var b=document.getElementById('altcha-box');if(!b)return;` +
	`var c=document.getElementById('altcha-chk');` +
	`var t=document.getElementById('altcha-status');` +
	`var p=document.getElementById('altcha-payload');` +
	`var f=b.closest('form');` +
	`if(!c||!t||!p||!f||!window.crypto||!crypto.subtle)return;` +
	`var mv=b.getAttribute('data-verifying')||'Verificando...';` +
	`var mok=b.getAttribute('data-verified')||'Verificado';` +
	`var mf=b.getAttribute('data-failed')||'Error';` +
	`var r=false,s=false,waitSubmit=false;` +
	`function hex(u){var h='',a=new Uint8Array(u);for(var i=0;i<a.length;i++){h+=a[i].toString(16).padStart(2,'0');}return h;}` +
	`function run(){` +
	`if(r||s)return;` +
	`r=true;b.className='altcha-box verifying';c.disabled=true;t.textContent=mv;` +
	`fetch(b.getAttribute('data-challenge')).then(function(res){if(!res.ok)throw 0;return res.json();}).then(function(ch){` +
	`var enc=new TextEncoder();var salt=ch.salt;var tgt=ch.challenge;var max=ch.maxnumber||50000;var n=0;` +
	`function batch(){` +
	`var end=Math.min(n+1500,max+1);var pr=[];` +
	`for(var i=n;i<end;i++){(function(cur){pr.push(crypto.subtle.digest('SHA-256',enc.encode(salt+cur)).then(function(buf){return hex(buf)===tgt?cur:-1;}));})(i);}` +
	`return Promise.all(pr).then(function(res){for(var j=0;j<res.length;j++){if(res[j]>=0)return res[j];}n=end;if(n<=max)return batch();return -1;});` +
	`}` +
	`return batch().then(function(sol){` +
	`if(sol<0)throw 0;` +
	`p.value=btoa(JSON.stringify({algorithm:ch.algorithm,challenge:ch.challenge,number:sol,salt:ch.salt,signature:ch.signature}));` +
	`s=true;r=false;b.className='altcha-box verified';c.checked=true;c.disabled=false;c.required=false;t.textContent=mok;` +
	`if(waitSubmit){waitSubmit=false;f.requestSubmit?f.requestSubmit():f.submit();}` +
	`});` +
	`}).catch(function(){` +
	`r=false;waitSubmit=false;b.className='altcha-box';c.checked=false;c.disabled=false;t.textContent=mf;` +
	`});` +
	`}` +
	`c.addEventListener('click',function(e){if(s){e.preventDefault();return;}if(!r){e.preventDefault();run();}});` +
	`f.addEventListener('submit',function(e){if(!s){e.preventDefault();waitSubmit=true;if(!r)run();}});` +
	`})();`

// signupLinkHTML es el enlace «crea una cuenta» del login. Solo cuando el alta
// está publicada: ofrecer una puerta que da 404 es mala interfaz.
func signupLinkHTML(loc, safeReturn string) string {
	return `<p class="alt"><a class="btn-link" data-signup href="` +
		html.EscapeString(withReturnTo("/signup", html.UnescapeString(safeReturn))) +
		`">` + html.EscapeString(i18n.T(loc, "login.signup_link")) + `</a></p>`
}

// soloCuentasPersonales indica si TODAS las organizaciones del usuario son
// personales. Es lo que distingue a quien se registró por su cuenta —y tiene que
// verificar el correo antes de entrar— de quien llegó invitado o federado, que
// ya llega verificado y además pertenece a una organización de verdad.
func (s *Server) soloCuentasPersonales(ctx context.Context, userID string) bool {
	mems, err := s.store.ListOrgMembershipsForUser(ctx, userID)
	if err != nil || len(mems) == 0 {
		return false
	}
	for _, m := range mems {
		if m.Kind != domain.OrgKindPersonal {
			return false
		}
	}
	return true
}

// signupDisponible dice si el alta autoservicio está abierta: publicada y con
// SMS con el que verificar el móvil. Sin SMS no se dan cuentas.
func (s *Server) signupDisponible() bool { return s.cfg.SignupEnabled && s.sms != nil }

func (s *Server) handleSignupCaptcha(w http.ResponseWriter, r *http.Request) {
	s.escribirReto(w, r, s.signupDisponible())
}

// handleLoginCaptcha da el reto del captcha del login (/login/captcha).
func (s *Server) handleLoginCaptcha(w http.ResponseWriter, r *http.Request) {
	s.escribirReto(w, r, s.cfg.LoginCaptchaEnabled)
}

// escribirReto responde un reto ALTCHA nuevo si ese captcha está activo.
func (s *Server) escribirReto(w http.ResponseWriter, r *http.Request, activo bool) {
	if !activo || s.captcha == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	ch, err := s.captcha.CreateChallenge(50000, 10*time.Minute)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(ch)
}

func (s *Server) handleSignupForm(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !s.signupDisponible() {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "signup.title"),
			`<p class="sub">`+html.EscapeString(i18n.T(loc, "signup.disabled"))+`</p>`)))
		return
	}
	// ?sent=1 es la página de «revisa tu correo» del reenvío a las cuentas antiguas,
	// que se registraron con un correo ajeno y aún tienen que verificarlo.
	if r.URL.Query().Get("sent") == "1" {
		_, _ = w.Write([]byte(signupSentPage(loc)))
		return
	}
	safeReturn := html.EscapeString(r.URL.Query().Get("return_to"))
	_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "signup.title"),
		signupFormHTML(loc, safeReturn, "", "", "", suiteDomain(s.cfg.Issuer), "", s.cfg.SignupCaptchaEnabled))))
}

// handleSignupSubmit es el primer paso del alta: valida lo rellenado, lo guarda
// como alta pendiente y manda el código al móvil. La cuenta nace en
// handleSignupCode.
//
//nolint:gocyclo // alta autoservicio: captcha, @usuario, móvil, contraseña y SMS en un flujo lineal.
func (s *Server) handleSignupSubmit(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	if !s.signupDisponible() {
		httpFailHTML(w, r, http.StatusNotFound, i18n.T(loc, "signup.disabled"))
		return
	}
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, i18n.T(loc, "common.bad_origin"))
		return
	}
	if err := r.ParseForm(); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	username := domain.NormalizeUsername(r.Form.Get("username"))
	name := strings.TrimSpace(r.Form.Get("name"))
	password := r.Form.Get("password")
	phoneRaw := strings.TrimSpace(r.Form.Get("phone"))
	returnTo := r.Form.Get("return_to")
	dominio := suiteDomain(s.cfg.Issuer)

	fallo := func(status int, msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "signup.title"),
			signupFormHTML(loc, html.EscapeString(returnTo), html.EscapeString(username),
				html.EscapeString(name), html.EscapeString(phoneRaw), dominio, i18n.T(loc, msg), s.cfg.SignupCaptchaEnabled))))
	}
	if s.cfg.SignupCaptchaEnabled && s.captcha != nil {
		altcha := r.Form.Get("altcha")
		if ok, err := s.captcha.Verify(r.Context(), altcha); !ok || err != nil {
			fallo(http.StatusBadRequest, "signup.captcha_failed")
			return
		}
	}
	if !domain.ValidUsername(username) {
		fallo(http.StatusBadRequest, "signup.username_invalid")
		return
	}
	// Reservado se dice igual que ocupado: «soporte» no es un nombre mal escrito,
	// es uno que no se puede tener.
	if domain.UsernameReservado(username) {
		fallo(http.StatusConflict, "signup.username_taken")
		return
	}
	phone, ok := domain.NormalizePhone(phoneRaw)
	if !ok {
		fallo(http.StatusBadRequest, "signup.phone_invalid")
		return
	}
	// La política por defecto de la casa (12 caracteres): una cuenta personal no
	// tiene organización que la endurezca, y la suya es la de todos.
	if err := domain.ValidatePassword(domain.DefaultPasswordPolicy, password); err != nil {
		fallo(http.StatusBadRequest, "signup.weak_password")
		return
	}
	if name == "" {
		name = username
	}

	ctx := r.Context()
	// Antes de gastar un SMS: si el @usuario está cogido, que se sepa ya y no
	// después de esperar el código. Se vuelve a comprobar al crear la cuenta.
	libre, err := s.store.UsernameLibre(ctx, username)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if !libre {
		fallo(http.StatusConflict, "signup.username_taken")
		return
	}
	if !s.smsPermitido(r, phone) {
		fallo(http.StatusTooManyRequests, "signup.sms_limit")
		return
	}
	hash, err := domain.HashPassword(password)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	code, err := domain.NewSignupCode()
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	ahora := time.Now()
	alta := domain.SignupCode{
		ID: uuid.NewString(), Phone: phone, Username: username, Name: name, PasswordHash: hash,
		SentAt: ahora, ExpiresAt: ahora.Add(domain.SignupCodeTTL),
	}
	alta.CodeHash = domain.HashSignupCode(alta.ID, code)
	if err := s.store.CreateSignupCode(ctx, alta); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if err := s.sms.Send(ctx, phone, s.textoSMS(loc, code)); err != nil {
		s.log.ErrorContext(ctx, "enviar el código del alta por SMS falló", "error", err)
		_ = s.store.DeleteSignupCode(ctx, alta.ID)
		fallo(http.StatusBadGateway, "signup.sms_failed")
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry("", "", "auth.signup.code_sent", username, r))
	signupCodePage(w, loc, http.StatusOK, alta, html.EscapeString(returnTo), "", "")
}

// handleSignupCode es el segundo paso: confirma el código y crea la cuenta, o
// manda otro código si se pide (`resend`).
//
//nolint:gocyclo // confirmar el código, reenviarlo y crear la cuenta en un flujo lineal.
func (s *Server) handleSignupCode(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	if !s.signupDisponible() {
		httpFailHTML(w, r, http.StatusNotFound, i18n.T(loc, "signup.disabled"))
		return
	}
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, i18n.T(loc, "common.bad_origin"))
		return
	}
	if err := r.ParseForm(); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	id := r.Form.Get("id")
	returnTo := r.Form.Get("return_to")
	safeReturn := html.EscapeString(returnTo)
	ctx := r.Context()
	ahora := time.Now()
	// Un alta que ya no existe, caducó o agotó sus intentos: se empieza de nuevo.
	// Las tres se dicen igual.
	caducada := func() {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "signup.title"),
			alertHTML(i18n.T(loc, "signup.code_expired"))+
				`<p class="alt"><a class="btn-link" href="`+html.EscapeString(withReturnTo("/signup", returnTo))+`">`+
				html.EscapeString(i18n.T(loc, "signup.code_restart"))+`</a></p>`)))
	}
	if uuid.Validate(id) != nil {
		caducada()
		return
	}

	if r.Form.Get("resend") != "" {
		alta, err := s.store.GetSignupCode(ctx, id, ahora)
		if err != nil {
			caducada()
			return
		}
		if ahora.Sub(alta.SentAt) < domain.SignupCodeResendAfter {
			signupCodePage(w, loc, http.StatusTooManyRequests, alta, safeReturn, i18n.T(loc, "signup.code_wait"), "")
			return
		}
		if !s.smsPermitido(r, alta.Phone) {
			signupCodePage(w, loc, http.StatusTooManyRequests, alta, safeReturn, i18n.T(loc, "signup.sms_limit"), "")
			return
		}
		code, err := domain.NewSignupCode()
		if err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		if err := s.store.RenewSignupCode(ctx, id, domain.HashSignupCode(id, code), ahora); err != nil {
			// Otra pestaña lo acaba de reenviar, o caducó entre medias.
			signupCodePage(w, loc, http.StatusTooManyRequests, alta, safeReturn, i18n.T(loc, "signup.code_wait"), "")
			return
		}
		if err := s.sms.Send(ctx, alta.Phone, s.textoSMS(loc, code)); err != nil {
			s.log.ErrorContext(ctx, "reenviar el código del alta por SMS falló", "error", err)
			signupCodePage(w, loc, http.StatusBadGateway, alta, safeReturn, i18n.T(loc, "signup.sms_failed"), "")
			return
		}
		signupCodePage(w, loc, http.StatusOK, alta, safeReturn, "", i18n.T(loc, "signup.code_resent"))
		return
	}

	code := strings.TrimSpace(r.Form.Get("code"))
	alta, err := s.store.TrySignupCode(ctx, id, ahora)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			caducada()
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if subtle.ConstantTimeCompare([]byte(domain.HashSignupCode(id, code)), []byte(alta.CodeHash)) != 1 {
		if alta.Attempts >= domain.SignupCodeMaxAttempts {
			_ = s.store.DeleteSignupCode(ctx, id)
			caducada()
			return
		}
		signupCodePage(w, loc, http.StatusBadRequest, alta, safeReturn, i18n.T(loc, "signup.code_wrong"), "")
		return
	}
	// El código es bueno: el número es de quien lo ha tecleado. Ahora sí se le
	// puede decir que ese móvil ya tiene cuenta.
	_ = s.store.DeleteSignupCode(ctx, id)
	if _, err := s.store.GetUserByPhone(ctx, alta.Phone); err == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "signup.title"),
			alertHTML(i18n.T(loc, "signup.phone_has_account"))+
				`<p class="alt"><a class="btn-link" href="`+html.EscapeString(withReturnTo("/login/", returnTo))+`">`+
				html.EscapeString(i18n.T(loc, "signup.have_account"))+`</a></p>`)))
		return
	}

	email := alta.Username + "@" + suiteDomain(s.cfg.Issuer)
	user, org, err := s.crearCuentaPersonal(ctx, domain.User{
		Email: email, Username: alta.Username, Name: alta.Name, PasswordHash: alta.PasswordHash,
		Phone: alta.Phone,
		// La dirección la da Sytue en su propio dominio: no hay nada ajeno que
		// demostrar, así que nace verificada y la persona entra ya.
		EmailVerified: true, Status: domain.StatusActive,
	})
	if errors.Is(err, domain.ErrConflict) {
		// Alguien se quedó el @usuario (o el móvil) mientras llegaba el SMS.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "signup.title"),
			signupFormHTML(loc, safeReturn, html.EscapeString(alta.Username), html.EscapeString(alta.Name),
				html.EscapeString(alta.Phone), suiteDomain(s.cfg.Issuer), i18n.T(loc, "signup.username_taken"),
				s.cfg.SignupCaptchaEnabled))))
		return
	}
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(user.ID, org.ID, "auth.signup", alta.Username, r))
	_ = s.store.Audit(ctx, nil, auditEntry(user.ID, org.ID, "org.created", org.Slug, r))

	// Dentro, sin volver a teclear la contraseña. createSession aplica igual el
	// bloqueo de emergencia y el Acceso Condicional; si alguno lo impide, la
	// cuenta ya existe y se entra por el login de siempre.
	if err := s.createSession(w, r, user.ID, org.ID, []string{"pwd"}); err != nil {
		http.Redirect(w, r, withReturnTo("/login/", returnTo), http.StatusSeeOther)
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(user.ID, org.ID, "auth.login.success", "signup", r))
	signupListoPage(w, loc, user, returnTo, s.suiteHomeURL())
}

// signupListoPage es el final del alta: «Todo listo», lo que hace falta para
// volver —@usuario, correo y móvil, y que se entra con cualquiera— y un solo
// botón. Lleva a Sytue Hub; si el alta empezó en una app que pidió entrar, a esa
// app, que es a donde iba (la regla de siempre: solo rutas internas).
func signupListoPage(w http.ResponseWriter, loc string, u domain.User, returnTo, hub string) {
	esc := html.EscapeString
	destino, boton := hub, i18n.T(loc, "signup.done_hub")
	if rutaInternaSegura(returnTo) {
		destino, boton = returnTo, i18n.T(loc, "common.continue")
	}
	fila := func(dt, dd string) string { return `<div><dt>` + esc(dt) + `</dt><dd>` + esc(dd) + `</dd></div>` }
	body := pasosHTML(loc, 3) +
		`<p class="sub">` + esc(i18n.T(loc, "signup.done_body")) + `</p>` +
		`<dl class="resumen">` + fila(i18n.T(loc, "signup.username"), "@"+u.Username) +
		fila(i18n.T(loc, "common.email"), u.Email) + fila(i18n.T(loc, "signup.phone"), u.Phone) + `</dl>` +
		`<p class="fp hint">` + esc(i18n.T(loc, "signup.done_signin")) + `</p>` +
		`<a class="cta" href="` + esc(destino) + `">` + esc(boton) + `</a>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "signup.done_title"), body)))
}

// smsPermitido gasta un SMS del cupo de ese número y del de la IP. Si el
// limitador falla se deja pasar: el captcha y el limitador del formulario
// siguen delante, y un Postgres con hipo no debe cerrar el alta.
func (s *Server) smsPermitido(r *http.Request, phone string) bool {
	for _, l := range []struct {
		lim httpx.RateLimiter
		key string
	}{{s.smsPhoneLimiter, "sms-alta:tel:" + phone}, {s.smsIPLimiter, "sms-alta:ip:" + clientIP(r)}} {
		if l.lim == nil {
			continue
		}
		if res, err := l.lim.Allow(r.Context(), l.key); err == nil && !res.Allowed {
			return false
		}
	}
	return true
}

// textoSMS compone el SMS: el código y, en la última línea, el formato ligado al
// dominio que entienden Android (WebOTP) e iOS para rellenarlo solo EN ESTA WEB y
// en ninguna otra, que es lo que lo hace inútil en una página de phishing.
func (s *Server) textoSMS(loc, code string) string {
	host := strings.TrimPrefix(strings.TrimPrefix(s.cfg.Issuer, "https://"), "http://")
	host = strings.TrimRight(host, "/")
	return fmt.Sprintf(i18n.T(loc, "signup.sms_text"), code) + "\n\n@" + host + " #" + code
}

// handleSignupResend reenvía el enlace de verificación a una cuenta personal
// que aún no lo ha usado. Misma respuesta exista o no la cuenta.
func (s *Server) handleSignupResend(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	if !s.cfg.SignupEnabled {
		httpFailHTML(w, r, http.StatusNotFound, i18n.T(loc, "signup.disabled"))
		return
	}
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, i18n.T(loc, "common.bad_origin"))
		return
	}
	if err := r.ParseForm(); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	email := domain.NormalizeEmail(r.Form.Get("email"))
	ctx := r.Context()
	if u, err := s.store.GetUserByEmail(ctx, email); err == nil && !u.EmailVerified && u.Active() &&
		s.soloCuentasPersonales(ctx, u.ID) {
		if err := s.sendVerificationLink(ctx, u); err != nil {
			s.log.WarnContext(ctx, "reenviar verificación", "error", err)
		}
	}
	http.Redirect(w, r, "/signup?sent=1", http.StatusSeeOther)
}

// crearCuentaPersonal da de alta a la persona Y su organización personal en una
// sola transacción: o existen las dos o ninguna. La organización nace con su
// contrato gratuito (lo pone CreateOrg al ver `kind = personal`), la persona es
// su `owner` —es SU espacio— y el alta guiada queda marcada como hecha, porque
// para una persona sola no hay nada que preguntar.
//
// El buzón se reserva con su dirección EXACTA antes de la membresía, que al
// encontrarlo puesto no lo toca: sin eso, una dirección cogida caería al patrón
// nombre.apellidos y la persona tendría un correo que no eligió. Si está cogida,
// ErrConflict y no se crea nada.
func (s *Server) crearCuentaPersonal(ctx context.Context, u domain.User) (domain.User, domain.Organization, error) {
	var user domain.User
	var org domain.Organization
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		var err error
		if user, err = s.store.CreateUser(ctx, q, u); err != nil {
			return err
		}
		org, err = s.store.CreateOrg(ctx, q, domain.Organization{
			Slug: domain.PersonalSlug(u.Email), Name: u.Email,
			Status: domain.StatusActive, Kind: domain.OrgKindPersonal,
		})
		if err != nil {
			return err
		}
		if err := s.store.ReserveMailbox(ctx, q, org.ID, user.ID, u.Email); err != nil {
			return err
		}
		if err := s.store.UpsertMembership(ctx, q, domain.Membership{
			OrgID: org.ID, UserID: user.ID, Role: domain.RoleOwner,
		}); err != nil {
			return err
		}
		_, err = s.store.UpsertOnboarding(ctx, q, user.ID, string(domain.OnboardingIndividual), domain.StepDone, org.ID, true)
		return err
	})
	return user, org, err
}

// signupFormHTML pinta el primer paso del alta. Una pantalla, una idea por campo:
// cada uno dice para qué es con una frase debajo, y el botón es «Continuar»
// porque lo que sigue es el código del móvil, no la cuenta hecha.
// El @usuario va con el dominio pegado a la derecha porque eso ES lo que se está
// eligiendo: una dirección, no un apodo.
func signupFormHTML(loc, safeReturn, safeUsername, safeName, safePhone, dominio, errMsg string, captchaEnabled bool) string {
	esc := html.EscapeString
	b := pasosHTML(loc, 1) + `<p class="sub">` + esc(i18n.T(loc, "signup.subtitle")) + `</p>`
	if errMsg != "" {
		b += alertHTML(errMsg)
	}
	captchaBox := ""
	if captchaEnabled {
		captchaBox = captchaHTML(loc, "/signup/captcha")
	}
	return b + `<form method="post" action="/signup">` +
		`<input type="hidden" name="return_to" value="` + safeReturn + `">` +
		`<label for="name">` + esc(i18n.T(loc, "signup.name")) + `</label>` +
		`<input id="name" type="text" name="name" autocomplete="name" maxlength="120" value="` + safeName + `"` +
		` aria-describedby="name-hint" autofocus>` +
		`<p class="fp hint" id="name-hint">` + esc(i18n.T(loc, "signup.name_hint")) + `</p>` +
		`<label for="username">` + esc(i18n.T(loc, "signup.username")) + `</label>` +
		`<div class="handle"><input id="username" type="text" name="username" value="` + safeUsername + `"` +
		` autocomplete="username" spellcheck="false" autocapitalize="none" minlength="3" maxlength="64"` +
		` pattern="@?[A-Za-z0-9]([A-Za-z0-9.\-]*[A-Za-z0-9])?" aria-describedby="username-hint" required>` +
		`<span aria-hidden="true">@` + esc(dominio) + `</span></div>` +
		`<p class="fp hint" id="username-hint">` + esc(i18n.T(loc, "signup.username_hint")) + `</p>` +
		`<label for="phone">` + esc(i18n.T(loc, "signup.phone")) + `</label>` +
		`<input id="phone" type="tel" name="phone" value="` + safePhone + `" autocomplete="tel" inputmode="tel"` +
		` maxlength="24" aria-describedby="phone-hint" required>` +
		`<p class="fp hint" id="phone-hint">` + esc(i18n.T(loc, "signup.phone_hint")) + `</p>` +
		`<label for="password">` + esc(i18n.T(loc, "common.password")) + `</label>` +
		`<input id="password" type="password" name="password" autocomplete="new-password" minlength="12"` +
		` aria-describedby="password-hint" required>` +
		`<p class="fp hint" id="password-hint">` + esc(i18n.T(loc, "signup.password_hint")) + `</p>` +
		captchaBox +
		`<button type="submit">` + esc(i18n.T(loc, "common.continue")) + `</button></form>` +
		`<p class="fp hint">` + esc(i18n.T(loc, "signup.legal")) + `</p>` +
		`<p class="alt"><a class="btn-link" href="` + esc(withReturnTo("/login/", html.UnescapeString(safeReturn))) + `">` +
		esc(i18n.T(loc, "signup.have_account")) + `</a></p>`
}

// pasosHTML es la barra de progreso del alta: tres tramos —datos, código, todo
// listo— con el paso actual dicho en texto para quien no la ve.
func pasosHTML(loc string, paso int) string {
	b := `<div class="pasos" role="img" aria-label="` + html.EscapeString(fmt.Sprintf(i18n.T(loc, "signup.step"), paso)) + `">`
	for n := 1; n <= 3; n++ {
		if n <= paso {
			b += `<span class="on"></span>`
		} else {
			b += `<span></span>`
		}
	}
	return b + `</div>`
}

// captchaHTML es la caja del captcha ALTCHA con su script, para el alta y el
// login. La casilla NO es obligatoria: al pulsar el botón del formulario el
// script resuelve el reto y reenvía solo, así que nadie tiene que marcar nada;
// marcarla antes solo lo adelanta. Sin JS no hay reto resuelto y el servidor
// rechaza el envío, que es lo que tiene que pasar.
func captchaHTML(loc, retoURL string) string {
	esc := html.EscapeString
	return `<div class="altcha-box" id="altcha-box" data-challenge="` + esc(retoURL) + `"` +
		` data-verifying="` + esc(i18n.T(loc, "signup.captcha_verifying")) + `"` +
		` data-verified="` + esc(i18n.T(loc, "signup.captcha_verified")) + `"` +
		` data-failed="` + esc(i18n.T(loc, "signup.captcha_failed")) + `">` +
		`<label class="altcha-lbl" for="altcha-chk">` +
		`<input type="checkbox" id="altcha-chk">` +
		`<span class="altcha-spinner" aria-hidden="true"></span>` +
		`<span class="altcha-check" aria-hidden="true"><svg viewBox="0 0 16 16" fill="none"><circle cx="8" cy="8" r="7" fill="var(--ok)"/><path d="M5 8.2l2 2 4-4.4" stroke="#fff" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg></span>` +
		`<span id="altcha-status">` + esc(i18n.T(loc, "signup.captcha_label")) + `</span>` +
		`</label>` +
		`<input type="hidden" name="altcha" id="altcha-payload" value="">` +
		`</div><script>` + altchaScript + `</script>`
}

// signupCodePage pinta la pantalla del código: a qué móvil se mandó (sus tres
// últimas cifras), el campo, y cómo pedir otro o empezar de nuevo.
func signupCodePage(w http.ResponseWriter, loc string, status int, alta domain.SignupCode, safeReturn, errMsg, okMsg string) {
	esc := html.EscapeString
	b := pasosHTML(loc, 2) + `<p class="sub">` + esc(fmt.Sprintf(i18n.T(loc, "signup.code_sent"), domain.PhoneTail(alta.Phone))) + `</p>`
	if errMsg != "" {
		b += alertHTML(errMsg)
	}
	if okMsg != "" {
		b += `<p class="sub" role="status">` + esc(okMsg) + `</p>`
	}
	ocultos := `<input type="hidden" name="id" value="` + esc(alta.ID) + `">` +
		`<input type="hidden" name="return_to" value="` + safeReturn + `">`
	b += `<form method="post" action="/signup/code">` + ocultos +
		`<label for="code">` + esc(i18n.T(loc, "signup.code_label")) + `</label>` +
		`<input id="code" class="code" type="text" name="code" inputmode="numeric" autocomplete="one-time-code"` +
		` pattern="[0-9]{6}" maxlength="6" spellcheck="false" autofocus required>` +
		`<button type="submit">` + esc(i18n.T(loc, "signup.code_submit")) + `</button></form>` +
		`<form method="post" action="/signup/code">` + ocultos + `<input type="hidden" name="resend" value="1">` +
		`<button type="submit" class="pk-btn">` + esc(i18n.T(loc, "signup.code_resend")) + `</button></form>` +
		`<p class="alt"><a class="btn-link" href="` + esc(withReturnTo("/signup", html.UnescapeString(safeReturn))) + `">` +
		esc(i18n.T(loc, "signup.code_restart")) + `</a></p>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "signup.code_title"), b)))
}

func signupSentPage(loc string) string {
	esc := html.EscapeString
	return loginPage(loc, i18n.T(loc, "signup.sent_title"),
		`<p class="sub">`+esc(i18n.T(loc, "signup.sent_body"))+`</p>`+
			`<p class="alt"><a class="btn-link" href="/login/">`+esc(i18n.T(loc, "signup.have_account"))+`</a></p>`)
}
