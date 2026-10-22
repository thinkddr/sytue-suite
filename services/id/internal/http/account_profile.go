// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sección «Perfil» de /account: los datos que el propio usuario mantiene
// —nombre, apellidos, puesto, departamento, ubicación e idioma— con su
// formulario POST. Sin JS, como el resto del self-service del IdP.
//
// Puesto, departamento y ubicación no son decorativos: son los atributos que
// alimentan las reglas de los grupos dinámicos (migración 0021), así que lo que
// alguien escriba aquí puede cambiar a qué grupos pertenece. Por eso el
// formulario lo dice en vez de dejarlo como un dato suelto.
package http

import (
	"html"
	"net/http"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
)

// profileCSS complementa accountShellCSS con el formulario de perfil. Contenido
// puro (sin <style>): su sha256 entra en la CSP (ver cspStyleHashes).
const profileCSS = `.pf{margin:14px 0 0}` +
	`.pf-2{display:grid;grid-template-columns:1fr 1fr;gap:0 12px}` +
	`@media(max-width:420px){.pf-2{grid-template-columns:1fr}}` +
	`.pf label{margin:12px 0 4px}` +
	`.pf select{width:100%;padding:10px 12px;font:inherit;color:var(--text);background:var(--surface);` +
	`border:1px solid var(--border);border-radius:8px;outline:none;transition:border-color .15s,box-shadow .15s}` +
	`.pf select:hover{border-color:var(--border-2)}` +
	`.pf select:focus-visible{border-color:var(--brand);box-shadow:0 0 0 3px var(--ring)}` +
	`.pf .note{margin:10px 0 0;color:var(--muted);font-size:12px;line-height:1.5}` +
	`.pf button{margin-top:18px}` +
	`.ok-msg{display:flex;gap:8px;align-items:center;margin:14px 0 0;padding:10px 12px;border-radius:8px;` +
	`background:var(--ok-bg);color:var(--ok);font-size:14px}` +
	`.ok-msg svg{flex:none;width:16px;height:16px}` +
	`.ro{display:flex;align-items:baseline;gap:10px;padding:9px 0;border-bottom:1px solid var(--border);font-size:14px}` +
	`.ro:last-child{border-bottom:0}.ro dt{flex:none;width:9rem;color:var(--muted);font-size:13px}` +
	`.ro dd{flex:1;min-width:0;margin:0;overflow-wrap:anywhere}`

// profileSectionHTML construye la tarjeta «Perfil» con el formulario editable.
func (s *Server) profileSectionHTML(r *http.Request, u domain.User) string {
	esc := html.EscapeString
	loc := i18n.From(r)
	campo := func(id, etiqueta, valor string, maxLen int) string {
		return `<label for="` + id + `">` + esc(etiqueta) + `</label>` +
			`<input type="text" id="` + id + `" name="` + id + `" value="` + esc(valor) + `"` +
			` maxlength="` + strconv.Itoa(maxLen) + `" autocomplete="` + autocompleteFor(id) + `">`
	}

	var b strings.Builder
	b.WriteString(`<p class="hint">` + esc(i18n.T(loc, "profile.hint")) + `</p>`)
	if r.URL.Query().Get("perfil") == "ok" {
		b.WriteString(okMsgHTML(i18n.T(loc, "profile.saved")))
	}
	b.WriteString(`<form class="pf" method="post" action="/account/profile">`)
	b.WriteString(campo("name", i18n.T(loc, "profile.display_name"), u.Name, 120))
	b.WriteString(`<div class="pf-2">` +
		`<div>` + campo("given_name", i18n.T(loc, "profile.given_name"), u.GivenName, 80) + `</div>` +
		`<div>` + campo("family_name", i18n.T(loc, "profile.family_name"), u.FamilyName, 80) + `</div>` +
		`</div>`)
	b.WriteString(`<div class="pf-2">` +
		`<div>` + campo("title", i18n.T(loc, "profile.job_title"), u.Title, 80) + `</div>` +
		`<div>` + campo("department", i18n.T(loc, "profile.department"), u.Department, 80) + `</div>` +
		`</div>`)
	b.WriteString(campo("location", i18n.T(loc, "profile.location"), u.Location, 80))

	// El idioma es la única preferencia de esta tarjeta que cambia algo fuera del
	// IdP, así que se dice: al guardarlo, el resto de la suite pasa a ese idioma.
	b.WriteString(`<label for="locale">` + esc(i18n.T(loc, "profile.language")) + `</label><select id="locale" name="locale">`)
	actual := domain.NormalizeLocale(u.Locale)
	for _, l := range domain.Locales {
		sel := ""
		if l == actual {
			sel = ` selected`
		}
		b.WriteString(`<option value="` + l + `"` + sel + `>` + esc(domain.LocaleNames[l]) + `</option>`)
	}
	b.WriteString(`</select>`)
	b.WriteString(`<p class="note">` + esc(i18n.T(loc, "profile.language_note")) + `</p>`)
	b.WriteString(`<p class="note">` + esc(i18n.T(loc, "profile.groups_note")) + `</p>`)
	b.WriteString(`<button type="submit">` + esc(i18n.T(loc, "profile.save")) + `</button></form>`)
	return b.String()
}

