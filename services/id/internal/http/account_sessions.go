// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sección "Sesiones activas" de /account (dispositivos con sesión SSO abierta):
// listado server-rendered con la sesión actual marcada y formularios POST para
// cerrar un dispositivo o todos los demás, más el espejo JSON en
// GET /api/v1/me/sessions. Los POST verifican Sec-Fetch-Site (ver sameOrigin):
// la cookie es SameSite=Lax con Domain compartido de la suite, así que un
// subdominio hermano comprometido podría postear CON cookie (same-site).
package http

import (
	"html"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
	"git.sytue.eu/sytue/one/services/id/internal/sesscrypto"
)

// sessionsCSS complementa accountCSS con el listado de sesiones. Contenido puro
// (sin <style>): su sha256 entra en la CSP (ver cspStyleHashes en middleware).
const sessionsCSS = `.sec{margin:28px 0 0;font-size:15px;letter-spacing:-.01em}` +
	`.sess{margin:4px 0 0;padding:0;list-style:none}` +
	`.s-row{padding:10px 0;border-bottom:1px solid var(--border)}.s-row:last-child{border-bottom:0}` +
	`.s-head{display:flex;align-items:center;gap:8px}` +
	`.s-head .pill,.s-head form{flex:none;margin-left:auto}` +
	`.s-dev{min-width:0;font-size:14px;font-weight:600;overflow-wrap:anywhere}` +
	`.s-meta{margin-top:2px;color:var(--muted);font-size:12px;overflow-wrap:anywhere}` +
	`.s-btn{margin:0;width:auto;padding:5px 10px;font:inherit;font-size:13px;font-weight:600;` +
	`color:var(--text);background:transparent;border:1px solid var(--border);border-radius:var(--radius-control);cursor:pointer;white-space:nowrap}` +
	`.s-btn:hover{background:var(--bg);border-color:var(--border-2)}.s-btn:active{background:var(--bg)}` +
	`.s-btn:focus-visible{outline:none;box-shadow:0 0 0 3px var(--ring)}` +
	// `white-space:normal` deshace el `nowrap` de `.s-btn` SOLO aquí. Ese `nowrap`
	// es correcto en el botón de fila —vive en un flex junto al nombre del
	// dispositivo y partirlo en dos líneas descoloca la fila—, pero este es de
	// ancho completo y su etiqueta lleva un contador: en alemán «Alle anderen
	// Sitzungen beenden (19)» pide 286px y solo tiene 240, así que empujaba
	// `section#sesiones`, `.acc-grid` y `main`, y sacaba el DOCUMENTO de /account
	// 6px fuera de la pantalla en los dos perfiles de 320 (iPhone SE, Galaxy S9+).
	// Medido en producción con test/e2e/repro-account-320.mjs, que es quien
	// señaló este botón: el detector reportaba el desbordamiento sin culpable
	// porque ninguna caja se sale —lo que no cabe es el ancho MÍNIMO del texto—.
	// Un botón de ancho completo puede ocupar dos líneas; recortar la etiqueta o
	// esconder el contador sería quitar información para que quepa.
	`.s-btn.wide{margin-top:12px;width:100%;padding:9px 12px;white-space:normal}` +
	`.s-empty{margin:8px 0 0;color:var(--muted);font-size:13px}`

