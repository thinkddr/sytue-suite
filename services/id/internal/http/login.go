// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
	"git.sytue.eu/sytue/one/platform/pkg/altcha"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
	"git.sytue.eu/sytue/one/services/id/internal/store"
	"git.sytue.eu/sytue/one/services/id/internal/totp"
)

// handleLoginForm sirve un formulario de login mínimo del Authorization Server.
// En la suite real el front es Astro (apps/frontend/apps/portal); este HTML es
// el fallback funcional del IdP para el flujo interactivo.
func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	returnTo := r.URL.Query().Get("return_to")
	stepUp := r.URL.Query().Get("step_up") == "1"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// returnTo se escapa para evitar XSS reflejado en el value del input.
	safeReturn := html.EscapeString(returnTo)
	if stepUp {
		// Paso 2: formulario del segundo factor (código TOTP). Postea a /login/mfa,
		// que consume el token de continuación (cookie httpOnly) y valida el binding.
		// La salida "Volver a iniciar sesión" evita el callejón sin salida si el
		// usuario no tiene el móvil a mano (conserva return_to).
		back := withReturnTo("/login/", returnTo)
		// La PASSKEY también vale como segundo factor, y aquí faltaba.
		//
		// `mfa_enabled` se pone a true tanto al confirmar un TOTP como al
		// registrar una passkey, pero este paso solo ofrecía el campo del código:
		// quien tenía passkey y NO tenía TOTP quedaba atrapado —se le pedía un
		// código que no existe, fallaba siempre, y la única salida era «volver a
		// iniciar sesión», que devuelve al mismo sitio—. El bloque es el mismo
		// del paso 1 y su script se muestra solo si el navegador soporta
		// WebAuthn, así que a quien use TOTP no le cambia nada.
		_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "mfa.title"),
			`<p class="sub">`+html.EscapeString(i18n.T(loc, "mfa.subtitle"))+`</p>`+
				`<form method="post" action="/login/mfa">`+
				`<input type="hidden" name="return_to" value="`+safeReturn+`">`+
				`<label for="code">`+html.EscapeString(i18n.T(loc, "mfa.code"))+`</label>`+
				`<input id="code" class="code" type="text" name="code" autocomplete="one-time-code" maxlength="11" spellcheck="false" autocapitalize="characters" autofocus required>`+
				`<button type="submit">`+html.EscapeString(i18n.T(loc, "mfa.verify"))+`</button></form>`+
				passkeyBlockHTML(loc)+
				`<p class="alt"><a class="btn-link" href="`+html.EscapeString(back)+`">`+html.EscapeString(i18n.T(loc, "login.back"))+`</a></p>`+
				`<script>`+loginScript+`</script>`)))
		return
	}
	_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "login.title"), loginFormHTML(loc, safeReturn, "", "", s.captchaEnLogin()))))
}

// emailFieldAttrs son los atributos del campo de identidad de TODOS los
// formularios del IdP (acceso, recuperación de contraseña y entrada por
// organización). Existe para que los tres den al navegador y a los gestores de
// contraseñas exactamente las mismas señales.
//
// `type=text` y no `type=email` a propósito, que es lo que hacen GitHub y GitLab
// en su campo de acceso. Con los dos —`type=email` mirando al perfil de
// direcciones y `autocomplete=username` mirando a la credencial guardada— algunos
// gestores rellenan DOS veces, y el segundo relleno no limpia lo escrito: el campo
// acababa con «correo@dominio correo@dominio». Y entonces la validación nativa de
// `type=email` rechazaba ese valor y BLOQUEABA el envío, así que «Continuar» no
// hacía nada y no había forma de entrar. El teclado de correo en el móvil lo sigue
// dando `inputmode`, y quien valida de verdad la dirección es el servidor.
const emailFieldAttrs = ` type="text" inputmode="email" name="email"` +
	` autocomplete="username" spellcheck="false" autocapitalize="none" required`

// emailFieldScript deja el campo de identidad con UNA sola dirección.
//
// Quitar `type=email` evitó que la validación nativa bloqueara el envío, pero no
// evita el doble relleno: hay gestores que escriben la credencial y, al ver que
// el valor parece un correo, la escriben otra vez SIN limpiar. El campo acaba
// con «correo@dominio correo@dominio» y, aunque el servidor ya se defiende
// —`domain.NormalizeEmail` se queda con el primer campo—, quien lo ve en
// pantalla no tiene forma de saber que va a funcionar. Este arreglo es de
// PRESENTACIÓN; la defensa del servidor se queda donde está.
//
// Cuándo actúa, y por qué distinto en cada evento:
//
//   - `input`: SOLO si los trozos son idénticos, que es la firma inequívoca del
//     doble relleno. Normalizar cualquier espacio aquí le borraría letras a quien
//     esté tecleando.
//   - `change` y `submit`: cualquier espacio sobra, porque una dirección no
//     lleva ninguno y para entonces ya ha terminado de escribir. Es también la
//     red para los gestores que rellenan sin emitir `input`. MENOS en un móvil
//     («+34 600 123 456»), que lleva espacios y se envía tal cual: lo normaliza
//     el servidor (domain.NormalizeLogin), y cortarlo aquí dejaba «+34».
//
// Se queda con el PRIMER trozo, el mismo criterio que el servidor: si los dos
// difieren, lo que el usuario vio primero es lo que se envía.
const emailFieldScript = `(function(){` +
	`var e=document.getElementById('email');if(!e){return;}` +
	`function p(){return e.value.split(/\s+/).filter(Boolean);}` +
	`function set(v){if(v&&v!==e.value){e.value=v;try{e.setSelectionRange(v.length,v.length);}catch(x){}}}` +
	`e.addEventListener('input',function(){var t=p();if(t.length>1&&t.every(function(x){return x===t[0];})){set(t[0]);}});` +
	`function n(){var t=p();if(t.length>1&&!/^[+0][\d\s().-]+$/.test(e.value.trim())){set(t[0]);}}` +
	`e.addEventListener('change',n);if(e.form){e.form.addEventListener('submit',n);}n();})();`

