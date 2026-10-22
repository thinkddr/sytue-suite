// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Login federado por SAML 2.0 (Sytue ID como Service Provider). Una empresa con
// Microsoft EntraID, ADFS u Okta federa su login corporativo por SAML contra Sytue.
// REUSA el modelo de la federación OIDC (mig 0015): federated_auth_states para el
// estado efímero single-use, federated_identities para el enlace, y el MISMO
// find-or-JIT con anti-takeover (resolveFederatedUser) — no se duplica ni el modelo
// ni el aprovisionamiento. Endpoints:
//
//	GET  /saml/{id}/metadata  metadata XML del SP (entityID + ACS) para registrar
//	                          en el IdP. Público, sin secretos.
//	GET  /saml/{id}/login     genera el AuthnRequest (binding redirect/POST),
//	                          persiste el request-ID (anti-replay) ligado al flujo y
//	                          redirige/postea al IdP con RelayState.
//	POST /saml/{id}/acs       Assertion Consumer Service: valida la Response con
//	                          crewjam (firma XML-DSig contra el cert del IdP,
//	                          Conditions, AudienceRestriction==SP, Recipient==ACS,
//	                          InResponseTo single-use), casa/aprovisiona el usuario y
//	                          abre la MISMA sesión SSO (amr=saml).
//
// El handle público del provider es su UUID. La página de login enruta por dominio
// de email a /saml/{id}/login cuando el provider del dominio es SAML.
package http

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/saml"
)

// samlCookieName ata el flujo SAML al navegador que lo inició (binding anti
// login-CSRF junto al RelayState persistido en BD). Ámbito /saml.
const samlCookieName = "sytue_id_saml"

// samlPostScriptHash es el sha256 (base64) del <script> de autoenvío que emite
// crewjam en el binding POST, para autorizarlo en la CSP de esa página SIN
// 'unsafe-inline'. Si crewjam cambiara ese script, el autoenvío degrada al botón
// visible «Continuar» (fallback sin JS); nada se rompe.
// ponytail: hash del script interno de crewjam; el fallback es el botón submit.
var samlPostScriptHash = func() string {
	const js = `document.getElementById('SAMLSubmitButton').style.visibility="hidden";` +
		`document.getElementById('SAMLRequestForm').submit();`
	sum := sha256.Sum256([]byte(js))
	return "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
}()

// samlACSURL / samlMetadataURL son los endpoints propios del SP para un provider,
// derivados del Issuer público (estables por provider). El admin registra el ACS y
// el entityID (metadata URL) en su IdP.
func (s *Server) samlACSURL(id string) string {
	return strings.TrimRight(s.cfg.Issuer, "/") + "/saml/" + id + "/acs"
}

func (s *Server) samlMetadataURL(id string) string {
	return strings.TrimRight(s.cfg.Issuer, "/") + "/saml/" + id + "/metadata"
}

// setSamlCookie usa SameSite=None (Secure) para que la cookie viaje en el POST
// cross-site del IdP hacia el ACS (una cookie Lax NO se envía en un POST cross-site).
// En dev sin Secure, los navegadores rechazan SameSite=None, así que se cae a Lax
// (SAML real es siempre https, donde None+Secure es correcto).
func (s *Server) samlSameSite() http.SameSite {
	if s.cfg.CookieSecure {
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}

func (s *Server) setSamlCookie(w http.ResponseWriter, relayState string, exp time.Time) {
	s.clearLegacyDomainCookie(w, samlCookieName, "/saml")
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure es config (CookieSecure); HttpOnly fijo.
		Name: secureCookieName(samlCookieName, s.cfg.CookieSecure), Value: relayState, Path: "/",
		Expires: exp, HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: s.samlSameSite(),
	})
}

func (s *Server) clearSamlCookie(w http.ResponseWriter) {
	s.clearLegacyDomainCookie(w, samlCookieName, "/saml")
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: idem setSamlCookie.
		Name: secureCookieName(samlCookieName, s.cfg.CookieSecure), Value: "", Path: "/",
		MaxAge: -1, HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: s.samlSameSite(),
	})
}

