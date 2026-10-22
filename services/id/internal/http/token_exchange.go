// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

// Tipos de token de RFC 8693 §3 que este AS acepta como sujeto y emite.
const (
	//nolint:gosec // G101: identificador del estándar, no una credencial.
	tokenTypeAccess = "urn:ietf:params:oauth:token-type:access_token"
	//nolint:gosec // G101: identificador del estándar, no una credencial.
	tokenTypeJWT = "urn:ietf:params:oauth:token-type:jwt"
)

// exchangeParams son los parámetros del canje ya extraídos del formulario.
//
// subject_token es el access token del PROPIO client (obtenido por
// client_credentials): es él quien presenta su identidad. A quién quiere
// representar va en requested_subject — RFC 8693 no define un parámetro para
// "actúa como este usuario" (su subject_token asume que ya tienes un token del
// usuario, que es justo lo que una migración no tiene), así que se usa el
// parámetro adicional que el propio §2.1 contempla, con el nombre de facto del
// ecosistema. Acepta el email o el uuid del usuario.
type exchangeParams struct {
	SubjectToken string
	Subject      string
	Audience     string
	Scope        string
}

// parseExchange valida la FORMA de la petición (RFC 8693 §2.1) y devuelve la
// descripción del `invalid_request` si algo falta o no está soportado (cadena
// vacía = petición bien formada).
func parseExchange(form url.Values) (exchangeParams, string) {
	p := exchangeParams{
		SubjectToken: strings.TrimSpace(form.Get("subject_token")),
		Subject:      strings.TrimSpace(form.Get("requested_subject")),
		Audience:     strings.TrimSpace(form.Get("audience")),
		Scope:        strings.TrimSpace(form.Get("scope")),
	}
	switch {
	case p.SubjectToken == "":
		return p, "falta subject_token"
	// subject_token_type es OBLIGATORIO (§2.1): sin él no sabemos qué nos dan.
	case !supportedTokenType(form.Get("subject_token_type"), false):
		return p, "subject_token_type no soportado"
	// requested_token_type es opcional; por defecto access token. No se emiten
	// ID tokens ni SAML por canje: pedirlos es un error, no un silencio.
	case !supportedTokenType(form.Get("requested_token_type"), true):
		return p, "requested_token_type no soportado"
	case p.Subject == "":
		return p, "falta requested_subject"
	}
	return p, ""
}

// supportedTokenType acepta los dos URN de token de acceso JWT de RFC 8693 §3.
// allowEmpty distingue el parámetro obligatorio del opcional.
func supportedTokenType(v string, allowEmpty bool) bool {
	switch v {
	case tokenTypeAccess, tokenTypeJWT:
		return true
	case "":
		return allowEmpty
	default:
		return false
	}
}

