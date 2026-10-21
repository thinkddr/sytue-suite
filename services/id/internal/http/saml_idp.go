// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Identidad SALIENTE por SAML 2.0 (Sytue ID como IdP). Sytue ID emite Assertions
// FIRMADAS hacia SP de terceros para SSO 1-click. Es el espejo de saml.go (Sytue ID
// como SP): toda la cripto SAML vive en internal/saml (idp.go aísla crewjam); aquí
// solo se orquesta la sesión SSO, el aislamiento por org y el transporte HTTP.
// Endpoints:
//
//	GET      /saml/idp/metadata      metadata del IdP de una org (EntityID + cert de
//	                                  firma + SSO/SLO). Pública, sin secretos. La org
//	                                  se resuelve por ?org=<slug|id>, ?sp=<id> o sesión.
//	GET+POST /saml/idp/sso           SSO SP-initiated: exige sesión de id (si no, va a
//	                                  login y REANUDA); casa el AuthnRequest con un SP
//	                                  registrado en la org de la SESIÓN (DENY-BY-DEFAULT)
//	                                  y emite la Response firmada al ACS.
//	GET      /saml/idp/initiate/{id} SSO IdP-initiated (1-click del portal): exige
//	                                  sesión; el SP debe ser de la org de la sesión y
//	                                  permitir IdP-initiated.
//	POST     /saml/idp/slo           SLO mínimo: consume el LogoutRequest, revoca la
//	                                  sesión y responde con un LogoutResponse firmado.
//
// Cada emisión se AUDITA (audit_log). Aislamiento por org DENY-BY-DEFAULT: un SP de la
// org A jamás recibe una Assertion emitida por una sesión de la org B.
package http

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"html"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
	"git.sytue.eu/sytue/one/services/id/internal/saml"
)

// idpPostScriptJS es el autoenvío del formulario POST de la Response; se autoriza en la
// CSP por hash (sin 'unsafe-inline'). Fallback sin JS: el <button> «Continuar».
const idpPostScriptJS = `document.getElementById('samlpost').submit();`

var idpPostScriptHash = func() string {
	sum := sha256.Sum256([]byte(idpPostScriptJS))
	return "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
}()

// idpEntityID / idpSSOURL / idpSLOURL / idpInitiateURL son los endpoints propios del
// IdP, derivados del Issuer público. El EntityID del IdP es ÚNICO POR ORG (== su
// metadata URL con ?org=<id>): cada tenant es su propio IdP con su propia clave.
func (s *Server) idpEntityID(orgID string) string {
	return strings.TrimRight(s.cfg.Issuer, "/") + "/saml/idp/metadata?org=" + url.QueryEscape(orgID)
}

func (s *Server) idpSSOURL() string { return strings.TrimRight(s.cfg.Issuer, "/") + "/saml/idp/sso" }

func (s *Server) idpSLOURL() string { return strings.TrimRight(s.cfg.Issuer, "/") + "/saml/idp/slo" }

func (s *Server) idpInitiateURL(spID string) string {
	return strings.TrimRight(s.cfg.Issuer, "/") + "/saml/idp/initiate/" + spID
}

// idpFor construye el IdP de una org: recupera (o genera perezosamente y sella) su
// clave de firma y la carga en crewjam vía internal/saml.
func (s *Server) idpFor(ctx context.Context, orgID string) (*saml.IDP, error) {
	priv, cert, err := s.store.GetOrCreateIDPKey(ctx, orgID, func() (string, string, error) {
		return saml.GenerateIDPKeyPEM(s.idpEntityID(orgID))
	})
	if err != nil {
		return nil, err
	}
	return saml.NewIDP(priv, cert, s.idpEntityID(orgID), s.idpSSOURL(), s.idpSLOURL())
}