// sessionsSectionHTML construye la sección "Sesiones activas" de /account: una
// fila por sesión vigente del usuario (dispositivo, IP, método, inicio y
// caducidad), la actual con distintivo y las demás con su botón de cierre.
func (s *Server) sessionsSectionHTML(r *http.Request, current domain.Session) string {
	esc := html.EscapeString
	loc := i18n.From(r)
	// El título y el CSS los aporta la tarjeta que la envuelve (ver accountPage).
	head := ""
	list, err := s.store.ListSessionsForUser(r.Context(), current.UserID, current.TokenHash)
	if err != nil {
		return head + `<p class="s-empty">` + esc(i18n.T(loc, "sessions.load_failed")) + `</p>`
	}
	if len(list) == 0 {
		// No debería ocurrir (la sesión actual siempre está), pero mejor un vacío
		// honesto que una lista rota.
		return head + `<p class="s-empty">` + esc(i18n.T(loc, "sessions.none")) + `</p>`
	}
	var b strings.Builder
	b.WriteString(head)
	b.WriteString(`<ul class="sess">`)
	lugares := s.ubicaciones(r, list)
	others := 0
	for _, ss := range list {
		dev := deviceES(loc, ss.UserAgent)
		b.WriteString(`<li class="s-row"><div class="s-head">`)
		b.WriteString(`<span class="s-dev" title="` + esc(ss.UserAgent) + `">` + esc(dev) + `</span>`)
		if ss.Current {
			b.WriteString(`<span class="pill ok">` + esc(i18n.T(loc, "sessions.this_one")) + `</span>`)
		} else {
			others++
			b.WriteString(`<form method="post" action="/account/sessions/revoke">` +
				`<input type="hidden" name="session_id" value="` + esc(ss.ID) + `">` +
				`<button type="submit" class="s-btn" aria-label="` + esc(i18n.T(loc, "sessions.close_aria")+" "+dev) + `">` + esc(i18n.T(loc, "sessions.close")) + `</button></form>`)
		}
		b.WriteString(`</div>`)
		meta := make([]string, 0, 4)
		if d := lugares[ss.ID]; d != "" {
			meta = append(meta, d)
		}
		meta = append(meta, amrPara(loc, ss.AMR),
			i18n.T(loc, "sessions.started")+" "+fecha(loc, ss.AuthTime.Local(), true),
			i18n.T(loc, "sessions.expires")+" "+fecha(loc, ss.ExpiresAt.Local(), true))
		b.WriteString(`<div class="s-meta">` + esc(strings.Join(meta, " · ")) + `</div></li>`)
	}
	b.WriteString(`</ul>`)
	if others > 0 {
		b.WriteString(`<p class="s-empty">` + esc(i18n.T(loc, "sessions.privacidad")) + `</p>`)
		b.WriteString(`<form method="post" action="/account/sessions/revoke-others">` +
			`<button type="submit" class="s-btn wide">` + esc(i18n.T(loc, "sessions.close_others")) + ` (` + strconv.Itoa(others) + `)</button></form>`)
	}
	return b.String()
}

// pastSessionsSectionHTML construye la tarjeta «Accesos recientes»: las
// sesiones ya terminadas, con su IP y su dispositivo.
//
// Existe porque cerrar sesión no debería borrar el rastro de que alguien entró.
// Quien sospecha de un acceso ajeno necesita mirar de dónde vino y cuándo, y en
// la lista de sesiones activas eso ya no está: al cerrarlas, desaparecen.
// Distingue además las que se cerraron de las que caducaron solas, porque una
// sesión cerrada a mano por alguien que no fuiste tú significa otra cosa.
func (s *Server) pastSessionsSectionHTML(r *http.Request, userID string) string {
	esc := html.EscapeString
	loc := i18n.From(r)
	list, err := s.store.ListPastSessionsForUser(r.Context(), userID)
	if err != nil {
		return `<p class="s-empty">` + esc(i18n.T(loc, "past.load_failed")) + `</p>`
	}
	if len(list) == 0 {
		return `<p class="hint">` + esc(i18n.T(loc, "past.empty")) + `</p>`
	}
	var b strings.Builder
	b.WriteString(`<p class="hint">` + esc(i18n.T(loc, "past.hint")) + `</p>`)
	b.WriteString(`<ul class="sess">`)
	lugares := s.ubicaciones(r, list)
	for _, ss := range list {
		fin := i18n.T(loc, "past.expired_on") + " " + fecha(loc, ss.ExpiresAt.Local(), true)
		if ss.RevokedAt != nil {
			fin = i18n.T(loc, "past.closed_on") + " " + fecha(loc, ss.RevokedAt.Local(), true)
		}
		meta := make([]string, 0, 4)
		if d := lugares[ss.ID]; d != "" {
			meta = append(meta, d)
		}
		meta = append(meta, amrPara(loc, ss.AMR), i18n.T(loc, "sessions.started")+" "+fecha(loc, ss.AuthTime.Local(), true), fin)
		b.WriteString(`<li class="s-row"><div class="s-head">` +
			`<span class="s-dev" title="` + esc(ss.UserAgent) + `">` + esc(deviceES(loc, ss.UserAgent)) + `</span>` +
			`</div><div class="s-meta">` + esc(strings.Join(meta, " · ")) + `</div></li>`)
	}
	b.WriteString(`</ul>`)
	return b.String()
}

