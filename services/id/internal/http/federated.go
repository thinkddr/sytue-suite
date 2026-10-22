// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Login federado (Sytue ID como broker/RP OIDC). Una empresa configura su IdP
// corporativo (Google Workspace, Microsoft EntraID, cualquier OIDC) y sus usuarios
// inician sesión en la suite con él. Flujo Authorization Code + PKCE contra el IdP
// upstream:
//
//	GET /federated/{id}/start     genera state+nonce(+PKCE), guarda el estado
//	                              ligado al flujo, y redirige al authorization
//	                              endpoint del upstream con redirect_uri propio.
//	GET /federated/{id}/callback  valida state (cookie + BD, un solo uso), canjea
//	                              el código, VALIDA el id_token (firma/iss/aud/
//	                              exp/nonce), casa/aprovisiona el usuario y abre la
//	                              MISMA sesión SSO (amr=federated), continuando el
//	                              flujo OIDC en curso (return_to).
//
// El handle público del provider es su UUID (globalmente inequívoco); el slug es
// solo legible y único por-org, no sirve de identificador público. La página de
// login enruta por dominio de email hacia /federated/{id}/start.
package http

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
)

// fedCookieName es la cookie httpOnly que ata el flujo federado al navegador que
// lo inició (control anti-CSRF junto al state persistido en BD). Ámbito /federated.
const fedCookieName = "sytue_id_fed"

// fedStateTTL acota la vida del flujo federado (del /start al /callback). Corta:
// limita la ventana de un state filtrado.
const fedStateTTL = 10 * time.Minute

// federatedRedirectURI es el redirect_uri propio (registrado en el IdP upstream)
// para un provider: <issuer>/federated/{id}/callback. Estable por provider.
func (s *Server) federatedRedirectURI(providerID string) string {
	return strings.TrimRight(s.cfg.Issuer, "/") + "/federated/" + providerID + "/callback"
}

// setFedCookie / clearFedCookie gestionan la cookie de binding del flujo.
func (s *Server) setFedCookie(w http.ResponseWriter, state string, exp time.Time) {
	s.clearLegacyDomainCookie(w, fedCookieName, "/federated")
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure es config (CookieSecure); HttpOnly y SameSite=Lax fijos.
		Name: secureCookieName(fedCookieName, s.cfg.CookieSecure), Value: state, Path: "/",
		Expires: exp, HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearFedCookie(w http.ResponseWriter) {
	s.clearLegacyDomainCookie(w, fedCookieName, "/federated")
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: idem setFedCookie.
		Name: secureCookieName(fedCookieName, s.cfg.CookieSecure), Value: "", Path: "/",
		MaxAge: -1, HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

// federatedError re-usa el shell de las páginas del IdP para un fallo del flujo
// federado (con enlace de vuelta al login por contraseña, sin callejón sin salida).
func federatedError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	loc := i18n.From(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	body := alertHTML(msg) +
		`<p class="alt"><a class="btn-link" href="/login/">` + html.EscapeString(i18n.T(loc, "login.back")) + `</a></p>`
	_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "federated.title"), body)))
}

