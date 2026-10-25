// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"crypto/rand"
	"html"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
)

// Contraseñas de aplicación: lo que se escribe en Thunderbird, Outlook, Apple
// Mail o el móvil.
//
// Sin esto Sytue Mail no se puede usar desde ningún cliente: el IMAP y el SMTP
// solo aceptan como contraseña un JWT de Sytue ID con scope `mail`, que dura
// minutos y que nadie puede obtener a mano. La suite, mientras tanto, publica su
// autoconfiguración e invita a configurarlos. Medido contra producción el
// 2026-08-16: `535 5.7.8 Authentication failed` con la contraseña de la cuenta.
//
// La contraseña se enseña UNA vez. No es una molestia heredada de otros: solo se
// guarda su hash, así que no hay forma de volver a mostrarla — y esa es
// exactamente la propiedad que hace que filtrar la base de datos no entregue las
// credenciales de correo de nadie.

// alfabetoAppPassword: minúsculas y dígitos SIN los caracteres que se confunden
// al copiar a mano de una pantalla a un móvil (l/1, o/0). Con 20 caracteres de
// este alfabeto son ~100 bits: sobra para que un ataque por fuerza bruta contra
// el IMAP no exista, que es el único ataque posible contra un secreto que no
// elige ninguna persona.
const alfabetoAppPassword = "abcdefghijkmnpqrstuvwxyz23456789"

// generarAppPassword devuelve la contraseña en claro, en grupos de cuatro
// separados por guion («abcd-efgh-ijkl-mnpq-rstu»). Los grupos son para poder
// dictarla y teclearla sin perder el sitio; los guiones se ignoran al verificar,
// así que da igual si el cliente los copia o no.
func generarAppPassword() (string, error) { return generarClave(20) }

// generarClave son n caracteres de alfabetoAppPassword en grupos de cuatro. La
// usan también las contraseñas temporales de la consola de plataforma, que se
// dictan y se teclean igual.
func generarClave(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 && i%4 == 0 {
			sb.WriteByte('-')
		}
		sb.WriteByte(alfabetoAppPassword[int(b[i])%len(alfabetoAppPassword)])
	}
	return sb.String(), nil
}

// NormalizeAppPassword deja la contraseña como se guardó su hash: sin espacios ni
// guiones y en minúsculas. Quien la copia de la pantalla se lleva los guiones y
// quien la teclea a veces no, y las dos formas tienen que entrar.
func NormalizeAppPassword(v string) string {
	return strings.ToLower(strings.NewReplacer("-", "", " ", "", "\t", "").Replace(strings.TrimSpace(v)))
}

// handleAccountAppPasswordCreate — POST /account/app-passwords — genera una
// contraseña de aplicación y la enseña UNA vez.
func (s *Server) handleAccountAppPasswordCreate(w http.ResponseWriter, r *http.Request) {
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
	etiqueta := strings.TrimSpace(r.Form.Get("label"))
	if len(etiqueta) > 60 {
		etiqueta = etiqueta[:60]
	}

	claro, err := generarAppPassword()
	if err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, i18n.T(i18n.From(r), "app_passwords.create_failed"))
		return
	}
	if _, err := s.store.CreateAppPassword(r.Context(), sess.UserID, sess.OrgID,
		domain.HashToken(NormalizeAppPassword(claro)), etiqueta); err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, i18n.T(i18n.From(r), "app_passwords.create_failed"))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "app_password.created", "", r))

	// Página terminal, sin PRG: tras un redirect la contraseña ya no existiría en
	// ninguna parte (solo se guarda el hash). Es el mismo patrón que los códigos
	// de recuperación del segundo factor.
	s.writeAppPasswordPage(w, r, claro, etiqueta)
}

// handleAccountAppPasswordRevoke — POST /account/app-passwords/revoke.
func (s *Server) handleAccountAppPasswordRevoke(w http.ResponseWriter, r *http.Request) {
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
	// La autorización va en el WHERE del store (user_id), no aquí: un id ajeno
	// simplemente no casa y devuelve false.
	ok2, err := s.store.RevokeAppPassword(r.Context(), sess.UserID, r.Form.Get("id"))
	if err != nil {
		accountRedirect(w, r, "ap_error")
		return
	}
	if ok2 {
		_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "app_password.revoked", "", r))
	}
	accountRedirect(w, r, "ap_revocada")
}

// fila pinta una entrada de la lista de definición. El `row` de /account es una
// clausura dentro de su handler, así que aquí va la misma forma sin sacarla de
// allí: mover un helper de dos líneas a un tercer sitio solo para compartirlo
// haría el cambio más grande que la función.
func fila(dt, dd string) string {
	return `<div class="row"><dt>` + dt + `</dt><dd>` + dd + `</dd></div>`
}