// ubicaciones descifra la ubicación de las sesiones que se PUEDEN descifrar,
// que es solo aquella cuyo token presenta el navegador ahora mismo.
//
// No es una limitación a la que dar la vuelta: cada sesión cifra con su propio
// token y en la base de datos solo hay hashes. Para poder enseñar de dónde
// vinieron TODAS haría falta una llave capaz de abrirlas todas y, exista donde
// exista, esa llave la tendríamos nosotros. Se prefiere enseñar menos y que la
// promesa sea cierta.
func (s *Server) ubicaciones(r *http.Request, list []domain.Session) map[string]string {
	out := make(map[string]string, 1)
	ck, err := s.requestCookie(r, sessionCookieName)
	if err != nil || ck.Value == "" {
		return out
	}
	for _, ss := range list {
		if !ss.Current || len(ss.LugarCifrado) == 0 {
			continue
		}
		if lugar, derr := sesscrypto.Descifrar(ck.Value, ss.LugarCifrado); derr == nil && lugar != "" {
			out[ss.ID] = lugar
		}
	}
	return out
}

// sameOrigin defiende los POST de /account contra CSRF. La cookie SameSite=Lax
// corta el POST cross-SITE; esta es la capa que falta contra un subdominio
// hermano (same-site: *.apps.sytue.eu sirve código de clientes bajo el mismo
// site que id.sytue.eu, así que su POST SÍ lleva la cookie).
//
// Manda Sec-Fetch-Site, no Origin, y la razón está MEDIDA con Chrome: este
// servicio responde «Referrer-Policy: no-referrer» (middleware.go), y con esa
// política la especificación de Fetch ordena serializar el Origin de una
// navegación POST como «null». Es decir, el POST LEGÍTIMO de nuestro propio
// formulario llega con Origin: null — idéntico byte a byte al de una página
// atacante que declare su propio no-referrer. Origin no puede distinguirlos:
//
//	form propio (mismo origen):  Origin=null  Sec-Fetch-Site=same-origin
//	página atacante (otro sitio): Origin=null  Sec-Fetch-Site=cross-site
//
// Sec-Fetch-Site lo calcula el navegador, es cabecera prohibida (ningún script
// la puede poner) y no depende de la referrer-policy. Un cliente que no la
// manda (Safari < 16.4, curl) cae al contraste por Origin de siempre: no es un
// hueco nuevo, y un cliente que no es un navegador no lleva la cookie ajena.
func (s *Server) sameOrigin(r *http.Request) bool {
	// "none" = navegación iniciada por el usuario (barra, marcador): ninguna
	// página la origina, así que no es CSRF.
	switch fs := r.Header.Get("Sec-Fetch-Site"); fs {
	case "same-origin", "none":
		return true
	case "same-site", "cross-site":
		slog.WarnContext(r.Context(), "account POST: fetch-site rechazado", "sec_fetch_site", fs)
		return false
	}
	o := r.Header.Get("Origin")
	if o == "" || o == "null" || originAllowed(o, s.cfg.RPOrigins) {
		return true
	}
	slog.WarnContext(r.Context(), "account POST: origin rechazado", "origin", o)
	return false
}

