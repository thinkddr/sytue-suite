// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

// El desbloqueo del cifrado con el inicio de sesión de Sytue ID (ADR 0084 y 0085).
//
// La persona solo mete su contraseña (o usa su passkey) aquí. La página de inicio
// de sesión deriva EN EL NAVEGADOR la KEK de su sobre (e2e/login.js); al acabar la
// autorización, en lugar de redirigir, este servicio pinta el RELEVO
// (e2e/relay.js), que abre el sobre en el navegador y deja la clave de desbloqueo
// en el almacén de este origen como CryptoKey no extraíble (ADR 0085); las apps web
// le piden al PUENTE (e2e/bridge.js) que abra o cierre con ella el envoltorio de su
// cuenta, sin recibirla nunca, y las nativas la reciben cifrada para su clave
// efímera en la URL de vuelta.
//
// Lo que ve este servidor: la sal pública, los sobres (texto cifrado que no puede
// abrir) y la contraseña al comprobarla, como siempre. Lo que NO ve: la KEK, la
// salida PRF de la passkey ni la clave de desbloqueo. El relevo solo se pinta si
// la app lo pide (`sytue_unlock=1` o `sytue_unlock_key`): el resto de clientes y
// de pruebas siguen recibiendo la redirección de siempre.

import (
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
)

var (
	//go:embed e2e/comun.js
	e2eComunJS string
	//go:embed e2e/login.js
	e2eLoginJS string
	//go:embed e2e/relay.js
	e2eRelayJS string
	//go:embed e2e/bridge.js
	e2eBridgeJS string
	// e2eArgon2JS es la compilación de Argon2id de hash-wasm 4.12.0 (MIT), la misma
	// versión que usan las apps web (packages/offline). Va copiada porque este
	// servicio no tiene paso de empaquetado; la prueba
	// packages/offline/src/matrix/sytue-id-pages.test.ts falla si deja de ser byte a
	// byte la de node_modules.
	//go:embed e2e/argon2.umd.min.js
	e2eArgon2JS string
)

// e2eScriptsLogin son los guiones del desbloqueo de la página de inicio de sesión:
// Argon2id solo donde hay contraseña que derivar.
func e2eScriptsLogin(conContrasena bool) string {
	out := ""
	if conContrasena {
		out += `<script>` + e2eArgon2JS + `</script>`
	}
	return out + `<script>` + e2eComunJS + `</script><script>` + e2eLoginJS + `</script>`
}

// cspConWasm es la CSP de la página de inicio de sesión: la de siempre más
// 'wasm-unsafe-eval', que deja COMPILAR WebAssembly (el Argon2id) y nada más: un
// guion que no esté en la lista de hashes sigue sin ejecutarse.
func cspConWasm(w http.ResponseWriter) {
	h := w.Header()
	if csp := h.Get("Content-Security-Policy"); csp != "" && !strings.Contains(csp, "'wasm-unsafe-eval'") {
		h.Set("Content-Security-Policy", strings.Replace(csp, "script-src ", "script-src 'wasm-unsafe-eval' ", 1))
	}
}

// unlockSaltRequest es el cuerpo de /login/unlock/salt.
type unlockSaltRequest struct {
	Email string `json:"email"`
}

// handleUnlockSalt — POST /login/unlock/salt — la sal pública con la que la página
// de inicio de sesión deriva la KEK antes de enviar la contraseña. Contesta lo
// mismo exista o no la cuenta (una sal estable por dirección), así que no sirve
// para saber quién tiene cuenta.
func (s *Server) handleUnlockSalt(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Origen no permitido."))
		return
	}
	var req unlockSaltRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	login := domain.NormalizeLogin(req.Email)
	if login == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	var kdf recoverywrap.KDF
	if user, err := s.userByLogin(r.Context(), login); err == nil && user.Active() {
		k, _, err := s.unlockKDF(r, user.ID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		kdf = k
	} else {
		kdf = s.saltKDF("login", login)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"kdf": kdf})
}

