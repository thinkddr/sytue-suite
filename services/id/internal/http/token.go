// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

// tokenResponse es la respuesta del token endpoint (RFC 6749 §5.1).
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
	// IssuedTokenType es obligatorio en la respuesta del canje RFC 8693 §2.2.1
	// (dice QUÉ se ha emitido); se omite en el resto de grants.
	IssuedTokenType string `json:"issued_token_type,omitempty"`
}

// tokenError escribe un error OAuth2 del token endpoint (RFC 6749 §5.2).
func tokenError(w http.ResponseWriter, r *http.Request, status int, code, desc string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	httpx.JSON(w, r, status, map[string]string{"error": code, "error_description": desc})
}

// handleToken implementa el token endpoint para los grants soportados.
func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		tokenError(w, r, http.StatusBadRequest, "invalid_request", "formulario inválido")
		return
	}
	grant := r.Form.Get("grant_type")

	// Bloqueo de emergencia (mig 0035): el otro punto de EMISIÓN. `revoke-all`
	// invalida lo ya emitido rotando la clave de firma; esto impide emitir de
	// nuevo mientras dure el corte —códigos de autorización, refresh y
	// client_credentials incluidos—. 503 y no 400: es un estado del servicio,
	// no un defecto de la petición, y el cliente no arregla nada reintentando
	// con otros parámetros. Un error de consulta DENIEGA (fail-closed).
	if cerrado, err := s.store.IsLockedDown(r.Context()); err != nil || cerrado {
		if err != nil {
			s.log.Error("no se pudo comprobar el bloqueo de emergencia", "error", err)
		}
		tokenError(w, r, http.StatusServiceUnavailable, "temporarily_unavailable",
			"emisión de tokens suspendida por bloqueo de emergencia")
		return
	}

	client, ok := s.authenticateClient(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="sytue-id"`)
		tokenError(w, r, http.StatusUnauthorized, "invalid_client", "autenticación de client fallida")
		return
	}

	switch grant {
	case "authorization_code":
		s.grantAuthCode(w, r, client)
	case "refresh_token":
		s.grantRefresh(w, r, client)
	case "client_credentials":
		s.grantClientCredentials(w, r, client)
	case domain.GrantTokenExchange:
		s.grantTokenExchange(w, r, client)
	default:
		tokenError(w, r, http.StatusBadRequest, "unsupported_grant_type", "grant_type no soportado")
	}
}

// grantAuthCode canjea un código de autorización por tokens (con PKCE).
func (s *Server) grantAuthCode(w http.ResponseWriter, r *http.Request, client domain.Client) {
	if !client.AllowsGrant("authorization_code") {
		tokenError(w, r, http.StatusBadRequest, "unauthorized_client", "")
		return
	}
	code := r.Form.Get("code")
	redirectURI := r.Form.Get("redirect_uri")
	verifier := r.Form.Get("code_verifier")
	secretoDispositivo := r.Form.Get(paramSecretoDispositivo)
	if code == "" {
		tokenError(w, r, http.StatusBadRequest, "invalid_request", "falta code")
		return
	}

	now := time.Now()
	var (
		ac         domain.AuthCode
		user       domain.User
		refreshRaw string
	)
	ctx := r.Context()
	// Todo el canje (consumir el código, cargar el usuario, validar PKCE y
	// persistir el refresh token) ocurre en UNA transacción: es atómico. Si la
	// inserción del refresh falla, el código no queda gastado (rollback).
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		return s.redeemAuthCode(ctx, q, client, code, redirectURI, verifier, secretoDispositivo, now, &ac, &user, &refreshRaw)
	})
	switch {
	case err == nil:
		// continúa abajo
	case errors.Is(err, errWrongClient):
		tokenError(w, r, http.StatusBadRequest, "invalid_grant", "el código no pertenece al client")
		return
	case errors.Is(err, errRedirectMismatch):
		tokenError(w, r, http.StatusBadRequest, "invalid_grant", "redirect_uri no coincide")
		return
	case errors.Is(err, errPKCEInvalid):
		tokenError(w, r, http.StatusBadRequest, "invalid_grant", "code_verifier inválido (PKCE)")
		return
	case errors.Is(err, errPKCERequired):
		tokenError(w, r, http.StatusBadRequest, "invalid_grant", "PKCE requerido")
		return
	case errors.Is(err, errDispositivo):
		tokenError(w, r, http.StatusBadRequest, "invalid_grant", "el código es de otro dispositivo")
		return
	case errors.Is(err, domain.ErrNotFound):
		tokenError(w, r, http.StatusBadRequest, "invalid_grant", "código inválido, expirado o ya usado")
		return
	default:
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}

	// El `amr` sale de la SESIÓN que respalda el código, no de una constante. Antes
	// era «pwd» fijo: daba igual que la persona hubiera pasado por TOTP o por una
	// passkey, el token afirmaba que solo hubo contraseña. Como `amr` es
	// precisamente la claim con la que un relying party decide si exige un segundo
	// paso, eso volvía insatisfacible toda política de MFA —la consola de
	// plataforma respondía 403 al admin global por su propio token—.
	amr, err := s.store.SessionAMR(ctx, ac.SessionID)
	if err != nil {
		// Sin `amr` antes que con uno inventado: el que decide es el consumidor.
		s.log.WarnContext(ctx, "no se pudo leer el amr de la sesión del código",
			"session_id", ac.SessionID, "error", err)
		amr = nil
	}

	resp := s.buildTokens(ctx, client, user, ac.OrgID, ac.Scope, ac.Nonce, ac.AuthTime, amr)
	if resp == nil {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	resp.RefreshToken = refreshRaw
	_ = s.store.Audit(ctx, nil, auditEntry(user.ID, ac.OrgID, "oauth.token.issued", client.ID, r))
	writeTokens(w, r, resp)
}

// Errores centinela internos del canje de código (no viajan al cliente; se
// traducen a respuestas OAuth2 en grantAuthCode).
var (
	errWrongClient      = errors.New("auth code: client no coincide")
	errRedirectMismatch = errors.New("auth code: redirect_uri no coincide")
	errPKCEInvalid      = errors.New("auth code: PKCE inválido")
	errPKCERequired     = errors.New("auth code: PKCE requerido")
	errDispositivo      = errors.New("auth code: secreto de dispositivo ausente o ajeno")
)

// redeemAuthCode ejecuta, dentro de la transacción q, el canje atómico del código
// de autorización: lo consume, valida client/redirect_uri/PKCE, carga el usuario
// y, si procede, persiste el refresh token ligado a la sesión SSO. Escribe los
// resultados en ac/user/refreshRaw. Devuelve errores centinela que grantAuthCode
// traduce a respuestas OAuth2.
func (s *Server) redeemAuthCode(ctx context.Context, q pg.Querier, client domain.Client,
	code, redirectURI, verifier, secretoDispositivo string, now time.Time,
	ac *domain.AuthCode, user *domain.User, refreshRaw *string,
) error {
	got, err := s.store.ConsumeAuthCode(ctx, q, domain.HashToken(code), now)
	if err != nil {
		return err
	}
	*ac = got
	// El código pertenece a este client y al redirect_uri usado en /authorize.
	if ac.ClientID != client.ID {
		return errWrongClient
	}
	// Defensa en profundidad del binding por organización de /authorize (NEG-05): el
	// código lleva la organización de la sesión que lo emitió, y una app acotada a una
	// organización no canjea un código de otra. Tras el arreglo de handleAuthorize un
	// código legítimo nunca llega aquí con esta discrepancia; esto cubre un código
	// emitido antes del despliegue o por cualquier otra vía.
	if !client.UsableByOrg(ac.OrgID) {
		return errWrongClient
	}
	if ac.RedirectURI != redirectURI {
		return errRedirectMismatch
	}
	// Verificación PKCE (RFC 7636). Obligatoria para públicos SIEMPRE y para
	// cualquier client que lo exija (defensa en profundidad frente a un authorize
	// que hubiera emitido el código sin challenge).
	if ac.CodeChallenge != "" {
		if !domain.VerifyPKCE(ac.CodeChallengeMethod, ac.CodeChallenge, verifier) {
			return errPKCEInvalid
		}
	} else if client.RequirePKCE || client.IsPublic() {
		return errPKCERequired
	}
	// NAT-02: un código atado a un dispositivo solo lo canjea quien tiene su secreto, la
	// misma prueba que PKCE (el hash viajó a /authorize; el secreto, solo aquí).
	if ac.Dispositivo != "" && !domain.VerifyPKCE("S256", ac.Dispositivo, secretoDispositivo) {
		return errDispositivo
	}
	u, err := s.store.GetUserByID(ctx, ac.UserID)
	if err != nil {
		return err
	}
	// La cuenta debe estar ACTIVA para canjear el código, igual que en grantRefresh:
	// era la asimetría por la que un usuario dado de baja seguía obteniendo tokens
	// frescos por /authorize mientras su sesión SSO viviera. Segunda línea de la verja
	// de currentSession (una comprobación por si el código se obtuvo antes de la baja).
	if !u.Active() {
		return domain.ErrInvalidGrant
	}
	if ac.OrgID != "" && !s.orgEntitled(ctx, ac.OrgID) {
		return domain.ErrInvalidGrant
	}
	*user = u
	// Emitir y persistir el refresh token DENTRO de la misma transacción,
	// ligándolo a la sesión SSO (SessionID) para la revocación en cascada.
	if client.AllowsGrant("refresh_token") && containsScope(ac.Scope, "offline_access") {
		raw, err := domain.GenerateToken(32)
		if err != nil {
			return err
		}
		if _, err := s.store.InsertRefreshToken(ctx, q, domain.RefreshToken{
			TokenHash: domain.HashToken(raw),
			ClientID:  client.ID,
			UserID:    u.ID,
			OrgID:     ac.OrgID,
			SessionID: ac.SessionID,
			Scope:     ac.Scope,
			AuthTime:  ac.AuthTime,
			ExpiresAt: now.Add(s.cfg.RefreshTokenTTL),
		}); err != nil {
			return err
		}
		*refreshRaw = raw
	}
	return nil
}

// refreshRotation es lo que produce la transacción del canje: el usuario y los
// datos del nuevo token, más los dos veredictos que deciden la respuesta HTTP.
type refreshRotation struct {
	user     domain.User
	newRaw   string
	newScope string
	authTime time.Time
	orgID    string
	// sessionID: de dónde se lee el `amr` real al refrescar.
	sessionID string
	// reuse: el token presentado ya estaba revocado/rotado (familia revocada).
	reuse bool
	// ok: la rotación se completó y hay token nuevo que entregar.
	ok bool
}

// validateRefreshOrg vuelve a comprobar las tres condiciones que mantienen un
// refresh token ligado a una organización: membresía, organización activa y
// suscripción pagada. Se mantiene fuera de rotateRefresh para que la transacción
// de rotación conserve una complejidad auditable.
func (s *Server) validateRefreshOrg(ctx context.Context, rt domain.RefreshToken) error {
	if rt.OrgID == "" {
		return nil
	}
	if _, err := s.store.GetMembership(ctx, rt.OrgID, rt.UserID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrInvalidGrant
		}
		return err
	}
	org, err := s.store.GetOrgByID(ctx, rt.OrgID)
	if err != nil {
		return err
	}
	if org.Status != domain.StatusActive || !s.orgEntitled(ctx, rt.OrgID) {
		return domain.ErrInvalidGrant
	}
	return nil
}

// rotateRefresh valida el refresh token presentado y lo rota, todo en una
// transacción. Separado de grantRefresh solo para que cada uno se lea entero:
// aquí las reglas del canje, allí la traducción a respuesta OAuth.
func (s *Server) rotateRefresh(ctx context.Context, client domain.Client, raw string, now time.Time) (refreshRotation, error) {
	var out refreshRotation
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		rt, err := s.store.GetRefreshToken(ctx, q, domain.HashToken(raw))
		if err != nil {
			return err
		}
		if rt.ClientID != client.ID {
			return domain.ErrInvalidGrant
		}
		// La atadura por organización de /authorize (NEG-05) vale también al
		// refrescar: un refresh token emitido ANTES de ese arreglo para una sesión de
		// otra organización seguiría rotando —y dando access tokens con el tenant
		// ajeno— hasta treinta días si aquí no se mirase.
		if !client.UsableByOrg(rt.OrgID) {
			return domain.ErrInvalidGrant
		}
		// Detección de reuso: si ya estaba revocado/rotado, revocar toda la familia.
		if rt.RevokedAt != nil || rt.RotatedTo != "" {
			out.reuse = true
			return s.store.RevokeRefreshChain(ctx, q, rt.ID)
		}
		if now.After(rt.ExpiresAt) {
			return domain.ErrExpired
		}
		out.user, err = s.store.GetUserByID(ctx, rt.UserID)
		if err != nil {
			return err
		}
		out.authTime = rt.AuthTime
		out.orgID = rt.OrgID
		out.sessionID = rt.SessionID
		out.newScope = rt.Scope

		// La membresía se REVALIDA en cada canje: el `org_id` viaja en el refresh token
		// y userClaims resuelve el `tenant` por GetOrgByID, que solo comprueba que la
		// organización exista. Sin esta verja, a quien se daba de baja de una org se le
		// seguía emitiendo un token con el claim `tenant` de esa org —y toda la suite
		// deriva de ahí el aislamiento—, así que la baja no surtía efecto hasta que
		// caducara el refresh. RemoveMembership revoca además estos tokens; esto lo hace
		// robusto aunque esa limpieza falle o los datos deriven.
		if err := s.validateRefreshOrg(ctx, rt); err != nil {
			return err
		}

		// Emitir el nuevo refresh token y enlazar la rotación.
		out.newRaw, err = domain.GenerateToken(32)
		if err != nil {
			return err
		}
		newID, err := s.store.InsertRefreshToken(ctx, q, domain.RefreshToken{
			TokenHash: domain.HashToken(out.newRaw),
			ClientID:  client.ID,
			UserID:    rt.UserID,
			OrgID:     rt.OrgID,
			SessionID: rt.SessionID,
			Scope:     rt.Scope,
			AuthTime:  rt.AuthTime,
			ExpiresAt: now.Add(s.cfg.RefreshTokenTTL),
		})
		if err != nil {
			return err
		}
		out.ok = true
		return s.store.RotateRefreshToken(ctx, q, rt.ID, newID)
	})
	return out, err
}

// grantRefresh canjea un refresh token (con rotación y detección de reuso).
func (s *Server) grantRefresh(w http.ResponseWriter, r *http.Request, client domain.Client) {
	if !client.AllowsGrant("refresh_token") {
		tokenError(w, r, http.StatusBadRequest, "unauthorized_client", "")
		return
	}
	raw := r.Form.Get("refresh_token")
	if raw == "" {
		tokenError(w, r, http.StatusBadRequest, "invalid_request", "falta refresh_token")
		return
	}
	ctx := r.Context()
	rot, err := s.rotateRefresh(ctx, client, raw, time.Now())
	if err != nil && !rot.reuse {
		if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrInvalidGrant) || errors.Is(err, domain.ErrExpired) {
			tokenError(w, r, http.StatusBadRequest, "invalid_grant", "refresh token inválido o expirado")
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if rot.reuse {
		_ = s.store.Audit(ctx, nil, auditEntry("", "", "oauth.refresh.reuse_detected", client.ID, r))
		tokenError(w, r, http.StatusBadRequest, "invalid_grant", "reutilización de refresh token detectada")
		return
	}
	if !rot.ok || !rot.user.Active() {
		tokenError(w, r, http.StatusBadRequest, "invalid_grant", "usuario no activo")
		return
	}

	// Ídem al refrescar: el segundo factor no se «pierde» porque caduque un access
	// token de quince minutos. Se conserva el de la sesión que sigue viva.
	amrRot, err := s.store.SessionAMR(ctx, rot.sessionID)
	if err != nil {
		amrRot = nil
	}
	resp := s.buildTokens(ctx, client, rot.user, rot.orgID, rot.newScope, "", rot.authTime, amrRot)
	if resp == nil {
		// buildTokens devuelve nil si la firma falla (clave activa ilegible, por
		// ejemplo). Aquí faltaba la comprobación que sí hace grantAuthCode, así que
		// ese fallo no daba un 500 sino un desreferenciado de nil en la línea de
		// abajo: cada refresco tumbaba su goroutine, y el refresco es el camino que
		// recorren TODAS las sesiones de la suite cada quince minutos.
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	resp.RefreshToken = rot.newRaw
	writeTokens(w, r, resp)
}

// grantClientCredentials emite un access token para máquina-a-máquina (RFC 6749 §4.4).
func (s *Server) grantClientCredentials(w http.ResponseWriter, r *http.Request, client domain.Client) {
	if !client.AllowsGrant("client_credentials") {
		tokenError(w, r, http.StatusBadRequest, "unauthorized_client", "")
		return
	}
	if client.IsPublic() {
		tokenError(w, r, http.StatusBadRequest, "invalid_client", "client público no permitido para client_credentials")
		return
	}
	if client.OrgID != "" && !s.orgEntitled(r.Context(), client.OrgID) {
		tokenError(w, r, http.StatusBadRequest, "unauthorized_client", "suscripción no activa")
		return
	}
	scope := strings.TrimSpace(r.Form.Get("scope"))
	for _, sc := range strings.Fields(scope) {
		if !client.AllowsScope(sc) {
			tokenError(w, r, http.StatusBadRequest, "invalid_scope", "scope no permitido: "+sc)
			return
		}
	}
	now := time.Now()
	at, err := s.issuer.IssueAccessToken(oidc.TokenClaims{
		Subject:  client.ID, // sub = client_id en client_credentials
		Audience: client.ID,
		Scope:    scope,
	}, now)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry("", "", "oauth.client_credentials", client.ID, r))
	writeTokens(w, r, &tokenResponse{
		AccessToken: at,
		TokenType:   "Bearer",
		ExpiresIn:   int(s.issuer.AccessTokenTTL().Seconds()),
		Scope:       scope,
	})
}

// buildTokens construye access + id token (sin persistir refresh).
func (s *Server) buildTokens(ctx context.Context, client domain.Client, user domain.User, orgID, scope, nonce string, authTime time.Time, amr []string) *tokenResponse {
	now := time.Now()
	common := s.userClaims(ctx, client, user, orgID, scope, now)
	common.Nonce = nonce
	common.AuthTime = authTime
	common.AMR = amr

	at, err := s.issuer.IssueAccessToken(common, now)
	if err != nil {
		return nil
	}
	resp := &tokenResponse{
		AccessToken: at,
		TokenType:   "Bearer",
		ExpiresIn:   int(s.issuer.AccessTokenTTL().Seconds()),
		Scope:       scope,
	}
	if containsScope(scope, "openid") {
		idt, err := s.issuer.IssueIDToken(common, now)
		if err != nil {
			return nil
		}
		resp.IDToken = idt
	}
	return resp
}

// userClaims compone los claims de IDENTIDAD del usuario en la org activa
// (tenant, roles, permisos efectivos, grupos) más su perfil. Es la base común de
// la emisión normal (buildTokens) y del canje RFC 8693, que le añade `act` y un
// TTL corto: así un token de impersonación lleva EXACTAMENTE la misma identidad
// que llevaría el usuario, ni más ni menos.
func (s *Server) userClaims(ctx context.Context, client domain.Client, user domain.User, orgID, scope string, now time.Time) oidc.TokenClaims {
	var tenant, mailbox string
	var roles, perms, groups, mailDomains []string
	var orgLogo, orgColor, orgForma string
	var orgKind, plan string
	var storageBytes, mailboxBytes int64
	if orgID != "" {
		if org, err := s.store.GetOrgByID(ctx, orgID); err == nil {
			tenant = org.Slug
			orgKind = string(org.Kind)
			if orgKind == "" {
				orgKind = string(domain.OrgKindOrg)
			}
			orgColor = org.ColorClaim()
			orgForma = org.FormaClaim()
		}
		// El contrato y sus cuotas viajan en el token para que cada servicio las
		// aplique sin preguntar: Drive fija su cuota (el pool de la organización),
		// Mail la del buzón. En una instalación AUTOALOJADA las organizaciones no
		// llevan cuota de plan —nadie les vende asientos; la pone el servicio—,
		// pero las cuentas personales sí, porque su plan es lo único que las acota.
		if sub, err := s.store.GetOrgSubscription(ctx, orgID); err == nil {
			plan = string(sub.EffectivePlan(now))
			if sub.Personal || !s.cfg.SelfHosted {
				storageBytes = sub.StorageBytes(now)
				mailboxBytes = sub.MailboxBytes(now)
			}
		}
		// Dirección de correo del usuario en esta organización. EnsureOrgMailbox es
		// idempotente y en el caso normal es una sola lectura por índice: la
		// dirección ya se asignó al darlo de alta. Que además ASIGNE si falta es la
		// red de seguridad de las membresías creadas fuera del servidor (los
		// subcomandos de bootstrap y seed no cargan configuración y no reparten
		// buzones): así nadie se queda sin dirección de forma permanente y muda.
		// Best-effort como `groups`: un fallo aquí no puede tumbar el login.
		if mb, err := s.store.EnsureOrgMailbox(ctx, nil, orgID, user.ID, ""); err == nil {
			mailbox = mb
		}
		// Los dominios propios verificados de la organización → claim `mail_domains`.
		// Sin ellos, Sytue Mail solo reconoce como suyo el dominio de la dirección
		// con la que se presenta cada cual, y rechaza como ajena una dirección en el
		// SEGUNDO dominio de la organización (mig 0052). Best-effort como `groups`:
		// un fallo aquí no puede tumbar el login.
		if ds, err := s.store.VerifiedOrgMailDomains(ctx, nil, orgID); err == nil {
			mailDomains = ds
		}
		if rs, err := s.store.RolesForUserInOrg(ctx, orgID, user.ID); err == nil {
			roles = rs
		}
		// Permisos granulares EFECTIVOS del usuario en el tenant activo → claim `perms`.
		// Unión de (a) roles personalizados ASIGNADOS de forma permanente (admin delegada,
		// mig 0019) y (b) los que conceden sus activaciones PIM ACTIVAS ahora (JIT, mig
		// 0020): la elegibilidad sin activar no concede, y una activación caducada deja de
		// conceder al re-mintar (expiración perezosa evaluada contra `now`). Best-effort
		// como `groups`: un fallo no tumba la emisión (a lo sumo se omiten los de PIM).
		var direct []string
		if ps, err := s.store.PermissionsForUserInOrg(ctx, orgID, user.ID); err == nil {
			direct = ps
		}
		acts, _ := s.store.ActivationPermsForUser(ctx, orgID, user.ID, now)
		perms = domain.EffectivePerms(direct, acts, now)
		// Nombres de los grupos del usuario en el tenant activo → claim `groups`
		// (share por-grupo de Drive). Best-effort: un fallo aquí no debe tumbar la
		// emisión del token (el claim simplemente se omite).
		if gs, err := s.store.GroupsForUser(ctx, orgID, user.ID); err == nil {
			groups = gs
		}
		// Hash del logo propio de la organización → claim `org_logo`, con el que la
		// barra superior compone la URL del logo y la pide como inmutable. Solo el
		// hash: el binario no tiene nada que hacer en un token. Best-effort como los
		// dos de arriba —sin él se pinta la marca de la casa, que es lo que ya pasa
		// con las organizaciones que no han subido ninguno—.
		orgLogo, _ = s.store.HashDeLogo(ctx, nil, orgID)
	}
	// Versión del avatar propio → claim `picture` del ID token. Best-effort:
	// sin avatar el claim se omite y las apps pintan las iniciales, que es el
	// comportamiento correcto para quien no ha subido foto. Mismo formato que
	// el ETag del byte-serving endpoint (base36 del nanosecond).
	// Sus direcciones en todas sus organizaciones → claim `mailboxes` (ADR 0053), SOLO
	// para los clients de la suite (sin organización: first-party). El dominio de cada
	// dirección dice en qué otra organización está, y eso no se le cuenta a una app de
	// terceros ni a una de una organización (ADR 0040). Best-effort como `groups`: sin
	// ellas, la campana enseña los de esta.
	var mailboxes []string
	if client.OrgID == "" && clientesConCampana[client.ID] {
		mailboxes, _ = s.store.BuzonesDeLaPersona(ctx, user.ID)
	}

	var avatarVersion string
	if tiene, ver, err := s.store.HasAvatar(ctx, user.ID); err == nil && tiene {
		avatarVersion = versionAvatar(ver)
	}

	// Cuenta de Google emulada: con scopes de Google el token abre además las rutas
	// /compat/google de los productos, y el ID token lleva `hd` como el de Google.
	var extra []string
	if domain.HasGoogleAPIScope(scope) {
		extra = []string{domain.GoogleAPIsAudience}
	}
	hd := ""
	if orgKind == string(domain.OrgKindOrg) && len(mailDomains) > 0 {
		hd = mailDomains[0]
	}

	return oidc.TokenClaims{
		ExtraAudience: extra,
		HostedDomain:  hd,
		Subject:       user.ID,
		Audience:      client.ID,
		Scope:         scope,
		Tenant:        tenant,
		Roles:         roles,
		Perms:         perms,
		Groups:        groups,
		OrgLogo:       orgLogo,
		OrgColor:      orgColor,
		OrgForma:      orgForma,
		AvatarVersion: avatarVersion,
		Email:         user.Email,
		EmailVerified: user.EmailVerified,
		Name:          user.Name,
		Mailbox:       mailbox,
		Mailboxes:     mailboxes,
		MailDomains:   mailDomains,
		OrgKind:       orgKind,
		Plan:          plan,
		StorageBytes:  storageBytes,
		MailboxBytes:  mailboxBytes,
	}
}

func writeTokens(w http.ResponseWriter, r *http.Request, resp *tokenResponse) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	httpx.JSON(w, r, http.StatusOK, resp)
}

func containsScope(scope, want string) bool {
	for _, s := range strings.Fields(scope) {
		if s == want {
			return true
		}
	}
	return false
}

// clientesConCampana son las superficies de la suite que pintan la campana del Hub y
// por eso reciben el claim `mailboxes` (ADR 0053): los BFF de las apps, las nativas y
// el propio Hub. Lista cerrada y no «todo client global»: también son globales la
// forja, la pasarela de integraciones o la API de DevX, y a ninguna le toca saber en
// qué otras organizaciones está alguien.
var clientesConCampana = map[string]bool{
	"sytue-one": true, "sytue-hub": true, "sytue-account": true, "sytue-mail": true,
	"sytue-drive": true, "sytue-calendar": true, "sytue-contacts": true, "sytue-docs": true,
	"sytue-notes": true, "sytue-syphotos": true, "sytue-chat": true, "sytue-meet": true,
	"sytue-console": true, "sytue-cloud": true, "sytue-syerp": true, "sytue-cast": true,
	"sytue-pages": true, "sytue-bases": true, "sytue-desktop": true, "sytue-mobile": true,
	"sytue-docus": true, "sytue-calcus": true, "sytue-diapos": true, "sytue-certipdf": true,
}
