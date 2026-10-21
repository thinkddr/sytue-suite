// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tarjeta «Claves de acceso» de /account: alta, renombrado y baja de passkeys
// desde la interfaz. El registro WebAuthn ya existía como API con Bearer
// (webauthn.go); lo que faltaba era poder usarlo desde el navegador con la
// sesión abierta, que es como lo hace una persona.
//
// Es la única parte del self-service que necesita JavaScript, porque
// navigator.credentials.create() no tiene equivalente en HTML. El alta se
// degrada con dignidad: si el navegador no soporta WebAuthn, el bloque de alta
// no aparece y el resto de la tarjeta —lista y bajas, que son formularios
// normales— sigue funcionando.
package http

import (
	"html"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/pg"

	"git.sytue.eu/sytue/one/services/id/internal/i18n"
)

// passkeysCSS estiliza la lista de claves. Contenido puro (sin <style>): su
// sha256 entra en la CSP (ver cspStyleHashes en middleware).
const passkeysCSS = `.pk-list{margin:14px 0 0;padding:0;list-style:none}` +
	`.pk-item{display:flex;align-items:center;gap:10px;padding:11px 0;border-bottom:1px solid var(--border)}` +
	`.pk-item:last-child{border-bottom:0}` +
	`.pk-ico{flex:none;width:32px;height:32px;border-radius:8px;display:grid;place-items:center;` +
	`background:var(--bg);border:1px solid var(--border);color:var(--muted)}` +
	`.pk-ico svg{width:16px;height:16px}` +
	`.pk-body{flex:1;min-width:0}` +
	`.pk-name{font-size:14px;font-weight:600;overflow-wrap:anywhere}` +
	`.pk-when{margin-top:2px;color:var(--muted);font-size:12px}` +
	`.pk-add{margin-top:16px;padding-top:16px;border-top:1px solid var(--border)}` +
	`.pk-add .row2{display:flex;gap:8px;align-items:flex-end}` +
	`.pk-add .row2>div{flex:1;min-width:0}` +
	`.pk-add label{margin:0 0 4px}` +
	`.pk-add button{margin:0;width:auto;flex:none;padding:10px 14px;white-space:nowrap}` +
	`.pk-none{margin:12px 0 0;color:var(--muted);font-size:13px;line-height:1.5}`