// saltKDF son los parámetros por defecto con una sal derivada de la KEK del
// servicio: la de quien aún no tiene sobre y la de una dirección sin cuenta.
func (s *Server) saltKDF(tipo, valor string) recoverywrap.KDF {
	mac := hmac.New(sha256.New, s.unlockSaltKey)
	mac.Write([]byte(tipo + "\x00" + valor))
	return recoverywrap.KDF{
		Alg: recoverywrap.AlgArgon2id, Memory: recoverywrap.DefaultMemoryKiB,
		Time: recoverywrap.DefaultTime, Threads: recoverywrap.DefaultThreads,
		Salt: mac.Sum(nil)[:16],
	}
}

// unlockKDF son los parámetros del sobre de contraseña de la persona (o los que
// tendrá, si aún no lo tiene) y sus sobres válidos.
func (s *Server) unlockKDF(r *http.Request, userID string) (recoverywrap.KDF, []recoverywrap.Envelope, error) {
	rows, err := s.store.UnlockEnvelopes(r.Context(), userID)
	if err != nil {
		return recoverywrap.KDF{}, nil, err
	}
	kdf := s.saltKDF("user", userID)
	var out []recoverywrap.Envelope
	for _, row := range rows {
		var e recoverywrap.Envelope
		if json.Unmarshal(row.Envelope, &e) != nil || e.Validate() != nil {
			continue
		}
		e.UpdatedAt = row.UpdatedAt.UTC().Format(time.RFC3339)
		out = append(out, e)
		if e.Kind == recoverywrap.KindPassword {
			kdf = e.KDF
		}
	}
	return kdf, out, nil
}

// unlockEnvelopePut es el cuerpo de PUT /e2e/envelopes. `sub` es la persona para
// la que el navegador cerró el sobre (va en su AAD): si no es la de la sesión, el
// sobre no abriría nunca y sustituiría a uno bueno, así que se rechaza.
type unlockEnvelopePut struct {
	Envelope json.RawMessage `json:"envelope"`
	Reset    bool            `json:"reset"`
	Sub      string          `json:"sub"`
}

// handleUnlockEnvelopePut — PUT /e2e/envelopes — guarda el sobre que el relevo
// acaba de cerrar en el navegador. Solo con la sesión de esta persona y desde este
// mismo origen; solo comprueba la forma (no lo puede abrir). Un sobre de passkey
// tiene que ser de una passkey suya.
func (s *Server) handleUnlockEnvelopePut(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Sec-Fetch-Site") != "same-origin" && !s.sameOrigin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Origen no permitido."))
		return
	}
	sess, ok := s.currentSession(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req unlockEnvelopePut
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.Sub != "" && req.Sub != sess.UserID {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Envelope for another person."))
		return
	}
	var e recoverywrap.Envelope
	if json.Unmarshal(req.Envelope, &e) != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	e.UpdatedAt = ""
	if e.Validate() != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid envelope."))
		return
	}
	if e.Kind == recoverywrap.KindPasskey {
		creds, err := s.store.ListCredentialsForUser(r.Context(), sess.UserID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		suya := false
		for _, c := range creds {
			suya = suya || b64u.EncodeToString(c.CredID) == e.CredentialID
		}
		if !suya {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unknown passkey."))
			return
		}
	}
	raw, err := json.Marshal(e)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	hint := base64.StdEncoding.EncodeToString(e.Hint)
	if err := s.store.PutUnlockEnvelope(r.Context(), sess.UserID, e.Kind, e.CredentialID, hint, raw, req.Reset); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	action := "e2e.unlock.envelope"
	if req.Reset {
		action = "e2e.unlock.reset"
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, action, e.Kind, r))
	w.WriteHeader(http.StatusNoContent)
}

// claveDeEntrega valida el `sytue_unlock_key` de una app nativa: un punto P-256
// sin comprimir en base64url. Una clave rota no pinta el relevo.
func claveDeEntrega(v string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil || len(raw) != 65 {
		return false
	}
	_, err = ecdh.P256().NewPublicKey(raw)
	return err == nil
}