// captchaEnLogin dice si el formulario de acceso lleva el captcha ALTCHA.
func (s *Server) captchaEnLogin() bool { return s.cfg.LoginCaptchaEnabled && s.captcha != nil }

// testBypassOK dice si la petición trae un bypass de pruebas válido: cabecera
// X-Sytue-Test-Auth con «timestamp|email» firmado (HMAC-SHA256) con la clave
// SYTUE_ID_LOGIN_BYPASS_KEY del despliegue, dentro de una ventana de ±5 min. La clave
// solo vive en el despliegue y en los secretos del CI — nunca en un navegador —,
// así que quien no la tenga resuelve el PoW como todo el mundo. Sin clave
// configurada, el bypass NO existe.
func (s *Server) testBypassOK(r *http.Request, email string) bool {
	if len(s.loginBypassKey) == 0 {
		return false
	}
	return altcha.VerifyTestBypass(s.loginBypassKey, r.Header.Get("X-Sytue-Test-Auth"), email, time.Now())
}

// loginFormHTML construye el formulario de acceso (email + contraseña), con un
// aviso de error opcional encima. Lo comparten el GET /login/ y los reintentos
// tras un fallo (loginFail): el estado de error conserva el propio formulario
// como acción de reintento. safeReturn y safeEmail llegan ya escapados.
func loginFormHTML(loc, safeReturn, safeEmail, errMsg string, conCaptcha bool) string {
	esc := html.EscapeString
	b := `<p class="sub">` + esc(i18n.T(loc, "login.subtitle")) + `</p>`
	if errMsg != "" {
		b += alertHTML(errMsg)
	}
	// Con el email conservado del intento anterior, el foco pasa a la contraseña.
	emailFocus, pwdFocus := ` autofocus`, ``
	if safeEmail != "" {
		emailFocus, pwdFocus = ``, ` autofocus`
	}
	captchaBox := ""
	if conCaptcha {
		captchaBox = captchaHTML(loc, "/login/captcha")
	}
	return b + `<form method="post" action="/login/">` +
		`<input type="hidden" name="return_to" value="` + safeReturn + `">` +
		`<label for="email">` + esc(i18n.T(loc, "login.identifier")) + `</label>` +
		`<input id="email"` + emailFieldAttrs + ` value="` + safeEmail + `"` + emailFocus + `>` +
		`<label for="password">` + esc(i18n.T(loc, "common.password")) + `</label>` +
		`<input id="password" type="password" name="password" autocomplete="current-password"` + pwdFocus + ` required>` +
		// El enlace de olvido conserva el viaje igual que el de SSO justo debajo:
		// sin return_to, quien restablece la contraseña aterriza en la cuenta de
		// Sytue ID en vez de en la app que pidió entrar, y tiene que rehacer el
		// OAuth entero. La validación anti-open-redirect sigue donde debe estar
		// (redirectAfterLogin): aquí solo se transporta.
		`<p class="fp"><a class="btn-link" href="` +
		esc(withReturnTo("/forgot-password", html.UnescapeString(safeReturn))) +
		`">` + esc(i18n.T(loc, "login.forgot")) + `</a></p>` + captchaBox +
		`<button type="submit">` + esc(i18n.T(loc, "common.continue")) + `</button></form>` +
		passkeyBlockHTML(loc) + ssoLinkHTML(loc, safeReturn) + signupLinkHTML(loc, safeReturn) +
		`<script>` + emailFieldScript + `</script><script>` + loginScript + `</script>`
}

// withReturnTo cuelga ?return_to= de una ruta INTERNA del IdP cuando hay viaje
// que conservar. returnTo llega sin escapar y el resultado se escapa al pintarlo.
// Es solo transporte: quien decide si un return_to es aceptable es
// redirectAfterLogin (única ruta relativa, anti-open-redirect).
func withReturnTo(path, returnTo string) string {
	if returnTo == "" {
		return path
	}
	return path + "?" + url.Values{"return_to": {returnTo}}.Encode()
}

// ssoLinkHTML es el enlace "Entrar con tu organización" (login federado). Mejora
// progresiva y sobria: un simple <a> a /login/sso (conserva return_to), sin JS.
func ssoLinkHTML(loc, safeReturn string) string {
	esc := html.EscapeString
	return `<p class="alt"><a class="btn-link" href="` +
		esc(withReturnTo("/login/sso", html.UnescapeString(safeReturn))) +
		`">` + esc(i18n.T(loc, "login.sso")) + `</a></p>`
}

// passkeyBlockHTML es el botón de acceso por passkey (oculto por defecto; el script
// lo revela solo si el navegador soporta WebAuthn: mejora progresiva, sin JS el
// login por contraseña sigue funcionando) y el hueco de error accesible.
func passkeyBlockHTML(loc string) string {
	return `<div class="or">` + html.EscapeString(i18n.T(loc, "common.or")) + `</div>` +
		`<button type="button" id="passkey-btn" class="pk-btn" hidden data-err="` +
		html.EscapeString(i18n.T(loc, "login.passkey_failed")) + `">` +
		`<svg viewBox="0 0 16 16" fill="none" aria-hidden="true">` +
		`<circle cx="5.75" cy="5.75" r="3" stroke="currentColor" stroke-width="1.5"/>` +
		`<path d="M8 8l5 5m-1.25-1.25L13 10.5M13 13l1.25-1.25" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>` +
		`</svg>` + html.EscapeString(i18n.T(loc, "login.passkey")) + `</button>` +
		`<p class="pk-err" id="passkey-err" role="alert" hidden></p>`
}

