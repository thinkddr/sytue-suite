// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// CRUD admin de clients OIDC por organización: registrar «a mano» desde Sytue Admin
// la app de terceros que va a iniciar sesión contra Sytue ID (el camino alternativo al
// registro dinámico de dcr.go, que exige que la app sepa hablar RFC 7591).
//
// Invariantes, iguales a las del DCR: PKCE obligatorio, scopes acotados al conjunto
// self-service (nunca admin/gestión), redirect_uris https exactos (anti open-redirect),
// secreto devuelto UNA sola vez (se persiste su hash Argon2id) y aislamiento por org
// deny-by-default (el org_id va en el WHERE; cross-org ⇒ 404). Sigue admin_apps.go.
package http

import (
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

// oidcClientRequest es el alta de un client desde la Console.
type oidcClientRequest struct {
	Name                   string   `json:"name"`
	RedirectURIs           []string `json:"redirect_uris"`
	PostLogoutRedirectURIs []string `json:"post_logout_redirect_uris"`
	Scopes                 []string `json:"scopes"`
	// Public: aplicación sin servidor (SPA, móvil/escritorio) — no puede guardar un
	// secreto, así que se registra como público (PKCE puro, auth method "none").
	Public bool `json:"public"`
}

// oidcClientWire es la forma JSON de un client para la Console. NUNCA lleva el
// secreto (has_secret indica si hay uno). Incluye los datos que el admin tiene que
// pegar en la app de terceros, derivados del MISMO documento de discovery que sirve
// el IdP: si cambia un endpoint, cambia aquí solo.
func (s *Server) oidcClientWire(c domain.Client) map[string]any {
	d := oidc.BuildDiscovery(strings.TrimRight(s.cfg.Issuer, "/"))
	return map[string]any{
		"client_id":                  c.ID,
		"name":                       c.Name,
		"type":                       string(c.Type),
		"redirect_uris":              c.RedirectURIs,
		"post_logout_redirect_uris":  c.PostLogoutRedirectURIs,
		"grant_types":                c.GrantTypes,
		"scopes":                     c.Scopes,
		"token_endpoint_auth_method": c.TokenEndpointAuthMethod,
		"require_pkce":               c.RequirePKCE,
		"has_secret":                 c.SecretHash != "",
		"created_at":                 c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		// Lo que se pega en la app de terceros:
		"issuer":                 d.Issuer,
		"discovery_url":          d.Issuer + "/.well-known/openid-configuration",
		"authorization_endpoint": d.AuthorizationEndpoint,
		"token_endpoint":         d.TokenEndpoint,
		"userinfo_endpoint":      d.UserinfoEndpoint,
		"jwks_uri":               d.JWKSURI,
		"end_session_endpoint":   d.EndSessionEndpoint,
	}
}

// handleAdminListOIDCClients — GET /api/v1/admin/apps/oidc-clients — clients de la org
// activa (los globales de la suite no son de ninguna org y no se listan). Sin secretos.
func (s *Server) handleAdminListOIDCClients(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	list, err := s.store.ListOrgClients(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	items := make([]map[string]any, 0, len(list))
	for _, c := range list {
		items = append(items, s.oidcClientWire(c))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// handleAdminCreateOIDCClient — POST /api/v1/admin/apps/oidc-clients. Devuelve el
// client_secret UNA sola vez (solo se guarda su hash).
func (s *Server) handleAdminCreateOIDCClient(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	var req oidcClientRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("name es obligatorio."))
		return
	}
	redirects, err := validatedRedirects(req.RedirectURIs)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(redirects) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"At least one redirect URL is required (https, without # or wildcards).",
		))
		return
	}
	// Las post-logout se validan igual: el end_session endpoint redirige a ellas.
	postLogout, err := validatedRedirects(req.PostLogoutRedirectURIs)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	client := domain.Client{
		ID:                     "app-" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		Name:                   domain.SanitizeDisplayName(name),
		Type:                   domain.ClientConfidential,
		RedirectURIs:           redirects,
		PostLogoutRedirectURIs: postLogout,
		GrantTypes:             []string{"authorization_code", "refresh_token"},
		ResponseTypes:          []string{"code"},
		// El conjunto self-service del DCR y, además, los scopes de Google que el
		// administrador marque. Nunca scopes de administración de la identidad.
		Scopes:                  scopesDeConsola(req.Scopes),
		RequirePKCE:             true,
		TokenEndpointAuthMethod: "client_secret_basic",
	}
	var secret string
	if req.Public {
		client.Type = domain.ClientPublic
		client.TokenEndpointAuthMethod = "none"
	} else {
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

	if err := s.store.CreateOAuthClient(r.Context(), nil, client, org.ID, "admin"); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "apps.oidc_client.created", client.ID, r))

	out := s.oidcClientWire(client)
	if secret != "" {
		out["client_secret"] = secret // única vez que se ve
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusCreated, out)
}

// handleAdminRotateOIDCClientSecret — POST /api/v1/admin/apps/oidc-clients/{id}/rotate-secret.
// Devuelve el secreto nuevo UNA sola vez; el anterior deja de valer al instante.
func (s *Server) handleAdminRotateOIDCClientSecret(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	client, err := s.loadOrgOIDCClient(r, org.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if client.IsPublic() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"This application is public (PKCE) and does not use a secret.",
		))
		return
	}
	secret, err := domain.GenerateToken(32)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	hash, err := domain.HashPassword(secret)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if err := s.store.SetClientSecretHash(r.Context(), nil, client.ID, hash); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "apps.oidc_client.secret_rotated", client.ID, r))
	client.SecretHash = hash
	out := s.oidcClientWire(client)
	out["client_secret"] = secret
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, out)
}

// handleAdminDeleteOIDCClient — DELETE /api/v1/admin/apps/oidc-clients/{id}. Cross-org ⇒ 404.
func (s *Server) handleAdminDeleteOIDCClient(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	id := chi.URLParam(r, "id")
	ok, err := s.store.DeleteOrgClient(r.Context(), org.ID, id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if !ok {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "apps.oidc_client.deleted", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"client_id": id, "deleted": true})
}

// loadOrgOIDCClient carga el client {id} acotado a la org (cross-org/inexistente ⇒ 404).
func (s *Server) loadOrgOIDCClient(r *http.Request, orgID string) (domain.Client, error) {
	c, err := s.store.GetOrgClient(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		return domain.Client{}, mapDomainErr(err)
	}
	return c, nil
}

// validatedRedirects normaliza y valida una lista de URLs de retorno con la MISMA
// regla que el registro dinámico (https exacto, sin fragmento ni comodines).
func validatedRedirects(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, u := range in {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if !validRegistrationRedirect(u) {
			return nil, httpx.ErrBadRequest.WithMessage("Invalid redirect URL: " + u)
		}
		out = append(out, u)
	}
	return out, nil
}

// scopesDeConsola es el conjunto self-service de siempre MÁS los scopes de APIs de
// Google que el administrador marque al registrar la app. Solo por esta vía: en el
// registro dinámico los scopes los elige la propia app, y un permiso sobre los datos
// de las personas de la organización lo tiene que decidir una persona.
func scopesDeConsola(pedidos []string) []string {
	out := filterDCRScopes(strings.Join(pedidos, " "))
	for _, sc := range pedidos {
		if _, ok := domain.GoogleScopes[sc]; ok && !slices.Contains(out, sc) {
			out = append(out, sc)
		}
	}
	return out
}