// handleFederatedStart inicia el flujo RP contra el IdP upstream del provider.
func (s *Server) handleFederatedStart(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		federatedError(w, r, http.StatusNotFound, "Proveedor de identidad desconocido.")
		return
	}
	prov, err := s.store.GetIdentityProviderByID(r.Context(), id)
	if err != nil || !prov.Enabled || !prov.Configured() {
		federatedError(w, r, http.StatusNotFound, "Este inicio de sesión no está disponible.")
		return
	}
	returnTo := safeInternalPath(r.URL.Query().Get("return_to"))

	// state + nonce (32B de entropía cada uno). El state va en cookie + URL; en BD
	// solo su hash. El nonce se guarda para validar el id_token en el callback.
	state, err1 := domain.GenerateToken(32)
	nonce, err2 := domain.GenerateToken(32)
	if err1 != nil || err2 != nil {
		federatedError(w, r, http.StatusInternalServerError, "No se pudo iniciar el flujo. Reinténtalo.")
		return
	}

	// PKCE S256 si el upstream lo anuncia (defensa ante intercepción del código).
	var verifier, challenge string
	if prov.PKCE {
		verifier, err = domain.GenerateToken(32)
		if err != nil {
			federatedError(w, r, http.StatusInternalServerError, "No se pudo iniciar el flujo. Reinténtalo.")
			return
		}
		sum := sha256.Sum256([]byte(verifier))
		challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	}

	exp := time.Now().Add(fedStateTTL)
	if err := s.store.InsertFederatedAuthState(r.Context(), nil,
		domain.HashToken(state), prov.ID, nonce, verifier, returnTo, exp); err != nil {
		federatedError(w, r, http.StatusInternalServerError, "No se pudo iniciar el flujo. Reinténtalo.")
		return
	}
	s.setFedCookie(w, state, exp)

	authURL, perr := url.Parse(prov.AuthorizationEndpoint)
	if perr != nil {
		federatedError(w, r, http.StatusInternalServerError, "Configuración del proveedor inválida.")
		return
	}
	v := authURL.Query()
	v.Set("response_type", "code")
	v.Set("client_id", prov.ClientID)
	v.Set("redirect_uri", s.federatedRedirectURI(prov.ID))
	v.Set("scope", defaultScopes(prov.Scopes))
	v.Set("state", state)
	v.Set("nonce", nonce)
	if challenge != "" {
		v.Set("code_challenge", challenge)
		v.Set("code_challenge_method", "S256")
	}
	authURL.RawQuery = v.Encode()

	_ = s.store.Audit(r.Context(), nil, auditEntry("", prov.OrgID, "auth.federated.start", prov.Slug, r))
	http.Redirect(w, r, authURL.String(), http.StatusFound)
}