// passkeysScript da de alta una passkey desde «Mi cuenta». Mismo baile
// base64url ↔ ArrayBuffer que el login (WebAuthn habla en binario y JSON no),
// pero con create() en vez de get(). Su sha256 entra en la CSP: sin
// 'unsafe-inline' y recalculado solo si se edita (ver cspScriptHashes).
//
// Dos cosas que estaban mal y hacían que esto pareciera roto sin serlo:
//
//   - No reenviaba `excludeCredentials`, así que un dispositivo que ya guardaba
//     una passkey de esta cuenta contestaba InvalidStateError y el alta fallaba
//     siempre. Ahora se manda, y el navegador avisa en vez de reventar.
//   - Todos los desenlaces se aplanaban al mismo «no se pudo»: cancelar el
//     diálogo, tener ya la llave registrada y que fallara el servidor se leían
//     igual. Se distinguen por el `name` de la excepción, y los textos vienen
//     del catálogo por data-* en vez de estar escritos en castellano aquí
//     dentro, que es lo que había.
const passkeysScript = `(function(){` +
	`var w=document.getElementById('pk-add');if(!w){return;}` +
	`if(!window.PublicKeyCredential||!navigator.credentials||!navigator.credentials.create){return;}` +
	`w.hidden=false;` +
	`var b=document.getElementById('pk-go'),n=document.getElementById('pk-name'),e=document.getElementById('pk-err');` +
	`function fail(m){if(e){e.textContent=m;e.hidden=false;}b.disabled=false;b.textContent=b.dataset.add;}` +
	`function d(s){s=s.replace(/-/g,'+').replace(/_/g,'/');while(s.length%4){s+='=';}var x=atob(s),a=new Uint8Array(x.length),i=0;for(;i<x.length;i++){a[i]=x.charCodeAt(i);}return a.buffer;}` +
	`function c(buf){var a=new Uint8Array(buf),s='',i=0;for(;i<a.length;i++){s+=String.fromCharCode(a[i]);}return btoa(s).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,'');}` +
	`b.addEventListener('click',function(){` +
	`if(e){e.hidden=true;}b.disabled=true;b.textContent=b.dataset.wait;` +
	`fetch('/account/passkeys/begin',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'})` +
	`.then(function(r){if(!r.ok){throw 0;}return r.json();}).then(function(o){` +
	`var ex=(o.excludeCredentials||[]).map(function(k){return {type:k.type,id:d(k.id)};});` +
	`return navigator.credentials.create({publicKey:{challenge:d(o.challenge),rp:o.rp,` +
	`user:{id:d(o.user.id),name:o.user.name,displayName:o.user.displayName},` +
	`pubKeyCredParams:o.pubKeyCredParams,timeout:o.timeout,attestation:o.attestation,` +
	`excludeCredentials:ex,authenticatorSelection:o.authenticatorSelection}});` +
	`}).then(function(cr){var a=cr.response;return fetch('/account/passkeys/finish',{method:'POST',` +
	`headers:{'Content-Type':'application/json'},body:JSON.stringify({id:cr.id,rawId:c(cr.rawId),type:cr.type,` +
	`name:(n&&n.value)||'',attestationObject:c(a.attestationObject),clientDataJSON:c(a.clientDataJSON)})});})` +
	`.then(function(r){if(!r.ok){throw 0;}window.location='/account?pk=ok#claves';})` +
	// Cada desenlace dice lo suyo. Cancelar el diálogo del sistema NO es un
	// error del que haya que informar como si algo se hubiera roto, y «ya la
	// tienes registrada» es la respuesta a la pregunta que se estaba haciendo.
	`.catch(function(err){var k=err&&err.name;` +
	`fail(k==='NotAllowedError'?b.dataset.cancel:(k==='InvalidStateError'?b.dataset.dup:b.dataset.err));});` +
	`});})();`