// grantTokenExchange implementa el canje RFC 8693 con IMPERSONACIÓN ACOTADA: un
// client de confianza obtiene un access token que actúa como un usuario de SU
// organización (`sub` del usuario, `act.sub` del client), con TTL de minutos y
// scopes recortados. Es lo que permite que una migración desde M365/Google
// escriba en la cuenta de CADA usuario y no en la del admin que lanzó el trabajo.
//
// Todo es DENY-BY-DEFAULT y en este orden, que importa:
//
//  1. el client debe ser delegable (mig 0025). Se comprueba ANTES que nada para
//     que un client cualquiera no pueda usar el endpoint como oráculo de qué
//     tokens son válidos ni de qué usuarios existen;
//  2. la petición debe estar bien formada;
//  3. el subject_token debe ser un access token VIVO de este IdP y del PROPIO
//     client, y no llevar ya `act` (un token de impersonación NO se re-canjea:
//     la delegación no se encadena ni se lava el actor original);
//  4. el usuario objetivo debe ser miembro ACTIVO de la organización del client
//     y no ser owner/admin salvo declaración explícita;
//  5. la audiencia debe ser un client conocido de la suite o de la misma org;
//  6. el scope resultante es la intersección de lo pedido con lo del client,
//     menos lo no delegable y lo que el usuario no tendría;
//  7. la traza se escribe ANTES de responder. Si no hay traza, no hay token.
//
// Los rechazos de (4) y (5) devuelven el MISMO invalid_target con el MISMO texto:
// desde fuera no se distingue "ese usuario no existe" de "existe pero es de otra
// organización" de "existe y es admin". El endpoint no enumera el directorio.
func (s *Server) grantTokenExchange(w http.ResponseWriter, r *http.Request, client domain.Client) {
	if err := domain.AuthorizeImpersonationClient(client); err != nil {
		tokenError(w, r, http.StatusBadRequest, "unauthorized_client",
			"el client no puede actuar en nombre de un usuario")
		return
	}
	if client.OrgID == "" || !s.orgEntitled(r.Context(), client.OrgID) {
		tokenError(w, r, http.StatusBadRequest, "unauthorized_client",
			"el client no puede actuar en nombre de un usuario")
		return
	}
	p, bad := parseExchange(r.Form)
	if bad != "" {
		tokenError(w, r, http.StatusBadRequest, "invalid_request", bad)
		return
	}

	set, err := s.keys.PublicJWKS()
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	tok, err := oidc.Verify(p.SubjectToken, s.cfg.Issuer, s.cfg.ResourceAudience, set)
	if err != nil {
		tokenError(w, r, http.StatusBadRequest, "invalid_grant", "subject_token inválido o expirado")
		return
	}
	if _, delegated := oidc.ActorOf(tok); delegated || tok.Subject() != client.ID {
		tokenError(w, r, http.StatusBadRequest, "invalid_grant", "subject_token inválido o expirado")
		return
	}

	ctx := r.Context()
	user, role, err := s.impersonationTarget(ctx, client.OrgID, p.Subject)
	if err == nil {
		err = domain.AuthorizeImpersonationTarget(client, user, role)
	}
	if err != nil {
		// Un client delegable pidiendo un sujeto fuera de su alcance es la señal
		// que interesa a las 3 de la mañana. Best-effort y SIN el identificador
		// pedido (sería un email): org + client + motivo bastan para alertar.
		_ = s.store.Audit(ctx, nil, auditEntry("", client.OrgID,
			"oauth.token.exchange.denied", client.ID, r))
		tokenError(w, r, http.StatusBadRequest, "invalid_target",
			"el sujeto solicitado no es impersonable por este client")
		return
	}

	// audience (§2.1): por defecto el propio client. Si se pide otra, debe ser un
	// client REAL de la suite (global) o de la misma organización — nunca uno de
	// otro tenant, que convertiría el canje en un puente entre organizaciones.
	aud := client.ID
	if p.Audience != "" {
		target, aerr := s.store.GetClient(ctx, p.Audience)
		if aerr != nil || (target.OrgID != "" && target.OrgID != client.OrgID) {
			tokenError(w, r, http.StatusBadRequest, "invalid_target", "audience desconocida")
			return
		}
		aud = target.ID
	}

	scope := domain.ImpersonationScope(p.Scope, client, role)
	if scope == "" {
		tokenError(w, r, http.StatusBadRequest, "invalid_scope", "ningún scope solicitado es delegable")
		return
	}

	now := time.Now()
	claims := s.userClaims(ctx, client, user, client.OrgID, scope, now)
	claims.Audience = aud
	claims.Actor = client.ID
	claims.TTL = domain.ImpersonationTokenTTL
	// Sin `amr` ni `auth_time`: el usuario NO se ha autenticado en este canje. Un
	// token de impersonación no debe poder pasar por una sesión suya reciente.
	at, err := s.issuer.IssueAccessToken(claims, now)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	ttl := s.issuer.EffectiveAccessTokenTTL(domain.ImpersonationTokenTTL)

	// AUDITORÍA BLOQUEANTE. actor_id es el usuario impersonado a propósito: es la
	// identidad que el token lleva y bajo la que aparecerá TODO lo que se haga con
	// él en los demás servicios, así que filtrando por él sale la historia
	// completa. Quién actuó de verdad va en target y en el detalle.
	e := auditEntry(user.ID, client.OrgID, "oauth.token.exchange", client.ID, r)
	e.Detail["actor_client"] = client.ID
	e.Detail["impersonated_user"] = user.ID
	e.Detail["scope"] = scope
	e.Detail["audience"] = aud
	e.Detail["expires_in"] = int(ttl.Seconds())
	if err := s.store.Audit(ctx, nil, e); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}

	writeTokens(w, r, &tokenResponse{
		AccessToken:     at,
		IssuedTokenType: tokenTypeAccess,
		TokenType:       "Bearer",
		ExpiresIn:       int(ttl.Seconds()),
		Scope:           scope,
	})
}

// impersonationTarget resuelve el usuario objetivo DENTRO de la organización del
// client: por email o por uuid. La pertenencia se consulta con el org_id del
// client en el WHERE (no se filtra en memoria), así que un usuario de otra
// organización es indistinguible de uno inexistente — los dos son ErrNotFound.
func (s *Server) impersonationTarget(ctx context.Context, orgID, ref string) (domain.User, domain.Role, error) {
	var (
		user domain.User
		err  error
	)
	switch {
	case strings.Contains(ref, "@"):
		user, err = s.store.GetUserByEmail(ctx, ref)
	case uuid.Validate(ref) == nil:
		user, err = s.store.GetUserByID(ctx, ref)
	default:
		return domain.User{}, "", domain.ErrNotFound
	}
	if err != nil {
		return domain.User{}, "", err
	}
	m, err := s.store.GetMembership(ctx, orgID, user.ID)
	if err != nil {
		return domain.User{}, "", err
	}
	return user, m.Role, nil
}
