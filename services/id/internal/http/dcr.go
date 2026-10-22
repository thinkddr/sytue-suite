// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Registro dinámico de clients OIDC (RFC 7591). Una app de terceros se registra por
// sí misma con un TOKEN DE ACCESO INICIAL emitido por un admin de la organización
// (POST /api/v1/admin/apps/dcr-tokens). El client resultante queda ACOTADO A LA ORG del
// token (org_id), con require_pkce y scopes limitados a un conjunto self-service seguro
// (nunca admin/gestión): un DCR no puede auto-otorgarse privilegios.
package http

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// dcrAllowedScopes acota los scopes que un client registrado dinámicamente puede pedir:
// solo el conjunto self-service (login + refresh). Nunca admin/directorio/gestión.
var dcrAllowedScopes = map[string]struct{}{
	"openid": {}, "profile": {}, "email": {}, "offline_access": {},
}

// dcrRequest son los metadatos de cliente de RFC 7591 §2 que soportamos.
type dcrRequest struct {
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope"`
	PostLogoutRedirectURIs  []string `json:"post_logout_redirect_uris"`
}

// dcrError escribe un error de registro (RFC 7591 §3.2.2).
func dcrError(w http.ResponseWriter, r *http.Request, status int, code, desc string) {
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, status, map[string]string{"error": code, "error_description": desc})
}

// handleDCRRegister implementa POST /oauth2/register (RFC 7591): valida el token de
// acceso inicial de la org, crea el client org-scoped y devuelve client_id/secret UNA
// sola vez.
func (s *Server) handleDCRRegister(w http.ResponseWriter, r *http.Request) {
	tok, ok := bearer(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="sytue-id"`)
		dcrError(w, r, http.StatusUnauthorized, "invalid_token", "falta el token de acceso inicial")
		return
	}
	orgID, err := s.store.OrgForDCRToken(r.Context(), domain.HashToken(tok), time.Now())
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="sytue-id", error="invalid_token"`)
		dcrError(w, r, http.StatusUnauthorized, "invalid_token", "token de acceso inicial inválido o caducado")
		return
	}

	var req dcrRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	grants := normalizeDCRGrants(req.GrantTypes)
	public := strings.EqualFold(strings.TrimSpace(req.TokenEndpointAuthMethod), "none")

	// redirect_uris obligatorios si hay authorization_code; validación exacta https
	// (o http://localhost en dev). Anti open-redirect: sin comodines ni fragmentos.
	needsRedirects := slices.Contains(grants, "authorization_code")
	redirects := make([]string, 0, len(req.RedirectURIs))
	for _, u := range req.RedirectURIs {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if !validRegistrationRedirect(u) {
			dcrError(w, r, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uri inválido: "+u)
			return
		}
		redirects = append(redirects, u)
	}
	if needsRedirects && len(redirects) == 0 {
		dcrError(w, r, http.StatusBadRequest, "invalid_redirect_uri", "se requiere al menos un redirect_uri para authorization_code")
		return
	}

	// Los post_logout se validaban… en la consola (admin_oidc_clients.go) y aquí no:
	// se copiaban tal cual del cuerpo. Y el end_session endpoint devuelve el destino
	// registrado VERBATIM —a `http.Redirect` y al `href`/`location.replace` de la
	// página de barrido—, así que un client registrado por sí mismo se apuntaba
	// cualquier cosa: un open redirect desde el dominio del IdP, y con un
	// `javascript:` un script en el origen donde se teclean las contraseñas. Misma
	// verja que los redirect_uri (https absoluto, o http://localhost en dev).
	postLogout, err := validatedRedirects(req.PostLogoutRedirectURIs)
	if err != nil {
		dcrError(w, r, http.StatusBadRequest, "invalid_client_metadata",
			"post_logout_redirect_uri inválido")
		return
	}

	scopes := filterDCRScopes(req.Scope)
	authMethod := "client_secret_basic"
	clientType := domain.ClientConfidential
	if public {
		authMethod = "none"
		clientType = domain.ClientPublic
	}

	client := domain.Client{
		ID:                      "dcr-" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		Name:                    dcrClientName(req.ClientName),
		Type:                    clientType,
		RedirectURIs:            redirects,
		PostLogoutRedirectURIs:  postLogout,
		GrantTypes:              grants,
		ResponseTypes:           normalizeDCRResponseTypes(grants),
		Scopes:                  scopes,
		RequirePKCE:             true, // PKCE obligatorio para todo client registrado.
		TokenEndpointAuthMethod: authMethod,
	}

	// Secreto solo para clients confidenciales; se devuelve UNA vez (se guarda su hash).
	var secret string
	if !public {
		secret, err = domain.GenerateToken(32)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
		hash, herr := domain.HashPassword(secret)
		if herr != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(herr))
			return
		}
		client.SecretHash = hash
	}

	if err := s.store.CreateOAuthClient(r.Context(), nil, client, orgID, "dcr"); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry("", orgID, "oauth.client.registered", client.ID, r))

	out := map[string]any{
		"client_id":                  client.ID,
		"client_id_issued_at":        time.Now().Unix(),
		"client_secret_expires_at":   0, // no expira
		"redirect_uris":              client.RedirectURIs,
		"grant_types":                client.GrantTypes,
		"response_types":             client.ResponseTypes,
		"token_endpoint_auth_method": client.TokenEndpointAuthMethod,
		"client_name":                client.Name,
		"scope":                      strings.Join(client.Scopes, " "),
	}
	if secret != "" {
		out["client_secret"] = secret
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusCreated, out)
}

// normalizeDCRGrants acota los grant types a los soportados; por defecto
// authorization_code + refresh_token. client_credentials NO se admite por DCR (es
// máquina-a-máquina; se aprovisiona por admin).
func normalizeDCRGrants(in []string) []string {
	allowed := map[string]struct{}{"authorization_code": {}, "refresh_token": {}}
	out := make([]string, 0, 2)
	for _, g := range in {
		if _, ok := allowed[g]; ok && !slices.Contains(out, g) {
			out = append(out, g)
		}
	}
	if len(out) == 0 {
		return []string{"authorization_code", "refresh_token"}
	}
	return out
}

// normalizeDCRResponseTypes deriva los response types de los grants (code si hay
// authorization_code).
func normalizeDCRResponseTypes(grants []string) []string {
	if slices.Contains(grants, "authorization_code") {
		return []string{"code"}
	}
	return []string{}
}

// filterDCRScopes intersecta los scopes pedidos con el conjunto self-service permitido;
// vacío ⇒ el conjunto por defecto (login + refresh).
func filterDCRScopes(scope string) []string {
	out := make([]string, 0, 4)
	for _, sc := range strings.Fields(scope) {
		if _, ok := dcrAllowedScopes[sc]; ok && !slices.Contains(out, sc) {
			out = append(out, sc)
		}
	}
	if len(out) == 0 {
		return []string{"openid", "profile", "email", "offline_access"}
	}
	return out
}

// validRegistrationRedirect exige una URL absoluta https (o http://localhost en dev),
// con host y sin fragmento (OAuth2 BCP: coincidencia exacta, sin comodines).
func validRegistrationRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Fragment != "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme == "http" {
		host := u.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	}
	return false
}

func dcrClientName(name string) string {
	if n := strings.TrimSpace(name); n != "" {
		return domain.SanitizeDisplayName(n)
	}
	return "App registrada"
}