// loginScript maneja la ceremonia de login por passkey (navigator.credentials.get)
// sin dependencias: detecta soporte, revela el botón, negocia begin/finish y navega
// al destino que devuelve el servidor. Su sha256 entra en la CSP (script-src), sin
// 'unsafe-inline'; si se edita, la CSP se recalcula sola (ver cspScriptHashes).
const loginScript = `(function(){` +
	`var b=document.getElementById('passkey-btn');` +
	`if(!b||!window.PublicKeyCredential||!navigator.credentials||!navigator.credentials.get){return;}` +
	`b.hidden=false;var err=document.getElementById('passkey-err');` +
	`function fail(m){if(err){err.textContent=m;err.hidden=false;}b.disabled=false;}` +
	`function d(s){s=s.replace(/-/g,'+').replace(/_/g,'/');while(s.length%4){s+='=';}var x=atob(s),a=new Uint8Array(x.length),i=0;for(;i<x.length;i++){a[i]=x.charCodeAt(i);}return a.buffer;}` +
	`function e(buf){var a=new Uint8Array(buf),s='',i=0;for(;i<a.length;i++){s+=String.fromCharCode(a[i]);}return btoa(s).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,'');}` +
	`b.addEventListener('click',function(){` +
	`if(err){err.hidden=true;}b.disabled=true;` +
	`var em=(document.getElementById('email')||{}).value||'';` +
	`var rt=(document.querySelector('input[name=return_to]')||{}).value||'';` +
	`fetch('/login/webauthn/begin',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({email:em})}).then(function(r){if(!r.ok){throw 0;}return r.json();}).then(function(o){` +
	`return navigator.credentials.get({publicKey:{challenge:d(o.challenge),rpId:o.rpId,timeout:o.timeout,userVerification:o.userVerification,allowCredentials:(o.allowCredentials||[]).map(function(c){return{type:'public-key',id:d(c.id)};})}});` +
	`}).then(function(c){var a=c.response;return fetch('/login/webauthn/finish',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:c.id,rawId:e(c.rawId),type:c.type,response:{authenticatorData:e(a.authenticatorData),clientDataJSON:e(a.clientDataJSON),signature:e(a.signature),userHandle:a.userHandle?e(a.userHandle):''},return_to:rt})});})` +
	`.then(function(r){if(!r.ok){throw 0;}return r.json();}).then(function(o){window.location=o.redirect||'/';}).catch(function(){fail(b.dataset.err);});` +
	`});})();`

// alertHTML es el aviso de error accesible de las páginas del IdP (icono + texto).
func alertHTML(msg string) string {
	return `<div class="alert" role="alert">` +
		`<svg viewBox="0 0 16 16" fill="none" aria-hidden="true">` +
		`<circle cx="8" cy="8" r="6.25" stroke="currentColor" stroke-width="1.5"/>` +
		`<path d="M8 4.75v4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>` +
		`<circle cx="8" cy="11.1" r=".9" fill="currentColor"/></svg>` +
		`<span>` + html.EscapeString(msg) + `</span></div>`
}

// coloresCSS es la copia vendorizada de la hoja canónica, compartida con la web
// y las apps nativas. La mantiene scripts/sync-branding.sh; no se edita aquí.
//
//go:embed colores.css
var coloresCSS string

//go:embed appearance.css
var appearanceCSS string

//go:embed theme_init.js
var themeInitJS string

// El IdP consume el mismo radio de botón que la web y las apps nativas.
var radioControl = tokens.MedidaEnForma("radius-control", false)

