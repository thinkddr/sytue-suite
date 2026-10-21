// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Página de cuenta de Sytue ID (self-service mínimo): identidad, organizaciones
// y estado de la sesión SSO, con el mismo shell visual que /login. La enlaza el
// menú de cuenta de la Sytue Shell ("Mi cuenta"). Server-rendered, sin JS.
package http

import (
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
)

// handleHome: la raíz del IdP no es un 404 — con sesión va a la cuenta y sin
// ella al login (con retorno a la cuenta).
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentSession(r); ok {
		http.Redirect(w, r, "/account", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/login/?return_to=/account", http.StatusFound)
}

// destinoAppCuenta devuelve a dónde mandar a quien pide `/account` cuando la app
// «Mi cuenta» está desplegada, o "" cuando no lo está.
//
// NUNCA AL HOST DE UNA ORGANIZACIÓN (decisión del dueño, 2026-09-24): la cuenta
// es de la persona, no de una organización, y su dirección es global. En
// producción esta redirección no llega a verse: en `id.<dominio>` el Ingress da
// `/account` a la app antes de que llegue aquí. Queda para cuando llegue igual
// —un despliegue que monte la app sin ese reparto—, y va a `account.<dominio>` y
// no a `id.<dominio>`, que sería volver a este mismo handler en bucle.
func (s *Server) destinoAppCuenta() string {
	if s.cfg.AccountAppPath == "" {
		return ""
	}
	dominio := suiteDomain(s.cfg.Issuer)
	if dominio == "" {
		return ""
	}
	return "https://account." + dominio + "/" + strings.Trim(s.cfg.AccountAppPath, "/")
}

// handleAccount muestra la página "Mi cuenta" al usuario con sesión SSO, o lo
// manda a la app cuando está desplegada.
//
// LA PÁGINA SIGUE AQUÍ y no se ha borrado: es la que responde en cualquier
// instalación que no despliegue la app, y sus formularios son el único camino de
// quien tenga esta pestaña abierta cuando se active la redirección. Retirarla es
// otro cambio, y con la app ya sirviéndose.
// handleAccountAvatarGet sirve el avatar del usuario de la SESIÓN (cookie),
// para los <img> de la barra superior y las fichas de las demás apps: un <img>
// no manda cabeceras de autorización. La identidad la da la cookie de sesión
// compartida de la suite (Domain=.sytue.eu), no la URL.
func (s *Server) handleAccountAvatarGet(w http.ResponseWriter, r *http.Request) {
	// El claim OIDC `picture` apunta aquí desde Sytue ID, pero quien lo pinta
	// normalmente es otra superficie de la suite (corporate., mail., ...).
	// La política global `same-origin` convierte esa imagen válida en una imagen
	// rota entre subdominios. `same-site` mantiene el recurso dentro del sitio
	// Sytue (misma schemeful site), sin abrirlo a orígenes de terceros.
	w.Header().Set("Cross-Origin-Resource-Policy", "same-site")
	sess, ok := s.currentSession(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	s.sirveAvatar(w, r, sess.UserID)
}

func (s *Server) handleAccount(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	sess, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login/?return_to=/account", http.StatusFound)
		return
	}
	// 302 y no 301: el destino depende de la organización ACTIVA de la sesión, y
	// un permanente se queda cacheado en el navegador de quien cambie de
	// organización o se dé de alta en una después.
	if destino := s.destinoAppCuenta(); destino != "" {
		// La query viaja: `?mfa=required` es lo que hace que la app diga POR QUÉ
		// se llega aquí desde un step-up (ver desviarAStepUp). El host y la ruta
		// son fijos, así que no abre ninguna redirección.
		if r.URL.RawQuery != "" {
			destino += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, destino, http.StatusFound)
		return
	}
	user, err := s.store.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, i18n.T(loc, "account.load_failed"))
		return
	}
	mems, _ := s.store.ListMembershipsForUser(r.Context(), user.ID)

	esc := html.EscapeString
	row := func(dt, dd string) string {
		return `<div class="row"><dt>` + esc(dt) + `</dt><dd>` + dd + `</dd></div>`
	}
	pill := func(ok bool, yes, no string) string {
		if ok {
			return `<span class="pill ok">` + esc(yes) + `</span>`
		}
		return `<span class="pill off">` + esc(no) + `</span>`
	}

	// Se resuelve el nombre de cada organización en vez de enseñar su UUID: el
	// identificador es correcto y no significa nada para quien lo lee. Si la
	// consulta falla se cae al id, que al menos permite dar el dato en soporte.
	orgs := make([]string, 0, len(mems))
	for _, m := range mems {
		etiqueta := `<span class="mono">` + esc(m.OrgID) + `</span>`
		if org, oerr := s.store.GetOrgByID(r.Context(), m.OrgID); oerr == nil && org.Name != "" {
			etiqueta = esc(org.Name)
		}
		orgs = append(orgs, etiqueta+` <span class="muted">· `+esc(rolePara(loc, string(m.Role)))+`</span>`)
	}
	if len(orgs) == 0 {
		orgs = append(orgs, `<span class="muted">`+esc(i18n.T(loc, "account.no_orgs"))+`</span>`)
	}

	// Llega de un step-up imposible: la app pedía segundo factor y esta cuenta no
	// tiene ninguno enrolado. Sin decirlo, la vuelta a /account parecía un rebote
	// arbitrario y no la instrucción que es.
	aviso := ""
	if r.URL.Query().Get("mfa") == "required" && !user.MFAEnabled {
		aviso = alertHTML(i18n.T(loc, "account.mfa_required"))
	}

	// Los datos que el usuario no edita —correo, organizaciones, alta— van juntos
	// en su propia tarjeta en vez de mezclarse con los campos editables: si todo
	// se ve igual, no hay forma de saber qué se puede cambiar sin probarlo.
	// El @usuario va arriba: es lo que esta persona le da a alguien de otra
	// organización para que la encuentre (ADR 0040).
	usuario := ""
	if user.Username != "" {
		usuario = row(i18n.T(loc, "signup.username"), esc("@"+user.Username))
	}
	cuenta := `<dl class="rows">` + usuario +
		row(i18n.T(loc, "common.email"), `<span title="`+esc(user.Email)+`">`+esc(user.Email)+`</span> `+
			pill(user.EmailVerified, i18n.T(loc, "account.verified"), i18n.T(loc, "account.unverified"))) +
		row(i18n.T(loc, "account.orgs"), strings.Join(orgs, "<br>")) +
		row(i18n.T(loc, "account.current_session"), esc(amrPara(loc, sess.AMR))+
			` <span class="muted">· `+esc(i18n.T(loc, "account.expires_on"))+` `+esc(fecha(loc, sess.ExpiresAt.Local(), true))+`</span>`) +
		row(i18n.T(loc, "account.member_since"), esc(fecha(loc, user.CreatedAt.Local(), false))) +
		`</dl>`

	body := aviso +
		`<div class="acc-grid">` +
		cardHTML("perfil", iconUser, i18n.T(loc, "profile.title"), s.profileSectionHTML(r, user)) +
		cardHTML("cuenta", iconGrid, i18n.T(loc, "account.card"), cuenta) +
		cardHTML("seguridad", iconShield, i18n.T(loc, "security.title"), s.securitySectionHTML(r, user)) +
		cardHTML("claves", iconKey, i18n.T(loc, "passkeys.title"), s.passkeysSectionHTML(r, user.ID)) +
		// Va junto a las passkeys porque es lo mismo desde la silla de quien mira:
		// «con qué entro además de con mi contraseña». Y es la ÚNICA credencial que
		// sirve para un cliente de correo, así que esconderla no era una opción.
		cardHTML("app-passwords", iconKey, i18n.T(loc, "app_passwords.title"), s.appPasswordsSectionHTML(r, user.ID)) +
		cardHTML("sesiones", iconDevice, i18n.T(loc, "sessions.title"), s.sessionsSectionHTML(r, sess)) +
		cardHTML("accesos", iconClock, i18n.T(loc, "past.title"), s.pastSessionsSectionHTML(r, user.ID)) +
		`</div>` +
		`<div class="acc-foot">` +
		`<a class="btn" href="` + esc(s.suiteHomeURL()) + `">` + esc(i18n.T(loc, "account.back_to_suite")) + `</a>` +
		`<a class="btn ghost" href="/account/apple.mobileconfig">` + esc(i18n.T(loc, "account.apple_profile")) + `</a>` +
		`<a class="btn ghost" href="/oauth2/logout">` + esc(i18n.T(loc, "account.sign_out")) + `</a>` +
		`</div>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(accountPage(loc, i18n.T(loc, "account.title"), user.Name, user.Email, body)))
}

// accountCSS complementa loginCSS con las filas de la ficha de cuenta.
// Contenido puro (sin <style>): su sha256 entra en la CSP (ver middleware).
const accountCSS = `.rows{margin:16px 0 0}` +
	`.row{display:flex;gap:16px;align-items:baseline;padding:12px 0;border-bottom:1px solid var(--border)}` +
	`.row:last-child{border-bottom:0}` +
	`.row dt{flex:none;color:var(--muted);font-size:13px}` +
	`.row dd{flex:1;min-width:0;margin:0;font-size:14px;font-weight:500;text-align:right;overflow-wrap:anywhere}` +
	`.mono{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:12px}` +
	`.pill{display:inline-block;vertical-align:middle;padding:2px 8px;border-radius:999px;font-size:12px;font-weight:600;line-height:1.4;white-space:nowrap}` +
	`.pill.ok{background:var(--ok-bg);color:var(--ok)}` +
	`.pill.off{background:var(--bg);color:var(--muted);border:1px solid var(--border)}` +
	`.actions{display:flex;flex-wrap:wrap;gap:8px;margin-top:24px}` +
	`.btn{flex:1 1 auto;text-align:center;padding:10px 12px;border-radius:var(--radius-control);white-space:nowrap;` +
	`background:var(--brand);color:#fff;text-decoration:none;font-weight:600;font-size:14px;transition:background .15s,border-color .15s}` +
	`.btn:hover{background:var(--brand-h)}.btn:active{background:var(--brand-a)}` +
	`.btn:focus-visible{outline:none;box-shadow:0 0 0 2px var(--ring),0 0 0 3px var(--ring-2)}` +
	`.btn.ghost{background:transparent;color:var(--text);border:1px solid var(--border)}` +
	`.btn.ghost:hover{background:var(--bg);border-color:var(--border-2)}.btn.ghost:active{background:var(--bg)}`

// roleES traduce el rol de membresía para mostrarlo (fallback: el literal).
// rolePara traduce el rol de membresía (fallback: el literal que venga).
func rolePara(loc, role string) string {
	switch role {
	case "owner", "admin", "member", "billing":
		return i18n.T(loc, "role."+role)
	}
	return role
}

// amrPara describe los métodos de autenticación de la sesión.
func amrPara(loc string, amr []string) string {
	out := make([]string, 0, len(amr))
	for _, m := range amr {
		switch m {
		case "pwd":
			m = i18n.T(loc, "amr.pwd")
		case "mfa", "otp":
			m = i18n.T(loc, "amr.mfa")
		case "webauthn":
			m = i18n.T(loc, "amr.webauthn")
		}
		out = append(out, m)
	}
	return strings.Join(out, " + ")
}

// fecha formatea una fecha en el idioma dado (ver i18n.Fecha).
func fecha(loc string, t time.Time, conHora bool) string {
	return i18n.Fecha(loc, t, conHora)
}

// suiteHomeURL deriva el inicio de la suite del issuer (id.<apex> -> one.<apex>).
func (s *Server) suiteHomeURL() string {
	u, err := url.Parse(s.cfg.Issuer)
	if err != nil || u.Host == "" {
		return "/"
	}
	if rest, ok := strings.CutPrefix(u.Host, "id."); ok {
		u.Host = "one." + rest
		u.Path = "/"
		return u.String()
	}
	return s.cfg.Issuer
}

// httpFailHTML responde un error simple con el shell de marca (sin stacktrace)
// y una acción de reintento: la raíz redirige a cuenta o login según la sesión.
func httpFailHTML(w http.ResponseWriter, r *http.Request, status int, msg string) {
	loc := i18n.From(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "common.error_title"),
		alertHTML(msg)+
			`<p class="alt"><a class="btn-link" href="/">`+html.EscapeString(i18n.T(loc, "common.retry"))+`</a></p>`)))
}
