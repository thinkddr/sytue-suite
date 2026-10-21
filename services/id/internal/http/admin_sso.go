// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Administración del inicio de sesión único (SSO / federación OIDC upstream) desde
// Sytue Admin. CRUD de los identity_providers de la organización ACTIVA del
// solicitante (claim tenant; requireRole ya exige owner/admin para esa org). Todo
// se acota por org (aislamiento tenant, fail-closed): un provider de otra org ⇒
// 404, nunca se toca. El client_secret solo se ESCRIBE: jamás se devuelve en claro
// (el wire expone has_secret, no el valor). Al crear/editar se resuelve el discovery
// del issuer (SSRF-safe) para cachear los endpoints; "probar" lo re-ejecuta.
package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/federation"
	"git.sytue.eu/sytue/one/services/id/internal/saml"
)

// providerWire es la forma JSON de un provider para la Console. NUNCA incluye el
// client_secret; has_secret indica si hay uno configurado. redirect_uri se calcula
// para que el admin lo registre en su IdP upstream.
func (s *Server) providerWire(p domain.IdentityProvider) map[string]any {
	// sp_entity_id efectivo: el configurado o, por defecto, la metadata URL (lo que
	// el admin debe registrar de audiencia en su IdP).
	spEntityID := p.SPEntityID
	if spEntityID == "" {
		spEntityID = s.samlMetadataURL(p.ID)
	}
	return map[string]any{
		"id":                     p.ID,
		"name":                   p.Name,
		"slug":                   p.Slug,
		"type":                   p.Type,
		"issuer":                 p.Issuer,
		"client_id":              p.ClientID,
		"has_secret":             p.ClientSecret != "",
		"scopes":                 p.Scopes,
		"attribute_mapping":      p.Mapping,
		"allowed_email_domains":  p.AllowedEmailDomains,
		"authorization_endpoint": p.AuthorizationEndpoint,
		"token_endpoint":         p.TokenEndpoint,
		"userinfo_endpoint":      p.UserinfoEndpoint,
		"jwks_uri":               p.JWKSURI,
		"pkce":                   p.PKCE,
		// SAML. idp_certificate es PÚBLICO (no un secreto): se devuelve para editar/ver.
		"idp_entity_id":   p.IDPEntityID,
		"idp_sso_url":     p.IDPSSOURL,
		"idp_certificate": p.IDPCertificate,
		"sp_entity_id":    spEntityID,
		"saml_binding":    defaultBinding(p.SAMLBinding),
		"name_id_format":  p.NameIDFormat,
		"acs_url":         s.samlACSURL(p.ID),
		"metadata_url":    s.samlMetadataURL(p.ID),
		"enabled":         p.Enabled,
		"configured":      p.Configured(),
		"redirect_uri":    s.federatedRedirectURI(p.ID),
		"created_at":      p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":      p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

func defaultBinding(b string) string {
	if b == "" {
		return "redirect"
	}
	return b
}

// handleAdminListProviders — GET /api/v1/admin/sso/providers — providers de la org
// activa (sin secreto).
func (s *Server) handleAdminListProviders(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	list, err := s.store.ListIdentityProviders(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	items := make([]map[string]any, 0, len(list))
	for _, p := range list {
		items = append(items, s.providerWire(p))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type ssoProviderRequest struct {
	Type                *string           `json:"type"` // 'oidc' (defecto) | 'saml'; inmutable tras crear
	Name                *string           `json:"name"`
	Slug                *string           `json:"slug"`
	AttributeMapping    map[string]string `json:"attribute_mapping"`
	AllowedEmailDomains []string          `json:"allowed_email_domains"`
	Enabled             *bool             `json:"enabled"`
	// OIDC:
	Issuer       *string `json:"issuer"`
	ClientID     *string `json:"client_id"`
	ClientSecret *string `json:"client_secret"`
	Scopes       *string `json:"scopes"`
	// SAML: o bien se pega la metadata XML del IdP, o bien los tres campos sueltos.
	MetadataXML    *string `json:"metadata_xml"`
	IDPEntityID    *string `json:"idp_entity_id"`
	IDPSSOURL      *string `json:"idp_sso_url"`
	IDPCertificate *string `json:"idp_certificate"`
	SPEntityID     *string `json:"sp_entity_id"`
	SAMLBinding    *string `json:"saml_binding"`
	NameIDFormat   *string `json:"name_id_format"`
}

// normalizeBinding acota el binding del AuthnRequest a 'redirect' (defecto) o 'post'.
func normalizeBinding(b string) string {
	if strings.ToLower(strings.TrimSpace(b)) == "post" {
		return "post"
	}
	return "redirect"
}

// normalizeMapping conserva solo las claves de atributo soportadas (email/name/
// groups) con nombres de claim no vacíos: no se guarda mapeo arbitrario.
func normalizeMapping(in map[string]string) map[string]string {
	out := map[string]string{}
	for _, k := range []string{"email", "name", "groups"} {
		if v, ok := in[k]; ok {
			if v = strings.TrimSpace(v); v != "" {
				out[k] = v
			}
		}
	}
	return out
}

// normalizeDomains baja a minúsculas, recorta y descarta vacíos/duplicados.
func normalizeDomains(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, d := range in {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		if _, dup := seen[d]; dup {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}

// ensureOpenID garantiza que el scope openid está presente (obligatorio OIDC).
func ensureOpenID(scopes string) string {
	scopes = strings.TrimSpace(scopes)
	if scopes == "" {
		return "openid email profile"
	}
	for _, s := range strings.Fields(scopes) {
		if s == "openid" {
			return scopes
		}
	}
	return "openid " + scopes
}

// discoveryHTTPError traduce un fallo de discovery a un 400 con mensaje sobrio (no
// filtra IPs internas ni detalle del upstream) o a un 502 si fue de red.
func discoveryHTTPError(err error) error {
	if errors.Is(err, federation.ErrScheme) {
		return httpx.ErrBadRequest.WithMessage("The issuer must be a public https URL.")
	}
	return httpx.ErrBadRequest.WithMessage("The provider configuration could not be discovered. Check the issuer.").Wrap(err)
}

// applySAMLConfig rellena los campos SAML de un provider desde la petición: si viene
// metadata_xml se parsea (entityID+SSO+cert), si no se toman los tres campos sueltos.
// Valida la config (EntityID, SSO https, cert que parsea) — sin red. Devuelve un
// error HTTP 400 con mensaje sobrio si la config no es válida.
func applySAMLConfig(p *domain.IdentityProvider, req ssoProviderRequest) error {
	if md := strings.TrimSpace(strDeref(req.MetadataXML)); md != "" {
		eid, sso, cert, err := saml.ParseMetadataXML([]byte(md))
		if err != nil {
			return httpx.ErrBadRequest.WithMessage("The IdP XML metadata could not be read (entityID, SSO URL and signing certificate).").Wrap(err)
		}
		p.IDPEntityID, p.IDPSSOURL, p.IDPCertificate = eid, sso, cert
	} else {
		if req.IDPEntityID != nil {
			p.IDPEntityID = strings.TrimSpace(*req.IDPEntityID)
		}
		if req.IDPSSOURL != nil {
			p.IDPSSOURL = strings.TrimSpace(*req.IDPSSOURL)
		}
		if req.IDPCertificate != nil {
			p.IDPCertificate = strings.TrimSpace(*req.IDPCertificate)
		}
	}
	if req.SPEntityID != nil {
		p.SPEntityID = strings.TrimSpace(*req.SPEntityID)
	}
	if req.SAMLBinding != nil {
		p.SAMLBinding = normalizeBinding(*req.SAMLBinding)
	}
	if req.NameIDFormat != nil {
		p.NameIDFormat = strings.TrimSpace(*req.NameIDFormat)
	}
	if err := saml.ValidateConfig(*p); err != nil {
		return httpx.ErrBadRequest.WithMessage("Check the SAML configuration: IdP EntityID, SSO URL (https) and signing certificate.").Wrap(err)
	}
	return nil
}

// handleAdminCreateProvider — POST /api/v1/admin/sso/providers — crea un provider
// en la org activa. Resuelve el discovery del issuer (SSRF-safe) y cachea los
// endpoints; si el discovery falla, no se crea (config inservible).
func (s *Server) handleAdminCreateProvider(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	var req ssoProviderRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	typ := strings.ToLower(strings.TrimSpace(strDeref(req.Type)))
	if typ == "" {
		typ = "oidc"
	}
	if typ != "oidc" && typ != "saml" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("type must be 'oidc' or 'saml'."))
		return
	}
	p := domain.IdentityProvider{
		OrgID:               org.ID,
		Type:                typ,
		Name:                strings.TrimSpace(strDeref(req.Name)),
		Slug:                strings.ToLower(strings.TrimSpace(strDeref(req.Slug))),
		Mapping:             normalizeMapping(req.AttributeMapping),
		AllowedEmailDomains: normalizeDomains(req.AllowedEmailDomains),
		CreatedBy:           c.Subject,
		Enabled:             req.Enabled != nil && *req.Enabled,
	}
	if p.Name == "" || !tenancy.ValidSlug(p.Slug) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("name and slug (lowercase) are required."))
		return
	}
	if typ == "saml" {
		if err := applySAMLConfig(&p, req); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	} else {
		p.Issuer = strings.TrimRight(strings.TrimSpace(strDeref(req.Issuer)), "/")
		p.ClientID = strings.TrimSpace(strDeref(req.ClientID))
		p.ClientSecret = strDeref(req.ClientSecret)
		p.Scopes = ensureOpenID(strDeref(req.Scopes))
		if p.Issuer == "" || p.ClientID == "" {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("issuer and client_id are required for OIDC."))
			return
		}
		// Discovery del issuer (SSRF-safe): cachea endpoints y detecta soporte PKCE.
		eps, derr := s.fed.Discover(r.Context(), p.Issuer)
		if derr != nil {
			httpx.Fail(w, r, discoveryHTTPError(derr))
			return
		}
		applyEndpoints(&p, eps)
	}
	created, err := s.store.CreateIdentityProvider(r.Context(), nil, p)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err)) // ErrConflict (slug repetido) ⇒ 409
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "sso.provider.created", created.Slug, r))
	httpx.JSON(w, r, http.StatusCreated, s.providerWire(created))
}