// autocompleteFor da la pista de autorrelleno del navegador para cada campo.
// Sin esto el navegador no ofrece los datos que ya conoce y el formulario se
// rellena a mano entero.
func autocompleteFor(id string) string {
	switch id {
	case "name":
		return "name"
	case "given_name":
		return "given-name"
	case "family_name":
		return "family-name"
	case "title":
		return "organization-title"
	case "location":
		return "address-level2"
	}
	return "off"
}

// okMsgHTML es la confirmación en verde tras un guardado correcto.
func okMsgHTML(msg string) string {
	return `<p class="ok-msg" role="status">` +
		`<svg viewBox="0 0 16 16" fill="none" aria-hidden="true">` +
		`<circle cx="8" cy="8" r="6.25" stroke="currentColor" stroke-width="1.5"/>` +
		`<path d="M5.25 8.25l1.9 1.9 3.6-3.9" stroke="currentColor" stroke-width="1.5" ` +
		`stroke-linecap="round" stroke-linejoin="round"/></svg>` +
		`<span>` + html.EscapeString(msg) + `</span></p>`
}

// handleAccountProfile guarda el perfil editable del usuario (PRG a /account).
func (s *Server) handleAccountProfile(w http.ResponseWriter, r *http.Request) {
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

	p := domain.ProfileEdit{
		Name:       r.Form.Get("name"),
		GivenName:  r.Form.Get("given_name"),
		FamilyName: r.Form.Get("family_name"),
		Title:      r.Form.Get("title"),
		Department: r.Form.Get("department"),
		Location:   r.Form.Get("location"),
		Locale:     r.Form.Get("locale"),
	}
	updated, err := s.store.SetProfile(r.Context(), nil, sess.UserID, p)
	if err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, i18n.T(i18n.From(r), "profile.save_failed"))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "profile.updated", "", r))

	// El idioma elegido se refleja en la cookie que lee el resto de la suite; sin
	// esto habría que volver a elegirlo en cada superficie y la elección de aquí
	// solo valdría dentro del IdP.
	s.setLocaleCookie(w, updated.Locale)

	http.Redirect(w, r, "/account?perfil=ok#perfil", http.StatusSeeOther)
}

// setLocaleCookie publica el idioma para toda la suite.
//
// A diferencia de la cookie de sesión —host-only con prefijo __Host- para que
// ningún subdominio pueda tocarla—, esta SÍ lleva Domain: su función es que
// Correo, Calendario y el resto se sirvan en el idioma elegido, y para eso han
// de poder leerla. Es legible por JS a propósito, por el mismo motivo.
//
// Que sea compartida no afecta al aislamiento de perfiles: no identifica a
// nadie ni autoriza nada, solo dice en qué idioma pintar la interfaz. Un
// subdominio comprometido que la cambiara conseguiría, como mucho, enseñar la
// interfaz en otro idioma.
func (s *Server) setLocaleCookie(w http.ResponseWriter, locale string) {
	suite := suiteDomain(s.cfg.Issuer)
	if suite == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Preferencia de idioma, no credencial: legible por la suite a propósito. Secure sigue la config del entorno.
		Name:     "sytue_locale",
		Value:    domain.NormalizeLocale(locale),
		Path:     "/",
		Domain:   "." + suite,
		MaxAge:   365 * 24 * 60 * 60,
		HttpOnly: false,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}