// idpForExisting construye el IdP de una org SOLO si su clave YA existe, sin
// acuñarla. Devuelve (nil, domain.ErrNotFound) si la organización no tiene clave
// de IdP todavía: la metadata pública no debe generar una por una petición anónima
// (ID-02). La clave la crea un camino autenticado (registrar un SP, o el primer SSO).
func (s *Server) idpForExisting(ctx context.Context, orgID string) (*saml.IDP, error) {
	priv, cert, err := s.store.GetIDPKey(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return saml.NewIDP(priv, cert, s.idpEntityID(orgID), s.idpSSOURL(), s.idpSLOURL())
}

// resolveIDPMetadataOrg resuelve la org cuya metadata de IdP se pide: por ?org=
// (slug o UUID), por ?sp=<id> (la org del SP) o por la sesión SSO. Sin ninguna ⇒ error.
func (s *Server) resolveIDPMetadataOrg(r *http.Request) (string, error) {
	q := r.URL.Query()
	if orgParam := strings.TrimSpace(q.Get("org")); orgParam != "" {
		if _, err := uuid.Parse(orgParam); err == nil {
			if org, err := s.store.GetOrgByID(r.Context(), orgParam); err == nil {
				return org.ID, nil
			}
			return "", domain.ErrNotFound
		}
		org, err := s.store.GetOrgBySlug(r.Context(), orgParam)
		if err != nil {
			return "", err
		}
		return org.ID, nil
	}
	if spID := strings.TrimSpace(q.Get("sp")); spID != "" {
		if _, err := uuid.Parse(spID); err != nil {
			return "", domain.ErrNotFound
		}
		sp, err := s.store.GetSAMLServiceProviderByID(r.Context(), spID)
		if err != nil {
			return "", err
		}
		return sp.OrgID, nil
	}
	if sess, ok := s.currentSession(r); ok && sess.OrgID != "" {
		return sess.OrgID, nil
	}
	return "", domain.ErrNotFound
}

// handleSAMLIdPMetadata publica la metadata del IdP de una org (EntityID, cert de firma,
// SSO/SLO). Pública, sin secretos: el admin del SP de terceros la registra en su lado.
func (s *Server) handleSAMLIdPMetadata(w http.ResponseWriter, r *http.Request) {
	orgID, err := s.resolveIDPMetadataOrg(r)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// Primero get-only: la metadata no acuña una clave por una petición anónima
	// cualquiera (ID-02). Si la organización todavía no tiene clave, SOLO se genera
	// cuando ya registró algún SP —un acto autenticado de su administrador
	// (apps.manage)—: es el orden natural (registrar el SP y después descargar esta
	// metadata) y acota la generación a las organizaciones que de verdad usan SAML
	// saliente. Una organización sin ningún SP responde 404 y no se le crea nada,
	// así que probar slugs ya no fuerza claves RSA ni filas en saml_idp_keys.
	idp, err := s.idpForExisting(r.Context(), orgID)
	if errors.Is(err, domain.ErrNotFound) {
		sps, lerr := s.store.ListSAMLServiceProviders(r.Context(), orgID)
		if lerr != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(lerr))
			return
		}
		if len(sps) == 0 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		idp, err = s.idpFor(r.Context(), orgID)
	}
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	md, err := idp.MetadataXML()
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="sytue-idp-metadata.xml"`)
	_, _ = w.Write(md)
}

// handleSAMLIdPSSO atiende el SSO SP-initiated: decodifica el AuthnRequest, exige sesión
// SSO (si no hay, va a login y REANUDA el flujo), casa el SP con un registro de la org
// de la sesión y emite la Response firmada.
func (s *Server) handleSAMLIdPSSO(w http.ResponseWriter, r *http.Request) {
	info, err := saml.DecodeAuthnRequest(r)
	if err != nil {
		federatedError(w, r, http.StatusBadRequest, "La solicitud de inicio de sesión no es válida.")
		return
	}

	sess, ok := s.currentSession(r)
	if !ok {
		// Reanudar tras login: re-presenta el AuthnRequest como un GET redirect-binding
		// (normaliza POST→GET, pues el cuerpo POST se pierde en el rebote al login).
		enc, e := saml.DeflateBase64(info.Raw)
		if e != nil {
			federatedError(w, r, http.StatusInternalServerError, "No se pudo iniciar el flujo. Reinténtalo.")
			return
		}
		resume := "/saml/idp/sso?" + url.Values{"SAMLRequest": {enc}, "RelayState": {info.RelayState}}.Encode()
		http.Redirect(w, r, "/login/?"+url.Values{"return_to": {resume}}.Encode(), http.StatusFound)
		return
	}

	// DENY-BY-DEFAULT: el SP DEBE estar registrado y habilitado en la org de la SESIÓN.
	// Un SP de otra org no casa aquí (aislamiento tenant, sin fuga cross-org).
	sp, err := s.store.GetSAMLServiceProviderByEntityID(r.Context(), sess.OrgID, info.SPEntityID)
	if err != nil || !sp.Enabled || !sp.Configured() {
		_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "saml.idp.sso.denied", info.SPEntityID, r))
		federatedError(w, r, http.StatusForbidden, "Esta aplicación no está autorizada para tu organización.")
		return
	}
	// El ACS solicitado (si el AuthnRequest lo trae) DEBE casar el registrado: la
	// Assertion solo se envía al ACS registrado (anti redirección de la Assertion).
	if info.ACSURL != "" && info.ACSURL != sp.ACSURL {
		federatedError(w, r, http.StatusBadRequest, "La URL de retorno de la aplicación no coincide con la registrada.")
		return
	}

	form, err := s.emitAssertion(r, sess, sp, info.RequestID, info.RelayState)
	if err != nil {
		federatedError(w, r, http.StatusInternalServerError, "No se pudo completar el inicio de sesión. Reinténtalo.")
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "saml.idp.sso.issued", sp.EntityID, r))
	s.writeSAMLResponseForm(w, form)
}

// handleSAMLIdPInitiate atiende el SSO IdP-initiated (1-click del portal): exige sesión
// y que el SP sea de la org de la sesión y permita IdP-initiated.
func (s *Server) handleSAMLIdPInitiate(w http.ResponseWriter, r *http.Request) {
	spID := chi.URLParam(r, "spID")
	if _, err := uuid.Parse(spID); err != nil {
		federatedError(w, r, http.StatusNotFound, "Aplicación desconocida.")
		return
	}
	sess, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login/?"+url.Values{"return_to": {r.URL.RequestURI()}}.Encode(), http.StatusFound)
		return
	}
	sp, err := s.store.GetSAMLServiceProviderByID(r.Context(), spID)
	// Aislamiento tenant: solo la org de la sesión, habilitado y con IdP-initiated.
	if err != nil || sp.OrgID != sess.OrgID || !sp.Enabled || !sp.Configured() || !sp.IDPInitiated {
		_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "saml.idp.initiate.denied", spID, r))
		federatedError(w, r, http.StatusNotFound, "Esta aplicación no está disponible.")
		return
	}
	relayState := r.URL.Query().Get("RelayState")
	if relayState == "" {
		relayState = sp.RelayStateDefault
	}
	form, err := s.emitAssertion(r, sess, sp, "", relayState) // unsolicited (InResponseTo vacío)
	if err != nil {
		federatedError(w, r, http.StatusInternalServerError, "No se pudo completar el inicio de sesión. Reinténtalo.")
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "saml.idp.initiate.issued", sp.EntityID, r))
	s.writeSAMLResponseForm(w, form)
}

// handleSAMLIdPSLO atiende el Single Logout (mínimo): consume el LogoutRequest, revoca
// la sesión SSO y responde con un LogoutResponse firmado hacia el SP.
func (s *Server) handleSAMLIdPSLO(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	info, err := saml.DecodeLogoutRequest(r)
	if err != nil {
		federatedError(w, r, http.StatusBadRequest, "La solicitud de cierre de sesión no es válida.")
		return
	}
	sess, ok := s.currentSession(r)
	s.clearSessionCookie(w)
	if !ok {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(loginPage(loc, "Sesión cerrada", `<p class="sub">Tu sesión ya estaba cerrada.</p>`)))
		return
	}
	_ = s.store.RevokeSession(r.Context(), nil, sess.TokenHash)
	sp, err := s.store.GetSAMLServiceProviderByEntityID(r.Context(), sess.OrgID, info.SPEntityID)
	if err != nil || !sp.Configured() {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(loginPage(loc, "Sesión cerrada", `<p class="sub">Tu sesión se ha cerrado.</p>`)))
		return
	}
	idp, err := s.idpFor(r.Context(), sess.OrgID)
	if err != nil {
		federatedError(w, r, http.StatusInternalServerError, "No se pudo completar el cierre de sesión.")
		return
	}
	form, err := idp.MakeLogoutResponse(sp, info.RequestID, r.PostFormValue("RelayState"))
	if err != nil {
		federatedError(w, r, http.StatusInternalServerError, "No se pudo completar el cierre de sesión.")
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "saml.idp.slo", sp.EntityID, r))
	s.writeSAMLResponseForm(w, form)
}

// emitAssertion construye la Response firmada para un SP a partir del sujeto de la
// sesión (usuario + grupos de la org). inResponseTo vacío ⇒ IdP-initiated.
func (s *Server) emitAssertion(r *http.Request, sess domain.Session, sp domain.SAMLServiceProvider, inResponseTo, relayState string) (saml.ResponseForm, error) {
	ctx := r.Context()
	u, err := s.store.GetUserByID(ctx, sess.UserID)
	if err != nil {
		return saml.ResponseForm{}, err
	}
	// Grupos del usuario en la org de la sesión → atributo `groups` (best-effort).
	groups, _ := s.store.GroupsForUser(ctx, sess.OrgID, sess.UserID)
	idp, err := s.idpFor(ctx, sess.OrgID)
	if err != nil {
		return saml.ResponseForm{}, err
	}
	return idp.MakeResponse(r, sp, inResponseTo, relayState, saml.UserIdentity{
		StableID: u.ID,
		Email:    u.Email,
		Name:     u.Name,
		Groups:   groups,
	})
}

// writeSAMLResponseForm renderiza el formulario POST autoenviable hacia el ACS del SP
// con una CSP ACOTADA (form-action al origen del SP; script de autoenvío por hash).
func (s *Server) writeSAMLResponseForm(w http.ResponseWriter, form saml.ResponseForm) {
	origin := "https:"
	if u, err := url.Parse(form.ACSURL); err == nil && u.Host != "" {
		origin = u.Scheme + "://" + u.Host
	}
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src '"+idpPostScriptHash+"'; form-action "+origin+"; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	doc := `<!doctype html><html lang="es"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1">` +
		`<link rel="icon" href="/favicon.svg" type="image/svg+xml">` +
		`<link rel="apple-touch-icon" href="/apple-touch-icon.png">` +
		`<title>Redirigiendo a la aplicación…</title></head><body>` +
		`<noscript><p>Continúa hacia la aplicación:</p></noscript>` +
		`<form method="post" action="` + html.EscapeString(form.ACSURL) + `" id="samlpost">` +
		`<input type="hidden" name="SAMLResponse" value="` + html.EscapeString(form.SAMLResponse) + `">` +
		`<input type="hidden" name="RelayState" value="` + html.EscapeString(form.RelayState) + `">` +
		`<noscript><button type="submit">Continuar</button></noscript>` +
		`</form><script>` + idpPostScriptJS + `</script></body></html>`
	_, _ = w.Write([]byte(doc))
}