// writeAppPasswordPage enseña la contraseña recién creada, una sola vez, con las
// instrucciones de servidor que hacen falta para configurar el cliente.
func (s *Server) writeAppPasswordPage(w http.ResponseWriter, r *http.Request, claro, etiqueta string) {
	esc := html.EscapeString
	loc := i18n.From(r)
	// Las MISMAS coordenadas que el perfil de configuración de Apple
	// (`mobileconfig.go`), que a su vez salen del dominio de la suite: si algún
	// día el despliegue publica otros FQDN, las dos pantallas cambian a la vez.
	cuerpo := `<p class="rc-intro">` + esc(i18n.T(loc, "app_passwords.shown_once")) + `</p>` +
		`<div class="totp-key" aria-label="` + esc(i18n.T(loc, "app_passwords.the_password")) + `">` + esc(claro) + `</div>` +
		`<dl class="rows">` +
		fila(esc(i18n.T(loc, "app_passwords.imap_server")), `<span class="mono">`+esc(s.cfg.MailIMAPAddr)+`</span> · SSL/TLS`) +
		fila(esc(i18n.T(loc, "app_passwords.smtp_server")), `<span class="mono">`+esc(s.cfg.MailSMTPAddr)+`</span> · SSL/TLS`) +
		`</dl>` +
		`<p class="hint">` + esc(i18n.T(loc, "app_passwords.user_is_email")) + `</p>` +
		`<p><a class="btn" href="/account">` + esc(i18n.T(loc, "common.done")) + `</a></p>`

	titulo := i18n.T(loc, "app_passwords.created")
	if etiqueta != "" {
		titulo += " · " + etiqueta
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(loginPage(loc, titulo, cuerpo)))
}

// appPasswordsSectionHTML pinta la tarjeta de /account: lo que hay, para qué es
// cada una y cuándo se usó por última vez.
func (s *Server) appPasswordsSectionHTML(r *http.Request, userID string) string {
	esc := html.EscapeString
	loc := i18n.From(r)
	var b strings.Builder

	b.WriteString(`<p class="hint">` + esc(i18n.T(loc, "app_passwords.hint")) + `</p>`)
	// El mismo parámetro `sec` que usan las demás tarjetas de seguridad: lo
	// escribe accountRedirect, y leer otro distinto dejaría el aviso mudo.
	switch r.URL.Query().Get("sec") {
	case "ap_revocada":
		b.WriteString(okMsgHTML(i18n.T(loc, "app_passwords.revoked")))
	case "ap_error":
		b.WriteString(alertHTML(i18n.T(loc, "app_passwords.revoke_failed")))
	}

	lista, err := s.store.ListAppPasswords(r.Context(), userID)
	if err != nil {
		b.WriteString(`<p class="pk-none">` + esc(i18n.T(loc, "app_passwords.load_failed")) + `</p>`)
		return b.String()
	}
	if len(lista) == 0 {
		b.WriteString(`<p class="pk-none">` + esc(i18n.T(loc, "app_passwords.none")) + `</p>`)
	} else {
		b.WriteString(`<ul class="pk-list">`)
		for _, a := range lista {
			nombre := a.Label
			if strings.TrimSpace(nombre) == "" {
				nombre = i18n.T(loc, "app_passwords.unnamed")
			}
			// El «nunca usada» es la mitad del valor de esta lista: una contraseña
			// de hace un mes sin uso dice que ese cliente jamás llegó a conectarse.
			cuando := i18n.T(loc, "app_passwords.created_on") + " " + fecha(loc, a.CreatedAt.Local(), false)
			if a.LastUsedAt != nil {
				cuando += " · " + i18n.T(loc, "app_passwords.used_on") + " " + fecha(loc, a.LastUsedAt.Local(), false)
			} else {
				cuando += " · " + i18n.T(loc, "app_passwords.never_used")
			}
			b.WriteString(`<li class="pk-item">` +
				`<span class="pk-ico">` + iconKey + `</span>` +
				`<span class="pk-body"><span class="pk-name">` + esc(nombre) + `</span>` +
				`<span class="pk-when">` + esc(cuando) + `</span></span>` +
				`<form method="post" action="/account/app-passwords/revoke">` +
				`<input type="hidden" name="id" value="` + esc(a.ID) + `">` +
				`<button type="submit" class="btn-danger">` + esc(i18n.T(loc, "app_passwords.revoke")) + `</button>` +
				`</form></li>`)
		}
		b.WriteString(`</ul>`)
	}

	b.WriteString(`<form method="post" action="/account/app-passwords" class="sec-form">` +
		`<label for="ap-label">` + esc(i18n.T(loc, "app_passwords.label")) + `</label>` +
		`<input id="ap-label" type="text" name="label" maxlength="60" spellcheck="false" ` +
		`placeholder="` + esc(i18n.T(loc, "app_passwords.label_example")) + `">` +
		`<button type="submit">` + esc(i18n.T(loc, "app_passwords.create")) + `</button></form>`)
	return b.String()
}