// passkeysSectionHTML construye la tarjeta con las claves del usuario.
func (s *Server) passkeysSectionHTML(r *http.Request, userID string) string {
	esc := html.EscapeString
	loc := i18n.From(r)
	var b strings.Builder

	b.WriteString(`<p class="hint">` + esc(i18n.T(loc, "passkeys.hint")) + `</p>`)
	switch r.URL.Query().Get("pk") {
	case "ok":
		b.WriteString(okMsgHTML(i18n.T(loc, "passkeys.added")))
	case "borrada":
		b.WriteString(okMsgHTML(i18n.T(loc, "passkeys.removed")))
	case "ultima":
		b.WriteString(alertHTML(i18n.T(loc, "passkeys.last_one")))
	}

	creds, err := s.store.ListCredentialsForUser(r.Context(), userID)
	if err != nil {
		b.WriteString(`<p class="pk-none">` + esc(i18n.T(loc, "passkeys.load_failed")) + `</p>`)
		return b.String()
	}
	if len(creds) == 0 {
		b.WriteString(`<p class="pk-none">` + esc(i18n.T(loc, "passkeys.none")) + `</p>`)
	} else {
		b.WriteString(`<ul class="pk-list">`)
		for _, c := range creds {
			nombre := c.Name
			if strings.TrimSpace(nombre) == "" {
				nombre = i18n.T(loc, "passkeys.unnamed")
			}
			cuando := i18n.T(loc, "passkeys.added_on") + " " + fecha(loc, c.CreatedAt.Local(), false)
			if c.LastUsedAt != nil {
				cuando += " · " + i18n.T(loc, "passkeys.used_on") + " " + fecha(loc, c.LastUsedAt.Local(), false)
			} else {
				cuando += " · " + i18n.T(loc, "passkeys.never_used")
			}
			b.WriteString(`<li class="pk-item">` +
				`<span class="pk-ico">` + iconKey + `</span>` +
				`<span class="pk-body"><span class="pk-name">` + esc(nombre) + `</span>` +
				`<span class="pk-when">` + esc(cuando) + `</span></span>` +
				`<form method="post" action="/account/passkeys/delete">` +
				`<input type="hidden" name="credential_id" value="` + esc(c.ID) + `">` +
				`<button type="submit" class="s-btn" aria-label="` + esc(i18n.T(loc, "passkeys.remove")+" "+nombre) + `">` + esc(i18n.T(loc, "passkeys.remove")) + `</button>` +
				`</form></li>`)
		}
		b.WriteString(`</ul>`)
	}

	// hidden por defecto: el script lo muestra solo si el navegador sabe WebAuthn.
	// Al revés (mostrar y ocultar) el botón parpadearía en los que no lo soportan.
	b.WriteString(`<div class="pk-add" id="pk-add" hidden>` +
		`<div class="row2"><div>` +
		`<label for="pk-name">` + esc(i18n.T(loc, "passkeys.name")) + `</label>` +
		`<input type="text" id="pk-name" maxlength="60" placeholder="` + esc(i18n.T(loc, "passkeys.name_placeholder")) + `" autocomplete="off">` +
		`</div><button type="button" id="pk-go"` +
		` data-add="` + esc(i18n.T(loc, "passkeys.add")) + `"` +
		` data-wait="` + esc(i18n.T(loc, "passkeys.confirming")) + `"` +
		` data-err="` + esc(i18n.T(loc, "passkeys.failed")) + `"` +
		` data-dup="` + esc(i18n.T(loc, "passkeys.already")) + `"` +
		` data-cancel="` + esc(i18n.T(loc, "passkeys.cancelled")) + `">` +
		esc(i18n.T(loc, "passkeys.add")) + `</button></div>` +
		`<p class="pk-err" id="pk-err" hidden role="alert"></p></div>`)
	return b.String()
}

// handlePasskeyDelete quita una passkey del usuario (formulario POST, PRG).
func (s *Server) handlePasskeyDelete(w http.ResponseWriter, r *http.Request) {
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
	id := r.Form.Get("credential_id")
	if !uuidRe.MatchString(id) {
		http.Redirect(w, r, "/account#claves", http.StatusSeeOther)
		return
	}

	// Guard contra dejar la cuenta con el segundo factor activado y ningún factor
	// con el que cumplirlo: si esta es la última passkey y no hay TOTP confirmado,
	// quitarla dejaría mfa_enabled=true sin nada detrás, y la siguiente aplicación
	// que exija MFA daría un paso imposible en vez de un acceso. Se rechaza y se
	// dice cómo salir del paso.
	creds, err := s.store.ListCredentialsForUser(r.Context(), sess.UserID)
	if err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, i18n.T(i18n.From(r), "passkeys.load_failed"))
		return
	}
	if len(creds) <= 1 && !s.totpConfirmed(r, sess.UserID) {
		user, uerr := s.store.GetUserByID(r.Context(), sess.UserID)
		if uerr == nil && user.MFAEnabled {
			http.Redirect(w, r, "/account?pk=ultima#claves", http.StatusSeeOther)
			return
		}
	}

	var borrada bool
	ctx := r.Context()
	err = s.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		borrada, e = s.store.DeleteCredentialForUser(ctx, q, sess.UserID, id)
		return e
	})
	if err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, i18n.T(i18n.From(r), "passkeys.remove_failed"))
		return
	}
	if borrada {
		_ = s.store.Audit(r.Context(), nil,
			auditEntry(sess.UserID, sess.OrgID, "webauthn.credential.removed", id, r))
		s.avisarSeguridad(r.Context(), sess.OrgID, sess.UserID, "security.passkey.removed", nil)
	}
	http.Redirect(w, r, "/account?pk=borrada#claves", http.StatusSeeOther)
}