// loginCSS conserva la estructura propia del IdP y toma los colores de la suite.
// El bloque completo entra en la CSP por su hash, igual que las otras hojas del IdP.
var loginCSS = coloresCSS + appearanceCSS + fmt.Sprintf(":root{--radius-control:%g%s}", radioControl.Valor, radioControl.Unidad) + `/* Figtree, SIL OFL 1.1: /fonts/OFL.txt */@font-face{font-family:Figtree;src:url(/fonts/Figtree-Regular.ttf) format("truetype");font-weight:400;font-display:swap}@font-face{font-family:Figtree;src:url(/fonts/Figtree-Medium.ttf) format("truetype");font-weight:500;font-display:swap}@font-face{font-family:Figtree;src:url(/fonts/Figtree-SemiBold.ttf) format("truetype");font-weight:600;font-display:swap}@font-face{font-family:Figtree;src:url(/fonts/Figtree-Bold.ttf) format("truetype");font-weight:700;font-display:swap}` + `:root{--brand:var(--sytue-primary);--brand-h:var(--sytue-primary-hover);--brand-a:var(--sytue-primary-active);--link:var(--sytue-primary-text);--ok:var(--sytue-success-text);--ok-bg:var(--sytue-success-bg);--danger:var(--sytue-danger-text);--danger-bg:var(--sytue-danger-bg);--bg:var(--sytue-bg);--surface:var(--sytue-surface);--text:var(--sytue-text);--muted:var(--sytue-text-muted);--border:var(--sytue-border);--border-2:var(--sytue-border-strong);--ring:var(--sytue-focus-ring);--r:16px}` +
	`*{box-sizing:border-box}::selection{background:color-mix(in srgb,var(--ring) 26%,transparent)}` +
	// `dvh` y no `vh`: en Chrome Android `100vh` mide MÁS que la pantalla visible
	// (es la altura con la barra de direcciones retraída), así que `place-items:
	// center` centraba la tarjeta en una caja más alta que el hueco real y el
	// login abría desplazado. Regla 4 de apps/frontend/scripts/lint-design.sh: se
	// aplicó a los cuatro shells del frontend y aquí no llegaba porque el gate
	// solo recorría apps/frontend — ya no, ahora también mira services/**.go.
	`body{margin:0;min-height:100dvh;display:grid;place-items:center;padding:24px;background:var(--bg);color:var(--text);font:15px/1.5 "Figtree", "Inter",system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;-webkit-font-smoothing:antialiased}` +
	`.card{width:min(100%,384px);background:var(--surface);border:1px solid var(--border);border-radius:var(--r);box-shadow:0 1px 2px rgba(var(--sytue-ink-rgb),.04),0 10px 30px rgba(var(--sytue-ink-rgb),.07);padding:32px 28px}` +
	`.brand{display:flex;align-items:center;gap:8px;font-weight:700;font-size:17px;letter-spacing:-.02em}.brand .logo{display:inline-flex;width:24px;height:24px}.brand .logo svg{width:24px;height:24px;border-radius:6px}` +
	`.brand .tag{font-weight:600;font-size:11px;line-height:1;letter-spacing:.06em;color:var(--muted);border:1px solid var(--border);border-radius:999px;padding:3px 7px}` +
	`.aviso{margin-top:28px;padding-top:14px;border-top:1px solid var(--border);color:var(--muted);font-size:12px;line-height:1.5}` +
	`h1{margin:24px 0 0;font-size:20px;line-height:1.3;letter-spacing:-.02em}.sub{margin:4px 0 0;color:var(--muted);font-size:14px}` +
	`.alert{display:flex;gap:8px;align-items:flex-start;margin:16px 0 0;padding:10px 12px;border-radius:10px;background:var(--danger-bg);color:var(--danger);font-size:14px;line-height:1.45}.alert svg{flex:none;width:16px;height:16px;margin-top:2px}` +
	`label{display:block;margin:16px 0 4px;font-size:13px;font-weight:500;color:var(--muted)}` +
	// La casilla conserva su apariencia y foco nativos, sin el cromo de los campos.
	// :where mantiene la especificidad de input para respetar .code y .handle input.
	`input:where(:not([type=checkbox]):not([type=radio])){width:100%;padding:10px 12px;font:inherit;color:var(--text);background:var(--surface);border:1px solid var(--border-2);border-radius:10px;outline:none;transition:border-color .15s,box-shadow .15s}` +
	`input:where(:not([type=checkbox]):not([type=radio])):hover{border-color:var(--border-2)}input:where(:not([type=checkbox]):not([type=radio])):focus-visible{border-color:var(--brand);box-shadow:0 0 0 2px var(--ring)}` +
	`input:-webkit-autofill{-webkit-box-shadow:0 0 0 1000px var(--surface) inset;-webkit-text-fill-color:var(--text);caret-color:var(--text)}` +
	`input.code{font-size:22px;font-weight:600;letter-spacing:.35em;text-align:center;font-variant-numeric:tabular-nums}` +
	// EN TÁCTIL LOS CAMPOS NO BAJAN DE 16px, o iOS amplía la página al enfocarlos
	// y no la devuelve al salir: se queda ampliada, con los márgenes descuadrados.
	// Aquí pasa porque `body` va a 15px y los `input` heredan con `font:inherit`.
	//
	// ES LA PRIMERA PANTALLA QUE VE CUALQUIERA EN UN MÓVIL, así que el zoom salta
	// antes incluso de entrar. La suite lo arregla en su hoja compartida, pero
	// Sytue ID se sirve como servicio Go con este CSS autocontenido y no la usa:
	// la regla tiene que estar también aquí. Medido contra producción el
	// 2026-09-07 con la sonda de `test/e2e`: 15px en `email` y en `password`.
	//
	// `max(16px,1rem)` y no `16px`: a quien SUBE el tamaño base del navegador no
	// se le encoge el campo por debajo de su preferencia. 16 es un suelo.
	//
	// `input.code` se queda en sus 22px: ya está por encima y es deliberado.
	`@media (pointer:coarse){input:not([type=checkbox]):not([type=radio]),select,textarea{font-size:max(16px,1rem)}}` +
	`button{margin-top:24px;width:100%;padding:12px 16px;font:inherit;font-weight:600;color:var(--sytue-on-primary);background:var(--brand);border:0;border-radius:var(--radius-control);cursor:pointer;transition:background .15s}` +
	`button:hover{background:var(--brand-h)}button:active{background:var(--brand-a)}button:focus-visible{outline:none;box-shadow:0 0 0 1px var(--surface),0 0 0 3px var(--ring)}button:disabled{opacity:.55;cursor:not-allowed}` +
	`.btn-link{display:inline-block;color:var(--link);text-decoration:none;font-weight:600;font-size:14px;border-radius:var(--radius-control)}.btn-link:hover{text-decoration:underline}.btn-link:focus-visible{outline:none;box-shadow:0 0 0 2px var(--ring)}` +
	`.alt{margin:20px 0 0;text-align:center;font-size:14px}.muted{color:var(--muted);font-weight:400}` +
	`.fp{margin:10px 0 0;text-align:right;font-size:13px}` +
	// El @usuario del alta con su dominio pegado: lo que se elige es una dirección.
	`.handle{display:flex}.handle input{min-width:0;border-radius:10px 0 0 10px}` +
	`.handle span{display:flex;align-items:center;padding:0 12px;border:1px solid var(--border);border-left:0;border-radius:0 10px 10px 0;background:var(--bg);color:var(--muted);font-size:14px;white-space:nowrap}` +
	`.fp.hint{text-align:left;color:var(--muted)}` +
	// El recorrido del alta: barra de tres pasos, resumen de la cuenta y el botón
	// final, que es un enlace (lleva a Sytue Hub o a la app que pidió entrar).
	`.pasos{display:flex;gap:6px;margin:12px 0 14px}.pasos span{flex:1;height:4px;border-radius:2px;background:var(--border)}.pasos span.on{background:var(--brand)}` +
	`.resumen{margin:20px 0 0;padding:2px 14px;border:1px solid var(--border);border-radius:10px}` +
	`.resumen div{display:flex;justify-content:space-between;gap:12px;padding:10px 0;border-bottom:1px solid var(--border);font-size:14px}.resumen div:last-child{border-bottom:0}` +
	`.resumen dt{color:var(--muted)}.resumen dd{margin:0;font-weight:600;text-align:right;overflow-wrap:anywhere}` +
	`.cta{display:block;margin-top:24px;padding:12px 16px;text-align:center;font-weight:600;color:var(--sytue-on-primary);background:var(--brand);border-radius:var(--radius-control);text-decoration:none;transition:background .15s}` +
	`.cta:hover{background:var(--brand-h)}.cta:active{background:var(--brand-a)}.cta:focus-visible{outline:none;box-shadow:0 0 0 1px var(--surface),0 0 0 3px var(--ring)}` +
	`.or{display:flex;align-items:center;gap:12px;margin:20px 0 0;color:var(--muted);font-size:12px}.or::before,.or::after{content:"";flex:1;height:1px;background:var(--border)}` +
	`.pk-btn{margin-top:16px;display:flex;align-items:center;justify-content:center;gap:8px;width:100%;padding:11px 16px;font:inherit;font-weight:600;color:var(--text);background:transparent;border:1px solid var(--border-2);border-radius:var(--radius-control);cursor:pointer;transition:background .15s,border-color .15s}` +
	`.pk-btn:hover{background:var(--bg);border-color:var(--brand)}.pk-btn:focus-visible{outline:none;box-shadow:0 0 0 2px var(--ring)}.pk-btn svg{width:16px;height:16px;flex:none}` +
	`.pk-err{margin:12px 0 0;padding:10px 12px;border-radius:10px;background:var(--danger-bg);color:var(--danger);font-size:14px;line-height:1.45}` +
	`.altcha-box{margin-top:16px;padding:12px;background:var(--bg);border:1px solid var(--border);border-radius:10px;user-select:none}` +
	`.altcha-lbl{display:flex;align-items:center;gap:10px;cursor:pointer;font-size:13px;font-weight:500;color:var(--text);margin:0}` +
	// El texto puede ocupar dos líneas en móvil; la casilla sigue siendo cuadrada.
	`.altcha-box input[type=checkbox]{flex:none;width:18px;height:18px;cursor:pointer;accent-color:var(--brand);margin:0;padding:0}` +
	`.altcha-spinner{display:none;width:16px;height:16px;border:2px solid var(--border-2);border-top-color:var(--brand);border-radius:50%;animation:altcha-spin .6s linear infinite}` +
	`.altcha-check{display:none;width:16px;height:16px;color:var(--ok);flex:none}` +
	`.altcha-check svg{width:16px;height:16px;display:block}` +
	`.altcha-box.verifying .altcha-spinner{display:inline-block}` +
	`.altcha-box.verifying input[type=checkbox]{display:none}` +
	`.altcha-box.verified .altcha-check{display:inline-block}` +
	`.altcha-box.verified input[type=checkbox]{display:none}` +
	`@keyframes altcha-spin{to{transform:rotate(360deg)}}`