// quiereRelevo dice si esta autorización acaba en el relevo y, para una app
// nativa, con qué clave pública. Solo los clientes de la suite (sin organización):
// una app de terceros recibe la redirección de siempre.
func quiereRelevo(req authzRequest, client domain.Client) (appKey string, ok bool) {
	if client.OrgID != "" {
		return "", false
	}
	if client.EsNativa() {
		if req.UnlockKey != "" && claveDeEntrega(req.UnlockKey) {
			return req.UnlockKey, true
		}
		return "", false
	}
	return "", req.Unlock
}

// relevoDatos es lo que el relevo necesita, en el propio documento. No lleva nada
// secreto: los sobres y su huella son públicos, y `next` es la redirección que se
// habría mandado.
type relevoDatos struct {
	Sub       string                  `json:"sub"`
	Next      string                  `json:"next"`
	AppKey    string                  `json:"app_key"`
	Envelopes []recoverywrap.Envelope `json:"envelopes"`
	Hint      string                  `json:"hint"`
	Salt      string                  `json:"salt"`
	Creds     []string                `json:"creds"`
	AuthAge   int64                   `json:"auth_age"`
}

// entregarCodigo termina la autorización: la redirección de siempre, o el relevo
// del desbloqueo cuando la app lo pide. Si el relevo no se puede pintar, se
// redirige igual: entrar no depende del desbloqueo.
func (s *Server) entregarCodigo(w http.ResponseWriter, r *http.Request, req authzRequest, client domain.Client, sess domain.Session, destino string) {
	appKey, ok := quiereRelevo(req, client)
	if !ok {
		http.Redirect(w, r, destino, http.StatusFound)
		return
	}
	kdf, envelopes, err := s.unlockKDF(r, sess.UserID)
	if err != nil {
		s.log.Warn("desbloqueo: sin sobres para el relevo; se redirige sin él", "error", err)
		http.Redirect(w, r, destino, http.StatusFound)
		return
	}
	datos := relevoDatos{
		Sub: sess.UserID, Next: destino, AppKey: appKey, Envelopes: envelopes,
		Salt: base64.StdEncoding.EncodeToString(kdf.Salt), Creds: []string{},
		AuthAge: time.Since(sess.AuthTime).Milliseconds(),
	}
	if datos.Envelopes == nil {
		datos.Envelopes = []recoverywrap.Envelope{}
	}
	for _, e := range envelopes {
		datos.Hint = base64.StdEncoding.EncodeToString(e.Hint)
	}
	if creds, err := s.store.ListCredentialsForUser(r.Context(), sess.UserID); err == nil {
		for _, c := range creds {
			datos.Creds = append(datos.Creds, b64u.EncodeToString(c.CredID))
		}
	}
	// json.Marshal escapa <, > y &: el bloque no puede cerrar el <script>.
	raw, err := json.Marshal(datos)
	if err != nil {
		http.Redirect(w, r, destino, http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(relevoPage(i18n.From(r), destino, string(raw))))
}

// relevoPage pinta el relevo: «Desbloqueando…» mientras el guion trabaja, la
// salida a mano si el navegador no deja ir solo a la app, el aviso de una passkey
// que aún no abre nada (entrar una vez con la contraseña) y el de que la
// contraseña ya no abre el sobre (con «empezar de cero» tras confirmar).
func relevoPage(loc, destino, datos string) string {
	esc := html.EscapeString
	t := func(k string) string { return esc(i18n.T(loc, k)) }
	body := `<p class="sub" id="e2e-working" role="status">` + t("e2e.unlocking") + `</p>` +
		`<p class="alt" id="e2e-manual" hidden><a class="btn-link" id="e2e-next" href="` + esc(destino) + `">` + t("e2e.back_to_app") + `</a></p>` +
		`<noscript><p class="alt"><a class="btn-link" href="` + esc(destino) + `">` + t("e2e.back_to_app") + `</a></p></noscript>` +
		`<section id="e2e-passkey-first" hidden>` +
		alertHTML(i18n.T(loc, "e2e.passkey_first_title")) +
		`<p class="sub">` + t("e2e.passkey_first_body") + `</p>` +
		`<p class="alt"><a class="btn-link" href="` + esc(destino) + `">` + t("e2e.back_to_app") + `</a></p>` +
		`</section>` +
		`<section id="e2e-lost" hidden>` +
		alertHTML(i18n.T(loc, "e2e.lost_title")) +
		`<p class="sub">` + t("e2e.lost_body") + `</p>` +
		`<p class="alt"><a class="btn-link" href="` + esc(destino) + `">` + t("e2e.continue") + `</a></p>` +
		`<label class="check"><input type="checkbox" id="e2e-understood"> ` + t("e2e.start_over_confirm") + `</label>` +
		`<button type="button" id="e2e-restart" disabled>` + t("e2e.start_over") + `</button>` +
		`</section>` +
		`<script type="application/json" id="e2e-relay">` + datos + `</script>` +
		`<script>` + e2eComunJS + `</script><script>` + e2eRelayJS + `</script>`
	return loginPage(loc, i18n.T(loc, "e2e.title"), body)
}

// origenDelPuente devuelve el origen que pide el puente si es de la suite: la
// MISMA regla que CORS (los nombres de la lista y los subdominios directos del
// dominio, nunca `x.apps.sytue.eu`, donde corre código de clientes).
func (s *Server) origenDelPuente(o string) (string, bool) {
	u, err := url.Parse(o)
	if err != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil ||
		!s.esquemaDelPuente(u) {
		return "", false
	}
	origen := u.Scheme + "://" + u.Host
	if origen != o || origen == strings.TrimRight(s.cfg.Issuer, "/") {
		return "", false
	}
	return origen, httpx.OrigenPermitido(origen, strings.Split(pcfg.FirstPartyCORSOrigins(s.cfg.Env), ","))
}

// esquemaDelPuente: https siempre; http solo fuera de producción y en *.localhost,
// que es como corre la suite en desarrollo.
func (s *Server) esquemaDelPuente(u *url.URL) bool {
	if u.Scheme == "https" {
		return true
	}
	return u.Scheme == "http" && !s.cfg.Env.IsProd() && strings.HasSuffix(u.Hostname(), ".localhost")
}

// handleUnlockBridge — GET /e2e/bridge?o=<origen> — el puente que una app web de
// la suite carga en un iframe oculto. Solo se deja incrustar por ESE origen
// (`frame-ancestors`), y su guion solo le contesta a él.
func (s *Server) handleUnlockBridge(w http.ResponseWriter, r *http.Request) {
	origen, ok := s.origenDelPuente(r.URL.Query().Get("o"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	raw, err := json.Marshal(map[string]string{"origin": origen})
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	h := w.Header()
	// La CSP general dice `frame-ancestors 'none'` y X-Frame-Options DENY: esta es
	// la única página del IdP que vive en un iframe, y solo en el de ese origen.
	h.Del("X-Frame-Options")
	h.Set("Content-Security-Policy", "default-src 'none'; script-src "+cspScriptHashes+
		"; connect-src 'none'; frame-ancestors "+origen+"; base-uri 'none'; form-action 'none'")
	h.Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><meta name="robots" content="noindex">` +
		`<link rel="icon" href="/favicon.svg" type="image/svg+xml"><link rel="apple-touch-icon" href="/apple-touch-icon.png">` +
		`<title>Sytue ID</title></head><body>` +
		`<script type="application/json" id="e2e-bridge">` + string(raw) + `</script>` +
		`<script>` + e2eComunJS + `</script><script>` + e2eBridgeJS + `</script></body></html>`))
}

// uriSinPrompt es la petición de autorización sin `prompt`: a donde vuelve el
// inicio de sesión que `prompt=login` obliga a hacer. Con él dentro, la vuelta
// pediría otro inicio de sesión, y otro, sin salida.
func uriSinPrompt(r *http.Request) string {
	u := *r.URL
	q := u.Query()
	if !q.Has("prompt") {
		return r.URL.RequestURI()
	}
	q.Del("prompt")
	u.RawQuery = q.Encode()
	return u.RequestURI()
}

// pidePromptLogin dice si la petición lleva `prompt=login` (OIDC Core §3.1.2.1:
// lista separada por espacios).
func pidePromptLogin(r *http.Request) bool {
	for _, p := range strings.Fields(r.URL.Query().Get("prompt")) {
		if p == "login" {
			return true
		}
	}
	return false
}