// handleFederatedCallback recibe el redirect del IdP upstream, valida el flujo y
// abre la sesión SSO.
func (s *Server) handleFederatedCallback(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	q := r.URL.Query()

	// Error devuelto por el propio IdP upstream (p. ej. access_denied).
	if e := q.Get("error"); e != "" {
		s.clearFedCookie(w)
		_ = s.store.Audit(r.Context(), nil, auditEntry("", "", "auth.federated.failed", e, r))
		federatedError(w, r, http.StatusUnauthorized, "Tu proveedor de identidad rechazó el acceso.")
		return
	}
	state, code := q.Get("state"), q.Get("code")

	// Binding anti-CSRF: la cookie del navegador que inició el flujo DEBE traer el
	// mismo state que la URL. Sin ella (o distinta) ⇒ posible login-CSRF: se corta.
	cookie, cerr := s.requestCookie(r, fedCookieName)
	s.clearFedCookie(w)
	if state == "" || code == "" || cerr != nil ||
		!constTimeEqual(cookie.Value, state) {
		federatedError(w, r, http.StatusBadRequest, "El enlace de acceso caducó o no es válido. Vuelve a intentarlo.")
		return
	}

	// Consumir el estado (un solo uso, atómico) y validar que es de ESTE provider.
	st, err := s.store.ConsumeFederatedAuthState(r.Context(), nil, domain.HashToken(state), time.Now())
	if err != nil || st.ProviderID != id {
		federatedError(w, r, http.StatusBadRequest, "El enlace de acceso caducó o no es válido. Vuelve a intentarlo.")
		return
	}

	prov, err := s.store.GetIdentityProviderByID(r.Context(), id)
	if err != nil || !prov.Enabled || !prov.Configured() {
		federatedError(w, r, http.StatusNotFound, "Este inicio de sesión no está disponible.")
		return
	}

	// Canje del código + validación completa del id_token (firma/iss/aud/exp/nonce).
	claims, err := s.fed.Authenticate(r.Context(), prov, code,
		s.federatedRedirectURI(prov.ID), st.CodeVerifier, st.Nonce)
	if err != nil {
		_ = s.store.Audit(r.Context(), nil, auditEntry("", prov.OrgID, "auth.federated.failed", prov.Slug, r))
		federatedError(w, r, http.StatusUnauthorized, "No se pudo validar la respuesta de tu proveedor de identidad.")
		return
	}

	// El email (si el provider acota dominios) debe caer dentro de allowed_email_domains.
	if !prov.DomainAllowed(claims.Email) {
		_ = s.store.Audit(r.Context(), nil, auditEntry("", prov.OrgID, "auth.federated.domain_rejected", prov.Slug, r))
		federatedError(w, r, http.StatusForbidden, "Tu dirección de correo no está autorizada para este acceso.")
		return
	}

	userID, outcome, err := s.resolveFederatedUser(r, prov, claims)
	if err != nil {
		s.auditFederatedRefusal(r, prov, claims, err)
		federatedError(w, r, http.StatusForbidden, federatedRefusalMessage(err))
		return
	}

	// Abrir la MISMA sesión SSO que el login normal, con amr=federated.
	if err := s.createSession(w, r, userID, prov.OrgID, []string{"federated"}); err != nil {
		// mapDomainErr traduce el block del Acceso Condicional (ErrAccessDenied) a 403.
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	entry := auditEntry(userID, prov.OrgID, "auth.federated.login", prov.Slug, r)
	entry.Detail["jit"] = outcome.jit
	entry.Detail["linked"] = outcome.linked
	_ = s.store.Audit(r.Context(), nil, entry)

	// Continuar el flujo OIDC/authorize en curso (return_to) si lo hay.
	redirectAfterLogin(w, r, st.ReturnTo)
}

// federatedOutcome informa cómo se resolvió el usuario (para auditoría).
type federatedOutcome struct {
	jit    bool // se creó el usuario en el acto (JIT)
	linked bool // se creó el enlace federated_identities en este login
}

// Errores del find-or-provision (se traducen a un mensaje sobrio y a auditoría).
var (
	errFedEmailUnverified = errors.New("federated: email no verificado por el upstream")
	errFedCrossOrg        = errors.New("federated: el email pertenece a una cuenta de otra organización")
	errFedSuspended       = errors.New("federated: usuario suspendido")
)

// resolveFederatedUser casa el sujeto externo con un usuario Sytue o lo aprovisiona
// JIT, SIEMPRE acotado a la organización del provider (aislamiento tenant). Reglas
// anti account-takeover:
//
//  1. Enlace existente (provider, sub) ⇒ ese usuario, SI sigue activo y SIGUE siendo
//     miembro de la org (las dos cosas se revalidan: el enlace no caduca solo).
//  2. Si no, se EXIGE email verificado por el upstream. Con email verificado:
//     - usuario global que YA es miembro de la org del provider ⇒ se enlaza (seguro).
//     - usuario global que existe pero NO es miembro de la org ⇒ SE RECHAZA: un
//     upstream malicioso/mal configurado no puede secuestrar una identidad que
//     pertenece a otra organización afirmando su email.
//     - sin usuario global ⇒ JIT: alta en la org del provider (sin contraseña,
//     email verificado, atributos mapeados) y enlace.
//  3. Nunca se enlaza/aprovisiona por email NO verificado.
//
//nolint:gocognit // resolución de identidad federada: vinculación por sub/email, JIT y sus verjas de seguridad, en un solo flujo.
func (s *Server) resolveFederatedUser(r *http.Request, prov domain.IdentityProvider, claims domain.FederatedClaims) (string, federatedOutcome, error) {
	ctx := r.Context()

	// 1) Enlace existente por (provider, sub externo).
	if uid, err := s.store.GetFederatedUserID(ctx, prov.ID, claims.Subject); err == nil {
		// Fail-CLOSED al leer el usuario. Antes era `if uerr == nil && !u.Active()`, es
		// decir: si la lectura FALLABA, la comprobación de cuenta desactivada se saltaba
		// y el login seguía. Y el caso que más importa no es un fallo transitorio de BD
		// sino el usuario BORRADO: GetUserByID devuelve ErrNotFound, `uerr != nil`, y se
		// abría sesión para una cuenta que ya no existe.
		u, uerr := s.store.GetUserByID(ctx, uid)
		if uerr != nil {
			return "", federatedOutcome{}, errFedSuspended
		}
		if !u.Active() {
			return "", federatedOutcome{}, errFedSuspended
		}
		// La MEMBRESÍA se revalida en cada entrada, no se da por hecha con el enlace. El
		// enlace se creó cuando la persona era miembro, pero eso fue entonces: darla de
		// baja de la organización no borra el enlace federado, así que sin esto seguiría
		// entrando por el botón de su IdP indefinidamente. Mismo criterio que la
		// revocación de refresh tokens al quitar una membresía.
		if _, merr := s.store.GetMembership(ctx, prov.OrgID, uid); merr != nil {
			return "", federatedOutcome{}, errFedCrossOrg
		}
		return uid, federatedOutcome{}, nil
	}

	// 2) A partir de aquí se exige email VERIFICADO (nunca enlazar por email sin verificar).
	if !claims.EmailVerified || claims.Email == "" {
		return "", federatedOutcome{}, errFedEmailUnverified
	}

	if existing, err := s.store.GetUserByEmail(ctx, claims.Email); err == nil {
		// Usuario global existente: SOLO se enlaza si YA es miembro de la org del
		// provider. Si no lo es, se rechaza (anti secuestro cross-org).
		if _, merr := s.store.GetMembership(ctx, prov.OrgID, existing.ID); merr != nil {
			return "", federatedOutcome{}, errFedCrossOrg
		}
		if !existing.Active() {
			return "", federatedOutcome{}, errFedSuspended
		}
		if err := s.store.LinkFederatedIdentity(ctx, nil, prov.ID, claims.Subject, existing.ID); err != nil {
			return "", federatedOutcome{}, err
		}
		return existing.ID, federatedOutcome{linked: true}, nil
	}

	// 3) JIT: alta del usuario en la org del provider + membresía + enlace, en una tx.
	var userID string
	err := s.store.InTx(ctx, func(qx pg.Querier) error {
		u, e := s.store.CreateUser(ctx, qx, domain.User{
			Email:         claims.Email,
			EmailVerified: true, // el upstream lo asertó (email_verified=true)
			Name:          claims.Name,
			Status:        domain.StatusActive,
		})
		if e != nil {
			return e
		}
		if e := s.store.UpsertMembership(ctx, qx, domain.Membership{
			OrgID: prov.OrgID, UserID: u.ID, Role: domain.RoleMember,
		}); e != nil {
			return e
		}
		userID = u.ID
		return s.store.LinkFederatedIdentity(ctx, qx, prov.ID, claims.Subject, u.ID)
	})
	if err != nil {
		// Carrera: dos logins JIT del mismo email a la vez ⇒ unique_violation del
		// email. Re-resolver: si el usuario ya existe y es miembro, enlazar.
		if errors.Is(err, domain.ErrConflict) {
			if existing, gerr := s.store.GetUserByEmail(ctx, claims.Email); gerr == nil {
				if _, merr := s.store.GetMembership(ctx, prov.OrgID, existing.ID); merr == nil {
					if lerr := s.store.LinkFederatedIdentity(ctx, nil, prov.ID, claims.Subject, existing.ID); lerr == nil {
						return existing.ID, federatedOutcome{linked: true}, nil
					}
				}
				return "", federatedOutcome{}, errFedCrossOrg
			}
		}
		return "", federatedOutcome{}, err
	}
	return userID, federatedOutcome{jit: true, linked: true}, nil
}

func (s *Server) auditFederatedRefusal(r *http.Request, prov domain.IdentityProvider, claims domain.FederatedClaims, err error) {
	action := "auth.federated.refused"
	switch {
	case errors.Is(err, errFedEmailUnverified):
		action = "auth.federated.email_unverified"
	case errors.Is(err, errFedCrossOrg):
		action = "auth.federated.link_conflict"
	case errors.Is(err, errFedSuspended):
		action = "auth.federated.suspended"
	}
	entry := auditEntry("", prov.OrgID, action, prov.Slug, r)
	if claims.Email != "" {
		entry.Detail["email_domain"] = emailDomain(claims.Email)
	}
	_ = s.store.Audit(r.Context(), nil, entry)
}

func federatedRefusalMessage(err error) string {
	switch {
	case errors.Is(err, errFedEmailUnverified):
		return "Tu proveedor de identidad no confirmó tu correo. Contacta con tu administrador."
	case errors.Is(err, errFedCrossOrg):
		return "Ya existe una cuenta con ese correo en otra organización. Contacta con tu administrador."
	case errors.Is(err, errFedSuspended):
		return "Tu cuenta está suspendida. Contacta con tu administrador."
	default:
		return "No se pudo completar el acceso. Contacta con tu administrador."
	}
}

// --- Enrutado de login por dominio de email ("Entrar con tu organización") ---

// handleSSOForm sirve el formulario mínimo para iniciar sesión con la organización:
// el usuario teclea su email y, si su dominio casa un provider activo, se le
// redirige a /federated/{id}/start. Mejora progresiva (form sin JS).
func (s *Server) handleSSOForm(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "federated.title"),
		ssoFormHTML(loc, html.EscapeString(r.URL.Query().Get("return_to")), "", ""))))
}