// loginPage envuelve el contenido en el shell HTML con la marca Sytue.
//
// El idioma llega explícito y no se saca aquí de la petición: así el `lang` del
// documento y el texto que envuelve no pueden acabar en idiomas distintos, que
// es lo que rompe a un lector de pantalla —anuncia en un idioma y lee en otro—.
func loginPage(loc, title, body string) string {
	return `<!doctype html><html data-theme="light" lang="` + html.EscapeString(loc) + `"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<link rel="icon" href="/favicon.svg" type="image/svg+xml">` +
		`<link rel="apple-touch-icon" href="/apple-touch-icon.png">` +
		`<meta name="color-scheme" content="light">` +
		`<meta name="robots" content="noindex"><title>` + html.EscapeString(title) + ` · Sytue ID</title>` +
		`<style>` + loginCSS + `</style><script>` + themeInitJS + `</script></head><body><main class="card sytue-glass-surface">` +
		`<div class="brand"><span class="logo"><svg viewBox="0 0 40 40" aria-hidden="true">` +
		`<defs><clipPath id="m"><rect width="40" height="40" rx="9"/></clipPath></defs>` +
		`<g clip-path="url(#m)"><rect width="40" height="40" fill="#0A0A0A"/>` +
		`<polygon points="40,14 40,40 14,40" fill="#b98f4f"/></g></svg></span>Sytue<span class="tag">ID</span></div>` +
		`<h1>` + html.EscapeString(title) + `</h1>` + body + `<footer class="aviso">` + html.EscapeString(i18n.T(loc, "common.privacy_notice")) + `</footer>` +
		`</main></body></html>`
}