// handleAdminGetProvider — GET /api/v1/admin/sso/providers/{id} — un provider de la
// org activa (sin secreto). Cross-org ⇒ 404.
func (s *Server) handleAdminGetProvider(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := s.loadOrgProvider(r, org.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p.ClientSecret = "" // defensa en profundidad: el GET nunca expone el secreto.
	httpx.JSON(w, r, http.StatusOK, s.providerWire(p))
}

// handleAdminUpdateProvider — PATCH /api/v1/admin/sso/providers/{id} — edita un
// provider de la org activa. Si cambia el issuer, re-ejecuta discovery. El secreto
// solo se re-escribe si viene en el cuerpo.
//
//nolint:gocognit,gocyclo // PATCH parcial: cada campo opcional del proveedor SSO trae su validación; el flujo es una lista, no un anidamiento.
func (s *Server) handleAdminUpdateProvider(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	p, err := s.loadOrgProvider(r, org.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req ssoProviderRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.Name != nil {
		p.Name = strings.TrimSpace(*req.Name)
	}
	if req.Slug != nil {
		p.Slug = strings.ToLower(strings.TrimSpace(*req.Slug))
	}
	if req.AttributeMapping != nil {
		p.Mapping = normalizeMapping(req.AttributeMapping)
	}
	if req.AllowedEmailDomains != nil {
		p.AllowedEmailDomains = normalizeDomains(req.AllowedEmailDomains)
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	// El tipo es inmutable: cada rama solo toca sus propios campos.
	if p.IsSAML() {
		// Se re-valida la config SAML solo si se van a tocar sus campos o si se
		// activa (permite guardar borradores sin cert). ValidateConfig no hace red.
		touchesSAML := req.MetadataXML != nil || req.IDPEntityID != nil ||
			req.IDPSSOURL != nil || req.IDPCertificate != nil
		if touchesSAML {
			if err := applySAMLConfig(&p, req); err != nil {
				httpx.Fail(w, r, err)
				return
			}
		} else {
			if req.SPEntityID != nil {
				p.SPEntityID = strings.TrimSpace(*req.SPEntityID)
			}
			if req.SAMLBinding != nil {
				p.SAMLBinding = normalizeBinding(*req.SAMLBinding)
			}
			if req.NameIDFormat != nil {
				p.NameIDFormat = strings.TrimSpace(*req.NameIDFormat)
			}
			if p.Enabled {
				if err := saml.ValidateConfig(p); err != nil {
					httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Check the SAML configuration before enabling it: EntityID, SSO URL (https) and signing certificate.").Wrap(err))
					return
				}
			}
		}
	} else {
		if req.ClientID != nil {
			p.ClientID = strings.TrimSpace(*req.ClientID)
		}
		if req.Scopes != nil {
			p.Scopes = ensureOpenID(*req.Scopes)
		}
		// Cambio de issuer ⇒ re-descubrir endpoints (SSRF-safe).
		if req.Issuer != nil {
			newIssuer := strings.TrimRight(strings.TrimSpace(*req.Issuer), "/")
			if newIssuer != p.Issuer {
				p.Issuer = newIssuer
				eps, derr := s.fed.Discover(r.Context(), p.Issuer)
				if derr != nil {
					httpx.Fail(w, r, discoveryHTTPError(derr))
					return
				}
				applyEndpoints(&p, eps)
			}
		}
		if p.Issuer == "" || p.ClientID == "" {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("issuer and client_id cannot be left empty."))
			return
		}
	}
	if p.Name == "" || !tenancy.ValidSlug(p.Slug) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("name and slug cannot be left empty."))
		return
	}
	// Un provider habilitado DEBE estar configurado (o el flujo no arranca).
	if p.Enabled && !p.Configured() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Complete and test the configuration before enabling the provider."))
		return
	}
	updated, err := s.store.UpdateIdentityProvider(r.Context(), nil, p, req.ClientSecret)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "sso.provider.updated", updated.Slug, r))
	updated.ClientSecret = ""
	httpx.JSON(w, r, http.StatusOK, s.providerWire(updated))
}