// uuidRe valida el session_id del formulario antes de tocar la BD: un valor no
// uuid haría fallar el cast en Postgres (500) en vez de un rechazo limpio.
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// handleSessionRevoke cierra UNA sesión del usuario (botón "Cerrar"). Los ids
// desconocidos o ajenos son un no-op silencioso (el guard vive en el store);
// en ambos casos PRG a /account con la lista ya refrescada.
func (s *Server) handleSessionRevoke(w http.ResponseWriter, r *http.Request) {
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
	if id := r.Form.Get("session_id"); uuidRe.MatchString(id) {
		var revoked bool
		ctx := r.Context()
		err := s.store.InTx(ctx, func(q pg.Querier) error {
			var e error
			revoked, e = s.store.RevokeSessionByID(ctx, q, sess.UserID, id)
			return e
		})
		if err != nil {
			httpFailHTML(w, r, http.StatusInternalServerError, i18n.T(i18n.From(r), "sessions.close_failed"))
			return
		}
		if revoked {
			_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "session.revoked", id, r))
		}
	}
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

// handleSessionRevokeOthers cierra todas las sesiones del usuario salvo la
// actual ("Cerrar las demás sesiones"), con cascada sobre sus refresh tokens.
func (s *Server) handleSessionRevokeOthers(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login/?return_to=/account", http.StatusFound)
		return
	}
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, i18n.T(i18n.From(r), "common.bad_origin"))
		return
	}
	var n int
	ctx := r.Context()
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		n, e = s.store.RevokeOtherSessions(ctx, q, sess.UserID, sess.ID)
		return e
	})
	if err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, i18n.T(i18n.From(r), "sessions.close_others_failed"))
		return
	}
	if n > 0 {
		_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "session.revoked_others", strconv.Itoa(n), r))
	}
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

// handleMeSessions expone GET /api/v1/me/sessions (JSON): las sesiones activas
// del usuario del access token. Si la petición llega además con la cookie de
// sesión del IdP (navegador same-site), marca cuál es la actual.
func (s *Server) handleMeSessions(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	currentHash := ""
	if ck, err := s.requestCookie(r, sessionCookieName); err == nil && ck.Value != "" {
		currentHash = domain.HashToken(ck.Value)
	}
	list, err := s.store.ListSessionsForUser(r.Context(), c.Subject, currentHash)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, ss := range list {
		out = append(out, map[string]any{
			"id":         ss.ID,
			"ip":         ss.IP,
			"user_agent": ss.UserAgent,
			"device":     deviceES(i18n.From(r), ss.UserAgent),
			"amr":        ss.AMR,
			"auth_time":  ss.AuthTime,
			"expires_at": ss.ExpiresAt,
			"current":    ss.Current,
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"sessions": out})
}

// deviceES resume un User-Agent en "Navegador · SO" legible; si no reconoce el
// patrón cae al literal truncado. Heurística mínima por tokens, no un parser
// de UA (el UA completo queda en el title y en el JSON).
func deviceES(loc, ua string) string {
	browser := ""
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/"), strings.Contains(ua, "Opera"):
		browser = "Opera"
	case strings.Contains(ua, "Firefox/"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/"), strings.Contains(ua, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	os := ""
	switch {
	case strings.Contains(ua, "Windows"):
		os = "Windows"
	case strings.Contains(ua, "Android"):
		os = "Android"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"):
		os = "iOS"
	case strings.Contains(ua, "Mac OS X"), strings.Contains(ua, "Macintosh"):
		os = "macOS"
	case strings.Contains(ua, "Linux"):
		os = "Linux"
	}
	switch {
	case browser != "" && os != "":
		return browser + " · " + os
	case browser != "":
		return browser
	case os != "":
		return os
	case ua == "":
		return i18n.T(loc, "sessions.unknown_device")
	}
	if len(ua) > 48 {
		return ua[:48] + "…"
	}
	return ua
}