// handleLoginSubmit autentica con email + contraseña (Argon2id) y crea sesión SSO.
//
//nolint:gocyclo // login: credencial, Acceso Condicional, MFA y sesión en un flujo lineal auditado paso a paso.
func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	// El único formulario del IdP que NO comprobaba el origen, y el que más falta
	// le hace: sin esto una página cualquiera podía enviar SUS credenciales desde
	// el navegador de otra persona y dejarla dentro de la cuenta del atacante sin
	// que se notara —el login-CSRF clásico—. A partir de ahí, todo lo que esa
	// persona escriba, suba o envíe en la suite cae en la cuenta ajena. Ni
	// SameSite ni el bucket por IP tapan esto: la petición no necesita ninguna
	// cookie previa. Misma verja que /account y /forgot-password.
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, i18n.T(i18n.From(r), "common.bad_origin"))
		return
	}
	if err := r.ParseForm(); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	email := domain.NormalizeLogin(r.Form.Get("email"))
	password := r.Form.Get("password")
	returnTo := r.Form.Get("return_to")
	// El captcha, antes de mirar la cuenta: un envío sin reto resuelto no llega a
	// gastar un Argon2id ni el cupo de intentos de nadie.
	if s.captchaEnLogin() && !s.testBypassOK(r, email) {
		if ok, err := s.captcha.Verify(r.Context(), r.Form.Get("altcha")); !ok || err != nil {
			s.loginFail(w, r, "signup.captcha_failed")
			return
		}
	}

	user, ok := s.credencialValida(w, r, email, password)
	if !ok {
		return
	}

	orgID := s.orgTrasLogin(r, user)
	amr := []string{"pwd"}
	forceMFA, sigue := s.accesoCondicional(w, r, user, orgID, returnTo)
	if !sigue {
		return
	}

	// Contraseña TEMPORAL (la fijó un operador de plataforma, mig 0113): sirve
	// para llegar hasta aquí, no para abrir sesión. Va al formulario de
	// restablecimiento de siempre con un token recién emitido, y la persona entra
	// después con la suya —y con su segundo factor, si lo tiene—. Saber la
	// temporal es lo único que da este token, y es justo lo que se le entregó.
	if user.PasswordMustChange {
		link, err := s.enlaceDeReseteo(r.Context(), user.ID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		_ = s.store.Audit(r.Context(), nil, auditEntry(user.ID, orgID, "auth.password.change_required", "", r))
		http.Redirect(w, r, link, http.StatusSeeOther)
		return
	}

	// Si el usuario tiene MFA habilitado (o el Acceso Condicional lo fuerza), exigir
	// el segundo factor antes de crear sesión. Se emite un token de continuación de un
	// solo uso, corta vida y ligado a IP/UA (F8); el paso 2 (/login/mfa) lo consume y
	// solo entonces se abre la sesión con amr elevado. La contraseña ya verificada NO
	// abre sesión por sí sola.
	if user.MFAEnabled || forceMFA {
		if err := s.beginMFAStepUp(w, r, user.ID, orgID, returnTo); err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		_ = s.store.Audit(r.Context(), nil, auditEntry(user.ID, orgID, "auth.mfa.challenge", "", r))
		stepURL := "/login/?" + url.Values{"step_up": {"1"}, "return_to": {returnTo}}.Encode()
		http.Redirect(w, r, stepURL, http.StatusFound)
		return
	}

	if err := s.createSession(w, r, user.ID, orgID, amr); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(user.ID, orgID, "auth.login.success", "", r))
	redirectAfterLogin(w, r, returnTo)
}

// credencialValida comprueba email y contraseña y que la cuenta pueda entrar. Si
// no, ya ha contestado (fallo genérico, correo sin verificar o error interno) y
// devuelve false. Va aparte de handleLoginSubmit solo para que ese flujo se lea
// de un vistazo; el orden de las comprobaciones es el mismo.
//
// El lockout suave por cuenta se aplica en credencialFallida, DESPUÉS de mirar
// la contraseña. Comprobarlo antes —que es lo que se hacía— dejaba fuera de la
// plataforma a quien teclea la contraseña correcta: el bucket se gasta en cada
// POST, así que cualquiera que conozca un correo podía mantenerlo agotado desde
// una sola IP y la persona no volvía a entrar. Consumirlo solo cuando la
// credencial falla conserva el tope de intentos (5/min contra un mismo email,
// vengan de donde vengan) sin que un tercero pueda cerrarle la puerta a nadie.
func (s *Server) credencialValida(w http.ResponseWriter, r *http.Request, email, password string) (domain.User, bool) {
	user, err := s.userByLogin(r.Context(), email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Mensaje genérico (no enumerar usuarios).
			s.credencialFallida(w, r, email)
			return domain.User{}, false
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return domain.User{}, false
	}
	if !user.Active() || user.PasswordHash == "" {
		s.credencialFallida(w, r, email)
		return domain.User{}, false
	}
	okPwd, err := domain.VerifyPassword(password, user.PasswordHash)
	if err != nil || !okPwd {
		_ = s.store.Audit(r.Context(), nil, auditEntry(user.ID, "", "auth.login.failed", "", r))
		s.credencialFallida(w, r, email)
		return domain.User{}, false
	}
	// Quien se registró por su cuenta entra cuando ha demostrado que el correo es
	// suyo, no antes. Es la única defensa real del alta autoservicio: sin ella,
	// cualquiera abre cuentas —con su espacio— a nombre de direcciones ajenas. Se
	// distingue por las membresías, no por un flag: una cuenta personal sin
	// verificar es exactamente «solo organizaciones personales y sin verificar».
	// Las cuentas invitadas o federadas ya llegan verificadas y no pasan por aquí.
	if !user.EmailVerified && s.soloCuentasPersonales(r.Context(), user.ID) {
		_ = s.store.Audit(r.Context(), nil, auditEntry(user.ID, "", "auth.login.unverified", "", r))
		s.loginFail(w, r, msgVerifyFirst)
		return domain.User{}, false
	}
	return user, true
}

// orgTrasLogin resuelve la organización activa de quien acaba de entrar (primera
// membresía; multi-org real lo gestionaría un selector de org tras login).
//
// Sin ninguna organización, pero con una invitación esperando a este correo: se
// materializa aquí. El enlace de la invitación viaja por correo y el correo se pierde
// —filtros, un MSA caído, una dirección mal escrita—, así que la persona acaba
// entrando por su cuenta y la suite la recibía con el asistente de alta preguntándole
// «¿para quién es Sytue?», cuando alguien ya la había dado de alta en su empresa.
// Quien fue invitado tiene que quedarse DENTRO de esa organización, no configurando
// una nueva.
//
// Solo con el correo verificado: aceptar por el enlace prueba control del buzón y esta
// vía tiene que probar lo mismo, o sería una forma de entrar en una organización
// ajena registrándose con la dirección adecuada.
func (s *Server) orgTrasLogin(r *http.Request, user domain.User) string {
	orgID := ""
	if mems, err := s.store.ListMembershipsForUser(r.Context(), user.ID); err == nil && len(mems) > 0 {
		orgID = mems[0].OrgID
	}
	if orgID == "" && user.EmailVerified {
		if invs, err := s.store.ClaimPendingInvitations(r.Context(), nil, user.ID, user.Email, time.Now()); err == nil && len(invs) > 0 {
			orgID = invs[0].OrgID
			_ = s.store.Audit(r.Context(), nil,
				auditEntry(user.ID, orgID, "org.member.invitation_claimed", user.Email, r))
		}
	}
	return orgID
}