// handleAdminTestProvider — POST /api/v1/admin/sso/providers/{id}/test — prueba la
// config. OIDC: re-ejecuta el discovery del issuer (SSRF-safe) y cachea endpoints.
// SAML: valida la config (EntityID, SSO https, cert que parsea) sin red.
func (s *Server) handleAdminTestProvider(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	p, err := s.loadOrgProvider(r, org.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if p.IsSAML() {
		if err := saml.ValidateConfig(p); err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The SAML configuration is not valid: check EntityID, SSO URL (https) and signing certificate.").Wrap(err))
			return
		}
		_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "sso.provider.tested", p.Slug, r))
		p.ClientSecret = ""
		httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": true, "provider": s.providerWire(p)})
		return
	}
	eps, derr := s.fed.Discover(r.Context(), p.Issuer)
	if derr != nil {
		httpx.Fail(w, r, discoveryHTTPError(derr))
		return
	}
	applyEndpoints(&p, eps)
	updated, err := s.store.UpdateIdentityProvider(r.Context(), nil, p, nil)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "sso.provider.tested", updated.Slug, r))
	updated.ClientSecret = ""
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"ok":       true,
		"pkce":     eps.PKCE,
		"provider": s.providerWire(updated),
	})
}

// handleAdminDeleteProvider — DELETE /api/v1/admin/sso/providers/{id} — borra un
// provider de la org activa (cascada: enlaces y estados). Cross-org ⇒ 404.
func (s *Server) handleAdminDeleteProvider(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	id := chi.URLParam(r, "id")
	if _, perr := uuid.Parse(id); perr != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	ok, err := s.store.DeleteIdentityProvider(r.Context(), org.ID, id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if !ok {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "sso.provider.deleted", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "deleted": true})
}

// loadOrgProvider carga el provider {id} acotado a la org (cross-org/inexistente ⇒
// ErrNotFound HTTP, sin revelar su existencia).
func (s *Server) loadOrgProvider(r *http.Request, orgID string) (domain.IdentityProvider, error) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		return domain.IdentityProvider{}, httpx.ErrNotFound
	}
	p, err := s.store.GetIdentityProvider(r.Context(), orgID, id)
	if err != nil {
		return domain.IdentityProvider{}, mapDomainErr(err) // ErrNotFound ⇒ 404
	}
	return p, nil
}

func applyEndpoints(p *domain.IdentityProvider, eps federation.Endpoints) {
	p.AuthorizationEndpoint = eps.Authorization
	p.TokenEndpoint = eps.Token
	p.UserinfoEndpoint = eps.Userinfo
	p.JWKSURI = eps.JWKS
	p.PKCE = eps.PKCE
}

func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