func ssoFormHTML(loc, safeReturn, safeEmail, errMsg string) string {
	b := `<p class="sub">` + html.EscapeString(i18n.T(loc, "federated.subtitle")) + `</p>`
	if errMsg != "" {
		b += alertHTML(errMsg)
	}
	return b + `<form method="post" action="/login/sso">` +
		`<input type="hidden" name="return_to" value="` + safeReturn + `">` +
		`<label for="email">` + html.EscapeString(i18n.T(loc, "federated.work_email")) + `</label>` +
		`<input id="email"` + emailFieldAttrs + ` value="` + safeEmail + `" autofocus>` +
		`<button type="submit">` + html.EscapeString(i18n.T(loc, "common.continue")) + `</button></form>` +
		`<script>` + emailFieldScript + `</script>` +
		`<p class="alt"><a class="btn-link" href="/login/">` + html.EscapeString(i18n.T(loc, "federated.use_password")) + `</a></p>`
}

// handleSSOSubmit resuelve el provider por el dominio del email y redirige a su
// flujo. Sin provider para ese dominio ⇒ re-render con aviso (cae al login normal).
func (s *Server) handleSSOSubmit(w http.ResponseWriter, r *http.Request) {
	loc := i18n.From(r)
	// Mismo guard que el resto de formularios del IdP: este arranca un flujo de
	// login y no tiene por qué poder dispararlo una página de fuera.
	if !s.sameOrigin(r) {
		httpFailHTML(w, r, http.StatusForbidden, i18n.T(loc, "common.bad_origin"))
		return
	}
	if err := r.ParseForm(); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	email := domain.NormalizeEmail(r.Form.Get("email"))
	returnTo := r.Form.Get("return_to")
	dom := emailDomain(email)
	ssoFail := func(msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(loginPage(loc, i18n.T(loc, "federated.title"),
			ssoFormHTML(loc, html.EscapeString(returnTo), html.EscapeString(email), msg))))
	}
	if dom == "" {
		ssoFail("Introduce un correo válido.")
		return
	}
	prov, err := s.store.FindProviderByEmailDomain(r.Context(), dom)
	if err != nil {
		ssoFail("No hay inicio de sesión único configurado para ese dominio. Usa tu contraseña.")
		return
	}
	// El provider del dominio puede ser OIDC (flujo /federated) o SAML (flujo /saml).
	start := "/federated/" + prov.ID + "/start"
	if prov.IsSAML() {
		start = "/saml/" + prov.ID + "/login"
	}
	if rt := safeInternalPath(returnTo); rt != "" {
		start += "?" + url.Values{"return_to": {rt}}.Encode()
	}
	http.Redirect(w, r, start, http.StatusFound)
}

// --- helpers ---

// safeInternalPath admite solo rutas internas relativas (anti open-redirect); una
// URL absoluta o protocolo-relativa se descarta. Es la MISMA regla que aplica
// redirectAfterLogin: ver rutaInternaSegura.
func safeInternalPath(p string) string {
	if rutaInternaSegura(p) {
		return p
	}
	return ""
}

func emailDomain(email string) string {
	at := strings.LastIndexByte(email, '@')
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(email[at+1:])
}

func defaultScopes(scopes string) string {
	if strings.TrimSpace(scopes) == "" {
		return "openid email profile"
	}
	return scopes
}

func constTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