// accesoCondicional (paridad Entra): TRAS verificar la credencial y ANTES de emitir
// la sesión, evalúa las políticas de la org para (usuario, grupos, IP de origen, app
// pendiente en return_to). block ⇒ deniega el login; require_mfa ⇒ fuerza el segundo
// factor aunque el usuario no lo pidiera. Un usuario excluido (break-glass de admins)
// sale como allow. Sin políticas ⇒ allow (anti-lockout). Devuelve si hay que forzar el
// segundo factor y si el login sigue (false = ya se contestó).
func (s *Server) accesoCondicional(w http.ResponseWriter, r *http.Request, user domain.User, orgID, returnTo string) (forceMFA, sigue bool) {
	caDecision := s.resolveAccessDecision(r.Context(), r, user.ID, orgID, clientIDFromReturnTo(returnTo))
	switch caDecision.Action {
	case domain.AccessBlock:
		s.auditAccessDecision(r.Context(), r, user.ID, orgID, "auth.ca.blocked", caDecision)
		s.loginFail(w, r, msgAccessBlocked)
		return false, false
	case domain.AccessRequireMFA:
		if !user.MFAEnabled {
			// No se puede enrolar un segundo factor antes de abrir sesión: se deniega
			// (un admin excluido de la política entra igual: break-glass).
			s.auditAccessDecision(r.Context(), r, user.ID, orgID, "auth.ca.mfa_unenrolled", caDecision)
			s.loginFail(w, r, msgMFAEnrollRequired)
			return false, false
		}
		return true, true
	case domain.AccessAllow:
		// allow: el login sigue con la política MFA propia del usuario.
	}
	return false, true
}

// userByLogin resuelve lo tecleado en el campo de identidad (ya pasado por
// domain.NormalizeLogin) a la cuenta. Se entra con cuatro cosas y la contraseña
// es la de la cuenta con cualquiera de ellas (ADR 0041):
//
//   - el móvil verificado en el alta («+34600123456»);
//   - el @usuario («@ana» o «ana»);
//   - una dirección de Sytue de la persona —su buzón o su correo de acceso—;
//   - su correo de recuperación confirmado.
//
// Las direcciones van antes que el correo de recuperación: una dirección de un
// buzón es de quien lo tiene, y si alguien la hubiera puesto como recuperación
// en otra cuenta, no puede quitarle la entrada a su dueño.
func (s *Server) userByLogin(ctx context.Context, ident string) (domain.User, error) {
	if strings.HasPrefix(ident, "+") {
		return s.store.GetUserByPhone(ctx, ident)
	}
	if username, ok := domain.UsernameDeLogin(ident); ok {
		return s.store.GetUserByUsername(ctx, username)
	}
	u, err := s.store.GetUserByAddress(ctx, ident)
	if errors.Is(err, domain.ErrNotFound) {
		return s.store.GetUserByRecoveryEmail(ctx, ident)
	}
	return u, err
}

// beginMFAStepUp emite y persiste un token de continuación MFA (paso 1→2): un
// solo uso, corta vida (cfg.MFAContinuationTTL) y ligado a IP/UA. Lo entrega en
// una cookie httpOnly acotada a /login. Solo se persiste el hash del token.
func (s *Server) beginMFAStepUp(w http.ResponseWriter, r *http.Request, userID, orgID, returnTo string) error {
	token, err := domain.GenerateToken(32)
	if err != nil {
		return err
	}
	exp := time.Now().Add(s.cfg.MFAContinuationTTL)
	cont := domain.MFAContinuation{
		TokenHash: domain.HashToken(token),
		UserID:    userID,
		OrgID:     orgID,
		ReturnTo:  returnTo,
		IP:        clientIP(r),
		UserAgent: r.UserAgent(),
		ExpiresAt: exp,
	}
	if err := s.store.InsertMFAContinuation(r.Context(), nil, cont); err != nil {
		return err
	}
	s.setMFACookie(w, token, exp)
	return nil
}

// handleMFAVerify verifica el segundo factor (TOTP) y eleva la sesión (amr mfa).
// Consume el token de continuación (un solo uso), valida el binding IP/UA y la
// expiración, y solo entonces crea la sesión SSO. El flujo WebAuthn de login
// (passkeys) se sirve en /api/v1/webauthn; al superarlo se eleva amr a webauthn.
func (s *Server) handleMFAVerify(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, i18n.T(i18n.From(r), "common.bad_origin"))
		return
	}
	if err := r.ParseForm(); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	code := strings.TrimSpace(r.Form.Get("code"))

	c, err := s.requestCookie(r, mfaCookieName)
	if err != nil || c.Value == "" {
		s.loginFail(w, r, msgMFAFailed)
		return
	}

	// Consumir la continuación de forma atómica (anti-replay) y validar binding.
	cont, err := s.store.ConsumeMFAContinuation(r.Context(), nil, domain.HashToken(c.Value), time.Now())
	s.clearMFACookie(w)
	if err != nil {
		s.loginFail(w, r, msgMFAFailed)
		return
	}
	if !cont.Valid(time.Now(), r.UserAgent()) {
		// Otro navegador u otro sistema, o caducado: posible robo del token.
		_ = s.store.Audit(r.Context(), nil, auditEntry(cont.UserID, cont.OrgID, "auth.mfa.binding_mismatch", "", r))
		s.loginFail(w, r, msgMFAFailed)
		return
	}
	// La IP ya NO decide —cambia sola entre wifi y datos, y con IPv4/IPv6 en la
	// misma red—, pero que cambie a mitad del segundo factor es un dato que
	// quiere tener quien investigue después. Se anota y se sigue.
	if ahora := clientIP(r); ahora != cont.IP {
		_ = s.store.Audit(r.Context(), nil,
			auditEntry(cont.UserID, cont.OrgID, "auth.mfa.ip_changed", cont.IP+" → "+ahora, r))
	}

	// Verificar el segundo factor: un código TOTP vigente O, como respaldo, un
	// código de recuperación de un solo uso (que se consume atómicamente).
	secret, serr := s.store.GetTOTPSecret(r.Context(), cont.UserID)
	if serr != nil || !totp.Validate(secret, code, time.Now()) {
		norm := domain.NormalizeRecoveryCode(code)
		if norm == "" || !s.consumeRecoveryCode(r, cont.UserID, norm) {
			_ = s.store.Audit(r.Context(), nil, auditEntry(cont.UserID, cont.OrgID, "auth.mfa.failed", "", r))
			s.loginFail(w, r, msgMFAFailed)
			return
		}
		_ = s.store.Audit(r.Context(), nil, auditEntry(cont.UserID, cont.OrgID, "auth.mfa.recovery_used", "", r))
	}

	// Segundo factor superado: crear sesión con amr elevado (pwd + mfa).
	if err := s.createSession(w, r, cont.UserID, cont.OrgID, []string{"pwd", "mfa"}); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(cont.UserID, cont.OrgID, "auth.mfa.success", "", r))
	redirectAfterLogin(w, r, cont.ReturnTo)
}