// loadSAMLProvider carga un provider SAML por UUID (uso interno del flujo, ligado al
// auth_state por provider_id). Devuelve false si no existe, no es SAML, está inactivo
// o no está configurado.
func (s *Server) loadSAMLProvider(r *http.Request, id string) (domain.IdentityProvider, bool) {
	if _, err := uuid.Parse(id); err != nil {
		return domain.IdentityProvider{}, false
	}
	prov, err := s.store.GetIdentityProviderByID(r.Context(), id)
	if err != nil || !prov.IsSAML() {
		return domain.IdentityProvider{}, false
	}
	return prov, true
}

// handleSAMLMetadata publica la metadata XML del SP para este provider (entityID +
// ACS URL). Pública y sin secretos; los IdP la descargan para configurar el SP.
func (s *Server) handleSAMLMetadata(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	prov, ok := s.loadSAMLProvider(r, id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	sp, err := saml.BuildSP(prov, s.samlACSURL(id), s.samlMetadataURL(id))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	md, err := sp.MetadataXML()
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="sytue-sp-metadata.xml"`)
	_, _ = w.Write(md)
}

// handleSAMLLogin inicia el flujo SP-initiated: genera el AuthnRequest, persiste su
// ID (para casar el InResponseTo, single-use) ligado al RelayState, y redirige o
// postea al IdP.
func (s *Server) handleSAMLLogin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	prov, ok := s.loadSAMLProvider(r, id)
	if !ok || !prov.Enabled || !prov.Configured() {
		federatedError(w, r, http.StatusNotFound, "Este inicio de sesión no está disponible.")
		return
	}
	returnTo := safeInternalPath(r.URL.Query().Get("return_to"))

	relayState, err := domain.GenerateToken(32)
	if err != nil {
		federatedError(w, r, http.StatusInternalServerError, "No se pudo iniciar el flujo. Reinténtalo.")
		return
	}
	sp, err := saml.BuildSP(prov, s.samlACSURL(id), s.samlMetadataURL(id))
	if err != nil {
		federatedError(w, r, http.StatusInternalServerError, "Configuración del proveedor inválida.")
		return
	}
	var (
		requestID, redirectURL string
		form                   []byte
	)
	if prov.SAMLBinding == "post" {
		requestID, form, err = sp.MakePost(relayState)
	} else {
		requestID, redirectURL, err = sp.MakeRedirect(relayState)
	}
	if err != nil {
		federatedError(w, r, http.StatusInternalServerError, "No se pudo iniciar el flujo. Reinténtalo.")
		return
	}

	exp := time.Now().Add(fedStateTTL)
	// Reusa federated_auth_states: la columna nonce guarda el request-ID emitido (lo
	// que el IdP debe devolver en InResponseTo). El borrado atómico en el ACS lo hace
	// de un solo uso (anti-replay). code_verifier no se usa en SAML ('').
	if err := s.store.InsertFederatedAuthState(r.Context(), nil,
		domain.HashToken(relayState), prov.ID, requestID, "", returnTo, exp); err != nil {
		federatedError(w, r, http.StatusInternalServerError, "No se pudo iniciar el flujo. Reinténtalo.")
		return
	}
	s.setSamlCookie(w, relayState, exp)
	_ = s.store.Audit(r.Context(), nil, auditEntry("", prov.OrgID, "auth.saml.start", prov.Slug, r))

	if form != nil {
		s.writeSAMLPostForm(w, form, prov.IDPSSOURL)
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// writeSAMLPostForm sirve el formulario de autoenvío del binding POST con una CSP
// ACOTADA a esa respuesta: form-action al origen del IdP (la CSP global solo permite
// la propia suite) y el script de autoenvío por hash. El <input submit> visible es
// el fallback si el JS no corre.
func (s *Server) writeSAMLPostForm(w http.ResponseWriter, form []byte, idpSSOURL string) {
	origin := "https:"
	if u, err := url.Parse(idpSSOURL); err == nil && u.Host != "" {
		origin = u.Scheme + "://" + u.Host
	}
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src '"+samlPostScriptHash+"'; form-action "+origin+"; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	doc := `<!doctype html><html lang="es"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1">` +
		`<link rel="icon" href="/favicon.svg" type="image/svg+xml">` +
		`<link rel="apple-touch-icon" href="/apple-touch-icon.png">` +
		`<title>Redirigiendo a tu proveedor de identidad…</title></head><body>` +
		`<noscript><p>Continúa hacia el proveedor de identidad de tu organización:</p></noscript>` +
		string(form) + `</body></html>`
	_, _ = w.Write([]byte(doc))
}

// handleSAMLACS es el Assertion Consumer Service: recibe la SAML Response (POST del
// IdP), la valida con crewjam y abre la sesión SSO reaprovechando el find-or-JIT OIDC.
func (s *Server) handleSAMLACS(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := r.ParseForm(); err != nil { // crewjam lee r.PostForm; hay que parsearlo antes.
		federatedError(w, r, http.StatusBadRequest, "Solicitud inválida.")
		return
	}
	relayState := r.PostFormValue("RelayState")

	// Binding anti login-CSRF: la cookie del navegador que inició el flujo DEBE traer
	// el mismo RelayState que el POST del IdP. Sin ella (o distinta) ⇒ se corta.
	cookie, cerr := s.requestCookie(r, samlCookieName)
	s.clearSamlCookie(w)
	if relayState == "" || r.PostFormValue("SAMLResponse") == "" || cerr != nil ||
		!constTimeEqual(cookie.Value, relayState) {
		federatedError(w, r, http.StatusBadRequest, "El enlace de acceso caducó o no es válido. Vuelve a intentarlo.")
		return
	}

	// Consumir el estado (un solo uso, atómico) y validar que es de ESTE provider.
	st, err := s.store.ConsumeFederatedAuthState(r.Context(), nil, domain.HashToken(relayState), time.Now())
	if err != nil || st.ProviderID != id {
		federatedError(w, r, http.StatusBadRequest, "El enlace de acceso caducó o no es válido. Vuelve a intentarlo.")
		return
	}

	prov, ok := s.loadSAMLProvider(r, id)
	if !ok || !prov.Enabled || !prov.Configured() {
		federatedError(w, r, http.StatusNotFound, "Este inicio de sesión no está disponible.")
		return
	}
	sp, err := saml.BuildSP(prov, s.samlACSURL(id), s.samlMetadataURL(id))
	if err != nil {
		federatedError(w, r, http.StatusInternalServerError, "Configuración del proveedor inválida.")
		return
	}

	// Validación COMPLETA de la Response con crewjam: firma XML-DSig contra el cert
	// del IdP, Conditions (NotBefore/NotOnOrAfter), AudienceRestriction==SPEntityID,
	// Recipient==ACS, e InResponseTo ∈ {st.Nonce}. st.Nonce es el request-ID emitido
	// en /login, ya consumido (borrado) ⇒ un replay de la misma Response no casaría.
	claims, err := sp.Authenticate(r, []string{st.Nonce}, prov)
	if err != nil {
		_ = s.store.Audit(r.Context(), nil, auditEntry("", prov.OrgID, "auth.saml.failed", prov.Slug, r))
		federatedError(w, r, http.StatusUnauthorized, "No se pudo validar la respuesta de tu proveedor de identidad.")
		return
	}

	// Si el provider acota dominios, el email debe caer dentro (defensa anti-abuso).
	if !prov.DomainAllowed(claims.Email) {
		_ = s.store.Audit(r.Context(), nil, auditEntry("", prov.OrgID, "auth.saml.domain_rejected", prov.Slug, r))
		federatedError(w, r, http.StatusForbidden, "Tu dirección de correo no está autorizada para este acceso.")
		return
	}

	// MISMO find-or-JIT anti account-takeover que OIDC: por federated_identity → por
	// email verificado acotado a miembro de la org → JIT en la org del provider.
	userID, outcome, err := s.resolveFederatedUser(r, prov, claims)
	if err != nil {
		s.auditFederatedRefusal(r, prov, claims, err)
		federatedError(w, r, http.StatusForbidden, federatedRefusalMessage(err))
		return
	}

	// Abrir la MISMA sesión SSO que el login normal, con amr=saml.
	if err := s.createSession(w, r, userID, prov.OrgID, []string{"saml"}); err != nil {
		// mapDomainErr traduce el block del Acceso Condicional (ErrAccessDenied) a 403.
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	entry := auditEntry(userID, prov.OrgID, "auth.saml.login", prov.Slug, r)
	entry.Detail["jit"] = outcome.jit
	entry.Detail["linked"] = outcome.linked
	_ = s.store.Audit(r.Context(), nil, entry)

	// Continuar el flujo OIDC/authorize en curso (return_to) si lo hay.
	redirectAfterLogin(w, r, st.ReturnTo)
}