// Mensajes de fallo de autenticación (genéricos: no enumeran usuarios).
const (
	msgBadCredentials = "login.bad_credentials" //nolint:gosec // G101: clave de traducción, no una credencial (el nombre contiene "Cred").
	msgTooManyTries   = "login.too_many_tries"
	msgMFAFailed      = "login.mfa_failed"
	msgVerifyFirst    = "login.verify_first"
)

// credencialFallida contabiliza un intento fallido contra el bucket por cuenta y
// re-renderiza el formulario. Es el ÚNICO sitio donde se consume ese bucket, y
// esa es toda la defensa: mientras solo lo gasten los fallos, agotarlo frena la
// fuerza bruta distribuida (muchas IP contra un mismo email, que el límite por IP
// no ve) pero no puede dejar fuera a quien sí sabe su contraseña.
//
// El mensaje cambia a «demasiados intentos» cuando el bucket queda en deuda: es
// la misma información que ya daba antes y no distingue una cuenta que existe de
// una que no, porque la clave es el email tecleado, exista o no.
func (s *Server) credencialFallida(w http.ResponseWriter, r *http.Request, email string) {
	msg := msgBadCredentials
	if email != "" && s.acctLimiter != nil {
		if res, err := s.acctLimiter.Allow(r.Context(), "login:"+email); err == nil && !res.Allowed {
			_ = s.store.Audit(r.Context(), nil, auditEntry("", "", "auth.login.locked", "", r))
			msg = msgTooManyTries
		}
	}
	s.loginFail(w, r, msg)
}

// loginFail re-renderiza el formulario de acceso con un aviso de error: el
// reintento es la propia página (sin callejones sin salida). Conserva return_to
// y el email tecleado (r.Form ya está parseado en todos los llamantes). 401 para
// que los clientes no navegador distingan el fallo.
func (s *Server) loginFail(w http.ResponseWriter, r *http.Request, msg string) {
	loc := i18n.From(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	safeReturn := html.EscapeString(r.Form.Get("return_to"))
	safeEmail := html.EscapeString(domain.NormalizeLogin(r.Form.Get("email")))
	// `msg` es una CLAVE del catálogo. T devuelve la clave tal cual si no la
	// encuentra, así que un mensaje literal de algún llamante antiguo se sigue
	// mostrando en vez de desaparecer.
	_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "login.title"),
		loginFormHTML(loc, safeReturn, safeEmail, i18n.T(loc, msg), s.captchaEnLogin()))))
}

func redirectAfterLogin(w http.ResponseWriter, r *http.Request, returnTo string) {
	// Solo se permiten rutas relativas internas como return_to (anti open-redirect).
	if rutaInternaSegura(returnTo) {
		http.Redirect(w, r, returnTo, http.StatusFound)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// rutaInternaSegura es la ÚNICA regla de qué return_to puede usarse como destino:
// una ruta relativa de este mismo servicio.
//
// La versión anterior era `empieza por "/" y no por "//"`, y con eso `/\evil.com`
// pasaba. Para el navegador no es una ruta: en la posición de la autoridad, la
// especificación de URL manda tratar `\` como `/` en los esquemas http/https, así
// que `/\evil.com` resuelto contra el IdP es `https://evil.com`. Un open redirect
// desde el dominio donde se teclean las contraseñas es la mitad del trabajo de
// cualquier phishing, y además convierte al IdP en el rebote «de confianza» al
// final de un flujo OAuth.
//
// Se rechaza también cualquier carácter de control (CR, LF, TAB, NUL): los
// navegadores los ELIMINAN de la URL antes de interpretarla, así que `/\tevil.com`
// o un `\r\n` partido esconden el mismo destino de esta comprobación, y el salto
// de línea además abre la puerta a inyectar cabeceras en Location.
func rutaInternaSegura(p string) bool {
	if p == "" || p[0] != '/' {
		return false
	}
	if len(p) > 1 && (p[1] == '/' || p[1] == '\\') {
		return false
	}
	return strings.IndexFunc(p, func(r rune) bool { return r < 0x20 || r == 0x7f }) < 0
}

// auditEntry construye una entrada de auditoría con la IP del cliente.
func auditEntry(actorID, orgID, action, target string, r *http.Request) store.AuditEntry {
	return store.AuditEntry{
		ActorID: actorID,
		OrgID:   orgID,
		Action:  action,
		Target:  target,
		IP:      clientIP(r),
		Detail:  map[string]any{"at": time.Now().UTC().Format(time.RFC3339)},
	}
}
